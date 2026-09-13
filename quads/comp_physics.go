package quads

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

type PhysicsComponent struct {
	baseComponent

	acceleration mgl32.Vec3
	velocity     mgl32.Vec3
	ApplyGravity bool
	// PassThrough is a collider things move straight through: it is on the
	// grid, so a ray finds it and the selection can pick it out, but the
	// collision solver skips it. A pickup is the case it exists for - Doom's
	// items are something you walk over, not something you bump into - and
	// without it the only way to be findable by a ray was to be a wall.
	PassThrough bool
	aabb        voxel.AABB
	aabbPerAnim map[AnimIndex]voxel.AABB
	// turnedFrom, turnedBy and turned remember WorldAABB's last rotation: the
	// local box, the rotation it was turned by and the result. Turning eight
	// corners is most of what culling a prop costs, and a prop that is not
	// turning asks for the same answer every frame.
	turnedFrom voxel.AABB
	turnedBy   mgl32.Quat
	turned     voxel.AABB
	hasTurned  bool
	// collisionAABB is the box the solver moves, for a thing whose collider is
	// not the box a ray picks it out by. A picture is drawn wide and flat and a
	// collider that wide is one that catches on everything beside it, so an item
	// says here how much room it really takes up.
	collisionAABB *voxel.AABB
	// pickBoxes is the box this thing is *picked* out by, per animation, facing
	// and frame, or nil for a kind that brought none. See SetPickBoxes: the box
	// in aabbPerAnim is one shape standing in for every view and every frame
	// because a solver must not care where the player is looking, and a ray does
	// care - it is testing the picture actually on the screen.
	pickBoxes PickBoxes
	// gridBox is the box this thing is listed on the collision grid by, worked
	// out on first use and then kept - see gridAABBAt. Every setter that feeds
	// it drops it again, so a box given after the thing is already on the grid
	// is not one the grid never hears about.
	gridBox *voxel.AABB
	// colliderBottom is how low the collision box reached below this thing's own
	// position when it was last looked at, so that a box changing shape under it
	// can be noticed - see PhysicsSystem.keepStandingOnIt.
	colliderBottom float32
	isGrounded     bool
	// system is the solver this is in, and stepFrom and stepTo where its last
	// fixed step moved this thing from and to - see RenderPosition.
	system           *PhysicsSystem
	stepFrom, stepTo mgl32.Vec3
}

// PickBoxes is one kind's picking boxes: by animation, then by the facing the
// drawer picks its frames with (see core.GetViewDirection), then by the frame it
// is on. Read once per kind and shared by every one of them that spawns; nothing
// writes to it after that.
type PickBoxes map[AnimIndex][][]voxel.AABB

// IsGrounded reports whether the last move ended on top of something. Jumping
// off thin air is not a thing.
func (g *PhysicsComponent) IsGrounded() bool {
	return g.isGrounded
}

func (g *PhysicsComponent) SetGrounded(grounded bool) {
	g.isGrounded = grounded
}

func (g *PhysicsComponent) Position() mgl32.Vec3 {
	return g.resolver.Transform().GetPosition()
}

func (g *PhysicsComponent) SetPosition(pos mgl32.Vec3) {
	g.resolver.Transform().SetPosition(pos)
}

// RenderPosition is where to draw this thing: part of the way from where the last
// physics step started it to where that step left it, by how far the clock is
// into the next one. The solver steps 60 times a second and the screen does not,
// so drawing Position shows one place for two frames and then jumps - measured
// on a 120 Hz panel as every other frame without a step. Moved since that step by
// something other than the solver, a respawn say, it is drawn where it is.
func (g *PhysicsComponent) RenderPosition() mgl32.Vec3 {
	now := g.Position()
	if g.system == nil || now != g.stepTo {
		return now
	}
	return g.stepFrom.Add(now.Sub(g.stepFrom).Mul(g.system.stepFraction()))
}

