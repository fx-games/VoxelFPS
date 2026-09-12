package main

import (
	"math"
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

// A slide door laid flat - a floor hatch, turned there by a facing that points
// down - slides sideways instead of up. Same door, same rise; the direction is
// the leaf's own up, which its rotation has turned into a horizontal one.
func TestFlatSlideDoorSlidesSideways(t *testing.T) {
	g := &Game{}
	door := testDoor(doorSlide, mgl32.Vec3{5, 10, 5})
	door.object.Transform().SetForward(mgl32.Vec3{0, -1, 0})
	g.doors = []*Door{door}

	door.Interact()
	for i := 0; i < 100 && door.openT < 1; i++ {
		g.updateDoors(0.1)
	}
	open := door.object.Transform().GetPosition()
	if got := open.Y(); math.Abs(float64(got-10)) > 1e-4 {
		t.Errorf("flat hatch rose to Y = %v, want it to stay at 10", got)
	}
	if moved := open.Sub(mgl32.Vec3{5, 10, 5}).Len(); math.Abs(float64(moved-doorSlideRise)) > 1e-4 {
		t.Errorf("flat hatch moved %v, want %v sideways", moved, doorSlideRise)
	}
}

// A floor hatch has to cover the hole it is a hatch in. The model stands on its
// own origin - a metre up from it - so laid flat that metre runs along the deck,
// and the leaf placed at the voxel's own centre covered half the hole and half
// the plate beside it, a voxel below the deck. That is the hatch that looked
// half open and could be stood in like a pit.
func TestFlatHatchCoversTheHoleItIsIn(t *testing.T) {
	object := quads.NewGameObject("hatch")
	object.Transform().SetForward(mgl32.Vec3{0, -1, 0}) // face down: a floor hatch

	// The ark's upper spine bulkhead: the hole is the voxel at 80,130,80.
	placed := slideDoorPlacement(object, mgl32.Vec3{80.5, 130, 80.5})

	up := object.Transform().GetRotation().Rotate(mgl32.Vec3{0, 1, 0})
	// The leaf runs one metre along its own up from where it is placed, and is
	// half a metre either side of that line across.
	from, to := placed, placed.Add(up)
	lowZ, highZ := min(from.Z(), to.Z()), max(from.Z(), to.Z())
	if mgl32.Abs(lowZ-80) > 0.001 || mgl32.Abs(highZ-81) > 0.001 {
		t.Errorf("leaf spans z %v..%v, want the hole's own 80..81", lowZ, highZ)
	}
	if mgl32.Abs(placed.X()-80.5) > 0.001 {
		t.Errorf("leaf is centred at x=%v, want the middle of the hole at 80.5", placed.X())
	}
	// Shut, it is the deck you walk on rather than a step down into the hole.
	if mgl32.Abs(placed.Y()-131) > 0.001 {
		t.Errorf("leaf lies at y=%v, want the deck's own surface at 131", placed.Y())
	}

	// An upright door is already where its model wants to be.
	upright := quads.NewGameObject("door")
	upright.Transform().SetForward2D(mgl32.Vec3{0, 0, -1})
	if got := slideDoorPlacement(upright, mgl32.Vec3{57.5, 116, 80.5}); got != (mgl32.Vec3{57.5, 116, 80.5}) {
		t.Errorf("upright door was moved to %v", got)
	}
}
