package quads

import (
	"regexp"
	"strings"
	"testing"
)

// The mesh shader used to carry its own copy of the emissive test, with the cube
// shader's defaults written out as literals - so turning the F1 sliders moved the
// walls and left every glowing model behind. The mask lives in the shared fragment
// header now. This fails if anyone writes the numbers back into a shader.
func TestNoShaderCarriesItsOwnEmissiveThresholds(t *testing.T) {
	// A smoothstep whose first two arguments are both literal numbers is the shape
	// the hardcoded copy had; the shared one passes emissiveCutoff.
	hardcoded := regexp.MustCompile(`smoothstep\(\s*[0-9.]+\s*,\s*[0-9.]+\s*,`)

	for name, source := range map[string]string{
		"cube":   chunkFragmentShaderSource,
		"mesh":   meshFragmentShaderSource,
		"sprite": quadFragmentShaderSource,
	} {
		if found := hardcoded.FindString(source); found != "" {
			t.Errorf("%s shader has a hardcoded emissive threshold (%q) - use emissiveMask", name, found)
		}
		// A shader must not redeclare the shared uniforms either: two declarations
		// of one name in a program is a GLSL error, which only shows at runtime.
		for _, uniform := range []string{"uniform float emissiveCutoff", "uniform float emissiveSaturation"} {
			if strings.Contains(source, uniform) {
				t.Errorf("%s shader redeclares %q, which the shared header already declares", name, uniform)
			}
		}
	}
}

// Both shaders that mask emissive texels have to be reachable from the F1 sliders,
// which means declaring the uniforms so SetUniformAttr can find them.
func TestShadersThatMaskEmissiveTexelsShareTheHeadersMask(t *testing.T) {
	if !strings.Contains(fragmentShaderSharedFunctions, "float emissiveMask(vec3 color)") {
		t.Fatal("the shared header no longer defines emissiveMask")
	}
	for name, source := range map[string]string{
		"cube": chunkFragmentShaderSource,
		"mesh": meshFragmentShaderSource,
	} {
		if !strings.Contains(source, "emissiveMask(") {
			t.Errorf("%s shader does not use the shared emissiveMask", name)
		}
	}
}
