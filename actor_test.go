package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// testActor builds a bare Actor good enough for TakeDamage/ApplyDamage/die to
// run without a whole game behind them: a real GameObject with a sprite (both
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

// This is the bug the user caught: radiusDamage used to call TakeDamage(true) -
// an instant kill - for every A_Explode in earshot, real detonation or not.
// GrenadeAlerter overrides FragGrenadeExplosion's Spawn state with its own,
// much smaller A_Explode(2, 240) (see weapons/bd_frag_grenade/source.txt) and
// fires once every fuse tick while the grenade is still in flight, well
// before it actually lands and goes off - so a bystander three and a half
// units away died the instant a grenade was thrown, never mind whether it
// ever hit anything. ApplyDamage takes exactly the number handed to it.
func TestApplyDamageTakesTheAmountGivenNotAnInstantKill(t *testing.T) {
	alerterPoke := testActor(t, defaultActorHealth, mgl32.Vec3{})
	alerterPoke.ApplyDamage(2)
	if alerterPoke.dying {
		t.Fatalf("GrenadeAlerter's own 2-damage poke killed a %d-health actor outright", defaultActorHealth)
	}
	if alerterPoke.Health != defaultActorHealth-2 {
		t.Errorf("Health = %d, want %d", alerterPoke.Health, defaultActorHealth-2)
	}

	realBlast := testActor(t, defaultActorHealth, mgl32.Vec3{})
	realBlast.ApplyDamage(67)
	realBlast.ApplyDamage(53) // FragGrenadeExplosion's own falloff calls stack
	if !realBlast.dying {
		t.Errorf("120 cumulative blast damage on a %d-health actor left it alive", defaultActorHealth)
	}

	dead := testActor(t, defaultActorHealth, mgl32.Vec3{})
	dead.dying = true
	dead.ApplyDamage(999)
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
