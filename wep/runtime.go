package wep

import (
	"fmt"
	"math/rand"
	"strings"
)

// The stepper walks a compiled state machine. It knows nothing about drawing,
// sound or the world: ops that do something out there are handed back from
// Update for the game to act on, which keeps this testable without a window and
// keeps the game free of anything ZDoom shaped.

// tics per second. Doom's clock, and the unit every compiled duration is in.
const TicRate = 35.0

// maxStepsPerUpdate bounds how many zero length steps a machine may walk
// through in one update. Compiled weapons are checked on load and should not
// need it, but the weapons folder is editable and a jump that closes a circle
// of zero tic steps would otherwise hang the game rather than complain.
//
// It is ZDoom's own ten thousand, and it has to be: a zero length loop is how
// a reload is written. Brutal Doom's dual MP40 refills its magazine one round
// at a time through a ten step loop it runs sixty four times, all inside one
// tic. At five hundred it ran out a fifth of the way through, the cursor
// stopped, and the weapon went invisible and could not be fired again -
// which reads as a missing frame and is not one.
const maxStepsPerUpdate = 10000

// ticEpsilon is the slack in the tic countdown. A frame time of a whole tic
// does not multiply back to exactly one, so without it a two tic frame is
// occasionally three.
const ticEpsilon = 1e-6

// WeaponTop and WeaponBottom are where a weapon is held and where it is
// carried while it is off the bottom of the screen, in the 320x200 space
// GZDoom positions every weapon sprite in. A weapon's own frames move it
// between and around them with offset(), which is how a recoil or a swing is
// written; RaiseSpeed is how far A_Raise and A_Lower travel in one tic, so a
// swap takes the sixteen tics it takes in Doom.
const (
	WeaponTop    = 32.0
	WeaponBottom = 128.0
	RaiseSpeed   = 6.0
)

// Inventory is what the player is carrying: reserve ammo, rounds chambered, and
// the counters scripts use as flags. One map for all three, because in ZDoom
// they are all the same thing.
type Inventory map[string]int

// Caps are the maximum amounts, compiled from each item's Inventory.MaxAmount.
type Caps map[string]int

// Give adds to an item without going over its cap.
func (inv Inventory) Give(caps Caps, item string, amount int) {
	max, capped := caps[item]
	total := inv[item] + amount
	if capped && total > max {
		total = max
	}
	inv[item] = total
}

// Take removes from an item without going below nothing.
func (inv Inventory) Take(item string, amount int) {
	total := inv[item] - amount
	if total < 0 {
		total = 0
	}
	inv[item] = total
}

// AtMax reports whether an item is at its cap, which is what an amount of zero
// in a compiled jump_if_inventory tests.
func (inv Inventory) AtMax(caps Caps, item string) bool {
	max, capped := caps[item]
	if !capped {
		return false
	}
	return inv[item] >= max
}

// Input is what the player is doing this frame.
type Input struct {
	Trigger bool
	// Alt is ZDoom's BT_ALTATTACK: the second attack button. A weapon with an
	// AltFire state of its own is a different gun on it - a scope, a bayonet,
	// a grenade under the barrel - and a weapon without one ignores it.
	Alt    bool
	Reload bool
}

// World answers the questions a script asks about its surroundings. Every one
// of them is a jump, so the answer has to come back now rather than as
// something for the game to do later.
//
// A game that has not built the thing being asked about answers false, which is
// what NoWorld does. That is not the same as dropping the op: the jump is still
// there, still compiled, and starts working the day the answer does.
type World interface {
	// Below and Above report whether the actor is standing on something or
	// underneath something.
	Below() bool
	Above() bool
	// CanSee reports whether the actor has a target it can see.
	CanSee() bool
	// TargetWithin reports whether the actor's target is closer than this.
	TargetWithin(distance float64) bool
	// TargetHealthBelow reports whether the target is hurt past this.
	TargetHealthBelow(health int) bool
	// TargetHas reports whether the target is carrying this.
	TargetHas(item string, amount int) bool
	// HasFlag reports whether the actor has one of ZDoom's flags set.
	HasFlag(name string) bool
}

// NoWorld answers no to everything, which is the honest answer in a game with
// no monsters, no targets and no actor standing anywhere.
type NoWorld struct{}

func (NoWorld) Below() bool                { return false }
func (NoWorld) Above() bool                { return false }
func (NoWorld) CanSee() bool               { return false }
func (NoWorld) TargetWithin(float64) bool  { return false }
func (NoWorld) TargetHealthBelow(int) bool { return false }
func (NoWorld) TargetHas(string, int) bool { return false }
func (NoWorld) HasFlag(string) bool        { return false }

