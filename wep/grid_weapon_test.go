package wep

import (
	"image"
	"image/color"
	"testing"
)

// A frame given the hotspot this tool writes comes to rest with its bottom edge
// on the bottom of the screen, because the ready state holds a weapon at
// psy=32. That sum - the frame's own height, its hotspot, and Doom's 32 - is
// the whole of the placement, and it is what was wrong: with no hotspot at all
// the pistol drew as a thumbnail in the top left corner.
func TestAFrameHotspotStandsTheFrameOnTheBottomOfTheScreen(t *testing.T) {
	const psy, spaceHeight = 32.0, 200.0
	for _, height := range []int{64, 192, 256} {
		_, hotspotY := FrameHotspot(64, height)
		// weapon.go: the corner goes at psy - 100 - hotspot, from the middle.
		bottom := spaceHeight/2 + (psy - spaceHeight/2 - float64(hotspotY) + float64(height))
		if bottom != spaceHeight {
			t.Errorf("a %dpx frame's bottom lands at %v, want %v", height, bottom, spaceHeight)
		}
	}
	if x, _ := FrameHotspot(192, 192); x != -64 {
		t.Errorf("a 192px frame is centred at %d, want -64", x)
	}
}

func TestMagnifiedRepeatsEveryPixel(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{1, 0, 0, 255})
	source.Set(1, 0, color.RGBA{2, 0, 0, 255})

	out := Magnified(source, 3)
	if got := out.Bounds().Size(); got != (image.Point{6, 3}) {
		t.Fatalf("3x of a 2x1 frame is %v, want 6x3", got)
	}
	for x, want := range []uint8{1, 1, 1, 2, 2, 2} {
		for y := 0; y < 3; y++ {
			if red, _, _, _ := out.At(x, y).RGBA(); uint8(red>>8) != want {
				t.Errorf("pixel %d,%d is %d, want %d", x, y, red>>8, want)
			}
		}
	}
}
