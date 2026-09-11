package quads

import (
	"image"
	"image/color"
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