// Cursor is one layer's position in a state machine. A weapon runs two: its own
// animation, and the flash overlay A_GunFlash starts alongside it.
type Cursor struct {
	machine *StateMachine
	// labelStarts[i] is where the label covering step i begins, which is where
	// a Loop goes back to.
	labelStarts []int

	index    int
	ticsLeft float64
	// resting is set once the current step's ops have run and it is only
	// waiting out its duration.
	resting bool
	// holding is set by a duration of -1 and by a Wait: the cursor stays on
	// this frame until something moves it. Kept apart from ticsLeft so a long
	// frame time cannot look like one.
	holding bool
	// stopped is set by a Stop, and by a machine that has nowhere left to go.
	stopped bool
	// psx and psy are where this layer holds its sprite, in the 320x200 space
	// GZDoom draws weapons in: the psprite position. They persist from frame
	// to frame the way ZDoom's do rather than resetting - a frame that writes
	// offset() moves the gun and it stays moved until another frame, or
	// A_WeaponReady, puts it back - which is the whole of how a recoil that
	// the script never mentions again still ends.
	psx, psy float64
	// ready mirrors the weapon_ready op on the step being rested on, so input
	// is only accepted where the script says it is.
	ready Op
	// onReady is set while the rested-on step carries a weapon_ready.
	onReady bool
	// bobs is whether the weapon sways with the player's walk. ZDoom keeps it
	// on the layer rather than the frame: A_WeaponReady turns it on, or off
	// for WRF_NOBOB, and it stays that way until the next one - so a reload
	// that asked for no bob holds still for all of its frames and not only
	// the one that said so.
	bobs bool
	// sprite is the picture this layer is showing, which is what a "####"
	// frame keeps. ZDoom holds it on the actor and the psprite rather than on
	// the state, and only a state that names a sprite writes it - which is why
	// it survives a jump into somebody else's states. It starts empty: a fresh
	// psprite's sprite is SPR_TNT1, so a "####" with nothing before it shows
	// nothing.
	sprite string
}

// NewCursor starts a cursor at a label, holding its sprite where a weapon is
// held. An overlay reads its own position as a shift from the weapon's rather
// than an absolute one, so NewOverlayCursor starts at nothing instead.
func NewCursor(machine *StateMachine, label string) *Cursor {
	c := newCursor(machine, label)
	c.psy = WeaponTop
	return c
}

// NewOverlayCursor starts a cursor for a layer drawn over the weapon: a muzzle
// flash. ZDoom adds the weapon's own position to an overlay's, so a flash
// authored on top of the gun's frames follows it through a recoil for free.
func NewOverlayCursor(machine *StateMachine, label string) *Cursor {
	return newCursor(machine, label)
}

func newCursor(machine *StateMachine, label string) *Cursor {
	c := &Cursor{machine: machine, labelStarts: labelStarts(machine), bobs: true}
	c.stopped = true
	c.Goto(label)
	return c
}

// labelStarts works out, for every step, which label it belongs to. ZDoom's
// Loop goes back to the label the state sits under, and states fall through
// from one label into the next, so this is a scan rather than a lookup.
//
// A label beginning with "@" is not one of the mod's: it is somewhere the
// compiler had to be able to name - the landing spot of a `Goto Ready+6`, or
// the engine's own stop - and it does not begin a state. Counted as one, a
// `Loop` at the bottom of a Ready block whose middle is the target of some
// other state's `Goto Ready+6` goes back to the sixth line instead of the
// first, and misses the one frame in the block that costs any time. Brutal
// Doom's MP40 did exactly that: ten thousand steps an update, none of them
// taking a tic, the cursor abandoned wherever it happened to stop - which is a
// frame with a picture about two thirds of the time. The gun flickered.
func labelStarts(machine *StateMachine) []int {
	starts := make([]int, len(machine.Steps))
	isStart := make(map[int]bool, len(machine.Labels))
	for name, index := range machine.Labels {
		if strings.HasPrefix(name, "@") {
			continue
		}
		isStart[index] = true
	}
	current := 0
	for i := range starts {
		if isStart[i] {
			current = i
		}
		starts[i] = current
	}
	return starts
}

// Goto moves the cursor to a label. A label that is not there leaves the cursor
// where it was and reports false; loading rejects weapons that need this, so it
// only happens to hand-edited data.
func (c *Cursor) Goto(label string) bool {
	index, ok := c.machine.Label(label)
	if !ok {
		return false
	}
	c.jump(index)
	return true
}

