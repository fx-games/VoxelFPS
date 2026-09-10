package main

// The things that stand in the station rather than walk about it, and the
// sixteen guns.
//
// Every object in the game - a crate, a corpse, a med patch, a gun on the floor
// - has its pictures in one compound resource, objart.res 0x0546, whose blocks
// run object by object in the order the property table lists them. Each object
// takes three blocks plus however many extra in-world frames it declares: the
// first is the picture an editor would show, the middle ones are what the thing
// looks like in the world, and the last is a micro-image for the automap.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"github.com/inkyblackness/hacked/ss1/content/object"
	"github.com/memmaker/goray/wep"
)

// objectBlockStart is the first block of the first object. Block zero of the
// resource belongs to nothing.
const objectBlockStart = 1

// objectFrames is where one object's in-world pictures are: the block its own
// run starts at, and how many of them are the thing in the world rather than an
// icon.
//
// The run is laid out the way objload.c walks it: block start is the object's
// 2D bitmap - the inventory icon, the picture of the thing in your hands - then
// its in-world frames, then the editor's icon. Which is why the icon and the
// world art come out of the same run at different offsets.
type objectFrames struct {
	triple object.Triple
	prop   *object.Properties
	start  int
	world  int
}

// icon is the block holding this object's inventory picture.
func (o objectFrames) icon() int { return o.start }

// objectLayout walks the property table once and works out every object's own
// run of blocks. The run lengths differ, so this is a running total and cannot
// be done for one object on its own.
func (in *installer) objectLayout() []objectFrames {
	var out []objectFrames
	block := objectBlockStart
	in.properties.Iterate(func(triple object.Triple, prop *object.Properties) bool {
		extra := int(prop.Common.Bitmap3D.FrameNumber())
		out = append(out, objectFrames{triple: triple, prop: prop, start: block, world: extra + 1})
		block += 3 + extra
		return true
	})
	return out
}

// viewCount is how many ways round the engine can look at a sprite, and so how
// many frames a set of views has to have to be one.
const viewCount = 8

// installObjects writes a folder per object that has a picture worth standing
// in a level: every in-world frame it has, as one animation. An object the game
// draws as a model rather than a picture is skipped - there is no art to take.
func (in *installer) installObjects() (int, error) {
	written := 0
	for _, layout := range in.objectLayout() {
		if layout.triple.Class == critterClass {
			continue // critters are folders of their own, see critters.go
		}
		frames := in.worldFrames(layout)
		if len(frames) == 0 {
			continue
		}
		name := in.objectName(layout.triple)
		id := in.folderFor(name, layout.triple)

		grid := &sheet{}
		row := grid.addAll(frames)
		kind := &spriteKind{
			Name: name,
			Animations: map[string]*spriteAnim{
				"idle": idleFrom(row, layout.triple, layout.prop.Common.Bitmap3D.Animation()),
			},
			Provenance: map[string]string{
				"source": "System Shock (1994)",
				"file":   "OBJART.RES",
				"actor":  fmt.Sprintf("%d/%d/%d", layout.triple.Class, layout.triple.Subclass, layout.triple.Type),
			},
		}
		if err := in.writeKind(id, kind, grid, 8, nil); err != nil {
			return written, fmt.Errorf("%s: %w", id, err)
		}
		written++
	}
	return written, nil
}

