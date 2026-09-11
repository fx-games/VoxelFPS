package main

// Filing System Shock's art and sound where the game reads it from: the wall
// textures into the texture library, the objects and the critters into
// assets/sprites, the sixteen guns into assets/weapons.
//
// The one thing the game's own data files do not say is where a critter's
// pictures are. Everything else - what an object is called, how much it can
// take, which noise it makes - is in OBJPROP.DAT and the string resources;
// the art is laid out by position, and the layout is derived in criters.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/inkyblackness/hacked/ss1/content/audio/voc"
	"github.com/inkyblackness/hacked/ss1/content/bitmap"
	"github.com/inkyblackness/hacked/ss1/content/object"
	"github.com/inkyblackness/hacked/ss1/content/text"
	"github.com/inkyblackness/hacked/ss1/resource"
	"github.com/inkyblackness/hacked/ss1/resource/lgres"
	"github.com/inkyblackness/hacked/ss1/serial"
	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/geometry"
)

// Where each kind of thing lands, relative to the repo root.
const (
	textureDir = "assets/textures/shock"
	spritesDir = "assets/sprites"
	weaponsDir = "assets/weapons"
)

// idPrefix keeps this import's folders together in a listing, and out of the
// way of the other game's. The editor's palettes are the folders in name order.
const idPrefix = "ss_"

// Resources this importer reads by name. The numbers are the game's own; see
// inkyblackness/ss-specs, which is where they are written down.
const (
	// largeTextures is the full-size wall art: 293 of them, one resource each.
	largeTextures = resource.ID(0x03E8)
	textureCount  = 293
	// textureNames is the line of text naming each of those.
	textureNames = resource.ID(0x086A)
	// objectBitmaps is every object's art, one compound resource whose blocks
	// run object by object - see objectBlocks.
	objectBitmaps = resource.ID(0x0546)
	// objectShortNames and objectLongNames are what the game calls each object.
	objectShortNames = resource.ID(0x086D)
	objectLongNames  = resource.ID(0x0024)
	// soundEffects is the first of the digitised effects; the rest follow it.
	soundEffects = resource.ID(0x00C9)
	// handArt is the weapon-in-hand art: one resource a gun, two or four
	// blocks - the barrel at rest and the shot going off.
	handArt      = resource.ID(0x029E)
	handArtCount = 16
)

// pixelsPerVoxel is how much of System Shock's art makes one voxel of this
// world. Unlike Wolfenstein's, its sprites are not drawn into a tile of one
// fixed size - every picture is its own size, cropped to what is in it - so a
// kind's height in the world has to come off the art rather than off a single
// number, or a med patch stands as tall as a cyborg.
//
// Ninety-six is the game's own figure rather than a calibration. Every object
// bitmap in the world is drawn through g3_set_bitmap_scale at fix_make(0,
// 2048/3), which is one sixty-five-thousand-five-hundred-and-thirty-sixth times
// 682.67 - exactly 1/96 - and 96 is PHYSICS_RADIUS_UNIT, "radius units per
// square" in physunit.h. So one pixel of System Shock art is one ninety-sixth
// of a map square, and a map square is a voxel here. The humanoid mutant's 73
// pixels come out at 0.76 of a voxel, against a player 0.875 tall, which is the
// proportion the original had.
//
// It was forty, which is why everything imported was two and a half times the
// size it should be. If the whole set still wants nudging against this engine's
// player, that is what -ss-scale is for; it is a taste knob, not this number.
//
// ponytail: a per-object size does exist - PhysicsXR and PhysicsZ in the common
// properties - in units nothing in this engine shares. Read those and convert
// if anything imported ever needs to be a size the picture does not imply.
const pixelsPerVoxel = 96.0

