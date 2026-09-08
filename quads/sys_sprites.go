package quads

import (
	"cmp"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"slices"
)

type SpriteMode uint8

const (
	SpriteModeStatic SpriteMode = iota
	SpriteModeFollowHorizontal
	SpriteModeFollowHorizontalAndVertical
)

type SpriteOrigin uint8

const (
	SpriteOriginCenter SpriteOrigin = iota
	SpriteOriginBottomCenter
)

type SpriteAnimation struct {
	Frames            [][]uint16 // direction, frame -> texture index
	ShowTimeInSeconds []float32  // frame -> time in seconds
	Loop              bool
}

type SpriteDrawer struct {
	shader       *glhf.Shader
	quadVertices *glhf.VertexArray[glhf.GlUInt]
	quadsDrawn   int
	drawCalls    int

	animations         []SpriteAnimation
	spritesPerTexture  map[glhf.TextureAtlas][]*SpriteComponent
	transparentSprites []*SpriteComponent

	// Decals are quads stuck to the world geometry - see decals.go. They live
	// here because they are drawn with the same quad vertex array and shader.
	decals     []decal
	decalAtlas glhf.TextureAtlas
	nextDecal  int

	// Instance attribute scratch, reused every frame so the per-frame draw does
	// not allocate. Three batches: a static sprite needs face culling off, and
	// an additive one needs a blend function of its own.
	culledBatch   []float32
	unculledBatch []float32
	additiveBatch []float32
}

func NewSpriteDrawer(shader *glhf.Shader) *SpriteDrawer {
	return &SpriteDrawer{
		shader:            shader,
		spritesPerTexture: make(map[glhf.TextureAtlas][]*SpriteComponent),
	}
}
func (q *SpriteDrawer) AddAnimation(animation SpriteAnimation) int {
	index := len(q.animations)
	q.animations = append(q.animations, animation)
	return index
}
func (q *SpriteDrawer) uploadQuad() {
	var rawVertices []glhf.GlUInt
	changedIndices, quadVertices := NewUInt32QuadIndexedVerts()
	rawVertices = append(rawVertices, quadVertices...)
	vertices := glhf.NewVertexArray[glhf.GlUInt](q.shader, false)
	vertices.DebugName = "SpriteDrawer"
	vertices.SetIndices32(changedIndices)
	vertices.SetInstanceFormat(QuadInstanceFormat)
	vertices.Begin()
	vertices.SetVertexData(rawVertices)
	vertices.End()

	q.quadVertices = vertices
}

/*
0 -> leftU, topV
1 -> rightU, topV
2 -> rightU, bottomV
3 -> leftU, bottomV
*/
func (q *SpriteDrawer) Update(elapsed float64) {
	for atlas, sprites := range q.spritesPerTexture {
		for i := len(sprites) - 1; i >= 0; i-- {
			sprite := sprites[i]
			if sprite.IsDead(elapsed) {
				sprites = swapRemove(sprites, i)
				continue
			}
			q.updateSprite(sprite, elapsed)
		}
		q.spritesPerTexture[atlas] = sprites
	}

	for i := len(q.transparentSprites) - 1; i >= 0; i-- {
		sprite := q.transparentSprites[i]
		if sprite.IsDead(elapsed) {
			// Order matters here - the pass draws farthest first - so this one
			// shifts instead of swapping, and a dead sprite is not updated.
			q.transparentSprites = append(q.transparentSprites[:i], q.transparentSprites[i+1:]...)
			continue
		}
		q.updateSprite(sprite, elapsed)
	}
}
func (q *SpriteDrawer) updateSprite(sprite *SpriteComponent, elapsed float64) {
	currentAnimation := sprite.CurrentAnimation()
	if currentAnimation < 0 || currentAnimation >= len(q.animations) {
		return
	}
	var frames []uint16

	showTimes := q.animations[currentAnimation].ShowTimeInSeconds
	frames = q.animations[currentAnimation].Frames[0]
	sprite.Update(frames, showTimes, elapsed)
}

// TODO: This should not change when drawing the sprites multiple times in the same frame
func (q *SpriteDrawer) textureIndex(sprite *SpriteComponent, viewDirection int) (uint16, bool) {
	currentAnimation := sprite.CurrentAnimation()
	if currentAnimation < 0 || currentAnimation >= len(q.animations) {
		return sprite.TextureIndex(viewDirection)
	}
	var frames []uint16

	if len(q.animations[currentAnimation].Frames) == 1 {
		frames = q.animations[currentAnimation].Frames[0]
	} else {
		frames = q.animations[currentAnimation].Frames[viewDirection]
	}
	return sprite.TextureIndexAnimated(frames)
}

