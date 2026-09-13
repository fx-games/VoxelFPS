package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// isNPC is who bleeds. A hologram has a PathingComponent like any other
// actor - this is the regression for the bug the user caught: shooting one
// still put blood on the wall behind it and blood particles in the air,
// because isNPC only ever checked for the component, never what the actor
// behind it was.
func TestHologramIsNotAnNPCForBleedingPurposes(t *testing.T) {
	game := &Game{}
	ordinary := testActor(t, defaultActorHealth, mgl32.Vec3{})
	hologram := testActor(t, defaultActorHealth, mgl32.Vec3{})
	hologram.kind = &SpriteKind{Hologram: true}
	// isNPC's first check is having a PathingComponent at all - real actors get
	// one from spawnActor; testActor keeps it off the GameObject to stay a bare
	// stub, so a bleeding test has to put it there itself.
	ordinary.object.SetComponent(ordinary.pathing)
	hologram.object.SetComponent(hologram.pathing)
	game.actors = []*Actor{ordinary, hologram}

	if !game.isNPC(ordinary.object) {
		t.Error("an ordinary actor should still bleed")
	}
	if game.isNPC(hologram.object) {
		t.Error("a hologram should not bleed")
	}
	if game.isNPC(nil) {
		t.Error("nothing hit at all is not an NPC")
	}
}
