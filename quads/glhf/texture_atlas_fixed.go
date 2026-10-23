package glhf

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxellib/tpack"
	"image"
	"strconv"
)

type TextureAtlasFixed struct {
	*Texture

	itemSizeX    int // textureWidth of each item in the atlas in pixels
	itemSizeY    int // textureHeight of each item in the atlas in pixels
	itemPaddingX int
	itemPaddingY int
	itemMarginX  int
	itemMarginY  int

	sourceFile string

	batch      *SpriteBatch
	shader     *Shader
	maxSprites int
}

func (t *TextureAtlasFixed) SetGuiShader(shader *Shader) {
	t.shader = shader
	if t.batch != nil {
		t.tryInitBatch()
	}
}
func (t *TextureAtlasFixed) SetMaxSprites(maxSprites int) {
	t.maxSprites = maxSprites
	if t.batch != nil {
		t.tryInitBatch()
	}
}
func (t *TextureAtlasFixed) GetSpriteBatch() *SpriteBatch {
	if t.batch == nil {
		t.tryInitBatch()
	}
	return t.batch
}

func (t *TextureAtlasFixed) tryInitBatch() {
	if t.shader == nil || t.maxSprites == 0 {
		return
	}
	t.batch = NewSpriteBatch(t.shader, t, t.maxSprites)
}

func (t *TextureAtlasFixed) GetSourceRectWholeTexture() mgl32.Vec4 {
	rect := mgl32.Vec4{0, 0, float32(t.textureWidth), float32(t.textureHeight)}
	return rect
}

// Rects are in the format minX, minY, maxX, maxY in pixel coordinates.
func (t *TextureAtlasFixed) GetSourceAndDestRects(index int, drawPosInPixels mgl32.Vec2, normalizedOrigin mgl32.Vec2, scale mgl32.Vec2) (mgl32.Vec4, mgl32.Vec4) {
	sminU, sminV, smaxU, smaxV := t.GetUVInPx(index)
	sourceRect := mgl32.Vec4{float32(sminU), float32(sminV), float32(smaxU), float32(smaxV)}

	width := float32(t.itemSizeX) * scale.X()
	height := float32(t.itemSizeY) * scale.Y()
	originInPixels := mgl32.Vec2{width * normalizedOrigin.X(), height * normalizedOrigin.Y()}
	topleftDrawPos := drawPosInPixels.Sub(originInPixels)
	destRect := mgl32.Vec4{topleftDrawPos.X(), topleftDrawPos.Y(), topleftDrawPos.X() + width, topleftDrawPos.Y() + height}

	return sourceRect, destRect
}

func (t *TextureAtlasFixed) GetSourceRect(index int) mgl32.Vec4 {
	sminU, sminV, smaxU, smaxV := t.GetUVInPx(index)
	rect := mgl32.Vec4{float32(sminU), float32(sminV), float32(smaxU), float32(smaxV)}
	return rect
}

func (t *TextureAtlasFixed) NameOfFrame(i int) string {
	return strconv.Itoa(i)
}

func NewTextureAtlasFixed(texture *Texture, itemSizeX, itemSizeY int, sourceFile string) *TextureAtlasFixed {
	return &TextureAtlasFixed{
		Texture:      texture,
		itemSizeX:    itemSizeX,
		itemSizeY:    itemSizeY,
		sourceFile:   sourceFile,
		itemPaddingX: 0,
		itemPaddingY: 0,
		itemMarginX:  0,
		itemMarginY:  0,
	}
}
func (t *TextureAtlasFixed) SourceFile() string {
	return t.sourceFile
}

func (t *TextureAtlasFixed) CanAppend(image image.Image) bool {
	return image.Bounds().Dx() == t.itemSizeX && image.Bounds().Dy() == t.itemSizeY
}

