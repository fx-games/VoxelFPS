package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// testActor builds a bare Actor good enough for TakeDamage/die to run without a
// whole game behind them: a real GameObject with a sprite (both
// death paths play an animation on it) and a zero-value PhysicsSystem, whose
// RemoveObject is a safe no-op for an object it never tracked.
func testActor(t *testing.T, health int, at mgl32.Vec3) *Actor {
	t.Helper()
	object := quads.NewGameObject("t")
	object.Transform().SetPosition(at)
	object.SetComponent(quads.NewSprite(nil, []uint16{0}))
	object.SetComponent(quads.NewPhysicsComponent(false, voxel.AABB{}))
	return &Actor{
		Health:        health,
		MaxHealth:     health,
		object:        object,
		pathing:       quads.NewPathingComponent(),
		physicsSystem: &quads.PhysicsSystem{},
	}
}

// This is the bug the user caught: radiusDamage used to deal an instant kill
// for every A_Explode in earshot, real detonation or not.
// GrenadeAlerter overrides FragGrenadeExplosion's Spawn state with its own,
// much smaller A_Explode(2, 240) (see assets/weapons/bd_frag_grenade/source.txt) and
// fires once every fuse tick while the grenade is still in flight, well
// before it actually lands and goes off - so a bystander three and a half
// units away died the instant a grenade was thrown, never mind whether it
// ever hit anything. A hit that carries its own Amount costs exactly that.
func TestDamageWithAnAmountTakesItRatherThanKillingOutright(t *testing.T) {
	alerterPoke := testActor(t, defaultActorHealth, mgl32.Vec3{})
	alerterPoke.TakeDamage(Damage{Amount: 2})
	if alerterPoke.dying {
		t.Fatalf("GrenadeAlerter's own 2-damage poke killed a %d-health actor outright", defaultActorHealth)
	}
	if alerterPoke.Health != defaultActorHealth-2 {
		t.Errorf("Health = %d, want %d", alerterPoke.Health, defaultActorHealth-2)
	}

	realBlast := testActor(t, defaultActorHealth, mgl32.Vec3{})
	realBlast.TakeDamage(Damage{Amount: 67})
	realBlast.TakeDamage(Damage{Amount: 53}) // FragGrenadeExplosion's own falloff calls stack
	if !realBlast.dying {
		t.Errorf("120 cumulative blast damage on a %d-health actor left it alive", defaultActorHealth)
	}

	dead := testActor(t, defaultActorHealth, mgl32.Vec3{})
	dead.dying = true
	dead.TakeDamage(Damage{Amount: 999})
	if dead.Health != defaultActorHealth {
		t.Errorf("a second blast changed a corpse's health to %d, want it left at %d", dead.Health, defaultActorHealth)
	}
}

// radiusDamage only reaches what the blast actually covers, and passes the
// real amount through rather than substituting a kill.
func TestRadiusDamageOnlyHitsActorsInsideTheBlast(t *testing.T) {
	g := &Game{}
	near := testActor(t, defaultActorHealth, mgl32.Vec3{1, 0, 0})
	far := testActor(t, defaultActorHealth, mgl32.Vec3{100, 0, 0})
	g.actors = []*Actor{near, far}

	g.radiusDamage(mgl32.Vec3{}, 5, 2)

	if near.Health != defaultActorHealth-2 {
		t.Errorf("actor inside the blast radius has %d health, want %d", near.Health, defaultActorHealth-2)
	}
	if far.Health != defaultActorHealth {
		t.Errorf("actor outside the blast radius took damage: %d health, want %d", far.Health, defaultActorHealth)
	}
}

// testGame is enough Game for the companion's follow logic: it only ever
// reads the player's position. Its combat half needs a whole GL world behind
// it (fireHitscanWith raycasts the voxel map) and is left to be looked at in
// the running game.
func testGame(t *testing.T, playerAt mgl32.Vec3) *Game {
	t.Helper()
	object := quads.NewGameObject("Player")
	object.Transform().SetPosition(playerAt)
	game := &Game{}
	game.player = newPlayer(game, object)
	return game
}

// A companion has to actually stop when it has caught up. Nothing in the
// physics system slows a body down on its own - the pathfinder writes a
// velocity every frame while it has somewhere to be, and the last one it
// wrote stands until something overwrites it - so "arrived" that only
// switches the pathing off leaves the companion coasting away across the
// room at walking pace, forever.
func TestCompanionStopsDeadOnceItHasCaughtUp(t *testing.T) {
	game := testGame(t, mgl32.Vec3{10, 1, 10})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{11, 1, 10})
	companion.follow()
	companion.object.Physics().SetVelocity(mgl32.Vec3{2, -5, 3})

	game.followPlayer(companion, 1.0/60.0)

	if companion.pathing.Enabled {
		t.Error("a companion standing next to the player is still being driven by the pathfinder")
	}
	if velocity := companion.object.Physics().Velocity(); velocity.X() != 0 || velocity.Z() != 0 {
		t.Errorf("horizontal velocity = %v, want it zeroed - the companion coasts off otherwise", velocity)
	}
	if velocity := companion.object.Physics().Velocity(); velocity.Y() != -5 {
		t.Errorf("Y velocity = %f, want the fall left alone at -5", velocity.Y())
	}
}

// The other half: out of reach, it walks after the player - but re-aiming is
// a whole A* search (SetDestination throws the current route away and the
// pathfinder immediately runs a fresh one), so it happens on a clock rather
// than every frame.
func TestCompanionWalksAfterTheDistantPlayerButRepathsOnAClock(t *testing.T) {
	game := testGame(t, mgl32.Vec3{20, 1, 20})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{2, 1, 2})
	companion.follow()

	game.followPlayer(companion, 1.0/60.0)
	if !companion.pathing.Enabled {
		t.Fatal("a companion left behind is not walking after the player")
	}
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{20, 1, 20}) {
		t.Fatalf("destination = %v, want the player's position", got)
	}

	// The player moves; the very next frame must not buy another search.
	game.player.Transform().SetPosition(mgl32.Vec3{24, 1, 24})
	game.followPlayer(companion, 1.0/60.0)
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{20, 1, 20}) {
		t.Errorf("re-pathed to %v the very next frame, want one search per %.2fs", got, companionRepathInterval)
	}

	// ...but once the clock is up, it follows the player to where they went.
	game.followPlayer(companion, companionRepathInterval)
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{24, 1, 24}) {
		t.Errorf("destination = %v after the re-path interval, want the player's new position", got)
	}
}

// "Wait here" means here. A dismissed companion handed its old patrol back
// would turn round and walk off to the far end of a beat it was hired away
// from half a map ago.
func TestDismissedCompanionStaysWhereItWasLeft(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{30, 1, 30})
	companion.patrol = []mgl32.Vec3{{1, 1, 1}, {40, 1, 40}}
	companion.follow()
	companion.stopFollowing()

	game.actors = []*Actor{companion}
	game.updateActors(1.0 / 60.0)

	if companion.following {
		t.Error("still following after being dismissed")
	}
	if companion.pathing.Enabled {
		t.Errorf("dismissed companion is walking to %v instead of waiting", companion.pathing.Destination())
	}
}
