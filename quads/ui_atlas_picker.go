package quads

import (
	"fmt"

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
	onIndexRightClicked   func(selection int)
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

	// Set together by SetBrowser, and nil for a picker that is showing a finished
	// atlas rather than the texture library.
	browser  *TexBrowser
	dest     *AtlasIndex
	toast    func(string)
	dirLabel *BitmapFontMesh
}

// SetBrowser turns the picker into a file browser over a folder of assets.
// Cells are then directories and the things in them rather than atlas slots, and an
// index handed to a caller is the slot the picked texture was copied into - dest
// is what a texture is chosen *for*, the library is only where it is chosen from.
//
// dest is nil for a browser over something that is not a texture at all - the
// mesh browser, where what is chosen is the folder itself. The index then means
// the cell, and Browser().Entry says which folder that was.
func (a *GuiAtlasPicker) SetBrowser(browser *TexBrowser, dest *AtlasIndex, toast func(string)) {
	a.browser = browser
	a.dest = dest
	a.toast = toast
	a.dirLabel = NewBitmapFontMesh(a.uiContext.Shader, a.uiContext.Font, a.uiContext.ColorBatch)
	a.dirLabel.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	a.showDir()
}

// showDir adopts whatever directory the browser is on now.
func (a *GuiAtlasPicker) showDir() {
	a.atlas = a.browser.Atlas()
	a.verticalScrollOffset = 0
	a.currentSelectionIndex = -1
	a.dirLabel.SetText(a.browser.Dir())
	if a.border != nil {
		a.UpdateAtlas()
	}
}

// Resolve turns a cell the user acted on into a texture slot in the destination
// atlas, copying the library texture in if this is the first time it was picked.
// A directory cell is navigation, not a choice, so it resolves to nothing.
func (a *GuiAtlasPicker) Resolve(index int) (int, bool) {
	if a.browser == nil {
		return index, index >= 0
	}
	entry, isCell := a.browser.Entry(index)
	if !isCell {
		return -1, false
	}
	if entry.IsDir {
		a.browser.Open(entry.Path)
		a.showDir()
		return -1, false
	}
	if a.dest == nil {
		// Nothing to import into: the cell itself is the answer. See SetBrowser.
		return index, true
	}
	slot, err := a.dest.Import(entry.Path)
	if err != nil {
		if a.toast != nil {
			a.toast(err.Error())
		}
		return -1, false
	}
	return slot, true
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
	case glfw.KeyBackspace:
		if a.browser != nil {
			a.browser.Up()
			a.showDir()
			return true
		}
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
		a.indexLabel.SetText(a.labelFor(index))
	}

	return true
}

func (a *GuiAtlasPicker) indexFromScreenPos(xpos float64, ypos float64) int {
	relativeX := int(xpos) - a.atlasTopLeft.X
	relativeY := int(ypos) - a.atlasTopLeft.Y + int(a.verticalScrollOffset*a.tileScaleFactor)

	return a.atlas.GetIndexFromPxPosScaled(relativeX, relativeY, a.tileScaleFactor)
}

func (a *GuiAtlasPicker) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
	if action != glfw.Press || (button != glfw.MouseButtonLeft && button != glfw.MouseButtonRight) {
		return false
	}
	isValidSelection := a.HandleMousePosEvents(xpos, ypos)
	if !isValidSelection {
		return false
	}
	a.currentSelectionIndex = a.indexFromScreenPos(xpos, ypos)
	if button == glfw.MouseButtonRight {
		// The raw cell, not Resolve's answer: a right click must neither walk
		// into a folder nor cost the atlas a slot.
		if a.onIndexRightClicked != nil {
			a.onIndexRightClicked(a.currentSelectionIndex)
		}
		return true
	}
	selected, isChoice := a.Resolve(a.currentSelectionIndex)
	if isChoice && a.onIndexClicked != nil {
		a.onIndexClicked(selected)
	}

	return true
}

// labelFor names the cell under the cursor: its file name while browsing the
// library, its slot number while showing a finished atlas.
func (a *GuiAtlasPicker) labelFor(index int) string {
	if a.browser != nil {
		return a.browser.Label(index)
	}
	return strconv.Itoa(index)
}

func (a *GuiAtlasPicker) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
	_, atlasHeight := a.atlas.Width(), a.atlas.Height()

	scrollingSpace := int(float32(a.atlasSize.Y) / a.tileScaleFactor)

	a.currentSelectionIndex = -1

	a.verticalScrollOffset += float32(yoff * 0.5 * float64(a.scaledTileSize.Y))
	if a.verticalScrollOffset < 0 {
		a.verticalScrollOffset = 0
	}

	// A directory that does not fill the window has nothing to scroll: without the
	// floor, the clamp is negative and scrolling pushes the page off the top.
	if maxOffset := float32(max(atlasHeight-scrollingSpace, 0)); a.verticalScrollOffset > maxOffset {
		a.verticalScrollOffset = maxOffset
	}
	return true
}

func (a *GuiAtlasPicker) ShouldClose() bool {
	return a.shouldClose
}

// Browser is the folder being browsed, for a caller that has to turn a picked
// index back into the file it stands for.
func (a *GuiAtlasPicker) Browser() *TexBrowser { return a.browser }

func (a *GuiAtlasPicker) HandleDrop(names []string, xpos float64, ypos float64) bool {
	if a.browser != nil && a.dest == nil {
		// A browser that is not choosing textures has nowhere to put a dropped
		// one, and the grid under the cursor is a directory rather than an atlas
		// with room in it, so the drop is refused rather than landing somewhere.
		return false
	}
	if a.browser != nil {
		// The library is the way in, so a dropped texture is put into it rather
		// than refused: writing it straight into the page being browsed would
		// put it somewhere nothing can point at afterwards, and telling the
		// player to go and copy the file themselves is the same answer with
		// more steps.
		saved, err := a.browser.AcceptDropped(names)
		if saved > 0 {
			a.browser.Open(DroppedDir)
			a.showDir()
		}
		if a.toast != nil {
			switch {
			case err != nil:
				a.toast(err.Error())
			case saved == 0:
				a.toast("nothing dropped that is a PNG")
			default:
				a.toast(fmt.Sprintf("%d texture(s) added to %s/%s", saved, TexLibraryRoot, DroppedDir))
			}
		}
		return true
	}
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
				Image: ImageFromOSPath(name),
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

	if a.dirLabel != nil {
		a.dirLabel.SetPosition(mgl32.Vec2{float32(borderDrawPos.X) + 4, float32(borderDrawPos.Y) - float32(a.fontHeight) - 2})
	}
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
	if a.dirLabel != nil {
		a.dirLabel.Draw()
	}

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
	if a.browser != nil {
		a.browser.Close()
	}
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

// SetOnIndexRightClicked is the second button on a cell, for a picker whose
// cells can be taken away as well as chosen.
func (a *GuiAtlasPicker) SetOnIndexRightClicked(callback func(selection int)) {
	a.onIndexRightClicked = callback
}

func (a *GuiAtlasPicker) StartDrag(xpos float64, ypos float64) int {
	isValid := a.HandleMousePosEvents(xpos, ypos)
	if !isValid {
		return -1
	}
	if a.browser != nil {
		entry, isCell := a.browser.Entry(a.currentSelectionIndex)
		if !isCell {
			return -1
		}
		if entry.IsDir { // a folder is walked into, never dragged out
			a.browser.Open(entry.Path)
			a.showDir()
			return -1
		}
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
