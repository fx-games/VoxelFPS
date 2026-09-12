package voxel

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"math"
	"os"
	"slices"
	"time"
)

type Map struct {
	meshGeneration      uint64
	ChunkSizeHorizontal int32
	ChunkSizeHeight     int32
	ChunkSizeCube       int32

	chunks          []*Chunk
	width           int32
	height          int32
	depth           int32
	bounds          Int3
	chunkShader     *glhf.Shader
	terrainTexture  *glhf.TextureAtlasFixed
	mapInfoLogger   func(string)
	gameErrorLogger func(string)

	spawnCounter          int
	maxChunkHeightForDraw int32
	trianglesDrawn        int
	verticesUploaded      bool
	modelIndex            int
	drawCallCount         int
	lookupVoxels          *glhf.TextureBuffer
	textureBufferIsDirty  bool
	emissiveTextures      map[uint16]bool
	torchLights           map[Int3]bool
	locationTex           int32
	locationVox           int32

	// For occlusion culling: the chunks in the camera's frustum this frame that
	// get their box tested, and the one box they are all tested with.
	tested   []*Chunk
	chunkBox *glhf.VertexArray[glhf.GlUInt]
}

func NewDefaultMap(width, height, depth int32) *Map {
	return NewMap(width, height, depth, 32, 32)
}

func NewMap(width, height, depth, chunkSizeHorizontal, chunkSizeHeight int32) *Map {
	m := &Map{
		ChunkSizeHorizontal:   chunkSizeHorizontal,
		ChunkSizeHeight:       chunkSizeHeight,
		chunks:                make([]*Chunk, width*height*depth),
		width:                 width,
		height:                height,
		depth:                 depth,
		maxChunkHeightForDraw: height - 1,
		locationTex:           -1,
		locationVox:           -1,
	}

	m.ChunkSizeCube = m.ChunkSizeHorizontal * m.ChunkSizeHeight * m.ChunkSizeHorizontal
	//m.culler = occlusion.NewOcclusionCuller(512, m)
	return m
}

func NewMapWithEmptyChunks(width, height, depth, chunkSizeHorizontal, chunkSizeHeight int32) *Map {
	m := NewMap(width, height, depth, chunkSizeHorizontal, chunkSizeHeight)
	for x := int32(0); x < width; x++ {
		for y := int32(0); y < height; y++ {
			for z := int32(0); z < depth; z++ {
				m.NewChunk(x, y, z)
			}
		}
	}
	return m
}

// VoxelDimensions returns the chunk-aligned size of the voxel grid. The lookup
// buffer's stride, getBlockIndex and the cube shader's mapSize uniform all have to
// agree on these numbers, so they come from here and nowhere else.
func (m *Map) VoxelDimensions() (int32, int32, int32) {
	return m.width * m.ChunkSizeHorizontal, m.height * m.ChunkSizeHeight, m.depth * m.ChunkSizeHorizontal
}

// SetBounds records how big the level itself is, which is not how big the buffer
// holding it is: chunks are 32 on a side, so a 55x30x53 level lives in a 64x32x64
// buffer and every voxel past the level is padding rather than air anything could
// reach. It is saved with the map, because the map is the only thing that knows.
func (m *Map) SetBounds(bounds Int3) {
	m.bounds = bounds
}

// Bounds is the level's own size in voxels. Anything that walks or draws the
// *level* rather than the buffer - the pathfinder's ground search, the minimap -
// has to use this: the padding above a level reads as solid, and a ground search
// that ran to the top of the buffer found "floor" on the padding of every column.
//
// A map with none recorded is one whose whole buffer is the level, which is what
// a map built to a whole number of chunks is.
func (m *Map) Bounds() Int3 {
	if m.bounds.X > 0 && m.bounds.Y > 0 && m.bounds.Z > 0 {
		return m.bounds
	}
	x, y, z := m.VoxelDimensions()
	return Int3{x, y, z}
}

func (m *Map) SetLogger(mapLogger func(string), gameErrorLogger func(string)) {
	m.mapInfoLogger = mapLogger
	m.gameErrorLogger = gameErrorLogger
}

func (m *Map) logVoxelInfo(msg string) {
	if m.mapInfoLogger != nil {
		m.mapInfoLogger(msg)
	}
}

func (m *Map) logGameError(msg string) {
	if m.gameErrorLogger != nil {
		m.gameErrorLogger(msg)
	}
}
func (m *Map) SetModelTransformAttrIndex(modelIndex int) {
	m.modelIndex = modelIndex
}
func (m *Map) SetSize(width, height, depth int32) {
	m.width = width
	m.height = height
	m.depth = depth
	m.chunks = make([]*Chunk, width*height*depth)
	for i := range m.chunks {
		x := i % int(width)
		y := (i / int(width)) % int(height)
		z := i / (int(width) * int(height))
		m.chunks[i] = NewChunk(m, int32(x), int32(y), int32(z))
		println("Created empty chunk at", x, y, z)
	}
}

func (m *Map) SetChunk(x, y, z int32, c *Chunk) {
	m.chunks[x+y*m.width+z*m.width*m.height] = c
}
func (m *Map) ChangeMaxChunkHeightForDraw(change int32) int32 {
	newValue := m.maxChunkHeightForDraw + change
	if newValue < 0 {
		newValue = 0
	} else if newValue > m.height-1 {
		newValue = m.height - 1
	}
	m.maxChunkHeightForDraw = newValue
	return newValue
}
func (m *Map) ForceReupload() {
	m.verticesUploaded = false
}

func (m *Map) ForceUpload() {
	for _, chunk := range m.chunks {
		if chunk != nil {
			chunk.isDirty = true
			chunk.GenerateAndUpload()
		}
	}
	m.bake3dTextureIndices()
	m.verticesUploaded = true
	mapSizeX, mapSizeY, mapSizeZ := m.VoxelDimensions()
	println(fmt.Sprintf("[Greedy] Map with %d cubes volume (%d, %d, %d) meshed into %d triangles", mapSizeX*mapSizeY*mapSizeZ, mapSizeX, mapSizeY, mapSizeZ, m.TriangleCount()))

}

