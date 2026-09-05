package quads

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/stretchr/testify/assert"
)

// A decal has to end up flat on the surface it was shot at, facing away from it.
// The quad shader reads the normal as model * (0,0,1), so that is what is checked
// here, for every voxel face direction and both quad axes.
func TestDecalModelFacesTheSurface(t *testing.T) {
	normals := []mgl32.Vec3{
		{1, 0, 0}, {-1, 0, 0},
		{0, 1, 0}, {0, -1, 0},
		{0, 0, 1}, {0, 0, -1},
	}
	pos := mgl32.Vec3{3, 4, 5}
	for _, normal := range normals {
		model := DecalModel(pos, normal, 0.2, 1.1)

		facing := model.Mul4x1(mgl32.Vec4{0, 0, 1, 0}).Vec3().Normalize()
		assert.InDeltaSlicef(t, normal[:], facing[:], 1e-5, "normal %v", normal)

		// The quad's own plane must stay perpendicular to the normal, or the decal
		// would be tilted into the wall.
		right := model.Mul4x1(mgl32.Vec4{1, 0, 0, 0}).Vec3()
		up := model.Mul4x1(mgl32.Vec4{0, 1, 0, 0}).Vec3()
		assert.InDelta(t, 0, right.Dot(normal), 1e-5)
		assert.InDelta(t, 0, up.Dot(normal), 1e-5)

		// And it sits just off the wall, on the side the normal points to.
		center := model.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3()
		expected := pos.Add(normal.Mul(decalSurfaceOffset))
		assert.InDeltaSlicef(t, expected[:], center[:], 1e-5, "normal %v", normal)
	}
}

func TestDecalModelScalesTheQuad(t *testing.T) {
	const size = float32(0.4)
	model := DecalModel(mgl32.Vec3{}, mgl32.Vec3{0, 0, 1}, size, 0)
	// The quad is a unit square around its centre, so a corner is half a size away
	// on each axis.
	corner := model.Mul4x1(mgl32.Vec4{0.5, 0.5, 0, 1}).Vec3()
	assert.InDelta(t, size/2, corner.X(), 1e-5)
	assert.InDelta(t, size/2, corner.Y(), 1e-5)
}

// The ring buffer is what keeps the decal pass one fixed-size draw call: past
// MaxDecals the oldest decal is overwritten instead of the list growing.
func TestDecalRingBufferOverwritesOldest(t *testing.T) {
	drawer := NewSpriteDrawer(nil)
	for i := 0; i < MaxDecals+3; i++ {
		drawer.addDecal(nil, uint16(i), mgl32.Vec3{float32(i), 0, 0}, mgl32.Vec3{0, 1, 0}, 0.1, 0)
	}
	assert.Len(t, drawer.decals, MaxDecals)
	assert.Equal(t, 3, drawer.nextDecal)
	// The first three slots hold the three newest, the rest still hold their originals.
	assert.Equal(t, uint16(MaxDecals), drawer.decals[0].texIndex)
	assert.Equal(t, uint16(MaxDecals+2), drawer.decals[2].texIndex)
	assert.Equal(t, uint16(3), drawer.decals[3].texIndex)

	drawer.ClearDecals()
	assert.Empty(t, drawer.decals)
	assert.Equal(t, 0, drawer.nextDecal)
}

// A zero normal has no plane to lie in, so it is dropped rather than producing a
// NaN matrix that would take the whole instanced draw with it.
func TestDecalIgnoresZeroNormal(t *testing.T) {
	drawer := NewSpriteDrawer(nil)
	drawer.addDecal(nil, DecalBulletHole, mgl32.Vec3{1, 2, 3}, mgl32.Vec3{}, 0.1, 0)
	assert.Empty(t, drawer.decals)
}
