package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
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
		Dialogue:      dialogueFor(entity.Name),
		object:        object,
		pathing:       pather,
		physicsSystem: g.physicsSystem,
		patrol:        []mgl32.Vec3{spawnPos, farEnd},
		patrolIndex:   1,
		fallOrigin:    spawnPos.Y(),
		wasGrounded:   true,
	}
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
