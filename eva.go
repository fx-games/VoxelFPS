package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
)

// Outside the hull there is no air, no floor and no up. The walk is gone and
// what is left is the suit: the movement keys are thrusters, they push along
// the way the visor is pointing rather than along the ground, and velocity is
// kept between frames instead of being walked off by friction. Which side of
// the hull the player is on is a test on the position and nothing else - see
// core.Station.Pressurised - so there is no entering and no leaving to get
// wrong, and no way to end up weightless indoors.
//
// This is the same shape as climbVelocity: it either takes movement over for
// this frame and says so, or leaves gravity switched on and lets pollInput get
// on with the ordinary walk.

const (
	// evaThrust is what the pack pushes at, in world units per second squared.
	evaThrust = 12.0
	// evaMaxSpeed caps it, a little over a run: fast enough to cross the hull,
	// slow enough to stop at the airlock you were aiming at.
	evaMaxSpeed = 8.0
	// evaDrag is the suit's stabiliser: the fraction of the speed still there a
	// second after the thrusters stop. Real vacuum has none of this, and a real
	// jetpack is flown by counting out the stopping burn before you start the
	// starting one.
	//
	// ponytail: a stabiliser, not Newton. Take it to 1 and add a brake key if
	// drifting is meant to be the difficulty rather than the scenery.
	evaDrag = 0.35
	// evaRollSpeed is degrees a second the roll keys turn the horizon at. Slow
	// enough that the level is read while it turns rather than after it.
	evaRollSpeed = 90.0
	// evaLevelSpeed is how fast the horizon comes back once there is a deck
	// underfoot again: about a second from upside down, so gravity reads as
	// taking hold of the player rather than as a cut.
	evaLevelSpeed = 180.0
)

// inVacuum reports whether a point is outside the level's pressure hull. A
// level that is not a station has no vacuum anywhere.
func (g *Game) inVacuum(pos mgl32.Vec3) bool {
	return g.station != nil && !g.station.Pressurised(pos)
}

// evaMove flies the suit for one frame, and reports whether it took movement
// over. It is also what puts gravity back on the way in, so walking out of an
// airlock and back in needs nothing to happen at the threshold itself.
func (g *Game) evaMove(physics *quads.PhysicsComponent, moveDir [2]int, upDown, roll int, elapsed float64) bool {
	if !g.inVacuum(physics.Position()) {
		if g.inEVA {
			g.inEVA = false
			// The mouse goes back to turning the player rather than the visor,
			// reading its two angles off wherever the eyes are pointing now.
			g.Camera.SetFreeLook(false)
			g.Toast("Pressure restored")
		}
		// A tilted horizon is only allowed where nothing says which way up is.
		// Back inside it eases level rather than snapping, which is the whole
		// difference between gravity taking hold and a cut.
		g.levelHorizon(elapsed)
		physics.ApplyGravity = true
		return false
	}
	if !g.inEVA {
		g.inEVA = true
		// Out here the mouse turns the visor: yaw about the helmet's own up,
		// pitch about its own right, and no clamp short of vertical, because
		// there is nothing to be upside down relative to. See SetFreeLook.
		g.Camera.SetFreeLook(true)
		g.Toast("Vacuum - suit thrusters")
	}
	physics.ApplyGravity = false
	physics.SetGrounded(false)

	if roll != 0 {
		g.Camera.RollBy(float32(roll) * evaRollSpeed * float32(elapsed))
	}

	// Forward is the full look direction, pitch included: a pack pointed at the
	// hull pushes you at the hull, and up and down are the suit's own.
	thrust := mgl32.Vec3{}
	if moveDir[1] != 0 {
		thrust = thrust.Add(g.Camera.ForwardBackward(float32(moveDir[1])))
	}
	if moveDir[0] != 0 {
		thrust = thrust.Add(g.Camera.LeftRight(float32(moveDir[0])))
	}
	if upDown != 0 {
		// The visor's up, not the world's: once the suit can roll, "up" is
		// wherever the helmet is pointing and the world has no say in it.
		thrust = thrust.Add(g.Camera.GetUp().Mul(float32(upDown)))
	}

	velocity := physics.Velocity()
	if thrust.Len() > 0 {
		velocity = velocity.Add(thrust.Normalize().Mul(evaThrust * float32(elapsed)))
	}
	// Raised to the frame time rather than multiplied by it, so how quickly the
	// suit settles does not depend on how fast the machine is drawing.
	velocity = velocity.Mul(float32(math.Pow(evaDrag, elapsed)))
	if speed := velocity.Len(); speed > evaMaxSpeed {
		velocity = velocity.Mul(evaMaxSpeed / speed)
	}
	physics.SetVelocity(velocity)
	return true
}

// levelHorizon eases the roll back to zero, the short way round, at
// evaLevelSpeed. It is a no-op everywhere the horizon is already level, which
// is everywhere but the seconds after coming in out of the vacuum.
func (g *Game) levelHorizon(elapsed float64) {
	roll := g.Camera.Roll()
	if roll == 0 {
		return
	}
	g.Camera.SetRoll(levelledRoll(roll, float32(evaLevelSpeed*elapsed)))
}

// levelledRoll is one step of that ease: roll brought step degrees towards
// zero, and zero once it is within a step, so it settles instead of wobbling
// either side of level for ever.
func levelledRoll(roll, step float32) float32 {
	if mgl32.Abs(roll) <= step {
		return 0
	}
	if roll > 0 {
		return roll - step
	}
	return roll + step
}
