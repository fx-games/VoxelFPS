package quads

import (
	"testing"

	"github.com/memmaker/voxelfps/quads/glhf"
)

// TestScreenAnimationTick exercises the frame-advance math with no GL context
// at all: SetTexture only stores pointers into a slice, so a zero-value
// *glhf.Texture works fine as a stand-in for a loaded frame.
func TestScreenAnimationTick(t *testing.T) {
	mesh := &CompoundMesh{}
	f0, f1, f2 := &glhf.Texture{}, &glhf.Texture{}, &glhf.Texture{}
	anim := NewScreenAnimation(0, 1.0, []*glhf.Texture{f0, f1, f2})

	anim.tick(mesh, 0)
	if got := mesh.textures[0]; got != f0 {
		t.Fatalf("first tick: want frame 0 immediately, got %v", got)
	}

	anim.tick(mesh, 0.5)
	if got := mesh.textures[0]; got != f0 {
		t.Fatalf("mid-frame: want still frame 0, got %v", got)
	}

	anim.tick(mesh, 0.5)
	if got := mesh.textures[0]; got != f1 {
		t.Fatalf("crossing 1s: want frame 1, got %v", got)
	}

	anim.tick(mesh, 2.5)
	if got := mesh.textures[0]; got != f0 {
		t.Fatalf("skipping several frames in one tick: want frame 0, got %v", got)
	}
}
