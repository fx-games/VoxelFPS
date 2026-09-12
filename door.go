package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
)

// doorSlideMeshName and doorSwingLeafMeshName are the two prototype doors: a
// door plane that slides on its own (see spawnEntity's switch), and the
// moving leaf of a swing door - the frame it swings inside of is ordinary
// static scenery (assets/meshes/door_swing_frame), not a Door at all.
const (
	doorSlideMeshName     = "door_slide"
	doorSwingLeafMeshName = "door_swing_leaf"
)

type doorKind uint8

const (
	doorSlide doorKind = iota
	doorSwing
)

// doorSlideRise is how far a sliding door travels: one whole voxel, out of the
// opening and into the solid next to it - straight up for the upright ones. A door tile is
// empty (WallHeight 0) in the map, so that headroom is already there -
// nothing has to be carved out for it, the same way ladders rely on the
// climb volume's own tile being open rather than clearing one.
//
// doorSwingOpenAngle is how far a leaf swings, and negative because opening
// away from whichever side the frame's Colliders were authored against
// (jambs either side of local X0) reads more naturally than swinging into
// the corridor the player is standing in.
const (
	doorSlideRise      = 1.0
	doorSwingOpenAngle = -100.0

	doorSlideSpeed    = 1.0 / 0.6 // fully open or closed in 0.6s
	doorSwingSpeed    = 1.0 / 0.5 // fully open or closed in 0.5s
	doorAutoCloseHold = 5.0       // seconds a slide door stays open before it resets itself
)

// Door is a slide or swing door: whichever GameObject actually moves (the
// slide panel, or the swing leaf - never the swing frame, which is static
// scenery with its own opening already cut into its colliders). openT is 0
// closed, 1 open, and eases toward target at the kind's own speed; nothing
// here waits for the animation to finish before letting the player push
// through it - see updateDoors.
type Door struct {
	object *quads.GameObject
	kind   doorKind

	closedPos mgl32.Vec3 // slide only: where the panel sits shut

	openT  float32 // 0 = closed, 1 = fully open
	target float32 // where openT is easing toward

	// holdTimer counts down once a slide door has finished opening; reaching
	// zero sends it back to target 0. A swing door ignores this entirely -
	// "stays open until the next interaction" per its own spec.
	holdTimer float32

	// locked is a door a keypad is holding shut: Interact does nothing to it
	// until the four digits on the panel beside it have been typed in. The door
	// itself knows nothing about the code - which keypad holds which door is
	// worked out from where they stand, see Game.bindKeypads.
	locked bool
}

// spawnDoor turns a plain entity GameObject into a Door: spawnEntity has
// already given it a mesh and a physics box (see spawn_entities.go's switch
// on MeshName), this only adds the state machine that animates it and the
// registration that lets playerInteractWith find it again.
func (g *Game) spawnDoor(object *quads.GameObject, kind doorKind, spawnPos mgl32.Vec3) {
	door := &Door{
		object:    object,
		kind:      kind,
		closedPos: spawnPos,
	}
	g.doors = append(g.doors, door)
}

// doorFor finds the Door behind a GameObject a raycast or interact handed
// back, or nil for anything that is not one. See actor.go's actorFor, same
// shape for the same reason: this only runs once per keypress, not once a
// frame, so a linear scan over the level's handful of doors costs nothing.
func (g *Game) doorFor(object *quads.GameObject) *Door {
	for _, door := range g.doors {
		if door.object == object {
			return door
		}
	}
	return nil
}

// Interact is the E key landing on this door. A slide door always heads for
// (or stays at) open and gets its hold timer refreshed - pressing it again
// while it is already open just postpones the auto-close, which is the
// obvious thing for a re-press to do. A swing door has no timer: this simply
// toggles which way it is headed, open or shut, same as a real door handle.
func (d *Door) Interact() {
	if d.locked {
		return
	}
	switch d.kind {
	case doorSlide:
		d.target = 1
		d.holdTimer = doorAutoCloseHold
	case doorSwing:
		if d.target == 0 {
			d.target = 1
		} else {
			d.target = 0
		}
	}
}

// updateDoors eases every door's openT toward its target and applies that to
// the one thing that differs between the two kinds: a slide door translates,
// a swing door rotates about its own local origin - which is why
// assets/meshes/door_swing_leaf's mesh has its hinge edge at that origin rather
// than at its centre. PassThrough comes off physics the instant a door
// starts moving in either direction, not once it is fully open: waiting for
// the animation to finish blocked the player on a door that was already most
// of the way open.
func (g *Game) updateDoors(elapsed float64) {
	for _, door := range g.doors {
		if door.kind == doorSlide && door.target == 1 && door.openT >= 1 {
			door.holdTimer -= float32(elapsed)
			if door.holdTimer <= 0 {
				door.target = 0
			}
		}

		speed := float32(doorSlideSpeed)
		if door.kind == doorSwing {
			speed = doorSwingSpeed
		}
		step := speed * float32(elapsed)
		switch {
		case door.openT < door.target:
			door.openT = min(door.target, door.openT+step)
		case door.openT > door.target:
			door.openT = max(door.target, door.openT-step)
		}

		switch door.kind {
		case doorSlide:
			// Along the leaf's own up, not the world's. A door standing in a
			// wall has the two pointing the same way and rises into its lintel;
			// one laid flat - a floor hatch, turned there by a facing with a Y
			// in it - slides sideways under the deck instead. Same door, same
			// model, one vector.
			up := door.object.Transform().GetRotation().Rotate(mgl32.Vec3{0, 1, 0})
			door.object.Transform().SetPosition(door.closedPos.Add(up.Mul(doorSlideRise * door.openT)))
		case doorSwing:
			angle := mgl32.DegToRad(doorSwingOpenAngle) * door.openT
			door.object.Transform().SetRotation(mgl32.QuatRotate(angle, mgl32.Vec3{0, 1, 0}))
		}

		door.object.Physics().PassThrough = door.openT > 0
	}
}
