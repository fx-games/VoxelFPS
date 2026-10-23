package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
)

// climbSpeed is how fast a ladder is climbed, in world units per second.
const climbSpeed = 4.0

// climbVelocity overrides the player's movement for this frame if they are
// standing inside a ladder's climb volume (see Scenery.Climbable): forward or
// Space goes up, back or C goes down, gravity is off, and none of q3move runs
// this frame. It reports whether it took over, so pollInput knows whether to
// fall through to its own movement.
//
// wishDir still drives horizontal movement while climbing, same as a normal
// walk: without it, the only way out of a climb volume is to fall out the
// bottom, since nothing could ever carry the player sideways off the top rung
// onto whatever the ladder was climbed to reach.
func (g *Game) climbVelocity(physics *quads.PhysicsComponent, wishDir mgl32.Vec3, moveDir [2]int, upDown int) bool {
	if !g.onLadder(physics.Position()) {
		physics.ApplyGravity = true
		return false
	}

	physics.ApplyGravity = false
	physics.SetGrounded(false)

	vertical := float32(0)
	switch {
	case moveDir[1] > 0 || upDown > 0:
		vertical = climbSpeed
	case moveDir[1] < 0 || upDown < 0:
		vertical = -climbSpeed
	}
	horizontal := wishDir.Mul(climbSpeed)
	physics.SetVelocity(mgl32.Vec3{horizontal.X(), vertical, horizontal.Z()})
	return true
}

// onLadder reports whether pos is inside any registered climb volume.
func (g *Game) onLadder(pos mgl32.Vec3) bool {
	for _, volume := range g.climbVolumes {
		if volume.piece.IsAlive() && volume.box.Contains(pos) {
			return true
		}
	}
	return false
}