// Draw paints the opaque sprites. forShadows leaves out the ones that do not
// cast, which is the same set that is left out of the scene signature.
func (q *SpriteDrawer) Draw(observer core.Observer, spriteShader *glhf.Shader, forShadows bool) {
	// A shadow map wants the depth the sprite actually has; only the pass the
	// player looks at is biased.
	if !forShadows {
		gl.Enable(gl.POLYGON_OFFSET_FILL)
		gl.PolygonOffset(spriteDepthBiasFactor, spriteDepthBiasUnitsOpaque)
		defer func() {
			gl.PolygonOffset(0, 0)
			gl.Disable(gl.POLYGON_OFFSET_FILL)
		}()
	}
	spriteShader.Begin()

	if q.quadVertices == nil {
		q.uploadQuad()
	}

	quad := q.quadVertices
	quad.Begin()

	spritesDrawn := 0
	drawCalls := 0
	for currentAtlas, spriteList := range q.spritesPerTexture {
		if len(spriteList) == 0 {
			continue
		}

		currentAtlas.BeginOnUnit(gl.TEXTURE0)

		q.culledBatch = q.culledBatch[:0]
		q.unculledBatch = q.unculledBatch[:0]
		q.additiveBatch = q.additiveBatch[:0]
		for _, sprite := range spriteList {
			if forShadows && sprite.NoShadow {
				continue
			}
			if !sprite.GameObject().IsInViewFrustum(observer) {
				continue
			}

			viewDirection := core.GetViewDirection(observer.GetPosition(), sprite.Position(), sprite.LookDirection())
			// TODO: This should not change when drawing the sprites multiple times in the same frame
			texIndex, shouldShow := q.textureIndex(sprite, viewDirection)
			if !shouldShow {
				continue
			}
			// A static sprite keeps its own orientation, so the observer can end
			// up behind it and its back face has to survive culling. A billboard
			// is turned to face the observer and never shows its back.
			switch {
			case sprite.Additive:
				q.additiveBatch = appendInstance(q.additiveBatch, observer, sprite, texIndex)
			case sprite.DrawMode() == SpriteModeStatic:
				q.unculledBatch = appendInstance(q.unculledBatch, observer, sprite, texIndex)
			default:
				q.culledBatch = appendInstance(q.culledBatch, observer, sprite, texIndex)
			}
			spritesDrawn++
		}

		drawCalls += q.drawBatch(q.culledBatch, true)
		drawCalls += q.drawBatch(q.unculledBatch, false)
		drawCalls += q.drawAdditiveBatch(q.additiveBatch, spriteDepthBiasUnitsOpaque)

		currentAtlas.End()
	}

	quad.End()

	spriteShader.End()
	q.quadsDrawn = spritesDrawn
	q.drawCalls = drawCalls
}

