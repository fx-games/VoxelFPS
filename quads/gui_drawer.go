package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads/glhf"
)

type GuiQuad struct {
    Verts                *glhf.VertexSlice[glhf.GlFloat]
    currentAnimation     int
    textureIndex         uint16
    currentFrameTime     float32
    currentFrame         int
    changedFrameThisTick bool
    position             mgl32.Vec2
    isHidden             bool
    tintColor            mgl32.Vec4
    eventOnFrame         map[int]events.EventID
    bakedSize            mgl32.Vec2
    targetSize           mgl32.Vec2
}

func (q *GuiQuad) Draw() {
    q.Verts.Begin()
    q.Verts.Draw()
    q.Verts.End()
}

func (q *GuiQuad) CurrentAnimation() int {
    return q.currentAnimation
}

func (q *GuiQuad) TextureIndex(elapsed float64) uint16 {
    return q.textureIndex
}
func (q *GuiQuad) TextureIndexAnimated(frames []uint16, showTimes []float32, elapsed float64, loop bool) uint16 {
    q.currentFrameTime += float32(elapsed)
    q.changedFrameThisTick = false
    if q.currentFrameTime >= showTimes[q.currentFrame] {
        q.currentFrameTime = 0
        q.currentFrame += 1
        q.changedFrameThisTick = true
        if q.currentFrame >= len(frames) {
            q.currentFrame = 0
            if !loop {
                q.currentAnimation = -1
                return q.textureIndex
            }
        }
    }
    textureIndexToShowThisFrame := frames[q.currentFrame]
    if q.changedFrameThisTick {
        if event, ok := q.eventOnFrame[q.currentFrame]; ok {
            events.Publish(event, q)
        }
    }
    return textureIndexToShowThisFrame
}

func (q *GuiQuad) PlayAnimation(anim int) {
    q.currentAnimation = anim
    q.currentFrame = 0
    q.currentFrameTime = 0
}
func (q *GuiQuad) ClearEvents() {
    clear(q.eventOnFrame)
}
func (q *GuiQuad) PublishEventOnFrame(frame int, event events.EventID) {
    if q.eventOnFrame == nil {
        q.eventOnFrame = make(map[int]events.EventID)
    }
    q.eventOnFrame[frame] = event
}

func (q *GuiQuad) GetPosition() mgl32.Vec2 {
    return q.position
}

func (q *GuiQuad) SetPosition(offset mgl32.Vec2) {
    q.position = offset
}

func (q *GuiQuad) Hide() {
    q.isHidden = true
}

func (q *GuiQuad) Show() {
    q.isHidden = false
}

func (q *GuiQuad) SetTintColor(color mgl32.Vec4) {
    q.tintColor = color
}

func (q *GuiQuad) SetSize(width int, height int) {
    q.targetSize = mgl32.Vec2{float32(width), float32(height)}
}

func (q *GuiQuad) Scale() mgl32.Vec2 {
    return mgl32.Vec2{q.targetSize.X() / q.bakedSize.X(), q.targetSize.Y() / q.bakedSize.Y()}
}

type GuiDrawer struct {
    shader     *glhf.Shader
    quadsDrawn int
    drawCalls  int

    animations        []SpriteAnimation
    spritesPerTexture map[*glhf.Texture]map[*GuiQuad]bool
}

func NewGuiDrawer(shader *glhf.Shader) *GuiDrawer {
    return &GuiDrawer{
        shader:            shader,
        spritesPerTexture: make(map[*glhf.Texture]map[*GuiQuad]bool),
    }
}
func (q *GuiDrawer) AddAnimation(animation SpriteAnimation) int {
    index := len(q.animations)
    q.animations = append(q.animations, animation)
    return index
}
func (q *GuiDrawer) Draw(elapsed float64) {
    spritesDrawn := 0
    drawCalls := 0
    for currentAtlas, spriteIndices := range q.spritesPerTexture {
        currentAtlas.Begin()

        for sprite := range spriteIndices {
            if sprite.isHidden {
                continue
            }
            texIndex := q.TextureIndex(sprite, elapsed)
            baseTexIndex := sprite.textureIndex
            if texIndex != baseTexIndex {
                baseU, baseV, _, _ := currentAtlas.GetUV(baseTexIndex)
                leftU, topV, _, _ := currentAtlas.GetUV(texIndex)
                offU := leftU - baseU
                offV := topV - baseV
                q.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{float32(offU), float32(offV)})
            } else {
                q.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
            }

            offset := sprite.GetPosition()
            q.shader.SetUniformAttr(GuiShaderPosition, offset)
            q.shader.SetUniformAttr(GuiShaderTintColor, sprite.tintColor)

            scale := sprite.Scale()
            q.shader.SetUniformAttr(GuiShaderScale, scale)

            sprite.Draw()
            spritesDrawn++
            drawCalls++
        }

        currentAtlas.End()
    }
    q.shader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    q.shader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})

    q.quadsDrawn = spritesDrawn
    q.drawCalls = drawCalls
}

func (q *GuiDrawer) DrawnLastFrame() (int, int) {
    return q.quadsDrawn * 2, q.drawCalls
}

