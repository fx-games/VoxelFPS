package main

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
)

// WeaponEffect defines:
// - Hitscan or projectile
// - Firing effects / projectile flight animations
// - Hit/miss effects / projectile impact animations
type WeaponEffect uint8

const (
    WeaponEffectNone WeaponEffect = iota
    WeaponEffectHitscanVoxelDestruction
    WeaponEffectHitscanVoxelPaintGun
    WeaponEffectHitscanVoxelPaintGunFill
    WeaponEffectKillingProjectile
)

func (we WeaponEffect) IsHitscan() bool {
    return we == WeaponEffectHitscanVoxelDestruction ||
        we == WeaponEffectHitscanVoxelPaintGun ||
        we == WeaponEffectHitscanVoxelPaintGunFill
}

type Weapon struct {
    Texture           *glhf.Texture
    Animation         int
    FireFrame         int
    WeaponEffect      WeaponEffect
    DelayBetweenShots float64
    LastShotTime      float64
}

func (g *Game) loadSmallGun() *Weapon {
    weaponAtlas := quads.LoadTextureAtlas("assets/smallgun_sheet.png", 64, 64)
    anim := quads.SpriteAnimation{
        Frames:            [][]uint16{{0, 1, 2, 3, 4}},
        ShowTimeInSeconds: []float32{0.01, 0.01, 0.1, 0.1, 0.1},
    }
    animIndex := g.Gui.AddAnimation(anim)
    return NewWeapon(weaponAtlas, animIndex, 2, WeaponEffectHitscanVoxelDestruction, 0.1)
}

func NewWeapon(texture *glhf.Texture, animation int, fireFrame int, effect WeaponEffect, delayBetweenShots float64) *Weapon {
    return &Weapon{
        Texture:           texture,
        Animation:         animation,
        FireFrame:         fireFrame,
        WeaponEffect:      effect,
        DelayBetweenShots: delayBetweenShots,
    }
}

func (g *Game) switchWeapon(newWeapon *Weapon) {
    g.currentWeapon = newWeapon
    g.weaponQuad.ClearEvents()
    g.weaponQuad.PublishEventOnFrame(g.currentWeapon.FireFrame, events.EventPlayerWeaponFired)
}

func (g *Game) fireWeapon(origin, direction mgl32.Vec3, effect WeaponEffect) {
    if effect.IsHitscan() {
        g.fireHitscan(origin, direction, effect)
    } else {
        g.fireProjectile(origin, direction, effect)
    }
}

type HitscanEffect struct {
    Hit    RaycastResult
    Effect WeaponEffect
}

func (g *Game) fireHitscan(origin, direction mgl32.Vec3, effect WeaponEffect) {
    hit := g.rayCast(origin, direction)
    events.Publish(events.EventHitScanCollision, HitscanEffect{
        Hit:    hit,
        Effect: effect,
    })
}

func (g *Game) hitScanCollision(id events.EventID, event events.Event) {
    hitScanEffect := event.(HitscanEffect)
    hit := hitScanEffect.Hit
    weaponEffect := hitScanEffect.Effect
    hitPos := hit.WorldPos
    hitObject := hit.HitObject
    if hitObject != nil {
        hitPos = hit.HitInfo.CollisionWorldPosition
    }

    g.createImpactEffects(weaponEffect, hit.HitObject, hitPos)
    g.applyHitscanEffect(weaponEffect, hitScanEffect.Hit)
}
func (g *Game) fireProjectile(origin, direction mgl32.Vec3, weaponEffect WeaponEffect) {
    offsetOrigin := origin.Add(direction.Mul(0.8))

    sprite := quads.NewSprite(g.AtlasSprites, []uint16{179})
    newProjectile := quads.NewGameObject("Projectile")
    newProjectile.SetLifeTime(5)
    newProjectile.Transform().SetPosition(offsetOrigin)
    newProjectile.SetComponent(sprite)

    physics := quads.NewPhysicsComponent(false, voxel.NewAABB(mgl32.Vec3{-0.1, -0.1, -0.1}, mgl32.Vec3{0.1, 0.1, 0.1}))
    newProjectile.SetComponent(physics)

    newProjectile.SetComponent(quads.NewProjectileComponent(int(weaponEffect)))

    g.Sprites.Add(sprite)
    sprite.PlayAnimationLooping(g.projectileFlyingAnim)

    g.physicsSystem.AddObject(physics)
    physics.SetVelocity(direction.Mul(10))
}

