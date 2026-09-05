package quads

import (
	"cmp"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
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
	decalAtlas *glhf.TextureAtlasFixed
	nextDecal  int

	// Instance attribute scratch, reused every frame so the per-frame draw does
	// not allocate. Two batches because a static sprite needs face culling off.
	culledBatch   []float32
	unculledBatch []float32
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

func (q *SpriteDrawer) Draw(observer core.Observer, spriteShader *glhf.Shader) {
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
		for _, sprite := range spriteList {
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
			if sprite.DrawMode() == SpriteModeStatic {
				q.unculledBatch = appendInstance(q.unculledBatch, observer, sprite, texIndex)
			} else {
				q.culledBatch = appendInstance(q.culledBatch, observer, sprite, texIndex)
			}
			spritesDrawn++
		}

		drawCalls += q.drawBatch(q.culledBatch, true)
		drawCalls += q.drawBatch(q.unculledBatch, false)

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
func (q *SpriteDrawer) DrawTransparent(observer core.Observer, spriteShader *glhf.Shader) {
	if len(q.transparentSprites) == 0 {
		return
	}
	spriteShader.Begin()

	if q.quadVertices == nil {
		q.uploadQuad()
	}
	quad := q.quadVertices
	quad.Begin()

	spritesDrawn := 0
	drawCalls := 0

	// Transparent sprites are drawn last, farthest first. The sort puts the farthest
	// at index 0, so this has to iterate forwards: drawing nearest first let the near
	// sprite write depth and the ones behind it failed the depth test outright, so
	// they did not blend badly - they vanished.
	slices.SortStableFunc(q.transparentSprites, func(i, j *SpriteComponent) int {
		iPos := i.GameObject().Transform().GetPosition()
		jPos := j.GameObject().Transform().GetPosition()
		iDist := observer.GetPosition().Sub(iPos).LenSqr()
		jDist := observer.GetPosition().Sub(jPos).LenSqr()
		return cmp.Compare(jDist, iDist)
	})

	// ponytail: one instance per draw here. Blending needs the back-to-front order
	// kept and every sprite carries its own atlas, so batching would mean grouping
	// runs of equal atlas - worth it only once transparent sprites are more than a
	// handful. The opaque pass above is the one the benchmark measures.
	for i := 0; i < len(q.transparentSprites); i++ {
		sprite := q.transparentSprites[i]
		if !sprite.GameObject().IsInViewFrustum(observer) {
			continue
		}
		viewDirection := core.GetViewDirection(observer.GetPosition(), sprite.Position(), sprite.LookDirection())
		// TODO: This should not change when drawing the sprites multiple times in the same frame
		texIndex, shouldShow := q.textureIndex(sprite, viewDirection)
		if !shouldShow {
			continue
		}
		sprite.AtlasTexture.Begin() // THIS COULD BE SLOW..

		q.culledBatch = appendInstance(q.culledBatch[:0], observer, sprite, texIndex)
		drawCalls += q.drawBatch(q.culledBatch, sprite.DrawMode() != SpriteModeStatic)
		spritesDrawn++

		sprite.AtlasTexture.End()
	}

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
	if sprite.Origin() == SpriteOriginBottomCenter {
		originOffset = mgl32.Vec2{0, 0.5}
	}
	leftU, topV, rightU, bottomV := sprite.AtlasTexture.GetUV(int(texIndex))

	dst = append(dst, model[:]...)
	return append(dst, originOffset[0], originOffset[1],
		float32(leftU), float32(topV), float32(rightU), float32(bottomV),
		sprite.Emissive)
}

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
			visit(sprite.Position())
		}
	}
	for _, sprite := range q.transparentSprites {
		visit(sprite.Position())
	}
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

func (q *SpriteDrawer) DrawAABBs(lineShader *glhf.Shader, cam *core.FPSCamera) {
	for _, spriteIndices := range q.spritesPerTexture {
		for _, sprite := range spriteIndices {
			if !sprite.GameObject().HasComponent(ComponentTypePhysics) {
				continue
			}
			lineShader.SetUniformAttr(1, sprite.GameObject().Transform().GetTranslationMatrix())
			collObj := sprite.GameObject().Physics()
			collObj.AABBAt(mgl32.Vec3{}).Draw(lineShader)
		}
	}
}
func (q *SpriteDrawer) DrawOBBs(lineShader *glhf.Shader, cam *core.FPSCamera) {
	for _, spriteIndices := range q.spritesPerTexture {
		for _, sprite := range spriteIndices {
			if !sprite.GameObject().HasComponent(ComponentTypePhysics) {
				continue
			}
			collObj := sprite.GameObject().Physics()

			obb := collObj.OBB(cam)
			lineShader.SetUniformAttr(1, obb.Transform)
			obb.Draw(lineShader)
		}
	}
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
