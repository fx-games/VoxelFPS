package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads/glhf"
)

type AnimatedSprite struct {
    currentAnimation     int
    textureIndex         uint16
    currentFrameTime     float32
    currentFrame         int
    changedFrameThisTick bool
    isHidden             bool
    tintColor            mgl32.Vec4
    eventOnFrame         map[int]events.EventID
    dstRect              mgl32.Vec4
}

func (q *AnimatedSprite) CurrentAnimation() int {
    return q.currentAnimation
}

func (q *AnimatedSprite) TextureIndex(elapsed float64) uint16 {
    return q.textureIndex
}
func (q *AnimatedSprite) TextureIndexAnimated(frames []uint16, showTimes []float32, elapsed float64, loop bool) uint16 {
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

func (q *AnimatedSprite) PlayAnimation(anim int) {
    q.currentAnimation = anim
    q.currentFrame = 0
    q.currentFrameTime = 0
}
func (q *AnimatedSprite) ClearEvents() {
    clear(q.eventOnFrame)
}
func (q *AnimatedSprite) PublishEventOnFrame(frame int, event events.EventID) {
    if q.eventOnFrame == nil {
        q.eventOnFrame = make(map[int]events.EventID)
    }
    q.eventOnFrame[frame] = event
}

func (q *AnimatedSprite) Hide() {
    q.isHidden = true
}

func (q *AnimatedSprite) Show() {
    q.isHidden = false
}

func (q *AnimatedSprite) SetTintColor(color mgl32.Vec4) {
    q.tintColor = color
}

func (q *AnimatedSprite) DestinationRect() mgl32.Vec4 {
    return mgl32.Vec4{
        q.dstRect.X(),
        q.dstRect.Y(),
        q.dstRect.Z(),
        q.dstRect.W(),
    }
}

func (q *AnimatedSprite) SetDestRect(rect mgl32.Vec4) {
    q.dstRect = rect
}

type GuiDrawer struct {
    shader     *glhf.Shader
    quadsDrawn int
    drawCalls  int

    animations        []SpriteAnimation
    spritesPerTexture map[glhf.TextureAtlas]map[*AnimatedSprite]bool
}

func NewGuiDrawer(shader *glhf.Shader) *GuiDrawer {
    return &GuiDrawer{
        shader:            shader,
        spritesPerTexture: make(map[glhf.TextureAtlas]map[*AnimatedSprite]bool),
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
        curBatch := currentAtlas.GetSpriteBatch()
        for sprite := range spriteIndices {
            if sprite.isHidden {
                continue
            }
            texIndex := q.TextureIndex(sprite, elapsed)

            curBatch.DrawTextured(currentAtlas.GetSourceRect(texIndex), sprite.DestinationRect(), sprite.tintColor)
            spritesDrawn++
        }
    }
    q.quadsDrawn = spritesDrawn
    q.drawCalls = drawCalls
}

func (q *GuiDrawer) DrawnLastFrame() (int, int) {
    return q.quadsDrawn * 2, q.drawCalls
}

func (q *GuiDrawer) Add(texture *glhf.TextureAtlasFixed, textureIndex uint16, x, y int) *AnimatedSprite {
    if _, ok := q.spritesPerTexture[texture]; !ok {
        q.spritesPerTexture[texture] = make(map[*AnimatedSprite]bool)
    }
    quad := &AnimatedSprite{
        textureIndex:     textureIndex,
        currentAnimation: -1,
        tintColor:        mgl32.Vec4{1, 1, 1, 1},
        dstRect:          texture.GetDestinationRect(mgl32.Vec2{float32(x), float32(y)}, mgl32.Vec2{1, 1}),
    }
    q.spritesPerTexture[texture][quad] = true
    return quad
}

func (q *GuiDrawer) Remove(sprite *AnimatedSprite, texture *glhf.TextureAtlasFixed) {
    delete(q.spritesPerTexture[texture], sprite)
}

func (q *GuiDrawer) TextureIndex(sprite *AnimatedSprite, elapsed float64) int {
    currentAnimation := sprite.CurrentAnimation()
    if currentAnimation < 0 || currentAnimation >= len(q.animations) {
        return int(sprite.TextureIndex(elapsed))
    }
    var frames []uint16

    showTimes := q.animations[currentAnimation].ShowTimeInSeconds
    frames = q.animations[currentAnimation].Frames[0]
    loop := q.animations[currentAnimation].Loop
    return int(sprite.TextureIndexAnimated(frames, showTimes, elapsed, loop))
}

type TexturedQuad struct {
    Verts   *glhf.VertexArray[glhf.GlFloat]
    Texture *glhf.Texture
}

func (t *TexturedQuad) Draw() {
    t.Texture.Begin()
    t.Verts.Begin()
    t.Verts.Draw()
    t.Verts.End()
    t.Texture.End()
}

func NewGUIQuadVerts(shader *glhf.Shader, layer float32, x, y, width, height, scale float32) *glhf.VertexArray[glhf.GlFloat] {
    widthInPx := glhf.GlFloat(width * scale)
    heightInPx := glhf.GlFloat(height * scale)
    leftU, topV, rightU, bottomV := glhf.GlFloat(0), glhf.GlFloat(0), glhf.GlFloat(width-1), glhf.GlFloat(height-1)
    rawVerts := []glhf.GlFloat{
        // first triangle
        // Top-left
        glhf.GlFloat(x), glhf.GlFloat(y), glhf.GlFloat(layer), leftU, topV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),
        // Bottom-left
        glhf.GlFloat(x), glhf.GlFloat(y) + heightInPx, glhf.GlFloat(layer), leftU, bottomV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),
        // Bottom-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y) + heightInPx, glhf.GlFloat(layer), rightU, bottomV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),

        // second triangle
        // Top-left
        glhf.GlFloat(x), glhf.GlFloat(y), glhf.GlFloat(layer), leftU, topV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),
        // Bottom-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y) + heightInPx, glhf.GlFloat(layer), rightU, bottomV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),
        // Top-right
        glhf.GlFloat(x) + widthInPx, glhf.GlFloat(y), glhf.GlFloat(layer), rightU, topV, glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1), glhf.GlFloat(1),
    }
    vertices := glhf.NewVertexArray[glhf.GlFloat](shader, false)
    vertices.Begin()
    vertices.SetVertexData(rawVerts)
    vertices.End()
    return vertices
}

