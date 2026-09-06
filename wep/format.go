// Package wep is the compiled weapon format: what the migration writes and what
// the engine reads. It holds no ZDoom knowledge. A weapon written by hand and a
// weapon migrated from a mod are the same thing here.
package wep

import (
	"math/rand"
	"strings"
)

// Format is the version of the on-disk layout this build reads and writes. A
// weapons folder built by a different version says so on load, instead of
// failing three checks later on something that looks unrelated.
const Format = 1

// File names inside a weapon's folder.
const (
	WeaponFile = "weapon.json"
	// ActorsDir holds every actor any weapon spawns, one file each, named by
	// the hash of its content. 867 distinct actors were being stored 13710
	// times when each weapon kept its own copy.
	ActorsDir = "_actors"
	// SpritesDir holds the frames those actors are drawn with, one picture
	// each, shared for the same reason. They are packed into one atlas when the
	// game starts rather than by the migration, because which weapons are
	// installed is not something the migration knows.
	SpritesDir = "_sprites"
	// SharedAtlasFile is where that packed atlas is kept between runs, and
	// SharedAtlasKey is the chunk in it saying what it was packed from.
	SharedAtlasFile = "_sprites.png"
	SharedAtlasKey  = "spKY"
	// BulletChipSprites and BloodSplatSprite are the engine's own decal
	// pictures, which live in SpritesDir with everything else a weapon throws
	// at a wall. They are packed into the same atlas, in the resolution they
	// were authored: the 64x64 sheet next door is for 64x64 sprites, and a
	// seven pixel bullet chip blown up to fill one of its tiles is not a
	// 64x64 sprite, it is a smudge. `go run ./tools/gendecals` writes them.
	BloodSplatSprite = "BLOODSPLAT"
	AtlasFile        = "sprites.png"
	SoundsDir        = "sounds"
	SourceFile       = "source.txt"
	// KeyMapFile lives in the weapons folder itself, not in a weapon's folder.
	KeyMapFile = "keys.txt"
	// AtlasChunk is the PNG chunk the frame table rides in.
	AtlasChunk = "atLS"
)

// BulletChipSprites are ZDoom's five bullet holes, by their names in the
// shared picture store. Five because decaldef.txt puts BulletChip1..5 in one
// `decalgroup BulletChip` with equal weights, so a burst does not read as the
// same stamp printed in a row.
var BulletChipSprites = [5]string{"BULLETCHIP1", "BULLETCHIP2", "BULLETCHIP3", "BULLETCHIP4", "BULLETCHIP5"}

// EngineDecalSprites is every one of them together, for the sweep that deletes
// what the store holds and no weapon names: no weapon.json names these, so
// -prune would take them every single time.
var EngineDecalSprites = []string{
	BulletChipSprites[0], BulletChipSprites[1], BulletChipSprites[2],
	BulletChipSprites[3], BulletChipSprites[4], BloodSplatSprite,
}

// The impact kinds a puff can compile to. The migration's policy decides which
// ZDoom puff actor becomes which; the engine decides what each one looks like.
// Both ends name them from here rather than writing the string twice.
const (
	PuffBullet    = "bullet"
	PuffExplosion = "explosion"
)

