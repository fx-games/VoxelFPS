package quads

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

// GuiInventory is the two pane inventory overlay: what you are carrying down
// the left, and what the highlighted one of them is down the right.
//
// It is the Fallout 3 shape and almost none of it is new. The left pane is a
// GuiList, which already does the highlighting, the scrolling and the wheel;
// the right pane is a GuiBorder and a BitmapFontMesh, which is what every
// other panel here is made of. What this adds is the picture: an icon out of
// whatever atlas the caller hands it, which for a weapon is the same shared
// atlas its pickup on the floor is drawn from.
type GuiInventory struct {
	uiContext UIContext
	icons     glhf.TextureAtlas

	list   *GuiList
	border *GuiBorder
	detail *BitmapFontMesh
	title  *BitmapFontMesh

	items       []InventoryItem
	screen      mgl32.Vec2
	iconAt      mgl32.Vec2
	iconBox     float32
	shouldClose bool
	onClose     func()
	onSelect    func(index int)
}

// InventoryItem is one row in the list and the page behind it.
type InventoryItem struct {
	Name   string
	Detail []string
	// Icon is where the picture is in the atlas, or -1 for an item with none.
	Icon int
}

// How the overlay is laid out, as fractions of the screen: it covers the
// middle, the list takes the left third of that and the detail pane the rest.
const (
	inventoryWidth   = 0.8
	inventoryHeight  = 0.7
	inventoryListCut = 0.36
	inventoryPadding = 12
)

func NewGuiInventory(uiContext UIContext, icons glhf.TextureAtlas) *GuiInventory {
	inventory := &GuiInventory{
		uiContext: uiContext,
		icons:     icons,
		list:      NewGuiList(uiContext),
		detail:    NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch),
		title:     NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch),
	}
	inventory.detail.SetTintColor(mgl32.Vec4{0.3, 1, 0.3, 1})
	inventory.title.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	return inventory
}

// SetItems replaces what is carried. The selection is kept where it can be,
// so picking something up while the inventory is open does not move the
// highlight out from under the player.
func (v *GuiInventory) SetItems(items []InventoryItem) {
	selected := v.list.GetSelectedIndex()
	v.items = items
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	v.list.SetItems(names)
	if selected >= len(items) {
		selected = len(items) - 1
	}
	v.list.SetSelectedIndex(max(selected, 0))
}

func (v *GuiInventory) SetOnSelect(callback func(index int)) { v.onSelect = callback }
func (v *GuiInventory) SetOnClose(callback func())           { v.onClose = callback }
func (v *GuiInventory) ShouldClose() bool                    { return v.shouldClose }

func (v *GuiInventory) Close() {
	v.shouldClose = true
	if v.onClose != nil {
		v.onClose()
	}
}

func (v *GuiInventory) HandleResize(screenWidth, screenHeight int, screenCoordScale mgl32.Vec2) {
	v.screen = mgl32.Vec2{float32(screenWidth), float32(screenHeight)}
	size := mgl32.Vec2{v.screen.X() * inventoryWidth, v.screen.Y() * inventoryHeight}
	topLeft := mgl32.Vec2{(v.screen.X() - size.X()) / 2, (v.screen.Y() - size.Y()) / 2}

	listWidth := size.X() * inventoryListCut
	v.list.SetOrigin(topLeft.X(), topLeft.Y())
	v.list.SetBounds(listWidth, size.Y())

	detailLeft := topLeft.X() + listWidth + inventoryPadding*2
	detailWidth := size.X() - listWidth - inventoryPadding*2
	v.border = NewGuiBorderWithBounds(v.uiContext,
		mgl32.Vec2{detailLeft, topLeft.Y()}, mgl32.Vec2{detailWidth, size.Y()})
	v.border.SetBackground(mgl32.Vec4{0, 0, 0, 0.6})
	v.border.SetBorderColor(mgl32.Vec4{0.3, 1, 0.3, 1})

	v.title.SetPosition(mgl32.Vec2{detailLeft, topLeft.Y()})
	_, lineHeight := v.title.Size()
	// The picture sits under the name and the text under the picture, which is
	// the Fallout arrangement and the only one that works when the pictures are
	// all different shapes.
	v.iconBox = min(detailWidth, size.Y()/2)
	v.iconAt = mgl32.Vec2{detailLeft, topLeft.Y() + float32(lineHeight) + inventoryPadding}
	v.detail.SetPosition(mgl32.Vec2{detailLeft, v.iconAt.Y() + v.iconBox + inventoryPadding})
}

