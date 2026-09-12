package main

import (
	"image/color"
	"math"
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

// The four placement tools each have a palette of their own, and picking in one
// must not move the crosshair of another: filling a room with chairs and then
// scattering ammo through it is two jobs, and coming back to the first one should
// come back to the chair it was left on.
func TestEachPlacementToolKeepsItsOwnSlot(t *testing.T) {
	game := &Game{}
	game.editor.props[ModeProps3D] = []propKind{{label: "Torch", torch: true}, {label: "Table", scenery: "table"}}
	game.editor.props[ModeProps2D] = []propKind{{label: "Barrel", spriteKind: "props/clutter/barrel"}}
	game.editor.props[ModeWeapons] = []propKind{
		{label: "Chaingun", weapon: "bd_chaingun"},
		{label: "Chaingun ammo", weapon: "bd_chaingun", ammo: true},
	}

	game.editor.mode = ModeProps3D
	game.editor.propIndex[ModeProps3D] = 1
	game.editor.mode = ModeWeapons
	game.editor.propIndex[ModeWeapons] = 1

	game.editor.mode = ModeProps3D
	if got := game.palette()[game.paletteIndex()].label; got != "Table" {
		t.Errorf("the 3D tool came back holding %q, want the table it was left on", got)
	}
	game.editor.mode = ModeWeapons
	if got := game.palette()[game.paletteIndex()].label; got != "Chaingun ammo" {
		t.Errorf("the weapon tool came back holding %q", got)
	}
	// Only the four that place out of a palette share those keys; blocks and
	// actors have their own and must not be dragged into this.
	for mode := ModeBlocks; mode < editorModeCount; mode++ {
		if want := mode >= ModeProps3D && mode <= ModeWeapons; mode.placesProps() != want {
			t.Errorf("%s places props: %v, want %v", mode, mode.placesProps(), want)
		}
	}
}

// A gun left on the floor is the one thing the editor places that is not a mesh
// or a sheet of frames: what it is is a folder under assets/weapons, and the map
// file has to carry that name and which of the weapon's two pictures it is.
// Without both it comes back as a nameless entity with nothing to draw.
func TestAPlacedWeaponSurvivesTheMapFile(t *testing.T) {
	game := &Game{}
	game.editor.mode = ModeWeapons
	game.editor.props[ModeWeapons] = []propKind{
		{label: "Chaingun", weapon: "bd_chaingun"},
		{label: "Chaingun ammo", weapon: "bd_chaingun", ammo: true},
	}
	game.editor.propIndex[ModeWeapons] = 1

	entity := game.propEntity()
	if entity.WeaponName != "bd_chaingun" || !entity.WeaponAmmo {
		t.Fatalf("the ammo slot builds %+v, want the chaingun's ammo", entity)
	}
	game.recordPlaced(nil, entity, mgl32.Vec3{4, 1, 6})

	path := filepath.Join(t.TempDir(), "weapons.json")
	if err := saveJSONMap(path, game.mapWithPlacedEntities()); err != nil {
		t.Fatalf("save: %s", err)
	}
	loaded, err := loadJSONMap(path)
	if err != nil {
		t.Fatalf("load: %s", err)
	}
	if len(loaded.Entities) != 1 {
		t.Fatalf("came back with %d entities, want 1", len(loaded.Entities))
	}
	back := loaded.Entities[0].Entity
	if back.WeaponName != "bd_chaingun" || !back.WeaponAmmo {
		t.Errorf("the gun came back as %+v", back)
	}
}

// A ray looking down at a table, turned a quarter and stood off the origin,
// meets its lid and so counts as ground to stand a smaller piece on; the same
// ray looking straight at its side does not.
func TestLookingDownOnATableFindsItsTop(t *testing.T) {
	obb := voxel.OBB{
		AABB:      voxel.NewAABB(mgl32.Vec3{-0.5, 0, -0.5}, mgl32.Vec3{0.5, 0.8, 0.5}),
		Transform: mgl32.Translate3D(3, 1, 3).Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(90))),
	}
	fire := func(origin, direction mgl32.Vec3) mgl32.Vec3 {
		distance, hit := voxel.IntersectRayOBB(origin, direction, obb)
		if !hit {
			t.Fatalf("ray from %v missed the table", origin)
		}
		return origin.Add(direction.Mul(float32(distance)))
	}
	down := mgl32.Vec3{0.3, -1, 0.2}.Normalize()
	if top := fire(mgl32.Vec3{2.7, 3, 2.8}, down); !onLid(obb, top) || math.Abs(float64(top.Y()-1.8)) > 1e-4 {
		t.Errorf("looking down hit %v, want the lid at y 1.8", top)
	}
	if side := fire(mgl32.Vec3{0, 1.4, 3}, mgl32.Vec3{1, 0, 0}); onLid(obb, side) {
		t.Errorf("looking at the side hit %v and called it the lid", side)
	}
}