func AppendGUIQuadVertices(rawVertices []glhf.GlFloat, t glhf.TextureAtlas, textureIndex int, layer float32, drawPos mgl32.Vec2, widthInPx glhf.GlFloat, heightInPx glhf.GlFloat, color mgl32.Vec4) []glhf.GlFloat {
    var leftU, topV, rightU, bottomV glhf.GlFloat
    if t != nil {
        minU, minV, maxU, maxV := t.GetUVInPx(textureIndex)
        leftU, topV, rightU, bottomV = glhf.GlFloat(minU), glhf.GlFloat(minV), glhf.GlFloat(maxU), glhf.GlFloat(maxV)
    }
    r, g, b, a := color.X(), color.Y(), color.Z(), color.W()
    rawVertices = append(rawVertices, []glhf.GlFloat{
        // first triangle
        // Top-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()), glhf.GlFloat(layer), leftU, topV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),
        // Bottom-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()) + heightInPx, glhf.GlFloat(layer), leftU, bottomV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),
        // Bottom-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()) + heightInPx, glhf.GlFloat(layer), rightU, bottomV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),

        // second triangle
        // Top-left
        glhf.GlFloat(drawPos.X()), glhf.GlFloat(drawPos.Y()), glhf.GlFloat(layer), leftU, topV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),
        // Bottom-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()) + heightInPx, glhf.GlFloat(layer), rightU, bottomV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),
        // Top-right
        glhf.GlFloat(drawPos.X()) + widthInPx, glhf.GlFloat(drawPos.Y()), glhf.GlFloat(layer), rightU, topV, glhf.GlFloat(r), glhf.GlFloat(g), glhf.GlFloat(b), glhf.GlFloat(a),
    }...)
    return rawVertices
}
func NewGUIQuadVertsFromSize(shader *glhf.Shader, widthInPx, heightInPx int) *glhf.VertexArray[glhf.GlFloat] {
    var rawVertices []glhf.GlFloat
    var leftU, topV, rightU, bottomV glhf.GlFloat
    rightU = 1
    bottomV = 1
    quadWidth := glhf.GlFloat(widthInPx)
    quadHeight := glhf.GlFloat(heightInPx)
    rawVertices = append(rawVertices, []glhf.GlFloat{
        // first triangle
        // Top-left
        glhf.GlFloat(0), glhf.GlFloat(0), leftU, topV,
        // Bottom-left
        glhf.GlFloat(0), glhf.GlFloat(0) + quadHeight, leftU, bottomV,
        // Bottom-right
        glhf.GlFloat(0) + quadWidth, glhf.GlFloat(0) + quadHeight, rightU, bottomV,

        // second triangle
        // Top-left
        glhf.GlFloat(0), glhf.GlFloat(0), leftU, topV,
        // Bottom-right
        glhf.GlFloat(0) + quadWidth, glhf.GlFloat(0) + quadHeight, rightU, bottomV,
        // Top-right
        glhf.GlFloat(0) + quadWidth, glhf.GlFloat(0), rightU, topV,
    }...)

    vertices := glhf.NewVertexArray[glhf.GlFloat](shader, false)
    vertices.Begin()
    vertices.SetVertexData(rawVertices)
    vertices.End()
    return vertices
}
