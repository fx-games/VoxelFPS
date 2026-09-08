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
			data:     &wep.Effect{Missile: true, Speed: speed},
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

// A frag grenade's own fireball and the smoke column it spawns in the same
// breath (A_SpawnItem, no offset) land on the exact same call-site position -
// see spawnActorAt. spawnJitter is what keeps that from being a literal
// coincident point in world space, which two textured quads sharing a depth
// buffer cannot resolve cleanly at any bias (see the comment on spawnJitter):
// distinct sequence numbers must come back distinct, the offset has to stay
// small enough that nobody notices, and it has to stay bounded rather than
// drifting a long-lived actor further from its script's own position the
// longer the game session runs.
func TestSpawnJitterSeparatesCoincidentSpawnsWithoutDriftingFar(t *testing.T) {
	base := mgl32.Vec3{1, 2, 3}

	a := spawnJitter(base, 41)
	b := spawnJitter(base, 42)
	assert.NotEqual(t, a, b, "consecutive spawns landed on the exact same point again")

	for _, seq := range []int{0, 1, 5, 100, 100000} {
		got := spawnJitter(base, seq)
		assert.InDelta(t, 0, got.X()-base.X(), 1e-6, "only height should move")
		assert.InDelta(t, 0, got.Z()-base.Z(), 1e-6, "only height should move")
		assert.Less(t, got.Y()-base.Y(), spawnJitterStep*spawnJitterCycle,
			"a session's worth of spawns must not push this arbitrarily far off script")
		assert.GreaterOrEqual(t, got.Y(), base.Y())
	}

	// The cycle repeats rather than growing forever.
	assert.Equal(t, spawnJitter(base, 3), spawnJitter(base, 3+spawnJitterCycle))
}

// A frag grenade sets +BOUNCEONFLOORS/+BOUNCEONWALLS/+BOUNCEONCEILINGS with a
// BounceFactor of 0.5 and a WallBounceFactor of 0.25 (see
// weapons/bd_frag_grenade/source.txt); before bounceOff existed, move() froze
// anything that touched a solid voxel in place regardless of what its own
// class asked for, which is what "grenades stick to walls" was - a bouncing
// actor treated exactly like a bullet's puff. These pin the physics down
// without a voxel world behind it: bounceOff takes which axis move found
// blocked as plain bools instead of asking solidAt itself.
func TestBounceOffReflectsOnlyTheAxisTheSurfaceBlocked(t *testing.T) {
	grenade := &WorldActor{data: &wep.Effect{
		BounceFloors: true, BounceWalls: true, BounceCeilings: true,
		BounceFactor: 0.5, WallBounceFactor: 0.25,
	}}

	// Falling, and the floor is what stops it: only Y reflects, scaled by
	// BounceFactor, and it keeps moving up as far as it fell down - the
	// grenade goes on falling next frame, not stuck at the last airborne inch.
	grenade.velocity = mgl32.Vec3{2, -10, 0}
	from := mgl32.Vec3{0, 1, 0}
	step := grenade.velocity.Mul(0.1) // {0.2, -1, 0}
	to := from.Add(step)
	next, bounced := grenade.bounceOff(from, to, step, false, false, true)
	assert.True(t, bounced)
	assert.Equal(t, mgl32.Vec3{2, 5, 0}, grenade.velocity, "Y reflects at half speed, X is untouched")
	assert.Equal(t, to.X(), next.X(), "the clear horizontal axis still moves")
	assert.Equal(t, from.Y(), next.Y(), "the blocked vertical axis does not - there is nowhere to put it")

	// Flying dead into a wall square-on (no Z component at all): only X
	// reflects, at WallBounceFactor, and Y (unblocked) keeps falling under
	// gravity as normal.
	grenade.velocity = mgl32.Vec3{4, -1, 0}
	from = mgl32.Vec3{0, 1, 0}
	step = grenade.velocity.Mul(0.1) // {0.4, -0.1, 0}
	to = from.Add(step)
	next, bounced = grenade.bounceOff(from, to, step, true, false, false)
	assert.True(t, bounced)
	assert.Equal(t, mgl32.Vec3{-1, -1, 0}, grenade.velocity, "X reflects at a quarter speed, Y is untouched")
	assert.Equal(t, from.X(), next.X(), "the blocked wall axis does not move")
	assert.Equal(t, to.Y(), next.Y(), "the clear vertical axis still moves")

	// A ceiling bounces too - the flag is its own, checked by the sign of the
	// step rather than assumed from BounceFloors.
	grenade.velocity = mgl32.Vec3{0, 6, 0}
	from = mgl32.Vec3{0, 1, 0}
	step = grenade.velocity.Mul(0.1)
	to = from.Add(step)
	_, bounced = grenade.bounceOff(from, to, step, false, false, true)
	assert.True(t, bounced)
	assert.Equal(t, float32(-3), grenade.velocity.Y())

	// Something that does not bounce off a wall at all - only floors and
	// ceilings - stops there instead, the way every actor did before bounce
	// flags existed: settling is move()'s job once bounceOff reports false,
	// not bounceOff's, so nothing here should have touched the velocity.
	shell := &WorldActor{data: &wep.Effect{BounceFloors: true, BounceFactor: 0.5}}
	shell.velocity = mgl32.Vec3{3, -1, 0}
	from = mgl32.Vec3{0, 1, 0}
	step = shell.velocity.Mul(0.1)
	to = from.Add(step)
	_, bounced = shell.bounceOff(from, to, step, true, false, false)
	assert.False(t, bounced, "no BounceWalls, so a wall hit is not a bounce")
	assert.Equal(t, mgl32.Vec3{3, -1, 0}, shell.velocity, "and bounceOff must not have touched it")
}

