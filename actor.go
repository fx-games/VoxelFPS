package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// actorNames is which map entity names spawn an Actor rather than a static
// prop. A map says "Scientist"; this is the only place that has to know what
// that means.
var actorNames = map[string]bool{
	"Scientist": true,
}

// defaultActorHealth backs both the fall-damage pool and bodyShotsToKill: a
// default NPC's whole health bar is sized so that two ordinary body hits (see
// TakeDamage) empty it.
const defaultActorHealth = 100

// bodyShotsToKill is how many ordinary hits to the body a default NPC
// survives before dying, regardless of the weapon's own damage number - a
// headshot always kills in one hit instead. See TakeDamage.
const bodyShotsToKill = 2

// Actor is an NPC or monster: something in the world that walks its own beat
// rather than sitting where it spawned. It is deliberately not the thing a
// weapon spawns (a muzzle puff, a shell casing, a projectile) - see
// weapon_effect.go's SpawnedEffect for that, which used to be called
// WorldActor and get confused with this.
type Actor struct {
	Name      string
	Health    int
	MaxHealth int

	// Dialogue is the root line of this actor's conversation tree, or nil for
	// an actor nobody can talk to (every monster today). See dialogueFor and
	// GuiDialogue.
	Dialogue *quads.DialogueLine

	object        *quads.GameObject
	pathing       *quads.PathingComponent
	physicsSystem *quads.PhysicsSystem

	// dying is set the instant Health first drops to zero or below, whether
	// from a shot or a fall (see die()). It is the guard that stops a dead
	// actor from doing anything else ever again: TakeDamage refuses further
	// hits, updateActors refuses to re-trigger the death sequence, and
	// die() itself has already stopped the pathing component that would
	// otherwise keep walking a corpse toward its last patrol point.
	dying bool

	// following marks this actor as the player's companion: it walks after the
	// player instead of its patrol beat, and shoots whatever the player last
	// shot. Set and cleared from its own dialogue tree - see dialogueFor.
	following bool

	// repathClock and attackClock are the two cooldowns a companion runs on:
	// how often it re-aims its path at the player, and how often it fires. See
	// followPlayer and attack for why neither may happen every frame.
	repathClock float64
	attackClock float64

	// patrol is the beat this actor walks, one leg to the next and back to the
	// first, forever.
	//
	// ponytail: always exactly two points (spawn and PatrolTo), because that is
	// all the map format authors today. A real patrol route with several stops
	// is a slice with more than two entries and updateActors already loops it
	// correctly - it just has nothing today that writes a longer one.
	patrol      []mgl32.Vec3
	patrolIndex int

	// fallOrigin is the Y the actor was at when it was last grounded, tracked
	// while airborne so a landing can tell how far the drop was. wasGrounded is
	// what applyFallDamage uses to notice the grounded -> airborne -> grounded
	// round trip a landing is - see applyFallDamage.
	fallOrigin  float32
	wasGrounded bool
}

// spawnActor turns a plain entity GameObject into an Actor: a pathing
// component driving it, and a patrol beat to walk. object and physics are
// spawnEntity's own work already done; this only adds what makes it an actor
// rather than a prop.
func (g *Game) spawnActor(object *quads.GameObject, physics *quads.PhysicsComponent, entity core.MapEntity, spawnPos mgl32.Vec3) *Actor {
	if sprite := object.Sprite(); sprite != nil {
		atlas := sprite.AtlasTexture
		dyingAABB := voxel.NewAABB(
			atlas.AABBFromOpaqueAtIndex(int(g.Sprites.GetLastFrameIndex(int(quads.AnimDying), 0))),
		)
		physics.SetAABBForAnimation(quads.AnimDying, dyingAABB)
	}

	pather := quads.NewPathingComponent()
	object.SetComponent(pather)

	farEnd := mgl32.Vec3{5.5, 1, 2.5}
	if entity.PatrolTo != nil {
		farEnd = *entity.PatrolTo
	}

	actor := &Actor{
		Name:          entity.Name,
		Health:        defaultActorHealth,
		MaxHealth:     defaultActorHealth,
		object:        object,
		pathing:       pather,
		physicsSystem: g.physicsSystem,
		patrol:        []mgl32.Vec3{spawnPos, farEnd},
		patrolIndex:   1,
		fallOrigin:    spawnPos.Y(),
		wasGrounded:   true,
	}
	actor.Dialogue = dialogueFor(actor)
	pather.SetDestination(farEnd)

	g.Pathfinder.AddMovingObject(pather)
	g.actors = append(g.actors, actor)
	return actor
}

