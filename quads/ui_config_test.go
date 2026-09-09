package quads

import (
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

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

// One menu vocabulary: WASD+E and cursor keys+Enter must do the same thing.
func TestGuiConfigAcceptsBothKeySets(t *testing.T) {
	value := 0
	opt := &GuiOptionList{name: "Res", value: &value, options: []string{"a", "b", "c"}}
	g := &GuiConfig{GuiList: &GuiList{}, options: []GuiOption{opt}}
	g.SetItems([]string{opt.String()})

	for _, pair := range [][2]glfw.Key{
		{glfw.KeyD, glfw.KeyRight},
		{glfw.KeyA, glfw.KeyLeft},
	} {
		for _, key := range pair {
			before := value
			if !g.handleKeys(0, key, 0, glfw.Press, 0) {
				t.Fatalf("key %v not consumed", key)
			}
			if value == before {
				t.Errorf("key %v changed nothing", key)
			}
		}
	}
}
