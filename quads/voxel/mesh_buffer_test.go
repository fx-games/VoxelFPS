package voxel

import "testing"

// A chunk is 32 on every axis, so the far corner of a face in it is 32 on every
// axis too, and has to come back out of the vertex it is packed into. It did not
// on y: five bits wrapped 32 to 0, and every face touching a chunk's top layer
// was drawn folded - invisible on any level taller than one chunk.
func TestAChunkCornerSurvivesPacking(t *testing.T) {
	packed := NewMeshBuffer().Compress(Int3{X: 32, Y: 32, Z: 32}, North)
	// decompressVertex in shader_cubes.go, bit for bit.
	x, y, z, normal := packed&0x3F, packed>>6&0x3F, packed>>12&0x3F, packed>>18&0x7
	if x != 32 || y != 32 || z != 32 || normal != uint32(North) {
		t.Errorf("packed %d,%d,%d facing %d, unpacked %d,%d,%d facing %d", 32, 32, 32, North, x, y, z, normal)
	}
}