func (m *Map) bake3dTextureIndices() {
	mapSizeX, mapSizeY, mapSizeZ := m.VoxelDimensions()

	texSize := mapSizeX * mapSizeY * mapSizeZ * 6
	lookupTable := make([]uint16, texSize)
	for z := int32(0); z < mapSizeZ; z++ {
		for y := int32(0); y < mapSizeY; y++ {
			for x := int32(0); x < mapSizeX; x++ {
				block := m.GetGlobalBlock(x, y, z)
				blockWallsStartIndex := (x + y*mapSizeX + z*mapSizeX*mapSizeY) * 6

				if block == nil || block.IsAir() {
					continue
				}

				blockPos := Int3{x, y, z}
				faceTextures := [6]uint16{
					block.ZYTextureIndexPos(), block.ZYTextureIndexNeg(),
					block.XZTextureIndexPos(), block.XZTextureIndexNeg(),
					block.XYTextureIndexPos(), block.XYTextureIndexNeg(),
				}
				for face, textureIndex := range faceTextures {
					lookupTable[blockWallsStartIndex+int32(face)] = m.packFaceLookup(textureIndex, blockPos, face)
				}
			}
		}
	}
	if m.lookupVoxels != nil || m.lookupVoxels.Size() != int(texSize) {
		m.lookupVoxels = glhf.NewTextureBuffer(lookupTable)
	} else {
		m.lookupVoxels.SetPixels(0, lookupTable)
	}
}

// Each entry in the voxel lookup buffer describes one face of one block, and the
// cube shader already fetches it once per fragment to find the texture. Everything
// else that is per face rides along in the spare bits of that same uint16 rather
// than costing a buffer, a fetch or a vertex attribute:
//
//	bit 15     emissive
//	bits 14-11 baked torch light, 0..15
//	bit 10     solid: set on every face of a block, so an air cell is the one
//	           whose entries are all zero even when a block's texture is 0
//	bits 9-0   texture index
//
// Blocks carry a byte texture id (see NewBlock), so 10 bits is 4 times the range
// the data model can produce. The solid bit is what the cube shader's ambient
// occlusion reads to know which neighbours are blocks.
const (
	EmissiveTextureBit  = uint16(0x8000)
	TorchLightMask      = uint16(0x7800)
	TorchLightShift     = 11
	SolidBlockBit       = uint16(0x0400)
	EmissiveTextureMask = uint16(0x03FF)
)

// packFaceLookup builds the lookup entry for one face of the solid block at
// blockPos: its texture, whether that texture glows, and the baked light standing
// in front of it.
//
// The light is read off the neighbouring block rather than this one because light
// lives in air - which is also why it is per face, and why it belongs here instead
// of in the vertices. A vertex-borne level forces greedy meshing to split every
// quad the gradient crosses; a lookup entry costs nothing, because the fragment is
// already reading this exact texel for its texture index.
func (m *Map) packFaceLookup(textureIndex uint16, blockPos Int3, face int) uint16 {
	entry := textureIndex&EmissiveTextureMask | SolidBlockBit
	if m.emissiveTextures[textureIndex] {
		entry |= EmissiveTextureBit
	}
	lit := m.GetBlockFromInt3(blockPos.Add(emissiveFaceOffsets[face]))
	entry |= uint16(lit.GetTorchLight()) << TorchLightShift
	return entry
}

// withEmissiveBit is packFaceLookup without a position, for the callers that only
// care whether a texture is marked.
func (m *Map) withEmissiveBit(textureIndex uint16) uint16 {
	if m.emissiveTextures[textureIndex] {
		return textureIndex | EmissiveTextureBit
	}
	return textureIndex
}

// ToggleEmissiveTexture makes every face using this texture glow, or stops it. Returns
// the new state. Keyed on the texture rather than on the block, so marking one screen
// panel lights every screen panel in the level - which is usually what you want, and is
// what makes it a one-keypress way to see the bloom threshold working.
func (m *Map) ToggleEmissiveTexture(textureIndex uint16) bool {
	if m.emissiveTextures == nil {
		m.emissiveTextures = make(map[uint16]bool)
	}
	nowEmissive := !m.emissiveTextures[textureIndex]
	if nowEmissive {
		m.emissiveTextures[textureIndex] = true
	} else {
		delete(m.emissiveTextures, textureIndex)
	}
	m.textureBufferIsDirty = true
	return nowEmissive
}

// SetEmissiveTextures replaces the whole set, for a map loader to call.
func (m *Map) SetEmissiveTextures(textureIndices []uint16) {
	m.emissiveTextures = make(map[uint16]bool, len(textureIndices))
	for _, index := range textureIndices {
		m.emissiveTextures[index] = true
	}
	m.textureBufferIsDirty = true
}

func (m *Map) EmissiveTextureCount() int {
	return len(m.emissiveTextures)
}

// remeshDirtyChunks brings dirty chunks back up to date, on the main thread,
// before anything is drawn. GreedyMeshing reads chunk.data and the chunk's shared
// chunkHelper, so meshing on a goroutine raced with the editor writing blocks -
// and one goroutine per dirty chunk per frame was spawned until the upload landed.
//
// ponytail: synchronous, a couple of chunks per frame. If painting large areas
// hitches, the fix is meshing from a copy of chunk.data, not a goroutine on the
// live one.
const remeshBudgetPerFrame = 2

func (m *Map) remeshDirtyChunks() {
	budget := remeshBudgetPerFrame
	for _, chunk := range m.chunks {
		if budget == 0 {
			return
		}
		if chunk == nil || !chunk.isDirty {
			continue
		}
		chunk.GenerateAndUpload()
		budget--
	}
}

// DrawWithShader draws the chunks the observer can see. With a box shader - the
// camera's pass - that is what its frustum holds and no wall hides; without one,
// everything in its frustum.
func (m *Map) DrawWithShader(observer core.Observer, chunkShader, boxShader *glhf.Shader) {
	m.remeshDirtyChunks()
	if m.textureBufferIsDirty {
		// The light has to be re-flooded before the buffer is packed, because the
		// buffer is where it ends up. Anything that dirties the buffer - a block
		// placed, a texture repainted, a panel switched on - can have moved a wall
		// the light was flowing around, and this is the one place that knows. The
		// flag is cleared after the flood, which sets it again on its way through.
		//
		// ponytail: a full re-flood of the map on any edit, ~8ms on raymap, in a
		// frame that already walks every block and re-uploads the buffer. If
		// painting ever hitches, re-flood only the chunks near the edit.
		m.bakeEmissiveTorchlight()
		m.textureBufferIsDirty = false
		m.bake3dTextureIndices()
	}
	if !m.verticesUploaded {
		m.ForceUpload()
	}
	chunkShader.Begin()

	m.terrainTexture.BeginOnUnit(gl.TEXTURE0)
	m.lookupVoxels.BeginOnUnit(gl.TEXTURE1)

	m.trianglesDrawn = 0
	m.drawCallCount = 0
	m.tested = m.tested[:0]
	for _, chunk := range m.chunks {
		if chunk == nil || chunk.meshBuffer.TriangleCount() == 0 || !m.chunkIsVisible(observer, chunk) || chunk.chunkPosY > m.maxChunkHeightForDraw {
			continue
		}
		if boxShader != nil && m.testsOcclusion(chunk, observer.GetPosition()) {
			m.tested = append(m.tested, chunk)
			if chunk.hidden {
				continue
			}
		}
		m.trianglesDrawn += chunk.meshBuffer.TriangleCount()
		m.drawCallCount += 1
		chunk.Draw(chunkShader, m.modelIndex)
	}
	chunkShader.End()

	if boxShader != nil {
		m.drawOccluded(chunkShader, boxShader)
	}

	m.lookupVoxels.End()
	m.terrainTexture.End()

	glhf.ActiveTexture(gl.TEXTURE0)
}