func (c *Cursor) jump(index int) {
	c.index = index
	// Leaving a frame early gives up whatever it had left, but not the debt:
	// a negative count is time already spent that the next frame owes back.
	if c.ticsLeft > 0 {
		c.ticsLeft = 0
	}
	c.resting = false
	c.holding = false
	c.stopped = false
	c.onReady = false
}

// has reports whether the machine has a label at all, which is how ZDoom
// decides whether the alt fire button does anything.
func (c *Cursor) has(label string) bool {
	_, ok := c.machine.Label(label)
	return ok
}

// Stopped reports whether the cursor has run out, which for a spawned actor
// means it is done and can be removed.
func (c *Cursor) Stopped() bool { return c.stopped }

// Step is what the cursor is currently showing, or nil when it is stopped or
// out of bounds.
func (c *Cursor) Step() *Step {
	if c.stopped || c.index < 0 || c.index >= len(c.machine.Steps) {
		return nil
	}
	return &c.machine.Steps[c.index]
}

// Sprite is the picture to draw for the step the cursor is on, with DECORATE's
// keep markers read against what this layer is already showing. Everything that
// draws a cursor asks this rather than reading Step().Sprite, which can be a
// marker naming no picture at all.
func (c *Cursor) Sprite() string {
	step := c.Step()
	if step == nil {
		return ""
	}
	return ResolveSprite(step.Sprite, step.SpriteAlt, c.sprite)
}

// The DECORATE keep markers, as the migration writes them into a step's sprite
// name. `"####"` keeps the sprite the actor is showing and lets the frame
// letter change, `"----"` keeps both, and a frame letter of `#` keeps the
// letter. GZDoom stores the first two as reserved sprite indices - SPR_NOCHANGE
// and SPR_FIXED - and the third as STF_SAMEFRAME on the state, and reads all
// three in AActor::SetState.
//
// A step that keeps both halves is written as KeepBoth on its own, whichever of
// the two spellings the mod used, because there is nothing left to say. The
// migration puts it on its Loop, Goto, Stop and Wait steps too: those are not
// states in ZDoom at all, they are links between them, and a link changes
// nothing about what is being shown. Leaving their sprite empty instead said
// "show nothing", which blanked every kept frame reached through a loop.
const (
	KeepSprite = "####"
	KeepBoth   = "----"
	KeepFrame  = '#'
)

// IsKeepMarker says whether a step's sprite name is one of the keep markers
// rather than the name of a picture.
func IsKeepMarker(name string) bool {
	if name == KeepBoth {
		return true
	}
	if len(name) < 5 {
		return false
	}
	return name[:4] == KeepSprite || name[4] == KeepFrame
}

// ResolveSprite turns a step's frame name into the picture to show, given what
// the layer is showing now. Ordinary names - which is nearly all of them - come
// straight back.
//
// A marker that keeps both halves is the name it is keeping. A marker that
// keeps one half has to be spelled out, and the spelling is not enough on its
// own: the shared picture store names a frame after its own bytes as well, so
// the answer is looked up among the names the migration listed for this step.
func ResolveSprite(name string, alt []string, current string) string {
	if name == KeepBoth {
		return current
	}
	if len(name) < 5 {
		return name
	}
	head, frame := name[:4], name[4]
	sameSprite := head == KeepSprite
	sameFrame := frame == KeepFrame
	if !sameSprite && !sameFrame {
		return name
	}
	if sameSprite {
		if len(current) < 5 {
			// An actor's sprite is SPR_TNT1 until a state names one, so a kept
			// sprite with nothing before it keeps nothing and shows nothing.
			return ""
		}
		head = current[:4]
	}
	if sameFrame {
		// Its frame, though, starts at 0 - which is 'A'. A "#" letter on a
		// state that does name a sprite therefore draws that sprite's first
		// frame rather than nothing.
		frame = 'A'
		if len(current) >= 5 {
			frame = current[4]
		}
	}
	wanted := head + string(frame) + name[5:]
	for _, one := range alt {
		if strings.HasPrefix(one, wanted) {
			return one
		}
	}
	// A weapon's own frames are in its own atlas under the name they were
	// written with, so there is nothing to look up.
	return wanted
}

// Ready reports whether the cursor is resting on a frame that accepts input,
// and what that frame allows.
func (c *Cursor) Ready() (Op, bool) { return c.ready, c.onReady }

// Offsets are where this layer is holding its sprite, in GZDoom's 320x200
// weapon space: x from the middle of it, y down from the top.
func (c *Cursor) Offsets() (x, y float64) { return c.psx, c.psy }

