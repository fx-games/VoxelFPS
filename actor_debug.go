package main

// The actor page of the F1 menu: a switch that stops every NPC where it stands,
// and one button per animation so each of the six can be looked at without
// having to provoke it.
//
// It exists because a level full of imported characters is a level full of
// things that immediately run at you. Six hundred folders came out of two DOS
// games and most of the work of checking them is looking - at a walk cycle, at
// which frame a death rests on, at whether a facing is the right way round -
// and none of that can be done while they are closing in.

import (
	"fmt"

	"github.com/memmaker/goray/quads"
)

// actorAnimations is the debug page's list of things to play, in the order the
// engine numbers them. Idle is first because it is what everything goes back
// to, and death is last because it is the one that cannot be undone.
var actorAnimations = []struct {
	label string
	anim  quads.AnimIndex
}{
	{"Play Idle Animation", quads.AnimIdle},
	{"Play Movement Animation", quads.AnimWalk},
	{"Play Attack Animation", quads.AnimFire},
	{"Play Hurt Animation", quads.AnimHit},
	{"Play Death Animation", quads.AnimDying},
}

// actorOptions is the page. The switch first, then the animations.
func (g *Game) actorOptions() []quads.GuiOption {
	options := []quads.GuiOption{
		quads.NewGuiOptionBool("Actors Active", &g.actorsActive).
			ActivateFunc(g.applyActorsActive),
	}
	for _, entry := range actorAnimations {
		options = append(options, quads.NewGuiOptionAction(entry.label, func() {
			g.closeAllMenus()
			g.playActorAnimation(entry.anim)
		}))
	}
	return options
}

// applyActorsActive is what the switch does beyond flipping the bool: switching
// them off has to stop the ones already moving, because updateActors is what
// writes their velocity and it is about to stop being called for them.
func (g *Game) applyActorsActive() {
	if g.actorsActive {
		return
	}
	for _, actor := range g.actors {
		if actor.dying || !actor.object.IsAlive() {
			continue
		}
		actor.halt()
		actor.hunting = false
	}
}

// playActorAnimation puts every actor still standing on the same animation, so
// a whole rank of imported characters can be compared in one frame.
//
// Death is played the way dying plays it - frozen on its last frame - because
// an animation that loops back to a corpse standing up is not what a death
// animation looks like. It does not actually kill anything: nothing about the
// actor changes, so switching back to idle brings it straight back.
func (g *Game) playActorAnimation(anim quads.AnimIndex) {
	played := 0
	for _, actor := range g.actors {
		sprite := actor.object.Sprite()
		if sprite == nil || actor.dying || !actor.object.IsAlive() {
			continue
		}
		switch anim {
		case quads.AnimDying:
			sprite.PlayAnimationFreezeLastFrame(int(anim))
		case quads.AnimIdle, quads.AnimWalk:
			sprite.PlayAnimationLooping(int(anim))
		default:
			sprite.PlayAnimation(int(anim))
		}
		played++
	}
	g.Toast(fmt.Sprintf("%s: %d actor(s)", actorAnimationLabel(anim), played))
}

// actorAnimationLabel is what the toast calls the animation, off the same table
// the menu is built from so the two cannot drift.
func actorAnimationLabel(anim quads.AnimIndex) string {
	for _, entry := range actorAnimations {
		if entry.anim == anim {
			return entry.label
		}
	}
	return "Animation"
}
