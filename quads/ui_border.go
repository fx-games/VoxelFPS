package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

type GuiBorder struct {
	shader          *glhf.Shader
	texture         glhf.TextureAtlas
	texIndices      []uint16 // 0: top-left, 1: top-right, 2: bottom-right, 3: bottom-left, 4: top, 5: bottom
	paddingHoriz    int
	paddingVert     int
	backgroundColor mgl32.Vec4
	borderColor     mgl32.Vec4
	position        mgl32.Vec2
	size            mgl32.Vec2
	drawBackground  bool
	colorBatch      *glhf.SpriteBatch
}

func CornersToPositionAndSize(cornerPositions []mgl32.Vec2) (pos, size mgl32.Vec2) {
	minX := cornerPositions[0].X()
	minY := cornerPositions[0].Y()
	maxX := cornerPositions[2].X()
	maxY := cornerPositions[2].Y()
	return mgl32.Vec2{minX, minY}, mgl32.Vec2{maxX - minX, maxY - minY}
}
func NewGuiBorder(uiContext UIContext) *GuiBorder {
	g := &GuiBorder{
		shader:       uiContext.Shader,
		texture:      uiContext.Atlas,
		texIndices:   []uint16{7, 8, 10, 9, 6, 5},
		paddingHoriz: 5,
		paddingVert:  5,
		borderColor:  mgl32.Vec4{1, 1, 1, 1},
		colorBatch:   uiContext.ColorBatch,
	}
	return g
}

// NewGuiBorderWithBounds creates a new GuiBorder.
func NewGuiBorderWithBounds(uiContext UIContext, pos, size mgl32.Vec2) *GuiBorder {
	g := NewGuiBorder(uiContext)
	g.SetPosition(pos)
	g.SetSize(size)
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
	g.backgroundColor = background
	g.drawBackground = true
}

func (g *GuiBorder) SetPosition(position mgl32.Vec2) {
	g.position = position
}

func (g *GuiBorder) getBackgroundDstRect() mgl32.Vec4 {
	minX := g.position.X() - float32(g.paddingHoriz)
	minY := g.position.Y() - float32(g.paddingVert)
	maxX := g.position.X() + g.size.X() + float32(g.paddingHoriz)
	maxY := g.position.Y() + g.size.Y() + float32(g.paddingVert)
	return mgl32.Vec4{minX, minY, maxX, maxY}
}

func (g *GuiBorder) Draw() {
	if g.drawBackground {
		g.colorBatch.DrawColoredRect(g.getBackgroundDstRect(), g.backgroundColor)
	}
	g.DrawWithoutBackground()
}

func (g *GuiBorder) DrawWithoutBackground() {
	// we need one quad for each corner
	// and one quad each for a top and bottom border
	batch := g.texture.GetSpriteBatch()

	// top-left corner
	tlPos := mgl32.Vec2{float32(-g.paddingHoriz), float32(-g.paddingVert)}.Add(g.position)
	tlTextureIndex := g.texIndices[0]
	tlSrc, tlDst := g.texture.GetSourceAndDestRects(int(tlTextureIndex), tlPos, mgl32.Vec2{0, 0}, mgl32.Vec2{1, 1})
	batch.DrawTextured(tlSrc, tlDst, g.borderColor)

	// top-right corner
	trPos := mgl32.Vec2{float32(g.size.X()) + float32(g.paddingHoriz), float32(-g.paddingVert)}.Add(g.position)
	trTextureIndex := g.texIndices[1]
	trSrc, trDst := g.texture.GetSourceAndDestRects(int(trTextureIndex), trPos, mgl32.Vec2{1, 0}, mgl32.Vec2{1, 1})
	batch.DrawTextured(trSrc, trDst, g.borderColor)

	// bottom-left corner
	blPos := mgl32.Vec2{float32(-g.paddingHoriz), float32(g.size.Y()) + float32(g.paddingVert)}.Add(g.position)
	blTextureIndex := g.texIndices[3]
	blSrc, blDst := g.texture.GetSourceAndDestRects(int(blTextureIndex), blPos, mgl32.Vec2{0, 1}, mgl32.Vec2{1, 1})
	batch.DrawTextured(blSrc, blDst, g.borderColor)

	// bottom-right corner
	brPos := mgl32.Vec2{float32(g.size.X()) + float32(g.paddingHoriz), float32(g.size.Y()) + float32(g.paddingVert)}.Add(g.position)
	brTextureIndex := g.texIndices[2]
	brSrc, brDst := g.texture.GetSourceAndDestRects(int(brTextureIndex), brPos, mgl32.Vec2{1, 1}, mgl32.Vec2{1, 1})
	batch.DrawTextured(brSrc, brDst, g.borderColor)

	// top border (stretch)
	borderWidth := g.size.X() + float32(g.paddingHoriz)*2

	topPos := tlPos
	topTextureIndex := g.texIndices[4]
	topSrc := g.texture.GetSourceRect(int(topTextureIndex))
	topHeight := topSrc[3] - topSrc[1]
	topDst := mgl32.Vec4{topPos.X(), topPos.Y(), topPos.X() + borderWidth, topPos.Y() + topHeight}
	batch.DrawTextured(topSrc, topDst, g.borderColor)

	// bottom border (stretch)
	bottomTextureIndex := g.texIndices[5]
	botSrc := g.texture.GetSourceRect(int(bottomTextureIndex))
	botDst := mgl32.Vec4{blDst.X(), blDst.Y(), blDst.X() + borderWidth, blDst.W()}
	batch.DrawTextured(botSrc, botDst, g.borderColor)
}

func (g *GuiBorder) ContainsPoint(point mgl32.Vec2) bool {
	return point.X() >= g.position.X() && point.X() <= g.position.X()+g.size.X() && point.Y() >= g.position.Y() && point.Y() <= g.position.Y()+g.size.Y()
}

func (g *GuiBorder) SetSize(size mgl32.Vec2) {
	g.size = size
}

func (g *GuiBorder) Width() int {
	return int(g.size.X() + float32(g.paddingHoriz)*2)
}

func (g *GuiBorder) Height() int {
	return int(g.size.Y() + float32(g.paddingVert)*2)
}
