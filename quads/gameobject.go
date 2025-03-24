package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
)

type AnimIndex uint8

const (
    AnimIdle AnimIndex = iota
    AnimWalk
    AnimDying
    AnimHit
    AnimPrepareFire
    AnimFire
    AnimCount
)

type ComponentType uint8

const (
    ComponentTypePhysics ComponentType = iota
    ComponentTypeCamera
    ComponentTypeSprite
    ComponentTypeMesh
    ComponentTypeProjectile
    ComponentTypePathing
)

type baseComponent struct {
    resolver ComponentResolver
}

func (s *baseComponent) Init(resolver ComponentResolver) {
    s.resolver = resolver
}

func (s *baseComponent) IsDead(elapsed float64) bool {
    return s.GameObject().IsDead(elapsed)
}

func (s *baseComponent) GameObject() *GameObject {
    return s.resolver.GameObject()
}

type ComponentResolver interface {
    Transform() *core.Transform
    GameObject() *GameObject
    GetComponent(ComponentType) Component
    HasComponent(ComponentType) bool
}

type Component interface {
    Init(resolver ComponentResolver)
    Type() ComponentType
}

var nextID uint64

func NextID() uint64 {
    returnedId := nextID
    nextID++
    return returnedId
}

type GameObject struct {
    ID         uint64
    transform  *core.Transform
    components map[ComponentType]Component

    DisplayName string

    isDead   bool
    lifeTime float64
}

func NewGameObject(name string) *GameObject {
    g := &GameObject{
        DisplayName: name,
        transform:   core.NewDefaultTransform(),
        components:  make(map[ComponentType]Component),
        ID:          NextID(),
    }
    return g
}

func (g *GameObject) SetLifeTime(lifeTime float64) {
    g.lifeTime = lifeTime
}

func (g *GameObject) Transform() *core.Transform {
    return g.transform
}
func (g *GameObject) GameObject() *GameObject {
    return g
}
func (g *GameObject) GetComponent(componentType ComponentType) Component {
    return g.components[componentType]
}

func (g *GameObject) HasComponent(componentType ComponentType) bool {
    _, ok := g.components[componentType]
    return ok
}

func (g *GameObject) SetComponent(c Component) {
    if g.components == nil {
        g.components = make(map[ComponentType]Component)
    }
    g.components[c.Type()] = c
    c.Init(g)
}

func (g *GameObject) IsProjectile() bool {
    return false
}

func (g *GameObject) IsMovable() bool {
    return true
}

func (g *GameObject) IntersectsRay(start mgl32.Vec3, end mgl32.Vec3) (bool, mgl32.Vec3) {
    return true, mgl32.Vec3{0, 0, 0}
}

func (g *GameObject) Name() string {
    return g.DisplayName
}

func (g *GameObject) IsDead(elapsed float64) bool {
    if g.isDead {
        return true
    }
    if g.lifeTime > 0 {
        g.lifeTime -= elapsed
        if g.lifeTime <= 0 {
            g.isDead = true
            return true
        }
        return false
    }
    return false
}

func (g *GameObject) SetTransform(transform *core.Transform) {
    g.transform = transform
}

func (g *GameObject) Sprite() *SpriteComponent {
    return g.components[ComponentTypeSprite].(*SpriteComponent)
}

func (g *GameObject) Physics() *PhysicsComponent {
    return g.components[ComponentTypePhysics].(*PhysicsComponent)
}

func (g *GameObject) Camera() *CameraComponent {
    return g.components[ComponentTypeCamera].(*CameraComponent)
}

func (g *GameObject) Kill() {
    g.isDead = true
}

func (g *GameObject) IsAlive() bool {
    return !g.isDead
}

func (g *GameObject) IsInViewFrustum(camera *core.FPSCamera) bool {
    if g.HasComponent(ComponentTypePhysics) {
        if !camera.IsCubeInViewFrustum(g.Physics().AABB().Extremes()) {
            return false
        }
    } else {
        if !camera.IsSphereInViewFrustum(g.Transform().GetPosition(), 0.5) {
            return false
        }
    }
    return true
}
