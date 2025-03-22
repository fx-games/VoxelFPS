package quads

import (
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
)

type GuiTexturePickerCube struct {
    textures map[voxel.CubeSide]uint16
    atlas    *glhf.Texture

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
            consumed = g.draggedTextureIndex != -1 && g.textureSlots.SetSlotAt(xpos, ypos, g.draggedTextureIndex)
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
    return false
}

func NewGuiTexturePickerCube(shader *glhf.Shader, uiTexture *glhf.Texture, font BitmapFont, texAtlas *glhf.Texture, preselection map[voxel.CubeSide]uint16) *GuiTexturePickerCube {
    var cubeSides []string
    for i := voxel.CubeSide(0); i < 6; i++ {
        cubeSides = append(cubeSides, i.String())
    }
    slots := NewGuiTextureSlots(shader, uiTexture, font, texAtlas, cubeSides)
    for i := voxel.CubeSide(0); i < 6; i++ {
        preTex, hasPreselectedValue := preselection[i]
        if !hasPreselectedValue {
            continue
        }
        slots.SetSlot(int(i), preTex)
    }

    return &GuiTexturePickerCube{
        atlas:        texAtlas,
        textures:     preselection,
        atlasPicker:  NewGuiAtlasPicker(shader, uiTexture, font, texAtlas),
        textureSlots: slots,
    }
}

func (g *GuiTexturePickerCube) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
    g.atlasPicker.SetCentered(screenWidth, screenHeight, screenCoordScale)
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
