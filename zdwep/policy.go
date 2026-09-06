package zdwep

import (
	"fmt"
	"sort"
	"strings"

	"github.com/memmaker/goray/wep"
)

// The policy is the checked-in record of what every DECORATE action function
// becomes. Every function named here has an explicit disposition, either a
// lowering or a deliberate drop; a function named nowhere is a migration
// failure. That is the whole point: "ignored" is always a decision somebody
// wrote down and reviewed, never a shrug at runtime.

// Call is an action function as the script wrote it.
type Call struct {
	Name string
	Args []string
	Raw  string
}

// Arg returns the nth argument, or the fallback when the script left it off.
func (c Call) Arg(n int, fallback string) string {
	if n < len(c.Args) && strings.TrimSpace(c.Args[n]) != "" {
		return strings.TrimSpace(c.Args[n])
	}
	return fallback
}

// Name returns the nth argument with its quotes removed.
func (c Call) NameArg(n int, fallback string) string {
	return strings.Trim(c.Arg(n, fallback), `"`)
}

// ParseCall splits `A_FireBullets (4, 0.5, 10, 5, "FDBulletPuff", 0)` into its
// name and its arguments, keeping brackets and quotes inside an argument
// intact so nested calls survive.
func ParseCall(action string) (Call, bool) {
	action = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(action), ";"))
	if action == "" {
		return Call{}, false
	}
	open := strings.Index(action, "(")
	if open < 0 {
		if strings.ContainsAny(action, " \t") {
			return Call{}, false // not a call, some construct we do not read
		}
		return Call{Name: action, Raw: action}, true
	}
	name := strings.TrimSpace(action[:open])
	if name == "" {
		return Call{}, false
	}
	body, ok := splitArgs(action[open:])
	if !ok {
		return Call{}, false
	}
	return Call{Name: name, Args: body, Raw: action}, true
}

