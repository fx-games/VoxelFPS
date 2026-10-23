package glhf

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/tpack"
	"image"
)

type TextureAtlas interface {
	GetUV(index int) (GlFloat, GlFloat, GlFloat, GlFloat)
	GetUVInPx(index int) (int, int, int, int)
	HasIndex(index int) bool
	LastValidIndex() int
	GetSourceRectScaled(index int, scaleFactor float32) mgl32.Vec4
	GetIndexFromPxPosScaled(x int, y int, scaleFactor float32) int
	AABBFromOpaqueAtIndex(index int) (mgl32.Vec3, mgl32.Vec3)
	Width() int
	Height() int
	Begin()
	BeginOnUnit(unit uint32)
	End()
	CanAppend(image image.Image) bool
	AppendImages(index int, files []tpack.NamedImage)
	SourceFile() string
	SaveAsPNG(file string) error
	Expand()
	NameOfFrame(i int) string
	GetSourceAndDestRects(index int, drawPosInPixels mgl32.Vec2, normalizedOrigin mgl32.Vec2, scale mgl32.Vec2) (mgl32.Vec4, mgl32.Vec4)
	GetSourceRect(index int) mgl32.Vec4
	GetSourceRectWholeTexture() mgl32.Vec4
	GetSpriteBatch() *SpriteBatch
	SetGuiShader(shader *Shader)
	SetMaxSprites(maxSprites int)
}
