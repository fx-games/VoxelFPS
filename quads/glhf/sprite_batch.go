package glhf

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
)

var PaintersOffset = 0.0001

const verticesPerQuad = 4

type SpriteBatch struct {
	twoDeeShader    *Shader
	gpuVertexBuffer *VertexArray[GlFloat]
	// 4 items in the buffer represent one vertex (x, y, u, v)
	cpuVertexBuffer      []GlFloat
	vertexElementCounter int
	atlas                BeginEnder
	elementsPerVertex    int
	// quadCapacity is the number of quads the index buffer can draw. The CPU
	// vertex buffer always holds exactly this many quads.
	quadCapacity int
}

func NewSpriteBatch(shader *Shader, atlas BeginEnder, quadCount int) *SpriteBatch {
	gpuVertexBuffer := NewVertexArray[GlFloat](shader, true)
	gpuVertexBuffer.DebugName = "SpriteBatch"
	gpuVertexBuffer.CreateQuadIndexBuffer(quadCount)
	elementsPerVertex := shader.VertexFormat().ElementsPerVertex()
	cpuVertexBuffer := make([]GlFloat, quadCount*verticesPerQuad*elementsPerVertex)

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
	return &SpriteBatch{
		twoDeeShader:      shader,
		gpuVertexBuffer:   gpuVertexBuffer,
		cpuVertexBuffer:   cpuVertexBuffer,
		atlas:             atlas,
		elementsPerVertex: elementsPerVertex,
		quadCapacity:      quadCount,
	}
}

func (a *SpriteBatch) HandleResize(windowWidth, windowHeight int) {
	// SHOLD HAPPEN ELSEWHERE
	//projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(windowWidth, windowHeight)
	//a.twoDeeShader.SetUniformAttr(Shader2DProjection, projectionMatrix)
}

func (a *SpriteBatch) Begin() {
	a.vertexElementCounter = 0
}

func (a *SpriteBatch) End() {
	if a.vertexElementCounter == 0 {
		return
	}
	a.gpuVertexBuffer.Begin()

	a.gpuVertexBuffer.SetVertexData(a.cpuVertexBuffer[:a.vertexElementCounter])
	a.gpuVertexBuffer.End()

}

// QuadCount is how many quads have been written since the last Begin, which is
// what Render draws.
func (a *SpriteBatch) QuadCount() int {
	return (a.vertexElementCounter / a.elementsPerVertex) / verticesPerQuad
}

func (a *SpriteBatch) Render() {
	a.RenderSlice(0, a.QuadCount())
}
func (a *SpriteBatch) RenderSlice(firstIndex int, quadCount int) {
	if quadCount == 0 {
		return
	}
	//a.twoDeeShader.Begin()
	a.gpuVertexBuffer.Begin()

	a.atlas.Begin()

	vertIndicesPerQuad := 6
	a.gpuVertexBuffer.DrawWithOffset(int32(firstIndex*vertIndicesPerQuad), int32(quadCount*vertIndicesPerQuad))

	a.atlas.End()

	a.gpuVertexBuffer.End()
	//a.twoDeeShader.End()

}

func (a *SpriteBatch) DrawColoredRect(dstRect mgl32.Vec4, color mgl32.Vec4) {
	a.DrawTextured(mgl32.Vec4{0, 0, 1, 1}, dstRect, color)
}

func (a *SpriteBatch) DrawColoredOutline(dstRect mgl32.Vec4, borderWidthInPx int, color mgl32.Vec4) {
	upperRect := mgl32.Vec4{
		dstRect[0],
		dstRect[1],
		dstRect[2],
		dstRect[1] + float32(borderWidthInPx),
	}
	lowerRect := mgl32.Vec4{
		dstRect[0],
		dstRect[3] - float32(borderWidthInPx),
		dstRect[2],
		dstRect[3],
	}
	leftRect := mgl32.Vec4{
		dstRect[0],
		dstRect[1] + float32(borderWidthInPx),
		dstRect[0] + float32(borderWidthInPx),
		dstRect[3] - float32(borderWidthInPx),
	}
	rightRect := mgl32.Vec4{
		dstRect[2] - float32(borderWidthInPx),
		dstRect[1] + float32(borderWidthInPx),
		dstRect[2],
		dstRect[3] - float32(borderWidthInPx),
	}
	a.DrawColoredRect(upperRect, color)
	a.DrawColoredRect(lowerRect, color)
	a.DrawColoredRect(leftRect, color)
	a.DrawColoredRect(rightRect, color)
}

