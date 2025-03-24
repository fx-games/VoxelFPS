package quads

type ProjectileComponent struct {
    baseComponent
    HitEffect int
}

func (g *ProjectileComponent) Type() ComponentType {
    return ComponentTypeProjectile
}

func (g *ProjectileComponent) Init(resolver ComponentResolver) {
    g.baseComponent.Init(resolver)
}

func NewProjectileComponent(hitEffect int) *ProjectileComponent {
    c := &ProjectileComponent{
        HitEffect: hitEffect,
    }
    return c
}
