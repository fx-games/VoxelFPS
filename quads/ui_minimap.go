package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

type Minimap struct {
    Texture       *glhf.Texture
    Pixels        []uint8
    Width, Height int
    GetColor      func(x, y int) [4]uint8
    GuiQuad       *glhf.VertexSlice[glhf.GlFloat]
    Scale         float32
    DrawPos       mgl32.Vec2
}

func (m *Minimap) Update() {
    for y := 0; y < m.Height; y++ {
        for x := 0; x < m.Width; x++ {
            col := m.GetColor(x, y)
            idx := (y*m.Width + x) * 4

            m.Pixels[idx] = col[0]
            m.Pixels[idx+1] = col[1]
            m.Pixels[idx+2] = col[2]
            m.Pixels[idx+3] = col[3]
        }
    }
    m.Texture.Begin()
    m.Texture.SetPixels(0, 0, m.Width, m.Height, m.Pixels)
    m.Texture.End()
}

func (m *Minimap) Draw(guiShader *glhf.Shader) {
    guiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    guiShader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    guiShader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    guiShader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{m.Scale, m.Scale})
    guiShader.SetUniformAttr(GuiShaderPosition, m.DrawPos)

    m.GuiQuad.Begin()
    m.Texture.Begin()

    m.GuiQuad.Draw()

    m.Texture.End()
    m.GuiQuad.End()
}

func (m *Minimap) Resize(windowWidth int, windowHeight int) {
    xPosRightBorder := float32(windowWidth) - float32(m.Width)*m.Scale
    m.DrawPos = mgl32.Vec2{xPosRightBorder, 0}
}