// idleFrom is what an object's extra pictures actually are.
//
// Only the objects whose Bitmap3D carries the animation bit are animations -
// eighty-three things in the game have more than one frame and eight of them
// animate. The rest use the frame number for something that is not time: a
// lever that is up or down, a panel that is intact or smashed, and - for the
// bodies lying about the station - eight views of the same corpse, which is the
// engine's own eight sectors and is why they were spinning through their own
// rotation as an animation.
//
// So: animated stays an animation, exactly eight stills become the eight
// facings, and anything else stands on its first frame with the others left on
// the sheet for whatever names them later.
//
// Class eleven is the exception to the flag. It is CLASS_ANIMATING - the
// explosions, the fires, the sparks - and none of its objects carries the bit,
// because the game does not put them on the animation list: they are effects,
// wound on frame by frame by effect.c. They are animations all the same.
func idleFrom(row []uint16, triple object.Triple, animated bool) *spriteAnim {
	switch {
	case animated, triple.Class == animatingClass:
		return &spriteAnim{Frames: [][]uint16{row}, FPS: 6, Loop: len(row) > 1}
	case len(row) == viewCount:
		views := make([][]uint16, len(row))
		for i, frame := range row {
			views[i] = []uint16{frame}
		}
		return &spriteAnim{Frames: views}
	default:
		return &spriteAnim{Frames: [][]uint16{{row[0]}}}
	}
}

// gunClass is which object class the guns are, and animatingClass the effects.
const (
	gunClass       = object.Class(0)
	animatingClass = object.Class(11)
)

// gunSounds is which digitised effect each gun makes, by the index the game
// gives that effect in digifx.res.
//
// The guns' own properties carry no sound index the way a critter's do - the
// engine picks the noise off the weapon's kind in code - so this is the game's
// own table: weapon_combat_fxs in GameSrc/weapons.c, indexed by the gun's place
// among the sixteen, resolved through the sfx numbers in Headers/sfxlist.h.
// Both are in the Night Dive source release; guessing at it from the sound set,
// which is what this was before, put the wrong noise on every gun.
//
// The two melee weapons name their miss, which is what the game plays when a
// swing connects with nothing. Their hits are a different sound again - meat,
// metal, and the epee's own - and nothing here spawns one yet.
var gunSounds = map[int]int{
	0:  39, // mini pistol
	1:  86, // dart pistol
	2:  40, // magnum
	3:  17, // assault rifle
	4:  41, // riot gun
	5:  38, // flechette
	6:  65, // skorpion
	7:  45, // magpulse
	8:  29, // rail gun
	9:  24, // pr-24 pipe, swinging at nothing
	10: 34, // laser epee, swinging at nothing
	11: 18, // sparq beam
	12: 94, // blaster
	13: 95, // ion beam
	14: 19, // stungun
	15: 97, // plasma rifle
}

// Gun subclasses, out of Headers/objwpn.h. What a gun does when the trigger
// goes down is decided by these and nothing else - see the switch at the end of
// player_fire_weapon in weapons.c.
const (
	gunSubclassPistol     = 0 // one shot a press, ammo
	gunSubclassAuto       = 1 // held down, as many shots as time has passed
	gunSubclassSpecial    = 2 // player_fire_slow_projectile_weapon
	gunSubclassHandToHand = 3 // a swing
	gunSubclassBeam       = 4 // a ray, instantly
	gunSubclassBeamProj   = 5 // player_fire_energy_proj, also a slow projectile
)

// The suit's power, which is the only thing the energy weapons spend. One pool
// for all five of them, exactly as the game has it: player_struct.energy, with
// MAX_ENERGY and MIN_ENERGY_USE out of gamesys.h and weapons.h.
const (
	energyPool   = "ShockEnergy"
	maxEnergy    = 255
	minEnergyUse = 10
	railGunIndex = 8
)

// energyWeapon is whether this gun spends power rather than rounds. Both energy
// subclasses do: the beams raycast through player_fire_energy and the two
// beam-projectile guns throw through player_fire_energy_proj, and each drains
// the suit before it does anything else.
func energyWeapon(subclass object.Subclass) bool {
	return subclass == gunSubclassBeam || subclass == gunSubclassBeamProj
}

// energyCost is what one shot takes out of the suit: the gun's own max_charge,
// which is the first byte after the eight of COMBAT_DATA_FIELDS in both the
// beam and the beam-projectile records.
func energyCost(specific []byte) int {
	if len(specific) <= 8 {
		return minEnergyUse
	}
	charge := int(specific[8])
	if charge < minEnergyUse {
		charge = minEnergyUse
	}
	return charge
}

