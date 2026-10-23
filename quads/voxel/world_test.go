package voxel

import "testing"

func TestWorldFollowsMapAndRoundTrips(t *testing.T) {
	m := NewMapWithEmptyChunks(1, 1, 1, 32, 32)
	m.SetBlock(3, 4, 5, NewBlock(7))
	m.SetBlockSideFill(Int3{3, 4, 5}, Top, 9)
	if !m.IsSolidBlockAt(3, 4, 5) || m.IsSolidBlockAt(3, 5, 5) {
		t.Fatal("world out of step with SetBlock")
	}
	dir := t.TempDir()
	if err := m.SaveRegions(dir); err != nil {
		t.Fatal(err)
	}
	back := NewMapWithEmptyChunks(1, 1, 1, 32, 32)
	if err := back.LoadRegions(dir); err != nil {
		t.Fatal(err)
	}
	b := back.GetGlobalBlock(3, 4, 5)
	if !b.IsSolid || b.GetTexture(Top) != 9 || b.GetTexture(North) != 7 {
		t.Fatalf("round trip lost the block: %+v", b)
	}
	m.SetAir(Int3{3, 4, 5})
	if m.IsSolidBlockAt(3, 4, 5) {
		t.Fatal("world kept a removed block")
	}
}