// Bobs reports whether the weapon sways with the player's walk right now. The
// last A_WeaponReady decides it.
func (c *Cursor) Bobs() bool { return c.bobs }

// StartRaise puts the weapon fully out of sight, ready to be brought up.
func (c *Cursor) StartRaise() { c.psy = WeaponBottom }

// Runtime is the state a machine needs while it runs: the inventory it branches
// on, what the player is doing, and a source of randomness for the durations
// and spreads a script asked to vary.
type Runtime struct {
	Inventory Inventory
	Caps      Caps
	Input     Input
	Rand      *rand.Rand
	// Ammo is the item the running weapon spends, so take_ammo knows what to
	// take, and AmmoUse is what one shot costs. Empty for an actor.
	Ammo    string
	AmmoUse int
	// FireLabel is where a ready frame sends the weapon when the trigger goes
	// down, and where a refire goes back to.
	FireLabel string
	// NoAutoFire is the weapon's own +WEAPON.NOAUTOFIRE, and TriggerDown is
	// ZDoom's attackdown: whether the trigger has already been read as a press.
	// A semi-automatic will not fire again until it has been let go.
	NoAutoFire  bool
	TriggerDown bool
	// FiringAlt is ZDoom's Weapon.bAltFire: which of the two buttons started
	// the attack that is running. A_ReFire reads it to decide whether a held
	// button goes back to Hold or to AltHold.
	FiringAlt bool
	// Buttons are the items this weapon reads instead of the player's buttons,
	// and held is how many tics each has left before it is taken back. See
	// Weapon.Buttons and SetButtons.
	Buttons map[string]string
	held    map[string]float64

	// Durations is how long each item lasts once it is given, in tics, and
	// expiring is how much of that is left. ZDoom's Powerup.Duration: an item
	// given as a pulse rather than as a state. See Tick.
	Durations map[string]int
	expiring  map[string]float64

	// Vars is the actor's own scratch space: the user_ variables its script
	// keeps timers and counters in, and the few properties it reads off itself.
	Vars map[string]float64
	// World answers what a script asks about its surroundings. Nil answers no.
	World World
}

// buttonHoldTics is how long one of a mod's control items lingers after the
// button that stands behind it is let go. Brutal Doom's own control loop takes
// them back four tics after it gave them, so a script that only looks at its
// trigger every few frames still sees the press.
const buttonHoldTics = 4.0

// scriptHeldItems are the items a mod's own ACS script gives back, and how
// long it takes to do it. Nothing here runs ACS, so an item given by a weapon
// and taken by a script is an item that stays for ever - and a weapon that
// branches on one is a weapon that works exactly once.
//
// Brutal Doom's railgun is the case that shows it. Its firing states open with
// A_JumpIfInventory("Pumping", 1, "Ready"), and every one of the three places
// that gives itself Pumping - the shot, the alt shot, the reload's PutMag -
// hands the taking back to ACS_NamedExecuteAlways("RailgunRecharge"). So the
// gun fired once, or reloaded once, and every trigger pull after that walked
// out of Fire on its first line. It is not that the gun is out of ammo and it
// is not the reload: it is a cooldown nobody was counting down.
//
// The number is the recharge's own length - Railgun/Recharge.ogg is 1.26
// seconds, which is what the player hears while the gun is not ready - and it
// is a knob: a mod that means a different cooldown by the same item wants its
// own entry here rather than a different constant.
var scriptHeldItems = map[string]int{
	"pumping": 44,
}

// SetButtons keeps the items a mod's control script would have kept: given
// while the player holds the button, and taken back a few tics after they let
// go. Without them a weapon that ends its firing states with a jump on one -
// which is how every Brutal Doom automatic loops - fires exactly once a press.
func (r *Runtime) SetButtons(input Input, elapsedSeconds float64) {
	if len(r.Buttons) == 0 {
		return
	}
	if r.held == nil {
		r.held = make(map[string]float64, len(r.Buttons))
	}
	for item, button := range r.Buttons {
		var down bool
		switch button {
		case ButtonAttack:
			down = input.Trigger
		case ButtonAlt:
			down = input.Alt
		case ButtonReload:
			down = input.Reload
		}
		if down {
			r.held[item] = buttonHoldTics
			r.Inventory.Give(r.Caps, item, 1)
			continue
		}
		if r.held[item] <= 0 {
			continue
		}
		r.held[item] -= elapsedSeconds * TicRate
		if r.held[item] <= 0 {
			r.held[item] = 0
			r.Inventory.Take(item, r.Inventory[item])
		}
	}
}

