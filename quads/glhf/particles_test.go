package glhf

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// writeParticle is the only part of the emit path that does not need a GL context,
// and it is the part that is easy to get wrong: it writes raw floats at fixed
// offsets that have to match getParticleVertexFormat.
func TestWriteParticleMatchesVertexFormat(t *testing.T) {
	stride := getParticleVertexFormat().Size() / SizeOfFloat32
	dst := make([]GlFloat, stride)
	props := ParticleProperties{
		Origin:               mgl32.Vec3{1, 2, 3},
		VelocityFromPosition: func(origin, pos mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3{0, 5, 0} },
		SizeBegin:            0.5,
		Lifetime:             2,
		ColorBegin:           mgl32.Vec3{1, 0, 0},
		ColorVariation:       10, // must still land inside [0,1]
	}

	v := &ParticleSystem{}
	v.writeParticle(dst, props, 0)

	if got := float32(dst[3]); got != 2 {
		t.Errorf("lifetimeLeft = %v, want 2", got)
	}
	if got := float32(dst[5]); got != 5 {
		t.Errorf("velocity Y = %v, want 5", got)
	}
	if got := float32(dst[7]); got != 0.5 {
		t.Errorf("sizeBegin = %v, want 0.5", got)
	}
	for i := 8; i < 11; i++ {
		if c := float32(dst[i]); c < 0 || c > 1 {
			t.Errorf("colour component %d = %v, outside [0,1]", i-8, c)
		}
	}
	// The origin a particle carries is its own spawn point, which is what the
	// MaxDistance bounce in the feedback shader measures against.
	for i := 0; i < 3; i++ {
		if dst[i] != dst[11+i] {
			t.Errorf("origin component %d = %v, want the position %v", i, dst[11+i], dst[i])
		}
	}
}

// A burst emits hundreds of particles at once, so this runs mid-combat.
func BenchmarkWriteParticle(b *testing.B) {
	stride := getParticleVertexFormat().Size() / SizeOfFloat32
	dst := make([]GlFloat, stride)
	props := ParticleProperties{Origin: mgl32.Vec3{1, 2, 3}, SizeBegin: 0.5, Lifetime: 2}
	v := &ParticleSystem{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v.writeParticle(dst, props, i)
	}
}
