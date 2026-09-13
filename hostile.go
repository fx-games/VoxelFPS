package main

// Enemies that come after you, and the health that makes that matter.
//
// The companion mechanic already had half of this - walk toward something,
// stop, face it, fire on a cooldown - and it is the same half a hostile needs;
// what is here is the other half: noticing, line of sight, the attack a kind
// brought with it out of its own game, and a player who can be hit.

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// How a hostile behaves, in numbers.
const (
	// hostileSightRange is how far one can see you at all. Past it it walks its
	// patrol as if you were not there.
	hostileSightRange = 28.0
	// hostileAttackInterval is the wait between attacks for a kind whose own
	// data did not say. A little over a second is the rate both source games
	// settle on for an ordinary guard.
	hostileAttackInterval = 1.2
	// hostileRepathInterval is how often a chase re-aims at the player. A path
	// is a whole A* search, so this is a rate limit and not a preference.
	hostileRepathInterval = 0.4
	// hostileMeleeRange is the reach of an attack whose kind named none.
	hostileMeleeRange = 2.0
	// hostileGiveUpRange is how far you have to get before it stops chasing and
	// goes back to its beat. Wider than sight, so an enemy does not flip
	// between hunting and patrolling on the edge of its own vision.
	hostileGiveUpRange = 40.0
)

// updateHostile is one enemy's whole mind: notice, chase, attack, give up.
// Everything it does to get anywhere is the pathfinder's, and everything it
// does when it gets there is attackPlayer's.
func (g *Game) updateHostile(actor *Actor, elapsed float64) {
	if g.player == nil {
		return
	}
	center := actor.object.Physics().AABB().Center()
	playerPos := g.player.Transform().GetPosition()
	distance := flatDistance(center, playerPos)

	if !actor.hunting {
		if distance > hostileSightRange || !g.hasLineOfSight(center, playerPos) {
			// Back on its beat, and pathing back on with it: attacking halts by
			// switching the component off, so something that has lost interest
			// has to be started again or it stands where it gave up.
			actor.pathing.Enabled = true
			g.patrol(actor)
			return
		}
		actor.hunting = true
		g.noticePlayer(actor)
	}
	if distance > hostileGiveUpRange {
		actor.hunting = false
		actor.pathing.Enabled = true
		g.patrol(actor)
		return
	}

	if distance <= actor.attackRange() && g.hasLineOfSight(center, playerPos) {
		g.attackPlayer(actor, center, playerPos, elapsed)
		return
	}
	// Out of reach, or something in the way: close the distance. The clock is
	// what keeps this from being a grid search a frame per enemy.
	actor.attackClock = 0
	actor.repathClock -= elapsed
	if actor.repathClock > 0 {
		return
	}
	actor.repathClock = hostileRepathInterval
	actor.pathing.Enabled = true
	actor.pathing.SetDestination(playerPos)
}

// attackRange is how close this one has to be, its own kind's number or the
// reach of a swing for a kind that named none.
func (a *Actor) attackRange() float32 {
	if a.kind != nil && a.kind.Attack != nil && a.kind.Attack.Range > 0 {
		return a.kind.Attack.Range
	}
	return hostileMeleeRange
}