// DrawTextured draws a 2D rectangle with the given source and destination rectangles.
// Rects are in the format minX, minY, maxX, maxY in pixel coordinates.
func (a *SpriteBatch) DrawTextured(sourceRect mgl32.Vec4, destRect mgl32.Vec4, tintColor mgl32.Vec4) {

	// HOW
	// to handle tint color?
	// background & border?

	// different textures?
	// PLAN: Bind all textures at the start of the game to texture units
	// Switch the texture units using uniforms
	// But that also means, it would make more sense to have one batch per texture.
	// hmm, have spritebatches as lazy initialized singletons on a texture object?

	if a.bufferIsFull() {
		a.grow()
	}
	layer := float32(0.5) - float32(PaintersOffset)
	InsertQuad(a.cpuVertexBuffer, a.vertexElementCounter, layer, sourceRect, destRect, tintColor)
	a.vertexElementCounter += a.elementsPerVertex * verticesPerQuad
	PaintersOffset += 0.0001
}

func InsertQuad(vertices []GlFloat, atIndex int, layer float32, srcRect, dstRect, color mgl32.Vec4) {
	// topleft
	vertices[atIndex+0] = GlFloat(dstRect.X())
	vertices[atIndex+1] = GlFloat(dstRect.Y())
	vertices[atIndex+2] = GlFloat(layer)

	vertices[atIndex+3] = GlFloat(srcRect.X())
	vertices[atIndex+4] = GlFloat(srcRect.Y())

	vertices[atIndex+5] = GlFloat(color.X())
	vertices[atIndex+6] = GlFloat(color.Y())
	vertices[atIndex+7] = GlFloat(color.Z())
	vertices[atIndex+8] = GlFloat(color.W())

	// topright
	vertices[atIndex+9] = GlFloat(dstRect.Z())
	vertices[atIndex+10] = GlFloat(dstRect.Y())
	vertices[atIndex+11] = GlFloat(layer)

	vertices[atIndex+12] = GlFloat(srcRect.Z())
	vertices[atIndex+13] = GlFloat(srcRect.Y())

	vertices[atIndex+14] = GlFloat(color.X())
	vertices[atIndex+15] = GlFloat(color.Y())
	vertices[atIndex+16] = GlFloat(color.Z())
	vertices[atIndex+17] = GlFloat(color.W())

	// bottomleft
	vertices[atIndex+18] = GlFloat(dstRect.X())
	vertices[atIndex+19] = GlFloat(dstRect.W())
	vertices[atIndex+20] = GlFloat(layer)

	vertices[atIndex+21] = GlFloat(srcRect.X())
	vertices[atIndex+22] = GlFloat(srcRect.W())

	vertices[atIndex+23] = GlFloat(color.X())
	vertices[atIndex+24] = GlFloat(color.Y())
	vertices[atIndex+25] = GlFloat(color.Z())
	vertices[atIndex+26] = GlFloat(color.W())

	// bottomright
	vertices[atIndex+27] = GlFloat(dstRect.Z())
	vertices[atIndex+28] = GlFloat(dstRect.W())
	vertices[atIndex+29] = GlFloat(layer)

	vertices[atIndex+30] = GlFloat(srcRect.Z())
	vertices[atIndex+31] = GlFloat(srcRect.W())

	vertices[atIndex+32] = GlFloat(color.X())
	vertices[atIndex+33] = GlFloat(color.Y())
	vertices[atIndex+34] = GlFloat(color.Z())
	vertices[atIndex+35] = GlFloat(color.W())
}
func (a *SpriteBatch) bufferIsFull() bool {
	return a.vertexElementCounter+(a.elementsPerVertex*verticesPerQuad) > len(a.cpuVertexBuffer)
}

// grow doubles the vertex buffer and the index buffer with it. Growing only the
// vertex buffer would make Render draw past the end of the index buffer.
func (a *SpriteBatch) grow() {
	if a.quadCapacity == 0 {
		a.quadCapacity = 1
	}
	a.quadCapacity *= 2
	a.cpuVertexBuffer = append(a.cpuVertexBuffer, make([]GlFloat, (a.quadCapacity*verticesPerQuad*a.elementsPerVertex)-len(a.cpuVertexBuffer))...)
	a.gpuVertexBuffer.CreateQuadIndexBuffer(a.quadCapacity)
}