// Op is one instruction. The set is closed: the migration fails on anything it
// cannot turn into one of these, so the engine never meets an instruction it
// does not know.
//
// One struct with optional fields rather than a union per op. The fields a
// given op does not use stay out of the JSON, and nothing has to hand-write an
// unmarshaller.
type Op struct {
	Op string `json:"op"`

	// Inventory: give, take, jump_if_inventory.
	Item string `json:"item,omitempty"`
	N    int    `json:"n,omitempty"`
	// AtMax marks the ZDoom idiom of writing an amount of 0 to mean "jump when
	// the item is at its maximum" rather than "jump when you have none".
	AtMax bool `json:"at_max,omitempty"`

	// Jumps: jump_if_inventory, jump_if_no_ammo, refire, goto.
	To string `json:"to,omitempty"`

	// weapon_ready.
	AllowFire bool `json:"allow_fire,omitempty"`
	// AllowAlt is the other half of it: ZDoom sets WF_WEAPONREADY and
	// WF_WEAPONREADYALT separately, so a frame can take the alt fire button
	// and refuse the trigger. The scoped railgun rests on such a frame - only
	// the second button, which zooms back out, does anything there.
	AllowAlt    bool `json:"allow_alt,omitempty"`
	AllowReload bool `json:"allow_reload,omitempty"`
	// NoBob is A_WeaponReady's WRF_NOBOB: the weapon holds still while it is
	// resting on this frame, however fast the player is moving. Scripts use it
	// for the frames of a reload, where the gun is being handled rather than
	// carried and a walking sway would fight the animation.
	NoBob bool `json:"no_bob,omitempty"`

	// play_sound. Attenuation is how fast the sound falls off with distance,
	// which is A_PlaySound's fifth argument; pitch is its seventh.
	Sound       string  `json:"sound,omitempty"`
	Channel     int     `json:"channel,omitempty"`
	Volume      float64 `json:"volume,omitempty"`
	Looping     bool    `json:"looping,omitempty"`
	Attenuation float64 `json:"attenuation,omitempty"`
	Pitch       float64 `json:"pitch,omitempty"`

	// fire_bullets.
	Count   int     `json:"count,omitempty"`
	SpreadH float64 `json:"spread_h,omitempty"`
	SpreadV float64 `json:"spread_v,omitempty"`
	Damage  int     `json:"damage,omitempty"`
	Puff    string  `json:"puff,omitempty"`
	// PuffActor is the actor the mod itself names, so the impact can draw its
	// own sprite (a spark, a puff of smoke) instead of the engine's generic
	// bullet hole. Empty when the script named nothing or named an actor the
	// weapon could not compile.
	PuffActor string `json:"puff_actor,omitempty"`

	// spawn_actor and fire_projectile.
	Actor string  `json:"actor,omitempty"`
	Angle float64 `json:"angle,omitempty"`
	// AngleTo above Angle means the script wrote a range - `random(0, 360)`,
	// which is how a cloud of smoke is scattered. Collapsed to one number it
	// is not a scatter at all: every puff leaves on the same bearing, and
	// Brutal Doom's flame smoke, fired at random(0,360), all came back at the
	// player because the middle of nought to three hundred and sixty is a
	// hundred and eighty.
	AngleTo     float64 `json:"angle_to,omitempty"`
	SpawnOffset float64 `json:"spawn_offset,omitempty"`
	// SpawnOffsetTo above SpawnOffset means the script asked for a random
	// sideways offset, as the shell casings do.
	SpawnOffsetTo float64 `json:"spawn_offset_to,omitempty"`
	SpawnHeight   float64 `json:"spawn_height,omitempty"`
	// SpawnForward is how far out in front of the spawner the thing appears,
	// along the way it is facing and level with it whatever it is looking at.
	// Only A_SpawnItem and A_SpawnItemEx have one: a missile leaves from where
	// the shooter stands and gets there under its own speed, so reading a
	// forward offset into one of those puts a puff of smoke in the player's
	// face for as long as it lives.
	SpawnForward   float64 `json:"spawn_forward,omitempty"`
	SpawnForwardTo float64 `json:"spawn_forward_to,omitempty"`
	// AimPitch is the angle a missile-spawning call fires at, in degrees and
	// in ZDoom's sign, where **positive aims down**: A_FireCustomMissile adds
	// it to the shooter's own pitch, and A_CustomMissile works a velocity out
	// of it directly. AimPitchTo above it means the script wrote a range.
	// (Op.Pitch above is a sound's pitch, which is a different thing.)
	AimPitch   float64 `json:"aim_pitch,omitempty"`
	AimPitchTo float64 `json:"aim_pitch_to,omitempty"`
	// AbsolutePitch means the pitch is the whole answer rather than an
	// adjustment to where the shooter is looking - A_CustomMissile's
	// CMF_ABSOLUTEPITCH, and its aimmode 2, which sets that flag itself.
	AbsolutePitch bool `json:"absolute_pitch,omitempty"`
	// VelX, VelY and VelZ are A_SpawnItemEx's own xvel/yvel/zvel, in Doom
	// units a tic, relative to the direction the spawn faces: X forward, Y
	// sideways, Z up. A spawned thing with no Speed of its own otherwise
	// just drifts at random; a script that hands it a real velocity - a
	// pickup that pops and scatters rather than sitting still - means that.
	//
	// A *To field above its own axis means the script wrote a range
	// (frandom(-2.5, 2.5), say) rather than one number, the same as
	// SpawnOffset/SpawnOffsetTo: rolled once per spawn, not folded to a
	// fixed midpoint at migration time, which is what a scatter needs to
	// look like more than one thing landed.
	VelX   float64 `json:"vel_x,omitempty"`
	VelXTo float64 `json:"vel_x_to,omitempty"`
	VelY   float64 `json:"vel_y,omitempty"`
	VelYTo float64 `json:"vel_y_to,omitempty"`
	VelZ   float64 `json:"vel_z,omitempty"`
	VelZTo float64 `json:"vel_z_to,omitempty"`
	// VelExprs replaces the three ranges above when the call writes its
	// velocity in terms of the thing doing the spawning. Final Doomer's gun
	// smoke is spawned with `A_SpawnItemEx("FDGunSmoke", 0,0,0, velx,
	// vely+frandom(-0.2,0.2), velz, ...)` - it inherits the throw of the
	// spawner it came out of - and folding `velx` to a number at migration
	// time makes it zero, which leaves every puff hanging in the muzzle
	// instead of leaving it. Three entries: x, y, z.
	VelExprs []Expr `json:"vel_exprs,omitempty"`
	// AbsoluteVelocity is SXF_ABSOLUTEVELOCITY: the three numbers are Doom's
	// own world axes rather than forward, sideways and up from the spawner's
	// facing. It travels with VelExprs more often than not, because velx and
	// vely read off another actor are already in world axes and re-rotating
	// them by a facing sends the spawn off at whatever angle the player
	// happens to be standing at.
	AbsoluteVelocity bool `json:"absolute_velocity,omitempty"`

	// flash_light and recoil.
	Level int     `json:"level,omitempty"`
	Delta float64 `json:"delta,omitempty"`

	// make_noise.
	Radius float64 `json:"radius,omitempty"`

	// jump_random: the chance out of 256 that the jump is taken, ZDoom's scale.
	Chance int `json:"chance,omitempty"`
	// Targets is where jump_random may go; one of them is picked.
	Targets []string `json:"targets,omitempty"`

	// print, log, select_weapon: the thing being named.
	Text string `json:"text,omitempty"`

	// Cond is a condition worked out while the game runs, for the jumps whose
	// answer depends on the actor rather than on the mod's settings.
	Cond Expr `json:"cond,omitempty"`
	// Set is what a set_var op assigns.
	Set Expr `json:"set,omitempty"`

	// Args carries the numbers of the ops that take several and whose meaning
	// is positional in ZDoom: a thrust's force and angle, a warp's offsets, a
	// rail attack's colours. Spelling every one of those out as its own field
	// would double this struct for ops the engine acts on as a group anyway.
	Args []float64 `json:"args,omitempty"`
	// ArgExprs replaces Args when at least one of them depends on the running
	// actor. `A_SetScale(scalex * frandom(0.08, 0.14))` shrinks a smoke puff
	// relative to how big it already is, and folding that to a number at
	// migration time makes it zero, which is a different animation entirely.
	ArgExprs []Expr `json:"arg_exprs,omitempty"`
	// Flags is the argument's flag words, verbatim, for the ops whose behaviour
	// turns on them.
	Flags string `json:"flags,omitempty"`

	// Source is the DECORATE call this op came from, kept so a compiled weapon
	// can be read against the script it was made from without opening both.
	Source string `json:"source,omitempty"`
}

