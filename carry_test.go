package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// An item in hand goes on the first solid thing inside a block of the player,
// and hangs a block in front of them when there is nothing that near.
func TestTheItemInHandIsAimedAtWhatIsInFrontOfIt(t *testing.T) {
	origin := mgl32.Vec3{0, 1, 0}
	forward := mgl32.Vec3{1, 0, 0}

	hitAt := func(x float32) RaycastResult {
		return RaycastResult{HitInfo: voxel.HitInfo3D{
			Hit:                    true,
			CollisionWorldPosition: mgl32.Vec3{x, 1, 0},
		}}
	}

	wall := mgl32.Vec3{0.4, 1, 0}
	if got := carryPosition(origin, forward, hitAt(0.4)); got != wall {
		t.Errorf("a wall within reach: got %v, want the wall at %v", got, wall)
	}

	arm := origin.Add(forward.Mul(carryReach))
	if got := carryPosition(origin, forward, hitAt(6)); got != arm {
		t.Errorf("a wall across the room: got %v, want arm's length at %v", got, arm)
	}
	if got := carryPosition(origin, forward, RaycastResult{}); got != arm {
		t.Errorf("nothing hit at all: got %v, want arm's length at %v", got, arm)
	}
}

// What may be carried is an item. A door is part of the level, and moving one
// would leave the level with a hole where the door used to be.
func TestOnlyItemsAreCarryable(t *testing.T) {
	game := &Game{}
	item := quads.NewGameObject("Note")
	item.SetComponent(quads.NewPhysicsComponent(false, pickupSlab))
	if !game.carryable(item) {
		t.Errorf("an item with a collider should be carryable")
	}

	door := quads.NewGameObject("Door")
	door.SetComponent(quads.NewPhysicsComponent(false, pickupSlab))
	game.doors = append(game.doors, &Door{object: door})
	if game.carryable(door) {
		t.Errorf("a door is part of the level, not an item")
	}
	if game.carryable(quads.NewGameObject("No collider")) {
		t.Errorf("a thing with no collider has nothing to put back on the grid")
	}
}