// StartItemTimer starts an item's clock, if it is one that expires. Called
// wherever an item is given: by the script itself, and by something the weapon
// put into the world handing an item back to it.
func (r *Runtime) StartItemTimer(item string) {
	duration := r.Durations[item]
	if duration <= 0 {
		return
	}
	if r.expiring == nil {
		r.expiring = make(map[string]float64, 4)
	}
	r.expiring[item] = float64(duration)
}

// Tick counts down the items that take themselves back, and takes them.
//
// A pulse is what several of these scripts are built on: Final Doomer's C4
// detonator gives itself an item that lasts two tics, and every charge in the
// air is watching for it. Without the clock the item stays for ever, and the
// next charge thrown goes off in the player's face.
func (r *Runtime) Tick(elapsedSeconds float64) {
	for item, left := range r.expiring {
		left -= elapsedSeconds * TicRate
		if left > 0 {
			r.expiring[item] = left
			continue
		}
		delete(r.expiring, item)
		r.Inventory.Take(item, r.Inventory[item])
	}
}

// Var reads one of the actor's values, which is what a shipped expression does.
func (r *Runtime) Var(name string) (float64, bool) {
	// CountInv("Item") in a script's own expression compiles to this rather
	// than a dedicated op, because it can appear anywhere a number can - the
	// inventory itself already has the answer, unlike an ACS script or a
	// class check, which read as zero until there is something to ask.
	if item, ok := strings.CutPrefix(name, "inv:"); ok {
		return float64(r.Inventory[item]), true
	}
	value, known := r.Vars[name]
	return value, known
}

// SetVar writes one.
func (r *Runtime) SetVar(name string, value float64) {
	if r.Vars == nil {
		r.Vars = make(map[string]float64)
	}
	r.Vars[name] = value
}

func (r *Runtime) world() World {
	if r.World == nil {
		return NoWorld{}
	}
	return r.World
}

// OutOfAmmo reports whether the weapon cannot pay for another shot.
func (r *Runtime) OutOfAmmo() bool {
	if r.Ammo == "" {
		return false // a weapon that costs nothing never runs out
	}
	use := r.AmmoUse
	if use < 1 {
		use = 1
	}
	return r.Inventory[r.Ammo] < use
}

func (r *Runtime) roll(low, high int) float64 {
	if high <= low {
		return float64(low)
	}
	if r.Rand == nil {
		return float64(low)
	}
	return float64(low + r.Rand.Intn(high-low+1))
}

// Update advances the cursor by a time step and returns the ops that want
// something done in the world, in the order they ran.
func (c *Cursor) Update(runtime *Runtime, elapsedSeconds float64) []Op {
	var effects []Op
	if c.stopped {
		return nil
	}
	// Only a frame the cursor is actually sitting on can be charged for the
	// time that passed. Charging before the first one has been entered banks a
	// debt against a frame that does not exist yet, and the machine starts one
	// step further along than it should.
	if c.resting {
		c.ticsLeft -= elapsedSeconds * TicRate
	}

	for steps := 0; steps < maxStepsPerUpdate; steps++ {
		if c.stopped {
			return effects
		}
		if !c.resting {
			done, ops := c.enter(runtime)
			effects = append(effects, ops...)
			if !done {
				continue // a jump moved us, run the step we landed on
			}
			c.resting = true
		}

		if c.onReady {
			if target, took := c.takeReadyInput(runtime); took {
				if c.Goto(target) {
					continue
				}
			}
		}
		// A frame that lasts until something moves it, or one still waiting out
		// its duration.
		if c.holding || c.ticsLeft > ticEpsilon {
			return effects
		}
		c.advance()
	}

	// A circle of steps that costs no time. Give up on this update rather than
	// on the weapon: a machine that spins is a bug in the data or in what the
	// engine tells it, and one that stops is a gun that has gone invisible and
	// will never fire again, which is far harder to see for what it is. The
	// cursor is left exactly as it was, so there is still a frame to draw and
	// the next update tries again from the same place.
	return append(effects, Op{Op: OpSpin, Source: fmt.Sprintf(
		"%d steps in one update, none of which took any time", maxStepsPerUpdate)})
}

