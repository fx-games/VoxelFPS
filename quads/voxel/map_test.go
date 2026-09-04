package voxel

import "testing"

// The cube shader indexes the voxel lookup buffer as gridX + gridY*mapSize.x +
// gridZ*mapSize.x*mapSize.y. It used to hardcode 64/32/64, which silently sampled
// the wrong texture on any other map. Both sides now read VoxelDimensions, so this
// pins the stride they have to share.
func TestVoxelDimensionsMatchLookupStride(t *testing.T) {
	m := NewMap(3, 2, 5, 32, 16)

	w, h, d := m.VoxelDimensions()
	if w != 96 || h != 32 || d != 160 {
		t.Fatalf("grid dimensions: got %d,%d,%d want 96,32,160", w, h, d)
	}

	if got, want := m.getBlockIndex(1, 1, 1), int32(1)+w+w*h; got != want {
		t.Fatalf("block index stride: got %d want %d", got, want)
	}
}

// The emissive flag shares a uint16 with the texture index in the voxel lookup buffer,
// and the cube shader splits them back apart with the same two constants. If the bit or
// the mask moves on one side only, faces silently sample the wrong texture - so this
// pins the encoding both sides depend on.
func TestEmissiveBitLeavesTextureIndexRecoverable(t *testing.T) {
	m := NewDefaultMap(1, 1, 1)

	const textureIndex = uint16(157)
	if got := m.withEmissiveBit(textureIndex); got != textureIndex {
		t.Fatalf("unmarked texture: got %d want %d", got, textureIndex)
	}

	if !m.ToggleEmissiveTexture(textureIndex) {
		t.Fatal("first toggle should switch the glow on")
	}

	packed := m.withEmissiveBit(textureIndex)
	if packed&EmissiveTextureBit == 0 {
		t.Errorf("emissive bit not set in %#x", packed)
	}
	if got := packed & EmissiveTextureMask; got != textureIndex {
		t.Errorf("texture index after masking: got %d want %d", got, textureIndex)
	}

	// A neighbouring texture must not be dragged along with it.
	if got := m.withEmissiveBit(textureIndex + 1); got != textureIndex+1 {
		t.Errorf("unrelated texture: got %#x want %d", got, textureIndex+1)
	}

	if m.ToggleEmissiveTexture(textureIndex) {
		t.Fatal("second toggle should switch the glow off")
	}
	if got := m.withEmissiveBit(textureIndex); got != textureIndex {
		t.Errorf("after toggling off: got %#x want %d", got, textureIndex)
	}
}

// The mask has to cover every texture index the atlas can produce, or a high index
// would collide with the flag.
func TestEmissiveMaskCoversAtlasIndices(t *testing.T) {
	if EmissiveTextureBit|EmissiveTextureMask != 0xFFFF {
		t.Errorf("bit and mask do not partition the uint16: %#x | %#x", EmissiveTextureBit, EmissiveTextureMask)
	}
	if EmissiveTextureBit&EmissiveTextureMask != 0 {
		t.Error("bit and mask overlap")
	}
}

// The flood-fill light path was fully built and never called: FillTorchlight had no
// callers, so every vertex shipped a light level of 0 and the shader threw the value
// away again. It is wired to the emissive texture set now - the panels that glow are
// the ones that light the room - so this pins what that has to mean.
func TestEmissiveFacesBakeLightIntoTheBlocksAroundThem(t *testing.T) {
	const litTexture = uint16(27)
	const plainTexture = uint16(3)

	// Solid rock with two sealed corridors carved along +X, so nothing leaks from one
	// into the other: a lit panel closes the end of the first, a plain wall the second.
	m := NewMapWithEmptyChunks(1, 1, 1, 8, 8)
	for x := int32(0); x < 8; x++ {
		for y := int32(0); y < 8; y++ {
			for z := int32(0); z < 8; z++ {
				m.SetBlock(x, y, z, NewBlock(byte(plainTexture)))
			}
		}
	}
	for x := int32(1); x < 7; x++ {
		m.SetBlock(x, 0, 1, NewAirBlock())
		m.SetBlock(x, 0, 5, NewAirBlock())
	}
	m.SetBlock(0, 0, 1, NewBlock(byte(litTexture)))

	m.SetEmissiveTextures([]uint16{litTexture})

	// The air in front of the panel gets the emitter's level, and each step away
	// costs one.
	for step := int32(0); step < 6; step++ {
		want := EmissiveTorchLight - byte(step)
		if got := m.GetGlobalBlock(1+step, 0, 1).GetTorchLight(); got != want {
			t.Errorf("light %d blocks from the panel: got %d want %d", step, got, want)
		}
	}

	// The wall the panel is painted on is solid: light lives in the air.
	if got := m.GetGlobalBlock(0, 0, 1).GetTorchLight(); got != 0 {
		t.Errorf("the panel block itself holds light %d, want 0", got)
	}

	// The second corridor is sealed and its walls are a texture nobody marked.
	if got := m.GetGlobalBlock(1, 0, 5).GetTorchLight(); got != 0 {
		t.Errorf("light reached the corridor behind the wall: got %d want 0", got)
	}

	// Dropping the texture from the set has to take its light with it, or a panel
	// switched off at runtime keeps lighting the corridor.
	m.SetEmissiveTextures(nil)
	if got := m.GetGlobalBlock(1, 0, 1).GetTorchLight(); got != 0 {
		t.Errorf("light survived the panel being unmarked: got %d want 0", got)
	}
}

// Greedy meshing folds neighbouring faces into one quad, and a quad carries a single
// light level. Merging across a light gradient therefore paints a whole run with one
// block's brightness - which is what happened while EqualForMerge only looked at
// visibility.
func TestGreedyMergeStopsAtALightLevelChange(t *testing.T) {
	lit := &VoxelFace{side: Top, lightLevel: 12}
	dimmer := &VoxelFace{side: Top, lightLevel: 11}
	alsoLit := &VoxelFace{side: Top, lightLevel: 12}

	if lit.EqualForMerge(dimmer) {
		t.Error("faces of different light level merged into one quad")
	}
	if !lit.EqualForMerge(alsoLit) {
		t.Error("faces of equal light level refused to merge")
	}
}
