package main

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/audio"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/wep"
)

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

	// touring marks this actor as a guide leading the player somewhere: it
	// walks its own patrol once, leg by leg, instead of looping it forever,
	// and waits at each leg for the player to catch up - see startTour and
	// updateTour. Set from its own dialogue tree, the same way following is.
	touring bool

	// repathClock and attackClock are the two cooldowns a companion runs on:
	// how often it re-aims its path at the player, and how often it fires. See
	// followPlayer and attack for why neither may happen every frame.
	repathClock float64
	attackClock float64

	// patrol is the beat this actor walks, one leg to the next and back to the
	// first, forever. The first point is always where the actor spawned; the
	// rest are the map's own route - see patrolRoute and the editor's path
	// tool, which appends to this slice live while a map is being built.
	patrol      []mgl32.Vec3
	patrolIndex int

	// game is who to tell when this actor dies, because a death cry is played
	// from the one place that knows where the player is standing. It is nil for
	// an actor built in a test, which is why playActorCue tolerates one.
	game *Game

	// kind is the folder this actor was spawned from: its sheet, its
	// animations, and the cues it makes a noise with. Nil for an actor placed
	// before there were sprite kinds, which is why every read of it is guarded.
	kind *SpriteKind

	// origin is this actor's own sound channel owner. Channels belong to the
	// thing making the sound, so one guard's death cry cannot cut off another's
	// - see audio.CueOptions.Origin.
	origin int

	// hostile is whether this one comes after the player, off its own kind.
	// hunting is whether it currently is - see hostile.go.
	hostile bool
	hunting bool

	// noticed is whether this one has already called out. Both games these
	// characters come from bark once, when they first see you, and then get on
	// with it - so this is latched rather than checked, or a guard standing at
	// the edge of earshot shouts every frame.
	noticed bool

	// fallOrigin is the Y the actor was at when it was last grounded, tracked
	// while airborne so a landing can tell how far the drop was. wasGrounded is
	// what applyFallDamage uses to notice the grounded -> airborne -> grounded
	// round trip a landing is - see applyFallDamage.
	fallOrigin  float32
	wasGrounded bool

	// glitchClock counts down to this actor's next display error, for a kind
	// that has one - see SpriteKind.Glitch and updateHologramGlitch.
	glitchClock float64
}

// spawnActor turns a plain entity GameObject into an Actor: a pathing
// component driving it, and a patrol beat to walk. object and physics are
// spawnEntity's own work already done; this only adds what makes it an actor
// rather than a prop.
func (g *Game) spawnActor(object *quads.GameObject, physics *quads.PhysicsComponent, entity core.MapEntity, spawnPos mgl32.Vec3) *Actor {
	if sprite := object.Sprite(); sprite != nil && !physics.HasAABBForAnimation(quads.AnimDying) {
		// A corpse is a different shape from the thing that was standing there,
		// so the box it is picked and stepped over by comes off the frame it
		// comes to rest on - measured the same way and at the same size as the
		// one it walked around with.
		//
		// Only for a kind that did not bring one. An imported folder writes a
		// box per animation, and its death's is measured off the same last
		// frame at import time rather than off the card at every spawn.
		corpse := g.Sprites.GetLastFrameIndex(sprite.Global(int(quads.AnimDying)), 0)
		physics.SetAABBForAnimation(quads.AnimDying,
			quads.SpriteAABB(sprite.AtlasTexture, int(corpse), sprite.Size))
	}

	pather := quads.NewPathingComponent()
	object.SetComponent(pather)

	patrol := patrolRoute(entity, spawnPos)

	// How tough this one is, and what it sounds like, are the folder's own -
	// see assets/sprites. A kind that says nothing gets the default, which is
	// what every actor had before a folder could say otherwise.
	kind := g.spriteKinds[entity.SpriteSheetName]
	health := defaultActorHealth
	hostile := false
	if kind != nil {
		if kind.Health > 0 {
			health = kind.Health
		}
		hostile = kind.Hostile
	}
	g.lastActorOrigin++

	actor := &Actor{
		Name:          entity.Name,
		game:          g,
		kind:          kind,
		hostile:       hostile,
		origin:        g.lastActorOrigin,
		Health:        health,
		MaxHealth:     health,
		object:        object,
		pathing:       pather,
		physicsSystem: g.physicsSystem,
		patrol:        patrol,
		patrolIndex:   1,
		fallOrigin:    spawnPos.Y(),
		wasGrounded:   true,
	}
	actor.Dialogue = dialogueFor(actor)
	// Only if anything is going to walk it, and not for a guide - see
	// SpriteKind.Guide. SetDestination switches the pathing component on, and
	// a level of actors that were told where to go before the switch was ever
	// consulted is a level of actors all walking to the same corner; a guide
	// gets its first destination from its own dialogue instead, see startTour.
	if g.actorsActive && (kind == nil || !kind.Guide) {
		pather.SetDestination(patrol[1])
	}

	g.Pathfinder.AddMovingObject(pather)
	g.actors = append(g.actors, actor)
	return actor
}

