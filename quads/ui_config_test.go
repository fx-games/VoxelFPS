package quads

import "testing"

func TestGuiOptionFloatAppliesOnEveryStep(t *testing.T) {
	value := float32(1.0)
	applied := 0
	option := NewGuiOptionFloat("Sky Fog", &value).StepSize(0.5).ActivateFunc(func() { applied++ })

	option.More()
	if value != 1.5 || applied != 1 {
		t.Errorf("after More: value %v, applied %d times, want 1.5 and 1", value, applied)
	}

	option.Less()
	if value != 1.0 || applied != 2 {
		t.Errorf("after Less: value %v, applied %d times, want 1.0 and 2", value, applied)
	}
}