// attackPlayer holds the enemy still, faces it at the player and attacks on its
// own cooldown. The shot leaves from clear of the enemy's own collider, because
// a ray that starts inside a box reports a hit on it - which is how a guard
// shoots itself in the chest.
func (g *Game) attackPlayer(actor *Actor, center, playerPos mgl32.Vec3, elapsed float64) {
	actor.halt()
	actor.object.Transform().SetLookAt2D(playerPos)

	actor.attackClock -= elapsed
	if actor.attackClock > 0 {
		return
	}
	actor.attackClock = actor.attackInterval()

	actor.object.Sprite().PlayAnimationInterrupting(int(quads.AnimFire))
	g.playActorCue(actor, actorCueAttack)

	direction := playerPos.Sub(center).Normalize()
	half := actor.object.Physics().AABB().HalfExtents()
	muzzle := center.Add(direction.Mul(max(half.X(), half.Z()) + 0.2))

	attack := actor.kind.Attack
	damage := hostileDefaultDamage
	if attack != nil && attack.Damage > 0 {
		damage = attack.Damage
	}
	if attack != nil && attack.Projectile {
		// A thing that flies, with this enemy's damage in it and this enemy as
		// its owner: it settles on whatever it lands on, which may be the
		// player, may be the guard standing in the way, and may be a wall.
		g.fireProjectile(muzzle, direction, damage, actor.object)
		return
	}
	// A shot that lands at once. The puff goes where the ray stops so a miss
	// marks the wall behind the player rather than nothing at all.
	g.landEnemyHitscan(muzzle, direction, playerPos, damage)
}

// hostileDefaultDamage is what an attack whose kind named no number does. A
// fifth of the bar, so an enemy left alone is a real problem in five hits.
const hostileDefaultDamage = 20

// attackInterval is this one's own wait between attacks.
func (a *Actor) attackInterval() float64 {
	if a.kind != nil && a.kind.Attack != nil && a.kind.Attack.Interval > 0 {
		return a.kind.Attack.Interval
	}
	return hostileAttackInterval
}

// landEnemyHitscan is an enemy's shot arriving. The ray is cast to find what it
// hit so the impact is drawn somewhere real; whether the player is hurt is the
// clear line to them, not the ray, because rayCast deliberately skips the
// player - it is the picking ray as well as the shooting one.
func (g *Game) landEnemyHitscan(muzzle, direction, playerPos mgl32.Vec3, damage int) {
	reach := playerPos.Sub(muzzle).Len()
	hit := g.rayCastIgnoringHolograms(muzzle, direction)
	if hit.HitInfo.Hit && hit.HitInfo.Distance < float64(reach) {
		// Something solid stopped it short of the player: mark that instead.
		g.createImpactEffects(nil, wep.Op{Puff: wep.PuffBullet}, hit.HitObject,
			hit.WorldPosition(), hit.HitInfo.Side.AsDirection().ToVec3())
		return
	}
	g.player.TakeDamage(Damage{Amount: damage})
}

// patrol is the ordinary beat: the next point on it, once the last one has been
// reached. It deliberately does not switch the pathing component back on - a
// companion that has been told to wait here has had it switched off on purpose,
// and this is the call every actor makes every frame.
func (g *Game) patrol(actor *Actor) {
	if len(actor.patrol) < 2 || !actor.pathing.HasArrived() {
		return
	}
	actor.patrolIndex = (actor.patrolIndex + 1) % len(actor.patrol)
	actor.pathing.SetDestination(actor.patrol[actor.patrolIndex])
}

// hasLineOfSight is whether a straight line from one point to the other is
// clear of walls. Only walls: an enemy is not stopped by a barrel or by the
// guard standing next to it, which is the rule both source games use and the
// only one that does not turn a crowd into a queue.
func (g *Game) hasLineOfSight(from, to mgl32.Vec3) bool {
	// Both ends lift to roughly chest height, or a shot from something standing
	// on the floor is a shot into the floor.
	from = from.Add(mgl32.Vec3{0, eyeHeightOffset, 0})
	to = to.Add(mgl32.Vec3{0, eyeHeightOffset, 0})
	away := to.Sub(from)
	reach := away.Len()
	if reach < 0.001 {
		return true
	}
	direction := away.Mul(1 / reach)
	hit := voxel.DDARaycast(from, direction, float64(reach), func(x, y, z int32) bool {
		return g.CubeMap.IsSolidBlockAt(x, y, z)
	})
	return !hit.Hit || hit.Distance >= float64(reach)
}

// eyeHeightOffset lifts a line of sight off the floor an actor's collider is
// measured from.
const eyeHeightOffset = 0.6
