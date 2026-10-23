package main

// What it means to be shootable, for both of the things that are.
//
// Until enemies shot back there was only one of them: an NPC had health and the
// player did not, so "damage" was a method on Actor and nothing else. Now a
// bolt in flight can land on either, and the thing it lands on is decided by a
// physics collision that knows nothing about NPCs - so both sides of it are
// reached the same way, through Combatant.

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
)

// Damage is one hit arriving at something that can be hurt.
type Damage struct {
	// Amount is what it costs, in points. Zero means an ordinary shot with no
	// number of its own, which each target charges itself for at its own rate -
	// see Actor.TakeDamage. It is not "no damage": a hit that should cost
	// nothing is a hit that is not sent.
	Amount int
	// Headshot is a hit somewhere around the head, which kills outright. The
	// shooter decides this, not the target: it is where on the collider the
	// hit landed, which only the thing that cast the ray knows.
	Headshot bool
	// Source is what fired it, or nil for the world - a fall, a blast with
	// nobody left standing behind it. It is the GameObject rather than the
	// Combatant because a projectile carries it through the physics system,
	// which deals in objects.
	Source *quads.GameObject
}

// Combatant is anything in the world with health: an NPC, or the player.
//
// It is deliberately three methods and not a health bar's worth. Nothing reads
// a combatant's health through this - the HUD knows it is drawing the player's,
// and an NPC's is read off the Actor - so putting it here would be an interface
// method to satisfy rather than to call.
type Combatant interface {
	// Object is what it is in the world, for whatever needs its position or
	// wants to know whether a collision was with this one.
	Object() *quads.GameObject
	// Alive is whether it can still be hurt. A corpse is not: it has an object
	// and a place in the world for as long as the level lasts.
	Alive() bool
	// TakeDamage is a hit landing. Everything that hurts anything goes through
	// here, which is why god mode and death are checked in one place each
	// rather than at every source.
	TakeDamage(Damage)
}

// Player is whoever the camera is behind. It is the GameObject everything in
// the world already deals with - embedded, so every g.player.Transform() and
// g.player.Physics() in the rest of the engine still reads the same - with the
// health that makes being shot at mean something.
type Player struct {
	*quads.GameObject

	// game is who to tell when the player dies, because standing back up is a
	// fact about the level rather than about the player.
	game *Game

	health int
}

// playerMaxHealth is the bar. A hundred because that is the scale both
// importers fit their own game's damage numbers onto - see the two of them.
const playerMaxHealth = 100

// newPlayer wraps a freshly spawned player object with a full bar.
func newPlayer(game *Game, object *quads.GameObject) *Player {
	return &Player{GameObject: object, game: game, health: playerMaxHealth}
}

// Object, Alive and TakeDamage are Combatant.
func (p *Player) Object() *quads.GameObject { return p.GameObject }

func (p *Player) Alive() bool { return p != nil && p.health > 0 }

func (p *Player) TakeDamage(hit Damage) {
	if p == nil || p.health <= 0 || p.game.cheats.godMode {
		return
	}
	if hit.Headshot {
		p.health = 0
	} else {
		p.health -= p.damageFrom(hit)
	}
	if p.health > 0 {
		return
	}
	p.health = 0
	p.die()
}

// damageFrom is what one hit costs. A hit with no number of its own costs the
// same fraction of the bar an NPC's body shot costs it, which is the one rule
// both sides of a fight share.
func (p *Player) damageFrom(hit Damage) int {
	if hit.Amount > 0 {
		return hit.Amount
	}
	return playerMaxHealth / bodyShotsToKill
}

// Health and MaxHealth are what the HUD draws. They are not on Combatant - see
// the note there.
func (p *Player) Health() int    { return p.health }
func (p *Player) MaxHealth() int { return playerMaxHealth }

// Heal puts points back, never past the top.
func (p *Player) Heal(amount int) {
	p.health += amount
	if p.health > playerMaxHealth {
		p.health = playerMaxHealth
	}
}

// die puts the player back at the level's own start with a full bar. There is
// no death screen and no save to reload - this is a game being built, and
// standing up where you came in is the shortest thing that lets the fight carry
// on being tested.
func (p *Player) die() {
	p.health = playerMaxHealth
	game := p.game
	if game == nil {
		return
	}
	game.Toast("You died")
	p.Transform().SetPosition(game.mapSource.PlayerSpawn)
	if physics := p.Physics(); physics != nil {
		physics.SetVelocity(mgl32.Vec3{})
	}
	// Whatever was hunting has lost you: they are back where they were and so
	// are you, and a room full of enemies still mid-chase would shoot the
	// moment the screen came back.
	for _, actor := range game.actors {
		actor.hunting = false
		actor.noticed = false
	}
}

// combatantFor is what a GameObject handed over by a raycast or a collision
// turns out to be, or nil for a wall, a prop or a bolt in flight. It is the one
// place that knows the player is one of these too.
func (g *Game) combatantFor(object *quads.GameObject) Combatant {
	if object == nil {
		return nil
	}
	if g.player != nil && object == g.player.GameObject {
		return g.player
	}
	if actor := g.actorFor(object); actor != nil {
		return actor
	}
	return nil
}
