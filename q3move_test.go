package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

func flatSpeed(v mgl32.Vec3) float32 {
	return mgl32.Vec3{v.X(), 0, v.Z()}.Len()
}

// run advances count steps with a fixed wish direction, standing up.
func run(velocity, wishDir mgl32.Vec3, grounded bool, count int) mgl32.Vec3 {
	return runDucked(velocity, wishDir, grounded, false, count)
}

func runDucked(velocity, wishDir mgl32.Vec3, grounded, crouched bool, count int) mgl32.Vec3 {
	for i := 0; i < count; i++ {
		velocity = pmoveSingle(velocity, wishDir, grounded, crouched, pmoveStep)
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

// Ducking is PM_CmdScale's half speed, not a different walk: the same run, the
// same friction, topping out at half of what standing does.
func TestDuckedRunTopsOutAtHalfSpeed(t *testing.T) {
	wishDir := mgl32.Vec3{1, 0, 0}
	ducked := runDucked(mgl32.Vec3{}, wishDir, true, true, 250)
	if diff := math.Abs(float64(flatSpeed(ducked) - pmSpeed*pmDuckScale)); diff > 0.01 {
		t.Fatalf("ducked speed %v, want %v", flatSpeed(ducked), pmSpeed*pmDuckScale)
	}

	// And a player who was already running loses the difference rather than
	// keeping it: friction bites and the acceleration no longer replaces it.
	slowed := runDucked(mgl32.Vec3{pmSpeed, 0, 0}, wishDir, true, true, 250)
	if flatSpeed(slowed) > pmSpeed*pmDuckScale+0.01 {
		t.Fatalf("a run carried into a duck holds %v, want it down to %v", flatSpeed(slowed), pmSpeed*pmDuckScale)
	}
}

// Standing up is the half of crouching that can hurt: let go of the key under a
// half-height door and a player who stood up anyway would have their head in
// the wall, and the collision solver pushes that back out wherever it can - which
// is not necessarily the side they came in from.
func TestNoStandingUpUnderACeiling(t *testing.T) {
	// A ceiling at y=1: the voxel the head of a standing player at y=0.2 is in.
	ceiling := func(x, y, z int32) bool { return y >= 1 }
	if roomToStandAt(mgl32.Vec3{4.5, 0.2, 4.5}, ceiling) {
		t.Error("a player under a ceiling was told to stand up into it")
	}
	if !roomToStandAt(mgl32.Vec3{4.5, 0.2, 4.5}, func(x, y, z int32) bool { return false }) {
		t.Error("a player in the open was kept crouched")
	}

	// The box is narrower than a voxel but it still straddles the seam between
	// two of them, and a ceiling over either half is a ceiling.
	onSeam := func(x, y, z int32) bool { return y >= 1 && x == 5 }
	if roomToStandAt(mgl32.Vec3{4.9, 0.2, 4.5}, onSeam) {
		t.Error("a ceiling over one corner of the box was missed")
	}
}