// The op names. Anything outside this list fails validation on load.
const (
	OpWeaponReady     = "weapon_ready"
	OpRaise           = "raise"
	OpLower           = "lower"
	OpGive            = "give"
	OpTake            = "take"
	OpTakeAmmo        = "take_ammo"
	OpJumpIfInventory = "jump_if_inventory"
	OpJumpIfNoAmmo    = "jump_if_no_ammo"
	OpJumpRandom      = "jump_random"
	OpRefire          = "refire"
	// OpSpin is not something a script asks for: it is the stepper saying it
	// walked a circle of steps that cost no time and gave up for this update.
	OpSpin = "spin"
)

// The inputs a control item can stand for. They are the names the compiled
// weapon uses, so the engine and the migration agree without either knowing a
// mod's own vocabulary.
const (
	ButtonAttack = "attack"
	// ButtonAlt is the second attack button. Final Doomer's own control script
	// keeps an item for it as well as for the trigger, and its C4 charge is
	// detonated by reading them.
	ButtonAlt        = "alt"
	ButtonReload     = "reload"
	OpFireBullets    = "fire_bullets"
	OpFireProjectile = "fire_projectile"
	OpSpawnActor     = "spawn_actor"
	OpPlaySound      = "play_sound"
	OpStopSound      = "stop_sound"
	OpPrint          = "print"
	OpGunFlash       = "gun_flash"
	OpFlashLight     = "flash_light"
	OpRecoil         = "recoil"
	OpMakeNoise      = "make_noise"
	OpJumpIf         = "jump_if"
	OpSetVar         = "set_var"
	OpCountdown      = "countdown"
	OpSetTics        = "set_tics"

	// What a shot or a blast does to the world.
	OpExplode      = "explode"
	OpRadiusThrust = "radius_thrust"
	OpRadiusGive   = "radius_give"
	OpRailAttack   = "rail_attack"
	OpCustomPunch  = "custom_punch"
	OpSaw          = "saw"
	OpBFGSpray     = "bfg_spray"
	OpQuake        = "quake"
	OpDie          = "die"

	// How an actor moves.
	OpChangeVelocity = "change_velocity"
	OpScaleVelocity  = "scale_velocity"
	OpSetSpeed       = "set_speed"
	OpThrust         = "thrust"
	OpWarp           = "warp"
	OpSeekerMissile  = "seeker_missile"

	// What an actor puts into the world, and what it takes out.
	OpSpawnItem     = "spawn_item"
	OpSpawnParticle = "spawn_particle"
	OpRemove        = "remove"
	OpKillChildren  = "kill_children"

	// How an actor is drawn.
	OpSetAlpha = "set_alpha"
	OpFadeTo   = "fade_to"
	OpSetScale = "set_scale"

	// The player's own view.
	OpZoom         = "zoom"
	OpSetCrosshair = "set_crosshair"
	OpSelectWeapon = "select_weapon"
	OpPlayerFlag   = "player_flag"
	OpRunScript    = "run_script"

	// Jumps that need to look at the world.
	OpCheckFloor     = "check_floor"
	OpCheckCeiling   = "check_ceiling"
	OpCheckSight     = "check_sight"
	OpCheckFlag      = "check_flag"
	OpCheckTarget    = "check_target"
	OpCheckHealth    = "check_health"
	OpCheckTargetInv = "check_target_inventory"
	OpGiveToTarget   = "give_to_target"
	OpCheckReload    = "check_reload"
	OpCheckCloser    = "check_closer"

	// OpMonsterAction is one instruction of monster behaviour, with the name of
	// the thing to do carried as data.
	//
	// Every other op names something the engine acts on. Monster behaviour is a
	// whole subsystem this game does not have - there are no monsters - and
	// spelling out its thirty odd actions as thirty odd ops the engine ignores
	// would say less, not more. One op says the honest thing: this is the AI,
	// it is compiled and carried, and it is dispatched the day there is AI.
	OpMonsterAction = "monster_action"

	OpGoto = "goto"
	OpLoop = "loop"
	OpStop = "stop"
	// OpNop is an action the policy deliberately drops. It carries its Source so
	// the decision stays visible in the compiled weapon.
	OpNop = "nop"
)