// DrawTransparent draws the alpha-blended sprites. It is a separate pass from Draw
// because the skybox has to land in the framebuffer between the two: a translucent
// sprite drawn before the sky blends against the clear colour and gets a halo.
func (q *SpriteDrawer) DrawTransparent(observer core.Observer, spriteShader *glhf.Shader, forShadows bool) {
	if len(q.transparentSprites) == 0 {
		return
	}
	if !forShadows {
		gl.Enable(gl.POLYGON_OFFSET_FILL)
		gl.PolygonOffset(spriteDepthBiasFactor, spriteDepthBiasUnits)
		defer func() {
			gl.PolygonOffset(0, 0)
			gl.Disable(gl.POLYGON_OFFSET_FILL)
		}()
	}
	spriteShader.Begin()

	if q.quadVertices == nil {
		q.uploadQuad()
	}
	quad := q.quadVertices
	quad.Begin()

	spritesDrawn := 0
	drawCalls := 0

	// Transparent sprites are drawn last, farthest first, and none of them write
	// depth any more - see the DepthMask(false) in flush() below. They used to:
	// the nearest of a pile of overlapping smoke puffs wrote depth and the ones
	// behind it failed the depth test outright, so a dozen puffs stacked in the
	// same spot did not double-blend into something darker than any one of them.
	// That worked for a pile of the *same* translucent effect, and broke for two
	// *different* ones sharing a spawn point instead - a frag grenade's own
	// smoke column and its fireball land on the same call-site position (see
	// spawnJitter's own comment for why even a position nudge and three tiers of
	// polygon-offset still left this ties-with-itself at some camera distances),
	// and a translucent sprite that writes depth there is one that can tie with
	// another sprite's depth exactly, which a GPU resolves inconsistently
	// per-pixel - the fine Moire pattern a screenshot of exactly this pairing
	// showed, at two different distances, twice. Not writing depth trades the
	// old cosmetic risk (a big pile of the same translucent effect reads a shade
	// darker than intended) for never being able to tie with anything, which
	// for effects that are almost always one or two sprites deep rather than a
	// hundred is the safer default. Sorting back-to-front still keeps blending
	// correct without it: farthest drawn first means each later, nearer sprite
	// blends over what is already there in the right order.
	slices.SortStableFunc(q.transparentSprites, func(i, j *SpriteComponent) int {
		iPos := i.GameObject().Transform().GetPosition()
		jPos := j.GameObject().Transform().GetPosition()
		iDist := observer.GetPosition().Sub(iPos).LenSqr()
		jDist := observer.GetPosition().Sub(jPos).LenSqr()
		return cmp.Compare(jDist, iDist)
	})

	// Blending needs the back-to-front order kept, so this batches runs of
	// neighbours that share an atlas rather than sorting by texture. That used
	// to be one draw call per sprite, which was fine while transparent sprites
	// were a handful; everything a weapon throws into the world is translucent
	// smoke and it comes out of one shared atlas, so a run is now usually the
	// whole list.
	// A run also ends where the render style changes, because an additive
	// sprite is drawn with a blend function of its own.
	var runAtlas glhf.TextureAtlas
	runAdditive := false
	q.culledBatch = q.culledBatch[:0]
	flush := func() {
		if runAtlas == nil || len(q.culledBatch) == 0 {
			return
		}
		runAtlas.Begin()
		if runAdditive {
			drawCalls += q.drawAdditiveBatch(q.culledBatch, spriteDepthBiasUnits)
		} else {
			gl.DepthMask(false)
			drawCalls += q.drawBatch(q.culledBatch, true)
			gl.DepthMask(true)
		}
		runAtlas.End()
		q.culledBatch = q.culledBatch[:0]
	}

	for i := 0; i < len(q.transparentSprites); i++ {
		sprite := q.transparentSprites[i]
		if forShadows && sprite.NoShadow {
			continue
		}
		if !sprite.GameObject().IsInViewFrustum(observer) {
			continue
		}
		viewDirection := core.GetViewDirection(observer.GetPosition(), sprite.Position(), sprite.LookDirection())
		// TODO: This should not change when drawing the sprites multiple times in the same frame
		texIndex, shouldShow := q.textureIndex(sprite, viewDirection)
		if !shouldShow {
			continue
		}
		// A static sprite needs culling off, so it ends the run as surely as a
		// different atlas would.
		if sprite.AtlasTexture != runAtlas || sprite.Additive != runAdditive ||
			sprite.DrawMode() == SpriteModeStatic {
			flush()
			runAtlas = sprite.AtlasTexture
			runAdditive = sprite.Additive
		}
		q.culledBatch = appendInstance(q.culledBatch, observer, sprite, texIndex)
		spritesDrawn++
		if sprite.DrawMode() == SpriteModeStatic {
			runAtlas.Begin()
			gl.DepthMask(false)
			drawCalls += q.drawBatch(q.culledBatch, false)
			gl.DepthMask(true)
			runAtlas.End()
			q.culledBatch = q.culledBatch[:0]
			runAtlas = nil
		}
	}
	flush()

	quad.End()

	spriteShader.End()
	q.quadsDrawn += spritesDrawn
	q.drawCalls += drawCalls
}

// appendInstance packs one sprite into the instance attribute buffer, in the order
// given by QuadInstanceFormat: mat4 model, vec2 origin offset, vec4 uv, emissive.
//
// The UV rect is worked out here rather than in the shader. A fixed grid can be
// divided up from a tile size, but a packed atlas cannot, and both kinds answer
// GetUV, so asking is both simpler and general.
func appendInstance(dst []float32, observer core.Transformer, sprite *SpriteComponent, texIndex uint16) []float32 {
	model := sprite.Transform(observer)

	originOffset := mgl32.Vec2{0, 0} // offset zero = center of sprite
	switch {
	case sprite.PivotOverride != nil:
		originOffset = *sprite.PivotOverride
	case sprite.Origin() == SpriteOriginBottomCenter:
		originOffset = mgl32.Vec2{0, 0.5}
	}
	leftU, topV, rightU, bottomV := sprite.AtlasTexture.GetUV(int(texIndex))

	dst = append(dst, model[:]...)
	return append(dst, originOffset[0], originOffset[1],
		float32(leftU), float32(topV), float32(rightU), float32(bottomV),
		sprite.Emissive, sprite.BakedLight)
}

