package quads

import (
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/geometry"
    "github.com/memmaker/goray/quads/glhf"
    "path/filepath"
    "strconv"
    "strings"
)

type GuiAtlasPicker struct {
    atlas                 *glhf.Texture
    border                *GuiBorder
    uiTexture             *glhf.Texture
    uiShader              *glhf.Shader
    font                  BitmapFont
    fontHeight            int
    textColor             mgl32.Vec4
    textSelectColor       mgl32.Vec4
    scale                 mgl32.Vec2
    screenSize            geometry.Point
    atlasVerts            *glhf.VertexSlice[glhf.GlFloat]
    shouldClose           bool
    onClose               func()
    onIndexClicked        func(selection int)
    selectionTexture      *glhf.Texture
    selectionVerts        *glhf.VertexSlice[glhf.GlFloat]
    currentSelectionIndex int
    atlasTopLeft          geometry.Point
    tileSize              geometry.PointF
    atlasSize             geometry.Point
    screenScale           mgl32.Vec2

    verticalScrollOffset float32
    indexLabel           *BitmapFontMesh
    isDragging           bool
}

func (a *GuiAtlasPicker) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
    if action != glfw.Press {
        return false
    }

    switch key {
    case glfw.KeyEscape:
        a.Close()
        return true
    }
    return false
}

func (a *GuiAtlasPicker) selectionPosition() mgl32.Vec2 {
    selectionIndex := a.currentSelectionIndex
    tileSizeX, _ := a.atlas.GetAtlasItemSize()
    itemsPerRow := a.atlas.Width() / tileSizeX
    row := float32(selectionIndex / itemsPerRow)
    col := float32(selectionIndex % itemsPerRow)
    drawPosX := col * float32(a.tileSize.X)
    drawPosY := row * float32(a.tileSize.Y)
    return mgl32.Vec2{drawPosX, drawPosY + a.verticalScrollOffset}
}

func (a *GuiAtlasPicker) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {

    if a.isDragging {
        labelSize := a.indexLabel.SizeAsVec2()
        a.indexLabel.SetPosition(mgl32.Vec2{float32(xpos) - labelSize.X(), float32(ypos) - labelSize.Y()})
        return false
    }

    if xpos < float64(a.atlasTopLeft.X) || xpos > float64(a.atlasTopLeft.X+a.atlasSize.X) {
        return false
    }
    if ypos < float64(a.atlasTopLeft.Y) || ypos > float64(a.atlasTopLeft.Y+a.atlasSize.Y) {
        return false
    }

    index := a.indexFromScreenPos(xpos, ypos)

    if index != a.currentSelectionIndex {
        a.currentSelectionIndex = index
        a.indexLabel.SetText(strconv.Itoa(a.currentSelectionIndex))
    }

    return true
}

func (a *GuiAtlasPicker) indexFromScreenPos(xpos float64, ypos float64) int {
    relativeX := int(xpos) - a.atlasTopLeft.X
    relativeY := int(ypos) - a.atlasTopLeft.Y - int(a.verticalScrollOffset)

    tileSizeX, _ := a.atlas.GetAtlasItemSize()
    itemsPerRow := a.atlas.Width() / tileSizeX

    col := int(float64(relativeX) / a.tileSize.X)
    row := int(float64(relativeY) / a.tileSize.Y)

    index := row*itemsPerRow + col
    return index
}

func (a *GuiAtlasPicker) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
    if action != glfw.Press || button != glfw.MouseButtonLeft {
        return false
    }
    isValidSelection := a.HandleMousePosEvents(xpos, ypos)
    if !isValidSelection {
        return false
    }
    a.currentSelectionIndex = a.indexFromScreenPos(xpos, ypos)
    if a.onIndexClicked != nil {
        a.onIndexClicked(a.currentSelectionIndex)
    }

    return true
}

