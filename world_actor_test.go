package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func angleBetween(a, b mgl32.Vec3) float64 {
	return math.Acos(float64(a.Normalize().Dot(b.Normalize()))) * 180 / math.Pi
}

// A homing missile turns towards where it is going a little at a time. Turning
// as far as it liked would put it on a straight line to the target on the first
// step, which is not homing, it is teleporting the aim.
func TestSeekingTurnsGraduallyAndKeepsItsSpeed(t *testing.T) {
	velocity := mgl32.Vec3{0, 0, -10} // flying north at ten
	from := mgl32.Vec3{0, 0, 0}
	target := mgl32.Vec3{10, 0, 0} // due east, a quarter turn away

	maxTurn := 10 * math.Pi / 180 // ten degrees this step
	steered := steer(velocity, from, target, maxTurn)

	assert.InDelta(t, 10, float64(steered.Len()), 0.001, "homing does not speed it up")
	turned := angleBetween(velocity, steered)
	assert.InDelta(t, 10, turned, 1.0, "it turned about as far as it was allowed")

	// Given enough steps it comes round to the target.
	for i := 0; i < 20; i++ {
		steered = steer(steered, from, target, maxTurn)
	}
	assert.Less(t, angleBetween(steered, target.Sub(from)), 1.0)
}

func TestSeekingLeavesAlonWhatItCannotSteer(t *testing.T) {
	from := mgl32.Vec3{0, 0, 0}
	target := mgl32.Vec3{10, 0, 0}

	// Something standing still has no heading to turn.
	still := steer(mgl32.Vec3{}, from, target, 1)
	assert.Equal(t, mgl32.Vec3{}, still)

	// A target where it already is gives nothing to aim at.
	velocity := mgl32.Vec3{0, 0, -10}
	assert.Equal(t, velocity, steer(velocity, from, from, 1))

	// And no turn allowed means no turn.
	assert.Equal(t, velocity, steer(velocity, from, target, 0))
}

// Once it is pointed at the target it stays pointed at it, rather than
// overshooting by whatever it had left to turn.
func TestSeekingDoesNotOvershoot(t *testing.T) {
	velocity := mgl32.Vec3{10, 0, 0}
	steered := steer(velocity, mgl32.Vec3{}, mgl32.Vec3{100, 0, 0}, math.Pi)
	require.InDelta(t, 10, float64(steered.Len()), 0.001)
	assert.Less(t, angleBetween(steered, velocity), 0.001)
}
