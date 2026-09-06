package quads

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"math"
)

// scrollbarThickness and scrollbarGap are the bar's own width and how far it
// stands off from the text beside it, in the same pixels everything else in
// this file is measured in.
const (
	scrollbarThickness = 6
	scrollbarGap       = 4
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

	border    *GuiBorder
	scrollbar *GuiScrollbar

	scale       mgl32.Vec2
	shouldClose bool

	maxWidth       float32
	maxHeight      float32
	originIsCenter bool
	layoutIsDirty  bool

	// scrollbarReserve is how much of size.X() the scrollbar is taking up
	// this layout, 0 when every item fits and there is nothing to scroll.
	scrollbarReserve float32

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
		scrollbar:       NewGuiScrollbar(uiContext),
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

// HandleMousePosEvents only tests the rows Draw actually put on screen: an
// off-screen label keeps whatever position it was last drawn at, which is
// wherever it happened to scroll past, and it would ghost-hit for a click
// nowhere near it if this walked the whole list the way it used to when
// every label's position was fixed for good at layout.
func (g *GuiList) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {
	listWidth := g.size[0]
	for idx := 0; idx < min(g.linesShown(), len(g.labels)); idx++ {
		itemIndex := idx + g.vertScrollOffset
		if itemIndex >= len(g.labels) {
			break
		}
		pos, size := g.labels[itemIndex].GetRect()
		if float32(xpos) >= pos[0] && float32(xpos) <= pos[0]+listWidth && float32(ypos) >= pos[1] && float32(ypos) <= pos[1]+size[1] {
			g.selectedIndex = itemIndex
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
	g.scrollbarReserve = 0
	textMaxWidth := g.maxWidth
	if g.needsScrollbarFor(len(g.items)) {
		g.scrollbarReserve = scrollbarGap + scrollbarThickness
		textMaxWidth -= g.scrollbarReserve
	}
	g.size = mgl32.Vec2{float32(g.widthNeeded(g.items, int(textMaxWidth))) + g.scrollbarReserve, float32(g.heightNeeded(g.items, int(g.maxHeight)))}
	g.vertScrollOffset = 0
	topLeft := g.topLeft()
	size := g.size
	items := g.items
	// 0: top-left, 1: top-right, 2: bottom-right, 3: bottom-left

	g.border = NewGuiBorderWithBounds(g.uiContext, topLeft, size)
	g.border.SetBackground(mgl32.Vec4{0, 0, 0, 0.6})
	g.border.SetBorderColor(g.textColor)

	if g.scrollbarReserve > 0 {
		barPos := topLeft.Add(mgl32.Vec2{size.X() - scrollbarThickness, 0})
		g.scrollbar.SetBounds(barPos, mgl32.Vec2{scrollbarThickness, size.Y()})
	}

	g.labels = nil
	for idx := 0; idx < len(items); idx++ {
		mesh := g.newFontMesh(items[idx])
		mesh.SetPosition(g.rowPosition(idx))
		g.labels = append(g.labels, mesh)
	}
}

// rowPosition is where the row-th line on screen belongs, top-left or
// centred to match isCentered. It takes a screen row rather than an item
// index on purpose: Draw calls it with the row a scrolled-to item currently
// occupies, not the fixed place it would sit at unscrolled, which is what
// actually moves the text when the list scrolls rather than just changing
// which already-positioned mesh gets drawn where it always was.
func (g *GuiList) rowPosition(row int) mgl32.Vec2 {
	curPos := mgl32.Vec2{0, g.yFromIndex(row)}
	if g.isCentered {
		curPos = curPos.Add(mgl32.Vec2{g.textWidth() / 2, float32(g.lineHeight() / 2)})
	}
	return g.topLeft().Add(curPos)
}

// needsScrollbarFor is verticalScrollingNeeded's own question, asked before
// layout has anywhere to store the answer - about to lay out itemCount items,
// will they all fit in the height this list was given.
func (g *GuiList) needsScrollbarFor(itemCount int) bool {
	if itemCount == 0 {
		return false
	}
	return int(g.maxHeight)/g.lineHeight() < itemCount
}

// textWidth is how much of size.X() text actually gets to use - all of it,
// unless the scrollbar standing at the right edge is taking a slice.
func (g *GuiList) textWidth() float32 {
	return g.size.X() - g.scrollbarReserve
}

func (g *GuiList) ReplaceText(index int, text string) {
	if index < 0 || index >= len(g.labels) {
		return
	}
	width, _ := g.uiContext.Font.MeasureString([]string{text}, g.scale, 0, 2)
	//width, _ := g.labels[index].CalculateSizeScaled([]string{text})
	if float32(width) > g.textWidth() {
		g.labels[index].SetScale(mgl32.Vec2{(g.textWidth() / float32(width)) * g.scale.X(), g.scale.Y()})
	} else {
		g.labels[index].SetScale(g.scale)
	}
	g.labels[index].SetText(text)
}

func (g *GuiList) newFontMesh(label string) *BitmapFontMesh {
	size := mgl32.Vec2{g.textWidth(), g.size.Y()}
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
		mesh.SetPosition(g.rowPosition(idx))
		if itemIndex == g.selectedIndex {
			mesh.SetTintColor(g.textSelectColor)
			mesh.SetBackground(g.textColor)
		} else {
			mesh.SetTintColor(g.textColor)
			mesh.NoBackground()
		}

		mesh.Draw()
	}

	if g.verticalScrollingNeeded() {
		g.scrollbar.SetRange(len(g.labels), g.linesShown(), g.vertScrollOffset)
		g.scrollbar.Draw()
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
