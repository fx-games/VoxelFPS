package quads

import (
	"math"
	"testing"
)

// HDRFromDisplay is what the clear colour goes through in Go while the fog and the sky
// go through hdrFromDisplay in GLSL. The two are written out separately because Go and
// GLSL cannot share a function, so the property that matters is checked here: lifting a
// display colour and tone mapping it again has to give back the colour that went in. If
// someone changes one curve and not the other, this is what notices.
func TestHDRFromDisplayRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		exposure, gamma float32
	}{
		{DefaultExposure, DefaultGamma},
		{1.0, 1.0},
		{2.2, 2.2},
		{0.5, 1.8},
	} {
		for _, display := range []float32{0, 0.05, 0.25, 0.5, 0.75, 0.95} {
			hdr := HDRFromDisplay(display, tc.exposure, tc.gamma)
			got := ToneMap(hdr, tc.exposure, tc.gamma)
			if math.Abs(float64(got-display)) > 1e-4 {
				t.Errorf("exposure %v gamma %v: display %v -> hdr %v -> %v, want %v",
					tc.exposure, tc.gamma, display, hdr, got, display)
			}
		}
	}
}

// 1.0 has no finite HDR value under this curve (it is the limit), so it is clamped just
// short of it rather than returning +Inf into a clear colour.
func TestHDRFromDisplayClampsAtWhite(t *testing.T) {
	hdr := HDRFromDisplay(1.0, DefaultExposure, DefaultGamma)
	if math.IsInf(float64(hdr), 0) || math.IsNaN(float64(hdr)) {
		t.Fatalf("white lifted to %v, want a finite value", hdr)
	}
	if got := ToneMap(hdr, DefaultExposure, DefaultGamma); got < 0.99 {
		t.Errorf("white round tripped to %v, want ~1.0", got)
	}
}
