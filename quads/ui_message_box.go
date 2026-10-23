package quads

import (
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads/glhf"
)

type TextAlign int

const (
	AlignLeft TextAlign = iota
	AlignCenter
)

// messageBoxScreenMargin keeps the box off the very edge of the screen when
// picking a wrap width and when clamping an oversized image or word.
// messageBoxImageGap is the gap between the image and the text below it.
const (
	messageBoxScreenMargin = 40
	messageBoxImageGap     = 8
)

// GuiMessageBox is a bordered box showing an optional image above wrapped
// text, sized to its content and centered on screen. Load the image atlas
// with GlApplication.LoadFixedGuiAtlasWithBatch (one tile, the whole image).
type GuiMessageBox struct {
	uiContext UIContext
	label     *BitmapFontMesh
	border    *GuiBorder
	image     *glhf.TextureAtlasFixed
	imagePos  mgl32.Vec2

	text     string
	align    TextAlign
	isHidden bool
}

func NewGuiMessageBox(uiContext UIContext) *GuiMessageBox {
	label := NewBitmapFontMesh(uiContext.Shader, uiContext.Font, uiContext.ColorBatch)
	label.NoBackground()
	border := NewGuiBorder(uiContext)
	border.SetBackground(mgl32.Vec4{0, 0, 0, 0.8})
	return &GuiMessageBox{
		uiContext: uiContext,
		label:     label,
		border:    border,
		align:     AlignLeft,
		isHidden:  true,
	}
}

func (m *GuiMessageBox) SetAlign(align TextAlign) {
	m.align = align
}

// SetImage sets the image drawn centered above the text, or nil for none.
func (m *GuiMessageBox) SetImage(image *glhf.TextureAtlasFixed) {
	m.image = image
}

// Show wraps and lays out text (and the image, if set), then centers the box
// on a screenWidth x screenHeight screen - GuiSize() coordinates.
func (m *GuiMessageBox) Show(text string, screenWidth, screenHeight int) {
	m.text = text
	m.isHidden = false
	m.layout(screenWidth, screenHeight)
}

func (m *GuiMessageBox) Hide() {
	m.isHidden = true
}

func (m *GuiMessageBox) IsVisible() bool {
	return !m.isHidden
}

func (m *GuiMessageBox) layout(screenWidth, screenHeight int) {
	charWidth, _ := m.uiContext.Font.Texture.GetAtlasItemSize()
	maxChars := max(1, (screenWidth-messageBoxScreenMargin)/(charWidth+1))

	lines := wrapText(m.text, maxChars)
	if m.align == AlignCenter {
		lines = centerLines(lines)
	}
	m.label.SetMultilineText(lines)
	textW, textH := m.label.Size()

	imgW, imgH, gap := 0, 0, 0
	if m.image != nil {
		imgW, imgH = m.image.GetAtlasItemSize()
		gap = messageBoxImageGap
	}

	contentW := min(max(textW, imgW), screenWidth-messageBoxScreenMargin)
	contentH := min(imgH+gap+textH, screenHeight-messageBoxScreenMargin)

	topLeft := mgl32.Vec2{float32(screenWidth-contentW) / 2, float32(screenHeight-contentH) / 2}
	m.border.SetPosition(topLeft)
	m.border.SetSize(mgl32.Vec2{float32(contentW), float32(contentH)})

	m.label.SetPosition(topLeft.Add(mgl32.Vec2{float32(contentW-textW) / 2, float32(imgH + gap)}))
	m.imagePos = topLeft.Add(mgl32.Vec2{float32(contentW-imgW) / 2, 0})
}

func (m *GuiMessageBox) Draw() {
	if m.isHidden {
		return
	}
	m.border.Draw()
	if m.image != nil {
		imgW, imgH := m.image.GetAtlasItemSize()
		dst := mgl32.Vec4{m.imagePos.X(), m.imagePos.Y(), m.imagePos.X() + float32(imgW), m.imagePos.Y() + float32(imgH)}
		m.image.GetSpriteBatch().DrawTextured(m.image.GetSourceRectWholeTexture(), dst, mgl32.Vec4{1, 1, 1, 1})
	}
	m.label.Draw()
}

// wrapText breaks text into lines of at most maxChars, breaking only on
// whitespace between words. "\n" in text forces a paragraph break.
func wrapText(text string, maxChars int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) <= maxChars {
				line += " " + word
			} else {
				lines = append(lines, line)
				line = word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// centerLines pads each line with leading spaces to center it within the
// longest line - the bitmap font is monospace, so a leading space advances
// exactly as far as any other character (see BitmapFontMesh.Draw).
func centerLines(lines []string) []string {
	maxLen := 0
	for _, line := range lines {
		maxLen = max(maxLen, len(line))
	}
	centered := make([]string, len(lines))
	for i, line := range lines {
		centered[i] = strings.Repeat(" ", (maxLen-len(line))/2) + line
	}
	return centered
}