// KnownOps is every op this build understands, which is what load-time
// validation checks against.
var KnownOps = map[string]bool{
	OpWeaponReady: true, OpRaise: true, OpLower: true,
	OpGive: true, OpTake: true, OpTakeAmmo: true,
	OpJumpIfInventory: true, OpJumpIfNoAmmo: true, OpJumpRandom: true, OpRefire: true,
	OpFireBullets: true, OpFireProjectile: true, OpSpawnActor: true,
	OpPlaySound: true, OpStopSound: true, OpPrint: true,
	OpGunFlash: true, OpFlashLight: true,
	OpRecoil: true, OpMakeNoise: true,
	OpGoto: true, OpLoop: true, OpStop: true, OpNop: true,

	OpJumpIf: true, OpSetVar: true, OpCountdown: true, OpSetTics: true,

	OpExplode: true, OpRadiusThrust: true, OpRadiusGive: true,
	OpRailAttack: true, OpCustomPunch: true, OpSaw: true,
	OpBFGSpray: true, OpQuake: true, OpDie: true,

	OpChangeVelocity: true, OpScaleVelocity: true, OpSetSpeed: true,
	OpThrust: true, OpWarp: true, OpSeekerMissile: true,

	OpSpawnItem: true, OpSpawnParticle: true, OpRemove: true, OpKillChildren: true,

	OpSetAlpha: true, OpFadeTo: true, OpSetScale: true,

	OpZoom: true, OpSetCrosshair: true, OpSelectWeapon: true,
	OpPlayerFlag: true, OpRunScript: true,

	OpCheckFloor: true, OpCheckCeiling: true, OpCheckSight: true,
	OpCheckFlag: true, OpCheckTarget: true, OpCheckHealth: true,
	OpCheckTargetInv: true, OpGiveToTarget: true, OpCheckReload: true,
	OpCheckCloser: true, OpMonsterAction: true,
}

