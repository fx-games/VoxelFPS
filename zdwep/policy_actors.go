package zdwep

import (
	"fmt"
	"strings"

	"github.com/memmaker/goray/wep"
)

// A weapon spawns things, and those things have scripts of their own: a rocket
// seeks, a flame fades, a shell casing bounces and stops. Compiling a weapon
// without them leaves a gun that fires nothing, so this is the other half of
// the vocabulary - what a spawned actor does rather than what a weapon does.
//
// Some of it lands as arithmetic the stepper does itself: variables, timers,
// tic counts. The rest is handed to the game, because moving something through
// a world is the world's business.

// actorFunctions is the disposition of everything a spawned actor's script
// reaches for. It is folded into the policy alongside the weapon functions.
func actorFunctions() map[string]Lowering {
	return map[string]Lowering{
		// The actor's own scratch variables, and the timers built on them.
		"a_setuservar":   lowerSetVar,
		"a_setuserarray": lowerSetVar,
		"a_countdown":    lowerCountdown,
		"a_settics":      lowerSetTics,

		// Damage.
		"a_explode":        args(wep.OpExplode, "damage", "radius"),
		"a_radiusthrust":   args(wep.OpRadiusThrust, "force", "radius"),
		"a_radiusgive":     lowerRadiusGive,
		"a_railattack":     lowerRailAttack,
		"a_customrailgun":  lowerRailAttack,
		"a_custompunch":    lowerCustomPunch,
		"a_saw":            lowerSaw,
		"a_bfgspray":       lowerBFGSpray,
		"a_quake":          args(wep.OpQuake, "intensity", "duration", "damage_radius", "tremor_radius"),
		"a_die":            args(wep.OpDie),
		"a_noblockingfake": nil,

		// Movement.
		"a_changevelocity": args(wep.OpChangeVelocity, "x", "y", "z"),
		"a_scalevelocity":  args(wep.OpScaleVelocity, "factor"),
		"a_setspeed":       args(wep.OpSetSpeed, "speed"),
		"thrustthingz":     args(wep.OpThrust, "tid", "force", "up_down", "set_add"),
		"a_warp":           lowerWarp,
		"a_seekermissile":  args(wep.OpSeekerMissile, "threshold", "turn", "flags", "chance", "distance"),

		// Putting things into the world and taking them out again.
		"a_custommissile":  lowerSpawnMissile,
		"a_spawnitem":      lowerSpawnItem,
		"a_spawnitemex":    lowerSpawnItem,
		"a_spawnparticle":  lowerSpawnParticle,
		"a_killchildren":   args(wep.OpKillChildren),
		"a_removemaster":   args(wep.OpRemove),
		"thing_remove":     args(wep.OpRemove),
		"a_removechildren": args(wep.OpKillChildren),

		// How it looks.
		"a_settranslucent": lowerSetTranslucent,
		"a_fadein":         lowerFade(1),
		"a_fadeout":        lowerFade(0),
		"a_fadeto":         lowerFadeTo,
		"a_setscale":       lowerSetScale,

		// The player's view and loadout.
		"a_zoomfactor":               args(wep.OpZoom, "factor"),
		"a_setcrosshair":             args(wep.OpSetCrosshair, "which"),
		"a_selectweapon":             lowerSelectWeapon,
		"setplayerproperty":          lowerPlayerProperty,
		"a_log":                      lowerPrint,
		"a_logint":                   lowerPrint,
		"healthing":                  nil,
		"thing_changetid":            nil,
		"acs_namedexecutealways":     lowerRunScript,
		"acs_namedexecute":           lowerRunScript,
		"acs_namedexecutewithresult": lowerRunScript,

		// Jumps that have to look at the world.
		"a_checkfloor":              lowerCheck(wep.OpCheckFloor, 0),
		"a_checkceiling":            lowerCheck(wep.OpCheckCeiling, 0),
		"a_checksight":              lowerCheck(wep.OpCheckSight, 0),
		"a_checksightorrange":       lowerCheck(wep.OpCheckSight, 1),
		"a_jumpiftargetinlos":       lowerCheck(wep.OpCheckTarget, 0),
		"a_jumpifhealthlower":       lowerCheckHealth,
		"a_jumpifintargetinventory": lowerJumpIfTargetInventory,
		"a_checkflag":               lowerCheckFlag,
		"a_jumpifcloser":            lowerCheckCloser,
		"a_checkreload":             lowerCheckReload,

		// Giving and taking through whatever the actor is pointed at.
		"a_givetotarget":   lowerGiveToTarget,
		"a_takefromtarget": lowerTakeFromTarget,

		// ZDoom's map specials, which behave as their A_ cousins do.
		"radius_quake":  args(wep.OpQuake, "intensity", "duration", "damage_radius", "tremor_radius"),
		"thrustthing":   args(wep.OpThrust, "angle", "force", "limit", "tid"),
		"thing_destroy": args(wep.OpRemove),
	}
}