// splitArgs takes "(a, b(c, d), \"e,f\")" and returns its top level arguments.
func splitArgs(text string) ([]string, bool) {
	depth := 0
	inString := false
	var args []string
	var current strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case inString:
			current.WriteByte(c)
			if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
			current.WriteByte(c)
		case c == '(':
			depth++
			if depth > 1 {
				current.WriteByte(c)
			}
		case c == ')':
			depth--
			if depth == 0 {
				if arg := strings.TrimSpace(current.String()); arg != "" || len(args) > 0 {
					args = append(args, arg)
				}
				return args, true
			}
			current.WriteByte(c)
		case c == ',' && depth == 1:
			args = append(args, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	return nil, false
}

// Lowering turns one call into ops. Returning no ops and no error is how a
// deliberate drop is spelled.
type Lowering func(c *Compiler, call Call) ([]wep.Op, error)

// Settings are the mod's own configuration, which its scripts read through ACS
// and which we resolve to constants at migration time. The defaults are the
// values the mod ships its menu with.
type Settings struct {
	ACS map[string]float64
}

// DefaultSettings is the floor a mod's own CVARINFO is read on top of. It holds
// nothing: a setting this migration has not been told about is an error naming
// it, not a zero that quietly changes what a weapon does.
func DefaultSettings() Settings {
	return Settings{ACS: map[string]float64{}}
}

// Policy is the dispositions the migration runs under.
type Policy struct {
	// Functions maps a lowercased action function name to its lowering. A nil
	// lowering is a deliberate drop.
	Functions map[string]Lowering
	// Puffs maps a lowercased ZDoom puff actor to what it does where a hitscan
	// lands, in our world.
	Puffs map[string]string
	// Spawns maps a lowercased spawned actor to how it is handled: "sprite"
	// draws it from the mod's own frames, "drop" leaves it out.
	Spawns map[string]string
	// DefaultPuff and DefaultSpawn are what an unnamed puff or spawned actor
	// gets. Empty means a name with no line is an error.
	//
	// Loud failure is for action functions, where one we do not recognise
	// silently changes what a weapon does. A puff or a spawner is not that: the
	// worst an unnamed one does is leave the wrong mark or throw the wrong
	// thing into the air, and a mod ships a hundred of them. Every name that
	// takes the default is recorded on the compiled weapon instead, so the
	// decision is still reviewable, just after the fact.
	DefaultPuff  string
	DefaultSpawn string
	Settings     Settings
}

// KnownFunction reports whether the policy has a disposition for a call.
func (p Policy) KnownFunction(name string) bool {
	_, ok := p.Functions[strings.ToLower(name)]
	return ok
}

// DefaultPolicy is the disposition of every action function we have decided
// about. Adding a weapon that uses something else fails the migration until
// somebody adds a line here, which is the intended friction.
func DefaultPolicy() Policy {
	policy := defaultWeaponPolicy()
	for name, lowering := range actorFunctions() {
		policy.Functions[name] = lowering
	}
	for name, lowering := range monsterFunctions() {
		policy.Functions[name] = lowering
	}
	return policy
}

func defaultWeaponPolicy() Policy {
	return Policy{
		Settings:     DefaultSettings(),
		DefaultPuff:  wep.PuffBullet,
		DefaultSpawn: "sprite",
		Puffs: map[string]string{
			"fdbulletpuff": wep.PuffBullet,
			"bulletpuff":   wep.PuffBullet,
		},
		Spawns: map[string]string{},
		Functions: map[string]Lowering{
			// Shown to the player.
			"a_weaponready": lowerWeaponReady,
			"a_raise":       constOp(wep.Op{Op: wep.OpRaise}),
			"a_lower":       constOp(wep.Op{Op: wep.OpLower}),
			"a_gunflash":    lowerGunFlash,
			"a_light0":      lowerLight(0),
			"a_light1":      lowerLight(1),
			"a_light2":      lowerLight(2),
			"a_light":       lowerLightLevel,
			"a_setpitch":    lowerSetPitch,

			// Inventory and ammo.
			"a_giveinventory":   lowerGiveInventory,
			"a_takeinventory":   lowerTakeInventory,
			"a_jumpifinventory": lowerJumpIfInventory,

			// Control flow.
			"a_refire":       lowerRefire,
			"a_jumpif":       lowerJumpIf,
			"a_jump":         lowerJump,
			"a_jumpifnoammo": lowerJumpIfNoAmmo,

			// What the shot does.
			"a_firebullets":       lowerFireBullets,
			"a_firecustommissile": lowerFireCustomMissile,
			"a_alertmonsters":     lowerAlertMonsters,

			// Sound.
			"a_playsound":       lowerPlaySound,
			"a_playweaponsound": lowerPlaySound,
			"a_playsoundex":     lowerPlaySound,
			"a_stopsound":       lowerStopSound,
			"a_print":           lowerPrint,
			"a_printbold":       lowerPrint,

			// Deliberate drops. Each one is a decision, not an oversight.
			//
			// a_setangle: horizontal recoil, which our player controller does
			//   not take from scripts yet.
			// a_stop, a_noblocking: for actors falling out of the sky, not for
			//   anything a weapon does.
			// a_playerskinCheck and friends: cosmetic branching on the player
			//   sprite, and we have one player.
			// a_clearrefire: ZDoom counts how long the trigger has been held
			//   and this resets the count. We read the trigger live, so there
			//   is no count to reset.
			"a_clearrefire": nil,
			"a_setangle":    nil,
			"a_stop":        nil,
			"a_noblocking":  nil,
			"a_setblend":    nil,
			"a_recoil":      nil,
			"a_scream":      nil,
			"a_changeflag":  nil,
		},
	}
}

// puffKind is what a shot leaves where it lands. A puff the policy has not
// named takes the default and is recorded on the compiled weapon, because a
// mod ships dozens and the worst an unnamed one does is leave the wrong mark.
func (c *Compiler) puffKind(puffActor string) string {
	if puffActor == "" {
		return wep.PuffBullet
	}
	if mapped, known := c.policy.Puffs[strings.ToLower(puffActor)]; known {
		return mapped
	}
	c.defaulted("puff " + puffActor)
	if c.policy.DefaultPuff != "" {
		return c.policy.DefaultPuff
	}
	return wep.PuffBullet
}

// spawnDisposition says what to do with an actor a script throws into the
// world, taking the policy's default when it has no line of its own.
func (c *Compiler) spawnDisposition(actorName string) (string, bool) {
	if disposition, known := c.policy.Spawns[strings.ToLower(actorName)]; known {
		return disposition, true
	}
	if c.policy.DefaultSpawn == "" {
		return "", false
	}
	c.defaulted("spawns " + actorName)
	return c.policy.DefaultSpawn, true
}

func constOp(op wep.Op) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		op := op
		op.Source = call.Raw
		return []wep.Op{op}, nil
	}
}