// takeReadyInput decides whether the player's input moves the weapon off its
// ready frame.
//
// A held trigger fires a weapon again and again, which is what a ready frame is
// for - unless the weapon is semi-automatic, and then the trigger has to be let
// go and pulled again. ZDoom keeps that as one flag on the player, set when a
// ready frame reads the trigger and cleared the moment it is released.
func (c *Cursor) takeReadyInput(runtime *Runtime) (string, bool) {
	// P_CheckWeaponFire, which GZDoom runs only while the weapon is ready -
	// TickPSprites calls it behind a WF_WEAPONREADY check. So attackdown is
	// cleared here and nowhere else: letting the trigger go in the middle of a
	// firing sequence does not arm the next shot, and a semi-automatic waits
	// for a release it can see from a ready frame.
	//
	// That check is the two ready bits together:
	//
	//	if (player.WeaponState & (WF_WEAPONREADY | WF_WEAPONREADYALT))
	//		CheckWeaponFire();
	//
	// and A_WeaponReady(WRF_NOFIRE) sets neither, so a frame that reads no
	// button never reaches the code that clears attackdown. Cleared there too,
	// the latch a semi-automatic waits on was dropped by its own recoil: the
	// Dual 92s answer three WRF_NOFIRE frames and then three plain ones, and
	// the plain ones found the held trigger apparently freshly pressed and
	// fired again through the middle of the animation.
	if !c.ready.AllowFire && !c.ready.AllowAlt {
		if runtime.Input.Reload && c.ready.AllowReload {
			return "Reload", true
		}
		return "", false
	}
	// The two buttons are one chain of else-ifs in CheckWeaponFire, with one
	// attackdown between them: the trigger wins while both are held, and the
	// flag is only cleared by a frame that sees neither.
	if c.ready.AllowFire && runtime.Input.Trigger {
		if !runtime.TriggerDown || !runtime.NoAutoFire {
			runtime.TriggerDown = true
			runtime.FiringAlt = false
			label := runtime.FireLabel
			if label == "" {
				label = "Fire"
			}
			return label, true
		}
	} else if c.ready.AllowAlt && runtime.Input.Alt {
		// FireWeaponAlt does nothing at all without an AltFire state - it does
		// not fall back on the primary one - so a weapon with no second mode
		// simply ignores the button.
		if (!runtime.TriggerDown || !runtime.NoAutoFire) && c.has("AltFire") {
			runtime.TriggerDown = true
			runtime.FiringAlt = true
			return "AltFire", true
		}
	} else {
		runtime.TriggerDown = false
	}
	if runtime.Input.Reload && c.ready.AllowReload {
		return "Reload", true
	}
	return "", false
}