// System Shock counts time in CIT_CYCLE ticks a second, and both of the fire
// rates below are in those.
const shockTicksPerSecond = 280.0

// autofireSpeed is AUTOFIRE_SPEED out of weapons.c: an automatic weapon gets
// one shot for every this many ticks-times-fire_rate that have gone by, so its
// rate goes up with fire_rate where every other weapon's goes down.
const autofireSpeed = 1500.0

// fireInterval is how long this gun waits between shots, in seconds.
//
// Two formulas, because the game has two. Everything that is not automatic is
// held off by "(GunProps[..].fire_rate * 10) + last_fire > game_time" - ten
// ticks per point of fire_rate. An automatic weapon instead accumulates
// (game_time - last_fire) * fire_rate and spends AUTOFIRE_SPEED of it a shot.
func fireInterval(subclass object.Subclass, fireRate int) float64 {
	if fireRate <= 0 {
		fireRate = 1
	}
	if subclass == gunSubclassAuto {
		return autofireSpeed / (shockTicksPerSecond * float64(fireRate))
	}
	return float64(fireRate) * 10 / shockTicksPerSecond
}

// firingTics is that interval as this engine's per-frame tics: the whole firing
// animation plays and then refires, so one shot costs frames * tics, and Doom's
// clock runs at wep.TicRate. Never zero - a frame has to last a tic.
func firingTics(interval float64, frames int) int {
	if frames <= 0 {
		frames = 1
	}
	tics := int(interval*wep.TicRate/float64(frames) + 0.5)
	if tics < 1 {
		tics = 1
	}
	return tics
}

// projectileTriple is the thing a gun throws, for the two kinds of gun that
// throw one, or the zero triple for the fourteen that do not.
//
// Both live in the subclass-specific block, after the eight bytes of
// COMBAT_DATA_FIELDS every combat record starts with (a short, a byte, an int
// and a byte, packed). SpecialGunProp puts speed next and then proj_triple;
// BeamprojGunProp puts max_charge, attack_mass, attack_speed and speed first.
// Both store the triple as MAKETRIP: class in the high byte of three.
func projectileTriple(subclass object.Subclass, specific []byte) (object.Triple, bool) {
	at := 0
	switch subclass {
	case gunSubclassSpecial:
		at = 9
	case gunSubclassBeamProj:
		at = 13
	default:
		return object.Triple{}, false
	}
	if at+4 > len(specific) {
		return object.Triple{}, false
	}
	value := int(binary.LittleEndian.Uint32(specific[at:]))
	if value == 0 {
		return object.Triple{}, false
	}
	return object.TripleFromInt(value), true
}

