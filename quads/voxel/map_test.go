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
	if EmissiveTextureBit|TorchLightMask|EmissiveTextureMask != 0xFFFF {
		t.Errorf("the three fields do not partition the uint16: %#x | %#x | %#x",
			EmissiveTextureBit, TorchLightMask, EmissiveTextureMask)
	}
	for _, pair := range [][2]uint16{
		{EmissiveTextureBit, TorchLightMask},
		{EmissiveTextureBit, EmissiveTextureMask},
		{TorchLightMask, EmissiveTextureMask},
	} {
		if pair[0]&pair[1] != 0 {
			t.Errorf("fields overlap: %#x & %#x", pair[0], pair[1])
		}
	}
	// Four bits of light, and they have to sit where the shift says they do.
	if TorchLightMask>>TorchLightShift != 0xF {
		t.Errorf("light field is %#x wide after shifting, want 0xF", TorchLightMask>>TorchLightShift)
	}
	// Blocks carry a byte texture id, so the index field has to hold at least 255.
	if EmissiveTextureMask < 255 {
		t.Errorf("index field holds %d, too small for a byte texture id", EmissiveTextureMask)
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

	// DrawWithShader triggers this before it packs the lookup buffer; here there is
	// no GL context, so drive the bake itself.
	m.SetEmissiveTextures([]uint16{litTexture})
	m.bakeEmissiveTorchlight()

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
	m.bakeEmissiveTorchlight()
	if got := m.GetGlobalBlock(1, 0, 1).GetTorchLight(); got != 0 {
		t.Errorf("light survived the panel being unmarked: got %d want 0", got)
	}
}

// The lookup entry is the one thing the CPU and the cube shader both take apart, and
// it now carries three fields instead of two. Baked light moved in here from the
// vertex bits so that greedy meshing could go back to merging across a light gradient
// - which only works if all three survive the packing.
func TestFaceLookupPacksLightEmissiveAndTexture(t *testing.T) {
	const litTexture = uint16(27)

	// Solid rock with one corridor carved along +X, so the panel's other five faces
	// look into stone and only the +X one has lit air in front of it.
	m := NewMapWithEmptyChunks(1, 1, 1, 8, 8)
	for x := int32(0); x < 8; x++ {
		for y := int32(0); y < 8; y++ {
			for z := int32(0); z < 8; z++ {
				m.SetBlock(x, y, z, NewBlock(3))
			}
		}
	}
	for x := int32(1); x < 7; x++ {
		m.SetBlock(x, 0, 0, NewAirBlock())
	}
	m.SetBlock(0, 0, 0, NewBlock(byte(litTexture)))
	m.SetEmissiveTextures([]uint16{litTexture})
	m.bakeEmissiveTorchlight()

	// Face 0 is +X: the panel's lit side, facing the air the light is in.
	entry := m.packFaceLookup(litTexture, Int3{0, 0, 0}, 0)

	if got := entry & EmissiveTextureMask; got != litTexture {
		t.Errorf("texture index: got %d want %d", got, litTexture)
	}
	if entry&EmissiveTextureBit == 0 {
		t.Errorf("emissive bit not set in %#x", entry)
	}
	if got := byte((entry & TorchLightMask) >> TorchLightShift); got != EmissiveTorchLight {
		t.Errorf("baked light: got %d want %d", got, EmissiveTorchLight)
	}

	// The other five faces of the same block look into rock, so they are unlit even
	// though the texture is the same - which is the whole point of packing per face.
	for face := 1; face < 6; face++ {
		unlit := m.packFaceLookup(litTexture, Int3{0, 0, 0}, face)
		if got := (unlit & TorchLightMask) >> TorchLightShift; got != 0 {
			t.Errorf("face %d looks into rock but carries light %d", face, got)
		}
	}
}

// A torch is placed in the running editor, and every re-bake starts from dark. So
// the emitter has to be map state that the bake re-seeds - the first version called
// FillTorchlight once at placement, and the torch went out the next time a block was
// placed anywhere in the map.
func TestPlacedTorchesSurviveARebake(t *testing.T) {
	m := NewMapWithEmptyChunks(1, 1, 1, 8, 8)
	for x := int32(0); x < 8; x++ {
		for y := int32(0); y < 8; y++ {
			for z := int32(0); z < 8; z++ {
				m.SetBlock(x, y, z, NewBlock(3))
			}
		}
	}
	// One corridor along +X for the light to travel down.
	for x := int32(0); x < 7; x++ {
		m.SetBlock(x, 0, 0, NewAirBlock())
	}

	torch := Int3{0, 0, 0}
	m.AddTorchLight(torch)
	m.bakeEmissiveTorchlight()

	if got := m.GetGlobalBlock(0, 0, 0).GetTorchLight(); got != MaxTorchLight {
		t.Errorf("light at the torch: got %d want %d", got, MaxTorchLight)
	}
	if got := m.GetGlobalBlock(3, 0, 0).GetTorchLight(); got != MaxTorchLight-3 {
		t.Errorf("light three blocks away: got %d want %d", got, MaxTorchLight-3)
	}

	// A second bake is what a block placed elsewhere costs, and the torch has to
	// still be there afterwards.
	m.bakeEmissiveTorchlight()
	if got := m.GetGlobalBlock(3, 0, 0).GetTorchLight(); got != MaxTorchLight-3 {
		t.Errorf("light after a re-bake: got %d want %d", got, MaxTorchLight-3)
	}

	m.RemoveTorchLight(torch)
	m.bakeEmissiveTorchlight()
	if got := m.GetGlobalBlock(3, 0, 0).GetTorchLight(); got != 0 {
		t.Errorf("light survived the torch being removed: got %d want 0", got)
	}
}
