package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func flatSpeed(v mgl32.Vec3) float32 {
	return mgl32.Vec3{v.X(), 0, v.Z()}.Len()
}

// run advances count steps with a fixed wish direction.
func run(velocity, wishDir mgl32.Vec3, grounded bool, count int) mgl32.Vec3 {
	for i := 0; i < count; i++ {
		velocity = pmoveSingle(velocity, wishDir, grounded, pmoveStep)
	}
	return velocity
}

func TestGroundRunTopsOutAtWishSpeed(t *testing.T) {
	forward := mgl32.Vec3{1, 0, 0}
	// Two seconds is plenty to reach the cap from standing.
	velocity := run(mgl32.Vec3{}, forward, true, 250)
	if diff := math.Abs(float64(flatSpeed(velocity) - pmSpeed)); diff > 0.01 {
		t.Fatalf("ground speed %v, want %v", flatSpeed(velocity), pmSpeed)
	}
	// Running further must not push past it.
	velocity = run(velocity, forward, true, 250)
	if flatSpeed(velocity) > pmSpeed+0.01 {
		t.Fatalf("ground speed %v exceeds %v", flatSpeed(velocity), pmSpeed)
	}
}

func TestFrictionStopsThePlayer(t *testing.T) {
	velocity := run(mgl32.Vec3{pmSpeed, 0, 0}, mgl32.Vec3{}, true, 250)
	if flatSpeed(velocity) != 0 {
		t.Fatalf("still moving at %v after two seconds of friction", flatSpeed(velocity))
	}
}

func TestNoFrictionInTheAir(t *testing.T) {
	velocity := run(mgl32.Vec3{pmSpeed, 0, 0}, mgl32.Vec3{}, false, 250)
	if velocity.X() != pmSpeed {
		t.Fatalf("air speed %v, want it unchanged at %v", velocity.X(), pmSpeed)
	}
}

// Strafing in the air sideways to the current velocity keeps adding speed. That
// is the strafe jumping Quake 3 is known for, so it has to survive.
func TestAirStrafeGainsSpeed(t *testing.T) {
	const angle = 85 * math.Pi / 180
	wishDir := mgl32.Vec3{float32(math.Cos(angle)), 0, float32(math.Sin(angle))}
	velocity := run(mgl32.Vec3{pmSpeed, 0, 0}, wishDir, false, 125)
	if flatSpeed(velocity) <= pmSpeed {
		t.Fatalf("air speed %v, want more than %v", flatSpeed(velocity), pmSpeed)
	}
}

func TestJumpNeedsGroundAndAReleasedKey(t *testing.T) {
	var m q3Move
	if !m.CheckJump(true, true) {
		t.Fatal("first press on the ground should jump")
	}
	if m.CheckJump(true, true) {
		t.Fatal("holding the key should not jump again")
	}
	m.CheckJump(false, true)
	if m.CheckJump(true, false) {
		t.Fatal("jumped in mid air")
	}
}

// Quake 3 jumps 270/800 of a unit-scaled arc, about 45.5 units, which is most of
// the player's own height.
func TestJumpHeight(t *testing.T) {
	height := pmJumpVelocity * pmJumpVelocity / (2 * q3Gravity)
	if want := 45.5625 * quakeUnit; math.Abs(height-want) > 0.001 {
		t.Fatalf("jump height %v, want %v", height, want)
	}
}