// installWeapons writes a folder per gun. The art is the weapon-in-hand
// pictures: System Shock draws a weapon as its own barrel at the bottom of the
// view and a flash over it, which is two frames for a gun and four for a
// swing - the whole of the animation there is.
func (in *installer) installWeapons() (int, error) {
	written := 0
	gunIndex := 0
	var err error
	layouts := map[object.Triple]objectFrames{}
	for _, layout := range in.objectLayout() {
		layouts[layout.triple] = layout
	}
	in.properties.Iterate(func(triple object.Triple, prop *object.Properties) bool {
		if triple.Class != gunClass {
			return true
		}
		if gunIndex >= handArtCount {
			return false
		}
		frames := in.resourceFrames("HANDART.RES", handArt.Plus(gunIndex))
		index := gunIndex
		gunIndex++
		if len(frames) == 0 {
			return true
		}
		name := in.objectName(triple)
		// Laid out on one canvas at the game's own per-frame offsets before
		// anything else touches them, so every frame is the same size and the
		// engine's one anchor holds all of them still - see handart.go.
		frames = layOutHandArt(frames, handFramesFor(index))
		grid := &sheet{}
		grid.addAll(frames)

		var sounds map[string]io.Reader
		fireCue := ""
		if samples := in.soundWAV(gunSounds[index]); samples != nil {
			fireCue = "fire"
			sounds = map[string]io.Reader{"fire": bytes.NewReader(samples)}
		}
		// A gun with no ammo type is a beam or a melee weapon, and neither runs
		// out of anything this engine models.
		melee := triple.Subclass == gunSubclassHandToHand
		beam := triple.Subclass == gunSubclassBeam
		// The magpulse, the rail gun, the stungun and the plasma rifle throw
		// something that travels. The game says so itself: those four are the
		// two subclasses whose case in player_fire_weapon calls
		// player_fire_slow_projectile, and each names the object it throws in
		// its own properties - see projectileTriple.
		thrown, throws := projectileTriple(triple.Subclass, prop.Specific)
		interval := fireInterval(triple.Subclass, int(prop.Generic[0]))
		spec := wep.GridWeapon{
			Sheet:     grid.image(len(grid.frames)),
			FrameW:    tileWidth(grid),
			FrameH:    tileHeight(grid),
			Frames:    indices(len(grid.frames)),
			ID:        idPrefix + slug(name),
			Tag:       name,
			Prefix:    gunPrefix(index),
			Tics:      firingTics(interval, len(grid.frames)),
			Scale:     2,
			Damage:    12,
			Melee:     melee,
			Beam:      beam,
			Range:     72,
			FireSound: fireCue,
			Sounds:    sounds,
			// The gun as it lies on the floor, which is also its inventory
			// picture: block start of the object's own run, the bitmaps_2d
			// objload.c reads before the world frames. Without it the HUD drew
			// the first-person barrel, which at icon size is a smear.
			Pickup:     in.objectIcon(layouts[triple]),
			Source:     "System Shock (1994)",
			SourceFile: "HANDART.RES",
		}
		switch {
		case melee:
			// A swing costs nothing this engine models.
		case energyWeapon(triple.Subclass):
			// The energy weapons do not carry ammunition at all: they draw on
			// the suit's own power, one pool for all of them, and a shot costs
			// the gun's own max_charge out of it - see player_fire_energy and
			// weapon_energy_drain in weapons.c. MAX_ENERGY is 255 and
			// MIN_ENERGY_USE is 10, and both are the game's.
			spec.Ammo = energyPool
			spec.AmmoMax = maxEnergy
			spec.AmmoUse = energyCost(prop.Specific)
		default:
			spec.Ammo = "ShockClip"
			spec.AmmoMax = 100
		}
		if throws && !melee {
			if actor, sprites := in.slowProjectile(layouts[thrown], spec.Prefix, index != railGunIndex); actor != nil {
				spec.Beam = false
				spec.Projectile = actor.Name
				spec.Actors = map[string]*wep.Effect{actor.Name: actor}
				spec.ActorSprites = sprites
			}
		}
		if writeErr := wep.WriteGridWeapon(weaponsDir, spec); writeErr != nil {
			err = fmt.Errorf("%s: %w", spec.ID, writeErr)
			return false
		}
		written++
		return true
	})
	return written, err
}

// gunPrefix is the four letters this gun's frames are named with. Doom names a
// sprite in exactly four, and every weapon in the shared store has to have its
// own, so they are numbered rather than abbreviated from a name that might
// collide.
func gunPrefix(index int) string {
	return fmt.Sprintf("SG%02d", index)
}

func tileWidth(s *sheet) int  { w, _ := s.tile(); return w }
func tileHeight(s *sheet) int { _, h := s.tile(); return h }

func indices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// objectIcon is one object's inventory picture, or nil for a run this importer
// could not place.
func (in *installer) objectIcon(layout objectFrames) image.Image {
	if layout.world == 0 {
		return nil
	}
	img, _ := in.blockImage("OBJART.RES", objectBitmaps, layout.icon(), true, nil)
	if img == nil {
		return nil
	}
	return img
}

