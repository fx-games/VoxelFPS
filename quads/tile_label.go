package quads

import (
	"image"
	"image/color"
	"strings"
)

// The captions under the browser's cells. A grid of sixty-four pixel pictures is
// a memory game without them: a monitor and a television are the same grey box at
// that size, and two hundred System Shock models are named after the resource
// number they came out of. The name that names the thing has to be on the cell.
//
// They are drawn into the sheet, not over it. The picker uploads one directory as
// one texture and draws it as one quad, so a caption painted into that texture
// costs one blit per cell once, where a caption drawn as text would be a mesh per
// visible cell rebuilt every frame.

// TileLabelHeight is the strip under each picture the caption is written in. Two
// pixels of air, then the glyphs, then a pixel so two rows do not touch.
const TileLabelHeight = 13

// The glyph sheet the captions are written with, handed in by whoever loads the
// UI font - see GlApplication.SetupUIFont's caller. quads cannot read it itself:
// the font is baked into the binary by a package that imports this one.
var (
	labelGlyphs       *image.RGBA
	labelGlyphW       int
	labelGlyphH       int
	labelGlyphColumns int
	labelIndex        BitmapFontIndex
)

// labelAdvance is how far along the caption steps per character, as a fraction of
// a glyph's cell. The font's cells are eight wide and only the underscore draws
// into the eighth column - every letter and digit stops at the seventh - so
// stepping seven fits two more characters per cell and costs nothing.
const labelAdvanceNumer, labelAdvanceDenom = 7, 8

// labelCaptionPadding is the air between the picture and the writing under it,
// and labelGutter the air a caption leaves at each side of its own cell.
const (
	labelCaptionPadding = 2
	labelGutter         = 10
)

// SetTileLabelFont gives the browser its glyphs. Without it the captions are
// simply absent, which is what the picker did before there were any.
func SetTileLabelFont(sheet image.Image, glyphWidth, glyphHeight int, index BitmapFontIndex) {
	if sheet == nil || glyphWidth <= 0 || glyphHeight <= 0 {
		return
	}
	bounds := sheet.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			rgba.Set(x, y, sheet.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	labelGlyphs, labelGlyphW, labelGlyphH = rgba, glyphWidth, glyphHeight
	labelGlyphColumns = bounds.Dx() / glyphWidth
	labelIndex = index
}

// drawTileLabel writes one caption into the strip under a cell, centred, at the
// font's own size, cut with a dash when it does not fit.
//
// Cut rather than shrunk: these are eight pixel glyphs and half of one is four
// pixels of mush, so a name too long to fit small enough to read is worth less
// than most of that name written plainly. The whole of it is one hover away -
// the picker prints the cell under the cursor at full size.
func drawTileLabel(dst *image.RGBA, cell image.Rectangle, text string) {
	if labelGlyphs == nil || text == "" {
		return
	}
	advance := max(labelGlyphW*labelAdvanceNumer/labelAdvanceDenom, 1)
	// A gutter, not a hairline: two captions that run to the edges of adjacent
	// cells read as one long word, which is what a row of weapon names did.
	if fits := (cell.Dx() - labelGutter) / advance; len(text) > fits {
		if fits < 2 {
			return
		}
		text = text[:fits-1] + "-"
	}
	at := image.Pt(
		cell.Min.X+(cell.Dx()-len(text)*advance)/2,
		cell.Max.Y-TileLabelHeight+labelCaptionPadding,
	)
	for i, char := range []rune(text) {
		drawGlyph(dst, image.Pt(at.X+i*advance, at.Y), char)
	}
}

// drawGlyph copies one character out of the glyph sheet, drawn as flat green
// wherever the sheet is not transparent - the same green everything else in this
// editor's writing is, and one flat colour because a blended edge at eight pixels
// is a smudge.
func drawGlyph(dst *image.RGBA, at image.Point, char rune) {
	tile, known := labelIndex[char]
	if !known || labelGlyphColumns <= 0 {
		return
	}
	source := image.Pt(int(tile)%labelGlyphColumns*labelGlyphW, int(tile)/labelGlyphColumns*labelGlyphH)
	ink := color.RGBA{R: 0xC8, G: 0xF0, B: 0xC8, A: 0xFF}
	for y := 0; y < labelGlyphH; y++ {
		for x := 0; x < labelGlyphW; x++ {
			from := image.Pt(source.X+x, source.Y+y)
			if !from.In(labelGlyphs.Bounds()) {
				continue
			}
			if _, _, _, alpha := labelGlyphs.At(from.X, from.Y).RGBA(); alpha == 0 {
				continue
			}
			set(dst, at.X+x, at.Y+y, ink)
		}
	}
}

// tileLabelText is what a cell is captioned with: the name of the thing, without
// the extension a texture's file name carries. The path in front of it is the
// folder the browser is already showing, so it would be the same words on every
// cell of the page.
func tileLabelText(entry TexEntry) string {
	if entry.Name == ".." {
		return ""
	}
	if entry.IsDir {
		return entry.Name
	}
	if dot := strings.LastIndexByte(entry.Name, '.'); dot > 0 {
		return entry.Name[:dot]
	}
	return entry.Name
}