// Occlusion culling, with the GPU's occlusion queries. A chunk is 32 voxels on a
// side and inside a ship nearly all of them are behind a wall, a floor or a
// ceiling, which the frustum cannot know and the depth buffer can:
//
//  1. the chunks that were visible last time are drawn, as they always were;
//  2. every chunk's box is drawn into that depth buffer, invisibly, each inside
//     a query that counts whether any of it got past the depth test;
//  3. the chunks that were hidden last time are drawn on their query's word -
//     conditional rendering, decided on the GPU, so a chunk that comes into
//     view is drawn in the same frame and nothing pops in a frame late.
//
// The result of 2 is read back a frame later to sort the chunks for 1, which is
// only a guess at what to draw unconditionally: a wrong guess costs a wasted
// draw or a late one, never a missing chunk. The count of what was drawn leaves
// out 3, which is nothing unless something just came into view.
//
// A box the camera stands in or next to cannot be tested - its near side is
// clipped away and its far side is behind what it holds - so that chunk is
// always drawn. A shadow pass passes no box shader and culls nothing: a light
// has to see what the player cannot, it throws shadows where they can.
func (m *Map) testsOcclusion(chunk *Chunk, eye mgl32.Vec3) bool {
	if NewAABB(chunk.AsAABB()).InflatedBy(mgl32.Vec3{1, 1, 1}).Contains(eye) {
		chunk.hidden = false
		return false
	}
	if chunk.query != 0 {
		var ready, passed uint32
		gl.GetQueryObjectuiv(chunk.query, gl.QUERY_RESULT_AVAILABLE, &ready)
		if ready != 0 {
			gl.GetQueryObjectuiv(chunk.query, gl.QUERY_RESULT, &passed)
			chunk.hidden = passed == 0
		}
	}
	return true
}

func (m *Map) drawOccluded(chunkShader, boxShader *glhf.Shader) {
	box := m.box(boxShader)
	culling := gl.IsEnabled(gl.CULL_FACE)
	gl.Disable(gl.CULL_FACE) // from inside the frustum's reach a box shows its back
	gl.ColorMask(false, false, false, false)
	gl.DepthMask(false)
	// Pulled a hair towards the camera, so a box face lying on the very surface
	// its own chunk drew - a floor on the chunk's top edge - is in front of it
	// rather than a rounding error either side.
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(-1, -1)
	boxShader.Begin()
	box.Begin()
	for _, chunk := range m.tested {
		if chunk.query == 0 {
			gl.GenQueries(1, &chunk.query)
		}
		boxShader.SetMat4(m.modelIndex, chunk.GetMatrix())
		gl.BeginQuery(gl.ANY_SAMPLES_PASSED, chunk.query)
		box.Draw()
		gl.EndQuery(gl.ANY_SAMPLES_PASSED)
	}
	box.End()
	boxShader.End()
	gl.Disable(gl.POLYGON_OFFSET_FILL)
	gl.DepthMask(true)
	gl.ColorMask(true, true, true, true)
	if culling {
		gl.Enable(gl.CULL_FACE)
	}

	chunkShader.Begin()
	for _, chunk := range m.tested {
		if chunk.hidden {
			// Not chunk.Draw: its glValidateProgram inside a conditional render
			// segfaults Apple's GL driver in glEndConditionalRender.
			chunkShader.SetMat4(m.modelIndex, chunk.GetMatrix())
			chunk.meshBuffer.drawableVertexData.Begin()
			gl.BeginConditionalRender(chunk.query, gl.QUERY_WAIT)
			chunk.meshBuffer.drawableVertexData.Draw()
			gl.EndConditionalRender()
			chunk.meshBuffer.drawableVertexData.End()
		}
	}
	chunkShader.End()
}

// box is a chunk's bounding box in the chunks' own vertex format, so it is drawn
// with the chunk depth shader and the same model matrix as the chunk it stands for.
func (m *Map) box(shader *glhf.Shader) *glhf.VertexArray[glhf.GlUInt] {
	if m.chunkBox != nil {
		return m.chunkBox
	}
	var packer MeshBuffer
	corner := func(i int) glhf.GlUInt { // bit 0 is x, 1 is y, 2 is z
		return glhf.GlUInt(packer.Compress(Int3{
			int32(i&1) * m.ChunkSizeHorizontal,
			int32(i>>1&1) * m.ChunkSizeHeight,
			int32(i>>2&1) * m.ChunkSizeHorizontal,
		}, Top))
	}
	var vertices []glhf.GlUInt
	for _, side := range [6][4]int{{0, 1, 3, 2}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 3, 7, 6}, {0, 2, 6, 4}, {1, 3, 7, 5}} {
		for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
			vertices = append(vertices, corner(side[i]))
		}
	}
	m.chunkBox = glhf.NewVertexArray[glhf.GlUInt](shader, false)
	m.chunkBox.Begin()
	m.chunkBox.SetVertexData(vertices)
	m.chunkBox.End()
	return m.chunkBox
}

// MeshGeneration counts chunk mesh uploads. It only has to change when the world
// changes, so it is enough to tell a cached shadow map that it is stale.
func (m *Map) MeshGeneration() uint64 {
	return m.meshGeneration
}

func (m *Map) chunkIsVisible(frustum core.Culler, chunk *Chunk) bool {
	return frustum.IsCubeInViewFrustum(chunk.AsAABB())
}

/*
func (m *Map) isChunkVisibleInFrustum(planes []mgl32.Vec4, chunkPos Int3) bool {
    p := mgl32.Vec3{float32(chunkPos.X * m.ChunkSizeHorizontal), float32(chunkPos.Y * m.ChunkSizeHeight), float32(chunkPos.Z * m.ChunkSizeHorizontal)}
    horizOffset := float32(m.ChunkSizeHorizontal)

    points := []mgl32.Vec3{
        mgl32.Vec3{p.X(), p.Y(), p.Z()},
        mgl32.Vec3{p.X() + horizOffset, p.Y(), p.Z()},
        mgl32.Vec3{p.X() + horizOffset, p.Y(), p.Z() + horizOffset},
        mgl32.Vec3{p.X(), p.Y(), p.Z() + horizOffset},

        mgl32.Vec3{p.X(), p.Y() + 256, p.Z()},
        mgl32.Vec3{p.X() + horizOffset, p.Y() + 256, p.Z()},
        mgl32.Vec3{p.X() + horizOffset, p.Y() + 256, p.Z() + horizOffset},
        mgl32.Vec3{p.X(), p.Y() + 256, p.Z() + horizOffset},
    }
    for _, plane := range planes {
        var in, out int
        for _, point := range points {
            if plane.Dot(point.Vec4(1)) < 0 {
                out++
            } else {
                in++
            }
            if in != 0 && out != 0 {
                break
            }
        }
        if in == 0 {
            return false
        }
    }
    return true
}

*/