// enter runs the ops on the current step. It reports whether the cursor stayed
// where it is, which is when the step's duration starts running.
func (c *Cursor) enter(runtime *Runtime) (stayed bool, effects []Op) {
	step := c.Step()
	if step == nil {
		c.stopped = true
		return true, nil
	}
	c.onReady = false
	// The sprite is settled on the way in, before the frame's own action runs,
	// which is the order ZDoom sets a state in - and it is settled for every
	// step including the zero-tic ones a jump passes straight through, because
	// those set the actor's sprite too.
	c.sprite = ResolveSprite(step.Sprite, step.SpriteAlt, c.sprite)
	// ZDoom reads a frame's offset() as an absolute position rather than a
	// nudge, and only where it is written: a zero axis leaves that axis where
	// the last frame that did write it put it. Applied before the frame's own
	// action, because that is the order ZDoom sets a state in, and it is what
	// lets A_WeaponReady on an offset frame still put the weapon back.
	if step.OffsetX != 0 {
		c.psx = float64(step.OffsetX)
	}
	if step.OffsetY != 0 {
		c.psy = float64(step.OffsetY)
	}

	for _, op := range step.Ops {
		switch op.Op {
		case OpNop:
			// Deliberately dropped at migration time; it is here so the
			// decision is visible, not so anything happens.

		case OpWeaponReady:
			c.ready, c.onReady = op, true
			c.bobs = !op.NoBob
			// A_WeaponReady is also where ZDoom puts the sprite back where a
			// weapon is held, which is what ends the recoil a Fire state's
			// offset() frames left behind.
			c.psx, c.psy = 0, WeaponTop

		case OpRaise:
			// The script loops on this frame; each pass brings the weapon up a
			// little, and arriving is what sends it to Ready.
			c.psy -= RaiseSpeed
			if c.psy <= WeaponTop {
				c.psy = WeaponTop
				if c.Goto("Ready") {
					return false, effects
				}
				c.stopped = true
				return true, effects
			}

		case OpLower:
			c.psy += RaiseSpeed
			if c.psy >= WeaponBottom {
				c.psy = WeaponBottom
				c.stopped = true // fully out of sight, and out of the way
				return true, effects
			}

		case OpGive:
			runtime.Inventory.Give(runtime.Caps, op.Item, max(op.N, 1))
			runtime.StartItemTimer(op.Item)
		case OpTake:
			// An amount of zero is ZDoom's "take all of it", which is how a
			// script clears a counter it has been adding to - the C4 launcher
			// writes `A_TakeInventory("FDHellboundC4Active", 0)` to forget
			// every charge it has in the air at once.
			if op.N <= 0 {
				runtime.Inventory.Take(op.Item, runtime.Inventory[op.Item])
				break
			}
			runtime.Inventory.Take(op.Item, op.N)
		case OpTakeAmmo:
			if runtime.Ammo != "" {
				amount := op.N
				if amount <= 0 {
					amount = runtime.Inventory[runtime.Ammo]
				}
				runtime.Inventory.Take(runtime.Ammo, amount)
			}

		case OpJumpIfInventory:
			if c.inventoryTest(runtime, op) && c.Goto(op.To) {
				return false, effects
			}

		case OpJumpIfNoAmmo:
			if runtime.OutOfAmmo() && c.Goto(op.To) {
				return false, effects
			}

		case OpJumpRandom:
			if target, taken := c.rollJump(runtime, op); taken && c.Goto(target) {
				return false, effects
			}

		case OpJumpIf:
			if op.Cond.True(runtime, runtime.Rand) && c.Goto(op.To) {
				return false, effects
			}

		case OpSetVar:
			runtime.SetVar(op.Item, op.Set.Eval(runtime, runtime.Rand))

		case OpCountdown:
			// The actor counts itself down and goes to its Death state when it
			// runs out, which is how a timed effect ends itself.
			left := runtime.Vars["reactiontime"] - 1
			runtime.SetVar("reactiontime", left)
			if left <= 0 && c.Goto(op.To) {
				return false, effects
			}

		case OpSetTics:
			if len(op.Args) > 0 {
				c.ticsLeft = op.Args[0]
				c.resting = true
				return true, effects
			}

		case OpCheckFloor, OpCheckCeiling, OpCheckSight, OpCheckFlag,
			OpCheckTarget, OpCheckHealth, OpCheckTargetInv, OpCheckCloser,
			OpCheckReload:
			if c.askWorld(runtime, op) && c.Goto(op.To) {
				return false, effects
			}

		case OpRefire:
			// ZDoom's A_ReFire does not go back to Fire: it re-enters the
			// weapon's attack state for a repeat shot, which is Hold when the
			// weapon has one - P_FireWeapon asks GetAtkState(refire) for it.
			// Fire is where the wind-up lives, so going back through it made
			// the minigun spin up again between every pair of bullets and
			// shoot three times a second. A state the call named itself is
			// left alone.
			//
			// Which button repeats is the one that started the attack, which
			// is what Weapon.bAltFire says: A_ReFire calls FireWeaponAlt for
			// an alt attack and FireWeapon for a primary one, and each asks
			// its own GetAtkState for Hold or AltHold.
			held, hold, fire := runtime.Input.Trigger, "Hold", "Fire"
			if runtime.FiringAlt {
				held, hold, fire = runtime.Input.Alt, "AltHold", "AltFire"
			}
			if held {
				// No state of its own: repeat the attack that is running,
				// which is Hold when the weapon has one and the attack's own
				// entry state when it does not.
				if op.To == "" {
					if c.Goto(hold) || c.Goto(fire) {
						return false, effects
					}
					break
				}
				if strings.EqualFold(op.To, fire) && c.Goto(hold) {
					return false, effects
				}
				if c.Goto(op.To) {
					return false, effects
				}
			}

		case OpGoto:
			if c.Goto(op.To) {
				return false, effects
			}
			c.stopped = true
			return true, effects

		case OpLoop:
			c.jump(c.labelStarts[c.index])
			return false, effects

		case OpStop:
			c.stopped = true
			return true, effects

		default:
			// Everything else happens out in the world.
			effects = append(effects, op)
		}
	}

	// Added, not assigned. A frame ends part way through an update - the clock
	// runs in seconds and a tic is a fixed slice of one - so the overshoot has
	// to be carried onto the next frame or every frame lasts up to a whole
	// extra frame of wall time. Thrown away, a weapon's rate of fire came out
	// of the machine's frame rate: the same pistol fired thirteen and a half
	// times a second at sixty and sixteen times at two hundred and forty.
	c.ticsLeft += float64(step.Tics)
	if step.TicsMax > step.Tics {
		c.ticsLeft += runtime.roll(step.Tics, step.TicsMax) - float64(step.Tics)
	}
	if step.Tics < 0 {
		c.holding = true
	}
	return true, effects
}

