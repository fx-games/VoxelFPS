package quads

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"math"
)

type GuiList struct {
	labels        []*BitmapFontMesh
	selectedIndex int

	textColor       mgl32.Vec4
	textSelectColor mgl32.Vec4
	fontHeight      int
	listOrigin      mgl32.Vec2
	size            mgl32.Vec2

	isHidden         bool
	isCentered       bool
	vertScrollOffset int
	items            []string
	keyHandler       func(selectedIndex int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool
	mouseHandler     func(selectedIndex int, xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool

	border *GuiBorder

	scale       mgl32.Vec2
	shouldClose bool

	maxWidth       float32
	maxHeight      float32
	originIsCenter bool
	layoutIsDirty  bool

	closeCallback func()
	uiContext     UIContext
}

// NewGuiList creates a new GuiList.
func NewGuiList(uiContext UIContext) *GuiList {
	_, h := uiContext.Font.Texture.GetAtlasItemSize()
	return &GuiList{
		uiContext:       uiContext,
		fontHeight:      h,
		textColor:       mgl32.Vec4{0.3, 1, 0.3, 1},
		textSelectColor: mgl32.Vec4{0.0, 0.2, 0, 1},
		scale:           mgl32.Vec2{1, 1},
		maxWidth:        math.MaxFloat32,
		maxHeight:       math.MaxFloat32,
		layoutIsDirty:   true,
	}
}

func (g *GuiList) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
}

func (g *GuiList) HandleDrop(names []string, x float64, y float64) bool {
	return false
}

func (g *GuiList) ShouldClose() bool {
	return g.shouldClose
}

func (g *GuiList) Close() {
	g.shouldClose = true
	if g.closeCallback != nil {
		g.closeCallback()
	}
}

func (g *GuiList) SetKeyHandler(handler func(selectedIndex int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool) {
	g.keyHandler = handler
}

func (g *GuiList) SetMouseHandler(handler func(selectedIndex int, xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool) {
	g.mouseHandler = handler
}

func (g *GuiList) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
	if action != glfw.Press {
		return false
	}
	if g.keyHandler != nil {
		return g.keyHandler(g.selectedIndex, key, scancode, action, mods)
	}
	return false
}

func (g *GuiList) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {
	listWidth := g.size[0]
	for idx, label := range g.labels {
		pos, size := label.GetRect()
		if float32(xpos) >= pos[0] && float32(xpos) <= pos[0]+listWidth && float32(ypos) >= pos[1] && float32(ypos) <= pos[1]+size[1] {
			g.selectedIndex = idx
			return true
		}
	}
	return false
}

func (g *GuiList) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
	if action != glfw.Press {
		return false
	}
	if g.mouseHandler != nil {
		return g.mouseHandler(g.selectedIndex, xpos, ypos, button, action, mods)
	}
	return false
}

func (g *GuiList) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
	if yoff > 0 {
		g.SelectNext()
	} else {
		g.SelectPrevious()
	}
	return true
}

func (g *GuiList) SetOnClose(closeCallback func()) {
	g.closeCallback = closeCallback
}

func (g *GuiList) SelectNext() {
	if len(g.labels) == 0 {
		g.selectedIndex = 0
		return
	}
	g.selectedIndex = (g.selectedIndex + 1) % len(g.labels)
	g.ensureSelectedVisible()
}

func (g *GuiList) SelectPrevious() {
	if len(g.labels) == 0 {
		g.selectedIndex = 0
		return
	}
	g.selectedIndex = g.selectedIndex - 1
	if g.selectedIndex < 0 {
		g.selectedIndex = len(g.labels) - 1
	}
	g.ensureSelectedVisible()
}

func (g *GuiList) ScrollUp() {
	if !g.verticalScrollingNeeded() {
		return
	}
	g.vertScrollOffset -= 1
	if g.vertScrollOffset < 0 {
		g.vertScrollOffset = 0
	}
}

func (g *GuiList) ScrollDown() {
	if !g.verticalScrollingNeeded() {
		return
	}
	g.vertScrollOffset += 1
	if g.vertScrollOffset > len(g.labels)-g.linesShown() {
		g.vertScrollOffset = len(g.labels) - g.linesShown()
	}
}

func (g *GuiList) ensureSelectedVisible() {
	if g.selectedIndex < g.vertScrollOffset {
		g.vertScrollOffset = g.selectedIndex
	} else if g.selectedIndex > g.lastVisibleLine() {
		g.vertScrollOffset = g.selectedIndex - g.linesShown() + 1
	}
}

func (g *GuiList) GetSelectedIndex() int {
	return g.selectedIndex
}

func (g *GuiList) SetSelectedIndex(idx int) {
	g.selectedIndex = idx
}

func (g *GuiList) yFromIndex(idx int) float32 {
	return float32(g.lineHeight() * idx)
}

func (g *GuiList) heightNeeded(items []string, maxHeight int) int {
	if len(items) == 0 {
		return maxHeight
	}
	return g.lineHeight() * min(len(items), maxHeight/g.lineHeight())
}

func (g *GuiList) widthNeeded(items []string, maxWidth int) int {
	if len(items) == 0 {
		return maxWidth
	}
	var widthNeeded int
	for idx := 0; idx < len(items); idx++ {
		strW, _ := g.uiContext.Font.MeasureString([]string{items[idx]}, mgl32.Vec2{1, 1}, 0, 2)
		if strW > widthNeeded {
			widthNeeded = strW
		}
	}
	return min(widthNeeded, maxWidth)
}

func (g *GuiList) linesShown() int {
	return int(g.size[1]) / g.lineHeight()
}

