package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
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
	g.camera.SetParent(g.GameObject().Transform())
}

func NewCameraComponent(cam core.Camera, eyeHeight float64) *CameraComponent {
	c := &CameraComponent{
		camera:    cam,
		eyeHeight: eyeHeight,
	}
	c.camera.SetPosition(mgl32.Vec3{0, float32(eyeHeight), 0})
	return c
}
