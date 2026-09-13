package quads

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"
)

// SignPictureWidth and SignPictureHeight are a sign's texture: the display on
// the left three quarters and the casing on the right quarter, which is the
// split tools/genpanels gives the sign's faces (its atlasSplit). The display is
// 384x128, the sign's own 3:1, so the writing is not stretched.
const SignPictureWidth, SignPictureHeight = 512, 128

// SignPicture draws a sign's texture with its writing on the display.
//
// The writing is split into lines at "|", and a line starting with "!" is a
// warning and is written red. It is drawn at the biggest whole-pixel scale all
// of it fits at, centred: a room name is written big enough to read down a
// corridor, and a deck directory of six lines small enough to fit.
func SignPicture(text string) *image.RGBA {
	picture := image.NewRGBA(image.Rect(0, 0, SignPictureWidth, SignPictureHeight))
	display := image.Rect(0, 0, SignPictureWidth*3/4, SignPictureHeight)
	fill(picture, picture.Bounds(), color.RGBA{58, 64, 68, 255}) // casing
	fill(picture, display.Inset(3), color.RGBA{28, 33, 36, 255}) // bezel
	screen := display.Inset(5)
	fill(picture, screen, color.RGBA{8, 14, 16, 255})
	for y := screen.Min.Y; y < screen.Max.Y; y += 4 {
		fill(picture, image.Rect(screen.Min.X, y, screen.Max.X, y+1), color.RGBA{13, 21, 23, 255})
	}
	if labelGlyphs == nil || text == "" {
		return picture
	}

	lines := strings.Split(text, "|")
	longest := 1
	for _, line := range lines {
		longest = max(longest, len(strings.TrimPrefix(line, "!")))
	}
	// Margins cut to what lets four lines stand at twice the font's size: a deck
	// directory is four lines, and at the font's own size it is unreadable from
	// further off than arm's length.
	area := screen.Inset(3)
	advance := max(labelGlyphW*labelAdvanceNumer/labelAdvanceDenom, 1)
	scale := min(area.Dx()/(longest*advance), area.Dy()/(len(lines)*labelGlyphH), 6)
	scale = max(scale, 1)

	top := area.Min.Y + (area.Dy()-len(lines)*labelGlyphH*scale)/2
	for row, line := range lines {
		ink := color.RGBA{255, 184, 72, 255} // amber
		if warning, isWarning := strings.CutPrefix(line, "!"); isWarning {
			line, ink = warning, color.RGBA{255, 70, 56, 255}
		}
		left := area.Min.X + (area.Dx()-len(line)*advance*scale)/2
		for i, char := range line {
			drawGlyphScaled(picture, area, image.Pt(left+i*advance*scale, top+row*labelGlyphH*scale), char, scale, ink)
		}
	}
	return picture
}

// drawGlyphScaled is drawGlyph at a whole-pixel scale in any ink, kept inside
// clip: a line too long for the display at the smallest scale is cut at its
// edge rather than painted over the casing.
func drawGlyphScaled(dst *image.RGBA, clip image.Rectangle, at image.Point, char rune, scale int, ink color.RGBA) {
	tile, known := labelIndex[char]
	if !known || labelGlyphColumns <= 0 {
		return
	}
	source := image.Pt(int(tile)%labelGlyphColumns*labelGlyphW, int(tile)/labelGlyphColumns*labelGlyphH)
	for y := 0; y < labelGlyphH; y++ {
		for x := 0; x < labelGlyphW; x++ {
			if _, _, _, alpha := labelGlyphs.At(source.X+x, source.Y+y).RGBA(); alpha == 0 {
				continue
			}
			fill(dst, image.Rect(at.X+x*scale, at.Y+y*scale, at.X+(x+1)*scale, at.Y+(y+1)*scale).Intersect(clip), ink)
		}
	}
}

