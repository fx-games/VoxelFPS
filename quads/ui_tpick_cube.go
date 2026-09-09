package quads

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/voxel"
)

type GuiTexturePickerCube struct {
	textures map[voxel.CubeSide]uint16

	textureSlots        *GuiTextureSlots
	atlasPicker         *GuiAtlasPicker
	onClose             func()
	shouldClose         bool
	draggedTextureIndex int
}

func (g *GuiTexturePickerCube) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
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

func (g *GuiTexturePickerCube) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {
	if g.atlasPicker.HandleMousePosEvents(xpos, ypos) {
		return true
	}
	return g.textureSlots.HandleMousePosEvents(xpos, ypos)
}

func (g *GuiTexturePickerCube) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {

	if button == glfw.MouseButtonLeft {
		if action == glfw.Press {
			g.draggedTextureIndex = g.atlasPicker.StartDrag(xpos, ypos)
			return g.draggedTextureIndex != -1
		} else if action == glfw.Release {
			g.atlasPicker.EndDrag()
			// Resolving on the drop, not on the pick up: a library texture dragged
			// around and let go over nothing must not have cost the atlas a slot.
			if g.draggedTextureIndex != -1 && g.textureSlots.IsSlotAt(xpos, ypos) {
				if slot, isChoice := g.atlasPicker.Resolve(g.draggedTextureIndex); isChoice {
					consumed = g.textureSlots.SetSlotAt(xpos, ypos, slot)
				}
			}
			if consumed {
				g.texturesChanged()
			}
			g.draggedTextureIndex = -1

			return consumed
		}
	}

	return false
}

func (g *GuiTexturePickerCube) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
	if g.atlasPicker.HandleScrollEvents(xoff, yoff) {
		return true
	}
	return g.textureSlots.HandleScrollEvents(xoff, yoff)
}

func (g *GuiTexturePickerCube) Draw() {
	g.textureSlots.Draw()
	g.atlasPicker.Draw()
}

func (g *GuiTexturePickerCube) ShouldClose() bool {
	return g.shouldClose
}

func (g *GuiTexturePickerCube) HandleDrop(names []string, x float64, y float64) bool {
	return g.atlasPicker.HandleDrop(names, x, y)
}

// NewGuiTexturePickerCube is the six sides of a block, filled by dragging out of
// the texture library. The slots show the map's own atlas - that is where a
// dropped texture lands, and what the block's stored indices mean - while the
// grid above them is whatever library folder is open.
func NewGuiTexturePickerCube(
	uiContext UIContext,
	dest *AtlasIndex,
	browser *TexBrowser,
	preselection map[voxel.CubeSide]uint16,
	toast func(string),
) *GuiTexturePickerCube {
	var cubeSides []string
	for i := voxel.CubeSide(0); i < 6; i++ {
		cubeSides = append(cubeSides, i.String())
	}
	slots := NewGuiTextureSlots(uiContext, dest.Atlas, cubeSides)
	for i := voxel.CubeSide(0); i < 6; i++ {
		preTex, hasPreselectedValue := preselection[i]
		if !hasPreselectedValue {
			continue
		}
		slots.SetSlot(int(i), preTex)
	}

	g := &GuiTexturePickerCube{
		textures:     preselection,
		atlasPicker:  NewGuiAtlasPicker(uiContext, browser.Atlas()),
		textureSlots: slots,
	}
	g.atlasPicker.SetBrowser(browser, dest, toast)
	return g
}

func (g *GuiTexturePickerCube) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
	g.atlasPicker.HandleResize(screenWidth, screenHeight, screenCoordScale)
	g.textureSlots.SetCentered(screenWidth, screenHeight)
}

func (g *GuiTexturePickerCube) SetOnClose(closeCallback func()) {
	g.onClose = closeCallback
}

func (g *GuiTexturePickerCube) Close() {
	g.shouldClose = true
	g.atlasPicker.Close()
	g.textureSlots.Close()
	if g.onClose != nil {
		g.onClose()
	}
}

func (g *GuiTexturePickerCube) texturesChanged() {
	for i := 0; i < 6; i++ {
		index := g.textureSlots.GetSlotIndex(i)
		if index < 0 {
			index = 0
		}
		g.textures[voxel.CubeSide(i)] = index
	}
}
