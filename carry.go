package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
)

// Carrying an item. The carry key takes whatever the selection is on into the
// player's hands, where it hangs in front of them as a ghost and follows the
// aim; the next press puts it down where the aim has it, and gravity has it
// again from there. It is how a weapon is laid on a table rather than dropped
// on the floor, and it is the same key in the editor, where standing the
// level's items exactly where they belong is the whole job.

// carryReach is how far in front of the player an item in hand hangs, and so
// how far the aim may push it into the world: about a block, so an item goes
// on the first solid thing inside that or hangs at arm's length.
const carryReach = 1.0

// toggleCarry is the carry key: take what is being looked at, or put down what
// is being held.
func (g *Game) toggleCarry() {
	if g.heldItem != nil {
		g.dropItem()
		return
	}
	object := g.hoveredObj()
	if !g.carryable(object) {
		return
	}
	// An arm's length, the same reach the pickup beam has: an item across the
	// room is something to walk to, not something to take hold of.
	if object.Transform().GetPosition().Sub(g.Camera.GetPosition()).Len() > pickupReach {
		return
	}
	g.heldItem = object
	// Off the physics grid while it is in hand: nothing collides with it,
	// gravity does not pull on it, and the aiming ray looks through it instead
	// of finding the very thing it is placing.
	g.physicsSystem.RemoveObject(object.Physics())
	g.ghostItem(object, true)
	g.Toast("Carrying " + object.DisplayName)
}

// dropItem puts the held item down where it is being aimed. It goes back on the
// grid with gravity switched on, so an item placed above the floor falls to it.
func (g *Game) dropItem() {
	object := g.heldItem
	g.heldItem = nil
	g.ghostItem(object, false)
	physics := object.Physics()
	physics.ApplyGravity = true
	physics.SetVelocity(mgl32.Vec3{})
	g.physicsSystem.AddObject(physics)
	// A pickup is lit by the room it stands in rather than by the lights that
	// reach it - see spawnPickup - and it is standing in a different room now.
	// Only something that was baked to begin with: a decoration is lit the
	// ordinary way and a baked value would override it.
	if sprite := heldSprite(object); sprite != nil && sprite.BakedLight > 0 {
		sprite.BakedLight = bakedLightIn(g.CubeMap, object.Transform().GetPosition())
	}
	// A save writes where the level's things stand, and this one has just been
	// stood somewhere else. A pickup off the weapon range has no record in the
	// map file and so nothing to correct.
	//
	// ponytail: the position it was placed at, not the one it rolls to a stop
	// at - aiming at something solid within a block is what the key is for, so
	// the two are the same unless it was let go into thin air. Record it on
	// landing if that stops being true.
	if record := g.placedRecord(object); record != nil {
		record.pos = object.Transform().GetPosition()
	}
	g.Toast("Placed " + object.DisplayName)
}

// carryable is a thing that may be taken and put down again: an item, not a
// piece of the level. Actors walk, doors swing and a keypad is screwed to the
// wall; scenery is furniture the selection already looks straight through, so
// it never gets this far.
func (g *Game) carryable(object *quads.GameObject) bool {
	if object == nil || !object.HasComponent(quads.ComponentTypePhysics) {
		return false
	}
	if g.actorFor(object) != nil || g.doorFor(object) != nil || g.keypadFor(object) != nil {
		return false
	}
	// A torch is its model and the light it floods the voxels around it with,
	// and only the model would move: the light is baked where the torch was
	// dropped. See dropTorch.
	if record := g.placedRecord(object); record != nil && record.entity.MeshName == torchMeshName {
		return false
	}
	return true
}

// carryPosition is where the item in hand is: on the first solid thing the aim
// meets within a block, and otherwise hanging a block in front of the player.
// The distance is measured to the point that was hit rather than read off the
// ray, because a ray that stopped on an object stopped at the object's own
// surface and not at the voxel the walk had reached.
func carryPosition(origin, direction mgl32.Vec3, aim RaycastResult) mgl32.Vec3 {
	if at := aim.WorldPosition(); aim.HitInfo.Hit && at.Sub(origin).Len() <= carryReach {
		return at
	}
	return origin.Add(direction.Mul(carryReach))
}

// ghostItem is what "in hand" looks like: the item goes half transparent, and
// half transparent sprites belong in the drawer's transparent pass, which is
// what Add sorts it into. A mesh item has no tint to give it and makes do with
// the outline DrawGui puts round whatever is being carried.
func (g *Game) ghostItem(object *quads.GameObject, held bool) {
	sprite := heldSprite(object)
	if sprite == nil {
		return
	}
	g.Sprites.Remove(sprite)
	sprite.Tint = mgl32.Vec4{1, 1, 1, 1}
	if held {
		sprite.Tint = mgl32.Vec4{0.6, 1, 0.6, 0.5}
	}
	g.Sprites.Add(sprite)
}

// heldSprite is the item's picture, or nil for one that is a model - a note is
// a mesh, and a mesh has neither a tint nor a baked light to give it.
func heldSprite(object *quads.GameObject) *quads.SpriteComponent {
	if !object.HasComponent(quads.ComponentTypeSprite) {
		return nil
	}
	return object.Sprite()
}
