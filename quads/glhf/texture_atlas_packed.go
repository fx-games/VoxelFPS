package glhf

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/tpack"
	"image"
	"image/draw"
)

type TextureAtlasPacked struct {
	*Texture
	packedInfo PackedInfo
	sourceFile string
	shader     *Shader
	maxSprites int
	batch      *SpriteBatch
}

func (t *TextureAtlasPacked) GetSpriteBatch() *SpriteBatch {
	if t.batch == nil {
		t.batch = NewSpriteBatch(t.shader, t, t.maxSprites)
	}
	return t.batch
}

func (t *TextureAtlasPacked) GetSourceRect(index int) mgl32.Vec4 {
	sminU, sminV, smaxU, smaxV := t.GetUVInPx(index)
	sourceRect := mgl32.Vec4{float32(sminU), float32(sminV), float32(smaxU), float32(smaxV)}
	return sourceRect
}

func (t *TextureAtlasPacked) GetSourceRectWholeTexture() mgl32.Vec4 {
	return mgl32.Vec4{0, 0, float32(t.textureWidth), float32(t.textureHeight)}
}

func (t *TextureAtlasPacked) GetSourceAndDestRects(index int, drawPosInPixels mgl32.Vec2, normalizedOrigin mgl32.Vec2, scale mgl32.Vec2) (mgl32.Vec4, mgl32.Vec4) {
	baseWidth, baseHeight := t.packedInfo.SizeForLayout(index)
	drawOffsetX, drawOffsetY := t.packedInfo.PositionOffsetForLayout(index)
	drawOffsetX = int(float32(drawOffsetX) * scale.X())
	drawOffsetY = int(float32(drawOffsetY) * scale.Y())
	sminU, sminV, smaxU, smaxV := t.GetUVInPx(index)
	sourceRect := t.GetSourceRect(index)

	layoutWidth := float32(baseWidth) * scale.X()
	layoutHeight := float32(baseHeight) * scale.Y()

	drawWidth := float32(smaxU-sminU) * scale.X()
	drawHeight := float32(smaxV-sminV) * scale.Y()

	originInPixels := mgl32.Vec2{float32(layoutWidth * normalizedOrigin.X()), float32(layoutHeight * normalizedOrigin.Y())}
	topleftDrawPos := mgl32.Vec2{float32(drawPosInPixels.X()) - float32(originInPixels.X()) + float32(drawOffsetX), float32(drawPosInPixels.Y()) - float32(originInPixels.Y()) + float32(drawOffsetY)}
	destRect := mgl32.Vec4{float32(topleftDrawPos.X()), float32(topleftDrawPos.Y()), float32(topleftDrawPos.X() + float32(drawWidth)), float32(topleftDrawPos.Y() + float32(drawHeight))}

	return sourceRect, destRect
}

func NewTextureAtlasPacked(texture *Texture, packedInfo PackedInfo, sourceFile string) *TextureAtlasPacked {
	return &TextureAtlasPacked{
		Texture:    texture,
		sourceFile: sourceFile,
		packedInfo: packedInfo,
	}
}
func (t *TextureAtlasPacked) SourceFile() string {
	return t.sourceFile
}

func (t *TextureAtlasPacked) CanAppend(image image.Image) bool {
	return true
}

func (t *TextureAtlasPacked) GetUV(index int) (GlFloat, GlFloat, GlFloat, GlFloat) {
	minU, minV, maxU, maxV := t.packedInfo.UV(int(index))
	topLeftU := GlFloat(minU) / GlFloat(t.textureWidth)
	topLeftV := GlFloat(minV) / GlFloat(t.textureHeight)

	bottomRightU := GlFloat(maxU) / GlFloat(t.textureWidth)
	bottomRightV := GlFloat(maxV) / GlFloat(t.textureHeight)

	return topLeftU, topLeftV, bottomRightU, bottomRightV
}
func (t *TextureAtlasPacked) GetUVInPx(index int) (int, int, int, int) {
	return t.packedInfo.UV(index)
}

