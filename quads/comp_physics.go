package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

type PhysicsComponent struct {
	baseComponent

	acceleration mgl32.Vec3
	velocity     mgl32.Vec3
	ApplyGravity bool
	aabb         voxel.AABB
	aabbPerAnim  map[AnimIndex]voxel.AABB
	isGrounded   bool
}

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

func (g *PhysicsComponent) Transform(cam *core.FPSCamera) mgl32.Mat4 {
	if g.resolver.HasComponent(ComponentTypeSprite) {
		sprite := g.resolver.GetComponent(ComponentTypeSprite).(*SpriteComponent)
		return sprite.Transform(cam)
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

	if aabb, ok := g.aabbPerAnim[AnimIndex(sprite.CurrentAnimation())]; ok {
		return aabb.Add(pos)
	}
	return g.aabb.Add(pos)
}

func (g *PhysicsComponent) AABB() voxel.AABB {
	transform := g.resolver.Transform()
	return g.AABBAt(transform.GetPosition())
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
}

func (g *PhysicsComponent) SetAABBForAnimation(dying AnimIndex, aabb voxel.AABB) {
	if g.aabbPerAnim == nil {
		g.aabbPerAnim = make(map[AnimIndex]voxel.AABB)
	}
	g.aabbPerAnim[dying] = aabb
}

// OBB is the collider as it is drawn, ie. sprites face the camera. Use it for
// picking, not for physics.
func (g *PhysicsComponent) OBB(camera *core.FPSCamera) voxel.OBB {
	return voxel.OBB{AABB: g.AABBAt(mgl32.Vec3{}), Transform: g.Transform(camera)}
}

// HasOrientedCollider reports whether the object carries a rotation of its own.
// Sprites do not: they are billboards that turn towards the camera, and letting
// physics use that transform would make collisions depend on where the player
// is looking.
func (g *PhysicsComponent) HasOrientedCollider() bool {
	return !g.resolver.HasComponent(ComponentTypeSprite)
}

// WorldOBB is the collider in world space, independent of the camera.
func (g *PhysicsComponent) WorldOBB() voxel.OBB {
	return voxel.OBB{
		AABB:      g.AABBAt(mgl32.Vec3{}),
		Transform: g.GameObject().Transform().GetTransformMatrix(),
	}
}