// Transform is the frame this thing's collider stands in: where it is and which
// way it is turned, and not how big it is drawn. A sprite's box is measured in
// world units already - see SpriteComponent.Orientation, which is why this is
// not the sprite's full draw transform.
func (g *PhysicsComponent) Transform(cam *core.FPSCamera) mgl32.Mat4 {
	if g.resolver.HasComponent(ComponentTypeSprite) {
		sprite := g.resolver.GetComponent(ComponentTypeSprite).(*SpriteComponent)
		return sprite.Orientation(cam)
	}
	return g.GameObject().Transform().GetTransformMatrix()
}

func (g *PhysicsComponent) IsMovable() bool {
	return true
}

func (g *PhysicsComponent) IsProjectile() bool {
	return false
}

func (g *PhysicsComponent) Type() ComponentType {
	return ComponentTypePhysics
}

func NewPhysicsComponent(applyGravity bool, aabb voxel.AABB) *PhysicsComponent {
	return &PhysicsComponent{
		ApplyGravity: applyGravity,
		aabb:         aabb,
	}
}

func (g *PhysicsComponent) Acceleration() mgl32.Vec3 {
	return g.acceleration
}

func (g *PhysicsComponent) AABBAt(pos mgl32.Vec3) voxel.AABB {
	if !g.resolver.HasComponent(ComponentTypeSprite) {
		return g.aabb.Add(pos)
	}

	sprite := g.resolver.GetComponent(ComponentTypeSprite).(*SpriteComponent)

	// Keyed by the sprite's own animation number, not the slot it plays out of:
	// every kind of thing registers its own six and gets its own block, so the
	// slot for "dying" is a different number for a guard than for a mutant.
	if aabb, ok := g.aabbPerAnim[AnimIndex(sprite.LocalAnimation())]; ok {
		return aabb.Add(pos)
	}
	return g.aabb.Add(pos)
}

// SetCollisionAABB gives this thing a collider in world units, for when the
// box a ray picks it out by is written in some other units. See collisionAABB.
func (g *PhysicsComponent) SetCollisionAABB(aabb voxel.AABB) {
	g.collisionAABB = &aabb
	g.gridBox = nil
}

// CollisionAABBAt is the box the collision solver moves and stops. It is the
// picking box unless the thing has been given a collider of its own.
func (g *PhysicsComponent) CollisionAABBAt(pos mgl32.Vec3) voxel.AABB {
	if g.collisionAABB != nil {
		return g.collisionAABB.Add(pos)
	}
	return g.AABBAt(pos)
}

func (g *PhysicsComponent) AABB() voxel.AABB {
	transform := g.resolver.Transform()
	return g.CollisionAABBAt(transform.GetPosition())
}

func (g *PhysicsComponent) AffectedByGravity() bool {
	return g.ApplyGravity
}

func (g *PhysicsComponent) Velocity() mgl32.Vec3 {
	return g.velocity
}

func (g *PhysicsComponent) SetVelocity(newVelocity mgl32.Vec3) {
	g.velocity = newVelocity
}
func (g *PhysicsComponent) SetAABB(aabb voxel.AABB) {
	g.aabb = aabb
	g.gridBox = nil
}

// HasAABBForAnimation is whether this one already carries a box of its own,
// which is what tells a kind that brought its measurements from one that has to
// have them taken.
func (g *PhysicsComponent) HasAABBForAnimation(anim AnimIndex) bool {
	_, ok := g.aabbPerAnim[anim]
	return ok
}

func (g *PhysicsComponent) SetAABBForAnimation(dying AnimIndex, aabb voxel.AABB) {
	if g.aabbPerAnim == nil {
		g.aabbPerAnim = make(map[AnimIndex]voxel.AABB)
	}
	g.aabbPerAnim[dying] = aabb
	g.gridBox = nil
}

