package quads

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
	"io"
	"os"
	"strings"
)

type BitmapFontMesh struct {
	currentPos              mgl32.Vec2
	shader                  *glhf.Shader
	font                    BitmapFont
	characterToTextureIndex func(character rune) uint16
	scale                   mgl32.Vec2
	discardColor            mgl32.Vec3
	useDiscardColor         bool
	tintColor               mgl32.Vec4
	backgroundColor         mgl32.Vec4
	useTintColor            bool

	paddingBetweenCharacters int
	paddingBetweenLines      int
	originIsCenter           bool
	isHidden                 bool
	lastText                 []string
	renderBackground         bool
	fieldWidth               int
	colorBatch               *glhf.SpriteBatch
	backgroundRect           mgl32.Vec4
	layer                    float32
	tWidth                   int
	tHeight                  int
}

func NewBitmapFontMesh(shader *glhf.Shader, font BitmapFont, colorBatch *glhf.SpriteBatch) *BitmapFontMesh {
	b := &BitmapFontMesh{
		shader:                  shader,
		font:                    font,
		characterToTextureIndex: font.Index.GetMapper(),
		scale:                   mgl32.Vec2{1, 1},
		paddingBetweenLines:     2,
		tintColor:               mgl32.Vec4{1, 1, 1, 1},
		colorBatch:              colorBatch,
		layer:                   0.2,
	}
	return b
}
func (t *BitmapFontMesh) SetBackground(color mgl32.Vec4) {
	t.backgroundColor = color
	t.renderBackground = true
}

func (t *BitmapFontMesh) SetDiscardColor(color mgl32.Vec3) {
	t.discardColor = color
	t.useDiscardColor = true
}

func (t *BitmapFontMesh) SetCenterOrigin(center bool) {
	t.originIsCenter = center
}

func (t *BitmapFontMesh) SetTintColor(color mgl32.Vec4) {
	t.tintColor = color
	t.useTintColor = true
}
func (t *BitmapFontMesh) SetAtlasFontMapper(mapper func(character rune) uint16) {
	t.characterToTextureIndex = mapper
}
func (t *BitmapFontMesh) SetScale(scale mgl32.Vec2) {
	t.scale = scale
}

// SetText sets the text to be displayed by the BitmapFontMesh.
// NOTE: Will trigger a recalculation of the mesh vertices and their upload to the GPU.
func (t *BitmapFontMesh) SetText(text string) {
	t.SetMultilineText([]string{text})
}

func getDimensions(text []string) (int, int) {
	maxChars := 0
	for _, line := range text {
		if len(line) > maxChars {
			maxChars = len(line)
		}
	}
	return maxChars, len(text)
}

func (t *BitmapFontMesh) UpdateVertices() {
	t.SetMultilineText(t.lastText)
}

// SetMultilineText sets the text to be displayed by the BitmapFontMesh.
// NOTE: Will trigger a recalculation of the mesh vertices and their upload to the GPU.
func (t *BitmapFontMesh) SetMultilineText(text []string) {
	if t.font.Texture == nil {
		println("BitmapFontMesh: texture is nil")
		return
	}
	if t.characterToTextureIndex == nil {
		println("BitmapFontMesh: characterToTextureIndex is nil")
		return
	}
	if isSameText(text, t.lastText) {
		return
	}
	t.lastText = text
	t.isHidden = false

	tWidth, tHeight := t.CalculateSizeScaled(text)
	t.tWidth = tWidth
	t.tHeight = tHeight

	backgroundDrawPos := mgl32.Vec2{0, 0}
	if t.originIsCenter {
		backgroundDrawPos = backgroundDrawPos.Add(mgl32.Vec2{float32(-t.fieldWidth / 2), float32(-tHeight / 2)})
	}

	backgroundWidth := max(tWidth, t.fieldWidth)
	t.backgroundRect = mgl32.Vec4{backgroundDrawPos.X(), backgroundDrawPos.Y(), backgroundDrawPos.X() + float32(backgroundWidth), backgroundDrawPos.Y() + float32(tHeight)}
}

