package wep

// Building a weapon folder out of a plain grid of frames, for weapons that were
// never in a Doom mod. tools/gensheet is the command form of this; the DOS-game
// importers call it directly, because a Wolfenstein-family weapon is a grid of
// frames and nothing else.

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxellib/tpack"
)

// GridWeapon is everything a weapon cut out of a grid needs. Only Sheet, ID,
// Prefix and Frames are required; the rest has a default that produces the
// smallest weapon that works - an idle frame that takes the trigger, a firing
// animation that goes off once and comes back, and the raise and lower a
// weapon needs to be swapped in and out.
type GridWeapon struct {
	// Sheet is the grid every frame is cut from, and FrameW/FrameH one cell of
	// it. Frames are the cells of the firing animation, in order, by index -
	// the first is also the idle frame and the weapon's own picture.
	Sheet          image.Image
	FrameW, FrameH int
	Frames         []int

	// ID is the folder name, Tag what the player sees, Prefix the four letters
	// the frames are named with, Doom style.
	ID, Tag, Prefix string

	// Tics is how long each firing frame lasts, Scale how many times every
	// frame is magnified on the way in (a 64x64 DOS tile is a fifth of the
	// width of Doom's weapon space), Damage what one shot does.
	Tics, Scale, Damage int

	// Melee makes this a swing rather than a shot: the same animation, but the
	// hit is A_CustomPunch's, which only reaches as far as Range.
	Melee bool
	Range float64

	// Beam makes the shot a rail attack: one ray straight down the line of
	// fire, with a bolt of light drawn along it as far as it got. It is what
	// System Shock's three energy weapons do, and the beam is the whole of what
	// makes them look like energy weapons - none of it is in their art.
	Beam bool

	// Projectile, when set, makes the weapon fire that actor instead of a
	// hitscan, by the name it is keyed under in Actors. Actors are shared
	// between weapons and stored by the hash of what is in them, so two
	// weapons throwing the same grenade write the same file.
	//
	// ActorSprites are the frames those actors are drawn with, by the bare
	// name their steps use. They go in the shared picture store under a name
	// carrying the picture's own hash, and the steps are rewritten to match -
	// a sprite name is only unique inside the game it came from.
	Projectile   string
	Actors       map[string]*Effect
	ActorSprites map[string]image.Image

	// FireSound is a cue this weapon's own sounds folder holds, played on the
	// first firing frame. Sounds are the files to copy in, by the cue name
	// they answer to.
	FireSound string
	Sounds    map[string]io.Reader

	// Ammo is what a shot spends, AmmoUse how much of it one shot takes, and
	// AmmoMax how much of it can be carried. A weapon that names none never
	// runs out, which is what a knife wants; AmmoUse zero means one.
	Ammo    string
	AmmoUse int
	AmmoMax int

	// Pickup is the picture of the weapon lying on the floor, which is also
	// what the HUD and the inventory draw it as. Optional: without one the idle
	// frame stands in, which is a first-person view of a gun held at the bottom
	// of the screen and reads badly at icon size. Both DOS games ship real
	// pickup art - Blake Stone's floor sprites, one per gun - so pass it.
	Pickup image.Image

	// Provenance says which game and which of its own names this came out of.
	Source, SourceFile string
}

// WriteGridWeapon writes the whole folder: the packed atlas, the sounds, the
// weapon.json, and the weapon's own picture into the shared store the HUD and a
// pickup on the floor draw from.
func WriteGridWeapon(weaponsDir string, spec GridWeapon) error {
	if spec.Sheet == nil || spec.ID == "" || len(spec.Prefix) != 4 || len(spec.Frames) == 0 {
		return fmt.Errorf("grid weapon %q: needs a sheet, an id, a four-letter prefix and frames", spec.ID)
	}
	if len(spec.Frames) > 26 {
		return fmt.Errorf("grid weapon %q: %d frames, and Doom frame letters run out at 26", spec.ID, len(spec.Frames))
	}
	if spec.Tics <= 0 {
		spec.Tics = 2
	}
	if spec.Scale <= 0 {
		spec.Scale = 1
	}
	if spec.Damage <= 0 {
		spec.Damage = 5
	}

	dir := filepath.Join(weaponsDir, spec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var images []tpack.NamedImage
	var names []string
	perRow := spec.Sheet.Bounds().Dx() / spec.FrameW
	for position, index := range spec.Frames {
		rect := image.Rect(
			(index%perRow)*spec.FrameW, (index/perRow)*spec.FrameH,
			(index%perRow+1)*spec.FrameW, (index/perRow+1)*spec.FrameH,
		).Add(spec.Sheet.Bounds().Min)
		name := fmt.Sprintf("%s%c0", strings.ToUpper(spec.Prefix), 'A'+rune(position))
		frame := Magnified(Cropped(spec.Sheet, rect), spec.Scale)
		hotspotX, hotspotY := FrameHotspot(frame.Bounds().Dx(), frame.Bounds().Dy())
		names = append(names, name)
		images = append(images, tpack.NamedImage{
			Image: frame, Name: name, OffsetX: hotspotX, OffsetY: hotspotY,
		})
	}
	if err := WriteAtlas(images, filepath.Join(dir, AtlasFile)); err != nil {
		return err
	}

	weapon := spec.build(names)
	if err := spec.writeSounds(dir, weapon); err != nil {
		return err
	}
	iconName, iconImage := names[0], images[0].Image
	if spec.Pickup != nil {
		iconName, iconImage = names[0]+"_PICKUP", spec.Pickup
	}
	icon, err := writeSharedIcon(weaponsDir, iconName, iconImage)
	if err != nil {
		return err
	}
	weapon.Icon = icon
	if err := MeasurePickups(weaponsDir, weapon); err != nil {
		return err
	}
	if err := spec.writeActors(weaponsDir, weapon); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(dir, WeaponFile), weapon)
}

