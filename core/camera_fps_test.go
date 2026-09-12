package core

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// Pointing the camera must not move it. The camera hangs off the player, so its
// own position is where it sits relative to them - the eye height - while
// GetPosition answers where that ends up in the world. Anything that reads the
// one and writes the other moves the camera by the whole of the player's own
// position, which is what -pitch used to do to a capture: the player stood in
// the hall and the picture came out from above the roof.
func TestPointingTheCameraDoesNotMoveIt(t *testing.T) {
	player := NewDefaultTransform()
	player.SetPosition(mgl32.Vec3{15.5, 1, 21.5})

	camera := NewFPSCamera(mgl32.Vec3{}, 640, 480, 0.1)
	camera.SetPosition(mgl32.Vec3{0, 0.9, 0}) // the eye, relative to the player
	camera.SetParent(player)

	before := camera.GetPosition()
	want := mgl32.Vec3{15.5, 1.9, 21.5}
	if before.Sub(want).Len() > 0.001 {
		t.Fatalf("the eye is at %v before anything happens, want %v", before, want)
	}

	yaw, _ := camera.GetRotation()
	camera.SetAngles(yaw, -12)

	if after := camera.GetPosition(); after.Sub(before).Len() > 0.001 {
		t.Errorf("looking down 12 degrees moved the eye from %v to %v", before, after)
	}
	if _, pitch := camera.GetRotation(); pitch != -12 {
		t.Errorf("pitch = %v, want -12", pitch)
	}
}

// Rolling tilts what the visor calls up and leaves everything gravity owns
// alone: the direction being looked at does not move, and neither does the
// direction the feet would walk in. See FPSCamera.SetRoll and eva.go.
func TestRollTiltsTheVisorAndNotTheWalk(t *testing.T) {
	camera := NewFPSCamera(mgl32.Vec3{}, 640, 480, 0.1)
	camera.SetForward2D(mgl32.Vec3{0, 0, -1})

	front, walk, right := camera.GetForward(), camera.FpsWalkDirection, camera.cameraRight

	camera.SetRoll(90)
	if moved := camera.GetForward().Sub(front).Len(); moved > 0.001 {
		t.Errorf("roll moved the look direction by %v", moved)
	}
	if moved := camera.FpsWalkDirection.Sub(walk).Len(); moved > 0.001 {
		t.Errorf("roll leaned the walk direction by %v", moved)
	}
	// A quarter turn puts up where right was, give or take the sign of the
	// turn, and up must stay perpendicular to the way the visor points.
	if up := camera.GetUp(); mgl32.Abs(up.Dot(right)) < 0.999 {
		t.Errorf("up = %v after a quarter roll, want it lying along the old right %v", up, right)
	}
	if up := camera.GetUp(); mgl32.Abs(up.Dot(front)) > 0.001 {
		t.Errorf("up = %v is not square to the look direction", up)
	}

	// Rolling all the way round comes home to level, the short way.
	camera.SetRoll(190)
	if got := camera.Roll(); got != -170 {
		t.Errorf("roll of 190 kept as %v, want -170 so levelling takes the near side", got)
	}
}

// Free look is the suit's: the mouse turns the visor about its own axes, so
// there is no up to be stuck at. Eighteen ten-degree steps upwards from level
// come out looking backwards and upside down - which the walking camera, with
// its clamp short of vertical, can never do.
func TestFreeLookHasNoUpToBeStuckAt(t *testing.T) {
	camera := NewFPSCamera(mgl32.Vec3{}, 640, 480, 1)
	camera.SetForward2D(mgl32.Vec3{0, 0, -1})

	camera.SetFreeLook(true)
	for i := 0; i < 18; i++ {
		camera.ChangeAngles(0, -10) // ten degrees up, at a sensitivity of one
	}
	if front := camera.GetForward(); front.Z() < 0.99 {
		t.Errorf("front = %v after half a loop, want it pointing back along +Z", front)
	}
	if up := camera.GetUp(); up.Y() > -0.99 {
		t.Errorf("up = %v after half a loop, want it upside down", up)
	}

	// The same eighteen steps on the ground stop short of vertical instead.
	walking := NewFPSCamera(mgl32.Vec3{}, 640, 480, 1)
	walking.SetForward2D(mgl32.Vec3{0, 0, -1})
	for i := 0; i < 18; i++ {
		walking.ChangeAngles(0, -10)
	}
	if _, pitch := walking.GetRotation(); pitch != 89 {
		t.Errorf("walking pitch = %v, want it held at the 89 degree clamp", pitch)
	}
}

// Coming back in off free look keeps the eyes where they are: the two angles
// are read back off the basis rather than reset, so gravity taking over is not
// a jump cut.
func TestLeavingFreeLookKeepsTheView(t *testing.T) {
	camera := NewFPSCamera(mgl32.Vec3{}, 640, 480, 1)
	camera.SetForward2D(mgl32.Vec3{0, 0, -1})
	camera.SetFreeLook(true)
	camera.ChangeAngles(35, -20)
	camera.RollBy(40)

	front, roll := camera.GetForward(), camera.Roll()
	camera.SetFreeLook(false)
	if moved := camera.GetForward().Sub(front).Len(); moved > 0.001 {
		t.Errorf("the view jumped by %v when gravity took over", moved)
	}
	if got := camera.Roll(); mgl32.Abs(got-roll) > 0.001 {
		t.Errorf("roll = %v after leaving free look, want the %v it had", got, roll)
	}
}