// monsterActions are what a monster does: look for a target, walk towards it,
// swing at it, flinch, die. All of it lands on one op, because it is one
// subsystem and this game has none of it yet.
var monsterActions = []string{
	"a_look", "a_lookex", "a_chase", "a_fastchase", "a_wander", "a_facetarget",
	"a_pain", "a_scream", "a_xscream", "a_playerscream", "a_fall",
	"a_noblocking", "a_bossdeath", "a_monsterrefire", "a_spidrefire",
	"a_cleartarget", "a_queuecorpse", "a_metal", "a_babymetal", "a_hoof",
	"a_setinvulnerable", "a_unsetinvulnerable", "a_meleeattack",
	"a_rearrangepointers", "a_copyfriendliness", "a_setshootable",
	"a_unsetshootable", "a_setsolid", "a_unsetsolid", "a_setfloat",
	"a_unsetfloat", "a_facemaster", "a_settarget", "a_settracer",
	"a_missileattack", "a_comboattack", "a_skullattack", "a_skelmissile",
	"a_skelwhoosh", "a_skelfist", "a_posattack", "a_sposattack",
	"a_sposattackusematksound", "a_sposattackuseatksound", "a_troopattack",
	"a_headattack", "a_bruisattack", "a_sargattack", "a_cyberattack",
	"a_bspiattack", "a_fatattack1", "a_fatattack2", "a_fatattack3",
	"a_vileattack", "a_vilechase", "a_viletarget", "a_firecrackle",
	"a_startfire", "a_fire", "a_painattack", "a_paindie", "a_facemovementdirection",
}

// monsterFunctions gives every monster action the same op, keeping the name.
func monsterFunctions() map[string]Lowering {
	functions := make(map[string]Lowering, len(monsterActions))
	for _, name := range monsterActions {
		action := strings.TrimPrefix(name, "a_")
		functions[name] = func(c *Compiler, call Call) ([]wep.Op, error) {
			return []wep.Op{{Op: wep.OpMonsterAction, Text: action, Source: call.Raw}}, nil
		}
	}
	return functions
}

func lowerGiveToTarget(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	amount, err := c.evalInt(call.Arg(1, "1"))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	return []wep.Op{{Op: wep.OpGiveToTarget, Item: item, N: amount, Source: call.Raw}}, nil
}

func lowerTakeFromTarget(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	amount, err := c.evalInt(call.Arg(1, "1"))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	return []wep.Op{{
		Op: wep.OpGiveToTarget, Item: item, N: -amount, Source: call.Raw,
	}}, nil
}

// A_CheckReload sends a weapon to its Reload state when it has run dry.
func lowerCheckReload(c *Compiler, call Call) ([]wep.Op, error) {
	c.needLabel("Reload")
	return []wep.Op{{Op: wep.OpCheckReload, To: "Reload", Source: call.Raw}}, nil
}

// A_JumpIfCloser(distance, label) branches on how far off the target is.
func lowerCheckCloser(c *Compiler, call Call) ([]wep.Op, error) {
	distance, err := c.evalFloat(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	target, err := c.jumpTarget(call.Arg(1, ""))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpCheckCloser, Args: []float64{distance}, To: target, Source: call.Raw,
	}}, nil
}

// args builds a lowering for a function whose arguments are just numbers, in
// the order ZDoom writes them. The names are only for the error message.
func args(op string, names ...string) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		defaults := make([]string, len(names))
		for i := range defaults {
			defaults[i] = "0"
		}
		values, programs, err := c.evalArgs(call, defaults...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(names, ", "), err)
		}
		return []wep.Op{{
			Op: op, Args: values, ArgExprs: programs,
			Flags: flagWords(call), Source: call.Raw,
		}}, nil
	}
}