// build is the state machine: ready, fire, select, deselect, and nothing else.
func (spec GridWeapon) build(names []string) *Weapon {
	idle := names[0]
	states := StateMachine{Labels: map[string]int{}}

	states.Labels["ready"] = len(states.Steps)
	states.Steps = append(states.Steps,
		Step{Sprite: idle, Tics: 1, Ops: []Op{{Op: OpWeaponReady, AllowFire: true}}},
		Step{Tics: 0, Ops: []Op{{Op: OpLoop}}},
	)

	states.Labels["fire"] = len(states.Steps)
	for position, name := range names {
		step := Step{Sprite: name, Tics: spec.Tics}
		if position == 0 {
			step.Ops = spec.fireOps()
		}
		states.Steps = append(states.Steps, step)
	}
	states.Steps = append(states.Steps,
		Step{Tics: 0, Ops: []Op{{Op: OpRefire, To: "fire"}}},
		Step{Tics: 0, Ops: []Op{{Op: OpGoto, To: "ready"}}},
	)

	states.Labels["select"] = len(states.Steps)
	states.Steps = append(states.Steps,
		Step{Sprite: idle, Tics: 1, Ops: []Op{{Op: OpRaise}}},
		Step{Tics: 0, Ops: []Op{{Op: OpLoop}}},
	)

	states.Labels["deselect"] = len(states.Steps)
	states.Steps = append(states.Steps,
		Step{Sprite: idle, Tics: 1, Ops: []Op{{Op: OpLower}}},
		Step{Tics: 0, Ops: []Op{{Op: OpLoop}}},
	)

	tag := spec.Tag
	if tag == "" {
		tag = spec.ID
	}
	weapon := &Weapon{
		Format: Format,
		ID:     spec.ID,
		Tag:    tag,
		Render: Render{Scale: [2]float64{1, 1}, Alpha: 1},
		States: states,
		Provenance: Provenance{
			Source:     spec.Source,
			Actor:      spec.ID,
			SourceFile: spec.SourceFile,
			Tool:       Format,
		},
	}
	if spec.Ammo != "" {
		use := spec.AmmoUse
		if use <= 0 {
			use = 1
		}
		weapon.Ammo = Ammo{Type: spec.Ammo, Use: use}
		weapon.AmmoPickup = spec.Ammo
		limit := spec.AmmoMax
		if limit <= 0 {
			limit = 100
		}
		weapon.Items = map[string]Item{spec.Ammo: {Max: limit, Initial: limit}}
	}
	return weapon
}

// fireOps is what the first firing frame does: a swing, a missile or a bullet,
// and the noise it makes.
func (spec GridWeapon) fireOps() []Op {
	var ops []Op
	if spec.FireSound != "" {
		ops = append(ops, Op{Op: OpPlaySound, Sound: spec.FireSound, Volume: 1})
	}
	switch {
	case spec.Beam:
		ops = append(ops, Op{Op: OpRailAttack, Damage: spec.Damage, Puff: PuffBullet})
	case spec.Melee:
		reach := spec.Range
		if reach <= 0 {
			reach = 64
		}
		// A_CustomPunch's reach is its own first argument, in Doom units.
		ops = append(ops, Op{Op: OpCustomPunch, Damage: spec.Damage, Args: []float64{reach}, Puff: PuffBullet})
	case spec.Projectile != "":
		ops = append(ops, Op{Op: OpFireProjectile, Actor: spec.Projectile})
	default:
		ops = append(ops, Op{Op: OpFireBullets, Count: 1, SpreadH: 1, SpreadV: 1, Damage: spec.Damage, Puff: PuffBullet})
	}
	return ops
}

