package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
)

type CameraComponent struct {
	baseComponent
	camera    core.Camera
	eyeHeight float64
}

func (g *CameraComponent) Type() ComponentType {
	return ComponentTypeCamera
}

func (g *CameraComponent) Init(resolver ComponentResolver) {
	g.baseComponent.Init(resolver)
	var parent core.Transformer = g.GameObject().Transform()
	if g.GameObject().HasComponent(ComponentTypePhysics) {
		parent = drawnTransform{g.GameObject().Transform(), g.GameObject().Physics()}
	}
	g.camera.SetParent(parent)
}

// drawnTransform is the transform the camera rides: the object's own, moved to
// where its physics says to draw it rather than where the last step left it. See
// PhysicsComponent.RenderPosition. Aiming reads the camera too, so a shot leaves
// from the drawn eye, at most one step behind the collider.
type drawnTransform struct {
	*core.Transform
	physics *PhysicsComponent
}

func (d drawnTransform) GetTransformMatrix() mgl32.Mat4 {
	m := d.Transform.GetTransformMatrix()
	m.SetCol(3, d.physics.RenderPosition().Vec4(1))
	return m
}

func (d drawnTransform) GetPosition() mgl32.Vec3 {
	return d.physics.RenderPosition()
}

func NewCameraComponent(cam core.Camera, eyeHeight float64) *CameraComponent {
	c := &CameraComponent{
		camera:    cam,
		eyeHeight: eyeHeight,
	}
	c.camera.SetPosition(mgl32.Vec3{0, float32(eyeHeight), 0})
	return c
}