// flagWords picks the flag names out of a call, so a lowering that turns on
// them can read them without the expression evaluator having to give a flag a
// number it does not have.
func flagWords(call Call) string {
	var found []string
	for _, arg := range call.Args {
		for _, word := range strings.FieldsFunc(arg, func(r rune) bool {
			return r == '|' || r == ' ' || r == '\t' || r == '(' || r == ')'
		}) {
			upper := strings.ToUpper(strings.TrimSpace(word))
			if strings.Contains(upper, "_") && upper == word || isFlagWord(upper) {
				found = append(found, upper)
			}
		}
	}
	return strings.Join(found, "|")
}

func isFlagWord(upper string) bool {
	for _, family := range flagFamilies {
		if strings.HasPrefix(strings.ToLower(upper), family) {
			return true
		}
	}
	return false
}

// A_SetUserVar(name, value) writes one of the actor's own variables, which is
// how a script keeps a countdown or remembers which frame it is on.
func lowerSetVar(c *Compiler, call Call) ([]wep.Op, error) {
	name := strings.ToLower(call.NameArg(0, ""))
	if name == "" {
		return nil, fmt.Errorf("%s with no variable", call.Name)
	}
	// A_SetUserArray takes an index between the name and the value, and the
	// index is part of the variable's name here, the way an expression reads
	// it.
	valueArg := 1
	if strings.EqualFold(call.Name, "A_SetUserArray") {
		valueArg = 2
		name = fmt.Sprintf("%s[%s]", name, call.Arg(1, "0"))
	}
	value, err := c.eval(call.Arg(valueArg, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op:     wep.OpSetVar,
		Item:   name,
		Set:    value.program(),
		Source: call.Raw,
	}}, nil
}

// A_Countdown ticks the actor's reaction time down and sends it to its Death
// state when it runs out, which is how a timed effect ends itself.
func lowerCountdown(c *Compiler, call Call) ([]wep.Op, error) {
	c.needLabel("Death")
	return []wep.Op{{Op: wep.OpCountdown, To: "Death", Source: call.Raw}}, nil
}

func lowerSetTics(c *Compiler, call Call) ([]wep.Op, error) {
	tics, err := c.evalFloat(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpSetTics, Args: []float64{tics}, Source: call.Raw}}, nil
}

