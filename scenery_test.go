package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// Every folder under assets/meshes/ has to load and to have the model the folder is
// there for. A piece that only half exists is a panic at spawn time otherwise,
// which is the first time anybody walks into that room.
func TestEverySceneryFolderLoadsAndHasItsMesh(t *testing.T) {
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(library) == 0 {
		t.Fatalf("no scenery in %s", meshDir)
	}
	// The imported models are a folder deep, one category each - see
	// modelCategory in tools/importss - so a loader that reads only the top level
	// finds the engine's own dozen and none of the ninety.
	if _, ok := library["furniture/ss3d_16_couch"]; !ok {
		t.Errorf("the couch is missing: scenery in a category folder is not being found")
	}
	for id, piece := range library {
		if piece.Name == "" {
			t.Errorf("%s has no name", id)
		}
		gltf := filepath.Join(meshDir, id, "mesh.gltf")
		glb := filepath.Join(meshDir, id, "mesh.glb")
		if _, err := os.Stat(gltf); err != nil {
			if _, err := os.Stat(glb); err != nil {
				t.Errorf("%s: no mesh.gltf or mesh.glb", id)
			}
		}
	}
}

// The weapon range stands a gun on a shelf by standing it at the height of a
// board, and the boards are boxes in assets/meshes/shelves/scenery.json that match
// assets/meshes/shelves/shelf.bbmodel by hand. Nothing but these numbers links the
// three, so when they drift the guns hang in the air or sink into the wood.
func TestThingsStandOnTheShelfsOwnBoards(t *testing.T) {
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	shelves, known := library[shelvesMeshName]
	if !known {
		t.Fatalf("no %s in %s", shelvesMeshName, meshDir)
	}
	for _, board := range shelfBoards {
		headroom := float32(0)
		standsOnSomething := false
		for _, box := range shelves.Colliders {
			if box[1].Y() == board {
				standsOnSomething = true
			}
			// The nearest box above this board is the ceiling of the slot.
			if gap := box[0].Y() - board; gap > 0 && (headroom == 0 || gap < headroom) {
				headroom = gap
			}
		}
		if !standsOnSomething {
			t.Errorf("nothing in the shelf tops out at %v, so things put there stand on air", board)
		}
		if headroom < pickupMaxHeight {
			t.Errorf("the slot at %v is %v tall, and a pickup may be %v", board, headroom, pickupMaxHeight)
		}
	}
}

// A shelf is one thing: one model, one name, and the boxes it collides as all
// belong to it, so the ray that hits a board reports the shelf rather than a
// nameless box. The selection then looks straight through it - furniture is
// somewhere to put things, not something to pick out.
func TestAShelfIsOneThingAndIsNotSelectable(t *testing.T) {
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	shelves, known := library[shelvesMeshName]
	if !known {
		t.Fatalf("no %s in %s", shelvesMeshName, meshDir)
	}
	rack := quads.NewGameObject(shelves.Name)
	rack.Transform().SetPosition(mgl32.Vec3{1, 1, 1})
	shapes := shelves.colliders(rack)
	if len(shapes) != len(shelves.Colliders) {
		t.Fatalf("the shelves collide as %d boxes, and there are %d", len(shapes), len(shelves.Colliders))
	}
	for i, shape := range shapes {
		if shape.GameObject() != rack {
			t.Fatalf("box %d belongs to %s rather than to the shelf it is part of",
				i, shape.GameObject().DisplayName)
		}
		// Relative to where the piece stands, so the boxes move with it.
		if got, want := shape.AABBAt(mgl32.Vec3{}), voxel.NewAABB(shelves.Colliders[i][0], shelves.Colliders[i][1]); got != want {
			t.Errorf("box %d is %v, and the shelf says %v", i, got, want)
		}
	}

	looking := func(at *quads.GameObject, furniture bool) *Game {
		game := &Game{furniture: map[*quads.GameObject]bool{}, lastHitResult: &RaycastResult{
			HitObject: at,
			HitInfo:   voxel.HitInfo3D{Hit: true, Distance: 1},
		}}
		game.furniture[at] = furniture
		return game
	}
	if object := looking(rack, true).hoveredObj(); object != nil {
		t.Errorf("the selection picked out %s, which is furniture", object.DisplayName)
	}
	// Everything that is not furniture still is.
	barrel := quads.NewGameObject("Barrel")
	if looking(barrel, false).hoveredObj() == nil {
		t.Errorf("the selection lost the barrel with the shelves")
	}
	// The editor picks out everything, furniture included.
	editing := looking(rack, true)
	editing.editor.active = true
	if editing.hoveredObj() != rack {
		t.Errorf("the editor could not select %s", rack.DisplayName)
	}
}

// ladderAt is one of the ark's own ladder segments: the piece out of its
// folder, stood in a tile with the same facing build.map gives it, which turns
// the model's own +Z (the side the rails are on, see tools/genladder's wallZ)
// to face world -Z.
func ladderAt(t *testing.T, at mgl32.Vec3) (*Game, *Scenery) {
	t.Helper()
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	ladder, known := library["ladder"]
	if !known {
		t.Fatalf("no ladder in %s", meshDir)
	}
	piece := quads.NewGameObject(ladder.Name)
	piece.Transform().SetPosition(at)
	piece.Transform().SetForward2D(mgl32.Vec3{0, 0, 1})
	game := &Game{}
	game.registerClimbable(piece, ladder, nil)
	return game, ladder
}

