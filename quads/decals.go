package quads

import (
	"math"
	"math/rand"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
)

// Decal texture indices in the sprite atlas. The pixels are written by
// `go run ./tools/gendecals`.
const (
	DecalBulletHole    = uint16(185)
	DecalBloodSplatter = uint16(186)
)

// MaxDecals is how many decals stay on the walls. Past that the oldest one is
// overwritten, which keeps the instance buffer a fixed size and the pass one
// draw call no matter how long the shooting goes on.
const MaxDecals = 256

// decalSurfaceOffset lifts a decal off the surface it sticks to, so it wins the
// depth test against the wall instead of z-fighting with it. In world units,
// where a voxel is 1.
const decalSurfaceOffset = float32(0.01)

// A decal is a quad glued to a surface: a bullet hole or a splatter. It is not a
// game object and not a sprite - it never moves, never animates and never needs
// an update - so it is only the model matrix that puts the quad on the wall plus
// the atlas tile to stamp there. That is exactly one instance in the quad
// shader's instance buffer, which is why decals cost one draw call in total.
type decal struct {
	model    mgl32.Mat4
	center   mgl32.Vec3
	radius   float32
	texIndex uint16
}

// AddDecal sticks a decal on the surface at pos, facing along normal. size is the
// edge length of the quad in world units. The quad is rolled by a random angle
// around the normal so repeated hits on the same wall do not line up.
func (q *SpriteDrawer) AddDecal(atlas *glhf.TextureAtlasFixed, texIndex uint16, pos, normal mgl32.Vec3, size float32) {
	q.addDecal(atlas, texIndex, pos, normal, size, rand.Float32()*2*math.Pi)
}

// ponytail: one atlas for all decals, kept in a single list. Grouping by atlas
// only pays off once decals come from more than one sheet.
func (q *SpriteDrawer) addDecal(atlas *glhf.TextureAtlasFixed, texIndex uint16, pos, normal mgl32.Vec3, size, roll float32) {
	if normal.Len() == 0 {
		return
	}
	q.decalAtlas = atlas
	newDecal := decal{
		model:    DecalModel(pos, normal, size, roll),
		center:   pos.Add(normal.Normalize().Mul(decalSurfaceOffset)),
		radius:   size * 0.71, // half the quad's diagonal
		texIndex: texIndex,
	}
	if len(q.decals) < MaxDecals {
		q.decals = append(q.decals, newDecal)
		return
	}
	q.decals[q.nextDecal] = newDecal
	q.nextDecal = (q.nextDecal + 1) % MaxDecals
}

// DecalModel is the model matrix that puts a unit quad on a surface: rotated so
// its local +Z - the direction the quad shader hands the lighting as the normal -
// points along normal, rolled around that axis, scaled to size and moved onto the
// surface with a small offset so it does not z-fight the wall.
func DecalModel(pos, normal mgl32.Vec3, size, roll float32) mgl32.Mat4 {
	facing := normal.Normalize()
	alignment := mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, 1}, facing).Mat4()
	offsetPos := pos.Add(facing.Mul(decalSurfaceOffset))
	return mgl32.Translate3D(offsetPos.X(), offsetPos.Y(), offsetPos.Z()).
		Mul4(alignment).
		Mul4(mgl32.HomogRotate3DZ(roll)).
		Mul4(mgl32.Scale3D(size, size, size))
}

// DrawDecals draws every decal in one instanced call. It is deliberately left out
// of the shadow depth pass: a decal sits a centimetre off the wall it is stuck to,
// so putting it in a light's depth map would shadow the very surface it decorates.
func (q *SpriteDrawer) DrawDecals(observer core.Observer, spriteShader *glhf.Shader) {
	if len(q.decals) == 0 || q.decalAtlas == nil {
		return
	}
	spriteShader.Begin()
	if q.quadVertices == nil {
		q.uploadQuad()
	}
	q.quadVertices.Begin()

	tileW, tileH := q.decalAtlas.GetAtlasItemSize()
	spriteShader.SetUniformAttr(QuadShaderAtlasItemSize, [2]int32{int32(tileW), int32(tileH)})
	q.decalAtlas.BeginOnUnit(gl.TEXTURE0)

	// Oldest first, so a fresh decal blends over the ones it lands on. nextDecal
	// is the oldest slot once the ring has wrapped, and 0 while it is still
	// filling up, which is the insertion order either way.
	q.culledBatch = q.culledBatch[:0]
	for i := range q.decals {
		d := &q.decals[(q.nextDecal+i)%len(q.decals)]
		if !observer.IsSphereInViewFrustum(d.center, d.radius) {
			continue
		}
		q.culledBatch = append(q.culledBatch, d.model[:]...)
		// origin offset (centred), texture index, emissive
		q.culledBatch = append(q.culledBatch, 0, 0, float32(d.texIndex), 0)
	}
	q.quadsDrawn += len(q.culledBatch) / QuadInstanceFloats
	// No depth writes. Every decal on one wall sits at the same distance from it,
	// so two overlapping ones have all but identical depth and z-fight for the
	// pixels they share. Nothing needs to be occluded by a decal - they are the
	// last thing drawn onto the geometry - so the fight is settled by not
	// entering the fight: they depth-test against the wall and blend against
	// each other in the order they were shot.
	gl.DepthMask(false)
	// Culling off: a decal keeps its own orientation and the observer can end up
	// looking at the wall from the side the quad's back face is on.
	q.drawCalls += q.drawBatch(q.culledBatch, false)
	gl.DepthMask(true)

	q.decalAtlas.End()
	q.quadVertices.End()
	spriteShader.End()
}

// ClearDecals wipes the walls, used when a new map is loaded.
func (q *SpriteDrawer) ClearDecals() {
	q.decals = q.decals[:0]
	q.nextDecal = 0
}