func (t *TextureAtlasFixed) GetUV(index int) (GlFloat, GlFloat, GlFloat, GlFloat) {
	tileWidthInPx := t.itemSizeX
	tileHeightInPx := t.itemSizeY

	xCount := t.columns()
	if xCount == 0 {
		return 0, 0, 1, 1 // the whole texture: a placeholder smaller than one tile
	}

	leftBorder := t.itemMarginX + ((int(index) % xCount) * (tileWidthInPx + t.itemPaddingX))
	topBorder := t.itemMarginY + ((int(index) / xCount) * (tileHeightInPx + t.itemPaddingY))

	topLeftU := GlFloat(leftBorder) / GlFloat(t.textureWidth)
	topLeftV := GlFloat(topBorder) / GlFloat(t.textureHeight)

	bottomRightU := GlFloat(leftBorder+tileWidthInPx) / GlFloat(t.textureWidth)
	bottomRightV := (GlFloat(topBorder+tileHeightInPx) / GlFloat(t.textureHeight)) - 0.0001

	return topLeftU, topLeftV, bottomRightU, bottomRightV
}
func (t *TextureAtlasFixed) GetUVInPx(index int) (int, int, int, int) {
	tileWidthInPx := t.itemSizeX
	tileHeightInPx := t.itemSizeY

	xCount := t.columns()
	if xCount == 0 {
		return 0, 0, t.textureWidth, t.textureHeight
	}

	leftBorder := t.itemMarginX + ((index % xCount) * (tileWidthInPx + t.itemPaddingX))
	topBorder := t.itemMarginY + ((index / xCount) * (tileHeightInPx + t.itemPaddingY))

	return leftBorder, topBorder, leftBorder + tileWidthInPx, topBorder + tileHeightInPx
}

// columns is how many tiles fit across the texture. It is zero when the texture
// is smaller than one tile, which is what a failed load leaves behind: the
// magenta placeholder is 1x1 and every caller here divides by this.
func (t *TextureAtlasFixed) columns() int {
	if t.itemSizeX <= 0 {
		return 0
	}
	return t.textureWidth / t.itemSizeX
}

func (t *TextureAtlasFixed) HasIndex(index int) bool {
	return index < int(t.GridWidth()*t.GridHeight())
}
func (t *TextureAtlasFixed) LastValidIndex() int {
	return int(t.GridWidth()*t.GridHeight()) - 1
}
func (t *TextureAtlasFixed) SetAtlasItemSize(width int, height int) {
	t.itemSizeX = width
	t.itemSizeY = height
}

func (t *TextureAtlasFixed) SetPaddingBetweenItems(x, y int) {
	t.itemPaddingX = x
	t.itemPaddingY = y
}

func (t *TextureAtlasFixed) GetAtlasItemSize() (int, int) {
	return t.itemSizeX, t.itemSizeY
}

func (t *TextureAtlasFixed) SetAtlasMargin(x int, y int) {
	t.itemMarginX = x
	t.itemMarginY = y
}

func (t *TextureAtlasFixed) GridWidth() int {
	return t.columns()
}

func (t *TextureAtlasFixed) GridHeight() int {
	if t.itemSizeY <= 0 {
		return 0
	}
	return t.textureHeight / t.itemSizeY
}

func (t *TextureAtlasFixed) AppendImages(index int, images []tpack.NamedImage) {
	lastIndex := index + len(images) - 1
	if lastIndex > t.LastValidIndex() {
		panic("SetPixelsAtIndexMulti: index out of bounds")
	}
	t.tex.bind()

	for i, p := range images {
		topLeftU, topLeftV, _, _ := t.GetUV(int(index) + int(i))

		w := t.itemSizeX
		h := t.itemSizeY

		minX := int(float32(topLeftU) * float32(t.textureWidth))
		minY := int(float32(topLeftV) * float32(t.textureHeight))

		t.SetPixelsRGBA(minX, minY, w, h, p.Pixels())

		glErr := gl.GetError()
		if glErr != gl.NO_ERROR {
			panic(glErr)
		}
	}
	t.tex.restore()
}
func (t *TextureAtlasFixed) SetPixelsAtIndex(index int, pixels []uint8) {
	topLeftU, topLeftV, _, _ := t.GetUV(index)

	w := t.itemSizeX
	h := t.itemSizeY

	minX := int(float32(topLeftU) * float32(t.textureWidth))
	minY := int(float32(topLeftV) * float32(t.textureHeight))

	t.tex.bind()
	t.SetPixelsRGBA(minX, minY, w, h, pixels)
	t.tex.restore()
	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
}