// SetPickBoxes gives this thing the boxes of the pictures it is actually drawn
// as, so that a ray is tested against the one on the screen. The single box it
// collides by is the middle of every view of a whole animation - see
// geometry.BoxAcrossDirections - and for a thing whose silhouette moves, that
// box is nothing like the picture in front of you: System Shock's avian mutant
// is twice as wide with its wings out as with them folded, and the crosshair
// picked it out most of a wingspan beside itself.
func (g *PhysicsComponent) SetPickBoxes(boxes PickBoxes) {
	g.pickBoxes = boxes
	g.gridBox = nil
}

// OBB is the collider as it is drawn, ie. sprites face the camera. Use it for
// picking, not for physics.
func (g *PhysicsComponent) OBB(camera *core.FPSCamera) voxel.OBB {
	return voxel.OBB{AABB: g.pickBoxFrom(camera), Transform: g.Transform(camera)}
}

// pickBoxFrom is the box to test a ray against from where the camera stands: the
// picture the sprite is drawn as this instant, if the kind brought one, and
// otherwise the one box it has. The frame and the facing are looked up the way
// SpriteDrawer looks up the picture itself, or the outline would be around a
// frame that is not the one on the screen.
func (g *PhysicsComponent) pickBoxFrom(camera *core.FPSCamera) voxel.AABB {
	fallback := g.AABBAt(mgl32.Vec3{})
	if g.pickBoxes == nil || !g.resolver.HasComponent(ComponentTypeSprite) {
		return fallback
	}
	sprite := g.resolver.GetComponent(ComponentTypeSprite).(*SpriteComponent)
	anim, frame := AnimIndex(sprite.LocalAnimation()), sprite.CurrentFrame()
	if sprite.LocalAnimation() < 0 {
		// Nothing playing at all, which is most of a level: a prop that was
		// placed and left alone is drawn from its own TextureIndices, and those
		// are the first frame of each facing of its idle - see
		// SpriteComponent.TextureIndex and SpriteKind.idleFrames.
		anim, frame = AnimIdle, 0
	}
	facings := g.pickBoxes[anim]
	facing := core.GetViewDirection(camera.GetPosition(), camera.GetForward(), sprite.Position(), sprite.LookDirection())
	if len(facings) == 1 {
		// Drawn the same from everywhere - an explosion, a barrel - which is
		// the same one list SpriteDrawer falls back to.
		facing = 0
	}
	if facing < 0 || facing >= len(facings) {
		return fallback
	}
	frames := facings[facing]
	if frame < 0 || frame >= len(frames) || isEmptyBox(frames[frame]) {
		return fallback
	}
	return frames[frame]
}

// isEmptyBox is a frame that draws nothing, which measures nothing.
func isEmptyBox(box voxel.AABB) bool {
	return box.Max().X() <= box.Min().X() || box.Max().Y() <= box.Min().Y()
}

// gridAABBAt is the box PhysicsSystem lists this thing on the grid by, at the
// given position. It is the union of every box anything ever tests it against:
// the solver's box for whichever animation is playing, the collider it was
// given instead, and the picking box of every frame of every facing.
//
// One of those on its own is not enough, because they are different shapes. The
// grid is the broad phase for the picking ray as much as for the solver: the ray
// walks cells and tests only what the cell it is in lists, so a cell the picking
// box reaches into and the listed box does not is a cell the ray finds nothing
// in. The CORTEX RVR is a metre and a half across and was listed as the median
// of its eight views - see geometry.BoxAcrossDirections - so from the angles
// where it is drawn wider than that median, the crosshair sat on its outline and
// selected nothing.
//
// Being one box for every animation also means a thing takes its listing with it
// when it moves: removeFromGrid works out the same cells addToGrid did, rather
// than the cells of whatever animation happens to be playing now.
func (g *PhysicsComponent) gridAABBAt(pos mgl32.Vec3) voxel.AABB {
	if g.gridBox == nil {
		box := g.aabb
		if g.collisionAABB != nil {
			box = box.Union(*g.collisionAABB)
		}
		for _, perAnim := range g.aabbPerAnim {
			box = box.Union(perAnim)
		}
		for _, facings := range g.pickBoxes {
			for _, frames := range facings {
				for _, frame := range frames {
					box = box.Union(frame)
				}
			}
		}
		// Every listing is swung round its own position, whether the thing
		// turns to face the camera or carries a rotation of its own: the cells
		// are worked out from extents alone (they are only translated below),
		// and a turned piece whose box is not square - a staircase is two tiles
		// deep and one wide - is listed in the cells it would stand in
		// unturned, so the solver never tests the cells it is really in.
		box = swungRound(box)
		g.gridBox = &box
	}
	return g.gridBox.Add(pos)
}

