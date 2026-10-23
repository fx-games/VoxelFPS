package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxelfps/quads/glhf"
)

type Minimap struct {
	Texture       *glhf.Texture
	Pixels        []uint8
	Width, Height int
	GetColor      func(x, y int) [4]uint8
	GuiQuad       *glhf.VertexArray[glhf.GlFloat]
	Scale         float32
	DrawPos       mgl32.Vec2
	shader        *glhf.Shader
}

func NewMiniMap(shader *glhf.Shader, width int, height int, getColor func(x int, y int) [4]uint8) *Minimap {
	scale := float32(4)
	emptyMap := make([]uint8, width*height*4)
	minimapTexture := glhf.NewTextureRGBA(width, height, false, emptyMap, gl.TEXTURE_2D)

	minimap := &Minimap{
		Width:    width,
		Height:   height,
		GetColor: getColor,
		Texture:  minimapTexture,
		Pixels:   emptyMap,
		Scale:    scale,
		shader:   shader,
	}
	return minimap
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
	m.Texture.SetPixelsRGBA(0, 0, m.Width, m.Height, m.Pixels)
	m.Texture.End()
}

func (m *Minimap) Draw() {
	m.Texture.Begin()

	m.GuiQuad.Begin()
	m.GuiQuad.Draw()
	m.GuiQuad.End()

	m.Texture.End()
}

func (m *Minimap) Resize(windowWidth int, windowHeight int) {
	xPosRightBorder := float32(windowWidth) - float32(m.Width)*m.Scale
	m.DrawPos = mgl32.Vec2{xPosRightBorder, 0}
	layer := float32(0.9)
	m.GuiQuad = NewGUIQuadVerts(m.shader, layer, xPosRightBorder, 0, float32(m.Width), float32(m.Height), m.Scale)
	m.GuiQuad.DebugName = "Minimap"
}