// ArgValues is the op's numbers, worked out now if they depend on the actor.
func (o Op) ArgValues(vars Vars, random *rand.Rand) []float64 {
	if len(o.ArgExprs) == 0 {
		return o.Args
	}
	values := make([]float64, len(o.ArgExprs))
	for i, expr := range o.ArgExprs {
		values[i] = expr.Eval(vars, random)
	}
	return values
}

// Step is one entry in a state machine: what to show, for how long, and what to
// do on arrival.
type Step struct {
	// Sprite names a frame in the folder's atlas. Empty means show nothing,
	// which is what ZDoom's TNT1 frames are for.
	//
	// It can also be one of DECORATE's keep markers, which name no picture of
	// their own and are read against whatever the actor is already showing:
	// "####" for the sprite, "----" for the sprite and the frame letter, and a
	// frame letter of "#" for the letter alone. They stay markers here rather
	// than being folded in at migration time because what they keep is a
	// runtime fact: an actor whose child overrides Spawn shows the child's
	// picture in the parent's "####" states, which is a thing only the cursor
	// knows. Cursor.Sprite resolves them; nothing else should read Sprite
	// directly.
	Sprite string `json:"sprite,omitempty"`
	// SpriteAlt is the pictures such a marker can land on - one per sprite the
	// actor uses - because a name half taken from the actor and half from the
	// step cannot be spelled out until both halves are known, and in the shared
	// store the name carries a hash of the picture that no amount of spelling
	// will produce. Empty when the marker keeps both halves, which needs no
	// name at all.
	SpriteAlt []string `json:"sprite_alt,omitempty"`
	Tics      int      `json:"tics"`
	// TicsMax above Tics means the script wrote a range and the duration is
	// picked per visit.
	TicsMax int  `json:"tics_max,omitempty"`
	Bright  bool `json:"bright,omitempty"`
	OffsetX int  `json:"offset_x,omitempty"`
	OffsetY int  `json:"offset_y,omitempty"`
	Ops     []Op `json:"ops,omitempty"`
}