// updateActors advances every actor's patrol: one that has arrived is given
// the next point on its beat. Everything about how it gets there -
// pathfinding, movement, gravity, ladders - is quads.PathfindingSystem's job;
// this only decides where "there" is next, which is cheap enough to do for a
// map's whole population of actors every frame.
func (g *Game) updateActors(elapsed float64) {
	for _, actor := range g.actors {
		if actor.dying || !actor.object.IsAlive() {
			continue
		}
		applyFallDamage(actor)
		if actor.Health <= 0 {
			actor.die()
			continue
		}
		if actor.following {
			g.updateCompanion(actor, elapsed)
			continue
		}
		if len(actor.patrol) < 2 || !actor.pathing.HasArrived() {
			continue
		}
		actor.patrolIndex = (actor.patrolIndex + 1) % len(actor.patrol)
		actor.pathing.SetDestination(actor.patrol[actor.patrolIndex])
	}
}

// actorFor finds the Actor behind a GameObject a raycast or collision handed
// back, or nil for anything that is not one (a wall, a prop, the player). A
// linear scan of the actor population is fine here - this only ever runs once
// per hit landed, not once a frame.
func (g *Game) actorFor(object *quads.GameObject) *Actor {
	for _, actor := range g.actors {
		if actor.object == object {
			return actor
		}
	}
	return nil
}

// Everything a companion does, in numbers. followDistance is how close it
// keeps to the player - near enough to be there, far enough not to stand in
// the shot. repathInterval is how often it re-aims its path at the player: a
// path is a whole A* search, so this is a rate limit, not a preference, and
// four a second is smooth enough to follow someone walking. engageRange is
// how far away a fight is still the companion's business, and attackInterval
// how often it fires at one.
const (
	companionFollowDistance = 3.0
	companionRepathInterval = 0.25
	companionEngageRange    = 20.0
	companionAttackInterval = 1.2
	// companionAttackDamage is one ordinary body hit's worth on a default
	// actor, so a companion needs bodyShotsToKill of them the same as the
	// player does. See TakeDamage - the shot is routed through the player's
	// own hitscan path, which decides head vs. body itself.
	companionAttackDamage = defaultActorHealth / bodyShotsToKill
)

// follow hires this actor as the player's companion, from its own dialogue.
// Both clocks start expired so it sets off after the player, and shoots,
// on the very next frame rather than up to a cooldown later.
func (a *Actor) follow() {
	a.following = true
	a.repathClock = 0
	a.attackClock = 0
}

// stopFollowing dismisses a companion: it stands where it was left. Dropping
// the patrol is what makes that stick - updateActors walks the beat of any
// actor that still has one, so a dismissed companion handed its old route
// back would turn round and walk off across the map, and "wait here" means
// here.
func (a *Actor) stopFollowing() {
	a.following = false
	a.patrol = nil
	a.halt()
}

// updateCompanion is one frame of a companion: shoot what the player is
// fighting if it is still up and within reach, otherwise keep up with the
// player. Nothing here decides *who* the enemy is - g.companionTarget is
// whoever the player last shot, see damage().
func (g *Game) updateCompanion(actor *Actor, elapsed float64) {
	target := g.companionTarget
	if target != nil && (target.dying || target.following || !target.object.IsAlive()) {
		target, g.companionTarget = nil, nil
	}
	if target != nil {
		center := actor.object.Physics().AABB().Center()
		targetCenter := target.object.Physics().AABB().Center()
		if flatDistance(center, targetCenter) <= companionEngageRange {
			g.companionAttack(actor, center, targetCenter, elapsed)
			return
		}
	}
	// A companion that walks out of a fight starts the next one fresh rather
	// than with a cooldown left over from the last shot it got off.
	actor.attackClock = 0
	g.followPlayer(actor, elapsed)
}

