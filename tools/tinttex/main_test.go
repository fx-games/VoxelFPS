package main

import (
	"image"
	"image/color"
	"testing"
)

func TestTint(t *testing.T) {
	lut := buildLUT(ramps["teal"], 1.0)

	m := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	m.SetNRGBA(0, 0, color.NRGBA{128, 128, 128, 255}) // gray: takes the tint
	m.SetNRGBA(1, 0, color.NRGBA{255, 0, 0, 255})     // saturated: kept as painted
	m.SetNRGBA(2, 0, color.NRGBA{200, 200, 200, 0})   // transparent: alpha untouched
	tint(m, lut, 1.0, 1.6)

	if got := m.NRGBAAt(0, 0); got.B <= got.R || got.G <= got.R {
		t.Errorf("gray did not turn teal: %v", got)
	}
	if got := m.NRGBAAt(1, 0); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("saturated red was recolored: %v", got)
	}
	if got := m.NRGBAAt(2, 0); got.A != 0 {
		t.Errorf("alpha changed: %v", got)
	}
}

func TestPickFamilyIsStable(t *testing.T) {
	fams := []string{"rust", "teal", "blue"}
	first := pickFamily(fams, "BLOCK_1A.PNG")
	for i := 0; i < 10; i++ {
		if got := pickFamily(fams, "BLOCK_1A.PNG"); got != first {
			t.Fatalf("family not stable: %s then %s", first, got)
		}
	}
	if pickFamily([]string{"rust"}, "anything") != "rust" {
		t.Error("single family must always win")
	}
}
