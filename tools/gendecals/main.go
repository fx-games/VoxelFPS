// Command gendecals paints the decal tiles into assets/base/sprites_64x64.png:
// a bullet hole and a blood splatter, in the two tiles named by the constants
// below. They are generated rather than drawn because both are a noisy blob
// around a centre - a handful of numbers each - and generating them keeps the
// tile indices the engine uses and the pixels that fill them in one place.
//
// Run with `go run ./tools/gendecals` from the repository root. It rewrites the
// atlas in place and is idempotent: every tile it touches is cleared first.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
)

const (
	atlasPath = "assets/base/sprites_64x64.png"
	tileSize  = 64
	// Must match DecalBulletHole and DecalBloodSplatter in package quads.
	bulletHoleIndex = 185
	bloodIndex      = 186
)

func main() {
	atlas := loadRGBA(atlasPath)
	tilesPerRow := atlas.Bounds().Dx() / tileSize

	writeTile(atlas, tilesPerRow, bulletHoleIndex, bulletHole(rand.New(rand.NewSource(1))))
	writeTile(atlas, tilesPerRow, bloodIndex, bloodSplatter(rand.New(rand.NewSource(2))))

	out, err := os.Create(atlasPath)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	if err = png.Encode(out, atlas); err != nil {
		log.Fatal(err)
	}
}

// bulletHole is a black core inside a bright scorched rim, with dust fading out
// around it. The rim carries the shape: on a dark wall a black hole alone reads as
// nothing at all, so the ring around it is what makes the hit visible.
func bulletHole(rng *rand.Rand) *image.RGBA {
	return paint(func(dx, dy float64) color.RGBA {
		// The jitter breaks up the outline so the hole does not read as a stamped
		// circle. It stays under a pixel and a half, or the edge turns to speckles.
		r := math.Hypot(dx, dy) + rng.Float64()*1.4 - 0.7
		switch {
		case r < 11:
			return color.RGBA{6, 5, 5, 255}
		case r < 15:
			// Scorched rim, brightest right at the hole.
			t := (r - 11) / 4
			shade := uint8(150 - 70*t)
			return color.RGBA{shade, uint8(float64(shade) * 0.92), uint8(float64(shade) * 0.85), 255}
		case r < 27:
			t := 1 - (r-15)/12
			alpha := uint8(150 * t * t)
			shade := uint8(70 + 50*t)
			return premultiplyAlpha(color.RGBA{shade, shade, shade, alpha})
		}
		return color.RGBA{}
	})
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

// paint fills a tile from a function of the offset from its centre in pixels.
func paint(shade func(dx, dy float64) color.RGBA) *image.RGBA {
	tile := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))
	const center = tileSize/2 - 0.5
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			tile.SetRGBA(x, y, shade(float64(x)-center, float64(y)-center))
		}
	}
	return tile
}

// premultiplyAlpha converts a straight-alpha colour to the premultiplied form
// image/png writes, which is what image.RGBA stores.
func premultiplyAlpha(c color.RGBA) color.RGBA {
	a := uint32(c.A)
	return color.RGBA{
		uint8(uint32(c.R) * a / 255),
		uint8(uint32(c.G) * a / 255),
		uint8(uint32(c.B) * a / 255),
		c.A,
	}
}

func writeTile(atlas *image.RGBA, tilesPerRow, index int, tile image.Image) {
	origin := image.Pt((index%tilesPerRow)*tileSize, (index/tilesPerRow)*tileSize)
	target := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(tileSize, tileSize))}
	draw.Draw(atlas, target, image.Transparent, image.Point{}, draw.Src)
	draw.Draw(atlas, target, tile, image.Point{}, draw.Src)
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