// Climbing is holding onto a ladder, so the volume that turns gravity off has
// to be the ladder and the room to be touching it - not the voxel it stands
// in. A box that filled the whole voxel made the voxel itself the ladder:
// jumping anywhere inside it left the player hanging in mid-air, and walking
// across it over a shaft meant they could not fall at all.
func TestTheClimbVolumeOnlyReachesAsFarAsTheLadderCanBeTouched(t *testing.T) {
	// The ark's spine: the tile is x 80..81, z 79..80, and the rails end up
	// hard against the wall at the z=79 edge of it.
	game, ladder := ladderAt(t, mgl32.Vec3{80.5, 128, 79.5})

	// Pressed against the rungs: the collider in front of them (z 79.00-79.15)
	// stops the player's own 0.2-deep box at 79.35, which has to be a climb.
	if !game.onLadder(mgl32.Vec3{80.5, 128, 79.35}) {
		t.Error("standing flush against the rungs is not a climb")
	}
	// Half a tile back, in the same voxel, touching nothing.
	if game.onLadder(mgl32.Vec3{80.5, 128, 79.9}) {
		t.Error("the far side of the ladder's own voxel still climbs - the voxel is the ladder again")
	}
	// And in mid-air over it, which is the jump that used to hang.
	if game.onLadder(mgl32.Vec3{80.5, 128.5, 79.9}) {
		t.Error("jumping in the ladder's voxel, clear of the rungs, still hangs in mid-air")
	}
	// Beside it rather than in front of it: the shaft is three tiles wide.
	if game.onLadder(mgl32.Vec3{81.5, 128, 79.35}) {
		t.Error("the tile beside the ladder climbs")
	}

	// The volume is still the model's own depth plus the player's reach, not
	// less: a box that stopped short of the collider could never be entered.
	if len(ladder.Climbable) != 1 {
		t.Fatalf("the ladder has %d climb volumes, want 1", len(ladder.Climbable))
	}
	if front := ladder.Climbable[0][1].Z(); front < 0.5 {
		t.Errorf("the climb volume stops at z=%v, short of the collider's own 0.5 - nothing can reach it", front)
	}
}

// A collider is held in the model's own space, and the rotation belongs to
// whoever tests it: the solver and the picking ray carry the object's matrix
// (WorldOBB, OBB), and the frustum test and the debug outline ask WorldAABB for
// the same thing as plain extents. Turning the stored box as well applies the
// rotation twice - on the ark's half-turned ladders that landed their solid
// half exactly where the empty half of the tile is, so the player could not
// stand in front of the rungs and the OBB view drew the box on the wrong side.
func TestAPiecesColliderIsHeldUnturnedAndTurnedOnlyWhereItIsUsed(t *testing.T) {
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	ladder := library["ladder"]
	if ladder == nil || len(ladder.Colliders) == 0 {
		t.Fatal("the ladder has no collider to place")
	}
	piece := quads.NewGameObject(ladder.Name)
	piece.Transform().SetPosition(mgl32.Vec3{80.5, 128, 79.5})
	piece.Transform().SetForward2D(mgl32.Vec3{0, 0, 1}) // build.map's facing=0,1: a half turn

	shape := ladder.colliders(piece)[0]
	authored := voxel.NewAABB(ladder.Colliders[0][0], ladder.Colliders[0][1])
	if got := shape.AABBAt(mgl32.Vec3{}); got != authored {
		t.Errorf("the stored box is %v, and scenery.json says %v - anything that carries the piece's matrix will turn it again", got, authored)
	}

	// And once turned, it is where the rungs are: hard against the wall at the
	// z=79 edge of the tile, not against its open z=80 side.
	world := shape.WorldAABB()
	if got := world.Max().Z(); got > 79.2 {
		t.Errorf("the collider reaches to z=%v, want it against the wall at ~79.15 - it is on the side the player stands in", got)
	}
	if got := world.Min().Z(); got < 78.9 {
		t.Errorf("the collider starts at z=%v, want ~79.0 - it has been turned out of its own tile", got)
	}
	if got := world.Min().Y(); got != 128 {
		t.Errorf("the collider sits at y=%v, want the piece's own 128", got)
	}
}

// WorldAABB keeps the turned box between frames, so a piece that stops turning
// is not turned again every time it is culled. It must still follow a piece that
// does turn or move - a sliding door moves, a piece placed in the editor turns.
func TestWorldAABBFollowsATurnAndAMoveAfterCaching(t *testing.T) {
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	ladder := library["ladder"]
	if ladder == nil || len(ladder.Colliders) == 0 {
		t.Fatal("the ladder has no collider to place")
	}
	piece := quads.NewGameObject(ladder.Name)
	piece.Transform().SetPosition(mgl32.Vec3{80.5, 128, 79.5})
	piece.Transform().SetForward2D(mgl32.Vec3{0, 0, 1})
	shape := ladder.colliders(piece)[0]

	fresh := func() voxel.AABB {
		transform := piece.Transform()
		return shape.AABBAt(mgl32.Vec3{}).Rotated(transform.GetRotation()).Add(transform.GetPosition())
	}
	if got, want := shape.WorldAABB(), fresh(); got != want {
		t.Fatalf("first call: WorldAABB %v, want %v", got, want)
	}
	if got, want := shape.WorldAABB(), fresh(); got != want {
		t.Fatalf("second call, nothing changed: WorldAABB %v, want %v", got, want)
	}

	piece.Transform().SetForward2D(mgl32.Vec3{1, 0, 0}) // a quarter turn
	if got, want := shape.WorldAABB(), fresh(); got != want {
		t.Errorf("after a turn: WorldAABB %v, want %v - the cached turn was reused", got, want)
	}

	piece.Transform().SetPosition(mgl32.Vec3{60.5, 125, 70.5})
	if got, want := shape.WorldAABB(), fresh(); got != want {
		t.Errorf("after a move: WorldAABB %v, want %v", got, want)
	}
}