// StateMachine keeps ZDoom's shape: one flat list of steps, with labels as
// offsets into it, so a label runs on into the next one unless a step jumps
// away.
type StateMachine struct {
	Steps  []Step         `json:"steps"`
	Labels map[string]int `json:"labels"`
}

// Label finds where a label starts, case-insensitively.
func (sm StateMachine) Label(name string) (int, bool) {
	index, ok := sm.Labels[strings.ToLower(name)]
	return index, ok
}

// Ammo is what a weapon spends when it fires.
type Ammo struct {
	// Type is the inventory item, empty for a weapon that costs nothing.
	Type string `json:"type,omitempty"`
	Use  int    `json:"use,omitempty"`
	// Type2 and Use2 are AmmoType2/AmmoUse2: a second ammo pool, alongside
	// Type rather than instead of it. A two-stage reload (a magazine drawn
	// from a reserve) is the common shape - Type is the reserve, Type2 the
	// loaded rounds - and both need to start full, or the weapon reads as
	// permanently needing a reload it can never finish from nothing.
	Type2 string `json:"type2,omitempty"`
	Use2  int    `json:"use2,omitempty"`
}

// Render is how the weapon or actor is drawn.
type Render struct {
	// Scale multiplies the sprite's size. It composes with the game's own UI
	// scale rather than replacing it.
	Scale [2]float64 `json:"scale"`
	// YAdjust shifts a weapon sprite vertically, in the sprite's own pixels.
	YAdjust int     `json:"y_adjust,omitempty"`
	Alpha   float64 `json:"alpha,omitempty"`
	// Style is the ZDoom RenderStyle name, lowercased, eg. "translucent".
	Style string `json:"style,omitempty"`
}

// Item is an inventory item a weapon uses: ammo, or a counter it branches on.
type Item struct {
	Max int `json:"max"`
	// Initial is what the player starts with.
	Initial int `json:"initial,omitempty"`
	// Duration is ZDoom's Powerup.Duration in tics: an item that takes itself
	// back after a while. Several of these mods use one as a pulse rather than
	// as a power - Final Doomer's C4 detonator is an item that exists for two
	// tics, which is long enough for every charge in the air to notice it and
	// short enough that the next charge thrown is not blown up on the way out.
	// Zero for an item that stays until something takes it.
	Duration int `json:"duration,omitempty"`
}

// Provenance records where a compiled weapon came from, so a shipped weapon
// carries its own attribution and a re-migration says what moved.
type Provenance struct {
	Source     string `json:"source"`
	Revision   string `json:"revision,omitempty"`
	Actor      string `json:"actor"`
	SourceFile string `json:"source_file"`
	Tool       int    `json:"tool"`
	Credit     string `json:"credit,omitempty"`
}

// Actor is something a weapon puts into the world: a muzzle smoke puff, an
// ejected shell casing, the puff a hitscan leaves where it lands.
//
// Actors live in their own files, shared between the weapons that spawn them
// and named by the hash of what is in them. Two weapons that spawn the same
// smoke point at the same file, and because the name is the content, they
// cannot disagree about what it holds and there is no order to load them in.
type Actor struct {
	Format int          `json:"format"`
	Name   string       `json:"name"`
	Render Render       `json:"render"`
	Speed  float64      `json:"speed,omitempty"`
	States StateMachine `json:"states"`
	// NoGravity actors hang where they are put, which is what smoke does.
	NoGravity bool `json:"no_gravity,omitempty"`
	// Missile marks an actor that dies on the floor rather than settling into
	// its own spin forever: a shell casing's tumble is a Loop with no exit of
	// its own, and it is the engine's +MISSILE handling that sends it to
	// Death the moment it lands. Speed alone does not say this - a casing is
	// launched by the spawner's own velocity, not a Speed property on itself.
	Missile bool `json:"missile,omitempty"`
	// Decal is the mark this actor leaves on the wall it hits, by ZDoom's own
	// name for it - "BulletChip", "Scorch", "HeatSlash". Empty for an actor
	// that marks nothing, which is what `Decal "None"` says and what an actor
	// with no Decal property anywhere in its line inherits: ZDoom's BulletPuff
	// carries none, and Doom's bullet chips come from the weapon.
	Decal string `json:"decal,omitempty"`
	// NoDecal is +NODECAL, which stops a shot marking the wall whatever the
	// weapon that fired it says.
	NoDecal bool `json:"no_decal,omitempty"`
	// SeeSound is what a missile makes as it leaves the barrel. ZDoom plays it
	// from P_SpawnMissile rather than from any state, so it appears nowhere in
	// the actor's script: Final Doomer's plasma rifle has no A_PlaySound of
	// its own anywhere in Fire, and its whole firing sound is one SeeSound on
	// the ball it throws.
	SeeSound string `json:"see_sound,omitempty"`
	// Impact is what this actor does where a hitscan lands, when it is used as
	// a puff. Empty for actors that are only drawn.
	Impact string `json:"impact,omitempty"`
	// Stock marks an actor the mod names but does not define, because GZDoom
	// provides it: a plain bullet puff, an exploding barrel. It has no states
	// of its own here, and the engine supplies the behaviour.
	Stock bool `json:"stock,omitempty"`
}

