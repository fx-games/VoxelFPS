package quads

// A projectile is a thing in flight that settles what it does when it arrives.
// It carries the damage itself rather than leaving it with whoever fired it,
// which is what makes a bolt something you can walk out of the way of.
type ProjectileComponent struct {
	baseComponent
	// Damage is what a hit costs whatever it lands on, in the same points a
	// Combatant counts. Zero is a bolt that only makes a noise and a light.
	Damage int
	// Owner is what fired it. A bolt passes through its own shooter, and the
	// hit it lands carries the owner on so the far end knows which side of the
	// fight it came from.
	Owner *GameObject
}

func (g *ProjectileComponent) Type() ComponentType {
	return ComponentTypeProjectile
}

func (g *ProjectileComponent) Init(resolver ComponentResolver) {
	g.baseComponent.Init(resolver)
}

func NewProjectileComponent(damage int, owner *GameObject) *ProjectileComponent {
	return &ProjectileComponent{Damage: damage, Owner: owner}
}