func fill(dst *image.RGBA, area image.Rectangle, c color.RGBA) {
	area = area.Intersect(dst.Bounds())
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

// GraffitiPictureWidth and GraffitiPictureHeight are graffiti's texture: the
// sprayed wall on the left three quarters, 2:1 like the panel genpanels makes
// for it, and nothing at all on the right quarter the panel's edges sample.
const GraffitiPictureWidth, GraffitiPictureHeight = 320, 120

// GraffitiPicture sprays words onto a transparent picture: what the survivors
// wrote on the walls of the ship, drawn on a panel with nothing behind the
// paint, so the wall it hangs on is the wall it was written on.
//
// Lines split at "|" like a sign's. The ink is the line's first character: "!"
// red, "~" black, "*" yellow, anything else white. Letters are the sign's font
// blown up as big as the words fit, knocked about a little each, with
// overspray round them and paint run down from them - seeded by the words, so
// the same graffiti looks the same every time the level loads.
func GraffitiPicture(text string) *image.RGBA {
	picture := image.NewRGBA(image.Rect(0, 0, GraffitiPictureWidth, GraffitiPictureHeight))
	if labelGlyphs == nil || text == "" {
		return picture
	}
	seed := uint32(2166136261)
	for _, b := range []byte(text) {
		seed = (seed ^ uint32(b)) * 16777619
	}
	random := rand.New(rand.NewSource(int64(seed)))

	wall := image.Rect(0, 0, GraffitiPictureWidth*3/4, GraffitiPictureHeight)
	lines := strings.Split(text, "|")
	inks := make([]color.RGBA, len(lines))
	longest := 1
	for i, line := range lines {
		inks[i] = color.RGBA{226, 222, 208, 255}
		switch {
		case strings.HasPrefix(line, "!"):
			inks[i] = color.RGBA{178, 28, 22, 255}
		case strings.HasPrefix(line, "~"):
			inks[i] = color.RGBA{22, 20, 20, 255}
		case strings.HasPrefix(line, "*"):
			inks[i] = color.RGBA{222, 178, 36, 255}
		}
		if strings.ContainsAny(line[:min(len(line), 1)], "!~*") {
			line = line[1:]
		}
		lines[i] = line
		longest = max(longest, len(line))
	}
	area := wall.Inset(6)
	advance := max(labelGlyphW*labelAdvanceNumer/labelAdvanceDenom, 1)
	// Fractional rather than whole pixels, unlike a sign: a sprayed letter is
	// not meant to be crisp, and a whole-pixel scale leaves a line of twenty
	// letters at the font's own size - a caption, not graffiti. The runs under
	// the last line want a little room of their own.
	scale := min(float64(area.Dx())/float64(longest*advance), float64(area.Dy()-6)/float64(len(lines)*labelGlyphH), 12)
	dot := max(int(math.Ceil(scale)), 1)
	top := float64(area.Min.Y) + (float64(area.Dy()-6)-float64(len(lines)*labelGlyphH)*scale)/2
	spray := func(x, y int, ink color.RGBA, alpha uint8) {
		if !(image.Point{x, y}).In(wall) {
			return
		}
		if picture.RGBAAt(x, y).A >= alpha {
			return
		}
		ink.A = alpha
		picture.SetRGBA(x, y, ink)
	}
	for row, line := range lines {
		ink := inks[row]
		left := float64(area.Min.X) + (float64(area.Dx())-float64(len(line)*advance)*scale)/2
		for i, char := range line {
			tile, known := labelIndex[char]
			if !known || labelGlyphColumns <= 0 {
				continue
			}
			// Each letter is its own stroke: a little high or low, a little
			// left or right of where the last one put it.
			jx, jy := (random.Float64()-0.5)*scale*0.5, (random.Float64()-0.5)*scale*0.5
			atX := left + float64(i*advance)*scale + jx
			atY := top + float64(row*labelGlyphH)*scale + jy
			source := image.Pt(int(tile)%labelGlyphColumns*labelGlyphW, int(tile)/labelGlyphColumns*labelGlyphH)
			for gy := 0; gy < labelGlyphH; gy++ {
				for gx := 0; gx < labelGlyphW; gx++ {
					if _, _, _, alpha := labelGlyphs.At(source.X+gx, source.Y+gy).RGBA(); alpha == 0 {
						continue
					}
					x0, y0 := int(atX+float64(gx)*scale), int(atY+float64(gy)*scale)
					x1, y1 := int(atX+float64(gx+1)*scale+0.999), int(atY+float64(gy+1)*scale+0.999)
					for py := y0; py < y1; py++ {
						for px := x0; px < x1; px++ {
							spray(px, py, ink, 255)
						}
					}
					// Overspray: a few faint dots round every block of paint.
					for range dot {
						spray(x0+random.Intn(dot*2+1)-dot/2, y0+random.Intn(dot*2+1)-dot/2, ink, uint8(60+random.Intn(90)))
					}
					// And now and then the paint ran.
					if random.Intn(14) == 0 {
						x := x0 + random.Intn(dot)
						run := dot + random.Intn(dot*3)
						for d := 0; d < run; d++ {
							spray(x, y1+d, ink, uint8(255-d*180/run))
						}
					}
				}
			}
		}
	}
	return picture
}
