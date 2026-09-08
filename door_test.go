package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

func testDoor(kind doorKind, at mgl32.Vec3) *Door {
	object := quads.NewGameObject("t")
	object.Transform().SetPosition(at)
	object.SetComponent(quads.NewPhysicsComponent(false, voxel.AABB{}))
	return &Door{object: object, kind: kind, closedPos: at}
}

// A slide door opens on interact, blocks nothing while it is moving or open,
// and closes itself again once its hold timer runs out with no further
// interaction - the "resets after about 5 seconds" half of the spec.
func TestSlideDoorOpensAndAutoCloses(t *testing.T) {
	g := &Game{}
	door := testDoor(doorSlide, mgl32.Vec3{0, 0, 0})
	g.doors = []*Door{door}

	door.Interact()
	for i := 0; i < 100 && door.openT < 1; i++ {
		g.updateDoors(0.1)
	}
	if door.openT != 1 {
		t.Fatalf("openT = %v after interact, want fully open", door.openT)
	}
	if got := door.object.Transform().GetPosition().Y(); got != doorSlideRise {
		t.Errorf("open panel Y = %v, want %v", got, doorSlideRise)
	}
	if door.object.Physics().PassThrough != true {
		t.Errorf("open door still blocks movement")
	}

	// Held open, then the timer runs out on its own.
	g.updateDoors(doorAutoCloseHold - 0.5)
	if door.target != 1 {
		t.Fatalf("door closed early, with %v left on its hold timer", door.holdTimer)
	}
	g.updateDoors(1.0)
	if door.target != 0 {
		t.Fatalf("door did not start closing once its hold timer ran out")
	}
	for i := 0; i < 100 && door.openT > 0; i++ {
		g.updateDoors(0.1)
	}
	if door.openT != 0 {
		t.Fatalf("openT = %v once fully closed, want 0", door.openT)
	}
	if door.object.Physics().PassThrough != false {
		t.Errorf("closed door still passes through")
	}
}

// Re-pressing a slide door before it closes refreshes the hold timer instead
// of letting it close underneath the player.
func TestSlideDoorReInteractRefreshesHold(t *testing.T) {
	g := &Game{}
	door := testDoor(doorSlide, mgl32.Vec3{})
	g.doors = []*Door{door}
	door.Interact()
	for i := 0; i < 100 && door.openT < 1; i++ {
		g.updateDoors(0.1)
	}

	g.updateDoors(doorAutoCloseHold - 0.5)
	door.Interact()
	g.updateDoors(1.0)
	if door.target != 1 {
		t.Fatalf("door started closing despite being re-interacted with before its hold ran out")
	}
}

// A swing door has no timer: each interact just flips which way it is
// headed, and it stays there until the next one.
func TestSwingDoorToggles(t *testing.T) {
	g := &Game{}
	door := testDoor(doorSwing, mgl32.Vec3{})
	g.doors = []*Door{door}

	door.Interact()
	for i := 0; i < 100 && door.openT < 1; i++ {
		g.updateDoors(0.1)
	}
	if door.openT != 1 {
		t.Fatalf("openT = %v after interact, want fully open", door.openT)
	}
	wantAngle := mgl32.QuatRotate(mgl32.DegToRad(doorSwingOpenAngle), mgl32.Vec3{0, 1, 0})
	if got := door.object.Transform().GetRotation(); got != wantAngle {
		t.Errorf("open leaf rotation = %v, want %v", got, wantAngle)
	}

	// Stays open - no auto-close, unlike a slide door.
	g.updateDoors(60)
	if door.target != 1 || door.openT != 1 {
		t.Fatalf("swing door closed itself without a second interact")
	}

	door.Interact()
	for i := 0; i < 100 && door.openT > 0; i++ {
		g.updateDoors(0.1)
	}
	if door.openT != 0 {
		t.Fatalf("openT = %v after a second interact, want fully closed", door.openT)
	}
}
