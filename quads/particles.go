package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

type ParticleName int

const (
	ParticlesBlood ParticleName = iota
	ParticlesBulletImpact
	ParticlesExplosion
	ParticlesSmoke
	ParticlesCustom
)

var Particles = map[ParticleName]glhf.ParticleProperties{
	ParticlesBlood: {
		PositionVariation:    mgl32.Vec3{0.2, 0.2, 0.2},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3{0, 0, 0} },
		VelocityVariation:    mgl32.Vec3{0.1, 0.1, 0.1},
		SizeBegin:            0.02,
		SizeVariation:        0.005,
		SizeEnd:              0.01,
		Lifetime:             0.4,
		ColorBegin:           mgl32.Vec3{0.7, 0.01, 0.01},
		ColorEnd:             mgl32.Vec3{0.4, 0.01, 0.01},
	},
	ParticlesBulletImpact: {
		PositionVariation:    mgl32.Vec3{0.2, 0.2, 0.2},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3{0, 0, 0} },
		VelocityVariation:    mgl32.Vec3{0.1, 0.1, 0.1},
		SizeBegin:            0.02,
		SizeVariation:        0.005,
		SizeEnd:              0.01,
		Lifetime:             0.4,
		ColorBegin:           mgl32.Vec3{0.7, 0.7, 0.7},
		ColorEnd:             mgl32.Vec3{0.01, 0.01, 0.01},
	},
	ParticlesExplosion: {
		PositionVariation: mgl32.Vec3{0.02, 0.02, 0.02},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 {
			return pos.Sub(origin).Normalize().Mul(20)
		},
		VelocityVariation: mgl32.Vec3{0.1, 0.1, 0.1},
		SizeBegin:         0.1,
		SizeVariation:     0.05,
		SizeEnd:           0.5,
		Lifetime:          0.5,
		ColorBegin:        mgl32.Vec3{1, 1, 1},
		ColorEnd:          mgl32.Vec3{0.01, 0.01, 0.01},
	},
}