// companionAttack holds the companion still, faces it at the target and fires
// on the cooldown. The shot is g.fireHitscanWith - the same call every one of
// the player's own hitscan weapons makes - so the impact puff, the blood and
// the damage all come out of the one path that already works.
func (g *Game) companionAttack(actor *Actor, center, targetCenter mgl32.Vec3, elapsed float64) {
	actor.halt()
	actor.object.Transform().SetLookAt2D(targetCenter)

	actor.attackClock -= elapsed
	if actor.attackClock > 0 {
		return
	}
	actor.attackClock = companionAttackInterval

	direction := targetCenter.Sub(center).Normalize()
	// The ray has to start clear of the shooter's own collider. rayCast takes
	// the nearest box in a cell, and a ray starting inside a box reports a
	// (negative, therefore nearest) hit on it - which is why the player is
	// skipped there by name. A companion is not, so it would shoot itself in
	// the chest every time.
	half := actor.object.Physics().AABB().HalfExtents()
	muzzle := center.Add(direction.Mul(max(half.X(), half.Z()) + 0.1))

	actor.object.Sprite().PlayAnimationInterrupting(int(quads.AnimFire))
	g.fireHitscanWith(nil, muzzle, direction, wep.Op{Damage: companionAttackDamage, Puff: wep.PuffBullet})
}

// followPlayer walks the companion after the player, and stops it once it is
// close enough. The re-path is on a clock because SetDestination throws the
// current route away and the pathfinder immediately runs a fresh A* for it:
// doing that every frame is a whole grid search per companion per frame, for
// a destination that has moved a few centimetres.
func (g *Game) followPlayer(actor *Actor, elapsed float64) {
	playerPos := g.player.Transform().GetPosition()
	if flatDistance(actor.object.Transform().GetPosition(), playerPos) <= companionFollowDistance {
		actor.halt()
		return
	}
	actor.repathClock -= elapsed
	if actor.repathClock > 0 {
		return
	}
	actor.repathClock = companionRepathInterval
	actor.pathing.SetDestination(playerPos)
}

// halt stops an actor where it stands. Nothing in the physics system slows a
// body down on its own - the pathfinder simply writes a velocity every frame
// while it has somewhere to be - so switching the pathing off is not enough
// to stop anything: the last velocity it wrote would carry the actor off
// across the room forever. Gravity goes back on because a halt part way up a
// ladder is a halt on a waypoint that had switched it off.
func (a *Actor) halt() {
	a.pathing.Enabled = false
	physics := a.object.Physics()
	physics.SetVelocity(mgl32.Vec3{0, physics.Velocity().Y(), 0})
	physics.ApplyGravity = true
	// Only out of the walk. A hit or a shot animation is playing over the top
	// of this and returns to whatever it interrupted by itself; cutting it
	// short here would mean a companion under fire never finished a frame of
	// anything.
	if a.object.Sprite().CurrentAnimation() == int(quads.AnimWalk) {
		a.object.Sprite().PlayAnimationLooping(int(quads.AnimIdle))
	}
}

// flatDistance is the distance between two points ignoring height: a
// companion one floor up is not standing next to you.
func flatDistance(a, b mgl32.Vec3) float32 {
	return mgl32.Vec2{a.X() - b.X(), a.Z() - b.Z()}.Len()
}