// The bug report this pins down: a grenade clipping a flat wall at a shallow
// angle used to reverse its ENTIRE horizontal velocity - X and Z both -
// because the old blockedByWall was one bool for the whole horizontal step,
// with no way to say which axis the wall actually opposed. A wall that only
// blocks X (it runs along X, facing along Z - a corridor's side wall, say)
// must only reflect X; Z, running along the wall, is untouched and the
// grenade deflects and slides rather than bouncing straight back the way it
// came.
func TestBounceOffOnlyReflectsTheAxisTheWallActuallyBlocked(t *testing.T) {
	grenade := &WorldActor{data: &wep.Effect{
		BounceWalls: true, WallBounceFactor: 1, // no damping, so the angle math is exact
	}}

	// Moving mostly along Z, grazing in the +X direction, into a wall that
	// only blocks X (its face runs along X, i.e. it opposes Z... no: it
	// blocks the X axis of the step, so it is a wall facing along X,
	// perpendicular to the corridor - the grenade is sliding along it).
	incoming := mgl32.Vec3{1, 0, 5}
	grenade.velocity = incoming
	from := mgl32.Vec3{0, 1, 0}
	step := incoming.Mul(0.1)
	to := from.Add(step)

	next, bounced := grenade.bounceOff(from, to, step, true, false, false)
	require.True(t, bounced)

	want := mgl32.Vec3{-incoming.X(), 0, incoming.Z()}
	assert.Equal(t, want, grenade.velocity,
		"only X should reflect - Z runs along the wall and must be left alone")
	assert.NotEqual(t, mgl32.Vec3{-incoming.X(), 0, -incoming.Z()}, grenade.velocity,
		"the old bug: reversing the whole horizontal vector instead of just the blocked axis")
	assert.Equal(t, from.X(), next.X(), "the blocked axis does not move")
	assert.Equal(t, to.Z(), next.Z(), "the clear axis keeps moving")

	// The reflection is shallow, not a reversal: the angle it turns through
	// is small (twice the angle between the incoming path and the wall,
	// here close to 180 minus a shallow glancing angle - the point is it is
	// nowhere near the ~0 turn a dead-on wall hit would keep it from having,
	// nor the near-180 a full reversal would produce for a mostly-Z path).
	turned := angleBetween(incoming, grenade.velocity)
	assert.Less(t, turned, 90.0, "a shallow graze should deflect, not reverse")

	// The symmetric case: only Z blocked, X must be left alone.
	grenade.velocity = incoming
	next, bounced = grenade.bounceOff(from, to, step, false, true, false)
	require.True(t, bounced)
	assert.Equal(t, mgl32.Vec3{incoming.X(), 0, -incoming.Z()}, grenade.velocity,
		"only Z should reflect - X runs along this wall")
	assert.Equal(t, to.X(), next.X(), "the clear axis keeps moving")
	assert.Equal(t, from.Z(), next.Z(), "the blocked axis does not move")

	// A true corner hit - both axes actually blocked - still reverses both,
	// same as a dead-on hit would.
	grenade.velocity = incoming
	_, bounced = grenade.bounceOff(from, to, step, true, true, false)
	require.True(t, bounced)
	assert.Equal(t, mgl32.Vec3{-incoming.X(), 0, -incoming.Z()}, grenade.velocity)
}