func (t *TextureAtlasFixed) Expand() {
	newWidth := t.textureWidth
	gridWidth := t.GridWidth()
	isVertExpansion := true
	if gridWidth < 8 {
		newWidth = t.textureWidth * 8
		gridWidth = 8
		isVertExpansion = false
	}

	newGridHeight := t.GridHeight()
	if isVertExpansion {
		newGridHeight++
	}
	newHeight := newGridHeight * t.itemSizeY
	newPixels := make([]uint8, newWidth*newHeight*4)
	for i := 0; i < len(newPixels); i++ {
		newPixels[i] = 255
	}

	sourcePixels := t.AllPixels()

	if isVertExpansion {
		copy(newPixels, sourcePixels)
	} else { // copy horizontally
		for y := 0; y < t.textureHeight; y++ {
			for x := 0; x < t.textureWidth; x++ {
				srcIndex := (y*t.textureWidth + x) * 4
				destIndex := (y*newWidth + x) * 4
				copy(newPixels[destIndex:destIndex+4], sourcePixels[srcIndex:srcIndex+4])
			}
		}
	}

	// deallocate old texture
	gl.DeleteTextures(1, &t.tex.obj)

	// create new texture
	gl.GenTextures(1, &t.tex.obj)

	t.Begin()

	// initial data
	gl.TexImage2D(
		t.target,
		0,
		gl.RGBA,
		int32(newWidth),
		int32(newHeight),
		0,
		gl.RGBA,
		gl.UNSIGNED_BYTE,
		gl.Ptr(newPixels),
	)

	borderColor := mgl32.Vec4{0, 0, 0, 0}
	gl.TexParameterfv(t.target, gl.TEXTURE_BORDER_COLOR, &borderColor[0])
	gl.TexParameteri(t.target, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_BORDER)
	gl.TexParameteri(t.target, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_BORDER)

	t.SetSmooth(t.smooth)
	t.End()

	t.textureWidth = newWidth
	t.textureHeight = newHeight
}

func (t *TextureAtlasFixed) GetSourceRectScaled(index int, scaleFactor float32) mgl32.Vec4 {
	tileSizeX, _ := t.GetAtlasItemSize()
	itemsPerRow := t.Width() / tileSizeX
	row := float32(index / itemsPerRow)
	col := float32(index % itemsPerRow)
	drawPosX := col * float32(t.itemSizeX) * scaleFactor
	drawPosY := row * float32(t.itemSizeY) * scaleFactor

	w := float32(t.itemSizeX) * scaleFactor
	h := float32(t.itemSizeY) * scaleFactor

	return mgl32.Vec4{drawPosX, drawPosY, drawPosX + w, drawPosY + h}
}

func (t *TextureAtlasFixed) GetIndexFromPxPosScaled(x int, y int, scaleFactor float32) int {
	tileSizeX, _ := t.GetAtlasItemSize()
	itemsPerRow := t.Width() / tileSizeX

	col := int(float32(x) / (float32(t.itemSizeX) * scaleFactor))
	row := int(float32(y) / (float32(t.itemSizeY) * scaleFactor))

	index := row*itemsPerRow + col
	return index
}

func (t *TextureAtlasFixed) AABBFromOpaqueAtIndex(index int) (mgl32.Vec3, mgl32.Vec3) {
	topLeftU, topLeftV, bottomRightU, bottomRightV := t.GetUV(index)

	minX := int(float32(topLeftU) * float32(t.textureWidth))
	minY := int(float32(topLeftV) * float32(t.textureHeight))
	maxX := int(float32(bottomRightU) * float32(t.textureWidth))
	maxY := int(float32(bottomRightV) * float32(t.textureHeight))

	w := maxX - minX
	h := maxY - minY
	t.tex.bind()
	pixels := t.Pixels(minX, minY, w, h)
	t.tex.restore()
	return aabbFromOpaquePixels(pixels, w, h)
}

func (t *TextureAtlasFixed) GetDestinationRect(topLeft mgl32.Vec2, scale mgl32.Vec2) mgl32.Vec4 {
	w := float32(t.itemSizeX) * scale.X()
	h := float32(t.itemSizeY) * scale.Y()
	return mgl32.Vec4{topLeft.X(), topLeft.Y(), topLeft.X() + w, topLeft.Y() + h}
}