func lowerRadiusGive(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
	radius, err := c.evalFloat(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	amount, err := c.evalInt(call.Arg(3, "1"))
	if err != nil {
		return nil, err
	}
	c.needItem(item)
	return []wep.Op{{
		Op: wep.OpRadiusGive, Item: item, N: amount,
		Args: []float64{radius}, Flags: flagWords(call), Source: call.Raw,
	}}, nil
}

// A_RailAttack(damage, offset, useammo, color1, color2, ...) and its custom
// cousin fire a beam that hits everything on its line.
func lowerRailAttack(c *Compiler, call Call) ([]wep.Op, error) {
	damage, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	offset, err := c.evalFloat(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	op := wep.Op{
		Op: wep.OpRailAttack, Damage: damage,
		Args: []float64{offset}, Flags: flagWords(call), Source: call.Raw,
	}
	if puff := call.NameArg(7, ""); puff != "" {
		op.Puff = c.puffKind(puff)
	} else {
		op.Puff = wep.PuffBullet
	}
	return []wep.Op{op}, nil
}

// A_CustomPunch(damage, norandom, flags, puff, range) is a melee swing.
func lowerCustomPunch(c *Compiler, call Call) ([]wep.Op, error) {
	damage, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	reach, err := c.evalFloat(call.Arg(4, "64"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpCustomPunch, Damage: damage,
		Puff: c.puffKind(call.NameArg(3, "")),
		Args: []float64{reach}, Flags: flagWords(call), Source: call.Raw,
	}}, nil
}

// A_Saw(fullsound, hitsound, damage, puff, flags, range) is a chainsaw: a melee
// swing that keeps going while the trigger is down.
func lowerSaw(c *Compiler, call Call) ([]wep.Op, error) {
	damage, err := c.evalInt(call.Arg(2, "2"))
	if err != nil {
		return nil, err
	}
	reach, err := c.evalFloat(call.Arg(5, "64"))
	if err != nil {
		return nil, err
	}
	op := wep.Op{
		Op: wep.OpSaw, Damage: damage,
		Puff: c.puffKind(call.NameArg(3, "")),
		Args: []float64{reach}, Flags: flagWords(call), Source: call.Raw,
	}
	if sound := call.NameArg(1, ""); sound != "" {
		op.Sound = sound
		c.needSound(sound)
	}
	return []wep.Op{op}, nil
}

// A_BFGSpray(actor, rays, damage) is the BFG's second half: a fan of hits on
// everything in front of the player when the ball lands.
func lowerBFGSpray(c *Compiler, call Call) ([]wep.Op, error) {
	rays, err := c.evalInt(call.Arg(1, "40"))
	if err != nil {
		return nil, err
	}
	damage, err := c.evalInt(call.Arg(2, "15"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpBFGSpray, Count: rays, Damage: damage,
		Puff: c.puffKind(call.NameArg(0, "")), Source: call.Raw,
	}}, nil
}

// A_Warp moves an actor relative to something else, which mods use to hang one
// effect off another.
func lowerWarp(c *Compiler, call Call) ([]wep.Op, error) {
	values := make([]float64, 0, 4)
	for i := 1; i <= 4; i++ {
		value, err := c.evalFloat(call.Arg(i, "0"))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return []wep.Op{{Op: wep.OpWarp, Args: values, Flags: flagWords(call), Source: call.Raw}}, nil
}

// A_CustomMissile(missiletype, spawnheight, spawnofs_xy, angle, ...) is
// A_FireCustomMissile from an actor rather than a weapon, but ZDoom orders its
// three numbers differently: height comes first here, angle last, where
// A_FireCustomMissile puts angle first and height last. A smoke actor spawning
// its own trail with `A_CustomMissile("Smoke", 0, 0, 180)` means angle 180,
// no height - read the other way round it lands 180 units above the gun.
func lowerSpawnMissile(c *Compiler, call Call) ([]wep.Op, error) {
	return c.spawn(call, wep.OpFireProjectile, 3, 2, 1)
}

// A_SpawnItem(actor, distance, height) and A_SpawnItemEx(actor, x, y, z, ...).
func lowerSpawnItem(c *Compiler, call Call) ([]wep.Op, error) {
	return c.spawn(call, wep.OpSpawnItem, 3, 1, 2)
}

// spawn is the shape all four spawning functions share: an actor name that the
// policy has to have decided about, and three numbers whose positions differ.
func (c *Compiler) spawn(call Call, op string, angleArg, offsetArg, heightArg int) ([]wep.Op, error) {
	actorName := call.NameArg(0, "")
	if actorName == "" {
		return nil, fmt.Errorf("%s with no actor", call.Name)
	}
	disposition, ok := c.spawnDisposition(actorName)
	if !ok {
		return nil, fmt.Errorf("spawned actor %q has no entry in the policy", actorName)
	}
	if disposition == "drop" {
		return []wep.Op{{Op: wep.OpNop, Source: call.Raw}}, nil
	}
	angle, err := c.eval(call.Arg(angleArg, "0"))
	if err != nil {
		return nil, err
	}
	offset, err := c.eval(call.Arg(offsetArg, "0"))
	if err != nil {
		return nil, err
	}
	height, err := c.eval(call.Arg(heightArg, "0"))
	if err != nil {
		return nil, err
	}
	c.needActor(actorName)
	return []wep.Op{{
		Op:            op,
		Actor:         actorName,
		Angle:         angle.Range.Value(),
		SpawnOffset:   offset.Range.Min,
		SpawnOffsetTo: offset.Range.Max,
		SpawnHeight:   height.Range.Value(),
		Flags:         flagWords(call),
		Source:        call.Raw,
	}}, nil
}

// A_SpawnParticle(colour, flags, lifetime, size, angle, ...). The colour is
// written as a hex triplet or a name, so it is text rather than a number the
// expression evaluator should be asked about.
func lowerSpawnParticle(c *Compiler, call Call) ([]wep.Op, error) {
	numbers := make([]float64, 0, 3)
	for i := 2; i <= 4; i++ {
		value, err := c.evalFloat(call.Arg(i, "0"))
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, value)
	}
	return []wep.Op{{
		Op:     wep.OpSpawnParticle,
		Text:   strings.Trim(call.Arg(0, ""), `"`),
		Args:   numbers,
		Flags:  flagWords(call),
		Source: call.Raw,
	}}, nil
}

func lowerSetTranslucent(c *Compiler, call Call) ([]wep.Op, error) {
	alpha, err := c.evalFloat(call.Arg(0, "1"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpSetAlpha, Args: []float64{alpha},
		Text: strings.Trim(call.Arg(1, ""), `"`), Source: call.Raw,
	}}, nil
}

// A_FadeIn and A_FadeOut walk the actor's alpha towards one end or the other.
func lowerFade(target float64) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		step, err := c.evalFloat(call.Arg(0, "0.1"))
		if err != nil {
			return nil, err
		}
		return []wep.Op{{
			Op: wep.OpFadeTo, Args: []float64{target, step},
			Flags: flagWords(call), Source: call.Raw,
		}}, nil
	}
}