// drawAdditiveBatch is the same draw under ZDoom's Add render style: what the
// sprite paints is added to the scene rather than blended over it, and it does
// not write depth. Both halves matter. Blended, a muzzle haze authored at six
// percent alpha darkens everything behind it instead of glowing, and forty
// overlapping puffs of it turn a lit room black. Writing depth, the nearest
// puff would hide every sprite drawn after it, because a batch is not sorted.
// restoreUnits is the bias the caller's own batches were drawn with and want
// back once the stronger additive one is done - Draw() runs at
// spriteDepthBiasUnitsOpaque, DrawTransparent() at spriteDepthBiasUnits, and
// hardcoding either here silently left the other's later batches (the next
// atlas's opaque sprites, in Draw()'s case) drawn at the wrong bias for the
// rest of the call.
func (q *SpriteDrawer) drawAdditiveBatch(instances []float32, restoreUnits float32) int {
	if len(instances) == 0 {
		return 0
	}
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE)
	gl.DepthMask(false)
	gl.PolygonOffset(spriteDepthBiasFactor, spriteDepthBiasUnitsAdditive)
	calls := q.drawBatch(instances, true)
	gl.PolygonOffset(spriteDepthBiasFactor, restoreUnits)
	gl.DepthMask(true)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	return calls
}

// spriteDepthBias pulls every world sprite a little towards the viewer before
// it is depth tested, which is GZDoom's own `state.SetDepthBias(-1, -128)` on
// the masked sprite pass and the same two numbers.
//
// It does not stop a sprite being cut by geometry - GZDoom's are cut too, and
// a big flash standing in a doorway is sliced there as well. What it stops is
// the other half of the problem: an effect sat flush against the wall or floor
// it belongs to, which without a bias z-fights the surface and comes out half
// eaten by it, one row of pixels at a time as the camera moves.
//
// ponytail: the constant slope-scaled pair, not a distance-aware one. GZDoom
// has run these two numbers for every sprite in every map for years.
const (
	spriteDepthBiasFactor = -1
	spriteDepthBiasUnits  = -128
	// spriteDepthBiasUnitsAdditive is stronger than the ordinary bias above, so
	// an additive sprite (a burst of fire) sits reliably closer to the viewer
	// than a translucent one (its smoke) or an opaque one (a fireball) at the
	// exact same spawn point - A_SpawnItem and A_CustomMissile both land there
	// with no offset of their own. This decides ordering against a sprite that
	// still writes real depth (an opaque one, or real world geometry); it is no
	// longer what keeps additive from tying with translucent, because
	// translucent does not write depth at all any more (see DrawTransparent) -
	// nothing for it to tie with.
	spriteDepthBiasUnitsAdditive = -256
	// spriteDepthBiasUnitsOpaque sits between the other two, so an ordinary
	// (non-additive, fully opaque) sprite - FragGrenadeExplosion's own fireball
	// frames, alpha 1, no render style - draws in front of a translucent one
	// (its smoke) testing against it at the same spawn point, the same way
	// additive already does against both. The fireball is spawn_item'd
	// alongside BarrelExplosionSmokeColumn at the very same position and,
	// unlike SmallExplosionFlames, has no Speed to drift away on: it sits there
	// through several seconds of held frames while the smoke drifts and fades
	// near it.
	spriteDepthBiasUnitsOpaque = -192
)

// drawBatch submits one instanced draw and reports the number of draw calls it cost.
func (q *SpriteDrawer) drawBatch(instances []float32, cullFace bool) int {
	if len(instances) == 0 {
		return 0
	}
	if !cullFace {
		gl.Disable(gl.CULL_FACE)
	}
	q.quadVertices.SetInstanceData(instances)
	q.quadVertices.DrawInstanced(int32(len(instances) / QuadInstanceFloats))
	if !cullFace {
		gl.Enable(gl.CULL_FACE)
	}
	return 1
}

// ForEachCaster visits the world position of every sprite that ends up in a
// shadow map. Used to notice when the casting scene changed.
func (q *SpriteDrawer) ForEachCaster(visit func(pos mgl32.Vec3)) {
	for _, spriteList := range q.spritesPerTexture {
		for _, sprite := range spriteList {
			if !sprite.NoShadow {
				visit(sprite.Position())
			}
		}
	}
	for _, sprite := range q.transparentSprites {
		if !sprite.NoShadow {
			visit(sprite.Position())
		}
	}
}