func (m *Map) GetChunkFromPosition(pos mgl32.Vec3) *Chunk {
	x := math.Floor(float64(pos.X()))
	y := math.Floor(float64(pos.Y()))
	z := math.Floor(float64(pos.Z()))
	return m.GetChunkFromBlock(int32(x), int32(y), int32(z))
}

func (m *Map) GetBlockFromPosition(position mgl32.Vec3) *Block {
	x := math.Floor(float64(position.X()))
	y := math.Floor(float64(position.Y()))
	z := math.Floor(float64(position.Z()))
	return m.GetGlobalBlock(int32(x), int32(y), int32(z))
}

func (m *Map) GetChunkFromBlock(x, y, z int32) *Chunk {
	return m.GetChunk(x/m.ChunkSizeHorizontal, y/m.ChunkSizeHeight, z/m.ChunkSizeHorizontal)
}
func (m *Map) GetChunk(x, y, z int32) *Chunk {
	i := x + y*m.width + z*m.width*m.height
	if i < 0 || i >= int32(len(m.chunks)) {
		return nil
	} else {
		return m.chunks[i]
	}
}
func (m *Map) ChunkExists(x, y, z int32) bool {
	return m.GetChunk(x, y, z) != nil
}
func (m *Map) SetBlock(x int32, y int32, z int32, block *Block) {
	if !m.Contains(x, y, z) {
		return
	}
	chunkX := x / m.ChunkSizeHorizontal
	chunkY := y / m.ChunkSizeHeight
	chunkZ := z / m.ChunkSizeHorizontal
	chunk := m.GetChunk(chunkX, chunkY, chunkZ)
	if chunk != nil {
		chunk.SetBlock(x%m.ChunkSizeHorizontal, y%m.ChunkSizeHeight, z%m.ChunkSizeHorizontal, block)
	}
}

func (m *Map) SetBlocks(pos []Int3, block *Block) {
	for _, p := range pos {
		m.SetBlock(p.X, p.Y, p.Z, block.Copy())
	}
	m.textureBufferIsDirty = true
}

func (m *Map) SetAir(blockPos Int3) {
	m.SetBlock(blockPos.X, blockPos.Y, blockPos.Z, NewAirBlock())
}

func (m *Map) SetAirBlocks(pos []Int3) {
	for _, p := range pos {
		m.SetAir(p)
	}
}

func (m *Map) Contains(x int32, y int32, z int32) bool {
	return x >= 0 && x < m.width*m.ChunkSizeHorizontal && y >= 0 && y < m.height*m.ChunkSizeHeight && z >= 0 && z < m.depth*m.ChunkSizeHorizontal
}

func (m *Map) GetBlockFromInt3(pos Int3) *Block {
	return m.GetGlobalBlock(int32(pos.X), int32(pos.Y), int32(pos.Z))
}

// GetGlobalBlock is the block at a cell, and air anywhere outside the map. The
// check has to come first: the chunk index is flattened, so a cell one past the
// east or top edge would otherwise read a block out of some other chunk - a wall
// standing in the air beyond the level.
func (m *Map) GetGlobalBlock(x int32, y int32, z int32) *Block {
	if !m.Contains(x, y, z) {
		return EmptyBlock
	}
	chunkX := x / m.ChunkSizeHorizontal
	chunkY := y / m.ChunkSizeHeight
	chunkZ := z / m.ChunkSizeHorizontal
	chunk := m.GetChunk(chunkX, chunkY, chunkZ)
	if chunk != nil {
		blockX := x % m.ChunkSizeHorizontal
		blockY := y % m.ChunkSizeHeight
		blockZ := z % m.ChunkSizeHorizontal
		return chunk.GetLocalBlock(blockX, blockY, blockZ)
	} else {
		return EmptyBlock
	}
}

var EmptyBlock = NewAirBlock()

func (m *Map) GetBlockAndChunk(blockPos Int3) (*Block, *Chunk) {
	if !m.ContainsGrid(blockPos) {
		return nil, nil
	}
	x := blockPos.X
	y := blockPos.Y
	z := blockPos.Z
	chunkX := x / m.ChunkSizeHorizontal
	chunkY := y / m.ChunkSizeHeight
	chunkZ := z / m.ChunkSizeHorizontal
	chunk := m.GetChunk(chunkX, chunkY, chunkZ)
	if chunk != nil {
		blockX := x % m.ChunkSizeHorizontal
		blockY := y % m.ChunkSizeHeight
		blockZ := z % m.ChunkSizeHorizontal
		return chunk.GetLocalBlock(blockX, blockY, blockZ), chunk
	} else {
		return nil, nil
	}
}

func (m *Map) IsSolidBlockAt(x int32, y int32, z int32) bool {
	block := m.GetGlobalBlock(x, y, z)
	return block != nil && !block.IsAir()
}

func (m *Map) ContainsGrid(position Int3) bool {
	return m.Contains(position.X, position.Y, position.Z)
}

func (m *Map) SetShader(chunkShader *glhf.Shader) {
	m.chunkShader = chunkShader
}

func (m *Map) SetTerrainTexture(texture *glhf.TextureAtlasFixed) {
	m.terrainTexture = texture
}

func (m *Map) NewChunk(cX int32, cY int32, cZ int32) *Chunk {
	chunk := NewChunk(m, cX, cY, cZ)
	m.SetChunk(cX, cY, cZ, chunk)
	return chunk
}

func (m *Map) SetFloorAtHeight(yLevel int, block *Block) {
	for _, chunk := range m.chunks {
		if chunk.chunkPosY != 0 {
			continue
		}
		for x := int32(0); x < m.ChunkSizeHorizontal; x++ {
			for z := int32(0); z < m.ChunkSizeHorizontal; z++ {
				chunk.SetBlock(x, int32(yLevel), z, block)
			}
		}
	}
}

type MapObject interface {
	GetOccupiedBlockOffsets(Int3) []Int3
	UnitID() uint64
	GetName() string
}

