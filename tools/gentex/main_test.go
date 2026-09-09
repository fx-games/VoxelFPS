package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// flatRef is the worst reference a palette can come from: two nearly identical
// dark colours. The ramp still has to come out usable.
func flatRef() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{20, 8, 8, 255})
			} else {
				img.Set(x, y, color.RGBA{200, 40, 40, 255})
			}
		}
	}
	return img
}

func TestPaletteRampSpans(t *testing.T) {
	p := extract(flatRef())
	if luma(p.ramp[4])-luma(p.ramp[0]) < 100 {
		t.Fatalf("ramp too flat: %v .. %v", p.ramp[0], p.ramp[4])
	}
	for i := 1; i < 5; i++ {
		if luma(p.ramp[i]) <= luma(p.ramp[i-1]) {
			t.Fatalf("ramp step %d not brighter: %v", i, p.ramp)
		}
	}
	if len(p.accent) == 0 {
		t.Fatal("saturated reference produced no accent")
	}
}

// Every surface must draw something at both of the sizes the set uses, and it
// must not come out as one flat colour - a generator that silently paints over
// itself is the failure this catches.
func TestGeneratorsDrawSomething(t *testing.T) {
	p := extract(flatRef())
	for name, gen := range kinds {
		for _, size := range []int{32, 128} {
			c := newCanvas(size, seedOf(name))
			gen(c, p, 0.25)
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, px := range c.px {
				l := luma(px)
				lo = math.Min(lo, l)
				hi = math.Max(hi, l)
				if math.IsNaN(l) {
					t.Fatalf("%s at %d: NaN pixel", name, size)
				}
			}
			if hi-lo < 12 {
				t.Errorf("%s at %d: near-flat output (%.1f..%.1f)", name, size, lo, hi)
			}
		}
	}
}

func TestRecipeCoversTheSet(t *testing.T) {
	for i := 1; i <= 230; i++ {
		name := "SSTX_" + pad3(i)
		gen, _, _ := recipe(name)
		if gen == nil {
			t.Fatalf("%s has no generator", name)
		}
	}
	for _, n := range []string{"CMP1_2", "MOLECUL8", "SCR_BRK3", "LOGO_11", "FACE2_4"} {
		gen, _, phase := recipe(n)
		if gen == nil {
			t.Fatalf("%s has no generator", n)
		}
		if phase < 0 || phase >= 1 {
			t.Fatalf("%s phase out of range: %v", n, phase)
		}
	}
}

func pad3(i int) string {
	s := ""
	for _, d := range []int{100, 10, 1} {
		s += string(rune('0' + (i/d)%10))
	}
	return s
}
