package quads

import (
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/geometry"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/tpack"
    "path/filepath"
    "strconv"
    "strings"
)

type GuiAtlasPicker struct {
    atlas                 glhf.TextureAtlas
    border                *GuiBorder
    fontHeight            int
    textColor             mgl32.Vec4
    textSelectColor       mgl32.Vec4
    screenSize            geometry.Point
    shouldClose           bool
    onClose               func()
    onIndexClicked        func(selection int)
    selectionTexture      *glhf.Texture
    currentSelectionIndex int
    atlasTopLeft          geometry.Point
    scaledTileSize        geometry.PointF
    atlasSize             geometry.Point
    screenScale           mgl32.Vec2

    verticalScrollOffset float32
    indexLabel           *BitmapFontMesh
    isDragging           bool
    tileScaleFactor      float32
    uiContext            UIContext
}

func NewGuiAtlasPicker(uiContext UIContext, atlasForSelection glhf.TextureAtlas) *GuiAtlasPicker {
    _, h := uiContext.Font.Texture.GetAtlasItemSize()
    indexLabel := NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch)
    indexLabel.SetTintColor(mgl32.Vec4{0.2, 1, 0.2, 1})
    return &GuiAtlasPicker{
        atlas:           atlasForSelection,
        uiContext:       uiContext,
        fontHeight:      h,
        textColor:       mgl32.Vec4{0.3, 1, 0.3, 1},
        textSelectColor: mgl32.Vec4{0.0, 0.2, 0, 1},
        indexLabel:      indexLabel,
    }
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

func (a *GuiAtlasPicker) selectionRect() mgl32.Vec4 {
    selectionIndex := a.currentSelectionIndex
    tl := a.atlasTopLeft.ToVec2()
    return a.atlas.GetSourceRectScaled(selectionIndex, a.tileScaleFactor).Add(mgl32.Vec4{tl.X(), tl.Y(), tl.X(), tl.Y()}).Sub(mgl32.Vec4{0, a.verticalScrollOffset * a.tileScaleFactor, 0, a.verticalScrollOffset * a.tileScaleFactor})
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
    relativeY := int(ypos) - a.atlasTopLeft.Y + int(a.verticalScrollOffset*a.tileScaleFactor)

    return a.atlas.GetIndexFromPxPosScaled(relativeX, relativeY, a.tileScaleFactor)
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
    _, atlasHeight := a.atlas.Width(), a.atlas.Height()

    scrollingSpace := int(float32(a.atlasSize.Y) / a.tileScaleFactor)

    a.currentSelectionIndex = -1

    a.verticalScrollOffset += float32(yoff * 0.5 * float64(a.scaledTileSize.Y))
    if a.verticalScrollOffset < 0 {
        a.verticalScrollOffset = 0
    }

    if a.verticalScrollOffset > float32(atlasHeight-scrollingSpace) {
        a.verticalScrollOffset = float32(atlasHeight - scrollingSpace)
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
    if index < 0 || index+len(names)-1 > a.atlas.LastValidIndex() {
        return false
    }
    //tileWidth, tileHeight := a.atlas.GetAtlasItemSize()
    var droppedImages = make([]tpack.NamedImage, 0, len(names))
    for _, name := range names {
        ext := filepath.Ext(name)
        if strings.ToLower(ext) == ".png" {
            loadedImage := tpack.NamedImage{
                Image: Disk.Image(name),
                Name:  realBaseName(name),
            }
            // REMINDER: This won't allow arbitrary sizes

            if !a.atlas.CanAppend(loadedImage) {
                continue
            }
            //pixels := pixelsFromImage(loadedImage)
            droppedImages = append(droppedImages, loadedImage)
        }
    }
    if len(droppedImages) == 0 {
        return false
    }

    a.appendImages(index, droppedImages)
    return true
}

func realBaseName(name string) string {
    return strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
}

func (a *GuiAtlasPicker) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
    a.screenSize = geometry.Point{X: screenWidth, Y: screenHeight}
    a.screenScale = screenCoordScale
    borderSize := a.screenSize.Scaled(0.9, 0.4)
    borderDrawPos := a.screenSize.CenterInBottomHalf(borderSize)

    a.border = NewGuiBorderWithBounds(a.uiContext, borderDrawPos.ToVec2(), borderSize.ToVec2())
    a.border.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.8})

    a.layoutAtlas(borderDrawPos, borderSize)
}

