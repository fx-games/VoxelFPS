package main

import (
	"image/color"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// A prop dropped anywhere - snapped to a tile centre or free between two of
// them - has to come back exactly where it was put. It used to have to survive
// being taken apart into a tile index and an offset inside the tile, because
// the geometry was a grid of tiles; now the position is simply the position,
// and this is what says it stays that way.
func TestPlacedPropsKeepTheirPositions(t *testing.T) {
	game := &Game{}
	positions := []mgl32.Vec3{
		{2.5, 1, 4.5},
		{2.9, 1, 4.1},
		{6.5, 2, 3.5},
		{0.01, 1, 7.99},
	}
	for _, at := range positions {
		game.recordPlaced(nil, core.MapEntity{Name: "Table", MeshName: "table"}, at)
	}

	saved := game.mapWithPlacedEntities()
	if len(saved.Entities) != len(positions) {
		t.Fatalf("saved %d entities, placed %d", len(saved.Entities), len(positions))
	}
	for i, spawn := range saved.Entities {
		if spawn.Position != positions[i] {
			t.Errorf("a prop placed at %v was saved at %v", positions[i], spawn.Position)
		}
	}
}

// A prop the editor deleted must be gone from the file, not merely dead in the
// running level - otherwise it comes straight back on the next load.
func TestDeletedPropsAreNotSaved(t *testing.T) {
	game := &Game{
		mapSource: core.MapLoadResult{
			Entities: []core.EntitySpawn{{Entity: core.MapEntity{Name: "Stale"}}},
		},
	}

	dead := quads.NewGameObject("prop")
	dead.Kill()
	game.recordPlaced(dead, core.MapEntity{Name: "Deleted"}, mgl32.Vec3{1.5, 1, 1.5})
	game.recordPlaced(quads.NewGameObject("prop"), core.MapEntity{Name: "Kept"}, mgl32.Vec3{2.5, 1, 1.5})

	saved := game.mapWithPlacedEntities()
	if len(saved.Entities) != 1 || saved.Entities[0].Entity.Name != "Kept" {
		t.Errorf("saved %v, want only the entity that is still alive", saved.Entities)
	}
	if len(game.mapSource.Entities) != 1 {
		t.Error("saving rewrote the loaded map's own entity list; it must be left alone")
	}
}

// Snapping is floor-and-centre, which is not the same as truncation once a map
// has anything at a negative coordinate: int(-0.5) is 0 and would snap a prop
// a whole tile the wrong way.
func TestGhostPositionSnapsToTileCentre(t *testing.T) {
	free := mgl32.Vec3{3.9, 2.25, -0.4}
	if got := ghostPosition(free, false); got != free {
		t.Errorf("free placement moved the ghost to %v", got)
	}
	got := ghostPosition(free, true)
	want := mgl32.Vec3{3.5, 2.25, -0.5}
	if got != want {
		t.Errorf("snapped to %v, want %v (height untouched)", got, want)
	}
}

// The route an actor walks is the whole list the editor wrote, starting from
// where it stands, and an entity that names no route at all still gets a leg to
// walk rather than a one-point patrol that would index out of range.
func TestPatrolRouteKeepsTheWholeRoute(t *testing.T) {
	spawn := mgl32.Vec3{1, 1, 1}
	route := []mgl32.Vec3{{2, 1, 2}, {3, 1, 3}, {4, 1, 4}}

	got := patrolRoute(core.MapEntity{Patrol: route}, spawn)
	if len(got) != len(route)+1 || got[0] != spawn || got[3] != route[2] {
		t.Errorf("patrol %v, want the spawn followed by %v", got, route)
	}

	legacy := mgl32.Vec3{9, 1, 9}
	if got := patrolRoute(core.MapEntity{PatrolTo: &legacy}, spawn); len(got) != 2 || got[1] != legacy {
		t.Errorf("PatrolTo gave %v, want two points ending at %v", got, legacy)
	}

	if got := patrolRoute(core.MapEntity{}, spawn); len(got) < 2 {
		t.Errorf("an entity with no route got %v, want somewhere to walk", got)
	}
}

// What the editor writes has to survive the trip through the file, or a route
// laid out by hand comes back as an actor standing still - which is exactly what
// it looks like when a field is added to the in-memory type and not to the one
// on disk.
func TestEditorFieldsSurviveTheMapFile(t *testing.T) {
	route := []mgl32.Vec3{{2, 1, 2}, {5, 1, 7}}
	source := core.MapLoadResult{
		CeilingColor: color.RGBA{},
		FloorColor:   color.RGBA{},
		Entities: []core.EntitySpawn{
			{Position: mgl32.Vec3{1.5, 1, 1.5}, Entity: core.MapEntity{Name: "Scientist", Patrol: route}},
			{Position: mgl32.Vec3{3.8, 1, 2.2}, Entity: core.MapEntity{Name: "Table", MeshName: "table", Facing: mgl32.Vec3{0, 0, 1}}},
		},
	}

	path := filepath.Join(t.TempDir(), "round-trip.json")
	if err := saveJSONMap(path, source); err != nil {
		t.Fatalf("save: %s", err)
	}
	loaded, err := loadJSONMap(path)
	if err != nil {
		t.Fatalf("load: %s", err)
	}

	if len(loaded.Entities) != 2 {
		t.Fatalf("came back with %d entities, want 2", len(loaded.Entities))
	}
	if !slices.Equal(loaded.Entities[0].Entity.Patrol, route) {
		t.Errorf("patrol came back as %v, want %v", loaded.Entities[0].Entity.Patrol, route)
	}
	table := loaded.Entities[1]
	if table.Position != (mgl32.Vec3{3.8, 1, 2.2}) || table.Entity.Facing != (mgl32.Vec3{0, 0, 1}) {
		t.Errorf("prop came back at %v facing %v", table.Position, table.Entity.Facing)
	}
}

// Every generated test map is this one call plus a few boxes, so a room that
// comes out open at the top or walled across the middle takes all six of them
// with it. A level is only blocks now - there is no tile grid left to check it
// against, which is exactly why this is worth checking.
func TestPaintRoomIsHollowWithAFloorAndAWall(t *testing.T) {
	size := voxel.Int3{X: 6, Y: 4, Z: 5}
	blocks := newLevel(size)
	paintRoom(blocks, roomBlock(0, 2, 3))

	if got := blocks.Bounds(); got != size {
		t.Fatalf("level reports bounds %v, want the size it was built at, %v", got, size)
	}

	solid := func(x, y, z int32) bool { return blocks.IsSolidBlockAt(x, y, z) }

	for x := int32(0); x < size.X; x++ {
		for z := int32(0); z < size.Z; z++ {
			if !solid(x, 0, z) {
				t.Errorf("floor has a hole at %d,%d", x, z)
			}
			if !solid(x, size.Y-1, z) {
				t.Errorf("ceiling has a hole at %d,%d", x, z)
			}
		}
	}

	// The four walls, and nothing between them: a room you cannot stand in is
	// not a room.
	for y := int32(1); y < size.Y-1; y++ {
		for x := int32(0); x < size.X; x++ {
			for z := int32(0); z < size.Z; z++ {
				onEdge := x == 0 || z == 0 || x == size.X-1 || z == size.Z-1
				if solid(x, y, z) != onEdge {
					t.Errorf("at %d,%d,%d solid=%v, want %v", x, y, z, !onEdge, onEdge)
				}
			}
		}
	}
}

// fillBlocks is what every platform, pillar and dividing wall in the test maps
// is made of, and it takes its two corners in either order.
func TestFillBlocksIsInclusiveAndOrderIndependent(t *testing.T) {
	blocks := newLevel(voxel.Int3{X: 8, Y: 8, Z: 8})
	fillBlocks(blocks, voxel.Int3{X: 5, Y: 3, Z: 4}, voxel.Int3{X: 2, Y: 1, Z: 2}, roomBlock(0, 0, 0))

	for x := int32(0); x < 8; x++ {
		for y := int32(0); y < 8; y++ {
			for z := int32(0); z < 8; z++ {
				inBox := x >= 2 && x <= 5 && y >= 1 && y <= 3 && z >= 2 && z <= 4
				if blocks.IsSolidBlockAt(x, y, z) != inBox {
					t.Fatalf("at %d,%d,%d solid=%v, want %v", x, y, z, !inBox, inBox)
				}
			}
		}
	}
}
