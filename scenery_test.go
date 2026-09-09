package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
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
}
