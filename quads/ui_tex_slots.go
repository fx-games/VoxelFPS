package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/glfw"
	"github.com/memmaker/voxelfps/quads/glhf"
	"image"
)

/*
Start by dropping the frontal pic.
Choose if you want directions? -> Add directions
Choose if you want to add animation frames? -> Add animation frames
Choose if you want to add additional animations? -> Add additional animations

Buttons:
 - Done
 - Add Directions
 - Rotate ViewDirection: On/Off
 - Play Animation: On/Off

*/

type GuiTextureSlots struct {
	background *GuiBorder

	assignedIndices []int
	slotSize        int
	slotPositions   []mgl32.Vec2
	labels          []*BitmapFontMesh
	shouldClose     bool
	closeCallback   func()

	screenWidth  int
	screenHeight int
	atlasTexture glhf.TextureAtlas
	hoveredIndex int
	labelTexts   []string
	uiContext    UIContext
}

func (g *GuiTextureSlots) HandleDrop(names []string, x float64, y float64) bool {
	if len(names) != 1 {
		return false
	}

	return false
}

func pixelsFromImage(image image.Image) []uint8 {
	frameWidth := image.Bounds().Dx()
	frameHeight := image.Bounds().Dy()
	pixels := make([]uint8, frameWidth*frameHeight*4)
	for y := 0; y < frameHeight; y++ {
		for x := 0; x < frameWidth; x++ {
			r, g, b, a := image.At(x, y).RGBA()

			pixels[(y*frameWidth+x)*4] = uint8((float64(r) / 0xffff) * 0xff)
			pixels[(y*frameWidth+x)*4+1] = uint8((float64(g) / 0xffff) * 0xff)
			pixels[(y*frameWidth+x)*4+2] = uint8((float64(b) / 0xffff) * 0xff)
			pixels[(y*frameWidth+x)*4+3] = uint8((float64(a) / 0xffff) * 0xff)
		}
	}

	return pixels
}

func (g *GuiTextureSlots) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
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

func (g *GuiTextureSlots) HandleMousePosEvents(xpos float64, ypos float64) (consumed bool) {
	g.hoveredIndex = -1
	for slotIndex, slotPos := range g.slotPositions {
		slotRect := mgl32.Vec4{slotPos.X(), slotPos.Y(), slotPos.X() + float32(g.slotSize), slotPos.Y() + float32(g.slotSize)}
		if float32(xpos) >= slotRect[0] && float32(xpos) <= slotRect[2] && float32(ypos) >= slotRect[1] && float32(ypos) <= slotRect[3] {
			g.hoveredIndex = slotIndex
			return true
		}
	}
	return false
}

func (g *GuiTextureSlots) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool) {
	return false
}
func (g *GuiTextureSlots) SetSlotAt(xpos float64, ypos float64, texIndex int) bool {
	slotIndex, overSlot := g.slotAt(xpos, ypos)
	if !overSlot {
		return false
	}
	g.assignedIndices[slotIndex] = texIndex
	return true
}

// IsSlotAt answers whether a drop here would land on a side, without committing to
// one. A picked texture costs the atlas a slot, so the caller has to be able to ask
// before it pays for it.
func (g *GuiTextureSlots) IsSlotAt(xpos float64, ypos float64) bool {
	_, overSlot := g.slotAt(xpos, ypos)
	return overSlot
}

func (g *GuiTextureSlots) slotAt(xpos float64, ypos float64) (int, bool) {
	for slotIndex, slotPos := range g.slotPositions {
		slotRect := mgl32.Vec4{slotPos.X(), slotPos.Y(), slotPos.X() + float32(g.slotSize), slotPos.Y() + float32(g.slotSize)}
		if float32(xpos) >= slotRect[0] && float32(xpos) <= slotRect[2] && float32(ypos) >= slotRect[1] && float32(ypos) <= slotRect[3] {
			return slotIndex, true
		}
	}
	return 0, false
}

