package quads

import (
	"math"
	"math/rand"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
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
	model  mgl32.Mat4
	center mgl32.Vec3
	radius float32
	// light is the flood-filled voxel light where it is stuck, so a bullet
	// hole in a lit room is as lit as the wall around it. Whoever sticks it
	// there samples it; the drawer has no map to ask.
	light    float32
	texIndex uint16
}

// AddDecal sticks a decal on the surface at pos, facing along normal. width and
// height are the quad's size in world units - a decal is its own picture's
// shape, and ZDoom's bullet chips are seven pixels by five to nine, so a square
// is wrong for all but one of them - and light is the baked light level there,
// 0 to 15. The quad is rolled by a random angle around the normal so repeated
// hits on the same wall do not line up.
func (q *SpriteDrawer) AddDecal(atlas glhf.TextureAtlas, texIndex uint16, pos, normal mgl32.Vec3, width, height, light float32) {
	q.addDecal(atlas, texIndex, pos, normal, width, height, rand.Float32()*2*math.Pi, light)
}

// ponytail: one atlas for all decals, kept in a single list. Grouping by atlas
// only pays off once decals come from more than one sheet, and both the bullet
// chips and the blood live in the shared picture store with everything else a
// weapon throws at a wall.
func (q *SpriteDrawer) addDecal(atlas glhf.TextureAtlas, texIndex uint16, pos, normal mgl32.Vec3, width, height, roll, light float32) {
	if normal.Len() == 0 {
		return
	}
	q.decalAtlas = atlas
	newDecal := decal{
		model:  DecalModel(pos, normal, width, height, roll),
		center: pos.Add(normal.Normalize().Mul(decalSurfaceOffset)),
		// Half the quad's diagonal, which is what the frustum test needs and
		// what a roll around the normal sweeps the corners through.
		radius:   float32(math.Hypot(float64(width), float64(height))) * 0.5,
		light:    light,
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
// points along normal, rolled around that axis, scaled to width by height and
// moved onto the surface with a small offset so it does not z-fight the wall.
func DecalModel(pos, normal mgl32.Vec3, width, height, roll float32) mgl32.Mat4 {
	facing := normal.Normalize()
	alignment := mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, 1}, facing).Mat4()
	offsetPos := pos.Add(facing.Mul(decalSurfaceOffset))
	return mgl32.Translate3D(offsetPos.X(), offsetPos.Y(), offsetPos.Z()).
		Mul4(alignment).
		Mul4(mgl32.HomogRotate3DZ(roll)).
		Mul4(mgl32.Scale3D(width, height, 1))
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
		leftU, topV, rightU, bottomV := q.decalAtlas.GetUV(int(d.texIndex))
		q.culledBatch = appendDecalInstance(q.culledBatch, d,
			float32(leftU), float32(topV), float32(rightU), float32(bottomV))
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

// appendDecalInstance packs one decal into the instance buffer: the model
// matrix, then every other field QuadInstanceFormat names, in its order.
//
// Every field of it. A short instance is not a decal with a default: the buffer
// is a flat run of floats read a fixed stride at a time, so leaving the last
// one off shifted every decal after the first into the one before it and lost
// the last one entirely - which is why no bullet hole was drawn at all from
// the day sprites learned their baked light until this was found.
func appendDecalInstance(dst []float32, d *decal, leftU, topV, rightU, bottomV float32) []float32 {
	dst = append(dst, d.model[:]...)
	// origin offset (centred), uv rect, emissive, baked light
	return append(dst, 0, 0, leftU, topV, rightU, bottomV, 0, d.light)
}

// ClearDecals wipes the walls, used when a new map is loaded.
func (q *SpriteDrawer) ClearDecals() {
	q.decals = q.decals[:0]
	q.nextDecal = 0
}