// slowProjectile is the thing one of the four throwing guns throws, built out
// of that object's own in-world frames: it tumbles along on those and explodes
// where it lands.
//
// The blast is the game's own EXPLOSION object rather than anything invented
// here - one picture per frame, the four the station uses everywhere something
// goes up.
// bright says whether the thing in the air lights itself. Three of the four are
// energy - the magpulse's magnetic bolt and the two beam-projectile guns' - and
// a bolt of plasma that takes the light of the room it is crossing reads as a
// sticker. The rail gun's is a metal slug and is lit like one.
func (in *installer) slowProjectile(layout objectFrames, prefix string, bright bool) (*wep.Effect, map[string]image.Image) {
	flying := in.worldFrames(layout)
	if len(flying) == 0 {
		return nil, nil
	}
	sprites := map[string]image.Image{}
	states := wep.StateMachine{Labels: map[string]int{}}

	states.Labels["spawn"] = len(states.Steps)
	for i, img := range flying {
		name := fmt.Sprintf("%sF%c", prefix, 'A'+rune(i))
		sprites[name] = img
		states.Steps = append(states.Steps, wep.Step{Sprite: name, Tics: 3, Bright: bright})
	}
	states.Steps = append(states.Steps, wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}})

	states.Labels["death"] = len(states.Steps)
	blast := in.worldFrames(in.layoutOf(explosionTriple))
	if len(blast) == 0 {
		blast = flying[len(flying)-1:]
	}
	for i, img := range blast {
		name := fmt.Sprintf("%sX%c", prefix, 'A'+rune(i))
		sprites[name] = img
		// A blast lights itself whatever threw it.
		step := wep.Step{Sprite: name, Tics: 3, Bright: true}
		if i == 0 {
			step.Ops = []wep.Op{{Op: wep.OpExplode, Damage: 24, N: 96}}
		}
		states.Steps = append(states.Steps, step)
	}
	states.Steps = append(states.Steps, wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpRemove}}})

	// All four throwers travel at one speed, because the game gives them one:
	// the byte before proj_triple in every one of the four records reads 6,
	// the rail gun's included. Its slug is a thing that flies and takes time to
	// arrive, exactly like the magpulse's bolt - "rail gun" is the name, not
	// the mechanic. The number here is this engine's own scale, since nothing
	// says what one of System Shock's units is worth in voxels a tic.
	return &wep.Effect{
		Format:  wep.Format,
		Name:    "Shock" + prefix,
		Render:  wep.Render{Scale: [2]float64{1, 1}, Alpha: 1},
		Speed:   30,
		States:  states,
		Missile: true,
		// A magpulse slug does drop, but not over the length of a corridor, and
		// the game's SLOW_PROJECTILE_GRAVITY is a hundredth of a square a tick.
		NoGravity: true,
	}, sprites
}

// explosionTriple is the animating object every blast in the station is drawn
// with: class 11 (animating), subclass 2, type 0.
var explosionTriple = object.TripleFrom(int(animatingClass), 2, 0)

// layoutOf is one object's run of blocks, by its triple.
func (in *installer) layoutOf(triple object.Triple) objectFrames {
	for _, layout := range in.objectLayout() {
		if layout.triple == triple {
			return layout
		}
	}
	return objectFrames{}
}

// worldFrames decodes an object's in-world pictures, which are stored as
// differences from one another and so have to be walked in order.
func (in *installer) worldFrames(layout objectFrames) []image.Image {
	var out []image.Image
	var previous []byte
	for i := 0; i < layout.world; i++ {
		img, pixels := in.blockImage("OBJART.RES", objectBitmaps, layout.start+1+i, true, previous)
		if img == nil {
			break
		}
		previous = pixels
		out = append(out, img)
	}
	return out
}
