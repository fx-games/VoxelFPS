package quads

import (
    "bytes"
    "encoding/binary"
    "encoding/hex"
    "fmt"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "image"
    "image/color"
    "io"
    "math"
    "os"
    "path"
    "strings"
)

type BitmapFontMesh struct {
    backgroundVertices      *glhf.VertexSlice[glhf.GlFloat]
    vertices                *glhf.VertexSlice[glhf.GlFloat]
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
}

func NewBitmapFontMesh(shader *glhf.Shader, font BitmapFont) *BitmapFontMesh {
    b := &BitmapFontMesh{
        shader:                  shader,
        font:                    font,
        characterToTextureIndex: font.Index.GetMapper(),
        scale:                   mgl32.Vec2{1, 1},
        paddingBetweenLines:     2,
        tintColor:               mgl32.Vec4{1, 1, 1, 1},
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
    paddingBetweenCharacters := float32(t.paddingBetweenCharacters)
    paddingBetweenLines := float32(t.paddingBetweenLines)

    curDrawPos := mgl32.Vec2{0, 0}
    cW, cH := t.font.Texture.GetAtlasItemSize()
    charWidth := float32(cW) * t.scale.X()
    charHeight := float32(cH) * t.scale.Y()
    gWidth := glhf.GlFloat(charWidth)
    gHeight := glhf.GlFloat(charHeight)

    tWidth, tHeight := t.CalculateSizeScaled(text)

    backgroundDrawPos := curDrawPos
    if t.originIsCenter {
        curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(-tWidth / 2), float32(-tHeight / 2)})
        backgroundDrawPos = backgroundDrawPos.Add(mgl32.Vec2{float32(-t.fieldWidth / 2), float32(-tHeight / 2)})
    }

    backgroundWidth := max(tWidth, t.fieldWidth)
    var backgroundVertices []glhf.GlFloat
    backgroundVertices = AppendGUIQuadVertices(backgroundVertices, t.font.Texture, 0, backgroundDrawPos, glhf.GlFloat(backgroundWidth), glhf.GlFloat(tHeight))
    t.setBackgroundVertices(backgroundVertices)

    var rawVertices []glhf.GlFloat
    for _, line := range text {
        for _, character := range line {
            if character == ' ' {
                curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(charWidth + paddingBetweenCharacters), 0})
                continue
            }
            textureIndex := t.characterToTextureIndex(character)

            rawVertices = AppendGUIQuadVertices(rawVertices, t.font.Texture, textureIndex, curDrawPos, gWidth, gHeight)

            curDrawPos = curDrawPos.Add(mgl32.Vec2{float32(charWidth + paddingBetweenCharacters)})
        }

        curDrawPos = mgl32.Vec2{0, curDrawPos.Y() + float32(charHeight+paddingBetweenLines)}
    }
    t.setVertices(rawVertices)
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
    if t.vertices == nil || t.isHidden {
        return
    }

    t.shader.SetUniformAttr(GuiShaderPosition, t.currentPos) // position offset

    if t.renderBackground && t.backgroundVertices != nil {
        t.shader.SetUniformAttr(GuiShaderTintColor, t.backgroundColor)
        t.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{-1, -1})
        t.backgroundVertices.Begin()
        t.backgroundVertices.Draw()
        t.backgroundVertices.End()
        t.shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    }

    if t.useTintColor {
        t.shader.SetUniformAttr(GuiShaderTintColor, t.tintColor) // tint
    } else {
        t.shader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    }
    /*
       if t.useDiscardColor {
           t.shader.SetUniformAttr(GuiShaderDiscardedColor, t.discardColor.Vec4(1.0)) // discard
       } else {
           t.shader.SetUniformAttr(GuiShaderDiscardedColor, mgl32.Vec4{0, 0, 0, 0})
       }
    */
    t.font.Texture.Begin()

    t.vertices.Begin()
    t.vertices.Draw()
    t.vertices.End()

    t.font.Texture.End()
}

func (t *BitmapFontMesh) setVertices(rawVertices []glhf.GlFloat) {
    vertices := glhf.MakeVertexSlice(t.shader, len(rawVertices)/4, len(rawVertices)/4)
    vertices.Begin()
    vertices.SetVertexData(rawVertices)
    vertices.End()
    t.vertices = vertices
}

