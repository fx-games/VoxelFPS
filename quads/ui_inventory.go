package quads

import (
	"slices"
	"strings"

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
	tabBar *BitmapFontMesh

	// items is everything carried and shown is the page of it the open tab
	// holds. The list only ever knows about shown, so every index it hands back
	// is an index into that.
	items       []InventoryItem
	shown       []InventoryItem
	tabs        []string
	tab         int
	screen      mgl32.Vec2
	iconAt      mgl32.Vec2
	iconBox     float32
	shouldClose bool
	onClose     func()
}

// InventoryItem is one row in the list and the page behind it.
type InventoryItem struct {
	Name   string
	Detail []string
	// Icon is where the picture is in the atlas, or -1 for an item with none.
	Icon int
	// Tab is which page of the inventory this item is on. The pages are in the
	// order the items first name them, so what tabs there are is decided by
	// what is being carried rather than by a list here that would go stale the
	// moment there was a new kind of thing to carry.
	Tab string
	// Use is what E on this row does - hold a weapon, play a recording - or nil
	// for something that is only there to be looked at. It is a closure on the
	// item rather than an index handed back to the caller because a tab shows
	// some of what is carried, so a row's position is no longer a name for it.
	Use func()
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
		tabBar:    NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch),
	}
	inventory.detail.SetTintColor(mgl32.Vec4{0.3, 1, 0.3, 1})
	inventory.title.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	inventory.tabBar.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	return inventory
}

// SetItems replaces what is carried, and works out what tabs that makes: one
// per distinct Tab, in the order the items name them. An open tab that is still
// there stays open.
func (v *GuiInventory) SetItems(items []InventoryItem) {
	v.items = items
	open := ""
	if v.tab < len(v.tabs) {
		open = v.tabs[v.tab]
	}
	v.tabs = nil
	for _, item := range items {
		if !slices.Contains(v.tabs, item.Tab) {
			v.tabs = append(v.tabs, item.Tab)
		}
	}
	v.tab = max(slices.Index(v.tabs, open), 0)
	v.showTab()
}

// showTab puts the open tab's items in the list. The selection is kept where it
// can be, so picking something up while the inventory is open does not move the
// highlight out from under the player.
func (v *GuiInventory) showTab() {
	selected := v.list.GetSelectedIndex()
	v.shown = nil
	for _, item := range v.items {
		if v.tab < len(v.tabs) && item.Tab != v.tabs[v.tab] {
			continue
		}
		v.shown = append(v.shown, item)
	}
	names := make([]string, len(v.shown))
	for i, item := range v.shown {
		names[i] = item.Name
	}
	v.list.SetItems(names)
	if selected >= len(v.shown) {
		selected = len(v.shown) - 1
	}
	v.list.SetSelectedIndex(max(selected, 0))
}

// selectTab opens the tab one along, wrapping. The highlight starts again at
// the top, since a row number means something different on every page.
func (v *GuiInventory) selectTab(step int) {
	if len(v.tabs) < 2 {
		return
	}
	v.tab = (v.tab + step + len(v.tabs)) % len(v.tabs)
	v.list.SetSelectedIndex(0)
	v.showTab()
}

// tabLine is the strip above the list: every tab, the open one in brackets.
func (v *GuiInventory) tabLine() string {
	var parts []string
	for index, name := range v.tabs {
		if index == v.tab {
			parts = append(parts, "["+name+"]")
			continue
		}
		parts = append(parts, " "+name+" ")
	}
	return strings.Join(parts, " ")
}

func (v *GuiInventory) SetOnClose(callback func()) { v.onClose = callback }
func (v *GuiInventory) ShouldClose() bool          { return v.shouldClose }

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
	// The tabs sit above both panes, which is the only place they can: the list
	// is a scrolling box of its own and the detail pane belongs to one row. The
	// strip has to be written before it can be measured - a font mesh measures
	// the text it was last given, and one that has never been given any is no
	// height at all, which puts the tabs straight through the list's top edge.
	v.tabBar.SetText(v.tabLine())
	_, tabHeight := v.tabBar.Size()
	v.tabBar.SetPosition(mgl32.Vec2{topLeft.X(), topLeft.Y() - float32(tabHeight) - inventoryPadding})
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
	case glfw.KeyEscape, glfw.KeyI:
		v.Close()
	case glfw.KeyTab, glfw.KeyRight, glfw.KeyD:
		v.selectTab(1)
	case glfw.KeyLeft, glfw.KeyA:
		v.selectTab(-1)
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

// selected does whatever the highlighted row is for, which is the row's own
// business and may be nothing at all - a note is read in the pane beside it.
func (v *GuiInventory) selected() {
	index := v.list.GetSelectedIndex()
	if index < 0 || index >= len(v.shown) {
		return
	}
	if use := v.shown[index].Use; use != nil {
		use()
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
	if len(v.tabs) > 1 {
		v.tabBar.SetText(v.tabLine())
		v.tabBar.Draw()
	}

	selected := v.list.GetSelectedIndex()
	if selected < 0 || selected >= len(v.shown) {
		v.title.SetText("nothing carried")
		v.title.Draw()
		return
	}
	item := v.shown[selected]
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