func (g *GuiList) lineHeight() int {
	horizPadding := 2
	return int((float32(g.fontHeight) * g.scale.Y()) + float32(horizPadding))
}

func (g *GuiList) verticalScrollingNeeded() bool {
	if len(g.labels) == 0 {
		return false
	}
	return g.linesShown() < len(g.labels)
}

func (g *GuiList) lastVisibleLine() int {
	return g.vertScrollOffset + g.linesShown() - 1
}

func (g *GuiList) SetItems(items []string) {
	g.items = items
	g.layoutIsDirty = true
}

func (g *GuiList) SetBounds(maxWidth, maxHeight float32) {
	g.maxWidth = maxWidth
	g.maxHeight = maxHeight
	g.layoutIsDirty = true
}

func (g *GuiList) SetCenteredOnScreen() {
	g.originIsCenter = true
	g.layoutIsDirty = true
}

func (g *GuiList) SetOrigin(x, y float32) {
	g.listOrigin = mgl32.Vec2{x, y}
	g.layoutIsDirty = true
}

func (g *GuiList) layout() {
	g.size = mgl32.Vec2{float32(g.widthNeeded(g.items, int(g.maxWidth))), float32(g.heightNeeded(g.items, int(g.maxHeight)))}
	g.vertScrollOffset = 0
	topLeft := g.topLeft()
	size := g.size
	items := g.items
	// 0: top-left, 1: top-right, 2: bottom-right, 3: bottom-left

	g.border = NewGuiBorderWithBounds(g.uiContext, topLeft, size)
	g.border.SetBackground(mgl32.Vec4{0, 0, 0, 0.6})
	g.border.SetBorderColor(g.textColor)

	g.labels = nil
	for idx := 0; idx < len(items); idx++ {
		mesh := g.newFontMesh(items[idx])
		//curPos := mgl32.Vec2{g.size.X() / 2, g.yFromIndex(idx) + float32(g.lineHeight()/2)}
		curPos := mgl32.Vec2{0, g.yFromIndex(idx)}
		if g.isCentered {
			curPos = curPos.Add(mgl32.Vec2{g.size.X() / 2, float32(g.lineHeight() / 2)})
		}

		drawPos := topLeft.Add(curPos)

		mesh.SetPosition(drawPos)
		g.labels = append(g.labels, mesh)
	}
}

func (g *GuiList) ReplaceText(index int, text string) {
	if index < 0 || index >= len(g.labels) {
		return
	}
	width, _ := g.uiContext.Font.MeasureString([]string{text}, g.scale, 0, 2)
	//width, _ := g.labels[index].CalculateSizeScaled([]string{text})
	if width > int(g.size.X()) {
		g.labels[index].SetScale(mgl32.Vec2{(float32(g.size.X()) / float32(width)) * g.scale.X(), g.scale.Y()})
	} else {
		g.labels[index].SetScale(g.scale)
	}
	g.labels[index].SetText(text)
}

func (g *GuiList) newFontMesh(label string) *BitmapFontMesh {
	size := g.size
	mesh := NewBitmapFontMesh(g.uiContext.Shader, g.uiContext.Font, g.uiContext.ColorBatch)
	mesh.SetWidth(int(size.X()))
	mesh.SetCenterOrigin(g.isCentered)
	mesh.SetBackground(g.textColor)
	mesh.SetTintColor(g.textColor)
	itemW, _ := mesh.CalculateSizeScaled([]string{label})
	itemW = int(float32(itemW) * g.scale.X())
	if itemW > int(size.X()) {
		mesh.SetScale(mgl32.Vec2{(float32(size.X()) / float32(itemW)) * g.scale.X(), g.scale.Y()})
	} else {
		mesh.SetScale(g.scale)
	}
	mesh.SetText(label)
	return mesh
}

func (g *GuiList) topLeft() mgl32.Vec2 {
	if g.originIsCenter {
		return mgl32.Vec2{g.listOrigin.X() - g.size.X()/2, g.listOrigin.Y() - g.size.Y()/2}
	}
	return g.listOrigin
}

func (g *GuiList) Draw() {
	if g.isHidden {
		return
	}
	if g.layoutIsDirty {
		g.layout()
		g.layoutIsDirty = false
	}

	if len(g.labels) > 0 {
		g.border.Draw()
	} else {
		g.border.DrawWithoutBackground()
	}

	for idx := 0; idx < min(g.linesShown(), len(g.labels)); idx++ {
		itemIndex := idx + g.vertScrollOffset
		if itemIndex >= len(g.labels) {
			break
		}
		mesh := g.labels[itemIndex]
		if itemIndex == g.selectedIndex {
			mesh.SetTintColor(g.textSelectColor)
			mesh.SetBackground(g.textColor)
		} else {
			mesh.SetTintColor(g.textColor)
			mesh.NoBackground()
		}

		mesh.Draw()
	}
}

func (g *GuiList) SetTextCentered(centered bool) {
	g.isCentered = centered
}

func (g *GuiList) Hide() {
	g.isHidden = true
}

func (g *GuiList) Show() {
	g.isHidden = false
}

func (g *GuiList) RemoveByIndex(index int) {
	if index < 0 || index >= len(g.labels) {
		return
	}
	g.labels = append(g.labels[:index], g.labels[index+1:]...)
	g.items = append(g.items[:index], g.items[index+1:]...)
	if g.selectedIndex >= len(g.labels) {
		g.selectedIndex = len(g.labels) - 1
	}
	g.ensureSelectedVisible()
}

func (g *GuiList) IsVisible() bool {
	return !g.isHidden
}