func (a *GuiAtlasPicker) layoutAtlas(borderDrawPos geometry.Point, borderSize geometry.Point) {
    atlasWidth, _ := a.atlas.Width(), a.atlas.Height()

    a.atlasTopLeft, a.atlasSize = borderDrawPos.CenterInParent(borderSize, geometry.PointF{X: 0.9, Y: 0.9})

    minU, minV, maxU, maxV := a.atlas.GetUVInPx(0)
    itemSizeX, itemSizeY := maxU-minU, maxV-minV
    a.tileScaleFactor = float32(a.atlasSize.X) / float32(atlasWidth)
    a.scaledTileSize = geometry.PointF{X: float64(itemSizeX) * float64(a.tileScaleFactor), Y: float64(itemSizeY) * float64(a.tileScaleFactor)}

    a.selectionTexture = glhf.NewRectOutlineTexture(int(a.scaledTileSize.X), int(a.scaledTileSize.Y), 1)
}

func (a *GuiAtlasPicker) UpdateAtlas() {
    pos := geometry.Point{X: int(a.border.position.X()), Y: int(a.border.position.Y())}
    size := geometry.Point{X: int(a.border.size.X()), Y: int(a.border.size.Y())}
    a.layoutAtlas(pos, size)
}
func (a *GuiAtlasPicker) atlasCutoff() glhf.IntVec2 {
    return glhf.IntVec2{int32(float32(a.atlasTopLeft.Y) * a.screenScale.X()), int32(float32(a.atlasTopLeft.Y+a.atlasSize.Y) * a.screenScale.Y())}
}

func (a *GuiAtlasPicker) Draw() {
    a.border.Draw()

    batch := a.atlas.GetSpriteBatch()
    batch.DrawTextured(a.atlasSrcRect(), a.atlasDstRect(), mgl32.Vec4{1, 1, 1, 1})

    if a.isCurrentSelectionValid() {
        selectionPosition := a.selectionRect()
        a.uiContext.ColorBatch.DrawColoredOutline(selectionPosition, 1, mgl32.Vec4{0.6, 1, 0.6, 1})
        if !a.isDragging {
            a.indexLabel.SetPosition(selectionPosition.Vec2())
            a.indexLabel.Draw()
        }
    }

    if a.isDragging {
        a.indexLabel.Draw()
    }
}

func (a *GuiAtlasPicker) atlasDstRect() mgl32.Vec4 {
    topLeft := a.atlasTopLeft.ToVec2()
    atlasSize := a.atlasSize.ToVec2()
    bottomRight := topLeft.Add(atlasSize)
    return mgl32.Vec4{topLeft.X(), topLeft.Y(), bottomRight.X(), bottomRight.Y()}
}

func (a *GuiAtlasPicker) atlasSrcRect() mgl32.Vec4 {
    width := float32(a.atlas.Width())
    displayAspectRatio := float64(a.atlasSize.X) / float64(a.atlasSize.Y)
    height := float32(a.atlas.Width()) / float32(displayAspectRatio)
    xStart := float32(0)
    yStart := a.verticalScrollOffset
    rect := mgl32.Vec4{xStart, yStart, width, yStart + height}
    return rect
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

func (a *GuiAtlasPicker) appendImages(index int, files []tpack.NamedImage) {
    a.atlas.AppendImages(index, files)
}

func (a *GuiAtlasPicker) isCurrentSelectionValid() bool {
    if a.currentSelectionIndex < 0 {
        return false
    }
    return a.atlas.HasIndex(a.currentSelectionIndex)
}