// Aiming in front picks the cell beside the eye's along the axis the view is
// mostly on, and never a cell the player stands in: looking straight down while
// flying with the body across two cells skips both of them.
func TestCellInFrontIsNextToThePlayerAndNeverInThem(t *testing.T) {
	bodyAt := func(feet mgl32.Vec3) voxel.AABB {
		return voxel.NewAABB(feet.Add(mgl32.Vec3{-0.2, 0, -0.2}), feet.Add(mgl32.Vec3{0.2, playerHeight, 0.2}))
	}
	eyeAt := func(feet mgl32.Vec3) mgl32.Vec3 { return feet.Add(mgl32.Vec3{0, playerEyeHeight, 0}) }
	standing := mgl32.Vec3{4.5, 1, 4.5}
	flying := mgl32.Vec3{4.5, 1.5, 4.5}
	cases := []struct {
		name    string
		feet    mgl32.Vec3
		forward mgl32.Vec3
		want    voxel.Int3
	}{
		{"ahead, a little down", standing, mgl32.Vec3{0.9, -0.3, 0.1}, voxel.Int3{X: 5, Y: 1, Z: 4}},
		{"behind along z", standing, mgl32.Vec3{0.2, 0.1, -0.9}, voxel.Int3{X: 4, Y: 1, Z: 3}},
		{"down, standing", standing, mgl32.Vec3{0.1, -0.95, 0}, voxel.Int3{X: 4, Y: 0, Z: 4}},
		{"down, flying across two cells", flying, mgl32.Vec3{0, -1, 0}, voxel.Int3{X: 4, Y: 0, Z: 4}},
		{"up, flying across two cells", flying, mgl32.Vec3{0, 1, 0}, voxel.Int3{X: 4, Y: 3, Z: 4}},
		{"ahead, straddling a cell edge", mgl32.Vec3{4.9, 1, 4.5}, mgl32.Vec3{1, 0, 0}, voxel.Int3{X: 6, Y: 1, Z: 4}},
	}
	for _, c := range cases {
		if got := cellInFront(eyeAt(c.feet), c.forward, bodyAt(c.feet)); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// A sign hangs on the wall the crosshair is on, facing out of it - and only on
// a wall: a floor, a ceiling and the top of a table are not where a sign goes,
// and on those the ghost falls back to standing on the ground.
func TestASignHangsOnTheWallItIsAimedAt(t *testing.T) {
	wall := func(side voxel.CubeSide, at mgl32.Vec3) RaycastResult {
		return RaycastResult{HitInfo: voxel.HitInfo3D{Hit: true, Side: side, CollisionWorldPosition: at}}
	}
	pos, facing, onWall := wall(voxel.North, mgl32.Vec3{80.5, 116, 78}).WallPosition()
	if !onWall {
		t.Fatal("the north face of a block is a wall and was not taken for one")
	}
	if want := (mgl32.Vec3{80.5, 116, 78 - signWallGap}); pos != want {
		t.Errorf("the sign hangs at %v, want %v - clear of the wall", pos, want)
	}
	if want := (mgl32.Vec3{0, 0, -1}); facing != want {
		t.Errorf("the sign looks %v, want %v - out of the wall", facing, want)
	}
	if _, _, onWall := wall(voxel.Top, mgl32.Vec3{1, 1, 1}).WallPosition(); onWall {
		t.Error("a floor was taken for a wall")
	}
	onProp := wall(voxel.North, mgl32.Vec3{1, 1, 1})
	onProp.HitObject = quads.NewGameObject("Table")
	if _, _, onWall := onProp.WallPosition(); onWall {
		t.Error("the side of a table was taken for a wall")
	}
	// And what hangs there is something to press E on, not furniture the
	// selection looks straight through - or it could never be written.
	if !interactableMesh(signMeshName) {
		t.Error("a sign is furniture, so the editor cannot reach it to write on it")
	}
}
