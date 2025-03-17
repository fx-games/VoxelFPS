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
    quadVertices *glhf.VertexSlice[glhf.GlUInt]
    quadsDrawn   int
    drawCalls    int

    animations         []SpriteAnimation
    spritesPerTexture  map[*glhf.Texture]map[*SpriteComponent]bool
    transparentSprites []*SpriteComponent
}

func NewSpriteDrawer(shader *glhf.Shader) *SpriteDrawer {
    return &SpriteDrawer{
        shader:            shader,
        spritesPerTexture: make(map[*glhf.Texture]map[*SpriteComponent]bool),
    }
}
func (q *SpriteDrawer) AddAnimation(animation SpriteAnimation) int {
    index := len(q.animations)
    q.animations = append(q.animations, animation)
    return index
}
func (q *SpriteDrawer) uploadQuad() {
    var indices []uint32
    var rawVertices []glhf.GlUInt
    changedIndices, quadVertices := q.getQuadVertices(indices, 0)
    indices = changedIndices
    rawVertices = append(rawVertices, quadVertices...)
    vertices := glhf.MakeUintIndexedVertexSlice(q.shader, 4, indices)
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
func (q *SpriteDrawer) getQuadVertices(indices []uint32, offset int) ([]uint32, []glhf.GlUInt) {
    indices = append(indices, uint32(offset+1), uint32(offset+0), uint32(offset+2), uint32(offset+2), uint32(offset+0), uint32(offset+3))
    quadVertices := []glhf.GlUInt{
        0, 1, 2, 3,
    }
    return indices, quadVertices
}

func (q *SpriteDrawer) Draw(camera *core.FPSCamera, elapsed float64) {
    q.shader.Begin()
    q.shader.SetUniformAttr(QuadShaderModelTransform, mgl32.Ident4())
    q.shader.SetUniformAttr(QuadShaderTintColor, mgl32.Vec4{1, 1, 1, 1})

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

        tileW, tileH := currentAtlas.GetAtlasItemSize()
        q.shader.SetUniformAttr(QuadShaderAtlasItemSize, [2]int32{int32(tileW), int32(tileH)})

        currentAtlas.Begin()

        for sprite := range spriteList {
            if sprite.IsDead(elapsed) {
                // mark for reuse
                q.Remove(sprite)
                continue
            }

            if !sprite.GameObject().IsInViewFrustum(camera) {
                continue
            }

            viewDirection := camera.GetViewDirection(sprite.Position(), sprite.LookDirection())

            texIndex, shouldShow := q.TextureIndex(sprite, viewDirection, elapsed)
            if !shouldShow {
                continue
            }

            q.drawSprite(camera, sprite, texIndex)
            spritesDrawn++
            drawCalls++
        }

        currentAtlas.End()
    }

    // transparent sprites should be drawn last and sorted by distance

    slices.SortStableFunc(q.transparentSprites, func(i, j *SpriteComponent) int {
        iPos := i.GameObject().Transform().GetPosition()
        jPos := j.GameObject().Transform().GetPosition()
        iDist := camera.GetPosition().Sub(iPos).LenSqr()
        jDist := camera.GetPosition().Sub(jPos).LenSqr()
        return cmp.Compare(jDist, iDist)
    })

    for i := len(q.transparentSprites) - 1; i >= 0; i-- {
        sprite := q.transparentSprites[i]
        sprite.AtlasTexture.Begin() // THIS COULD BE SLOW..

        if sprite.IsDead(elapsed) {
            // mark for reuse
            q.transparentSprites = append(q.transparentSprites[:i], q.transparentSprites[i+1:]...)
            continue
        }
        if !sprite.GameObject().IsInViewFrustum(camera) {
            continue
        }
        viewDirection := camera.GetViewDirection(sprite.Position(), sprite.LookDirection())
        texIndex, shouldShow := q.TextureIndex(sprite, viewDirection, elapsed)
        if !shouldShow {
            continue
        }
        q.drawSprite(camera, sprite, texIndex)
        spritesDrawn++
        drawCalls++

        sprite.AtlasTexture.End()
    }

    quad.End()

    q.shader.End()
    q.quadsDrawn = spritesDrawn
    q.drawCalls = drawCalls
}

func (q *SpriteDrawer) drawSprite(camera *core.FPSCamera, sprite *SpriteComponent, texIndex uint16) {
    q.shader.SetUniformAttr(QuadShaderModelTransform, sprite.Transform(camera))
    tintColor := sprite.TintColor()

    q.shader.SetUniformAttr(QuadShaderTintColor, tintColor)

    originOffset := mgl32.Vec2{0, 0} // offset zero = center of sprite
    if sprite.Origin() == SpriteOriginBottomCenter {
        originOffset = mgl32.Vec2{0, 0.5}
    }
    q.shader.SetUniformAttr(QuadShaderOriginOffset, originOffset)
    q.shader.SetUniformAttr(QuadShaderTextureIndex, uint32(texIndex))

    if sprite.DrawMode() == SpriteModeStatic {
        gl.Disable(gl.CULL_FACE)
    }
    if tintColor.W() < 1 {
        //gl.DepthMask(false)
    }
    q.quadVertices.Draw()
    if tintColor.W() < 1 {
        //gl.DepthMask(true)
    }
    if sprite.DrawMode() == SpriteModeStatic {
        gl.Enable(gl.CULL_FACE)
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
    if _, ok := q.spritesPerTexture[sprite.Atlas()]; !ok {
        q.spritesPerTexture[sprite.Atlas()] = make(map[*SpriteComponent]bool)
    }
    q.spritesPerTexture[sprite.Atlas()][sprite] = true
}

func (q *SpriteDrawer) Remove(sprite *SpriteComponent) {
    delete(q.spritesPerTexture[sprite.Atlas()], sprite)
}

func (q *SpriteDrawer) TextureIndex(sprite *SpriteComponent, viewDirection int, elapsed float64) (uint16, bool) {
    currentAnimation := sprite.CurrentAnimation()
    if currentAnimation < 0 || currentAnimation >= len(q.animations) {
        return sprite.TextureIndex(viewDirection, elapsed)
    }
    var frames []uint16

    showTimes := q.animations[currentAnimation].ShowTimeInSeconds
    if len(q.animations[currentAnimation].Frames) == 1 {
        frames = q.animations[currentAnimation].Frames[0]
    } else {
        frames = q.animations[currentAnimation].Frames[viewDirection]
    }
    return sprite.TextureIndexAnimated(frames, showTimes, elapsed)
}

func (q *SpriteDrawer) DrawAABBs(lineShader *glhf.Shader, cam *core.FPSCamera) {
    for _, spriteIndices := range q.spritesPerTexture {
        for sprite := range spriteIndices {
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
        for sprite := range spriteIndices {
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
    q.spritesPerTexture = make(map[*glhf.Texture]map[*SpriteComponent]bool)
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