// SpriteCount is how many sprites the drawer is keeping, drawn or not, which is
// a different number from how many it drew and the one that says whether
// something is piling up.
func (q *SpriteDrawer) SpriteCount() int {
	total := len(q.transparentSprites)
	for _, sprites := range q.spritesPerTexture {
		total += len(sprites)
	}
	return total
}

func (q *SpriteDrawer) DrawnLastFrame() (int, int) {
	return q.quadsDrawn * 2, q.drawCalls
}

func (q *SpriteDrawer) Add(sprite *SpriteComponent) {
	if sprite.Tint.W() < 1 {
		q.transparentSprites = append(q.transparentSprites, sprite)
		return
	}
	q.spritesPerTexture[sprite.Atlas()] = append(q.spritesPerTexture[sprite.Atlas()], sprite)
}

func (q *SpriteDrawer) Remove(sprite *SpriteComponent) {
	atlas := sprite.Atlas()
	list := q.spritesPerTexture[atlas]
	// ponytail: linear scan. The lists are per atlas and short; a lookup index
	// only pays off if removals start showing up in a profile.
	for i, candidate := range list {
		if candidate == sprite {
			q.spritesPerTexture[atlas] = swapRemove(list, i)
			return
		}
	}
}

// swapRemove drops index i by moving the last element into its place. The draw
// lists are unordered, so the cheap removal is the right one.
func swapRemove[T any](list []T, i int) []T {
	list[i] = list[len(list)-1]
	var zero T
	list[len(list)-1] = zero
	return list[:len(list)-1]
}

// The debug boxes are culled the way the sprites themselves are. They were not,
// and a box behind the player costs the same as one in front of it: a stress map
// of a thousand barrels queued a thousand boxes a frame to draw the six that
// were on screen.
func (q *SpriteDrawer) DrawAABBs(lineShader *glhf.Shader, cam *core.FPSCamera) {
	defer voxel.DrawDebugLines(lineShader)
	for _, spriteIndices := range q.spritesPerTexture {
		for _, sprite := range spriteIndices {
			if !q.hasDebugBox(sprite, cam) {
				continue
			}
			collObj := sprite.GameObject().Physics()
			// The box is in the space of the sprite's own quad, which is the
			// world only while the sprite is drawn at its natural size. A
			// pickup on a shelf is a fraction of that, and its box was drawn a
			// whole voxel wide. The quad's own scale is what the transform
			// applies; the third axis is not scaled there either.
			collObj.AABBAt(mgl32.Vec3{}).
				Scaled(mgl32.Vec3{sprite.Size.X(), sprite.Size.Y(), 1}).
				QueueDebugDraw(sprite.GameObject().Transform().GetTranslationMatrix())
		}
	}
}
func (q *SpriteDrawer) DrawOBBs(lineShader *glhf.Shader, cam *core.FPSCamera) {
	defer voxel.DrawDebugLines(lineShader)
	for _, spriteIndices := range q.spritesPerTexture {
		for _, sprite := range spriteIndices {
			if !q.hasDebugBox(sprite, cam) {
				continue
			}
			obb := sprite.GameObject().Physics().OBB(cam)
			obb.AABB.QueueDebugDraw(obb.Transform)
		}
	}
}

// hasDebugBox is whether this sprite has a box to draw and is somewhere the
// player can see it.
func (q *SpriteDrawer) hasDebugBox(sprite *SpriteComponent, frustum core.Culler) bool {
	return !sprite.Hidden &&
		sprite.GameObject().HasComponent(ComponentTypePhysics) &&
		sprite.GameObject().IsInViewFrustum(frustum)
}
func (q *SpriteDrawer) Clear() {
	q.spritesPerTexture = make(map[glhf.TextureAtlas][]*SpriteComponent)
	q.ClearDecals()
}

func (q *SpriteDrawer) GetFirstFrameIndex(anim int, viewDirection int) uint16 {
	return q.animations[anim].Frames[viewDirection][0]
}

func (q *SpriteDrawer) GetLastFrameIndex(anim int, viewDirection int) uint16 {
	frames := q.animations[anim].Frames[viewDirection]
	return frames[len(frames)-1]

}

func (q *SpriteDrawer) AddTransparent(sprite *SpriteComponent) {
	q.transparentSprites = append(q.transparentSprites, sprite)
}