// ControlItems are the inventory items a mod's own control script keeps in
// step with the player's buttons, and which input each one stands for. They are
// a mod's vocabulary rather than ZDoom's, and they belong here for the same
// reason every action function's disposition does: it is a decision somebody
// wrote down.
//
// Brutal Doom's BDControls.txt is an ACS loop that reads the buttons, gives one
// of these for each that is down, waits four tics and takes them all back. Its
// weapons never read a button directly - a full automatic ends its firing
// states with A_JumpIfInventory("FiredPrimary", 1, "Fire") - so without these
// every one of them fires exactly once a press.
var ControlItems = map[string]string{
	// Brutal Doom's.
	"firedprimary": wep.ButtonAttack,
	"reloading":    wep.ButtonReload,
	// Final Doomer's, from FDOOMSCR.acs, which sets one item per button every
	// tic from GetPlayerInput. Its C4 charge is armed and detonated entirely
	// through these: without them the charge is thrown and can never go off.
	"fdsynthfirefire":    wep.ButtonAttack,
	"fdsynthfirealtfire": wep.ButtonAlt,
}

// A_WeaponReady(flags) is what makes a weapon accept input, and its flags say
// which input. The scripts choose between a firing ready frame and a no-fire
// one by branching on ammo, so whether the trigger works is a property of the
// frame the weapon is resting on, not of the weapon.
func lowerWeaponReady(c *Compiler, call Call) ([]wep.Op, error) {
	flags := strings.ToUpper(call.Arg(0, ""))
	op := wep.Op{
		Op:          wep.OpWeaponReady,
		AllowFire:   !strings.Contains(flags, "WRF_NOFIRE") && !strings.Contains(flags, "WRF_NOPRIMARY"),
		AllowAlt:    !strings.Contains(flags, "WRF_NOFIRE") && !strings.Contains(flags, "WRF_NOSECONDARY"),
		AllowReload: strings.Contains(flags, "WRF_ALLOWRELOAD"),
		NoBob:       strings.Contains(flags, "WRF_NOBOB"),
		Source:      call.Raw,
	}
	return []wep.Op{op}, nil
}

func lowerGunFlash(c *Compiler, call Call) ([]wep.Op, error) {
	target := call.NameArg(0, "Flash")
	c.needLabel(target)
	return []wep.Op{{Op: wep.OpGunFlash, To: target, Source: call.Raw}}, nil
}

// A_Light(n) is the general form of A_Light0, A_Light1 and A_Light2.
func lowerLightLevel(c *Compiler, call Call) ([]wep.Op, error) {
	level, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpFlashLight, Level: level, Source: call.Raw}}, nil
}

func lowerStopSound(c *Compiler, call Call) ([]wep.Op, error) {
	channel, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpStopSound, Channel: channel, Source: call.Raw}}, nil
}

func lowerPrint(c *Compiler, call Call) ([]wep.Op, error) {
	return []wep.Op{{Op: wep.OpPrint, Text: call.NameArg(0, ""), Source: call.Raw}}, nil
}

// A_Jump(chance, state, ...) takes one of its destinations, chance out of 256
// of the time.
func lowerJump(c *Compiler, call Call) ([]wep.Op, error) {
	chance, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	if len(call.Args) < 2 {
		return nil, fmt.Errorf("A_Jump with no destination")
	}
	targets := make([]string, 0, len(call.Args)-1)
	for i := 1; i < len(call.Args); i++ {
		target, err := c.jumpTarget(call.Arg(i, ""))
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	op := wep.Op{Op: wep.OpJumpRandom, Chance: chance, Source: call.Raw}
	if len(targets) == 1 {
		op.To = targets[0]
	} else {
		op.Targets = targets
	}
	return []wep.Op{op}, nil
}

func lowerJumpIfNoAmmo(c *Compiler, call Call) ([]wep.Op, error) {
	target, err := c.jumpTarget(call.Arg(0, ""))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpJumpIfNoAmmo, To: target, Source: call.Raw}}, nil
}