func (a *GuiAtlasPicker) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
    atlasWidth, atlasHeight := a.atlas.Width(), a.atlas.Height()
    aspectRatio := float64(atlasWidth) / float64(atlasHeight)

    scrollingContentHeight := int(float64(a.atlasSize.X) / aspectRatio)

    scrollingSpace := int(a.atlasSize.Y)

    if scrollingContentHeight <= scrollingSpace {
        a.verticalScrollOffset = 0
        return true
    }
    a.verticalScrollOffset += float32(yoff * 0.5 * float64(a.tileSize.Y))
    if a.verticalScrollOffset > 0 {
        a.verticalScrollOffset = 0
    }

    if a.verticalScrollOffset < float32(-scrollingContentHeight+scrollingSpace) {
        a.verticalScrollOffset = float32(-scrollingContentHeight + scrollingSpace)
    }
    return true
}

func (a *GuiAtlasPicker) ShouldClose() bool {
    return a.shouldClose
}

func (a *GuiAtlasPicker) HandleDrop(names []string, xpos float64, ypos float64) bool {
    if xpos < float64(a.atlasTopLeft.X) || xpos > float64(a.atlasTopLeft.X+a.atlasSize.X) {
        return false
    }
    if ypos < float64(a.atlasTopLeft.Y) || ypos > float64(a.atlasTopLeft.Y+a.atlasSize.Y) {
        return false
    }

    index := a.indexFromScreenPos(xpos, ypos)
    if index < 0 || index+len(names) > a.atlas.LastValidIndex() {
        return false
    }
    tileWidth, tileHeight := a.atlas.GetAtlasItemSize()
    var pixelsFromFiles = make([][]uint8, 0, len(names))
    for _, name := range names {
        ext := filepath.Ext(name)
        if strings.ToLower(ext) == ".png" {
            loadedImage := LoadImage(name)
            if loadedImage.Bounds().Dx() != int(tileWidth) || loadedImage.Bounds().Dy() != int(tileHeight) {
                continue
            }
            pixels := pixelsFromImage(loadedImage)
            pixelsFromFiles = append(pixelsFromFiles, pixels)
        }
    }
    if len(pixelsFromFiles) == 0 {
        return false
    }

    a.appendImages(index, pixelsFromFiles)
    return true
}

func NewGuiAtlasPicker(shader *glhf.Shader, uiTexture *glhf.Texture, font BitmapFont, atlasForSelection *glhf.Texture) *GuiAtlasPicker {
    _, h := font.Texture.GetAtlasItemSize()
    return &GuiAtlasPicker{
        atlas:           atlasForSelection,
        uiShader:        shader,
        font:            font,
        uiTexture:       uiTexture,
        fontHeight:      h,
        textColor:       mgl32.Vec4{0.3, 1, 0.3, 1},
        textSelectColor: mgl32.Vec4{0.0, 0.2, 0, 1},
        scale:           mgl32.Vec2{1, 1},
        indexLabel:      NewBitmapFontMesh(shader, font),
    }
}

func (a *GuiAtlasPicker) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
    a.screenSize = geometry.Point{X: screenWidth, Y: screenHeight}
    a.screenScale = screenCoordScale
    borderSize := a.screenSize.Scaled(0.9, 0.4)
    borderDrawPos := a.screenSize.CenterInBottomHalf(borderSize)

    a.border = NewGuiBorderWithBounds(a.uiShader, a.uiTexture, borderDrawPos.ToVec2(), borderSize.ToVec2())
    a.border.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.8})

    a.layoutAtlas(borderDrawPos, borderSize)
}