func (t *BitmapFontMesh) getBackgroundRect() mgl32.Vec4 {
	return t.backgroundRect.Add(mgl32.Vec4{t.currentPos.X(), t.currentPos.Y(), t.currentPos.X(), t.currentPos.Y()})
}

func isSameText(text []string, text2 []string) bool {
	if len(text) != len(text2) {
		return false
	}
	for i, line := range text {
		if line != text2[i] {
			return false
		}
	}
	return true
}
func (t *BitmapFontMesh) Size() (int, int) {
	return t.CalculateSizeScaled(t.lastText)
}

func (t *BitmapFontMesh) SizeAsVec2() mgl32.Vec2 {
	width, height := t.Size()
	return mgl32.Vec2{float32(width), float32(height)}
}

func (t *BitmapFontMesh) CalculateSizeScaled(text []string) (int, int) {
	return t.font.MeasureString(text, t.scale, float32(t.paddingBetweenCharacters), float32(t.paddingBetweenLines))
}
func (t *BitmapFontMesh) Draw() {
	if t.isHidden {
		return
	}

	if t.renderBackground {
		t.colorBatch.DrawColoredRect(t.getBackgroundRect(), t.backgroundColor)
	}

	fontBatch := t.font.Texture.GetSpriteBatch()
	curDrawPos := t.currentPos

	if t.originIsCenter {
		curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(-t.tWidth / 2), float32(-t.tHeight / 2)})
	}
	paddingBetweenCharacters := float32(t.paddingBetweenCharacters)
	paddingBetweenLines := float32(t.paddingBetweenLines)

	cW, cH := t.font.Texture.GetAtlasItemSize()
	charWidth := float32(cW) * t.scale.X()
	charHeight := float32(cH) * t.scale.Y()

	// Where every line starts. A second line went back to x=0 rather than to
	// the mesh's own left edge, which put every line after the first hard
	// against the left of the screen - invisible for as long as the only
	// multi-line text was the debug overlay in that corner, and unmissable the
	// first time a panel somewhere else showed four lines.
	lineStart := curDrawPos.X()
	for _, line := range t.lastText {
		for _, character := range line {
			if character == ' ' {
				curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(charWidth + paddingBetweenCharacters), 0})
				continue
			}
			textureIndex := t.characterToTextureIndex(character)

			//rawVertices = AppendGUIQuadVertices(rawVertices, t.font.Texture, int(textureIndex), t.layer, curDrawPos, gWidth, gHeight, t.tintColor)
			srcRect := t.font.Texture.GetSourceRect(int(textureIndex))
			destRect := mgl32.Vec4{curDrawPos.X(), curDrawPos.Y(), curDrawPos.X() + float32(charWidth), curDrawPos.Y() + float32(charHeight)}
			fontBatch.DrawTextured(srcRect, destRect, t.tintColor)

			curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(charWidth + paddingBetweenCharacters)})
		}

		curDrawPos = mgl32.Vec2{lineStart, curDrawPos.Y() + float32(charHeight+paddingBetweenLines)}
	}
}

func (t *BitmapFontMesh) Clear() {
	t.lastText = []string{}
}

func (t *BitmapFontMesh) SetPosition(pos mgl32.Vec2) {
	t.currentPos = pos
}

func (t *BitmapFontMesh) Hide() {
	t.isHidden = true
}

func (t *BitmapFontMesh) GetText() string {
	return strings.Join(t.lastText, "\n")
}

func (t *BitmapFontMesh) Show() {
	t.isHidden = false
}

func (t *BitmapFontMesh) NoBackground() {
	t.renderBackground = false
}

func (t *BitmapFontMesh) SetWidth(width int) {
	t.fieldWidth = width
}

func (t *BitmapFontMesh) GetRect() (pos, size mgl32.Vec2) {
	if len(t.lastText) == 0 {
		return mgl32.Vec2{0, 0}, mgl32.Vec2{0, 0}
	}
	width, height := t.Size()
	return t.currentPos, mgl32.Vec2{float32(width), float32(height)}
}

