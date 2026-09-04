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
