package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

type GuiButton struct {
    border          *GuiBorder
    label           *BitmapFontMesh
    colorForeground mgl32.Vec4
    colorBackground mgl32.Vec4
}

func NewGuiButton(uiShader *glhf.Shader, uiTexture *glhf.Texture, font BitmapFont, pos mgl32.Vec2, label string) *GuiButton {
    fontMesh := NewBitmapFontMesh(uiShader, font)
    fontMesh.SetText(label)
    fontMesh.SetPosition(pos)

    g := &GuiButton{
        border: NewGuiBorder(uiShader, uiTexture, pos, fontMesh.SizeAsVec2()),
        label:  fontMesh,
    }
    return g
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
