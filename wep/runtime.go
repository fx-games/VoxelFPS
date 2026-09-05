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
const maxStepsPerUpdate = 512

// ticEpsilon is the slack in the tic countdown. A frame time of a whole tic
// does not multiply back to exactly one, so without it a two tic frame is
// occasionally three.
const ticEpsilon = 1e-6

// raisePerTic is how much of the way a weapon travels each tic while it is
// being brought up or put away. Doom moves a weapon six of its thirty two
// units per tic, which is the same six tics.
const raisePerTic = 6.0 / 32.0

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
	Reload  bool
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
	// raised is how far down the weapon is being held while it is swapped in or
	// out: 0 is up and ready, 1 is fully out of sight. A_Raise and A_Lower walk
	// it, and the script loops on them until it arrives.
	raised float64
	// ready mirrors the weapon_ready op on the step being rested on, so input
	// is only accepted where the script says it is.
	ready Op
	// onReady is set while the rested-on step carries a weapon_ready.
	onReady bool
}

// NewCursor starts a cursor at a label.
func NewCursor(machine *StateMachine, label string) *Cursor {
	c := &Cursor{machine: machine, labelStarts: labelStarts(machine)}
	c.stopped = true
	c.Goto(label)
	return c
}

// labelStarts works out, for every step, which label it belongs to. ZDoom's
// Loop goes back to the label the state sits under, and states fall through
// from one label into the next, so this is a scan rather than a lookup.
func labelStarts(machine *StateMachine) []int {
	starts := make([]int, len(machine.Steps))
	isStart := make(map[int]bool, len(machine.Labels))
	for _, index := range machine.Labels {
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
	c.ticsLeft = 0
	c.resting = false
	c.holding = false
	c.stopped = false
	c.onReady = false
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

// Ready reports whether the cursor is resting on a frame that accepts input,
// and what that frame allows.
func (c *Cursor) Ready() (Op, bool) { return c.ready, c.onReady }

// Raised is how far out of sight the weapon is being held, from 0 for fully up
// to 1 for fully away. Drawing shifts the sprite down by this much of its own
// height, which is what makes a weapon swap look like one.
func (c *Cursor) Raised() float64 { return c.raised }

// StartRaise puts the weapon fully out of sight, ready to be brought up.
func (c *Cursor) StartRaise() { c.raised = 1 }

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

	// Vars is the actor's own scratch space: the user_ variables its script
	// keeps timers and counters in, and the few properties it reads off itself.
	Vars map[string]float64
	// World answers what a script asks about its surroundings. Nil answers no.
	World World
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

	c.ticsLeft -= elapsedSeconds * TicRate

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

	// Only reachable when the data walks a circle of zero length steps, which
	// loading is supposed to have made impossible.
	c.stopped = true
	return append(effects, Op{Op: OpStop, Source: "gave up: too many steps in one update"})
}

// takeReadyInput decides whether the player's input moves the weapon off its
// ready frame.
func (c *Cursor) takeReadyInput(runtime *Runtime) (string, bool) {
	if runtime.Input.Trigger && c.ready.AllowFire {
		label := runtime.FireLabel
		if label == "" {
			label = "Fire"
		}
		return label, true
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

	for _, op := range step.Ops {
		switch op.Op {
		case OpNop:
			// Deliberately dropped at migration time; it is here so the
			// decision is visible, not so anything happens.

		case OpWeaponReady:
			c.ready, c.onReady = op, true

		case OpRaise:
			// The script loops on this frame; each pass brings the weapon up a
			// little, and arriving is what sends it to Ready.
			c.raised -= raisePerTic
			if c.raised <= 0 {
				c.raised = 0
				if c.Goto("Ready") {
					return false, effects
				}
				c.stopped = true
				return true, effects
			}

		case OpLower:
			c.raised += raisePerTic
			if c.raised >= 1 {
				c.raised = 1
				c.stopped = true // fully out of sight, and out of the way
				return true, effects
			}

		case OpGive:
			runtime.Inventory.Give(runtime.Caps, op.Item, max(op.N, 1))
		case OpTake:
			runtime.Inventory.Take(op.Item, max(op.N, 1))
		case OpTakeAmmo:
			if runtime.Ammo != "" {
				runtime.Inventory.Take(runtime.Ammo, max(op.N, 1))
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
			if runtime.Input.Trigger && c.Goto(op.To) {
				return false, effects
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

		case OpWait:
			// Stay on this frame until something moves it.
			c.holding = true
			return true, effects

		default:
			// Everything else happens out in the world.
			effects = append(effects, op)
		}
	}

	c.ticsLeft = float64(step.Tics)
	if step.TicsMax > step.Tics {
		c.ticsLeft = runtime.roll(step.Tics, step.TicsMax)
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
		Inventory: make(Inventory, len(weapon.Items)),
		Caps:      make(Caps, len(weapon.Items)),
		Vars:      make(map[string]float64),
		Rand:      random,
		Ammo:      weapon.Ammo.Type,
		AmmoUse:   weapon.Ammo.Use,
		FireLabel: "Fire",
	}
	for name, item := range weapon.Items {
		runtime.Caps[name] = item.Max
		runtime.Inventory[name] = item.Initial
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
	return fmt.Sprintf("%s[%d] %s %.0ft", label, c.index, step.Sprite, c.ticsLeft)
}
