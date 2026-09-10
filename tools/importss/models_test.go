package main

import "testing"

// The identifier on a textured face is the citmat resource offset itself - the
// game binds each texture with g3_set_vtext(id, MATERIAL_BASE + id) and a face
// names the same id. This once subtracted one, which slid every model's
// textures along by a slot.
//
// The oven, model 2, is the check: the game's own model_vtext_data lists 37, 15
// and 21 for it, and those are the three cells its faces have to land on.
func TestATextureIdIsItsOwnAtlasCell(t *testing.T) {
	for _, texture := range []int{37, 15, 21} {
		uv := textureUV(texture, 0, 0)
		x := int(uv[0] * atlasTexArea)
		y := int(uv[1] * atlasHeight)
		if cell := (y/atlasCell)*atlasColumns + x/atlasCell; cell != texture {
			t.Errorf("texture %d lands on cell %d", texture, cell)
		}
	}
}

// Set Colour And Shade names a slot in the object's own six-entry colour table,
// counting from one, not a palette index. Zero means the command changes
// nothing.
func TestACrossIndexIsNotAPaletteIndex(t *testing.T) {
	if _, ok := crossIndexColour(0); ok {
		t.Error("cross index zero painted something")
	}
	first, ok := crossIndexColour(1)
	if !ok || first != 0x33 {
		t.Errorf("cross index 1 = %#x, %v; want 0x33, true", first, ok)
	}
	sixth, _ := crossIndexColour(6)
	if sixth != 0x33+5*4 {
		t.Errorf("cross index 6 = %#x, want %#x", sixth, 0x33+5*4)
	}
	if _, ok := crossIndexColour(7); ok {
		t.Error("there are only six entries in the table")
	}
}

// A face mapped to the whole of its texture has corners at exactly 0 and 1, and
// both have to land inside that texture's own cell - at opposite ends of it.
// Wrapping the coordinate sent the 1 back to the 0, and the face was drawn as
// one column of pixels stretched across it.
func TestAFullFaceSpansItsWholeCell(t *testing.T) {
	const texture = 21
	near := textureUV(texture, 0, 0)
	far := textureUV(texture, 1, 1)
	if far[0] <= near[0] || far[1] <= near[1] {
		t.Fatalf("a face from 0,0 to 1,1 maps to %v..%v, which is not a span", near, far)
	}
	// Half a texel clear of both edges of the cell, and no further.
	left := float32(texture%atlasColumns) * atlasCell
	if got := near[0]*atlasTexArea - left; got != halfTexel {
		t.Errorf("near edge sits %v pixels into the cell, want %v", got, halfTexel)
	}
	if got := far[0]*atlasTexArea - left; got != atlasCell-halfTexel {
		t.Errorf("far edge sits %v pixels into the cell, want %v", got, atlasCell-halfTexel)
	}
}