// askWorld answers one of the jumps whose answer is about the surroundings
// rather than about the weapon.
func (c *Cursor) askWorld(runtime *Runtime, op Op) bool {
	world := runtime.world()
	switch op.Op {
	case OpCheckFloor:
		return world.Below()
	case OpCheckCeiling:
		return world.Above()
	case OpCheckSight, OpCheckTarget:
		return world.CanSee()
	case OpCheckFlag:
		return world.HasFlag(op.Text)
	case OpCheckHealth:
		return world.TargetHealthBelow(op.N)
	case OpCheckTargetInv:
		return world.TargetHas(op.Item, max(op.N, 1))
	case OpCheckCloser:
		if len(op.Args) == 0 {
			return false
		}
		return world.TargetWithin(op.Args[0])
	case OpCheckReload:
		return runtime.OutOfAmmo()
	}
	return false
}

// rollJump decides a random jump. ZDoom writes the chance out of 256, and a
// jump with several destinations picks one of them.
func (c *Cursor) rollJump(runtime *Runtime, op Op) (string, bool) {
	targets := op.Targets
	if len(targets) == 0 {
		if op.To == "" {
			return "", false
		}
		targets = []string{op.To}
	}
	chance := op.Chance
	if chance <= 0 {
		return "", false
	}
	if chance < 256 && runtime.roll(0, 255) >= float64(chance) {
		return "", false
	}
	if len(targets) == 1 {
		return targets[0], true
	}
	return targets[int(runtime.roll(0, len(targets)-1))], true
}

func (c *Cursor) inventoryTest(runtime *Runtime, op Op) bool {
	if op.AtMax {
		return runtime.Inventory.AtMax(runtime.Caps, op.Item)
	}
	return runtime.Inventory[op.Item] >= max(op.N, 1)
}

// advance moves to the next step, falling out of the machine when there is
// none, which a well formed script never does because it jumps first.
func (c *Cursor) advance() {
	c.index++
	c.resting = false
	c.holding = false
	c.onReady = false
	if c.index >= len(c.machine.Steps) {
		c.stopped = true
	}
}

// NewRuntime sets up the inventory and caps a weapon needs, filled to capacity
// so a freshly loaded weapon has something to fire. Pickups will replace the
// filling once there are pickups.
func NewRuntime(weapon *Weapon, random *rand.Rand) *Runtime {
	runtime := &Runtime{
		Inventory:  make(Inventory, len(weapon.Items)),
		Caps:       make(Caps, len(weapon.Items)),
		Vars:       make(map[string]float64),
		Rand:       random,
		Ammo:       weapon.Ammo.Type,
		AmmoUse:    weapon.Ammo.Use,
		FireLabel:  "Fire",
		NoAutoFire: weapon.NoAutoFire,
		Buttons:    weapon.Buttons,
		held:       map[string]float64{},
	}
	for name, item := range weapon.Items {
		runtime.Caps[name] = item.Max
		runtime.Inventory[name] = item.Initial
		duration := item.Duration
		if duration == 0 {
			duration = scriptHeldItems[strings.ToLower(name)]
		}
		if duration > 0 {
			if runtime.Durations == nil {
				runtime.Durations = make(map[string]int, 4)
			}
			runtime.Durations[name] = duration
		}
	}
	if weapon.Ammo.Type != "" {
		runtime.Inventory[weapon.Ammo.Type] = runtime.Caps[weapon.Ammo.Type]
	}
	// A second ammo type sits alongside the first rather than replacing it -
	// commonly the loaded rounds next to the reserve they get drawn from -
	// and starts full for the same reason: a freshly picked up gun is loaded,
	// not found already needing a reload it has nothing to reload from.
	if weapon.Ammo.Type2 != "" {
		runtime.Inventory[weapon.Ammo.Type2] = runtime.Caps[weapon.Ammo.Type2]
	}
	return runtime
}

// Describe is a one line summary of where a cursor is, for logs and tests.
func (c *Cursor) Describe() string {
	step := c.Step()
	if step == nil {
		return "stopped"
	}
	var label string
	for name, index := range c.machine.Labels {
		if index == c.labelStarts[c.index] && !strings.HasPrefix(name, "@") {
			label = name
			break
		}
	}
	return fmt.Sprintf("%s[%d] %s %.0ft", label, c.index, c.Sprite(), c.ticsLeft)
}
