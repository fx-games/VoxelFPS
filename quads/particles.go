package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads/glhf"
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
	ParticlesSmoke: {
		PositionVariation:    mgl32.Vec3{0.15, 0.15, 0.15},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3{0, 0.45, 0} },
		VelocityVariation:    mgl32.Vec3{0.25, 0.15, 0.25},
		SizeBegin:            0.03,
		SizeVariation:        0.015,
		SizeEnd:              0.12,
		SpreadLifetime:       0.004,
		Lifetime:             1.2,
		ColorBegin:           mgl32.Vec3{0.35, 0.35, 0.35},
		ColorEnd:             mgl32.Vec3{0.08, 0.08, 0.08},
	},
	// The shell flies outwards from the impact point. Speed and size are metres and
	// metres per second, same as everything else in the world: the 20 m/s and 0.5 m
	// quads this used to carry filled the screen with grey before the first frame
	// was over.
	ParticlesExplosion: {
		PositionVariation: mgl32.Vec3{0.05, 0.05, 0.05},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 {
			return pos.Sub(origin).Normalize().Mul(2.5)
		},
		VelocityVariation: mgl32.Vec3{0.4, 0.4, 0.4},
		SizeBegin:         0.045,
		SizeVariation:     0.02,
		SizeEnd:           0.01,
		Lifetime:          0.45,
		ColorBegin:        mgl32.Vec3{1, 0.85, 0.35},
		ColorEnd:          mgl32.Vec3{0.5, 0.06, 0.01},
	},
}