// Weapon is one compiled weapon: weapon.json.
type Weapon struct {
	Format int    `json:"format"`
	ID     string `json:"id"`
	Tag    string `json:"tag"`
	Slot   int    `json:"slot,omitempty"`

	Ammo   Ammo            `json:"ammo"`
	Items  map[string]Item `json:"items,omitempty"`
	Render Render          `json:"render"`

	// NoAutoFire is ZDoom's +WEAPON.NOAUTOFIRE: the weapon fires once for each
	// time the trigger goes down, rather than for as long as it is held. It is
	// what makes a pistol a pistol - without it a semi-automatic reads as a
	// machine gun, because its firing states take two tics and its ready frame
	// starts them again the moment they end.
	NoAutoFire bool `json:"no_auto_fire,omitempty"`

	// Buttons are the items a mod's own control script keeps in step with what
	// the player is holding down, mapped to the input that drives each. Brutal
	// Doom runs an ACS loop that gives FiredPrimary while the attack button is
	// down and takes it back four tics later, and its weapons read that rather
	// than the button: a full automatic loops on it at the end of its firing
	// states, and a weapon with no such loop fires once a press. Nothing here
	// runs ACS, so the engine keeps these items itself.
	Buttons map[string]string `json:"buttons,omitempty"`

	// Decal is the mark this weapon's shots leave on a wall, which in ZDoom
	// takes precedence over the puff's own: SpawnShootDecal reads the shooter's
	// ready weapon first and only falls back on the puff. Doom's hitscan
	// weapons carry "BulletChip"; a weapon that names nothing leaves the
	// decision to the puff.
	Decal string `json:"decal,omitempty"`

	// Reloadable is set when the weapon has a Reload label to jump to. Weapons
	// that pump inside their own firing states, as the Borstal shotgun does,
	// are not reloadable and the reload key does nothing for them.
	Reloadable bool `json:"reloadable"`

	States StateMachine `json:"states"`

	// Sounds maps a cue name to its file, relative to the weapon folder.
	Sounds map[string]string `json:"sounds,omitempty"`
	// Actors maps a lowercased actor name to the file holding it, under the
	// weapons folder's shared actor directory.
	Actors map[string]string `json:"actors,omitempty"`

	Provenance Provenance `json:"provenance"`
	// Dropped lists the DECORATE calls the policy deliberately did not compile,
	// so what was left out is visible without re-reading the mod.
	Dropped []string `json:"dropped,omitempty"`
	// Defaulted lists the puffs and spawned actors the policy had no line for
	// and gave its default to. Naming them here keeps the decision reviewable
	// after the fact, which is the trade for not having to name every one of a
	// mod's hundred spawners up front.
	Defaulted []string `json:"defaulted,omitempty"`
}

// Actors is actors.json: everything a weapon references.
type Actors struct {
	Format int               `json:"format"`
	Actors map[string]*Actor `json:"actors"`
}
