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
	Spawns   map[string]string
	Settings Settings
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
	return Policy{
		Settings: DefaultSettings(),
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

func constOp(op wep.Op) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		op := op
		op.Source = call.Raw
		return []wep.Op{op}, nil
	}
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
		AllowReload: strings.Contains(flags, "WRF_ALLOWRELOAD"),
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
	return []wep.Op{{Op: wep.OpRecoil, Delta: value.Value(), Source: call.Raw}}, nil
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
	return []wep.Op{{Op: wep.OpGive, Item: item, N: amount, Source: call.Raw}}, nil
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
		// A_ReFire with no state goes back to the weapon's firing states.
		c.needLabel("Fire")
		return []wep.Op{{Op: wep.OpRefire, To: "Fire", Source: call.Raw}}, nil
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
	if !condition.True() {
		return []wep.Op{{Op: wep.OpNop, Source: call.Raw}}, nil
	}
	target, err := c.jumpTarget(call.Arg(1, ""))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpGoto, To: target, Source: call.Raw}}, nil
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
	impact := wep.PuffBullet
	if puffActor != "" {
		mapped, known := c.policy.Puffs[strings.ToLower(puffActor)]
		if !known {
			return nil, fmt.Errorf("puff %q has no entry in the policy", puffActor)
		}
		impact = mapped
	}
	// A negative count means "fire this many, ignoring autoaim spread rules".
	if count < 0 {
		count = -count
	}
	return []wep.Op{{
		Op:      wep.OpFireBullets,
		Count:   count,
		SpreadH: spreadH.Value(),
		SpreadV: spreadV.Value(),
		Damage:  damage,
		Puff:    impact,
		Source:  call.Raw,
	}}, nil
}

// A_FireCustomMissile(type, angle, useammo, spawnofs_xy, spawnheight, flags, pitch).
func lowerFireCustomMissile(c *Compiler, call Call) ([]wep.Op, error) {
	actorName := call.NameArg(0, "")
	if actorName == "" {
		return nil, fmt.Errorf("A_FireCustomMissile with no actor")
	}
	disposition, known := c.policy.Spawns[strings.ToLower(actorName)]
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
	c.needActor(actorName)
	op := wep.Op{
		Op:            wep.OpSpawnActor,
		Actor:         actorName,
		Angle:         angle.Value(),
		SpawnOffset:   offset.Min,
		SpawnOffsetTo: offset.Max,
		SpawnHeight:   height.Value(),
		Source:        call.Raw,
	}
	// A missile that moves under its own power is a projectile; one that only
	// hangs in the air and animates is scenery.
	if c.actorIsMissile(actorName) {
		op.Op = wep.OpFireProjectile
	}
	return []wep.Op{op}, nil
}

func lowerAlertMonsters(c *Compiler, call Call) ([]wep.Op, error) {
	radius := 0.0
	if call.Arg(0, "") != "" {
		value, err := c.eval(call.Arg(0, "0"))
		if err != nil {
			return nil, err
		}
		radius = value.Value()
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
		Volume:      volume.Value(),
		Looping:     looping.True(),
		Attenuation: attenuation.Value(),
		Pitch:       pitch.Value(),
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
