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