// The three noises an actor makes, by the name its folder keys them under.
const (
	actorCueAlert  = "alert"
	actorCueAttack = "attack"
	actorCueDeath  = "death"
)

// actorHearingRange is how far one of these carries. Past it nothing is played
// at all, rather than played at nothing: a room of guards each barking at
// inaudible volume is still a room of guards each holding a mixer open.
const actorHearingRange = 32.0

// actorNoticeRange is how close the player has to come before an actor calls
// out. Shorter than earshot, so the bark is a thing that happens to you rather
// than a wall of voices the moment a level loads.
const actorNoticeRange = 14.0

// playActorCue is an actor making a noise, at the volume and the side its
// distance from the player earns it. There is no 3D mixer here - a cue carries
// a volume and a pan and nothing else - so the falloff is worked out at the one
// place a world sound is played from.
func (g *Game) playActorCue(actor *Actor, role string) {
	if g == nil || actor == nil || actor.kind == nil || g.audio == nil || g.player == nil {
		return
	}
	cue := actor.kind.Sounds[role]
	if cue == "" {
		return
	}
	at := actor.object.Transform().GetPosition()
	listener := g.player.Transform().GetPosition()
	away := at.Sub(listener)
	distance := away.Len()
	if distance > actorHearingRange {
		return
	}
	volume := 1 - float64(distance)/actorHearingRange
	pan := 0.0
	if distance > 0.01 {
		view := g.Camera.GetTransform()
		pan = float64(away.Normalize().Dot(view.GetRight()))
	}
	g.audio.PlayCueAt(actor.kind.ID, cue, audio.CueOptions{
		Volume: volume,
		Pan:    pan,
		Origin: actor.origin,
	})
}

// hologramGlitchPeriod is how long a hologram stays steady between display
// errors.
const hologramGlitchPeriod = 2.6

// updateHologramGlitch is a signal failure: every hologramGlitchPeriod, on a
// clock like attackClock's rather than every frame, the sprite is thrown into
// its own kind's "fire" slot - which for a Glitch kind is not a weapon but
// five frames of torn, static-shot, ghosted and blown-out art (see
// assets/sprites/actors/humans/scientist_hologram) - and PlayAnimationInterrupting
// resumes whatever it was doing (walking, standing) the moment those frames run
// out. A hologram that just went transparent for an instant would read as an
// engine bug; this reads as the projector's.
func (g *Game) updateHologramGlitch(actor *Actor, elapsed float64) {
	actor.glitchClock -= elapsed
	if actor.glitchClock > 0 {
		return
	}
	actor.glitchClock = hologramGlitchPeriod
	actor.object.Sprite().PlayAnimationInterrupting(int(quads.AnimFire))
}

