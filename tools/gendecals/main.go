// Command gendecals writes the engine's own decal pictures into the shared
// picture store, assets/weapons/_sprites: GZDoom's five bullet chips, a blood
// splatter, and the sheet of glass a window is glazed with. The game packs that store into one atlas when it starts, so these
// end up in the same sheet as everything else a weapon throws at a wall, and
// they go in at the resolution they were authored - a seven pixel chip stays
// seven pixels.
//
// The blood is generated: it is a noisy blob around a centre, a handful of
// numbers. The bullet chips are not ours to invent - they are the pictures
// ZDoom itself marks a wall with, and they live in assets/base/decals. What
// this does with them is what decaldef.txt says to: BulletChip1..5 each carry
// `shade "00 00 00"` and `translucent 0.85`, and a shaded decal in ZDoom uses
// the picture's grey as *alpha* and paints the shape in the shade colour. So a
// chip is a small black splotch, and the greyscale is its coverage.
//
// Run with `go run ./tools/gendecals` from the repository root. It is
// idempotent, and it deletes assets/weapons/_sprites.png afterwards so the next run
// of the game repacks the atlas with what it just wrote.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/memmaker/voxelfps/wep"
)

const (
	weaponsDir = "assets/weapons"
	chipPath   = "assets/base/decals/chip%d.png"
	// bloodSize is the blood splatter's own canvas. The numbers below - the
	// blob's radius, how far the droplets are thrown - are pixels in it.
	bloodSize = 64
	// chipShade is decaldef.txt's `translucent 0.85` on every one of
	// BulletChip1..5.
	chipShade = 0.85
)

func main() {
	store := filepath.Join(weaponsDir, wep.SpritesDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		log.Fatal(err)
	}
	for i, name := range wep.BulletChipSprites {
		write(store, name, bulletChip(fmt.Sprintf(chipPath, i+1)))
	}
	write(store, wep.BloodSplatSprite, bloodSplatter(rand.New(rand.NewSource(2))))
	write(store, wep.WindowGlassSprite, windowGlass())

	// The packed atlas is cached against the names and sizes of what went into
	// it, and a picture replaced by one of exactly the same size would not
	// change that. Deleting it is what makes this show up in the game.
	if err := os.Remove(filepath.Join(weaponsDir, wep.SharedAtlasFile)); err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
}

// bulletChip turns one of ZDoom's chip pictures into a decal. The grey is
// alpha and the colour is flat black, which is what `shade "00 00 00"` means.
// Nothing is scaled: seven pixels across is the picture, and how big it ends
// up on a wall is the engine's business, not the atlas's.
func bulletChip(path string) *image.RGBA {
	source := loadRGBA(path)
	bounds := source.Bounds()
	chip := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			grey := source.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
			alpha := uint8(float64(grey.R) * chipShade)
			// Black at that coverage, and black premultiplied is still black.
			chip.SetRGBA(x, y, color.RGBA{0, 0, 0, alpha})
		}
	}
	return chip
}

// bloodSplatter is one wobbly central blob plus droplets thrown around it.
func bloodSplatter(rng *rand.Rand) *image.RGBA {
	// Three sine terms of different frequency give the blob an outline that never
	// repeats around the circle without needing a noise function.
	phase := [3]float64{rng.Float64() * math.Pi * 2, rng.Float64() * math.Pi * 2, rng.Float64() * math.Pi * 2}
	type droplet struct{ x, y, r float64 }
	droplets := make([]droplet, 14)
	for i := range droplets {
		angle := rng.Float64() * math.Pi * 2
		dist := 12 + rng.Float64()*16
		droplets[i] = droplet{math.Cos(angle) * dist, math.Sin(angle) * dist, 1 + rng.Float64()*3.5}
	}

	return paint(func(dx, dy float64) color.RGBA {
		angle := math.Atan2(dy, dx)
		radius := 11 + 3*math.Sin(angle*3+phase[0]) + 2*math.Sin(angle*5+phase[1]) + 1.5*math.Sin(angle*11+phase[2])
		dist := math.Hypot(dx, dy)
		inside := dist < radius
		if !inside {
			for _, d := range droplets {
				if math.Hypot(dx-d.x, dy-d.y) < d.r {
					inside = true
					break
				}
			}
		}
		if !inside {
			return color.RGBA{}
		}
		// Darker towards the middle, where the blood pools.
		t := math.Min(dist/radius, 1)
		return color.RGBA{uint8(70 + 60*t), uint8(4 + 8*t), uint8(4 + 8*t), 235}
	})
}

// glassSize is the window pane's own canvas: one voxel face, two metres, so a
// pixel is about three centimetres.
const glassSize = 64

// windowGlass is one pane: a thin steel frame round a teal-tinted sheet.
//
// The frame is what makes it glass. A flat, barely tinted sheet was faithful to
// "a window is a hole in the wall" and read in the game as nothing at all -
// an opening you walk into. Framed, a glazed band is a row of panes with a
// mullion every two metres, which is what a window on a ship looks like, and a
// frame you are standing against says "glass" where a clear sheet said "gap".
//
// Two pixels of frame, a dark edge and a lit one, so two panes side by side
// meet in a four-pixel mullion. The tint is kept low because a glass voxel is
// glazed on both of its faces: every window is looked through twice.
func windowGlass() *image.RGBA {
	const frame = 2
	tile := image.NewRGBA(image.Rect(0, 0, glassSize, glassSize))
	for y := 0; y < glassSize; y++ {
		for x := 0; x < glassSize; x++ {
			edge := min(x, y, glassSize-1-x, glassSize-1-y)
			switch {
			case edge == 0:
				tile.SetRGBA(x, y, color.RGBA{48, 58, 62, 255})
			case edge < frame:
				tile.SetRGBA(x, y, color.RGBA{120, 140, 145, 255})
			default:
				// Lighter towards the top, and a faint streak across the corner, so
				// the sheet has a surface to it rather than being a tinted void.
				t := float64(y) / glassSize
				glint := 0.0
				if d := math.Abs(float64(x+y) - 0.7*glassSize); d < 4 {
					glint = 1 - d/4
				}
				tile.SetRGBA(x, y, color.RGBA{
					uint8(70 + 30*(1-t) + 80*glint),
					uint8(165 + 25*(1-t) + 50*glint),
					uint8(160 + 25*(1-t) + 50*glint),
					uint8(60 + 25*(1-t) + 35*glint),
				})
			}
		}
	}
	return tile
}

// paint fills the blood's canvas from a function of the offset from its centre.
func paint(shade func(dx, dy float64) color.RGBA) *image.RGBA {
	tile := image.NewRGBA(image.Rect(0, 0, bloodSize, bloodSize))
	const center = bloodSize/2 - 0.5
	for y := 0; y < bloodSize; y++ {
		for x := 0; x < bloodSize; x++ {
			tile.SetRGBA(x, y, shade(float64(x)-center, float64(y)-center))
		}
	}
	return tile
}

func write(store, name string, picture image.Image) {
	out, err := os.Create(filepath.Join(store, name+".png"))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, picture); err != nil {
		log.Fatal(err)
	}
}

func loadRGBA(path string) *image.RGBA {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		log.Fatal(err)
	}
	rgba := image.NewRGBA(decoded.Bounds())
	draw.Draw(rgba, decoded.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return rgba
}
