package glhf

import "testing"

// A texture that failed to load comes back as the 1x1 magenta placeholder, and it
// still gets wrapped in whatever atlas the caller asked for. Every tile lookup
// here divides by the tile count, so a placeholder used to divide by zero - a
// missing PNG taking the process down one frame after the loader let it live.
func TestFixedAtlasSurvivesATextureSmallerThanOneTile(t *testing.T) {
	placeholder := &Texture{textureWidth: 1, textureHeight: 1}
	atlas := NewTextureAtlasFixed(placeholder, 64, 64, "placeholder")

	u0, v0, u1, v1 := atlas.GetUV(7)
	if u0 != 0 || v0 != 0 || u1 != 1 || v1 != 1 {
		t.Errorf("GetUV = %v %v %v %v, want the whole texture", u0, v0, u1, v1)
	}
	if x0, y0, x1, y1 := atlas.GetUVInPx(7); x0 != 0 || y0 != 0 || x1 != 1 || y1 != 1 {
		t.Errorf("GetUVInPx = %d %d %d %d, want the whole texture", x0, y0, x1, y1)
	}
	if atlas.HasIndex(0) {
		t.Error("HasIndex(0) is true on a texture that holds no whole tile")
	}
}