// PositionToGridInt3 converts a Vec3 to a Int3 by flooring the values.
// DO NOT USE THIS FOR NEGATIVE VALUES, eg. direction vectors
func PositionToGridInt3(pos mgl32.Vec3) Int3 {
	return Int3{int32(math.Floor(float64(pos.X()))), int32(math.Floor(float64(pos.Y()))), int32(math.Floor(float64(pos.Z())))}
}

func DirectionToGridInt3(dir mgl32.Vec3) Int3 {
	return Int3{int32(math.Round(float64(dir.X()))), int32(math.Round(float64(dir.Y()))), int32(math.Round(float64(dir.Z())))}
}

func (m *Map) IsHumanoidPlaceable(blockPos Int3) (bool, string) {
	offsets := []Int3{
		{0, 0, 0},
		{0, 1, 0},
	}
	for _, offset := range offsets {
		occupiedBlockPos := blockPos.Add(offset)
		outsideOfWorld := !m.ContainsGrid(occupiedBlockPos)
		if outsideOfWorld {
			return false, "Outside of world"
		}
		isWall := m.IsSolidBlockAt(occupiedBlockPos.X, occupiedBlockPos.Y, occupiedBlockPos.Z)
		if isWall {
			return false, "Wall"
		}
	}
	below := blockPos.Add(Int3{0, -1, 0})
	return m.IsSolidBlockAt(below.X, below.Y, below.Z), "No floor"
}

var cardinalNeighbors = []Int3{
	{1, 0, 0},
	{-1, 0, 0},
	{0, 0, 1},
	{0, 0, -1},
}

var cardinalNeighborsUpDown = []Int3{
	{0, 1, 0},
	{0, -1, 0},
}
var cardinalNeighbors3D = append(cardinalNeighbors, cardinalNeighborsUpDown...)
var diagonalNeighbors = []Int3{
	{1, 0, 1},
	{-1, 0, 1},
	{1, 0, -1},
	{-1, 0, -1},
}

var allNeighbors = append(cardinalNeighbors, diagonalNeighbors...)

func (m *Map) GetBlockedCardinalNeighborsUpToHeight(unitID uint64, position Int3, height int32) []Int3 {
	neighbors := make([]Int3, 0, 4)
	for _, offset := range cardinalNeighbors {
		neighbor := position.Add(offset)
		for y := int32(0); y < height; y++ {
			if m.IsSolidBlockAt(neighbor.X, neighbor.Y+y, neighbor.Z) {
				neighbors = append(neighbors, neighbor)
				break
			}
		}
	}
	return neighbors
}

func (m *Map) GetNeighborsForGroundMovement(block Int3, keepPredicate func(neighbor Int3) bool) []Int3 {
	neighbors := make([]Int3, 0, 8)

	for _, offset := range cardinalNeighbors {
		neighbor := block.Add(offset)
		if m.ContainsGrid(neighbor) {
			if m.IsSolidBlockAt(neighbor.X, neighbor.Y, neighbor.Z) {
				// check one block above (climbing one block is allowed)
				neighbor = neighbor.Add(Int3{Y: 1})
			} else {
				// get the lowest solid block below and test the block above that
				neighbor = m.GetGroundPosition(neighbor)
			}
			if m.ContainsGrid(neighbor) && keepPredicate(neighbor) {
				neighbors = append(neighbors, neighbor)
			}
		}
	}

	for _, offset := range diagonalNeighbors {
		neighbor := block.Add(offset)
		neighbor = m.GetGroundPosition(neighbor)
		if m.ContainsGrid(neighbor) && keepPredicate(neighbor) {
			neighbors = append(neighbors, neighbor)
		}
	}

	return neighbors
}

func (m *Map) GetGroundPosition(startBlock Int3) Int3 {
	// iterate down until we hit a solid block
	for y := startBlock.Y; y >= 1; y-- {
		if m.IsSolidBlockAt(startBlock.X, y-1, startBlock.Z) || !m.ContainsGrid(Int3{startBlock.X, y - 1, startBlock.Z}) {
			return Int3{startBlock.X, y, startBlock.Z}
		}
	}
	return startBlock
}

func (m *Map) ClearAllChunks() {
	for _, chunk := range m.chunks {
		if chunk != nil {
			chunk.ClearAllBlocks()
		}
	}
}

func (m *Map) GetTerrainTexture() *glhf.TextureAtlasFixed {
	return m.terrainTexture
}

func (m *Map) GetSize() Int3 {
	return Int3{m.width, m.height, m.depth}
}

func (m *Map) CurrentlyDraws(x int32, y int32, z int32) bool {
	chunk := m.GetChunkFromBlock(x, y, z)
	return chunk != nil && chunk.chunkPosY <= m.maxChunkHeightForDraw
}

func (m *Map) ForBlockInHalfSphere(origin Int3, radius float64, applyToBlock func(origin Int3, radius float64, x int32, y int32, z int32)) {
	for x := int32(-radius); x <= int32(radius); x++ {
		for y := int32(0); y <= int32(radius); y++ {
			for z := int32(-radius); z <= int32(radius); z++ {
				if float64(x*x+y*y+z*z) <= radius*radius {
					if !m.Contains(origin.X+x, origin.Y+y, origin.Z+z) {
						continue
					}
					applyToBlock(origin, radius, origin.X+x, origin.Y+y, origin.Z+z)
				}
			}
		}
	}
}
func (m *Map) ForBlockInSphere(origin Int3, radius float64, applyToBlock func(origin Int3, radius float64, x int32, y int32, z int32)) {
	for x := int32(-radius); x <= int32(radius); x++ {
		for y := int32(-radius); y <= int32(radius); y++ {
			for z := int32(-radius); z <= int32(radius); z++ {
				if float64(x*x+y*y+z*z) <= radius*radius {
					if !m.Contains(origin.X+x, origin.Y+y, origin.Z+z) {
						continue
					}
					applyToBlock(origin, radius, origin.X+x, origin.Y+y, origin.Z+z)
				}
			}
		}
	}
}

type DistLocPair struct {
	Steps    int
	Distance float64
	Location Int3
}

// MaxTorchLight is the brightest a block can be. Four bits per block, and the
// vertex format carries the same four - see MeshBuffer.Compress.
const MaxTorchLight = byte(15)

// EmissiveTorchLight is what an emissive wall panel puts into the air in front of
// it. Below the maximum on purpose: a wall fitting is a fill light, and leaving
// headroom keeps a hand-placed FillTorchlight brighter than the scenery.
const EmissiveTorchLight = byte(12)

// FillTorchlight floods baked light out from one block, losing one level per step
// and stopping at solid blocks. It dirties the lookup buffer, which is re-baked
// before the next draw - no re-meshing, because light is not in the vertices.
func (m *Map) FillTorchlight(origin Int3) {
	m.fillTorchlightFrom(origin, MaxTorchLight)
}