// radiusDamage is a blast: everyone still standing within radius of center
// takes exactly the amount named, not an automatic kill. A_Explode's own
// number is what tells a real detonation (damage 67, falling off over five
// calls at wider and wider radii) apart from GrenadeAlerter's own Spawn
// override - the same op, fired once every fuse tick while the grenade is
// still in your hand, purely to give away its position with A_Explode(2, 240).
// Force-killing on either call meant the alerter's own harmless poke wiped
// out anyone nearby the instant the grenade was thrown, well before it
// actually went off.
func (g *Game) radiusDamage(center mgl32.Vec3, radius float32, amount int) {
	for _, actor := range g.actors {
		if actor.dying || !actor.object.IsAlive() {
			continue
		}
		if actor.object.Transform().GetPosition().Sub(center).Len() <= radius {
			actor.ApplyDamage(amount)
		}
	}
}

// TakeDamage is a shot landing on this actor. A headshot always kills outright;
// an ordinary body hit costs a fixed fraction of MaxHealth so that exactly
// bodyShotsToKill of them empty the health pool regardless of the weapon's own
// damage number - a default NPC's toughness is a design constant, not a
// function of whatever gun is pointed at it. Once dying, further hits are
// ignored: there is nothing left to kill twice.
func (a *Actor) TakeDamage(headshot bool) {
	if a.dying {
		return
	}
	if headshot {
		a.Health = 0
	} else {
		a.Health -= a.MaxHealth / bodyShotsToKill
	}
	if a.Health <= 0 {
		a.die()
		return
	}
	a.object.Sprite().PlayAnimationInterrupting(int(quads.AnimHit))
}

// ApplyDamage takes a flat amount off Health, for a source that already knows
// its own number instead of the fixed body/headshot split TakeDamage assumes -
// a blast radius, most likely, where a nearer call and a farther one carry
// genuinely different damage rather than "shot" vs "aimed shot".
func (a *Actor) ApplyDamage(amount int) {
	if a.dying || amount <= 0 {
		return
	}
	a.Health -= amount
	if a.Health <= 0 {
		a.die()
		return
	}
	a.object.Sprite().PlayAnimationInterrupting(int(quads.AnimHit))
}

// die is the one place an actor's life ends, whatever killed it - a shot or a
// fall. It stops the pathing component (a corpse does not keep walking its
// patrol) and takes the physics collider off the grid entirely (no more
// blocking, no more being picked by a raycast - see
// PhysicsSystem.RemoveObject), then freezes the sprite on its last dying
// frame. It deliberately does NOT call GameObject.Kill(): that also drops a
// sprite out of SpriteDrawer's draw list, which is what used to make a dead
// actor vanish outright instead of leaving a body on the ground.
func (a *Actor) die() {
	a.dying = true
	a.pathing.Enabled = false
	a.physicsSystem.RemoveObject(a.object.Physics())
	a.object.Sprite().PlayAnimationFreezeLastFrame(int(quads.AnimDying))
}

// fallDamageMinDrop is how far a drop has to be before it hurts at all: a
// stair step or the lip of a kerb is not a fall. fallDamagePerVoxel is health
// lost for every voxel past that - four voxels free, then it adds up fast,
// the way a fall down a stairwell in Doom is a shrug and off the roof is not.
const (
	fallDamageMinDrop  = 3.0
	fallDamagePerVoxel = 15
)

// applyFallDamage tracks the Y an actor was last resting at and, the moment
// it lands again, charges it for how far below that the landing was. Ladders
// do not count: PathfindingSystem.Update turns gravity off for the climb
// waypoint, so IsGrounded never goes false while climbing down one - only an
// actual fall (off an unguarded edge, a shove, however it happens) leaves the
// ground under an actor's own gravity.
func applyFallDamage(actor *Actor) {
	physics := actor.object.Physics()
	pos := actor.object.Transform().GetPosition()
	grounded := physics.IsGrounded()

	if !grounded {
		if actor.wasGrounded {
			actor.fallOrigin = pos.Y()
		}
		actor.wasGrounded = false
		return
	}

	if !actor.wasGrounded {
		drop := actor.fallOrigin - pos.Y()
		if drop > fallDamageMinDrop {
			actor.Health -= int((drop - fallDamageMinDrop) * fallDamagePerVoxel)
		}
	}
	actor.wasGrounded = true
}
