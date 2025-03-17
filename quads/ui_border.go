package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

type GuiBorder struct {
    shader          *glhf.Shader
    texture         *glhf.Texture
    verts           *glhf.VertexSlice[glhf.GlFloat]
    texIndices      []uint16 // 0: top-left, 1: top-right, 2: bottom-right, 3: bottom-left, 4: top, 5: bottom
    paddingHoriz    int
    paddingVert     int
    backgroundVerts *glhf.VertexSlice[glhf.GlFloat]
    backgroundColor mgl32.Vec4
    borderColor     mgl32.Vec4
    position        mgl32.Vec2
    size            mgl32.Vec2
}

func CornersToPositionAndSize(cornerPositions []mgl32.Vec2) (pos, size mgl32.Vec2) {
    minX := cornerPositions[0].X()
    minY := cornerPositions[0].Y()
    maxX := cornerPositions[2].X()
    maxY := cornerPositions[2].Y()
    return mgl32.Vec2{minX, minY}, mgl32.Vec2{maxX - minX, maxY - minY}
}

// NewGuiBorder creates a new GuiBorder.
func NewGuiBorder(shader *glhf.Shader, texture *glhf.Texture, pos, size mgl32.Vec2) *GuiBorder {
    g := &GuiBorder{
        shader:       shader,
        texture:      texture,
        texIndices:   []uint16{7, 8, 10, 9, 6, 5},
        paddingHoriz: 5,
        paddingVert:  5,
        position:     pos,
        size:         size,
        borderColor:  mgl32.Vec4{1, 1, 1, 1},
    }
    g.uploadVerts()
    return g
}
func (g *GuiBorder) SetBorderColor(colors mgl32.Vec4) {
    g.borderColor = colors
}
func (g *GuiBorder) SetAlpha(alpha float32) {
    g.borderColor = mgl32.Vec4{g.borderColor.X(), g.borderColor.Y(), g.borderColor.Z(), alpha}
    g.backgroundColor = mgl32.Vec4{g.backgroundColor.X(), g.backgroundColor.Y(), g.backgroundColor.Z(), alpha}
}

func (g *GuiBorder) GetAlpha() float32 {
    return g.borderColor.W()
}

func (g *GuiBorder) SetBackground(background mgl32.Vec4) {
    g.backgroundVerts = NewGUIQuadVerts(g.shader, nil, 0, int(-g.paddingHoriz), int(-g.paddingVert), int(g.size.X()+float32(g.paddingHoriz)*2), int(g.size.Y()+float32(g.paddingVert)*2))
    g.backgroundColor = background
}

func (g *GuiBorder) SetPosition(position mgl32.Vec2) {
    g.position = position
}

func (g *GuiBorder) uploadVerts() {
    // we need one quad for each corner
    // and one quad each for a top and bottom border

    tileWidth, tileHeight := g.texture.GetAtlasItemSize()

    var rawVerts []glhf.GlFloat

    tlPos := mgl32.Vec2{float32(-g.paddingHoriz), float32(-g.paddingVert)}
    tlTextureIndex := g.texIndices[0]

    // top-left corner
    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, tlTextureIndex, tlPos, glhf.GlFloat(tileWidth), glhf.GlFloat(tileHeight))

    trPos := mgl32.Vec2{g.size.X() - float32(tileWidth) + float32(g.paddingHoriz), float32(-g.paddingVert)}
    trTextureIndex := g.texIndices[1]

    // top-right corner
    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, trTextureIndex, trPos, glhf.GlFloat(tileWidth), glhf.GlFloat(tileHeight))

    brPos := mgl32.Vec2{g.size.X() - float32(tileWidth) + float32(g.paddingHoriz), g.size.Y() - float32(tileHeight) + float32(g.paddingVert)}
    brTextureIndex := g.texIndices[2]

    // bottom-right corner
    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, brTextureIndex, brPos, glhf.GlFloat(tileWidth), glhf.GlFloat(tileHeight))

    blPos := mgl32.Vec2{float32(-g.paddingHoriz), g.size.Y() - float32(tileHeight) + float32(g.paddingVert)}
    blTextureIndex := g.texIndices[3]

    // bottom-left corner
    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, blTextureIndex, blPos, glhf.GlFloat(tileWidth), glhf.GlFloat(tileHeight))

    // top border (stretch)

    topPos := tlPos
    borderWidth := g.size.X() + float32(g.paddingHoriz)*2
    topTextureIndex := g.texIndices[4]

    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, topTextureIndex, topPos, glhf.GlFloat(borderWidth), glhf.GlFloat(tileHeight))

    // bottom border (stretch)

    bottomPos := blPos
    bottomTextureIndex := g.texIndices[5]

    rawVerts = AppendGUIQuadVertices(rawVerts, g.texture, bottomTextureIndex, bottomPos, glhf.GlFloat(borderWidth), glhf.GlFloat(tileHeight))

    vertices := glhf.MakeVertexSlice(g.shader, len(rawVerts)/4, len(rawVerts)/4)
    vertices.Begin()
    vertices.SetVertexData(rawVerts)
    vertices.End()

    g.verts = vertices
}

func (g *GuiBorder) Draw() {
    g.shader.SetUniformAttr(GuiShaderPosition, g.position)

    if g.backgroundVerts != nil {
        g.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{-1, -1})
        g.shader.SetUniformAttr(GuiShaderTintColor, g.backgroundColor)

        g.backgroundVerts.Begin()
        g.backgroundVerts.Draw()
        g.backgroundVerts.End()

    }
    g.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    g.shader.SetUniformAttr(GuiShaderTintColor, g.borderColor)

    g.texture.Begin()
    g.verts.Begin()
    g.verts.Draw()
    g.verts.End()
    g.texture.End()
}

func (g *GuiBorder) DrawWithoutBackground() {
    g.shader.SetUniformAttr(GuiShaderPosition, g.position)
    g.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    g.shader.SetUniformAttr(GuiShaderTintColor, g.borderColor)

    g.texture.Begin()
    g.verts.Begin()
    g.verts.Draw()
    g.verts.End()
    g.texture.End()
}

func (g *GuiBorder) ContainsPoint(point mgl32.Vec2) bool {
    return point.X() >= g.position.X() && point.X() <= g.position.X()+g.size.X() && point.Y() >= g.position.Y() && point.Y() <= g.position.Y()+g.size.Y()
}
