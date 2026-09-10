package main

// Filing the decoded pages where the game reads them from: walls into the
// texture library, characters and scenery into assets/sprites, the five weapons
// into assets/weapons.
//
// Nothing here knows a sprite by number. Every frame is found by the name
// bstone gives it (names.go), which is what lets one rule build a guard's whole
// animation set out of whatever frames that guard happens to have - and lets a
// character the game shipped without a pain frame come out as a character
// rather than as a crash.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
	"path/filepath"

	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/wep"
)

// installDirs are where each kind of thing lands, relative to the repo root.
const (
	textureDir = "assets/textures/blake"
	spritesDir = "assets/sprites"
	weaponsDir = "assets/weapons"
)

// idPrefix keeps everything this import writes together in a folder listing and
// out of the way of whatever else is installed - the editor's palettes are the
// folders in name order.
const idPrefix = "bs_"

// sheetColumns is how wide a generated sprite sheet is. Eight because a
// directional animation is eight views and a row of them is then one facing per
// column, which is what makes a sheet readable when somebody opens it.
const sheetColumns = 8

// spriteScale is how tall one of these characters is in voxels.
//
// It is one, and that is not a guess: the Wolfenstein engine draws a sprite
// exactly as tall as a wall is, both being its 64-unit tile, and a wall here is
// one voxel. A guard's own pixels fill about 56 of those 64, which lands him at
// 0.875 - the same height as this engine's player, who is Doom's 56 units of
// worldUnitsPerDoomUnit. Two games' people the same size is the check that the
// number is right.
//
// It was 1.7 before, which is where "everything imported is enormous" came
// from: a man drawn twice the height of the man looking at him.
const spriteScale = 1.0

// installer carries the decoded file and the reverse of the name table, which
// is the lookup every rule below uses.
type installer struct {
	vswap  *vswap
	byName map[string]int
	sounds map[string][]byte
	// taken is every folder this run has already written. The characters claim
	// theirs first, because one of the game's own static pictures is called
	// after a character that has a folder of its own - and without this the
	// picture silently overwrote the character.
	taken map[string]bool
}

func newInstaller(v *vswap) *installer {
	byName := make(map[string]int, len(spriteNames))
	for index, name := range spriteNames {
		byName[name] = index
	}
	sounds := map[string][]byte{}
	for i, d := range v.digitizedTable() {
		name := soundNames[i]
		if name == "" {
			continue
		}
		if samples := v.samples(d); len(samples) > 0 {
			sounds[name] = audio.WAVFromPCM8(samples, pcmRate)
		}
	}
	return &installer{vswap: v, byName: byName, sounds: sounds, taken: map[string]bool{}}
}

// frame decodes one sprite by its name, or nil if the game has not got it. A
// missing frame is the normal case, not an error: the rules below ask for
// frames that only some characters have.
func (in *installer) frame(name string) image.Image {
	index, ok := in.byName[name]
	if !ok {
		return nil
	}
	img, err := in.vswap.sprite(index, blakePalette)
	if err != nil {
		return nil
	}
	return img
}

// pickup is the picture of a thing lying on the floor, by its sprite name, or
// nil for a weapon that has none. Named separately from frame only so a spec
// with an empty name asks for nothing rather than for sprite "".
func (in *installer) pickup(name string) image.Image {
	if name == "" {
		return nil
	}
	return in.frame(name)
}

// series collects the frames of a numbered run - "RENT_DIE_%d", "POD_SPIT%d" -
// starting at one and stopping at the first gap.
func (in *installer) series(format string) []image.Image {
	var out []image.Image
	for i := 1; ; i++ {
		img := in.frame(fmt.Sprintf(format, i))
		if img == nil {
			return out
		}
		out = append(out, img)
	}
}

// firstSeries returns the first of these runs the game actually has. The same
// animation is spelt differently from one character to the next - DIE_1 for the
// guards, DIE1 for the mutants - and the alternative to this is a per-character
// table of spellings.
func (in *installer) firstSeries(formats ...string) []image.Image {
	for _, format := range formats {
		if frames := in.series(format); len(frames) > 0 {
			return frames
		}
	}
	return nil
}

// firstFrame is the same idea for a single picture.
func (in *installer) firstFrame(names ...string) image.Image {
	for _, name := range names {
		if img := in.frame(name); img != nil {
			return img
		}
	}
	return nil
}