func (t *TextureAtlasPacked) HasIndex(index int) bool {
	return t.packedInfo.HasIndex(index)
}
func (t *TextureAtlasPacked) LastValidIndex() int {
	return t.packedInfo.LastValidIndex()
}
func (t *TextureAtlasPacked) NameOfFrame(index int) string {
	return t.packedInfo.Name(index)
}
func (t *TextureAtlasPacked) extractNamedImages() []tpack.NamedImage {
	var images []tpack.NamedImage
	for i := 0; i <= t.LastValidIndex(); i++ {
		w, h := t.packedInfo.UVSize(i)
		minX, minY := t.packedInfo.UVPosition(i)
		pixels := t.Pixels(minX, minY, w, h)
		targetW, targetH := t.packedInfo.SizeForLayout(i)
		sourceImage := image.NewRGBA(image.Rect(0, 0, w, h))
		sourceImage.Pix = pixels

		destImage := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
		drawOffsetX, drawOffsetY := t.packedInfo.PositionOffsetForLayout(i)

		dstRect := image.Rect(drawOffsetX, drawOffsetY, drawOffsetX+w, drawOffsetY+h)

		draw.Draw(destImage, dstRect, sourceImage, image.Point{}, draw.Src)
		images = append(images, tpack.NamedImage{
			Name:  t.packedInfo.Name(i),
			Image: destImage,
		})
	}
	return images
}

func (t *TextureAtlasPacked) AppendImages(index int, files []tpack.NamedImage) {
	lastIndex := index + len(files) - 1
	if lastIndex > t.LastValidIndex() {
		panic("SetPixelsAtIndexMulti: index out of bounds")
	}

	t.tex.bind()

	currentImages := t.extractNamedImages()
	allImages := append(currentImages[:index], files...)

	atlasImage, metadata := tpack.PackImagesIntoAtlasTrimmed(allImages)

	if atlasImage.Bounds().Dy() > t.textureHeight {
		t.expandToNewHeight(atlasImage.Bounds().Dy())
	}

	t.SetPixelsRGBA(0, 0, atlasImage.Bounds().Dx(), atlasImage.Bounds().Dy(), atlasImage.Pix)

	t.packedInfo = metadata

	t.tex.restore()
}
func (t *TextureAtlasPacked) Expand() {
	newHeight := t.textureHeight + 64
	t.expandToNewHeight(newHeight)
}

func (t *TextureAtlasPacked) expandToNewHeight(newHeight int) {
	newWidth := t.textureWidth
	newPixels := make([]uint8, newWidth*newHeight*4)
	for i := 0; i < len(newPixels); i++ {
		newPixels[i] = 0
	}

	sourcePixels := t.AllPixels()

	copy(newPixels, sourcePixels)

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

func (t *TextureAtlasPacked) GetSourceRectScaled(index int, scaleFactor float32) mgl32.Vec4 {
	x, y := t.packedInfo.UVPosition(index)
	drawPosX := float32(x) * scaleFactor
	drawPosY := float32(y) * scaleFactor
	w, h := t.packedInfo.UVSize(index)
	wScaled := float32(w) * scaleFactor
	hScaled := float32(h) * scaleFactor
	return mgl32.Vec4{drawPosX, drawPosY, drawPosX + wScaled, drawPosY + hScaled}
}

func (t *TextureAtlasPacked) GetIndexFromPxPosScaled(x int, y int, scaleFactor float32) int {
	for i := 0; i <= t.LastValidIndex(); i++ {
		topLeftX, topLeftY, bottomRightX, bottomRightY := t.GetSourceRectScaled(i, scaleFactor).Elem()
		if x >= int(topLeftX) && x <= int(bottomRightX) && y >= int(topLeftY) && y <= int(bottomRightY) {
			return i
		}
	}
	return -1
}

func (t *TextureAtlasPacked) AABBFromOpaqueAtIndex(index int) (mgl32.Vec3, mgl32.Vec3) {
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

func (t *TextureAtlasPacked) SetGuiShader(shader *Shader) {
	t.shader = shader
}

func (t *TextureAtlasPacked) SetMaxSprites(sprites int) {
	t.maxSprites = sprites
}
