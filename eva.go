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
)

// inVacuum reports whether a point is outside the level's pressure hull. A
// level that is not a station has no vacuum anywhere.
func (g *Game) inVacuum(pos mgl32.Vec3) bool {
	return g.station != nil && !g.station.Pressurised(pos)
}

// evaMove flies the suit for one frame, and reports whether it took movement
// over. It is also what puts gravity back on the way in, so walking out of an
// airlock and back in needs nothing to happen at the threshold itself.
func (g *Game) evaMove(physics *quads.PhysicsComponent, moveDir [2]int, upDown int, elapsed float64) bool {
	if !g.inVacuum(physics.Position()) {
		if g.inEVA {
			g.inEVA = false
			g.Toast("Pressure restored")
		}
		physics.ApplyGravity = true
		return false
	}
	if !g.inEVA {
		g.inEVA = true
		g.Toast("Vacuum - suit thrusters")
	}
	physics.ApplyGravity = false
	physics.SetGrounded(false)

	// Forward is the full look direction, pitch included: a pack pointed at the
	// hull pushes you at the hull. Up and down stay the world's own, which is
	// what makes lining up with a hatch possible at all.
	thrust := mgl32.Vec3{}
	if moveDir[1] != 0 {
		thrust = thrust.Add(g.Camera.ForwardBackward(float32(moveDir[1])))
	}
	if moveDir[0] != 0 {
		thrust = thrust.Add(g.Camera.LeftRight(float32(moveDir[0])))
	}
	if upDown != 0 {
		thrust = thrust.Add(mgl32.Vec3{0, float32(upDown), 0})
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
