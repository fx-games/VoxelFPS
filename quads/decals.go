package quads

import (
	"cmp"
	"math"
	"math/rand"
	"slices"

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

// A decal is a quad glued to a surface: a bullet hole or a splatter. It is not
// a game object and not a sprite - it never animates and never needs a per-tic
// update - so it is only the model matrix that puts the quad on the wall plus
// the atlas tile to stamp there. That is exactly one instance in the quad
// shader's instance buffer, which is why decals cost one draw call in total.
//
// parent is nil for the overwhelming majority stuck to a wall or other static
// terrain, in which case model and center are already world space and cost
// nothing further. A decal stuck to a moving GameObject instead - a door - has
// to follow it, so for those model and center are in parent's own local space
// and DrawDecals re-applies parent's current transform every frame, the same
// way a mesh's own vertices do.
type decal struct {
	model  mgl32.Mat4
	center mgl32.Vec3
	radius float32
	parent *GameObject
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
// hits on the same wall do not line up. parent is the GameObject the surface
// belongs to, or nil for raw terrain - see the decal type's own doc comment.
func (q *SpriteDrawer) AddDecal(atlas glhf.TextureAtlas, texIndex uint16, pos, normal mgl32.Vec3, width, height, light float32, parent *GameObject) {
	q.addDecal(atlas, texIndex, pos, normal, width, height, rand.Float32()*2*math.Pi, light, parent)
}

// ponytail: one atlas for all decals, kept in a single list. Grouping by atlas
// only pays off once decals come from more than one sheet, and both the bullet
// chips and the blood live in the shared picture store with everything else a
// weapon throws at a wall.
func (q *SpriteDrawer) addDecal(atlas glhf.TextureAtlas, texIndex uint16, pos, normal mgl32.Vec3, width, height, roll, light float32, parent *GameObject) {
	if normal.Len() == 0 {
		return
	}
	q.decalAtlas = atlas
	model := DecalModel(pos, normal, width, height, roll)
	center := pos.Add(normal.Normalize().Mul(decalSurfaceOffset))
	if parent != nil {
		// Store both relative to parent's transform as it is right now, so
		// DrawDecals can get back to the same world-space quad by reapplying
		// whatever that transform has become.
		parentTransform := parent.Transform().GetTransformMatrix()
		model = parentTransform.Inv().Mul4(model)
		center = parent.Transform().GetRelativePosition(center)
	}
	newDecal := decal{
		model:  model,
		center: center,
		// Half the quad's diagonal, which is what the frustum test needs and
		// what a roll around the normal sweeps the corners through.
		radius:   float32(math.Hypot(float64(width), float64(height))) * 0.5,
		parent:   parent,
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

// worldSpace is d's model matrix and centre, in world space right now: as
// stored for a static decal, or reapplied against parent's current transform
// for one stuck to a moving GameObject.
func (d *decal) worldSpace() (model mgl32.Mat4, center mgl32.Vec3) {
	if d.parent == nil {
		return d.model, d.center
	}
	parentTransform := d.parent.Transform().GetTransformMatrix()
	model = parentTransform.Mul4(d.model)
	center = parentTransform.Mul4x1(mgl32.Vec4{d.center.X(), d.center.Y(), d.center.Z(), 1}).Vec3()
	return model, center
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
	// Oldest first, so a fresh decal blends over the ones it lands on. nextDecal
	// is the oldest slot once the ring has wrapped, and 0 while it is still
	// filling up, which is the insertion order either way.
	q.drawQuads(observer, spriteShader, q.decalAtlas, q.decals, q.nextDecal)
}

// drawQuads is the one instanced call both decals and window panes come down
// to: everything in quads that survives the frustum, packed into the instance
// buffer starting at first and wrapping, and drawn blended without writing
// depth.
func (q *SpriteDrawer) drawQuads(observer core.Observer, spriteShader *glhf.Shader, atlas glhf.TextureAtlas, quads []decal, first int) {
	if len(quads) == 0 || atlas == nil {
		return
	}
	spriteShader.Begin()
	if q.quadVertices == nil {
		q.uploadQuad()
	}
	q.quadVertices.Begin()

	atlas.BeginOnUnit(gl.TEXTURE0)

	q.culledBatch = q.culledBatch[:0]
	for i := range quads {
		d := &quads[(first+i)%len(quads)]
		model, center := d.worldSpace()
		if !observer.IsSphereInViewFrustum(center, d.radius) {
			continue
		}
		leftU, topV, rightU, bottomV := atlas.GetUV(int(d.texIndex))
		q.culledBatch = appendDecalInstance(q.culledBatch, d, model,
			float32(leftU), float32(topV), float32(rightU), float32(bottomV))
	}
	q.quadsDrawn += len(q.culledBatch) / QuadInstanceFloats
	// No depth writes. Every decal on one wall sits at the same distance from it,
	// so two overlapping ones have all but identical depth and z-fight for the
	// pixels they share. Nothing needs to be occluded by one - they are the last
	// thing drawn onto the geometry - so the fight is settled by not entering it:
	// they depth-test against the wall and blend against each other in the order
	// they were shot. A pane has the same reason with the sky behind it instead.
	gl.DepthMask(false)
	// Culling off: these keep their own orientation and the observer can end up
	// looking at the surface from the side the quad's back face is on.
	q.drawCalls += q.drawBatch(q.culledBatch, false)
	gl.DepthMask(true)

	atlas.End()
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
func appendDecalInstance(dst []float32, d *decal, model mgl32.Mat4, leftU, topV, rightU, bottomV float32) []float32 {
	dst = append(dst, model[:]...)
	// origin offset (centred), uv rect, emissive, baked light
	return append(dst, 0, 0, leftU, topV, rightU, bottomV, 0, d.light)
}

// ClearDecals wipes the walls, used when a new map is loaded.
func (q *SpriteDrawer) ClearDecals() {
	q.decals = q.decals[:0]
	q.nextDecal = 0
}

// A pane is the same quad as a decal and is drawn by the same call, but it is
// part of the level rather than something that happened to it: the sheet of
// glass in a window. Three things differ. It is placed when the map loads, it
// is never overwritten by the next bullet hole - a ring of windows is more
// quads than MaxDecals holds - and it is drawn after the sky rather than
// before it, because what a window is for is showing the sky through it.

// AddPane glazes a window: a quad at pos facing along normal, width by height
// world units, lit by the baked light there. Unlike a decal it takes no roll
// and no parent - a window is square to its frame and does not move.
func (q *SpriteDrawer) AddPane(atlas glhf.TextureAtlas, texIndex uint16, pos, normal mgl32.Vec3, width, height, light float32) {
	if normal.Len() == 0 {
		return
	}
	q.paneAtlas = atlas
	q.panes = append(q.panes, decal{
		model:    DecalModel(pos, normal, width, height, 0),
		center:   pos.Add(normal.Normalize().Mul(decalSurfaceOffset)),
		radius:   float32(math.Hypot(float64(width), float64(height))) * 0.5,
		light:    light,
		texIndex: texIndex,
	})
}

// ClearPanes takes the glass out, used when a new map is loaded.
func (q *SpriteDrawer) ClearPanes() {
	q.panes = q.panes[:0]
	q.paneAtlas = nil
}

// DrawPanes draws every window the observer can see in one instanced call. It
// belongs after the skybox: a pane writes no depth, so a sky drawn later would
// paint straight over it.
func (q *SpriteDrawer) DrawPanes(observer core.Observer, spriteShader *glhf.Shader) {
	q.drawQuads(observer, spriteShader, q.paneAtlas, q.panesToDraw(observer), 0)
}

// panesToDraw is the glass in front of the observer, farthest first.
//
// Farthest first because none of these quads writes depth, so what is drawn last
// ends up on top whatever the distances say - and a station's windows are a
// ring, so the far side of it is behind the near side through the glass all day
// long. A decal never needs this: every decal on one wall is the same distance
// from it.
//
// Culled before it is sorted, not after. A level's glass is counted in
// thousands - the ark has 3626 panes - and sorting all of them to draw the
// twenty in shot was the whole cost of the pass. A pane takes no parent, so its
// centre is already world space and the cull is one sphere test each.
func (q *SpriteDrawer) panesToDraw(observer core.Observer) []decal {
	q.visiblePanes = q.visiblePanes[:0]
	for _, pane := range q.panes {
		if observer.IsSphereInViewFrustum(pane.center, pane.radius) {
			q.visiblePanes = append(q.visiblePanes, pane)
		}
	}
	eye := observer.GetPosition()
	slices.SortFunc(q.visiblePanes, func(a, b decal) int {
		return cmp.Compare(b.center.Sub(eye).LenSqr(), a.center.Sub(eye).LenSqr())
	})
	return q.visiblePanes
}