func (m *Map) fillTorchlightFrom(origin Int3, level byte) {
	if !m.ContainsGrid(origin) || level == 0 {
		return
	}

	startBlock, _ := m.GetBlockAndChunk(origin)
	if startBlock == nil || !startBlock.IsAir() {
		return // light lives in the air; the mesher only reads it off air blocks
	}
	if startBlock.GetTorchLight() >= level {
		return // already at least this bright from another emitter
	}
	startBlock.SetTorchLight(level)
	m.textureBufferIsDirty = true

	spread := func(origin, currentNode, currentNeighbor Int3, costToPrev int) (bool, int) {
		if m.IsSolidBlockAt(currentNeighbor.X, currentNeighbor.Y, currentNeighbor.Z) {
			return false, 0
		}

		nodeLight := m.GetBlockFromInt3(currentNode).GetTorchLight()
		// byte arithmetic: nodeLight-1 at 0 wraps to 255 and every dark block in
		// the map then looks like it should be lit to 255.
		if nodeLight == 0 {
			return false, 0
		}

		neighborBlock, _ := m.GetBlockAndChunk(currentNeighbor)
		if neighborBlock == nil {
			return false, 0
		}

		if neighborBlock.GetTorchLight() < nodeLight-1 {
			neighborBlock.SetTorchLight(nodeLight - 1)
			m.textureBufferIsDirty = true
			return true, costToPrev + 1
		}
		return false, 0
	}

	m.FloodFill(origin, int(level), spread)
}

// AddTorchLight registers a point emitter at a grid position - a torch dropped by
// the editor. It has to be map state rather than a one-off FillTorchlight call:
// bakeEmissiveTorchlight starts every re-bake from dark, so light that is not
// re-seeded there vanishes the next time a block is placed or a panel is toggled.
func (m *Map) AddTorchLight(pos Int3) {
	if m.torchLights == nil {
		m.torchLights = make(map[Int3]bool)
	}
	m.torchLights[pos] = true
	m.textureBufferIsDirty = true
}

// RemoveTorchLight drops an emitter. The light it cast goes away on the next bake.
func (m *Map) RemoveTorchLight(pos Int3) {
	if _, exists := m.torchLights[pos]; !exists {
		return
	}
	delete(m.torchLights, pos)
	m.textureBufferIsDirty = true
}

func (m *Map) TorchLightCount() int {
	return len(m.torchLights)
}

// emissiveFaceOffsets is the block a face of the given index faces into, in the one
// face order bake3dTextureIndices writes and the cube shader's normalLookup reads:
// +X, -X, +Y, -Y, +Z, -Z.
var emissiveFaceOffsets = [6]Int3{
	{1, 0, 0}, {-1, 0, 0},
	{0, 1, 0}, {0, -1, 0},
	{0, 0, 1}, {0, 0, -1},
}

// bakeEmissiveTorchlight turns the emissive texture set into per-block light: every
// face using one of those textures seeds the air in front of it, and the light
// floods from there. It is the cheap half of the lighting - dozens of static
// sources for four spare bits in a buffer the shader already reads - and it is what
// the shadow-mapped spotlights cannot afford to be.
//
// Called from DrawWithShader whenever the face lookup buffer needs rebuilding, which
// is the same signal for both: the buffer carries the light, so anything that
// invalidates one invalidates the other and they cannot drift apart.
//
// ponytail: one flood fill per emitting face over the whole map. Walls stop the
// fill after a few metres, so on a corridor map this is milliseconds. If a big
// open map makes a map load or a G keypress hitch, the fix is one multi-source
// BFS seeded with every emitter, not a goroutine.
func (m *Map) bakeEmissiveTorchlight() {
	m.clearTorchlight()

	// Hand-placed torches first: they are the brightest thing in the fill, so
	// seeding them before the panels saves the panels re-flooding blocks a torch
	// already lit brighter.
	for pos := range m.torchLights {
		m.FillTorchlight(pos)
	}

	if len(m.emissiveTextures) == 0 {
		return
	}

	start := time.Now()
	emitters := 0
	mapSizeX, mapSizeY, mapSizeZ := m.VoxelDimensions()
	for z := int32(0); z < mapSizeZ; z++ {
		for y := int32(0); y < mapSizeY; y++ {
			for x := int32(0); x < mapSizeX; x++ {
				block := m.GetGlobalBlock(x, y, z)
				if block == nil || block.IsAir() {
					continue
				}
				faces := [6]uint16{
					block.ZYTextureIndexPos(), block.ZYTextureIndexNeg(),
					block.XZTextureIndexPos(), block.XZTextureIndexNeg(),
					block.XYTextureIndexPos(), block.XYTextureIndexNeg(),
				}
				for face, textureIndex := range faces {
					if !m.emissiveTextures[textureIndex] {
						continue
					}
					m.fillTorchlightFrom(Int3{x, y, z}.Add(emissiveFaceOffsets[face]), EmissiveTorchLight)
					emitters++
				}
			}
		}
	}
	println(fmt.Sprintf("[Light] %d emissive faces baked into voxel light in %s", emitters, time.Since(start).Round(time.Millisecond)))
}

// clearTorchlight resets every block's baked light. Re-baking has to start from
// dark: a panel that stopped being emissive would otherwise keep its light.
func (m *Map) clearTorchlight() {
	mapSizeX, mapSizeY, mapSizeZ := m.VoxelDimensions()
	for z := int32(0); z < mapSizeZ; z++ {
		for y := int32(0); y < mapSizeY; y++ {
			for x := int32(0); x < mapSizeX; x++ {
				block := m.GetGlobalBlock(x, y, z)
				if block == nil || block.GetTorchLight() == 0 {
					continue
				}
				block.SetTorchLight(0)
				m.textureBufferIsDirty = true
			}
		}
	}
}
func (m *Map) FloodFill(origin Int3, maxSteps int, spreadFunc func(origin, location, neighbor Int3, stepsToLocation int) (bool, int)) {
	visited := make(map[Int3]bool)
	visited[origin] = true
	queue := []DistLocPair{{0, 0, origin}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.Steps > maxSteps {
			continue
		}
		for _, offset := range cardinalNeighbors3D {
			neighbor := current.Location.Add(offset)
			if !m.ContainsGrid(neighbor) {
				continue
			}
			_, alreadyVisited := visited[neighbor]
			if alreadyVisited {
				continue
			}

			shouldSpread, neighborSteps := spreadFunc(origin, current.Location, neighbor, current.Steps)

			if !shouldSpread {
				continue
			}

			neighborDistance := origin.Sub(neighbor).Length()

			visited[neighbor] = true
			queue = append(queue, DistLocPair{neighborSteps, neighborDistance, neighbor})
		}
	}
}
func (m *Map) ForBlockInSphericFloodFill(origin Int3, radius float64, applyToBlock func(origin Int3, steps int, distance float64, x int32, y int32, z int32)) {
	visited := make(map[Int3]bool)
	visited[origin] = true
	queue := []DistLocPair{{0, 0, origin}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		applyToBlock(origin, current.Steps, current.Distance, current.Location.X, current.Location.Y, current.Location.Z)
		for _, offset := range cardinalNeighbors3D {
			neighbor := current.Location.Add(offset)
			if !m.ContainsGrid(neighbor) {
				continue
			}
			_, alreadyVisited := visited[neighbor]
			if alreadyVisited {
				continue
			}
			if m.IsSolidBlockAt(neighbor.X, neighbor.Y, neighbor.Z) {
				continue
			}
			neighborDistance := origin.Sub(neighbor).Length()
			neighborSteps := current.Steps + 1
			if neighborDistance > radius {
				continue
			}
			if float64(neighborSteps) > 2*radius {
				continue
			}
			visited[neighbor] = true
			queue = append(queue, DistLocPair{neighborSteps, neighborDistance, neighbor})
		}
	}
}