// installWalls writes every wall page into the texture library, where the
// editor's picker finds it by being in a folder. Blake Stone draws each wall
// twice, light and dark, for the two axes; both are kept, because which one
// looks right on a face is the level author's call.
func (in *installer) installWalls() (int, error) {
	if err := os.MkdirAll(textureDir, 0o755); err != nil {
		return 0, err
	}
	for i := 0; i < in.vswap.wallCount(); i++ {
		img, err := in.vswap.wall(i, blakePalette)
		if err != nil {
			return i, err
		}
		if err := writePNG(filepath.Join(textureDir, fmt.Sprintf("W%03d.png", i)), img); err != nil {
			return i, err
		}
	}
	return in.vswap.wallCount(), nil
}

// sheet is a kind's picture under construction: frames are added and their
// index on the finished grid comes back, which is what the animations in
// sprite.json are written in terms of.
type sheet struct {
	frames []image.Image
	tileW  int
	tileH  int
}

func (s *sheet) add(img image.Image) uint16 {
	if s.tileW == 0 {
		s.tileW, s.tileH = img.Bounds().Dx(), img.Bounds().Dy()
	}
	s.frames = append(s.frames, img)
	return uint16(len(s.frames) - 1)
}

// addAll is the same for a run, and gives back the whole row of indices.
func (s *sheet) addAll(frames []image.Image) []uint16 {
	out := make([]uint16, 0, len(frames))
	for _, img := range frames {
		out = append(out, s.add(img))
	}
	return out
}

// image lays the frames out in a plain grid, which is the only layout
// TextureAtlasFixed reads: index is row-major, so the index handed out above is
// the index the game looks up.
func (s *sheet) image() image.Image {
	rows := (len(s.frames) + sheetColumns - 1) / sheetColumns
	out := image.NewRGBA(image.Rect(0, 0, sheetColumns*s.tileW, rows*s.tileH))
	for i, frame := range s.frames {
		at := image.Rect(
			(i%sheetColumns)*s.tileW, (i/sheetColumns)*s.tileH,
			(i%sheetColumns+1)*s.tileW, (i/sheetColumns+1)*s.tileH,
		)
		draw.Draw(out, at, frame, frame.Bounds().Min, draw.Src)
	}
	return out
}

// spriteKind is what gets written as sprite.json. It is the same shape the game
// reads (see sprite_kind.go); it is redeclared here rather than imported so the
// importer stays a tool that writes files rather than part of the game.
type spriteKind struct {
	Name       string                 `json:"name"`
	Tile       point                  `json:"tile"`
	Actor      bool                   `json:"actor"`
	Hostile    bool                   `json:"hostile,omitempty"`
	Attack     *spriteAttack          `json:"attack,omitempty"`
	Health     int                    `json:"health,omitempty"`
	Scale      float32                `json:"scale,omitempty"`
	Animations map[string]*spriteAnim `json:"animations"`
	Sounds     map[string]string      `json:"sounds,omitempty"`
	Provenance map[string]string      `json:"provenance,omitempty"`
}

type point struct{ X, Y int }

type spriteAttack struct {
	Damage     int     `json:"damage"`
	Range      float32 `json:"range"`
	Projectile bool    `json:"projectile,omitempty"`
	Interval   float64 `json:"interval,omitempty"`
}

type spriteAnim struct {
	Frames [][]uint16 `json:"frames"`
	FPS    float32    `json:"fps,omitempty"`
	Loop   bool       `json:"loop,omitempty"`
	// Box is the part of the tile this animation's frames actually draw on,
	// measured off the sheet - see measureBoxes.
	Box *geometry.Box `json:"box,omitempty"`
}

// dyingAnimation is the one animation measured off its last frame instead of
// all of them. A death ends with the body on the floor, and that is the shape
// it keeps: the box a corpse is stepped over and shot at by must not be the
// standing figure the animation starts with.
const dyingAnimation = "dying"

