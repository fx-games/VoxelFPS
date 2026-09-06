package main

import (
	"math"
	"math/rand"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/wep"
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

// A muzzle flash sits half a voxel ahead of the eye and a little under it. Aim
// down and it has to follow the barrel down; the flash that stayed at chest
// height while the gun pointed at the floor is what this stops.
func TestSpawnOffsetsFollowTheAim(t *testing.T) {
	forward, sideways, up := float32(0.5), float32(0), float32(-0.2)

	level := aimOffset(mgl32.Vec3{0, 0, -1}, forward, sideways, up)
	assert.InDelta(t, -0.2, level.Y(), 0.001, "level aim keeps the height as written")
	assert.InDelta(t, -0.5, level.Z(), 0.001)

	down := aimOffset(mgl32.Vec3{0, -1, 0}, forward, sideways, up)
	assert.InDelta(t, -0.5, down.Y(), 0.001, "looking down, the reach ahead is downwards")
	require.InDelta(t, level.Len(), down.Len(), 0.001, "the rig turns, it does not stretch")
}

// A shell casing leaves sideways because its spawn says so: "seven units a tic,
// ninety degrees round". Drop the angle and every casing in the game flies down
// the line of fire instead. ZDoom counts a positive angle to the left, the
// opposite way round from a positive sideways offset.
func TestSpawnVelocityTurnsWithItsAngle(t *testing.T) {
	aim := mgl32.Vec3{1, 0, 0}
	right := aim.Cross(mgl32.Vec3{0, 1, 0}) // which is +z

	sideways, _ := explicitVelocity(wep.Op{VelY: 2, VelYTo: 2}, aim, nil)
	require.Greater(t, sideways.Dot(right), float32(0), "a positive sideways velocity goes right")

	ejected, _ := explicitVelocity(wep.Op{VelX: 7, VelXTo: 7, Angle: -90, AngleTo: -90}, aim, nil)
	assert.Greater(t, ejected.Dot(right), float32(0), "a negative angle throws it right")
	assert.InDelta(t, 0, ejected.Dot(aim), 0.001, "ninety degrees round leaves nothing going forward")

	other := mustVelocity(t, wep.Op{VelX: 7, VelXTo: 7, Angle: 90, AngleTo: 90}, aim)
	assert.Less(t, other.Dot(right), float32(0), "a positive angle throws it left")
	assert.InDelta(t, ejected.Len(), other.Len(), 0.001, "the throw turns, it does not stretch")
}

func mustVelocity(t *testing.T, op wep.Op, aim mgl32.Vec3) mgl32.Vec3 {
	t.Helper()
	velocity, ok := explicitVelocity(op, aim, nil)
	require.True(t, ok)
	return velocity
}

// ZDoom's pitch aims down when it is positive, and A_CustomMissile under
// CMF_ABSOLUTEPITCH takes |cos(pitch)| across so a pitch past ninety degrees
// still goes forward - Brutal Doom's BigBlackSmoke is fired at random(40,160)
// and would otherwise come back at the player.
func TestAPitchAimsDownAndNeverBackwards(t *testing.T) {
	forward := mgl32.Vec3{1, 0, 0}

	level := pitchedBy(forward, 0, false)
	assert.InDelta(t, 0, level.Y(), 1e-6, "no pitch leaves the heading alone")

	down := pitchedBy(forward, 30, false)
	assert.InDelta(t, -0.5, down.Y(), 1e-6, "positive is down")
	assert.Greater(t, down.X(), float32(0))

	up := pitchedBy(forward, -30, false)
	assert.InDelta(t, 0.5, up.Y(), 1e-6, "negative is up")

	// Absolute, past vertical: still travelling forwards.
	past := pitchedBy(forward, 160, true)
	assert.Greater(t, past.X(), float32(0), "|cos| keeps it going the way it was aimed")
	assert.Less(t, past.Y(), float32(0), "and 160 degrees of pitch is still downwards")

	// Relative is measured from where the heading already points, absolute is
	// not: aiming 45 down and adding 45 more gets 90; absolute 45 stays 45.
	slanted := pitchedBy(forward, 45, false)
	assert.InDelta(t, -1, pitchedBy(slanted, 45, false).Y(), 1e-5)
	assert.InDelta(t, float64(pitchedBy(forward, 45, true).Y()),
		float64(pitchedBy(slanted, 45, true).Y()), 1e-6)
}

// Final Doomer's gun smoke is spawned with the velocity of the thing that made
// it - `A_SpawnItemEx("FDGunSmoke", 0,0,0, velx, vely+frandom(-0.2,0.2), velz,
// 0, SXF_ABSOLUTEVELOCITY)`. Two things have to hold for that to mean anything:
// the numbers have to be read off the spawner while the game runs, because
// folded at migration time every one of them is zero, and they have to be left
// in the world's own axes, because re-rotating an already absolute velocity by
// the spawner's facing sends the smoke off sideways for every heading but one.
func TestAnInheritedVelocityIsReadOffTheSpawnerAndLeftInWorldAxes(t *testing.T) {
	spawner := &wep.Runtime{Vars: map[string]float64{}, Rand: rand.New(rand.NewSource(1))}
	spawner.SetVar("velx", 0)
	spawner.SetVar("vely", -7) // travelling north, which is -z here
	spawner.SetVar("velz", 0)

	named := func(name string) wep.Expr { return wep.Expr{{Kind: wep.TokenVar, Name: name}} }
	op := wep.Op{
		Op:               wep.OpSpawnItem,
		AbsoluteVelocity: true,
		VelExprs:         []wep.Expr{named("velx"), named("vely"), named("velz")},
	}

	// The aim points east; the throw must not follow it.
	velocity, ok := explicitVelocity(op, mgl32.Vec3{1, 0, 0}, spawner)
	require.True(t, ok, "a velocity read off the spawner still counts as written")
	assert.InDelta(t, 0, velocity.X(), 1e-5, "nothing was going east")
	assert.Less(t, velocity.Z(), float32(0), "it keeps the spawner's own heading")

	// Without a spawner to read there is nothing to inherit, and the spawn is
	// left to whatever launchVelocity decides rather than pinned still.
	_, ok = explicitVelocity(op, mgl32.Vec3{1, 0, 0}, nil)
	assert.False(t, ok)
}

// GZDoom does not draw the first frame of a missile the viewer fired while it
// is still on top of them, which is what keeps a rocket - and Brutal Doom's
// eight bright tracers - out of your face on the frame you pull the trigger.
func TestAPointBlankMissileOfYourOwnIsNotDrawn(t *testing.T) {
	eye := mgl32.Vec3{0, 0, 0}
	fired := func(speed float64, velocity mgl32.Vec3, at mgl32.Vec3) *WorldActor {
		object := quads.NewGameObject("t")
		object.Transform().SetPosition(at)
		return &WorldActor{
			data:     &wep.Actor{Missile: true, Speed: speed},
			owner:    &wep.Runtime{},
			object:   object,
			velocity: velocity,
		}
	}

	// A tracer at 120 units a tic, spawned on the player: clipdist clamps to
	// twice the 16 unit radius, so anything inside half a voxel is skipped.
	quick := mgl32.Vec3{65, 0, 0}
	assert.True(t, fired(120, quick, mgl32.Vec3{0.16, 0, 0}).hiddenAtPointBlank(eye))
	assert.False(t, fired(120, quick, mgl32.Vec3{0.6, 0, 0}).hiddenAtPointBlank(eye),
		"once it is past the clip distance it is a thing in the world like any other")

	// Something a script parked where it wants it is not going anywhere, and
	// GZDoom's speed test is what stops the rule eating it.
	assert.False(t, fired(20, mgl32.Vec3{0.01, 0, 0}, mgl32.Vec3{0.1, 0, 0}).hiddenAtPointBlank(eye))

	// And nothing the player did not fire is touched.
	stray := fired(120, quick, mgl32.Vec3{0.16, 0, 0})
	stray.owner = nil
	assert.False(t, stray.hiddenAtPointBlank(eye))
}