func (v *GuiInventory) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
	if action != glfw.Press && action != glfw.Repeat {
		return false
	}
	switch key {
	case glfw.KeyEscape, glfw.KeyI, glfw.KeyTab:
		v.Close()
	case glfw.KeyDown, glfw.KeyS:
		v.list.SelectNext()
	case glfw.KeyUp, glfw.KeyW:
		v.list.SelectPrevious()
	case glfw.KeyEnter, glfw.KeyKPEnter, glfw.KeySpace, glfw.KeyE:
		v.selected()
	default:
		return false
	}
	return true
}

func (v *GuiInventory) selected() {
	if v.onSelect != nil && len(v.items) > 0 {
		v.onSelect(v.list.GetSelectedIndex())
	}
}

func (v *GuiInventory) HandleMousePosEvents(xpos, ypos float64) bool {
	return v.list.HandleMousePosEvents(xpos, ypos)
}

func (v *GuiInventory) HandleMouseButtonEvents(xpos, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
	if action != glfw.Press || button != glfw.MouseButtonLeft {
		return false
	}
	if !v.list.HandleMousePosEvents(xpos, ypos) {
		return false
	}
	v.selected()
	return true
}

func (v *GuiInventory) HandleScrollEvents(xoff, yoff float64) bool {
	return v.list.HandleScrollEvents(xoff, yoff)
}

func (v *GuiInventory) HandleDrop(names []string, x, y float64) bool { return false }

func (v *GuiInventory) Draw() {
	if v.border == nil {
		return
	}
	v.list.Draw()
	v.border.Draw()

	selected := v.list.GetSelectedIndex()
	if selected < 0 || selected >= len(v.items) {
		v.title.SetText("nothing carried")
		v.title.Draw()
		return
	}
	item := v.items[selected]
	v.title.SetText(item.Name)
	v.title.Draw()
	v.drawIcon(item.Icon)
	v.detail.SetMultilineText(item.Detail)
	v.detail.Draw()
}

// drawIcon paints the picture as large as it fits in its box.
func (v *GuiInventory) drawIcon(index int) {
	if index >= 0 && v.icons != nil {
		DrawIconInBox(v.icons, index, v.iconAt, v.iconBox)
	}
}

// DrawIconInBox paints one picture out of a packed atlas as large as it fits
// in a square box, centred and keeping its shape. A packed atlas has a
// different size for every frame, so the scale is worked out per picture.
//
// It places the picture itself rather than going through
// GetSourceAndDestRects, which puts a frame where its own canvas and hotspot
// say to. That is exactly right for a weapon sprite drawn in Doom's 320x200
// space and exactly wrong here: a fist frame authored a third of the way down
// a full-screen canvas landed six hundred pixels outside its own pane.
func DrawIconInBox(atlas glhf.TextureAtlas, index int, at mgl32.Vec2, box float32) {
	left, top, right, bottom := atlas.GetUVInPx(index)
	width, height := float32(right-left), float32(bottom-top)
	if width <= 0 || height <= 0 {
		return
	}
	scale := min(box/width, box/height)
	width, height = width*scale, height*scale
	corner := at.Add(mgl32.Vec2{(box - width) / 2, (box - height) / 2})
	atlas.GetSpriteBatch().DrawTextured(atlas.GetSourceRect(index),
		mgl32.Vec4{corner.X(), corner.Y(), corner.X() + width, corner.Y() + height},
		mgl32.Vec4{1, 1, 1, 1})
}