// installer holds the files this import reads. They are opened once: an
// lgres.Reader keeps its own directory, and the same resource is asked for
// several times over.
type installer struct {
	dataDir string
	opaque  color.Palette
	keyed   color.Palette

	// taken is every folder name this run has already written. Two different
	// things in the game share a name - there is a MUTANT critter and a MUTANT
	// animating object - and without this the second one silently overwrites
	// the first's folder.
	taken map[string]bool

	art        map[string]*lgres.Reader
	properties object.PropertiesTable
	shortNames []string
	longNames  []string
	texNames   []string
}

func newInstaller(dataDir string, opaque, keyed color.Palette) (*installer, error) {
	in := &installer{
		dataDir: dataDir,
		opaque:  opaque,
		keyed:   keyed,
		taken:   map[string]bool{},
		art:     map[string]*lgres.Reader{},
	}
	for _, name := range []string{"TEXTURE.RES", "OBJART.RES", "OBJART2.RES", "OBJART3.RES", "HANDART.RES", "DIGIFX.RES", "CYBSTRNG.RES", "CITMAT.RES", "OBJ3D.RES"} {
		f, err := os.Open(filepath.Join(dataDir, name))
		if err != nil {
			return nil, err
		}
		reader, err := lgres.ReaderFrom(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		in.art[name] = reader
	}
	if err := in.readProperties(); err != nil {
		return nil, err
	}
	in.shortNames = in.textBlocks("CYBSTRNG.RES", objectShortNames)
	in.longNames = in.textBlocks("CYBSTRNG.RES", objectLongNames)
	in.texNames = in.textBlocks("CYBSTRNG.RES", textureNames)
	return in, nil
}

// readProperties decodes OBJPROP.DAT: what every object in the game is, from
// how much damage it takes to which of its pictures is the one in the world.
func (in *installer) readProperties() error {
	raw, err := os.ReadFile(filepath.Join(in.dataDir, "OBJPROP.DAT"))
	if err != nil {
		return err
	}
	in.properties = object.StandardPropertiesTable()
	decoder := serial.NewDecoder(bytes.NewReader(raw))
	in.properties.Code(decoder)
	if err := decoder.FirstError(); err != nil {
		return fmt.Errorf("OBJPROP.DAT: %w", err)
	}
	return nil
}

// textBlocks reads a compound text resource - one string a block, which is how
// the game stores every list of names it has.
func (in *installer) textBlocks(file string, id resource.ID) []string {
	reader := in.art[file]
	if reader == nil {
		return nil
	}
	view, err := reader.View(id)
	if err != nil {
		return nil
	}
	codepage := text.DefaultCodepage()
	out := make([]string, view.BlockCount())
	for i := range out {
		block, err := view.Block(i)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(block)
		if err != nil {
			continue
		}
		out[i] = strings.TrimSpace(codepage.Decode(raw))
	}
	return out
}

// blockImage decodes one block of one resource. A compound resource's frames
// are stored as differences from the frame before, so a caller walking a whole
// animation hands the previous frame back in as the reference; decoded
// standalone, every frame after the first comes out as holes.
func (in *installer) blockImage(file string, id resource.ID, block int, keyed bool, previous []byte) (*image.RGBA, []byte) {
	reader := in.art[file]
	if reader == nil {
		return nil, nil
	}
	view, err := reader.View(id)
	if err != nil || block < 0 || block >= view.BlockCount() {
		return nil, nil
	}
	source, err := view.Block(block)
	if err != nil {
		return nil, nil
	}
	bmp, err := bitmap.DecodeReferenced(source, func(w, h int16) ([]byte, error) {
		if len(previous) == int(w)*int(h) {
			out := make([]byte, len(previous))
			copy(out, previous)
			return out, nil
		}
		return make([]byte, int(w)*int(h)), nil
	})
	if err != nil {
		return nil, nil
	}
	pal := in.opaque
	if keyed {
		pal = in.keyed
	}
	return toImage(bmp, pal), bmp.Pixels
}

// resourceFrames decodes every block of one resource in order, which is one
// animation: the blocks are frames and each is a difference from the last.
func (in *installer) resourceFrames(file string, id resource.ID) []image.Image {
	reader := in.art[file]
	if reader == nil {
		return nil
	}
	view, err := reader.View(id)
	if err != nil {
		return nil
	}
	var out []image.Image
	var previous []byte
	for i := 0; i < view.BlockCount(); i++ {
		img, pixels := in.blockImage(file, id, i, true, previous)
		if img == nil {
			break
		}
		previous = pixels
		out = append(out, img)
	}
	return out
}

// installTextures writes the full-size wall art into the texture library, named
// the way the game names it so a level author can find "reactor wall" rather
// than picture 214.
func (in *installer) installTextures() (int, error) {
	if err := os.MkdirAll(textureDir, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for i := 0; i < textureCount; i++ {
		img, _ := in.blockImage("TEXTURE.RES", largeTextures.Plus(i), 0, false, nil)
		if img == nil {
			continue
		}
		name := fmt.Sprintf("T%03d", i)
		if i < len(in.texNames) && in.texNames[i] != "" {
			name = fmt.Sprintf("%03d_%s", i, slug(in.texNames[i]))
		}
		if err := writePNG(filepath.Join(textureDir, name+".png"), img); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// sheet is a kind's picture under construction: frames go in and the index each
// one lands on in the finished grid comes back, which is what the animations in
// sprite.json are written in terms of.
//
// System Shock's frames are not all the same size, so unlike the other game's
// importer this one measures the biggest and pads every frame into a tile that
// size, anchored bottom-centre - which is where a thing standing on a floor has
// its feet.
type sheet struct {
	frames []image.Image
}

func (s *sheet) add(img image.Image) uint16 {
	s.frames = append(s.frames, img)
	return uint16(len(s.frames) - 1)
}

func (s *sheet) addAll(frames []image.Image) []uint16 {
	out := make([]uint16, 0, len(frames))
	for _, img := range frames {
		out = append(out, s.add(img))
	}
	return out
}

// tile is the cell every frame is padded into: the widest and the tallest of
// them, so nothing is cropped and every frame keeps its size relative to the
// others - a critter's death frame is lower than its walk and has to stay so.
func (s *sheet) tile() (int, int) {
	w, h := 1, 1
	for _, frame := range s.frames {
		if frame.Bounds().Dx() > w {
			w = frame.Bounds().Dx()
		}
		if frame.Bounds().Dy() > h {
			h = frame.Bounds().Dy()
		}
	}
	return w, h
}

// image lays the frames out in a plain grid, which is the only layout the
// engine's fixed atlas reads: index is row-major, so the index handed out by
// add is the index the game looks up.
func (s *sheet) image(columns int) image.Image {
	tileW, tileH := s.tile()
	rows := (len(s.frames) + columns - 1) / columns
	out := image.NewRGBA(image.Rect(0, 0, columns*tileW, rows*tileH))
	for i, frame := range s.frames {
		// Bottom centre: a thing standing on a floor keeps its feet where they
		// are and grows upwards and outwards from the middle.
		width, height := frame.Bounds().Dx(), frame.Bounds().Dy()
		originX := (i%columns)*tileW + (tileW-width)/2
		originY := (i/columns)*tileH + (tileH - height)
		box := image.Rect(originX, originY, originX+width, originY+height)
		draw.Draw(out, box, frame, frame.Bounds().Min, draw.Src)
	}
	return out
}

// The shapes of sprite.json, declared here rather than imported so the importer
// stays a tool that writes files rather than part of the game.
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

// folderFor is a name turned into a folder that is not already in use. A
// clashing name keeps its object triple, which is the only thing that tells two
// things the game calls the same apart.
func (in *installer) folderFor(name string, triple object.Triple) string {
	id := idPrefix + slug(name)
	if id == idPrefix || in.taken[id] {
		id = fmt.Sprintf("%s%s_%d_%d_%d", idPrefix, slug(name), triple.Class, triple.Subclass, triple.Type)
	}
	in.taken[id] = true
	return id
}

// writeKind puts one folder on disk: the grid, the JSON, and whatever sounds
// the kind names. It reports whether there was anything to write.
//
// A kind whose sheet has no opaque pixel in it is not written at all. A quarter
// of the objects in the property table are like that - the doors, the furniture
// the game draws as a 3D model instead, the triggers and AI hints, which are
// script and were never meant to be seen - and every one of them used to become a
// folder holding a picture of nothing. In an editor that lists what can be placed
// that is not harmless: it is a cell in the browser, a slot on the placement
// wheel and an exhibit in the gallery, all for a thing that appears in the world
// as an invisible rectangle. The model-drawn ones are in assets/meshes and can be
// placed from there, which is where they belonged all along.
func (in *installer) writeKind(id string, kind *spriteKind, grid *sheet, columns int, cues map[string]int) (bool, error) {
	if len(grid.frames) == 0 {
		return false, nil
	}
	tileW, tileH := grid.tile()
	// The sheet before the folder, so a kind that turns out to be blank leaves
	// no empty directory behind.
	sheet := grid.image(columns)
	if isBlank(sheet) {
		return false, nil
	}
	dir := filepath.Join(spritesDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	kind.Tile = point{X: tileW, Y: tileH}
	kind.Scale = float32(tileH) / pixelsPerVoxel
	if err := writePNG(filepath.Join(dir, "sheet.png"), sheet); err != nil {
		return false, err
	}
	measureBoxes(kind.Animations, sheet, tileW, tileH, columns)
	// And every frame on its own, beside the sheet: what the thing is picked
	// out by is the picture on the screen this instant, and the box above is
	// one shape for a whole animation seen from any angle. See
	// geometry.SpriteBoxesFile.
	if err := geometry.WriteFrameBoxes(dir, geometry.MeasureCells(sheet, tileW, tileH)); err != nil {
		return false, err
	}
	if len(cues) > 0 {
		kind.Sounds = map[string]string{}
		soundDir := filepath.Join(dir, "sounds")
		if err := os.MkdirAll(soundDir, 0o755); err != nil {
			return false, err
		}
		for cue, index := range cues {
			samples := in.soundWAV(index)
			if samples == nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(soundDir, cue+".wav"), samples, 0o644); err != nil {
				return false, err
			}
			kind.Sounds[cue] = cue
		}
		if len(kind.Sounds) == 0 {
			kind.Sounds = nil
			_ = os.Remove(soundDir)
		}
	}
	if err := writeJSON(filepath.Join(dir, "sprite.json"), kind); err != nil {
		return false, err
	}
	return true, nil
}

// isBlank is whether an image is entirely see-through. It is the honest test for
// "there is no art here": the frames are cut out of the game's own resources with
// a colour key, so a picture the game never drew comes out fully transparent
// rather than absent.
func isBlank(picture image.Image) bool {
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := picture.At(x, y).RGBA(); alpha != 0 {
				return false
			}
		}
	}
	return true
}

// soundWAV is one digitised effect as a WAV, or nil for the index the game uses
// to mean "no sound".
func (in *installer) soundWAV(index int) []byte {
	if index < 0 || index >= 0xFF {
		return nil
	}
	reader := in.art["DIGIFX.RES"]
	view, err := reader.View(soundEffects.Plus(index))
	if err != nil {
		return nil
	}
	block, err := view.Block(0)
	if err != nil {
		return nil
	}
	sound, err := voc.Load(block)
	if err != nil || len(sound.Samples) == 0 {
		return nil
	}
	return audio.WAVFromPCM8(sound.Samples, int(sound.SampleRate))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// slug turns a name out of the game into a folder or file name.
func slug(name string) string {
	var out strings.Builder
	previousDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			previousDash = false
		case !previousDash:
			out.WriteByte('_')
			previousDash = true
		}
	}
	return strings.Trim(out.String(), "_")
}

// pngBytes is one frame encoded, for the shared picture store a weapon's
// spawned actors draw from.
func pngBytes(img image.Image) []byte {
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}