func (t *BitmapFontMesh) setBackgroundVertices(rawVertices []glhf.GlFloat) {
    vertices := glhf.MakeVertexSlice(t.shader, len(rawVertices)/4, len(rawVertices)/4)
    vertices.Begin()
    vertices.SetVertexData(rawVertices)
    vertices.End()
    t.backgroundVertices = vertices
}

func (t *BitmapFontMesh) Clear() {
    t.vertices = nil
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
    if t.vertices == nil {
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
func NewBitmapFontIndexFromFile(filename string) BitmapFontIndex {
    index := map[rune]uint16{}
    file, err := os.Open(filename)
    if err != nil {
        println("could not open atlas index file")
        return index
    }
    defer file.Close()
    return NewBitmapFontIndexFromReader(file)
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

func CreateAtlasFromPBMs(directory string, glyphWidth, glyphHeight int) (*glhf.Texture, BitmapFontIndex) {
    // 967 files
    // 8x14 pixels

    // 8*16 = 128
    // 14*16 = 224
    indices := map[rune]uint16{}
    textureIndex := uint16(0)
    entries, readError := os.ReadDir(directory)
    if readError != nil {
        println(fmt.Sprintf("[Atlas] Error reading directory %s", directory))
        return nil, nil
    }
    fileCount := len(entries)
    squareCount := math.Ceil(math.Sqrt(float64(fileCount)))

    padding := int(0)

    atlasWidth := int(math.Ceil(squareCount * float64(glyphWidth+padding)))
    atlasHeight := int(math.Ceil(squareCount * float64(glyphHeight+padding)))

    if atlasWidth%4 != 0 {
        atlasWidth += 4 - (atlasWidth % 4)
    }

    if atlasHeight%4 != 0 {
        atlasHeight += 4 - (atlasHeight % 4)
    }

    pixels := image.NewNRGBA(image.Rect(0, 0, atlasWidth, atlasHeight)) // iterate over the files in the directory
    var drawColor color.Color
    for _, dirEntry := range entries {
        extension := strings.ToLower(path.Ext(dirEntry.Name()))
        if extension != ".pbm" {
            continue
        }
        nameWithoutExtension := strings.TrimSuffix(dirEntry.Name(), extension)
        hexString := nameWithoutExtension

        glyph := runeFromHexString(hexString)

        texturePath := path.Join(directory, dirEntry.Name())
        file, err := os.Open(texturePath)
        if err != nil {
            println(fmt.Sprintf("[Atlas] Error loading %s from %s", dirEntry, texturePath))
            continue
        }
        img, _, err := image.Decode(file)
        if err != nil {
            continue
        }
        file.Close()
        if glyphWidth != img.Bounds().Dx() {
            println(fmt.Sprintf("[Atlas] Error loading %s from %s: width mismatch", dirEntry, texturePath))
            continue
        }
        if glyphHeight != img.Bounds().Dy() {
            println(fmt.Sprintf("[Atlas] Error loading %s from %s: height mismatch", dirEntry, texturePath))
            continue
        }

        texturesPerRow := uint16(atlasWidth / glyphWidth) // 256 / 16 = 16
        // copy the image into the atlas
        tilePosX := textureIndex % texturesPerRow
        tilePosY := textureIndex / texturesPerRow
        offsetX := tilePosX * (uint16(glyphWidth) + uint16(padding))
        offsetY := tilePosY * (uint16(glyphHeight) + uint16(padding))
        for x := 0; x < glyphWidth; x++ {
            for y := 0; y < glyphHeight; y++ {
                r, _, _, _ := img.At(x, y).RGBA()

                drawColor = color.Transparent
                if r < 0x8000 { // black
                    drawColor = color.White
                }
                pixels.Set(int(offsetX)+x, int(offsetY)+y, drawColor)
            }
        }
        indices[glyph] = textureIndex
        println(fmt.Sprintf("[Atlas] %U %d -> %s", glyph, textureIndex, dirEntry))
        textureIndex++
    }
    /*
    	// debug write the atlas to a file
    	file, err := os.Create(path.Join(directory, "debug_atlas.png"))
    	if err != nil {
    		println("could not create debug_atlas.png")
    	}
    	err = png.Encode(file, pixels)
    	if err != nil {
    		println("could not encode debug_atlas.png")
    	}
    	file.Close()
    */

    texture := glhf.NewTexture(atlasWidth, atlasHeight, false, pixels.Pix)
    texture.SetAtlasItemSize(glyphWidth, glyphHeight)
    return texture, indices
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