func (q *GuiDrawer) Add(texture *glhf.Texture, textureIndex uint16, x, y int) *GuiQuad {
    tileW, tileH := texture.GetAtlasItemSize()
    sprite := NewGUIQuadVertsFromSubImage(q.shader, texture, textureIndex, int(x), int(y), int(tileW), int(tileH))
    if _, ok := q.spritesPerTexture[texture]; !ok {
        q.spritesPerTexture[texture] = make(map[*GuiQuad]bool)
    }
    quad := &GuiQuad{Verts: sprite, textureIndex: textureIndex, currentAnimation: -1, tintColor: mgl32.Vec4{1, 1, 1, 1}, bakedSize: mgl32.Vec2{float32(tileW), float32(tileH)}, targetSize: mgl32.Vec2{float32(tileW), float32(tileH)}}
    q.spritesPerTexture[texture][quad] = true
    return quad
}

func (q *GuiDrawer) Remove(sprite *GuiQuad, texture *glhf.Texture) {
    delete(q.spritesPerTexture[texture], sprite)
}

func (q *GuiDrawer) TextureIndex(sprite *GuiQuad, elapsed float64) uint16 {
    currentAnimation := sprite.CurrentAnimation()
    if currentAnimation < 0 || currentAnimation >= len(q.animations) {
        return sprite.TextureIndex(elapsed)
    }
    var frames []uint16

    showTimes := q.animations[currentAnimation].ShowTimeInSeconds
    frames = q.animations[currentAnimation].Frames[0]
    loop := q.animations[currentAnimation].Loop
    return sprite.TextureIndexAnimated(frames, showTimes, elapsed, loop)
}

func NewGUIQuadVertsFromSubImage(shader *glhf.Shader, atlas *glhf.Texture, textureIndex uint16, x, y, width, height int) *glhf.VertexSlice[glhf.GlFloat] {
    rawVerts := AppendGUIQuadVertices(make([]glhf.GlFloat, 0), atlas, textureIndex, mgl32.Vec2{float32(x), float32(y)}, glhf.GlFloat(width), glhf.GlFloat(height))
    vertices := glhf.MakeVertexSlice(shader, len(rawVerts)/4, len(rawVerts)/4)
    vertices.Begin()
    vertices.SetVertexData(rawVerts)
    vertices.End()
    return vertices
}

type TexturedQuad struct {
    Verts   *glhf.VertexSlice[glhf.GlFloat]
    Texture *glhf.Texture
}

func (t *TexturedQuad) Draw() {
    t.Texture.Begin()
    t.Verts.Begin()
    t.Verts.Draw()
    t.Verts.End()
    t.Texture.End()
}

func NewSelectionQuad(shader *glhf.Shader, width, height int) TexturedQuad {
    tex := glhf.NewRectOutlineTexture(width, height, 1)
    verts := NewGUIQuadVerts(shader, 0, 0, float32(width), float32(height))
    return TexturedQuad{Verts: verts, Texture: tex}
}

func NewGUIQuadVerts(shader *glhf.Shader, x, y, width, height float32) *glhf.VertexSlice[glhf.GlFloat] {
    widthInPx := glhf.GlFloat(width)
    heightInPx := glhf.GlFloat(height)
    leftU, topV, rightU, bottomV := glhf.GlFloat(0), glhf.GlFloat(0), glhf.GlFloat(1), glhf.GlFloat(1)
    rawVerts := []glhf.GlFloat{
        // first triangle
        // Top-left
        glhf.GlFloat(x), glhf.GlFloat(y), leftU, topV,
        // Bottom-left
        glhf.GlFloat(x), glhf.GlFloat(y) + heightInPx, leftU, bottomV,
        // Bottom-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y) + heightInPx, rightU, bottomV,

        // second triangle
        // Top-left
        glhf.GlFloat(x), glhf.GlFloat(y), leftU, topV,
        // Bottom-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y) + heightInPx, rightU, bottomV,
        // Top-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y), rightU, topV,
    }
    vertices := glhf.MakeVertexSlice(shader, len(rawVerts)/4, len(rawVerts)/4)
    vertices.Begin()
    vertices.SetVertexData(rawVerts)
    vertices.End()
    return vertices
}

func AppendGUIQuadVertices(rawVertices []glhf.GlFloat, t *glhf.Texture, textureIndex uint16, drawPos mgl32.Vec2, widthInPx glhf.GlFloat, heightInPx glhf.GlFloat) []glhf.GlFloat {
    var leftU, topV, rightU, bottomV glhf.GlFloat
    if t != nil {
        leftU, topV, rightU, bottomV = t.GetUV(textureIndex)
    }
    rawVertices = append(rawVertices, []glhf.GlFloat{
        // first triangle
        // Top-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()), leftU, topV,
        // Bottom-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()) + heightInPx, leftU, bottomV,
        // Bottom-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()) + heightInPx, rightU, bottomV,

        // second triangle
        // Top-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()), leftU, topV,
        // Bottom-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()) + heightInPx, rightU, bottomV,
        // Top-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()), rightU, topV,
    }...)
    return rawVertices
}
