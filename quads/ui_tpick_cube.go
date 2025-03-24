package quads

import (
    "fmt"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
)

type GuiTexturePickerCube struct {
    textures map[voxel.CubeSide]uint16

    textureSlots        *GuiTextureSlots
    atlasPicker         *GuiAtlasPicker
    onClose             func()
    shouldClose         bool
    draggedTextureIndex int
    buttons             []*GuiButton
    hoveredButton       int
    toast               func(string)
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
    g.hoveredButton = -1
    if g.atlasPicker.HandleMousePosEvents(xpos, ypos) {
        return true
    }

    if g.textureSlots.HandleMousePosEvents(xpos, ypos) {
        return true
    }
    for buttonIndex, button := range g.buttons {
        if button.Contains(mgl32.Vec2{float32(xpos), float32(ypos)}) {
            g.hoveredButton = buttonIndex
            return true
        }
    }

    return false
}

func (g *GuiTexturePickerCube) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {

    if button == glfw.MouseButtonLeft {
        if action == glfw.Press {

            for _, uiButton := range g.buttons {
                if uiButton.HandleMouseButtonEvents(xpos, ypos, button, action, mods) {
                    return true
                }
            }

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
    for bi, button := range g.buttons {
        button.Draw(bi == g.hoveredButton)
    }
}

func (g *GuiTexturePickerCube) ShouldClose() bool {
    return g.shouldClose
}

func (g *GuiTexturePickerCube) HandleDrop(names []string, x float64, y float64) bool {
    return g.atlasPicker.HandleDrop(names, x, y)
}

func NewGuiTexturePickerCube(
    shader *glhf.Shader,
    uiTexture *glhf.Texture,
    font BitmapFont,
    texAtlas *glhf.Texture,
    preselection map[voxel.CubeSide]uint16,
    toast func(string),
    onTextureSizeChanged func(),
) *GuiTexturePickerCube {
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

    g := &GuiTexturePickerCube{
        hoveredButton: -1,
        textures:      preselection,
        atlasPicker:   NewGuiAtlasPicker(shader, uiTexture, font, texAtlas),
        textureSlots:  slots,
        toast:         toast,
    }

    saveTex := NewGuiButton(shader, uiTexture, font)
    saveTex.SetText("Save")
    saveTex.Action = func() {
        atlasToSave := g.atlasPicker.atlas
        if atlasToSave.SourceFile != "" {
            errOnSave := atlasToSave.SaveAsPNG(atlasToSave.SourceFile)
            if errOnSave != nil {
                toast(fmt.Sprintf("FAIL: %v", errOnSave))
            } else {
                toast(fmt.Sprintf("Saved to %s", atlasToSave.SourceFile))
            }
        }
    }

    addRow := NewGuiButton(shader, uiTexture, font)
    addRow.SetText("Expand")
    addRow.Action = func() {
        g.atlasPicker.UpdateAtlas(g.atlasPicker.atlas.AddRow())
        selected := g.textureSlots.assignedIndices
        sWidth, sHeight := g.textureSlots.screenWidth, g.textureSlots.screenHeight
        g.textureSlots = NewGuiTextureSlots(shader, uiTexture, font, g.atlasPicker.atlas, cubeSides)
        g.textureSlots.SetCentered(sWidth, sHeight)
        g.textureSlots.assignedIndices = selected
        onTextureSizeChanged()
    }

    guiButtons := []*GuiButton{saveTex, addRow}

    g.buttons = guiButtons
    return g
}

func (g *GuiTexturePickerCube) HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2) {
    g.atlasPicker.HandleResize(screenWidth, screenHeight, screenCoordScale)
    g.textureSlots.SetCentered(screenWidth, screenHeight)
    buttonsStartY := float32(screenHeight) * 0.40
    for _, button := range g.buttons {
        buttonsStartX := screenWidth - button.Width() - 2
        button.SetPosition(mgl32.Vec2{float32(buttonsStartX), float32(buttonsStartY)})
        buttonsStartY += float32(button.Height()) + 4
    }
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