// patrolRoute is the beat the map asked for: the whole route the editor's path
// tool wrote, the one far end the older PatrolTo field names, or - for an entity
// that says nothing - a default leg, so that an actor dropped into a map with no
// route at all still walks rather than standing on its spawn forever.
//
// The route always starts at the spawn, so the loop closes back to where the
// actor stands, and always has a second point for updateActors to head for.
func patrolRoute(entity core.MapEntity, spawnPos mgl32.Vec3) []mgl32.Vec3 {
	if len(entity.Patrol) > 0 {
		return append([]mgl32.Vec3{spawnPos}, entity.Patrol...)
	}
	farEnd := mgl32.Vec3{5.5, 1, 2.5}
	if entity.PatrolTo != nil {
		farEnd = *entity.PatrolTo
	}
	return []mgl32.Vec3{spawnPos, farEnd}
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
		if actor.kind != nil && actor.kind.Glitch {
			g.updateHologramGlitch(actor, elapsed)
		}
		// A guide leading a tour is doing something the player explicitly
		// asked for through its own dialogue, not wandering or hunting on its
		// own initiative - the "Actors Active" switch below is for freezing
		// *that*, so it never gets a say over a tour already under way.
		if actor.touring {
			// Not while the conversation that started it is still on the
			// screen. openDialogue is modal for the player but the world
			// carries on updating behind it, so a guide agreed to on a reply
			// used to walk the whole route - all of thirty metres of it - in
			// the seconds the player spent reading the line that set it off,
			// and was standing at the far end, tour already finished, by the
			// time they closed it. It looked like a guide that would not go on
			// its own and had to be asked again at every stop; what asking
			// again did was start the route over. The player cannot move while
			// a modal screen is up (see setupModal), and a guide that waits
			// for the player at every leg has no business walking one while
			// they are held still.
			if !g.widgetsAreBlockingMovement {
				g.updateTour(actor, elapsed)
			}
			continue
		}
		// A guide is never an ordinary actor, tour or no tour. Its route is a
		// straight line between hand-picked points rather than anything the
		// pathfinder found (see updateTour), and updateTour never promised a
		// floor under the point it leaves it standing on - the ark's own tours
		// both end in mid-air in the spine shaft, at the foot of the ladder up.
		// Everything below here would either switch gravity back on (freeze,
		// through halt) and drop it down that shaft, or hand its tour route to
		// the pathfinder as a patrol beat and walk it back through the wall it
		// came in by.
		if actor.kind != nil && actor.kind.Guide {
			continue
		}
		// Inactive actors stand where they are: no patrol, no chase, no
		// attack. It has to switch the pathing off rather than just skip the
		// rest of this - deciding where to walk is this function's, but the
		// walking itself is the pathfinder's, and it will carry on to the last
		// destination anything set for as long as the component is enabled.
		// See actor_debug.go.
		if !g.actorsActive {
			actor.freeze()
			continue
		}
		if actor.following {
			g.updateCompanion(actor, elapsed)
			continue
		}
		// A hostile has a mind of its own - notice, chase, attack - and it is
		// the one that decides when to walk its beat. Everything else just
		// walks it.
		if actor.hostile {
			g.updateHostile(actor, elapsed)
			continue
		}
		g.noticePlayer(actor)
		g.patrol(actor)
	}
}

