package main

import "github.com/go-gl/mathgl/mgl32"

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
	pmAccelerate    = 10.0
	pmAirAccelerate = 1.0
	pmFriction      = 6.0
	pmStopSpeed     = 100.0 * quakeUnit
	pmSpeed         = 320.0 * quakeUnit
	pmJumpVelocity  = 270.0 * quakeUnit
	q3Gravity       = 800.0 * quakeUnit
	// pmStepSize is Q3's STEPSIZE: the tallest ledge that is walked straight up
	// instead of blocking. Stairs are built under it, walls above it.
	pmStepSize = 18.0 * quakeUnit
	// Below this speed friction stops the player outright, as in PM_Friction.
	pmMinSpeed = 1.0 * quakeUnit
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
func (m *q3Move) Advance(velocity mgl32.Vec3, wishDir mgl32.Vec3, grounded bool, elapsed float64) mgl32.Vec3 {
	if elapsed > pmoveMaxFrameTime {
		elapsed = pmoveMaxFrameTime
	}
	m.accumulator += elapsed
	for m.accumulator >= pmoveStep {
		velocity = pmoveSingle(velocity, wishDir, grounded, pmoveStep)
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

// pmoveSingle is PM_WalkMove / PM_AirMove for one step. Friction only applies on
// the ground; in the air the acceleration is a tenth as strong but is still
// allowed to reach the full wish speed, which is what lets a strafing player
// gain speed beyond pmSpeed.
func pmoveSingle(velocity, wishDir mgl32.Vec3, grounded bool, deltaTime float32) mgl32.Vec3 {
	accel := float32(pmAirAccelerate)
	if grounded {
		velocity = pmFrictionStep(velocity, deltaTime)
		accel = pmAccelerate
	}
	if wishDir == (mgl32.Vec3{}) {
		return velocity
	}
	return pmAccelerateStep(velocity, wishDir, pmSpeed, accel, deltaTime)
}

// pmFrictionStep is PM_Friction: it only slows the horizontal velocity, and
// below pmStopSpeed it bites at a constant rate so the player comes to a halt
// instead of creeping.
func pmFrictionStep(velocity mgl32.Vec3, deltaTime float32) mgl32.Vec3 {
	flat := mgl32.Vec3{velocity.X(), 0, velocity.Z()}
	speed := flat.Len()
	if speed < pmMinSpeed {
		return mgl32.Vec3{0, velocity.Y(), 0}
	}
	control := speed
	if control < pmStopSpeed {
		control = pmStopSpeed
	}
	newSpeed := speed - control*pmFriction*deltaTime
	if newSpeed < 0 {
		newSpeed = 0
	}
	scale := newSpeed / speed
	return mgl32.Vec3{velocity.X() * scale, velocity.Y(), velocity.Z() * scale}
}

// pmAccelerateStep is PM_Accelerate: only the part of the wish speed that is not
// already reached is added, so running straight ahead tops out at wishSpeed
// while movement sideways to the current velocity keeps adding to it.
func pmAccelerateStep(velocity, wishDir mgl32.Vec3, wishSpeed, accel, deltaTime float32) mgl32.Vec3 {
	addSpeed := wishSpeed - velocity.Dot(wishDir)
	if addSpeed <= 0 {
		return velocity
	}
	accelSpeed := accel * deltaTime * wishSpeed
	if accelSpeed > addSpeed {
		accelSpeed = addSpeed
	}
	return velocity.Add(wishDir.Mul(accelSpeed))
}
