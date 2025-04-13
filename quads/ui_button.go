package quads

import (
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
)

type GuiButton struct {
    border          *GuiBorder
    label           *BitmapFontMesh
    colorForeground mgl32.Vec4
    colorBackground mgl32.Vec4
    Action          func()
}

func NewGuiButton(uiContext UIContext) *GuiButton {
    g := &GuiButton{
        border:          NewGuiBorder(uiContext),
        label:           NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch),
        colorForeground: mgl32.Vec4{1, 1, 1, 1},
        colorBackground: mgl32.Vec4{0, 0, 0, 0.8},
    }
    return g
}

func (g *GuiButton) SetText(text string) {
    g.label.SetText(text)
    g.border.SetSize(g.label.SizeAsVec2())
}

func (g *GuiButton) SetPosition(pos mgl32.Vec2) {
    g.border.SetPosition(pos)
    g.label.SetPosition(pos)
}

func (g *GuiButton) Draw(isHovered bool) {
    if isHovered {
        g.label.SetTintColor(g.colorBackground)
        g.label.SetBackground(g.colorForeground)
    } else {
        g.label.SetTintColor(g.colorForeground)
        g.label.NoBackground()
    }
    g.border.Draw()
    g.label.Draw()
}

func (g *GuiButton) Contains(mousePosInPx mgl32.Vec2) bool {
    return g.border.ContainsPoint(mousePosInPx)
}

func (g *GuiButton) Width() int {
    return g.border.Width()
}

func (g *GuiButton) Height() int {
    return g.border.Height()
}
func (g *GuiButton) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
    if action != glfw.Press || button != glfw.MouseButtonLeft || !g.Contains(mgl32.Vec2{float32(xpos), float32(ypos)}) {
        return false
    }
    if g.Action != nil {
        g.Action()
    }
    return true
}
