package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "image"
)

/*
Start by dropping the frontal pic.
Choose if you want directions? -> Add directions
Choose if you want to add animation frames? -> Add animation frames
Choose if you want to add additional animations? -> Add additional animations

Buttons:
 - Done
 - Add Directions
 - Rotate ViewDirection: On/Off
 - Play Animation: On/Off

*/

type GuiSpriteImporter struct {
    background *GuiBorder
    uiShader   *glhf.Shader
    uiTexture  *glhf.Texture

    spriteDrawer *SpriteDrawer

    gameObject *GameObject

    baseWidth     int
    baseHeight    int
    texSlots      []*glhf.VertexSlice[glhf.GlFloat]
    slotSize      int
    slotPositions []mgl32.Vec2
    labels        []*BitmapFontMesh
    shouldClouse  bool
    closeCallback func()
    font          BitmapFont

    screenWidth  int
    screenHeight int
}

func (g *GuiSpriteImporter) HandleDrop(names []string, x float64, y float64) bool {
    if len(names) != 1 {
        return false
    }
    textFileName := names[0]
    droppedImage := LoadImage(textFileName)
    for slotIndex, slotPos := range g.slotPositions {
        if x >= float64(slotPos[0]) && x <= float64(slotPos[0])+float64(g.slotSize) &&
            y >= float64(slotPos[1]) && y <= float64(slotPos[1])+float64(g.slotSize) {
            g.updateSpriteAtlas(slotIndex, pixelsFromImage(droppedImage))
            return true
        }
    }

    return false
}

func pixelsFromImage(image image.Image) []uint8 {
    frameWidth := image.Bounds().Dx()
    frameHeight := image.Bounds().Dy()
    pixels := make([]uint8, frameWidth*frameHeight*4)
    for y := 0; y < frameHeight; y++ {
        for x := 0; x < frameWidth; x++ {
            r, g, b, a := image.At(x, y).RGBA()

            pixels[(y*frameWidth+x)*4] = uint8((float64(r) / 0xffff) * 0xff)
            pixels[(y*frameWidth+x)*4+1] = uint8((float64(g) / 0xffff) * 0xff)
            pixels[(y*frameWidth+x)*4+2] = uint8((float64(b) / 0xffff) * 0xff)
            pixels[(y*frameWidth+x)*4+3] = uint8((float64(a) / 0xffff) * 0xff)
        }
    }

    return pixels
}

func (g *GuiSpriteImporter) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
    if action != glfw.Press {
        return false
    }

    switch key {
    case glfw.KeyEscape:
        g.Close()
        return true
    }
    return false
}

func (g *GuiSpriteImporter) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {
    return false
}

func (g *GuiSpriteImporter) HandleMouseButtonEvents(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
    return false
}

func (g *GuiSpriteImporter) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
    return false
}

func (g *GuiSpriteImporter) ShouldClose() bool {
    return g.shouldClouse
}

func (g *GuiSpriteImporter) Close() {
    g.shouldClouse = true
    if g.closeCallback != nil {
        g.closeCallback()
    }
}

func NewGuiSpriteImporter(uiShader *glhf.Shader, uiTexture *glhf.Texture, font BitmapFont, spriteDrawer *SpriteDrawer, obj *GameObject) *GuiSpriteImporter {
    return &GuiSpriteImporter{
        uiShader:     uiShader,
        uiTexture:    uiTexture,
        spriteDrawer: spriteDrawer,
        baseWidth:    64,
        baseHeight:   64,
        shouldClouse: false,
        gameObject:   obj,
        font:         font,
    }
}

// This won't work as expected on a shared atlas.. like the terrain texture where we have
// decorations baked into the same atlas..
func (g *GuiSpriteImporter) updateSpriteAtlas(frameIndex int, withPixels []uint8) {
    spriteComponent := g.gameObject.Sprite()
    g.spriteDrawer.Remove(spriteComponent)

    if !spriteComponent.AtlasTexture.HasIndex(frameIndex) {
        spriteComponent.AtlasTexture = glhf.ExpandToIndex(spriteComponent.AtlasTexture, frameIndex)
    }

    spriteComponent.AtlasTexture.SetPixelsAtIndex(uint16(frameIndex), withPixels)
    spriteComponent.TextureIndices = g.idleFrame(frameIndex)

    g.spriteDrawer.Add(spriteComponent)
    g.SetCentered(g.screenWidth, g.screenHeight)
}

func (g *GuiSpriteImporter) idleFrame(frameIndex int) []uint16 {
    if frameIndex > 0 {
        return []uint16{0, 1, 2, 3, 4, 5, 6, 7}
    }
    return []uint16{0}
}

func (g *GuiSpriteImporter) SetCentered(screenWidth int, screenHeight int) {

    g.screenWidth = screenWidth
    g.screenHeight = screenHeight
    labelTexts := []string{"F", "FL", "L", "BL", "B", "BR", "R", "FR"}
    g.texSlots = nil
    g.slotPositions = nil
    g.labels = nil

    backgWidth := 0.9 * float32(screenWidth)
    backgHeight := 0.9 * float32(screenHeight)

    g.background = NewGuiBorder(g.uiShader, g.uiTexture, mgl32.Vec2{float32(screenWidth) * 0.05, float32(screenHeight) * 0.05}, mgl32.Vec2{backgWidth, backgHeight})
    g.background.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.8})

    topLeftX := float32(screenWidth) * 0.1
    topLeftY := float32(screenHeight) * 0.1
    if g.gameObject == nil {
        return
    }
    sprite := g.gameObject.Sprite()

    tileWidth := int(float32(screenWidth) * 0.8 * 0.125)
    tileHeight := tileWidth
    drawY := int(topLeftY)

    g.slotSize = tileWidth
    for i := 0; i < 8; i++ {
        drawX := int(topLeftX) + i*(tileWidth)
        slotPos := mgl32.Vec2{float32(drawX), float32(drawY)}
        g.slotPositions = append(g.slotPositions, slotPos)

        label := NewBitmapFontMesh(g.uiShader, g.font)
        label.SetTintColor(mgl32.Vec4{1, 1, 1, 1})
        label.SetWidth(tileWidth)
        label.SetCenterOrigin(true)
        label.SetText(labelTexts[i])
        _, labelH := label.Size()
        label.SetPosition(slotPos.Add(mgl32.Vec2{float32(tileWidth) * 0.5, float32(tileHeight) + (float32(labelH) * 0.5)}))

        g.labels = append(g.labels, label)
        if i >= len(sprite.TextureIndices) {
            continue
        }
        g.texSlots = append(g.texSlots, NewGUIQuadVerts(g.uiShader, sprite.AtlasTexture, sprite.TextureIndices[i], drawX, drawY, tileWidth, tileHeight))
    }

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
}

func (g *GuiSpriteImporter) Draw() {
    g.uiShader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    g.uiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    g.uiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    g.uiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})

    g.background.Draw()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
    g.uiShader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    g.uiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    g.uiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    g.uiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})

    texture := g.gameObject.Sprite().AtlasTexture

    texture.Begin()
    for _, texSlot := range g.texSlots {
        texSlot.Begin()
        texSlot.Draw()
        texSlot.End()
    }
    texture.End()

    for _, label := range g.labels {
        label.Draw()
    }

    glErr = gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
}

func (g *GuiSpriteImporter) isDirectional() bool {
    if g.gameObject == nil {
        return false
    }

    return g.gameObject.Sprite().IsDirectional()
}

func (g *GuiSpriteImporter) SetOnClose(closeCallback func()) {
    g.closeCallback = closeCallback
}