func (m *Map) TriangleCount() int {
	count := 0
	for _, chunk := range m.chunks {
		if chunk != nil {
			count += chunk.meshBuffer.TriangleCount()
		}
	}
	return count
}

func (m *Map) TriangleDrawCount() int {
	return m.trianglesDrawn
}

func (m *Map) DrawCalls() int {
	return m.drawCallCount
}

func (m *Map) DrawAABBs(lineShader *glhf.Shader) {
	for _, chunk := range m.chunks {
		if chunk != nil {
			// Already in world space, so it is placed by nothing.
			NewAABB(chunk.AsAABB()).QueueDebugDraw(mgl32.Ident4())
		}
	}
	DrawDebugLines(lineShader)
}

func (m *Map) ChunkIndex(x int32, y int32, z int32) int {
	return int(x + y*m.width + z*m.width*m.height)
}

func (m *Map) SaveToDisk(filename string) error {
	// serialize this map manually to a byte array
	outfile, err := os.Create(filename)
	if err != nil {
		return err
	}
	// use a gzip writer to compress the byte array
	// then write the compressed byte array to the file

	gzipWriter := gzip.NewWriter(outfile)
	// write 2xint32
	binary.Write(gzipWriter, binary.LittleEndian, m.ChunkSizeHorizontal)
	binary.Write(gzipWriter, binary.LittleEndian, m.ChunkSizeHeight)

	// write 3xint32
	binary.Write(gzipWriter, binary.LittleEndian, m.width)
	binary.Write(gzipWriter, binary.LittleEndian, m.height)
	binary.Write(gzipWriter, binary.LittleEndian, m.depth)
	// ...and the level's own size inside that buffer, see SetBounds.
	bounds := m.Bounds()
	binary.Write(gzipWriter, binary.LittleEndian, bounds.X)
	binary.Write(gzipWriter, binary.LittleEndian, bounds.Y)
	binary.Write(gzipWriter, binary.LittleEndian, bounds.Z)
	// write the map dimensions
	m.logVoxelInfo(fmt.Sprintf("[Map] Saving map with dimensions %d %d %d", m.width, m.height, m.depth))

	// write the number of chunks
	chunkCount := int16(len(m.chunks))
	binary.Write(gzipWriter, binary.LittleEndian, chunkCount)
	m.logVoxelInfo(fmt.Sprintf("[Map] Saving %d chunks", chunkCount))

	// write the chunks
	for _, chunk := range m.chunks {
		binary.Write(gzipWriter, binary.LittleEndian, chunk.chunkPosX)
		binary.Write(gzipWriter, binary.LittleEndian, chunk.chunkPosY)
		binary.Write(gzipWriter, binary.LittleEndian, chunk.chunkPosZ)
		m.logVoxelInfo(fmt.Sprintf("[Map] Saving chunk %d %d %d", chunk.chunkPosX, chunk.chunkPosY, chunk.chunkPosZ))
		for _, block := range chunk.data {
			if block.IsSolid {
				err = binary.Write(gzipWriter, binary.LittleEndian, uint8(1))
				if err != nil {
					return err
				}
				for cs := CubeSide(0); cs < 6; cs++ {
					var texIndex uint16
					if texIdOfSide, hasTexId := block.Textures[cs]; hasTexId {
						texIndex = texIdOfSide
					}
					err = binary.Write(gzipWriter, binary.LittleEndian, texIndex)
					if err != nil {
						return err
					}
				}
			} else {
				err = binary.Write(gzipWriter, binary.LittleEndian, uint8(0))
				if err != nil {
					return err
				}
			}
		}
		m.logVoxelInfo(fmt.Sprintf("[Map] Saved %d blocks", len(chunk.data)))
	}
	err = gzipWriter.Flush()
	if err != nil {
		return err
	}
	err = gzipWriter.Close()
	if err != nil {
		return err
	}
	err = outfile.Close()
	if err != nil {
		return err
	}

	return nil
}
func (m *Map) LoadFromDisk(filename string) {

	buffer, readErr := os.ReadFile(filename)
	if readErr != nil {
		panic(readErr)
	}
	m.LoadFromMemory(buffer)
}
func (m *Map) LoadFromMemory(source []byte) {
	reader := bytes.NewReader(source)
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		panic(err)
	}
	// read chunk size
	binary.Read(gzipReader, binary.LittleEndian, &m.ChunkSizeHorizontal)
	binary.Read(gzipReader, binary.LittleEndian, &m.ChunkSizeHeight)

	m.ChunkSizeCube = m.ChunkSizeHorizontal * m.ChunkSizeHeight * m.ChunkSizeHorizontal

	// read the map dimensions
	binary.Read(gzipReader, binary.LittleEndian, &m.width)
	binary.Read(gzipReader, binary.LittleEndian, &m.height)
	binary.Read(gzipReader, binary.LittleEndian, &m.depth)
	binary.Read(gzipReader, binary.LittleEndian, &m.bounds.X)
	binary.Read(gzipReader, binary.LittleEndian, &m.bounds.Y)
	binary.Read(gzipReader, binary.LittleEndian, &m.bounds.Z)
	m.logVoxelInfo(fmt.Sprintf("[Map] Loading map with dimensions %d %d %d", m.width, m.height, m.depth))

	// read the number of chunks
	m.maxChunkHeightForDraw = m.height - 1

	chunkCount := int16(0)
	binary.Read(gzipReader, binary.LittleEndian, &chunkCount)
	m.logVoxelInfo(fmt.Sprintf("[Map] Loading %d chunks", chunkCount))

	m.chunks = make([]*Chunk, chunkCount)

	// read the chunks
	for i := int16(0); i < chunkCount; i++ {
		var chunkPos [3]int32
		binary.Read(gzipReader, binary.LittleEndian, &chunkPos[0])
		binary.Read(gzipReader, binary.LittleEndian, &chunkPos[1])
		binary.Read(gzipReader, binary.LittleEndian, &chunkPos[2])
		m.logVoxelInfo(fmt.Sprintf("[Map] Loading chunk %d %d %d", chunkPos[0], chunkPos[1], chunkPos[2]))
		chunk := NewChunk(m, chunkPos[0], chunkPos[1], chunkPos[2])
		m.chunks[i] = chunk
		for j := int32(0); j < m.ChunkSizeCube; j++ {
			blockIsSolid := uint8(0)
			binary.Read(gzipReader, binary.LittleEndian, &blockIsSolid)
			if blockIsSolid == 1 {
				sideTextures := make(map[CubeSide]uint16)
				for cs := CubeSide(0); cs < 6; cs++ {
					var texIndex uint16
					binary.Read(gzipReader, binary.LittleEndian, &texIndex)
					sideTextures[cs] = texIndex
				}
				chunk.data[j] = NewBlockWithSides(sideTextures)
			} else {
				chunk.data[j] = NewAirBlock()
			}
		}
		chunk.SetDirty()
	}

	m.textureBufferIsDirty = true
}

