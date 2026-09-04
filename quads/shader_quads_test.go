package quads

import "testing"

// The instance buffer is one flat float array. If the declared layout and the
// packing in appendInstance ever disagree, every sprite reads its neighbour's
// attributes and the screen fills with garbage - with no error anywhere. These
// two facts are what keep them in step.
func TestInstanceLayoutMatchesThePackedStride(t *testing.T) {
	floats := 0
	for _, attr := range QuadInstanceFormat {
		floats += attr.Floats
	}
	if floats != QuadInstanceFloats {
		t.Errorf("QuadInstanceFormat packs %d floats, QuadInstanceFloats says %d", floats, QuadInstanceFloats)
	}
}

// A mat4 does not fit one attribute slot, it takes four consecutive ones. An
// attribute placed in the middle of the matrix would be overwritten by it.
func TestInstanceLocationsDoNotOverlap(t *testing.T) {
	taken := map[uint32]bool{0: true} // location 0 is the per-vertex cornerIndex
	for _, attr := range QuadInstanceFormat {
		slots := uint32(1)
		if attr.Floats == 16 {
			slots = 4
		}
		for slot := uint32(0); slot < slots; slot++ {
			location := attr.Location + slot
			if taken[location] {
				t.Errorf("attribute location %d is claimed twice", location)
			}
			taken[location] = true
		}
	}
}