func lowerFadeTo(c *Compiler, call Call) ([]wep.Op, error) {
	values, programs, err := c.evalArgs(call, "1", "0.1")
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpFadeTo, Args: values, ArgExprs: programs,
		Flags: flagWords(call), Source: call.Raw,
	}}, nil
}

// A_SetScale(x, y) usually resizes an actor relative to how big it already is,
// which is why its arguments have to survive as expressions.
func lowerSetScale(c *Compiler, call Call) ([]wep.Op, error) {
	values, programs, err := c.evalArgs(call, "1", call.Arg(0, "1"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpSetScale, Args: values, ArgExprs: programs, Source: call.Raw,
	}}, nil
}

func lowerSelectWeapon(c *Compiler, call Call) ([]wep.Op, error) {
	return []wep.Op{{
		Op: wep.OpSelectWeapon, Text: call.NameArg(0, ""), Source: call.Raw,
	}}, nil
}

// SetPlayerProperty(who, set, which) turns things like frozen or total freeze
// on and off. The property number is what says which.
func lowerPlayerProperty(c *Compiler, call Call) ([]wep.Op, error) {
	set, err := c.evalFloat(call.Arg(1, "0"))
	if err != nil {
		return nil, err
	}
	which, err := c.evalFloat(call.Arg(2, "0"))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpPlayerFlag, Args: []float64{set, which}, Source: call.Raw,
	}}, nil
}

// ACS_NamedExecute and friends run one of the mod's own scripts. We have no
// ACS, so the call is carried by name for the engine to answer, and recorded as
// left open rather than quietly dropped.
func lowerRunScript(c *Compiler, call Call) ([]wep.Op, error) {
	name := call.NameArg(0, "")
	c.defaulted("acs script " + name)
	return []wep.Op{{Op: wep.OpRunScript, Text: name, Source: call.Raw}}, nil
}

// lowerCheck builds a jump that the engine has to answer, because the answer is
// about the world: what is under the actor, what it can see. labelArg says
// which argument holds the destination.
func lowerCheck(op string, labelArg int) Lowering {
	return func(c *Compiler, call Call) ([]wep.Op, error) {
		target, err := c.jumpTarget(call.Arg(labelArg, ""))
		if err != nil {
			return nil, err
		}
		result := wep.Op{Op: op, To: target, Flags: flagWords(call), Source: call.Raw}
		if labelArg == 1 {
			if distance, err := c.evalFloat(call.Arg(0, "0")); err == nil {
				result.Args = []float64{distance}
			}
		}
		return []wep.Op{result}, nil
	}
}

func lowerCheckHealth(c *Compiler, call Call) ([]wep.Op, error) {
	health, err := c.evalInt(call.Arg(0, "0"))
	if err != nil {
		return nil, err
	}
	target, err := c.jumpTarget(call.Arg(1, ""))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{Op: wep.OpCheckHealth, N: health, To: target, Source: call.Raw}}, nil
}

func lowerJumpIfTargetInventory(c *Compiler, call Call) ([]wep.Op, error) {
	item := call.NameArg(0, "")
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
		Op: wep.OpCheckTargetInv, Item: item, N: amount, To: target, Source: call.Raw,
	}}, nil
}

func lowerCheckFlag(c *Compiler, call Call) ([]wep.Op, error) {
	target, err := c.jumpTarget(call.Arg(1, ""))
	if err != nil {
		return nil, err
	}
	return []wep.Op{{
		Op: wep.OpCheckFlag, Text: call.NameArg(0, ""), To: target, Source: call.Raw,
	}}, nil
}
