package main

import (
	"image/color"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
)

// A prop dropped anywhere - on a tile centre, between two of them, on a tile
// whose floor is raised - has to come back exactly where it was put. The map
// format has no room for a position, only a tile and an offset inside it, so
// this is the one calculation the whole of free placement rests on: it is
// reversed by toVoxelMap, which spawns from tile centre + LocalOffset.
func TestPlacedPropsRoundTripThroughTileOffsets(t *testing.T) {
	game := &Game{
		mapSource: core.MapLoadResult{
			MapDimensions: [3]int{8, 4, 8},
			MapData:       make([]core.TileInfo, 8*8),
		},
	}
	// One tile with a floor a voxel up, so the height half of the offset is
	// exercised rather than always cancelling out.
	game.mapSource.MapData[3*8+6].FloorOffset = 1

	positions := []mgl32.Vec3{
		{2.5, 1, 4.5},   // dead centre of a tile
		{2.9, 1, 4.1},   // free placement, off centre
		{6.5, 2, 3.5},   // on the raised tile
		{0.01, 1, 7.99}, // hard against two edges of the map
	}
	for _, at := range positions {
		game.recordPlaced(nil, core.MapEntity{Name: "Table", MeshName: "table"}, at)
	}

	saved := game.mapWithPlacedEntities()

	// Tile order, not placement order, is what comes back out of the file, so
	// every position has to be accounted for rather than matched up in turn.
	var respawned []mgl32.Vec3
	for index, tile := range saved.MapData {
		for _, entity := range tile.Entities {
			tileX, tileZ := index%saved.MapDimensions[0], index/saved.MapDimensions[0]
			respawned = append(respawned, tileSpawnPoint(tileX, tileZ, tile.FloorOffset).Add(entity.LocalOffset))
		}
	}
	if len(respawned) != len(positions) {
		t.Fatalf("saved %d entities, placed %d", len(respawned), len(positions))
	}
	for _, want := range positions {
		if !slices.ContainsFunc(respawned, func(got mgl32.Vec3) bool {
			return got.ApproxEqualThreshold(want, 1e-5)
		}) {
			t.Errorf("a prop placed at %v respawns somewhere else; got %v", want, respawned)
		}
	}
}

// A prop the editor deleted must be gone from the file, not merely dead in the
// running level - otherwise it comes straight back on the next load.
func TestDeletedPropsAreNotSaved(t *testing.T) {
	game := &Game{
		mapSource: core.MapLoadResult{
			MapDimensions: [3]int{4, 4, 4},
			MapData:       make([]core.TileInfo, 4*4),
		},
	}
	game.mapSource.MapData[0].Entities = []core.MapEntity{{Name: "Stale"}}

	dead := quads.NewGameObject("prop")
	dead.Kill()
	game.recordPlaced(dead, core.MapEntity{Name: "Deleted"}, mgl32.Vec3{1.5, 1, 1.5})
	game.recordPlaced(quads.NewGameObject("prop"), core.MapEntity{Name: "Kept"}, mgl32.Vec3{2.5, 1, 1.5})

	for _, tile := range game.mapWithPlacedEntities().MapData {
		for _, entity := range tile.Entities {
			if entity.Name != "Kept" {
				t.Errorf("saved %q, want only the entity that is still alive", entity.Name)
			}
		}
	}
	if game.mapSource.MapData[0].Entities == nil {
		t.Error("saving cleared the loaded map's own entity list; it must be left alone")
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

// The two fields the editor's actor and prop tools added have to survive the
// trip through the file, or a route laid out by hand comes back as an actor
// standing still - which is exactly what it looks like when a new field is
// added to the in-memory type and not to the one on disk.
func TestEditorFieldsSurviveTheMapFile(t *testing.T) {
	route := []mgl32.Vec3{{2, 1, 2}, {5, 1, 7}}
	source := core.MapLoadResult{
		MapDimensions: [3]int{4, 4, 4},
		MapData:       make([]core.TileInfo, 4*4),
		CeilingColor:  color.RGBA{},
		FloorColor:    color.RGBA{},
	}
	source.MapData[5].Entities = []core.MapEntity{
		{Name: "Scientist", Patrol: route},
		{Name: "Table", MeshName: "table", Facing: mgl32.Vec3{0, 0, 1}, LocalOffset: mgl32.Vec3{0.3, 0, -0.2}},
	}

	path := filepath.Join(t.TempDir(), "round-trip.json")
	if err := saveJSONMap(path, source); err != nil {
		t.Fatalf("save: %s", err)
	}
	loaded, err := loadJSONMap(path)
	if err != nil {
		t.Fatalf("load: %s", err)
	}

	got := loaded.MapData[5].Entities
	if len(got) != 2 {
		t.Fatalf("tile came back with %d entities, want 2", len(got))
	}
	if !slices.Equal(got[0].Patrol, route) {
		t.Errorf("patrol came back as %v, want %v", got[0].Patrol, route)
	}
	if got[1].Facing != (mgl32.Vec3{0, 0, 1}) || got[1].LocalOffset != (mgl32.Vec3{0.3, 0, -0.2}) {
		t.Errorf("prop came back facing %v at offset %v", got[1].Facing, got[1].LocalOffset)
	}
}