func (a *GuiAtlasPicker) layoutAtlas(borderDrawPos geometry.Point, borderSize geometry.Point) {
    atlasWidth, atlasHeight := a.atlas.Width(), a.atlas.Height()
    aspectRatio := float64(atlasWidth) / float64(atlasHeight)

    a.atlasTopLeft, a.atlasSize = borderDrawPos.CenterInParent(borderSize, geometry.PointF{X: 0.9, Y: 0.9})
    atlasFullHeight := int(float64(a.atlasSize.X) / aspectRatio)

    itemSizeX, itemSizeY := a.atlas.GetAtlasItemSize()
    itemsPerRow := atlasWidth / itemSizeX
    itemsPerCol := atlasHeight / itemSizeY
    a.tileSize = geometry.PointF{X: float64(a.atlasSize.X) / float64(itemsPerRow), Y: float64(atlasFullHeight) / float64(itemsPerCol)}

    a.atlasVerts = NewGUIQuadVerts(a.uiShader, float32(a.atlasTopLeft.X), float32(a.atlasTopLeft.Y), float32(a.atlasSize.X), float32(atlasFullHeight))
    a.selectionVerts = NewGUIQuadVerts(a.uiShader, float32(a.atlasTopLeft.X), float32(a.atlasTopLeft.Y), float32(a.tileSize.X), float32(a.tileSize.Y))

    a.selectionTexture = glhf.NewRectOutlineTexture(int(a.tileSize.X), int(a.tileSize.Y), 1)
}

func (a *GuiAtlasPicker) UpdateAtlas(newAtlas *glhf.Texture) {
    a.atlas = newAtlas
    pos := geometry.Point{X: int(a.border.position.X()), Y: int(a.border.position.Y())}
    size := geometry.Point{X: int(a.border.size.X()), Y: int(a.border.size.Y())}
    a.layoutAtlas(pos, size)
}
func (a *GuiAtlasPicker) atlasCutoff() glhf.IntVec2 {
    return glhf.IntVec2{int32(float32(a.atlasTopLeft.Y) * a.screenScale.X()), int32(float32(a.atlasTopLeft.Y+a.atlasSize.Y) * a.screenScale.Y())}
}

func (a *GuiAtlasPicker) Draw() {
    a.uiShader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    a.uiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    a.uiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    a.uiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})

    a.border.Draw()

    a.uiShader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, a.verticalScrollOffset})
    a.uiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    a.uiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    a.uiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    a.uiShader.SetUniformAttr(GuiShaderVerticalCutoff, a.atlasCutoff())

    a.atlas.Begin()
    a.atlasVerts.Begin()
    a.atlasVerts.Draw()
    a.atlasVerts.End()
    a.atlas.End()

    selectionPosition := a.selectionPosition()
    a.uiShader.SetUniformAttr(GuiShaderPosition, selectionPosition)
    a.uiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    a.uiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{0.2, 1, 0.2, 1})
    a.uiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})

    a.selectionTexture.Begin()
    a.selectionVerts.Begin()
    a.selectionVerts.Draw()
    a.selectionVerts.End()
    a.selectionTexture.End()

    if a.isDragging {
        a.uiShader.SetUniformAttr(GuiShaderVerticalCutoff, glhf.IntVec2{0, 8192})
    } else {
        a.indexLabel.SetPosition(selectionPosition.Add(a.atlasTopLeft.ToVec2()))
    }

    a.indexLabel.Draw()

    a.uiShader.SetUniformAttr(GuiShaderVerticalCutoff, glhf.IntVec2{0, 8192})
}

func (a *GuiAtlasPicker) Close() {
    a.shouldClose = true
    if a.onClose != nil {
        a.onClose()
    }
}

func (a *GuiAtlasPicker) SetOnClose(closeCallback func()) {
    a.onClose = closeCallback
}

func (a *GuiAtlasPicker) SetOnIndexClicked(callback func(selection int)) {
    a.onIndexClicked = callback
}

func (a *GuiAtlasPicker) StartDrag(xpos float64, ypos float64) int {
    isValid := a.HandleMousePosEvents(xpos, ypos)
    if !isValid {
        return -1
    }
    a.isDragging = true
    return a.currentSelectionIndex
}

func (a *GuiAtlasPicker) EndDrag() {
    a.indexLabel.Hide()
    a.isDragging = false
}

func (a *GuiAtlasPicker) appendImages(index int, files [][]uint8) {
    a.atlas.SetPixelsAtIndexMulti(index, files)
}
