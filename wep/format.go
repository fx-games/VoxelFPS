// Package wep is the compiled weapon format: what the migration writes and what
// the engine reads. It holds no ZDoom knowledge. A weapon written by hand and a
// weapon migrated from a mod are the same thing here.
package wep

import "strings"

// Format is the version of the on-disk layout this build reads and writes. A
// weapons folder built by a different version says so on load, instead of
// failing three checks later on something that looks unrelated.
const Format = 1

// File names inside a weapon's folder.
const (
	WeaponFile = "weapon.json"
	ActorsFile = "actors.json"
	AtlasFile  = "sprites.png"
	SoundsDir  = "sounds"
	SourceFile = "source.txt"
	// KeyMapFile lives in the weapons folder itself, not in a weapon's folder.
	KeyMapFile = "keys.txt"
	// AtlasChunk is the PNG chunk the frame table rides in.
	AtlasChunk = "atLS"
)

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
	AllowFire   bool `json:"allow_fire,omitempty"`
	AllowReload bool `json:"allow_reload,omitempty"`

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

	// spawn_actor and fire_projectile.
	Actor       string  `json:"actor,omitempty"`
	Angle       float64 `json:"angle,omitempty"`
	SpawnOffset float64 `json:"spawn_offset,omitempty"`
	// SpawnOffsetTo above SpawnOffset means the script asked for a random
	// sideways offset, as the shell casings do.
	SpawnOffsetTo float64 `json:"spawn_offset_to,omitempty"`
	SpawnHeight   float64 `json:"spawn_height,omitempty"`

	// flash_light and recoil.
	Level int     `json:"level,omitempty"`
	Delta float64 `json:"delta,omitempty"`

	// make_noise.
	Radius float64 `json:"radius,omitempty"`

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
	OpRefire          = "refire"
	OpFireBullets     = "fire_bullets"
	OpFireProjectile  = "fire_projectile"
	OpSpawnActor      = "spawn_actor"
	OpPlaySound       = "play_sound"
	OpGunFlash        = "gun_flash"
	OpFlashLight      = "flash_light"
	OpRecoil          = "recoil"
	OpMakeNoise       = "make_noise"
	OpGoto            = "goto"
	OpLoop            = "loop"
	OpStop            = "stop"
	OpWait            = "wait"
	// OpNop is an action the policy deliberately drops. It carries its Source so
	// the decision stays visible in the compiled weapon.
	OpNop = "nop"
)

// KnownOps is every op this build understands, which is what load-time
// validation checks against.
var KnownOps = map[string]bool{
	OpWeaponReady: true, OpRaise: true, OpLower: true,
	OpGive: true, OpTake: true, OpTakeAmmo: true,
	OpJumpIfInventory: true, OpRefire: true,
	OpFireBullets: true, OpFireProjectile: true, OpSpawnActor: true,
	OpPlaySound: true, OpGunFlash: true, OpFlashLight: true,
	OpRecoil: true, OpMakeNoise: true,
	OpGoto: true, OpLoop: true, OpStop: true, OpWait: true, OpNop: true,
}

// Step is one entry in a state machine: what to show, for how long, and what to
// do on arrival.
type Step struct {
	// Sprite names a frame in the folder's atlas. Empty means show nothing,
	// which is what ZDoom's TNT1 frames are for.
	Sprite string `json:"sprite,omitempty"`
	Tics   int    `json:"tics"`
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
type Actor struct {
	Name   string       `json:"name"`
	Render Render       `json:"render"`
	Speed  float64      `json:"speed,omitempty"`
	States StateMachine `json:"states"`
	// NoGravity actors hang where they are put, which is what smoke does.
	NoGravity bool `json:"no_gravity,omitempty"`
	// Impact is what this actor does where a hitscan lands, when it is used as
	// a puff. Empty for actors that are only drawn.
	Impact string `json:"impact,omitempty"`
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

	// Reloadable is set when the weapon has a Reload label to jump to. Weapons
	// that pump inside their own firing states, as the Borstal shotgun does,
	// are not reloadable and the reload key does nothing for them.
	Reloadable bool `json:"reloadable"`

	States StateMachine `json:"states"`

	// Sounds maps a cue name to its file, relative to the weapon folder.
	Sounds map[string]string `json:"sounds,omitempty"`

	Provenance Provenance `json:"provenance"`
	// Dropped lists the DECORATE calls the policy deliberately did not compile,
	// so what was left out is visible without re-reading the mod.
	Dropped []string `json:"dropped,omitempty"`
}

// Actors is actors.json: everything a weapon references.
type Actors struct {
	Format int               `json:"format"`
	Actors map[string]*Actor `json:"actors"`
}