func (g *Game) physicsCollision(id events.EventID, event events.Event) {
    collisionEvent := event.(quads.CollisionEvent)
    mover := collisionEvent.ObjectA.GameObject()
    if mover.HasComponent(quads.ComponentTypeProjectile) && mover.IsAlive() {
        mover.Kill()

        projectileComponent := mover.GetComponent(quads.ComponentTypeProjectile).(*quads.ProjectileComponent)
        weaponEffect := WeaponEffect(projectileComponent.HitEffect)

        // impact effects
        var hitObject *quads.GameObject
        var relativePosition mgl32.Vec3
        // npc animation & world state changes
        if collisionEvent.ObjectB != nil {
            hitObject = collisionEvent.ObjectB.GameObject()
            relativePosition = hitObject.Transform().GetRelativePosition(collisionEvent.WorldPos)
        }

        g.createImpactEffects(weaponEffect, hitObject, collisionEvent.WorldPos)
        g.applyProjectileEffect(weaponEffect, hitObject, collisionEvent.WorldPos, relativePosition, collisionEvent.SurfaceNormal)
    }
}

func (g *Game) applyHitscanEffect(effect WeaponEffect, result RaycastResult) {
    hitObject := result.HitObject
    switch effect {
    case WeaponEffectHitscanVoxelDestruction:
        if hitObject == nil {
            g.cubeMap.SetAir(result.HitInfo.CollisionGridPosition)
        } else if hitObject.DisplayName == "Scientist" {
            relativeHitPos := result.Relative
            sprite := hitObject.Sprite()
            if relativeHitPos.Y() > 0.6 && relativeHitPos.Y() < 0.8 {
                sprite.PlayAnimationFreezeLastFrame(int(quads.AnimDying))
            } else {
                sprite.PlayAnimationInterrupting(int(quads.AnimHit))
            }
        }
    case WeaponEffectHitscanVoxelPaintGun:
        if hitObject == nil {
            paintedIndex := g.currentPaintGunTextureIndex
            blockPos := result.HitInfo.CollisionGridPosition
            cubeSide := result.HitInfo.Side
            g.cubeMap.SetBlockSide(blockPos, cubeSide, uint16(paintedIndex))
        }
    case WeaponEffectHitscanVoxelPaintGunFill:
        if hitObject == nil {
            paintedIndex := g.currentPaintGunTextureIndex
            blockPos := result.HitInfo.CollisionGridPosition
            cubeSide := result.HitInfo.Side
            g.cubeMap.SetBlockSideFill(blockPos, cubeSide, uint16(paintedIndex))
        }
    }

}
func (g *Game) applyProjectileEffect(effect WeaponEffect, hitObject *quads.GameObject, worldPos, relativeHitPos, normal mgl32.Vec3) {
    switch effect {
    case WeaponEffectKillingProjectile:
        if hitObject != nil && hitObject.DisplayName == "Scientist" {
            hitObject.Sprite().PlayAnimationFreezeLastFrame(int(quads.AnimDying))
        }
    }
}
func (g *Game) createImpactEffects(effect WeaponEffect, hitObject *quads.GameObject, pos mgl32.Vec3) {
    switch effect {
    case WeaponEffectHitscanVoxelDestruction:
        if hitObject == nil {
            g.EmitParticlesByName(quads.ParticlesBulletImpact, pos, 30)
        } else {
            bloodProps := quads.Particles[quads.ParticlesBlood].WithOrigin(pos)
            bloodProps.VelocityFromPosition = func(origin, pos mgl32.Vec3) mgl32.Vec3 {
                return mgl32.Vec3{0, -0.01, 0}
            }
            g.EmitParticles(quads.ParticlesCustom, bloodProps, 30)
        }
    case WeaponEffectKillingProjectile:
        g.spawnProjectileImpact(pos, g.AtlasSprites, g.projectileImpactAnim)
        if hitObject != nil {
            g.EmitParticlesByName(quads.ParticlesBlood, pos, 30)
        }
    }
}
