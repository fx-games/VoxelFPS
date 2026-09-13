package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Crouching, out of Quake 3's PM_CheckDuck and PM_CmdScale, in the Q3 units the
// rest of the movement is written in (see q3move.go): the collider comes down
// from 56 units to 40, the eye drops 14 with it, and a ducked player walks at
// half speed.
//
// The one rule that is not a number is standing up: it needs room overhead, or
// a player who ducked under a half-height door and let go of the key would
// stand up inside it and be pushed out of the world by the collision solver.
// Until there is room, the key can be let go of and the crouch stays.
const (
	crouchHeight   = 40 * quakeUnit
	crouchEyeDrop  = 14 * quakeUnit
	crouchHalfSize = 0.2

	// crouchEyeSpeed is how fast the eye travels between the two heights, in
	// world units a second: about a sixth of a second for the whole drop.
	// Q3 smooths the same movement, and for the same reason - the camera
	// jumping a quarter of the player's height in one frame reads as a glitch
	// rather than as ducking.
	crouchEyeSpeed = 1.4
)

// updateCrouch is one frame of it: the collider changes the instant the state
// does, because that is what the world has to agree with, and the eye follows
// over the next few frames. wanted is the key being held; it is overruled by a
// ceiling, never by anything else.
//
// It runs before the ladder and the suit have had their say, so that letting go
// of the key stands the player up wherever they are, and so that switching
// gravity off in the editor does not leave a crouch nothing can undo.
func (g *Game) updateCrouch(wanted bool, elapsed float64) {
	physics := g.player.Physics()
	if !wanted && g.crouched && !roomToStandAt(physics.Position(), g.solidAt) {
		wanted = true
	}
	if wanted != g.crouched {
		g.crouched = wanted
		height := float32(playerHeight)
		if wanted {
			height = crouchHeight
		}
		physics.SetAABB(playerBox(height))
	}

	target := float32(playerEyeHeight)
	if g.crouched {
		target = playerEyeHeight - crouchEyeDrop
	}
	step := float32(elapsed * crouchEyeSpeed)
	switch {
	case g.eyeHeight < target:
		g.eyeHeight = min(g.eyeHeight+step, target)
	case g.eyeHeight > target:
		g.eyeHeight = max(g.eyeHeight-step, target)
	default:
		return
	}
	// The camera hangs off the player's own transform, so this is the eye's
	// height above the feet and nothing else - see quads.NewCameraComponent,
	// which sets the standing one the same way.
	g.Camera.SetPosition(mgl32.Vec3{0, g.eyeHeight, 0})
}

// roomToStandAt reports whether a player standing up with their feet here would
// have their head in something solid. The four top corners of the standing box
// are what is asked about: the box is narrower than a voxel, so a corner in each
// cell it can reach into covers all of them.
//
// ponytail: only the level's own voxels, which is what solidAt answers for. A
// crate or a door leaf overhead is on the physics grid instead and is not
// consulted, so a player can stand up through one; ask the solver for the swept
// box if that ever matters.
func roomToStandAt(feet mgl32.Vec3, solid func(x, y, z int32) bool) bool {
	// A hair under the top of the box: exactly on it is the floor of the voxel
	// above, which a standing player's head does not actually occupy.
	head := int32(math.Floor(float64(feet.Y() + playerHeight - 0.01)))
	for _, corner := range [][2]float32{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		x := int32(math.Floor(float64(feet.X() + corner[0]*crouchHalfSize)))
		z := int32(math.Floor(float64(feet.Z() + corner[1]*crouchHalfSize)))
		if solid(x, head, z) {
			return false
		}
	}
	return true
}
