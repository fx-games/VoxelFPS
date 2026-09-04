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
	locationTex           int32
	locationVox           int32
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

				lookupTable[blockWallsStartIndex+0] = m.withEmissiveBit(block.ZYTextureIndexPos())
				lookupTable[blockWallsStartIndex+1] = m.withEmissiveBit(block.ZYTextureIndexNeg())

				lookupTable[blockWallsStartIndex+2] = m.withEmissiveBit(block.XZTextureIndexPos())
				lookupTable[blockWallsStartIndex+3] = m.withEmissiveBit(block.XZTextureIndexNeg())

				lookupTable[blockWallsStartIndex+4] = m.withEmissiveBit(block.XYTextureIndexPos())
				lookupTable[blockWallsStartIndex+5] = m.withEmissiveBit(block.XYTextureIndexNeg())
			}
		}
	}
	if m.lookupVoxels != nil || m.lookupVoxels.Size() != int(texSize) {
		m.lookupVoxels = glhf.NewTextureBuffer(lookupTable)
	} else {
		m.lookupVoxels.SetPixels(0, lookupTable)
	}
}

// EmissiveTextureBit rides in the top bit of each entry in the voxel lookup buffer.
// Texture indices are a few hundred at most and the entries are uint16, so the bit was
// free - no new buffer, no vertex format change, and emissive stays per face, which is
// the resolution greedy meshing left us with anyway.
const EmissiveTextureBit = uint16(0x8000)

// EmissiveTextureMask is what is left for the texture index itself.
const EmissiveTextureMask = uint16(0x7FFF)

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
	m.bakeEmissiveTorchlight()
	return nowEmissive
}

// SetEmissiveTextures replaces the whole set, for a map loader to call.
func (m *Map) SetEmissiveTextures(textureIndices []uint16) {
	m.emissiveTextures = make(map[uint16]bool, len(textureIndices))
	for _, index := range textureIndices {
		m.emissiveTextures[index] = true
	}
	m.textureBufferIsDirty = true
	m.bakeEmissiveTorchlight()
}

func (m *Map) EmissiveTextureCount() int {
	return len(m.emissiveTextures)
}

func (m *Map) Draw(cam core.Camera) {
	chunkShader := m.chunkShader
	m.DrawWithShader(cam, chunkShader)
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

func (m *Map) DrawWithShader(frustum core.Culler, chunkShader *glhf.Shader) {
	m.remeshDirtyChunks()
	if m.textureBufferIsDirty {
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
	for _, chunk := range m.chunks {
		if chunk == nil || !m.chunkIsVisible(frustum, chunk) || chunk.chunkPosY > m.maxChunkHeightForDraw {
			continue
		}
		m.trianglesDrawn += chunk.meshBuffer.TriangleCount()
		m.drawCallCount += 1
		chunk.Draw(chunkShader, m.modelIndex)
	}

	m.lookupVoxels.End()
	m.terrainTexture.End()

	chunkShader.End()

	gl.ActiveTexture(gl.TEXTURE0)
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

func (m *Map) ContainsVec(pos mgl32.Vec3) bool {
	x, y, z := pos.X(), pos.Y(), pos.Z()
	return m.Contains(int32(x), int32(y), int32(z))
}

func (m *Map) Contains(x int32, y int32, z int32) bool {
	return x >= 0 && x < m.width*m.ChunkSizeHorizontal && y >= 0 && y < m.height*m.ChunkSizeHeight && z >= 0 && z < m.depth*m.ChunkSizeHorizontal
}

func (m *Map) GetBlockFromInt3(pos Int3) *Block {
	return m.GetGlobalBlock(int32(pos.X), int32(pos.Y), int32(pos.Z))
}
func (m *Map) GetGlobalBlock(x int32, y int32, z int32) *Block {
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
// and stopping at solid blocks. Blocks it touches are dirty, so the next mesh pass
// bakes the new levels into the vertices.
func (m *Map) FillTorchlight(origin Int3) {
	m.fillTorchlightFrom(origin, MaxTorchLight)
}

func (m *Map) fillTorchlightFrom(origin Int3, level byte) {
	if !m.ContainsGrid(origin) || level == 0 {
		return
	}

	startBlock, startChunk := m.GetBlockAndChunk(origin)
	if startBlock == nil || !startBlock.IsAir() {
		return // light lives in the air; the mesher only reads it off air blocks
	}
	if startBlock.GetTorchLight() >= level {
		return // already at least this bright from another emitter
	}
	startBlock.SetTorchLight(level)
	startChunk.SetDirty()

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

		neighborBlock, neighborChunk := m.GetBlockAndChunk(currentNeighbor)
		if neighborBlock == nil {
			return false, 0
		}

		if neighborBlock.GetTorchLight() < nodeLight-1 {
			neighborBlock.SetTorchLight(nodeLight - 1)
			neighborChunk.SetDirty()
			return true, costToPrev + 1
		}
		return false, 0
	}

	m.FloodFill(origin, int(level), spread)
}

// emissiveFaceOffsets is the block a face of the given index faces into, in the
// face order bake3dTextureIndices writes: +X, -X, +Y, -Y, +Z, -Z.
var emissiveFaceOffsets = [6]Int3{
	{1, 0, 0}, {-1, 0, 0},
	{0, 1, 0}, {0, -1, 0},
	{0, 0, 1}, {0, 0, -1},
}

// bakeEmissiveTorchlight turns the emissive texture set into per-block light: every
// face using one of those textures seeds the air in front of it, and the light
// floods from there. It is the cheap half of the lighting - dozens of static
// sources for the cost of four bits a vertex - and it is what the shadow-mapped
// spotlights cannot afford to be.
//
// Called from the two places that change the emissive set, so the baked light and
// the glowing faces cannot drift apart.
//
// ponytail: one flood fill per emitting face over the whole map. Walls stop the
// fill after a few metres, so on a corridor map this is milliseconds. If a big
// open map makes a map load or a G keypress hitch, the fix is one multi-source
// BFS seeded with every emitter, not a goroutine.
func (m *Map) bakeEmissiveTorchlight() {
	m.clearTorchlight()
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
				if chunk := m.GetChunkFromBlock(x, y, z); chunk != nil {
					chunk.SetDirty()
				}
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
			NewAABB(chunk.AsAABB()).Draw(lineShader)
		}
	}
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
	m.lookupVoxels.SetPixels(int(blockIndex), []uint16{uint16(index)})
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