func (g *GuiTextureSlots) HandleScrollEvents(xoff float64, yoff float64) (consumed bool) {
	return false
}

func (g *GuiTextureSlots) ShouldClose() bool {
	return g.shouldClose
}

func (g *GuiTextureSlots) Close() {
	g.shouldClose = true
	if g.closeCallback != nil {
		g.closeCallback()
	}
}

func NewGuiTextureSlots(uiContext UIContext, atlasForDisplay glhf.TextureAtlas, labels []string) *GuiTextureSlots {
	return &GuiTextureSlots{
		shouldClose:     false,
		uiContext:       uiContext,
		atlasTexture:    atlasForDisplay,
		assignedIndices: make([]int, len(labels)),
		labelTexts:      labels,
	}
}
func (g *GuiTextureSlots) SetCentered(screenWidth int, screenHeight int) {

	g.screenWidth = screenWidth
	g.screenHeight = screenHeight
	g.slotPositions = nil
	g.labels = nil

	backgWidth := 0.9 * float32(screenWidth)
	backgHeight := 0.3 * float32(screenHeight)

	g.background = NewGuiBorderWithBounds(g.uiContext, mgl32.Vec2{float32(screenWidth) * 0.05, float32(screenHeight) * 0.05}, mgl32.Vec2{backgWidth, backgHeight})
	g.background.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.8})

	topLeftX := float32(screenWidth) * 0.1
	topLeftY := float32(screenHeight) * 0.1

	tileWidth := int(float32(screenWidth) * 0.8 * (1.0 / float32(len(g.labelTexts))))
	tileHeight := tileWidth
	drawY := int(topLeftY)

	g.slotSize = tileWidth
	for i := 0; i < len(g.labelTexts); i++ {
		drawX := int(topLeftX) + i*(tileWidth)
		slotPos := mgl32.Vec2{float32(drawX), float32(drawY)}
		g.slotPositions = append(g.slotPositions, slotPos)

		label := NewBitmapFontMesh(g.uiContext.Shader, g.uiContext.Font, g.uiContext.ColorBatch)
		label.SetTintColor(mgl32.Vec4{1, 1, 1, 1})
		label.SetWidth(tileWidth)
		label.SetCenterOrigin(true)
		label.SetText(g.labelTexts[i])
		_, labelH := label.Size()
		label.SetPosition(slotPos.Add(mgl32.Vec2{float32(tileWidth) * 0.5, float32(tileHeight) + (float32(labelH) * 0.5)}))

		g.labels = append(g.labels, label)
	}

}

func (g *GuiTextureSlots) Draw() {
	g.background.Draw()

	for slotIndex, slotPos := range g.slotPositions {
		texIndex := g.assignedIndices[slotIndex]
		drawCol := mgl32.Vec4{1, 1, 1, 1}
		if g.hoveredIndex == slotIndex {
			drawCol = mgl32.Vec4{1, 1, 0.5, 1}
		}
		dstRect := mgl32.Vec4{slotPos.X(), slotPos.Y(), slotPos.X() + float32(g.slotSize), slotPos.Y() + float32(g.slotSize)}

		if texIndex >= 0 { // valid
			srcRect := g.atlasTexture.GetSourceRect(texIndex)
			g.atlasTexture.GetSpriteBatch().DrawTextured(srcRect, dstRect, drawCol)
		} else { // just draw the empty slot
			g.uiContext.ColorBatch.DrawColoredOutline(dstRect, 1, drawCol)
		}
	}

	for _, label := range g.labels {
		label.Draw()
	}
}

func (g *GuiTextureSlots) SetOnClose(closeCallback func()) {
	g.closeCallback = closeCallback
}

func (g *GuiTextureSlots) GetSlotIndex(i int) uint16 {
	return uint16(g.assignedIndices[i])
}

func (g *GuiTextureSlots) SetSlot(index int, tex uint16) {
	g.assignedIndices[index] = int(tex)
}
