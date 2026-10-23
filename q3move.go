package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
)

// Player movement taken from Quake 3's bg_pmove.c: friction, acceleration
// towards a wish direction, weak acceleration in the air (which is what makes
// strafe jumping work), and a jump that needs the key to be released first.
// Gravity and collision response stay with the physics system.

// quakeUnit is how long one Quake 3 unit is in this game. Q3's player is 56
// units tall, ours is 0.9. Everything below is written in Q3 units and scaled
// by this, so speed, jump height and gravity keep their original proportions.
// Change it and the whole feel scales together.
const quakeUnit = 0.9 / 56.0

const (
	// pmSpeed is the top running speed (character.PMoveStep's own).
	pmSpeed = 320.0 * quakeUnit
	// pmDuckScale is PM_CmdScale's own: a ducked player asks for half the speed
	// a standing one does.
	pmDuckScale    = 0.5
	pmJumpVelocity = 270.0 * quakeUnit
	q3Gravity      = 800.0 * quakeUnit
	// pmStepSize is Q3's STEPSIZE: the tallest ledge that is walked straight up
	// instead of blocking. Stairs are built under it, walls above it.
	pmStepSize = 18.0 * quakeUnit
	// pmoveStep is Q3's pmove_fixed interval (8ms). Movement is advanced in
	// these so friction and acceleration do not depend on the framerate.
	pmoveStep = 1.0 / 125.0
	// pmoveMaxFrameTime caps how much movement one frame may ask for, so a
	// stall does not queue up hundreds of steps.
	pmoveMaxFrameTime = 0.25
)

// q3Move holds the movement state that has to survive between frames.
type q3Move struct {
	jumpHeld    bool
	accumulator float64
}

// Advance runs the pmove steps that fit into elapsed and returns the new
// velocity. wishDir is the horizontal direction the player is asking for, zero
// when no movement key is down.
func (m *q3Move) Advance(velocity mgl32.Vec3, wishDir mgl32.Vec3, grounded, crouched bool, elapsed float64) mgl32.Vec3 {
	if elapsed > pmoveMaxFrameTime {
		elapsed = pmoveMaxFrameTime
	}
	m.accumulator += elapsed
	for m.accumulator >= pmoveStep {
		velocity = pmoveSingle(velocity, wishDir, grounded, crouched, pmoveStep)
		m.accumulator -= pmoveStep
	}
	return velocity
}

// CheckJump reports whether the jump key press should launch the player. The
// key has to be released before it counts again, as in PM_CheckJump.
func (m *q3Move) CheckJump(pressed, grounded bool) bool {
	jump := pressed && !m.jumpHeld && grounded
	m.jumpHeld = pressed
	return jump
}

// pmoveSingle is PM_WalkMove / PM_AirMove for one step (VoxelLib
// character.PMoveStep). Friction only applies on the ground; in the air the
// acceleration is a tenth as strong but may still reach the full wish speed,
// which is what lets a strafing player gain speed beyond pmSpeed.
func pmoveSingle(velocity, wishDir mgl32.Vec3, grounded, crouched bool, deltaTime float32) mgl32.Vec3 {
	vx, vz := character.PMoveStep(float64(velocity.X()), float64(velocity.Z()), float64(wishDir.X()), float64(wishDir.Z()),
		grounded, crouched, float64(deltaTime))
	return mgl32.Vec3{float32(vx), velocity.Y(), float32(vz)}
}