// measureBoxes gives every animation the box its own frames deserve, measured
// off the sheet that is about to be written. Storing it here is what saves the
// engine reading the card back at every spawn - and, more to the point, what
// lets a walk and a death have different boxes at all: measured at spawn there
// is only one frame to measure, and every animation got the idle's box.
//
// Each facing is measured on its own and the facings are then reduced to one
// box by geometry.BoxAcrossDirections, which takes the middle of them rather
// than the outside - a box as wide as the widest angle a thing is ever drawn
// from is a box you walk into from every other angle. A death is the one
// animation measured off its last frame instead of all of them.
func measureBoxes(animations map[string]*spriteAnim, sheet image.Image, tileW, tileH, columns int) {
	for name, anim := range animations {
		var directions [][]int
		for _, direction := range anim.Frames {
			if len(direction) == 0 {
				continue
			}
			if name == dyingAnimation {
				directions = append(directions, []int{int(direction[len(direction)-1])})
				continue
			}
			var indices []int
			for _, frame := range direction {
				indices = append(indices, int(frame))
			}
			directions = append(directions, indices)
		}
		box := geometry.BoxAcrossDirections(sheet, tileW, tileH, columns, directions)
		if !box.Empty() {
			anim.Box = &box
		}
	}
}

// writeKind puts one folder on disk: the grid, the JSON, and whatever sounds
// the kind names.
func (in *installer) writeKind(id string, kind *spriteKind, grid *sheet, cues map[string][]string) error {
	dir := filepath.Join(spritesDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	kind.Tile = point{X: grid.tileW, Y: grid.tileH}
	sheet := grid.image()
	if err := writePNG(filepath.Join(dir, "sheet.png"), sheet); err != nil {
		return err
	}
	measureBoxes(kind.Animations, sheet, grid.tileW, grid.tileH, sheetColumns)
	// And every frame on its own, beside the sheet: what the thing is picked
	// out by is the picture on the screen this instant, and the box above is
	// one shape for a whole animation seen from any angle. See
	// geometry.SpriteBoxesFile.
	if err := geometry.WriteFrameBoxes(dir, geometry.MeasureCells(sheet, grid.tileW, grid.tileH)); err != nil {
		return err
	}
	if len(cues) > 0 {
		kind.Sounds = map[string]string{}
		soundDir := filepath.Join(dir, "sounds")
		if err := os.MkdirAll(soundDir, 0o755); err != nil {
			return err
		}
		for cue, takes := range cues {
			written := 0
			for _, take := range takes {
				samples := in.sounds[take]
				if samples == nil {
					continue
				}
				// A cue with several recordings is name.wav, name~2.wav and so
				// on - the audio player picks between the takes, which is how
				// Blake Stone's three different death cries stay three.
				file := cue + ".wav"
				if written > 0 {
					file = fmt.Sprintf("%s~%d.wav", cue, written+1)
				}
				if err := os.WriteFile(filepath.Join(soundDir, file), samples, 0o644); err != nil {
					return err
				}
				written++
			}
			if written > 0 {
				kind.Sounds[cue] = cue
			}
		}
	}
	return writeJSON(filepath.Join(dir, "sprite.json"), kind)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// weaponSpec is one of Blake Stone's five weapons: the frames it draws, what
// firing it does, and the noise it makes.
type weaponSpec struct {
	id, tag, prefix string
	ready           string // the SPR_ name of the idle frame
	attack          string // the printf pattern of its firing frames
	damage          int
	melee           bool
	ammo            string
	sound           string // a name in the digitized table
	pickup          string // the SPR_ name of the weapon lying on the floor
	projectile      bool
	// tics is how long one frame of the firing animation lasts, which is what
	// sets the rate of fire: the whole animation plays and then refires, so a
	// shot takes len(frames) * tics tics - five frames here, an idle and four
	// of the attack.
	//
	// The numbers come from attackinfo in bstone's 3d_agent.cpp, which is the
	// game's own table of tics per attack frame, and from T_Attack, which is
	// what decides how much of that table a held trigger actually walks: the
	// pistol runs all four frames and starts again (6+6+6+6), while the rifle,
	// the cannon and the launcher jump back two frames from the third, so their
	// held cycle is only the middle pair. Blake Stone counts at 70 tics a
	// second and this engine at Doom's 35, so the game's figure is halved.
	tics int
}

// blakeWeapons is the whole armoury, in the order the game gives them out.
var blakeWeapons = []weaponSpec{
	// 24 tics of Wolfenstein's knife, which is what these frames are.
	{id: "bs_knife", tag: "Knife", prefix: "BSKN", ready: "KNIFEREADY", attack: "KNIFEATK%d", damage: 20, melee: true, tics: 2},
	// 6+6+6+6 = 24 of the game's tics, 12 of ours, over five frames.
	{id: "bs_pistol", tag: "Charge Pistol", prefix: "BSPS", ready: "PISTOLREADY", attack: "PISTOLATK%d", damage: 12, ammo: "BlakeCharge", sound: "ATKCHARGED", pickup: "STAT_24", tics: 2},
	// The held cycle is the middle pair, 6+5 = 11 of the game's tics.
	{id: "bs_burst_rifle", tag: "Burst Rifle", prefix: "BSBR", ready: "MACHINEGUNREADY", attack: "MACHINEGUNATK%d", damage: 14, ammo: "BlakeCharge", sound: "ATKBURSTRIFLE", pickup: "STAT_27", tics: 1},
	// 6+3 = 9, the fastest thing in the game.
	{id: "bs_ion_cannon", tag: "Ion Cannon", prefix: "BSIC", ready: "CHAINREADY", attack: "CHAINATK%d", damage: 20, ammo: "BlakeCharge", sound: "ATKIONCANNON", pickup: "STAT_28", tics: 1},
	// 6+6 = 12.
	{id: "bs_grenade_launcher", tag: "Fission Detonator", prefix: "BSGR", ready: "GRENADEREADY", attack: "GRENADEATK%d", damage: 60, ammo: "BlakeCharge", sound: "ATKGRENADE", pickup: "STAT_46", projectile: true, tics: 1},
}

// installWeapons writes a weapon folder for each. The frames are cut from a
// grid built here rather than from a file, because there is no file: the frames
// are five separate pages of VSWAP.
func (in *installer) installWeapons() (int, error) {
	written := 0
	for _, spec := range blakeWeapons {
		ready := in.frame(spec.ready)
		if ready == nil {
			continue
		}
		grid := &sheet{}
		grid.add(ready)
		grid.addAll(in.series(spec.attack))

		fireCue := ""
		var sounds map[string]io.Reader
		// A name that is not in the digitised set at all is a mistake in the
		// table above, not a weapon that happens to be quiet, and it has to say
		// so: "ATKAUTOCHARGE" was one - the auto-charge pistol's shot is an
		// AdLib effect and never was digitised - and it left the game's own
		// starting weapon firing in silence with nothing anywhere to say why.
		if spec.sound != "" && !soundIsDigitised(spec.sound) {
			return written, fmt.Errorf("%s: %q is not a digitised sound", spec.id, spec.sound)
		}
		if samples := in.sounds[spec.sound]; samples != nil {
			fireCue = "fire"
			sounds = map[string]io.Reader{"fire": bytes.NewReader(samples)}
		}
		build := wep.GridWeapon{
			Sheet:     grid.image(),
			FrameW:    grid.tileW,
			FrameH:    grid.tileH,
			Frames:    indices(len(grid.frames)),
			ID:        spec.id,
			Tag:       spec.tag,
			Prefix:    spec.prefix,
			Tics:      spec.tics,
			Scale:     3,
			Damage:    spec.damage,
			Melee:     spec.melee,
			Range:     80,
			Ammo:      spec.ammo,
			AmmoMax:   100,
			FireSound: fireCue,
			Sounds:    sounds,
			// The floor sprite, not the idle frame. A first-person view of a
			// gun held at the bottom of the screen is mostly hand, and at HUD
			// icon size that is what it reads as; the game draws a pickup with
			// its own picture and so does this. The knife has none, because
			// Blake Stone has no knife - the frames are Wolfenstein's, left in
			// the file - and it keeps the idle frame.
			Pickup:     in.pickup(spec.pickup),
			Source:     "Blake Stone: Aliens of Gold",
			SourceFile: "VSWAP.BS6",
		}
		if spec.projectile {
			// The blast has a noise of its own, and it is the one every
			// exploding thing in the game makes: A_DeathScream's gr_explosionobj
			// case plays EXPLODE1SND (3d_act2.cpp), which is what the detonator
			// turns into the moment it lands. The actor plays it out of the
			// launcher's own cue scope, which is why it is added here rather
			// than in fissionGrenade.
			actor, sprites := in.fissionGrenade()
			build.Projectile = "BlakeGrenade"
			build.Actors = map[string]*wep.Effect{"BlakeGrenade": actor}
			build.ActorSprites = sprites
			if blast := in.sounds["EXPLODE1"]; blast != nil {
				if build.Sounds == nil {
					build.Sounds = map[string]io.Reader{}
				}
				build.Sounds[grenadeBlastCue] = bytes.NewReader(blast)
			}
		}
		if err := wep.WriteGridWeapon(weaponsDir, build); err != nil {
			return written, fmt.Errorf("%s: %w", spec.id, err)
		}
		written++
	}
	return written, nil
}

func indices(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