func lowerLight(level int) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		return []wep.Op{{Op: wep.OpFlashLight, Level: level, Source: call.Raw}}, nil
	}
}

// A_SetPitch writes an absolute pitch built from the current one, so the recoil
// we want is whatever is left once the current pitch reads as zero.
func lowerSetPitch(c *Compiler, call Call) ([]wep.Op, error) {
	value, err := c.eval(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpRecoil, Delta: value.Range.Value(), Source: call.Raw}}, nil
}

func lowerGiveInventory(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	if item == "" {
		return nil, fmt.Errorf("A_GiveInventory with no item")
	}
	amount, err := c.evalInt(call.Arg(1, "1"))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	// Its third argument is a pointer: a thing can hand an item to whoever
	// fired it. Final Doomer's C4 charge tells the player it exists that way,
	// and the weapon counts what it has in the air by asking its own
	// inventory - so dropped, the charge is thrown and then nothing about it
	// can ever be detonated.
	if pointsAtTheShooter(call.Arg(2, "")) {
		return []wep.Op{{Op: wep.OpGiveToTarget, Item: item, N: amount, Source: call.Raw}}, nil
	}
	return []wep.Op{{Op: wep.OpGive, Item: item, N: amount, Source: call.Raw}}, nil
}

// pointsAtTheShooter reads one of ZDoom's actor pointers. A missile's target is
// whoever fired it, and for anything a weapon spawns that is the player.
func pointsAtTheShooter(pointer string) bool {
	switch strings.ToUpper(strings.TrimSpace(pointer)) {
	case "AAPTR_TARGET", "AAPTR_MASTER", "AAPTR_PLAYER1":
		return true
	}
	return false
}

func lowerTakeInventory(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	if item == "" {
		return nil, fmt.Errorf("A_TakeInventory with no item")
	}
	amount, err := c.evalInt(call.Arg(1, "1"))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	if pointsAtTheShooter(call.Arg(3, "")) {
		return []wep.Op{{Op: wep.OpGiveToTarget, Item: item, N: -amount, Source: call.Raw}}, nil
	}
	if strings.EqualFold(item, c.ammoType) {
		return []wep.Op{{Op: wep.OpTakeAmmo, N: amount, Source: call.Raw}}, nil
	}
	return []wep.Op{{Op: wep.OpTake, Item: item, N: amount, Source: call.Raw}}, nil
}

// A_JumpIfInventory(item, amount, target). An amount of zero is ZDoom's idiom
// for "when the item is at its maximum", which is exactly the branch the
// Borstal shotgun opens its Fire state with, and exactly the branch the old
// interpreter took unconditionally.
func lowerJumpIfInventory(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	if item == "" {
		return nil, fmt.Errorf("A_JumpIfInventory with no item")
	}
	amount, err := c.evalInt(call.Arg(1, "1"))
	if err != nil {
		return nil, err
	}
	target, err := c.jumpTarget(call.Arg(2, ""))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	return []wep.Op{{
		Op:     wep.OpJumpIfInventory,
		Item:   item,
		N:      amount,
		AtMax:  amount == 0,
		To:     target,
		Source: call.Raw,
	}}, nil
}

func lowerRefire(c *Compiler, call Call) ([]wep.Op, error) {
	target := call.Arg(0, "")
	if target == "" {
		// A_ReFire with no state of its own repeats whichever attack is
		// running, and only the runtime knows which button that was. Writing
		// "Fire" here instead named the primary's state on an alt attack too,
		// and Brutal Doom's hand grenade - whose AltHold is two tics and an
		// A_ReFire - walked out of the alt chain into Fire and pulled the pin
		// again, for ever.
		return []wep.Op{{Op: wep.OpRefire, Source: call.Raw}}, nil
	}
	resolved, err := c.jumpTarget(target)
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpRefire, To: resolved, Source: call.Raw}}, nil
}