func (m *Map) SetBlockSide(blockPos Int3, side CubeSide, index uint16) {
	block := m.GetBlockFromInt3(blockPos)
	if block == nil || block.GetTexture(side) == index || block.IsAir() {
		return
	}
	block.Textures[side] = uint16(index)
	blockIndex := (m.getBlockIndex(blockPos.X, blockPos.Y, blockPos.Z) * 6) + int32(side)
	m.lookupVoxels.SetPixels(int(blockIndex), []uint16{m.packFaceLookup(index, blockPos, int(side))})
}

func (m *Map) getBlockIndex(x, y, z int32) int32 {
	mapSizeX, mapSizeY, _ := m.VoxelDimensions()
	return x + y*mapSizeX + z*mapSizeX*mapSizeY
}

func (m *Map) SetBlockSideFill(blockPos Int3, side CubeSide, index uint16) {
	var textureToReplace uint16

	block := m.GetBlockFromInt3(blockPos)
	textureToReplace = block.GetTexture(side)
	block.Textures[side] = uint16(index)
	m.textureBufferIsDirty = true

	exposedSideOffset := side.AsDirection()
	var neighborsToCheck []Int3

	switch side {
	case Bottom:
		fallthrough
	case Top:
		neighborsToCheck = append(neighborsToCheck,
			Int3{1, 0, 0},
			Int3{-1, 0, 0},
			Int3{0, 0, 1},
			Int3{0, 0, -1},
		)
	case South:
		fallthrough
	case North:
		neighborsToCheck = append(neighborsToCheck,
			Int3{1, 0, 0},
			Int3{-1, 0, 0},
			Int3{0, 1, 0},
			Int3{0, -1, 0},
		)
	case East:
		fallthrough
	case West:
		neighborsToCheck = append(neighborsToCheck,
			Int3{0, 1, 0},
			Int3{0, -1, 0},
			Int3{0, 0, 1},
			Int3{0, 0, -1},
		)
	}

	openPositons := make(map[Int3]bool)
	closedPositions := make(map[Int3]bool)

	pop := func() Int3 {
		for k := range openPositons {
			delete(openPositons, k)
			return k
		}
		return Int3{}
	}

	push := func(pos Int3) {
		if _, exists := closedPositions[pos]; exists {
			return
		}
		openPositons[pos] = true
	}

	push(blockPos)

	for len(openPositons) > 0 {
		currentPos := pop()
		closedPositions[currentPos] = true

		for _, offset := range neighborsToCheck {
			neighbor := currentPos.Add(offset)
			neighborBlock := m.GetBlockFromInt3(neighbor)

			if neighborBlock.IsAir() || neighborBlock.GetTexture(side) != textureToReplace {
				continue
			}

			exposedSide := neighbor.Add(exposedSideOffset)
			if m.IsSolidBlockAt(exposedSide.X, exposedSide.Y, exposedSide.Z) {
				continue
			}

			neighborBlock.Textures[side] = uint16(index)
			push(neighbor)
		}
	}
}

// UsedTextureIndices is every atlas slot the map's blocks actually point at,
// ascending and without duplicates. Baking an atlas for this map means baking
// exactly these, so the answer has to come from the blocks rather than from the
// atlas - the atlas is the superset the editor accumulated, this is the subset
// that survived.
//
// A solid block with a side missing from its map still draws that side, with
// slot 0 (see Block.GetTexture), so a missing side counts as a use of 0.
func (m *Map) UsedTextureIndices() []uint16 {
	seen := make(map[uint16]bool)
	for _, chunk := range m.chunks {
		if chunk == nil {
			continue
		}
		for _, block := range chunk.data {
			if block == nil || !block.IsSolid {
				continue
			}
			for cs := CubeSide(0); cs < 6; cs++ {
				seen[block.GetTexture(cs)] = true
			}
		}
	}
	used := make([]uint16, 0, len(seen))
	for index := range seen {
		used = append(used, index)
	}
	slices.Sort(used)
	return used
}

// RemapTextures moves every block onto new atlas slots, for after the atlas has
// been baked down to the tiles the map still uses. Every side is written back
// explicitly, including the ones that were only implicitly slot 0, so the block
// says what it draws instead of relying on the same default twice.
//
// The emissive set is keyed by texture too, and is moved with them - a panel that
// glowed before the bake has to still glow after it. Only the lookup buffer needs
// rebuilding: meshes carry no texture (see VoxelFace.EqualForMerge).
func (m *Map) RemapTextures(remap map[uint16]uint16) {
	for _, chunk := range m.chunks {
		if chunk == nil {
			continue
		}
		for _, block := range chunk.data {
			if block == nil || !block.IsSolid {
				continue
			}
			if block.Textures == nil {
				block.Textures = make(map[CubeSide]uint16, 6)
			}
			for cs := CubeSide(0); cs < 6; cs++ {
				block.Textures[cs] = remap[block.GetTexture(cs)]
			}
		}
	}
	if len(m.emissiveTextures) > 0 {
		moved := make(map[uint16]bool, len(m.emissiveTextures))
		for old := range m.emissiveTextures {
			if now, stillUsed := remap[old]; stillUsed {
				moved[now] = true
			}
		}
		m.emissiveTextures = moved
	}
	m.textureBufferIsDirty = true
}
