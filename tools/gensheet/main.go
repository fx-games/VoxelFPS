// Command gensheet builds a weapon folder out of a plain grid sprite sheet, for
// weapons that were never in a Doom mod.
//
//	go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
//	    -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 3
//
// The format does not care where a weapon came from, and this is the proof: what
// it writes is the same weapon.json the migration writes, only by hand.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memmaker/goray/tpack"
	"github.com/memmaker/goray/wep"
)

func main() {
	sheet := flag.String("sheet", "", "the sprite sheet to cut up")
	id := flag.String("id", "", "the weapon's folder name")
	tag := flag.String("tag", "", "the name the player sees")
	prefix := flag.String("prefix", "", "four letters to name the frames with, Doom style")
	size := flag.String("size", "64x64", "one frame's size, as WxH")
	frames := flag.String("frames", "", "the frames of the firing animation, in order, by index")
	tics := flag.Int("tics", 2, "how long each firing frame lasts")
	scale := flag.Int("scale", 1, "magnify every frame this many times, nearest neighbour")
	damage := flag.Int("damage", 5, "damage per shot")
	outDir := flag.String("out", "weapons", "the weapons folder to write into")
	flag.Parse()

	if *sheet == "" || *id == "" || *prefix == "" || *frames == "" {
		fail("need at least -sheet, -id, -prefix and -frames")
	}
	if len(*prefix) != 4 {
		fail("-prefix has to be four letters, the way Doom names sprites")
	}

	frameWidth, frameHeight := parseSize(*size)
	indices := parseFrames(*frames)

	sheetImage := readImage(*sheet)
	dir := filepath.Join(*outDir, *id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail("%v", err)
	}

	var images []tpack.NamedImage
	var names []string
	perRow := sheetImage.Bounds().Dx() / frameWidth
	for position, index := range indices {
		rect := image.Rect(
			(index%perRow)*frameWidth, (index/perRow)*frameHeight,
			(index%perRow+1)*frameWidth, (index/perRow+1)*frameHeight,
		).Add(sheetImage.Bounds().Min)
		name := fmt.Sprintf("%s%c0", strings.ToUpper(*prefix), 'A'+rune(position))
		frame := magnified(cropped(sheetImage, rect), *scale)
		hotspotX, hotspotY := frameHotspot(frame.Bounds().Dx(), frame.Bounds().Dy())
		names = append(names, name)
		images = append(images, tpack.NamedImage{
			Image: frame, Name: name, OffsetX: hotspotX, OffsetY: hotspotY,
		})
	}

	must(wep.WriteAtlas(images, filepath.Join(dir, wep.AtlasFile)))
	weapon := buildWeapon(*id, *tag, names, *tics, *damage, *sheet)
	must(wep.WriteJSON(filepath.Join(dir, wep.WeaponFile), weapon))
	fmt.Fprintf(os.Stderr, "wrote %s: %d frames\n", dir, len(names))
}

// buildWeapon writes out the smallest weapon that works: an idle frame that
// accepts the trigger, a firing animation that goes off once and comes back,
// and the raise and lower a weapon needs to be swapped in and out.
func buildWeapon(id, tag string, names []string, tics, damage int, sheet string) *wep.Weapon {
	idle := names[0]
	states := wep.StateMachine{Labels: map[string]int{}}

	states.Labels["ready"] = len(states.Steps)
	states.Steps = append(states.Steps,
		wep.Step{Sprite: idle, Tics: 1, Ops: []wep.Op{{Op: wep.OpWeaponReady, AllowFire: true}}},
		wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}},
	)

	states.Labels["fire"] = len(states.Steps)
	for position, name := range names {
		step := wep.Step{Sprite: name, Tics: tics}
		if position == 0 {
			step.Ops = []wep.Op{
				{Op: wep.OpFireBullets, Count: 1, SpreadH: 1, SpreadV: 1, Damage: damage, Puff: wep.PuffBullet},
			}
		}
		states.Steps = append(states.Steps, step)
	}
	states.Steps = append(states.Steps,
		wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpRefire, To: "fire"}}},
		wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpGoto, To: "ready"}}},
	)

	states.Labels["select"] = len(states.Steps)
	states.Steps = append(states.Steps,
		wep.Step{Sprite: idle, Tics: 1, Ops: []wep.Op{{Op: wep.OpRaise}}},
		wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}},
	)

	states.Labels["deselect"] = len(states.Steps)
	states.Steps = append(states.Steps,
		wep.Step{Sprite: idle, Tics: 1, Ops: []wep.Op{{Op: wep.OpLower}}},
		wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}},
	)

	if tag == "" {
		tag = id
	}
	return &wep.Weapon{
		Format: wep.Format,
		ID:     id,
		Tag:    tag,
		Render: wep.Render{Scale: [2]float64{1, 1}, Alpha: 1},
		States: states,
		Provenance: wep.Provenance{
			Source:     sheet,
			Actor:      id,
			SourceFile: sheet,
			Tool:       wep.Format,
		},
	}
}

// frameHotspot is the hotspot every vanilla Doom weapon frame carries, given to
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
func frameHotspot(width, height int) (x, y int) {
	const spaceWidth, weaponBottom = 320, 168
	return width/2 - spaceWidth/2, height - weaponBottom
}

// magnified blows a frame up by a whole number of pixels each way, nearest
// neighbour, which is the only enlargement that leaves pixel art as pixel art.
//
// It happens here rather than at draw time because there is nowhere at draw
// time to put it: every other weapon in the folder is authored at the size it
// is drawn, and the engine has no per-weapon magnification to add one for this
// sheet alone. A 64x64 Wolf3D tile is a fifth of the width of Doom's weapon
// space, so it is enlarged once, into the folder, and comes out an ordinary
// weapon.
func magnified(source image.Image, times int) image.Image {
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

// cropped cuts one frame out of the sheet, into an image whose own origin is
// zero, which is what the packer wants.
func cropped(source image.Image, rect image.Rectangle) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), source, rect.Min, draw.Src)
	return out
}

func readImage(path string) image.Image {
	file, err := os.Open(path)
	if err != nil {
		fail("%v", err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		fail("%s: %v", path, err)
	}
	return decoded
}

func parseSize(text string) (int, int) {
	width, height, found := strings.Cut(strings.ToLower(text), "x")
	if !found {
		fail("-size wants WxH, got %q", text)
	}
	w, err := strconv.Atoi(width)
	if err != nil {
		fail("-size: %v", err)
	}
	h, err := strconv.Atoi(height)
	if err != nil {
		fail("-size: %v", err)
	}
	return w, h
}

func parseFrames(text string) []int {
	var indices []int
	for _, field := range strings.Split(text, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			fail("-frames: %v", err)
		}
		indices = append(indices, value)
	}
	if len(indices) == 0 {
		fail("-frames named none")
	}
	if len(indices) > 26 {
		fail("-frames named %d, and Doom frame letters run out at 26", len(indices))
	}
	return indices
}

func must(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gensheet: "+format+"\n", args...)
	os.Exit(1)
}
