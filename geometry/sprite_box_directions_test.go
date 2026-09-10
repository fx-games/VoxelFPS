package geometry

import (
	"image"
	"image/color"
	"testing"
)

// sheetOfWidths draws a one-row grid sheet where cell i has an opaque block of
// widths[i] pixels, centred, running the full height. That is the shape the two
// DOS importers write: every frame padded into the same cell, bottom centred.
func sheetOfWidths(tile int, widths []int) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, tile*len(widths), tile))
	for i, width := range widths {
		left := i*tile + (tile-width)/2
		for y := 0; y < tile; y++ {
			for x := left; x < left+width; x++ {
				out.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return out
}

// TestTheBoxAcrossFacingsIsTheMiddleOneNotTheWidest is the whole point: a thing
// drawn wide from the side and narrow face on is bumped into at its middle
// width, not at its widest.
func TestTheBoxAcrossFacingsIsTheMiddleOneNotTheWidest(t *testing.T) {
	const tile = 40
	widths := []int{10, 20, 30, 40}
	sheet := sheetOfWidths(tile, widths)

	directions := [][]int{{0}, {1}, {2}, {3}}
	box := BoxAcrossDirections(sheet, tile, tile, len(widths), directions)

	// The four facings are 0.25, 0.5, 0.75 and 1.0 of the tile wide; the middle
	// two average to 0.625.
	if got := box.Max.X() - box.Min.X(); got < 0.62 || got > 0.63 {
		t.Fatalf("box width %v, want the middle of the facings (0.625)", got)
	}
	union := UnionOfCells(sheet, tile, tile, len(widths), []int{0, 1, 2, 3})
	if got := union.Max.X() - union.Min.X(); got < 0.99 {
		t.Fatalf("the union should still be the whole tile, got %v", got)
	}
}

// TestOneFacingIsStillTheUnionOfItsFrames: an explosion is drawn from one angle
// and grows, and the box it gets must still cover the whole blast.
func TestOneFacingIsStillTheUnionOfItsFrames(t *testing.T) {
	const tile = 40
	widths := []int{10, 40}
	sheet := sheetOfWidths(tile, widths)

	box := BoxAcrossDirections(sheet, tile, tile, len(widths), [][]int{{0, 1}})

	if got := box.Max.X() - box.Min.X(); got < 0.99 {
		t.Fatalf("box width %v, want the whole tile - one facing is its own union", got)
	}
}