// writeSounds copies the cues into the folder's own sounds/ directory and
// records them on the weapon, which is what makes PlayCue find them: the
// weapon's ID is the scope its whole tree is loaded under.
func (spec GridWeapon) writeSounds(dir string, weapon *Weapon) error {
	if len(spec.Sounds) == 0 {
		return nil
	}
	root := filepath.Join(dir, SoundsDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	weapon.Sounds = map[string]string{}
	for cue, source := range spec.Sounds {
		relative := filepath.Join(SoundsDir, cue+".wav")
		out, err := os.Create(filepath.Join(dir, relative))
		if err != nil {
			return err
		}
		_, err = io.Copy(out, source)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		weapon.Sounds[cue] = filepath.ToSlash(relative)
	}
	return nil
}

// writeActors puts every actor this weapon spawns in the shared store under the
// hash of what is in it, and their frames in the shared picture store under a
// name carrying the picture's own hash. The steps are rewritten to the stored
// names on the way, because that is what the loader checks them against.
func (spec GridWeapon) writeActors(weaponsDir string, weapon *Weapon) error {
	if len(spec.Actors) == 0 {
		return nil
	}
	store := filepath.Join(weaponsDir, SpritesDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	stored := make(map[string]string, len(spec.ActorSprites))
	for name, picture := range spec.ActorSprites {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, picture); err != nil {
			return err
		}
		file := SharedSpriteName(name, encoded.Bytes())
		if err := os.WriteFile(filepath.Join(store, file+".png"), encoded.Bytes(), 0o644); err != nil {
			return err
		}
		stored[name] = file
	}

	actorStore := filepath.Join(weaponsDir, ActorsDir)
	if err := os.MkdirAll(actorStore, 0o755); err != nil {
		return err
	}
	weapon.Actors = make(map[string]string, len(spec.Actors))
	for name, actor := range spec.Actors {
		actor.Format = Format
		for i := range actor.States.Steps {
			if renamed, ok := stored[actor.States.Steps[i].Sprite]; ok {
				actor.States.Steps[i].Sprite = renamed
			}
		}
		file, data, err := HashJSON(actor)
		if err != nil {
			return err
		}
		weapon.Actors[strings.ToLower(name)] = file
		if err := os.WriteFile(filepath.Join(actorStore, file), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeSharedIcon puts the idle frame in the shared picture store, which is
// where a pickup on the floor, the inventory and the HUD draw a weapon nobody is
// holding from - only the weapon in hand has its own atlas loaded. A grid sheet
// carries no separate pickup art, so the idle frame is the picture there is.
//
// Written without a hotspot: the frame's own is the weapon-space anchor below,
// which means nothing to a billboard lying on the floor.
func writeSharedIcon(weaponsDir, name string, frame image.Image) (string, error) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		return "", err
	}
	store := filepath.Join(weaponsDir, SpritesDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return "", err
	}
	stored := SharedSpriteName(name, encoded.Bytes())
	if err := os.WriteFile(filepath.Join(store, stored+".png"), encoded.Bytes(), 0o644); err != nil {
		return "", err
	}
	return stored, nil
}

// FrameHotspot is the hotspot every vanilla Doom weapon frame carries, given to
// a frame that has none: centred across Doom's 320x200 weapon space, with its
// bottom edge on the line at 168 that the status bar used to cover.
//
// A grid sheet has no hotspot to read. One rides in a picture's own grAb chunk
// and a grid is one picture with no chunk, so every frame cut out of it comes
// out zero - and zero puts a frame's top left corner in the middle of the
// space, which is where the Blake Stone pistol was: a thumbnail up in the left
// of the screen.
//
// 168 rather than 200 because the ready state holds a weapon at psy=32, Doom's
// WEAPONTOP, and 32 + 168 is 200: a frame anchored on that line comes to rest
// with its bottom edge on the bottom of the screen, which is where a weapon
// sits. Anchoring at 200 instead hangs it 32 units lower and the hand goes off
// the bottom.
func FrameHotspot(width, height int) (x, y int) {
	const spaceWidth, weaponBottom = 320, 168
	return width/2 - spaceWidth/2, height - weaponBottom
}

// Magnified blows a frame up by a whole number of pixels each way, nearest
// neighbour, which is the only enlargement that leaves pixel art as pixel art.
//
// It happens here rather than at draw time because there is nowhere at draw
// time to put it: every other weapon in the folder is authored at the size it
// is drawn, and the engine has no per-weapon magnification to add one for one
// sheet alone. A 64x64 Wolf3D tile is a fifth of the width of Doom's weapon
// space, so it is enlarged once, into the folder, and comes out an ordinary
// weapon.
func Magnified(source image.Image, times int) image.Image {
	if times <= 1 {
		return source
	}
	bounds := source.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx()*times, bounds.Dy()*times))
	for y := 0; y < bounds.Dy()*times; y++ {
		for x := 0; x < bounds.Dx()*times; x++ {
			out.Set(x, y, source.At(bounds.Min.X+x/times, bounds.Min.Y+y/times))
		}
	}
	return out
}

// Cropped cuts one frame out of a sheet, into an image whose own origin is
// zero, which is what the packer wants.
func Cropped(source image.Image, rect image.Rectangle) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), source, rect.Min, draw.Src)
	return out
}