// noticePlayer is an actor calling out the first time the player comes close
// enough. It is latched on the actor rather than on the sound, because the cue
// itself has no memory and the point is that a guard greets you once.
func (g *Game) noticePlayer(actor *Actor) {
	if actor.noticed || actor.kind == nil || actor.kind.Sounds[actorCueAlert] == "" {
		return
	}
	if g.player == nil {
		return
	}
	if flatDistance(actor.object.Transform().GetPosition(), g.player.Transform().GetPosition()) > actorNoticeRange {
		return
	}
	actor.noticed = true
	g.playActorCue(actor, actorCueAlert)
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

// tourWaitDistance is how close the player has to be before a touring guide
// takes its next step - see updateTour. Loose enough that a player who sets
// off the moment the guide does is still within it at the end of a leg twice
// this long: the guide waits for you, it does not make you sprint.
const tourWaitDistance = 9.0

// tourWalkSpeed is how fast a guide leading a tour crosses a room - the same
// brisk pace quads.PathingComponent's own default keeps an ordinary patrol to.
const tourWalkSpeed = 2.5

// tourArrivalRadius is how close counts as "there" - see the pathing
// package's own waypointArrivalRadius, which this mirrors.
const tourArrivalRadius = 0.2

// startTour sends a guide off on the route its map entity was given, kicked
// off by its own dialogue rather than the moment it spawns - see
// SpriteKind.Guide. It is a no-op for a guide with nowhere to go: patrol
// always has at least the spawn point in it, so one leg is what "no route"
// looks like.
func (a *Actor) startTour() {
	if len(a.patrol) < 2 {
		return
	}
	a.touring = true
	a.patrolIndex = 1
	a.object.Sprite().PlayAnimationLooping(int(quads.AnimWalk))
}

// tourIdle stops a touring guide's walk animation between legs, without
// actor.halt's other side effects: switching the pathing component off (a
// touring guide never turns it on to begin with - see startTour) and,
// critically, switching gravity back on. A guide waiting mid-route stands
// exactly where its own waypoint put it, on a floor updateTour never checks
// because it never needs to - see its own comment on why that floor cannot
// be trusted here at all. halt() turning gravity on every time a guide
// paused to wait was dropping it through the nearest gap - the ark's own
// ladder shafts among them - the moment the player fell behind by even one
// leg, which is what made the next leg look like it walked through a wall:
// it was above the deck it started on when it first found the wall solid,
// and now under it.
func (a *Actor) tourIdle() {
	if a.object.Sprite().IsPlaying(int(quads.AnimWalk)) {
		a.object.Sprite().PlayAnimationLooping(int(quads.AnimIdle))
	}
}

// waypointText is the guide answering "where are we going?", from wherever it
// has got to on its route. The route's points have no names - they are bare
// positions off the ark's own floor plan, see assets/maps/ark/build.map - so
// which stop of how many, and how far off it is, is the whole of what it can
// honestly say. patrol's first point is the spawn rather than a stop, which is
// why the count is one short of its length and patrolIndex reads directly as
// the number of the stop being walked to.
func (a *Actor) waypointText() string {
	stops := len(a.patrol) - 1
	switch {
	case stops < 1:
		return "I'm not going anywhere. This is the whole of my route."
	case !a.touring && a.patrolIndex >= stops:
		return fmt.Sprintf("Stop %d of %d - the last of them. The route ends here.", stops, stops)
	case !a.touring:
		return fmt.Sprintf("We haven't set off yet. It's %d stops from here once we do.", stops)
	}
	away := flatDistance(a.object.Transform().GetPosition(), a.patrol[a.patrolIndex])
	if away <= tourArrivalRadius {
		return fmt.Sprintf("Stop %d of %d. I'll wait here until you've caught up.", a.patrolIndex, stops)
	}
	return fmt.Sprintf("On our way to stop %d of %d, about %.0f metres ahead.", a.patrolIndex, stops, away)
}

// updateTour is a guide leading a tour: it walks a straight line to the next
// point on its own route rather than handing the destination to
// quads.PathfindingSystem, the way an ordinary patrol does. That system finds
// a deck's own floor by scanning straight up from y=0 (see its groundHeight) -
// a column that, eleven decks up a hull built round a central spine, is
// vacuum at y=0 and never solid at all. Every hostile and every companion on
// every other map goes through it just fine; a guide standing on the ark
// cannot, because its own starting column already fails that scan.
//
// ponytail: straight-line movement, no A* and no collision check against the
// route itself - correct here because every route is chosen off the ark's own
// floor plan (see assets/maps/ark/build.map) to be a clear, flat, single-deck
// corridor. A tour that had to turn a corner, dodge furniture or change decks
// would need groundHeight's assumption fixed instead of a workaround here.
//
// It only leaves for its next leg once it has arrived at this one *and* the
// player is close enough to have followed - a guide that walked on regardless
// would end up talking to an empty corridor. Unlike an ordinary patrol (see
// patrol, in hostile.go), the route runs once and stops at the far end
// instead of looping back to the start.
func (g *Game) updateTour(actor *Actor, elapsed float64) {
	transform := actor.object.Transform()
	position := transform.GetPosition()
	target := actor.patrol[actor.patrolIndex]
	toTarget := mgl32.Vec3{target.X() - position.X(), 0, target.Z() - position.Z()}
	distance := toTarget.Len()

	if distance <= tourArrivalRadius {
		if actor.patrolIndex >= len(actor.patrol)-1 {
			actor.touring = false
			actor.tourIdle()
			return
		}
		if g.player == nil || flatDistance(position, g.player.Transform().GetPosition()) > tourWaitDistance {
			actor.tourIdle()
			return
		}
		actor.patrolIndex++
		actor.object.Sprite().PlayAnimationLooping(int(quads.AnimWalk))
		return
	}

	transform.SetLookAt2D(target)
	step := float32(tourWalkSpeed) * float32(elapsed)
	if step > distance {
		step = distance
	}
	// Through the physics system rather than the transform: the collider has
	// to be re-listed on the grid it is picked out by - see Teleport.
	actor.physicsSystem.Teleport(actor.object.Physics(), position.Add(toTarget.Normalize().Mul(step)))
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
	g.playActorCue(actor, actorCueAttack)
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
	a.freeze()
	// Only out of the walk. A hit or a shot animation is playing over the top
	// of this and returns to whatever it interrupted by itself; cutting it
	// short here would mean a companion under fire never finished a frame of
	// anything.
	if a.object.Sprite().IsPlaying(int(quads.AnimWalk)) {
		a.object.Sprite().PlayAnimationLooping(int(quads.AnimIdle))
	}
}

// freeze is halt's movement half, without touching the sprite: the pathing off,
// the velocity it last wrote cleared, gravity back on.
//
// It is separate because an inactive actor is frozen every frame - see
// updateActors - and the F1 menu's actor page exists to play an animation on
// one while it stands there. Freezing through halt would put every one of them
// back on its idle a frame after the menu asked for a walk.
func (a *Actor) freeze() {
	a.pathing.Enabled = false
	physics := a.object.Physics()
	physics.SetVelocity(mgl32.Vec3{0, physics.Velocity().Y(), 0})
	physics.ApplyGravity = true
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
			actor.TakeDamage(Damage{Amount: amount})
		}
	}
}