func (t *BitmapFontMesh) GetPosition() mgl32.Vec2 {
	return t.currentPos

}

func (t *BitmapFontMesh) GetTintColor() mgl32.Vec4 {
	return t.tintColor
}

func (t *BitmapFontMesh) GetAlpha() float32 {
	return t.tintColor.W()
}

func (t *BitmapFontMesh) SetAlpha(alpha float32) {
	t.tintColor = mgl32.Vec4{t.tintColor.X(), t.tintColor.Y(), t.tintColor.Z(), alpha}
	t.backgroundColor = mgl32.Vec4{t.backgroundColor.X(), t.backgroundColor.Y(), t.backgroundColor.Z(), alpha}
}

func (t *BitmapFontMesh) ContainsPoint(point mgl32.Vec2) bool {
	rectPos, rectSize := t.GetRect()
	return point.X() >= rectPos.X() && point.X() <= rectPos.X()+rectSize.X() && point.Y() >= rectPos.Y() && point.Y() <= rectPos.Y()+rectSize.Y()
}

type BitmapFontIndex map[rune]uint16

func (i BitmapFontIndex) WriteAtlasIndex(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		println("could not create atlas index file")
		return
	}
	defer file.Close()
	for k, v := range i {
		_, writeErr := file.WriteString(fmt.Sprintf("%d:%d\n", k, v))
		if writeErr != nil {
			println("could not write to atlas index file")
			return
		}
	}
}

func (i BitmapFontIndex) GetMapper() func(character rune) uint16 {
	return func(character rune) uint16 {
		return i[character]
	}
}
func NewBitmapFontIndexFromReader(file io.ReadCloser) BitmapFontIndex {
	index := map[rune]uint16{}
	var k rune
	var v uint16
	for {
		_, scanError := fmt.Fscanf(file, "%d:%d\n", &k, &v)
		if scanError != nil {
			if scanError.Error() != "EOF" {
				panic("could not scan atlas index file")
			}
			break
		}
		index[k] = v
	}
	return index
}

type AtlasDescription struct {
	PositionOfCapitalA     [2]int
	PositionOfZero         *[2]int
	PositionOfSpecialChain *[2]int
	//Rows                  int
	Cols                  int
	SpecialCharacterChain []rune
}

func NewIndexFromDescription(desc AtlasDescription) BitmapFontIndex {
	result := map[rune]uint16{}

	//rows := desc.Rows
	cols := desc.Cols

	positionOfCapitalA := desc.PositionOfCapitalA
	indexOfCapitalA := positionOfCapitalA[0] + positionOfCapitalA[1]*cols

	indexOfZero := indexOfCapitalA + 26
	if desc.PositionOfZero != nil {
		positionOfZero := *desc.PositionOfZero
		indexOfZero = positionOfZero[0] + positionOfZero[1]*cols
	}
	indexOfSpecialChain := indexOfZero + 10
	if desc.PositionOfSpecialChain != nil {
		positionOfSpecialChain := *desc.PositionOfSpecialChain
		indexOfSpecialChain = positionOfSpecialChain[0] + positionOfSpecialChain[1]*cols
	}

	for i := 0; i < 26; i++ {
		result[rune(i+65)] = uint16(indexOfCapitalA + i)
	}

	for i := 0; i < 10; i++ {
		result[rune(i+48)] = uint16(indexOfZero + i)
	}

	for i, special := range desc.SpecialCharacterChain {
		result[special] = uint16(indexOfSpecialChain + i)
	}

	return result
}

func runeFromHexString(hexString string) rune {
	if len(hexString) < 8 {
		for i := len(hexString); i < 8; i++ {
			hexString = "0" + hexString
		}
	}
	codePointAsBytes, _ := hex.DecodeString(hexString)
	var glyph rune
	reader := bytes.NewReader(codePointAsBytes)
	readError := binary.Read(reader, binary.BigEndian, &glyph)
	if readError != nil {
		println(fmt.Sprintf("[runeFromHexString] Error reading %s", hexString))
	}
	return glyph
}