// A_JumpIf's condition is mod configuration read through ACS, so it folds to a
// constant here: either the branch always runs or it never does, and the
// compiled weapon says which without carrying the condition.
func lowerJumpIf(c *Compiler, call Call) ([]wep.Op, error) {
	condition, err := c.eval(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	target, err := c.jumpTarget(call.Arg(1, ""))
	if err != nil {
		return nil, err
	}
	// A condition built only from the mod's settings is settled here: either
	// the branch always runs or it never does, and the compiled weapon says
	// which without carrying the test.
	if condition.Folded() {
		if !condition.True() {
			return []wep.Op{{Op: wep.OpNop, Source: call.Raw}}, nil
		}
		return []wep.Op{{Op: wep.OpGoto, To: target, Source: call.Raw}}, nil
	}
	// A condition that reads the running actor has to travel with the weapon.
	return []wep.Op{{
		Op: wep.OpJumpIf, Cond: condition.Code, To: target, Source: call.Raw,
	}}, nil
}

// A_FireBullets(spread_xy, spread_z, numbullets, damage, puff, flags, range).
// The spread comes first and the count third, which is worth writing down
// because reading it the other way round turns a ten pellet shotgun into a
// four pellet one.
func lowerFireBullets(c *Compiler, call Call) ([]wep.Op, error) {
	spreadH, err := c.eval(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	spreadV, err := c.eval(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	count, err := c.evalInt(call.Arg(2, "1"))
	if err != nil {
		return nil, err
	}
	damage, err := c.evalInt(call.Arg(3, "0"))
	if err != nil {
		return nil, err
	}
	puffActor := call.NameArg(4, "")
	impact := c.puffKind(puffActor)
	// A negative count means "fire this many, ignoring autoaim spread rules".
	if count < 0 {
		count = -count
	}
	// Only a puff the mod actually defines is worth drawing as itself; the
	// stock ones (a plain bullet puff GZDoom supplies) have no states of their
	// own and would just be a dummy file in the shared store.
	if _, defined := c.idx.Actor(puffActor); defined {
		c.needActor(puffActor)
	} else {
		puffActor = ""
	}
	// FBF_USEAMMO is bit one of the flags, and the flags argument defaults to
	// exactly that: a call that names no flags spends a round, and one that
	// names some other flag and not this one does not. The Blacktail's laser
	// pointer fires a hitscan every tic with FBF_NOFLASH|FBF_NORANDOMPUFFZ,
	// and it costs nothing.
	ops := c.spendAmmo(usesAmmo(call.Arg(5, "")))
	ops = append(ops, wep.Op{
		Op:        wep.OpFireBullets,
		Count:     count,
		SpreadH:   spreadH.Range.Value(),
		SpreadV:   spreadV.Range.Value(),
		Damage:    damage,
		Puff:      impact,
		PuffActor: puffActor,
		Source:    call.Raw,
	})
	// A_FireBullets plays the weapon's own AttackSound on CHAN_WEAPON itself,
	// unconditionally, regardless of any flags the call carries - GZDoom's
	// own A_FireBullets does `if (weapon != NULL) A_StartSound(weapon.
	// AttackSound, CHAN_WEAPON)` before ever looking at what was asked for.
	// A weapon that already plays its own sound explicitly and never set
	// AttackSound is untouched; one that only ever set the property and never
	// called A_PlaySound - the older, simpler way to write one - was
	// otherwise silent.
	if sound, ok := c.mergedProperty(c.actorName, "attacksound"); ok {
		sound = strings.Trim(sound, `"`)
		if sound != "" {
			c.needSound(sound)
			ops = append([]wep.Op{{
				Op:      wep.OpPlaySound,
				Sound:   sound,
				Channel: 1, // CHAN_WEAPON
				Volume:  1,
				Source:  "AttackSound (implicit, from A_FireBullets)",
			}}, ops...)
		}
	}
	return ops, nil
}

// usesAmmo reads A_FireBullets's flags argument. Written out, only
// FBF_USEAMMO spends a round; left out entirely, the default is to spend one.
func usesAmmo(flags string) bool {
	flags = strings.TrimSpace(flags)
	if flags == "" {
		return true
	}
	if flags == "1" {
		return true
	}
	return strings.Contains(strings.ToUpper(flags), "FBF_USEAMMO")
}

// A_FireCustomMissile(type, angle, useammo, spawnofs_xy, spawnheight, flags, pitch).
func lowerFireCustomMissile(c *Compiler, call Call) ([]wep.Op, error) {
	actorName := call.NameArg(0, "")
	if actorName == "" {
		return nil, fmt.Errorf("A_FireCustomMissile with no actor")
	}
	disposition, known := c.spawnDisposition(actorName)
	if !known {
		return nil, fmt.Errorf("spawned actor %q has no entry in the policy", actorName)
	}
	if disposition == "drop" {
		return []wep.Op{{Op: wep.OpNop, Source: call.Raw}}, nil
	}
	angle, err := c.eval(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	offset, err := c.eval(call.Arg(3, "0"))
	if err != nil {
		return nil, err
	}
	height, err := c.eval(call.Arg(4, "0"))
	if err != nil {
		return nil, err
	}
	// The seventh argument is a pitch, and A_FireProjectile applies it the
	// simplest way there is: `self.Pitch += pitch` around the spawn, so it is
	// an adjustment to where the player is already looking, in ZDoom's sign
	// where positive aims down.
	pitch, err := c.eval(call.Arg(6, "0"))
	if err != nil {
		return nil, err
	}
	c.needActor(actorName)
	op := wep.Op{
		Op:            wep.OpSpawnActor,
		Actor:         actorName,
		Angle:         angle.Range.Min,
		AngleTo:       angle.Range.Max,
		SpawnOffset:   offset.Range.Min,
		SpawnOffsetTo: offset.Range.Max,
		SpawnHeight:   height.Range.Value(),
		AimPitch:      pitch.Range.Min,
		AimPitchTo:    pitch.Range.Max,
		Source:        call.Raw,
	}
	// A missile that moves under its own power is a projectile; one that only
	// hangs in the air and animates is scenery.
	if c.actorIsMissile(actorName) {
		op.Op = wep.OpFireProjectile
	}
	// Its third argument is useammo, and it defaults to true.
	return append(c.spendAmmo(call.Arg(2, "1") != "0"), op), nil
}

// spendAmmo is the round a firing function takes out of the weapon by itself.
// In ZDoom that is not the script's doing: A_FireBullets and A_FireCustomMissile
// call DepleteAmmo unless told not to, and a weapon whose script never takes
// its own ammo is not a weapon with infinite ammo - it is one whose ammo the
// engine spends. Final Doomer's C4 charges could be thrown for ever, so the
// state that arms the detonator, which is only reached with no charges left,
// never came.
func (c *Compiler) spendAmmo(uses bool) []wep.Op {
	if !uses || c.ammoType == "" {
		return nil
	}
	use := c.ammoUse(c.actorName)
	if use <= 0 {
		return nil
	}
	return []wep.Op{{Op: wep.OpTakeAmmo, N: use, Source: "the firing function's own ammo use"}}
}

func lowerAlertMonsters(c *Compiler, call Call) ([]wep.Op, error) {
	radius := 0.0
	if call.Arg(0, "") != "" {
		value, err := c.eval(call.Arg(0, "0"))
		if err != nil {
			return nil, err
		}
		radius = value.Range.Value()
	}
	return []wep.Op{{Op: wep.OpMakeNoise, Radius: radius, Source: call.Raw}}, nil
}

// A_PlaySound(sound, channel, volume, looping, attenuation, local, pitch).
// The fifth argument is attenuation, how fast the sound falls off with
// distance, not pitch; pitch is the seventh and is rarely written.
func lowerPlaySound(c *Compiler, call Call) ([]wep.Op, error) {
	sound := call.NameArg(0, "")
	if sound == "" {
		return nil, fmt.Errorf("%s with no sound", call.Name)
	}
	channel, err := c.evalInt(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	volume, err := c.eval(call.Arg(2, "1"))
	if err != nil {
		return nil, err
	}
	looping, err := c.eval(call.Arg(3, "0"))
	if err != nil {
		return nil, err
	}
	attenuation, err := c.eval(call.Arg(4, "1"))
	if err != nil {
		return nil, err
	}
	pitch, err := c.eval(call.Arg(6, "1"))
	if err != nil {
		return nil, err
	}
	c.needSound(sound)
	return []wep.Op{{
		Op:          wep.OpPlaySound,
		Sound:       sound,
		Channel:     channel,
		Volume:      volume.Range.Value(),
		Looping:     looping.Range.True(),
		Attenuation: attenuation.Range.Value(),
		Pitch:       pitch.Range.Value(),
		Source:      call.Raw,
	}}, nil
}

// SortedKeys is here so error messages list things in a stable order, which
// keeps a failing migration's output diffable.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