// Object and Alive are Combatant, alongside TakeDamage below.
func (a *Actor) Object() *quads.GameObject { return a.object }

func (a *Actor) Alive() bool { return a != nil && !a.dying && a.object.IsAlive() }

// TakeDamage is a hit landing on this actor.
//
// A headshot always kills outright. A hit that carries its own number - a
// blast, a bolt with damage in it - costs that many points. A hit with no
// number is an ordinary body shot, and costs a fixed fraction of MaxHealth so
// that exactly bodyShotsToKill of them empty the health pool regardless of the
// weapon's own damage: a default NPC's toughness is a design constant, not a
// function of whatever gun is pointed at it.
//
// Once dying, further hits are ignored: there is nothing left to kill twice.
// A hologram (SpriteKind.Hologram) ignores every hit outright, dying or not -
// light does not bleed, and there is nothing under it to empty a health bar.
func (a *Actor) TakeDamage(hit Damage) {
	if a.dying || (a.kind != nil && a.kind.Hologram) {
		return
	}
	switch {
	case hit.Headshot:
		a.Health = 0
	case hit.Amount > 0:
		a.Health -= hit.Amount
	default:
		a.Health -= a.MaxHealth / bodyShotsToKill
	}
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
	if a.game != nil {
		a.game.playActorCue(a, actorCueDeath)
	}
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
			// Through TakeDamage like everything else, so a landing that kills
			// runs the same death a shot does. The floor is nobody's shot, so
			// it carries no Source.
			actor.TakeDamage(Damage{Amount: int((drop - fallDamageMinDrop) * fallDamagePerVoxel)})
		}
	}
	actor.wasGrounded = true
}
