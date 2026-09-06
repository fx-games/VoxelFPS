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

// PivotFromCenter is a frame's hotspot the way a world sprite's quad wants
// it: an offset from that quad's own center, in fractions of its trimmed
// (visible) size rather than of the picture's full untrimmed layout - the
// convention SpriteComponent.PivotOverride and comp_sprite's two fixed
// origins already share. A hotspot that fell inside the padding this frame
// had trimmed away lands outside the quad's own box, which is right: the
// picture's anchor was never on top of its visible pixels to begin with.
func (t *TextureAtlasPacked) PivotFromCenter(index int) mgl32.Vec2 {
	trimWidth, trimHeight := t.packedInfo.UVSize(index)
	if trimWidth <= 0 || trimHeight <= 0 {
		return mgl32.Vec2{0, 0}
	}
	trimOffsetX, trimOffsetY := t.packedInfo.PositionOffsetForLayout(index)
	hotspotX, hotspotY := t.packedInfo.Hotspot(index)
	fracX := float32(hotspotX-trimOffsetX) / float32(trimWidth)
	fracY := float32(hotspotY-trimOffsetY) / float32(trimHeight)
	return mgl32.Vec2{fracX - 0.5, fracY - 0.5}
}

// Hotspot is a frame's own hotspot in its own raw source pixels - the same
// number regardless of how big that frame's own canvas happens to be, unlike
// a fraction of it. Two frames of one weapon are routinely authored on
// differently sized canvases (a recoil frame's flash needs more canvas than
// the idle pose does), and a hotspot delta only means the same physical
// distance both frames agree on when it stays in this space; dividing each
// one by its own canvas size first and subtracting the fractions answers a
// different, wrong question.
func (t *TextureAtlasPacked) Hotspot(index int) (int, int) {
	return t.packedInfo.Hotspot(index)
}

// NormalizedHotspot is where this frame's own picture says to anchor it, as a
// fraction of its full (untrimmed) size - the same space GetSourceAndDestRects
// already takes normalizedOrigin in, so a caller uses this in place of a
// fixed pivot to draw a frame exactly where its own artist put it, rather
// than at a corner or a center that is only right for a picture with no
// transparent padding trimmed off it.
func (t *TextureAtlasPacked) NormalizedHotspot(index int) mgl32.Vec2 {
	width, height := t.packedInfo.SizeForLayout(index)
	if width <= 0 || height <= 0 {
		return mgl32.Vec2{0.5, 1}
	}
	hotspotX, hotspotY := t.packedInfo.Hotspot(index)
	return mgl32.Vec2{float32(hotspotX) / float32(width), float32(hotspotY) / float32(height)}
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
		hotspotX, hotspotY := t.packedInfo.Hotspot(i)
		images = append(images, tpack.NamedImage{
			Name:    t.packedInfo.Name(i),
			Image:   destImage,
			OffsetX: hotspotX,
			OffsetY: hotspotY,
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
