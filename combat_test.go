package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
)

// The three shapes a hit comes in, and what each side of a fight charges itself
// for them. The one that is easy to get wrong is the third: a Damage with no
// Amount is an ordinary shot, not a free one, and reading it as zero would make
// every bullet in the game harmless.
func TestWhatEachShapeOfHitCosts(t *testing.T) {
	t.Run("an actor", func(t *testing.T) {
		aimed := testActor(t, defaultActorHealth, mgl32.Vec3{})
		aimed.TakeDamage(Damage{Headshot: true})
		if !aimed.dying {
			t.Errorf("a headshot left a %d-health actor standing", defaultActorHealth)
		}

		numbered := testActor(t, defaultActorHealth, mgl32.Vec3{})
		numbered.TakeDamage(Damage{Amount: 7})
		if numbered.Health != defaultActorHealth-7 {
			t.Errorf("a 7-point hit left %d health, want %d", numbered.Health, defaultActorHealth-7)
		}

		body := testActor(t, defaultActorHealth, mgl32.Vec3{})
		body.TakeDamage(Damage{})
		want := defaultActorHealth - defaultActorHealth/bodyShotsToKill
		if body.Health != want {
			t.Errorf("an ordinary body shot left %d health, want %d", body.Health, want)
		}
		// And exactly bodyShotsToKill of them finish the job, which is the
		// design constant the whole no-Amount case exists to keep.
		for i := 1; i < bodyShotsToKill; i++ {
			body.TakeDamage(Damage{})
		}
		if !body.dying {
			t.Errorf("%d body shots left a default actor standing", bodyShotsToKill)
		}
	})

	t.Run("the player", func(t *testing.T) {
		game := testGame(t, mgl32.Vec3{})

		game.player.TakeDamage(Damage{Amount: 30})
		if got := game.player.Health(); got != playerMaxHealth-30 {
			t.Errorf("a 30-point hit left %d health, want %d", got, playerMaxHealth-30)
		}

		game.player.Heal(playerMaxHealth)
		game.player.TakeDamage(Damage{})
		want := playerMaxHealth - playerMaxHealth/bodyShotsToKill
		if got := game.player.Health(); got != want {
			t.Errorf("an ordinary body shot left %d health, want %d", got, want)
		}
	})
}

// God mode is checked in TakeDamage rather than at every place damage comes
// from, so a new source of damage is covered by being written at all. If it
// ever moves out to the call sites this is what notices.
func TestGodModeIsCheckedWhereTheDamageLands(t *testing.T) {
	game := testGame(t, mgl32.Vec3{})
	game.cheats.godMode = true

	game.player.TakeDamage(Damage{Amount: 500})
	game.player.TakeDamage(Damage{Headshot: true})

	if got := game.player.Health(); got != playerMaxHealth {
		t.Errorf("god mode let %d points through, health is %d", playerMaxHealth-got, got)
	}
}

// A projectile arrives at a GameObject and has to find out what that object is.
// Both sides of a fight answer, and nothing else does - a wall or a barrel is
// not something to deal damage to.
func TestCombatantForFindsBothSidesAndNothingElse(t *testing.T) {
	game := testGame(t, mgl32.Vec3{})
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{5, 0, 5})
	game.actors = []*Actor{actor}

	if got := game.combatantFor(game.player.GameObject); got != Combatant(game.player) {
		t.Errorf("the player's own object came back as %#v", got)
	}
	if got := game.combatantFor(actor.object); got != Combatant(actor) {
		t.Errorf("an actor's object came back as %#v", got)
	}
	if got := game.combatantFor(quads.NewGameObject("barrel")); got != nil {
		t.Errorf("a barrel came back as a combatant: %#v", got)
	}
	if got := game.combatantFor(nil); got != nil {
		t.Errorf("nothing came back as a combatant: %#v", got)
	}
}

// A corpse is not a target. The projectile path checks Alive before it deals
// its damage, and an actor that has already died has to say so - otherwise a
// bolt landing on a body counts as a hit and the bolt is spent on it.
func TestACorpseIsNotAliveAndAFreshActorIs(t *testing.T) {
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{})
	if !actor.Alive() {
		t.Error("a fresh actor reports itself dead")
	}
	actor.TakeDamage(Damage{Headshot: true})
	if actor.Alive() {
		t.Error("a corpse reports itself alive")
	}
}
