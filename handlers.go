package main

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads"
)

func (g *Game) weaponHitScanMiss(id events.EventID, event events.Event) {
    hit := event.(RaycastResult)
    hitPos := hit.HitInfo.CollisionWorldPosition
    g.EmitParticlesByName(quads.ParticlesBulletImpact, hitPos, 30)
}

func (g *Game) weaponHitScanHit(id events.EventID, event events.Event) {
    hit := event.(RaycastResult)
    if hit.HitObject.DisplayName == "Scientist" {
        sprite := hit.HitObject.Sprite()
        if hit.Relative.Y() > 0.6 && hit.Relative.Y() < 0.8 {
            sprite.PlayAnimationFreezeLastFrame(int(quads.AnimDying))
        } else {
            sprite.PlayAnimationInterrupting(int(quads.AnimHit))
        }

        bloodProps := quads.Particles[quads.ParticlesBlood].WithOrigin(hit.WorldPos)
        bloodProps.VelocityFromPosition = func(origin, pos mgl32.Vec3) mgl32.Vec3 {
            return mgl32.Vec3{0, -0.01, 0}
        }

        g.EmitParticles(quads.ParticlesCustom, bloodProps, 30)
    } else {
        events.Publish(events.EventWeaponHitScanMiss, hit)
    }
}

func (g *Game) collision(id events.EventID, event events.Event) {
    collisionEvent := event.(quads.CollisionEvent)
    mover := collisionEvent.ObjectA.GameObject()
    if mover.DisplayName == "Projectile" && mover.IsAlive() {
        mover.Kill()
        g.spawnProjectileImpact(collisionEvent.WorldPos, g.projectileImpactAnim)
        if collisionEvent.ObjectB == nil {
            return
        }
        hitObject := collisionEvent.ObjectB.GameObject()
        if hitObject.DisplayName == "Scientist" {
            hitObject.Sprite().PlayAnimationFreezeLastFrame(int(quads.AnimDying))
            g.EmitParticlesByName(quads.ParticlesBlood, collisionEvent.WorldPos, 30)
        }
    }
}