// swungRound is the box a box can reach into at any turn about the vertical:
// a billboard swung to face whoever is looking at it (see
// SpriteComponent.Orientation), or a piece of scenery placed with a facing= of
// its own. Exact for the quarter turns a piece is ever given, generous for the
// angles in between, and - unlike turning the box by the rotation it has right
// now - still true after a door swings or an actor turns round, which is what
// lets one listing be cached and reused for removeFromGrid.
//
// Face on, a sprite's box is as wide as the picture and a few hundredths deep;
// from the side those two swap over. Listed on the grid as drawn from one angle,
// a thing is missing from every cell it only reaches into from another, and the
// picking ray tests what the cell it is in lists and nothing else. That is why a
// CORTEX RVR - half again as wide as a cell, and a sliver deep - picked out
// cleanly from one side of the room and not at all from the next.
//
// ponytail: the swing is about the vertical axis only, which is what
// SpriteModeFollowHorizontal does. A sprite that also pitches with the camera
// can reach above and below this; nothing selectable is drawn that way today.
func swungRound(box voxel.AABB) voxel.AABB {
	furthest := func(low, high float32) float64 {
		return math.Max(math.Abs(float64(low)), math.Abs(float64(high)))
	}
	x, z := furthest(box.Min().X(), box.Max().X()), furthest(box.Min().Z(), box.Max().Z())
	reach := float32(math.Sqrt(x*x + z*z))
	return voxel.NewAABB(
		mgl32.Vec3{-reach, box.Min().Y(), -reach},
		mgl32.Vec3{reach, box.Max().Y(), reach},
	)
}

// HasOrientedCollider reports whether the object carries a rotation of its own.
// Sprites do not: they are billboards that turn towards the camera, and letting
// physics use that transform would make collisions depend on where the player
// is looking.
func (g *PhysicsComponent) HasOrientedCollider() bool {
	return !g.resolver.HasComponent(ComponentTypeSprite)
}

// WorldAABB is where this collider really is, as plain extents: its own box,
// turned the way the object is turned, at the position the object stands.
//
// A collider is held in the model's own space and every consumer that has the
// object's matrix applies the rotation itself - the solver through WorldOBB,
// the picking ray through OBB. The ones that cannot (a frustum test takes six
// planes and a box, a debug outline draws one) ask here instead of translating
// the local box and quietly drawing, culling or testing a turned piece where
// the unturned one would have stood. That is what culled the ark's ladders: the
// model is hard against its own +Z, the shaft's are turned to face the other
// way, and the box left behind sat on the far side of the shaft from the rungs,
// so the segments nearest the camera fell out of the frustum at a steep look up
// or down while the rungs were plainly in shot.
func (g *PhysicsComponent) WorldAABB() voxel.AABB {
	transform := g.resolver.Transform()
	box := g.CollisionAABBAt(mgl32.Vec3{})
	if g.HasOrientedCollider() {
		rotation := transform.GetRotation()
		if !g.hasTurned || box != g.turnedFrom || rotation != g.turnedBy {
			g.turnedFrom, g.turnedBy, g.turned, g.hasTurned = box, rotation, box.Rotated(rotation), true
		}
		box = g.turned
	}
	return box.Add(transform.GetPosition())
}

// WorldOBB is the collider in world space, independent of the camera.
func (g *PhysicsComponent) WorldOBB() voxel.OBB {
	return voxel.OBB{
		AABB:      g.AABBAt(mgl32.Vec3{}),
		Transform: g.GameObject().Transform().GetTransformMatrix(),
	}
}
