package voxel

import (
    "fmt"
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
    "math"
)

type Map struct {
    ChunkSizeHorizontal int32
    ChunkSizeHeight     int32
    ChunkSizeCube       int32

    chunks          []*Chunk
    width           int32
    height          int32
    depth           int32
    chunkShader     *glhf.Shader
    terrainTexture  *glhf.Texture
    mapInfoLogger   func(string)
    gameErrorLogger func(string)

    spawnCounter          int
    maxChunkHeightForDraw int32
    trianglesDrawn        int
    verticesUploaded      bool
    modelIndex            int
    drawCallCount         int
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

func (m *Map) ForceUpload() {
    for _, chunk := range m.chunks {
        if chunk != nil {
            chunk.isDirty = true
            chunk.GenerateMesh()
        }
    }
    m.verticesUploaded = true
    mapSizeX := m.width * m.ChunkSizeHorizontal
    mapSizeY := m.height * m.ChunkSizeHeight
    mapSizeZ := m.depth * m.ChunkSizeHorizontal
    println(fmt.Sprintf("[Greedy] Map with %d cubes volume (%d, %d, %d) meshed into %d triangles", mapSizeX*mapSizeY*mapSizeZ, mapSizeX, mapSizeY, mapSizeZ, m.TriangleCount()))

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
func (m *Map) Draw(cam core.Camera) {

    if !m.verticesUploaded {
        m.ForceUpload()

    }
    m.chunkShader.Begin()

    m.chunkShader.SetTextureUnit("tex", gl.TEXTURE0)
    m.chunkShader.SetTextureUnit("voxelTexLookup", gl.TEXTURE1)

    m.terrainTexture.BeginOnUnit(gl.TEXTURE0)

    m.trianglesDrawn = 0
    m.drawCallCount = 0
    for _, chunk := range m.chunks {
        if chunk == nil || !m.chunkIsVisible(cam, chunk) || chunk.chunkPosY > m.maxChunkHeightForDraw {
            continue
        }
        m.trianglesDrawn += chunk.meshBuffer.TriangleCount()
        m.drawCallCount += 1
        chunk.Draw(m.chunkShader, m.modelIndex)

    }
    m.terrainTexture.End()

    m.chunkShader.End()
}

func (m *Map) chunkIsVisible(cam core.Camera, chunk *Chunk) bool {
    if chunk.ContainsWorldPosition(cam.GetPosition()) {
        return true
    }
    // TODO: THIS is very INACCURATE
    return cam.IsSphereInViewFrustum(chunk.AsSphere())
    // TODO: THIS DOES NOT WORK
    return cam.IsCubeInViewFrustum(chunk.AsAABB())
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
    chunkX := x / m.ChunkSizeHorizontal
    chunkY := y / m.ChunkSizeHeight
    chunkZ := z / m.ChunkSizeHorizontal
    chunk := m.GetChunk(chunkX, chunkY, chunkZ)
    if chunk != nil {
        chunk.SetBlock(x%m.ChunkSizeHorizontal, y%m.ChunkSizeHeight, z%m.ChunkSizeHorizontal, block)
    }
}

func (m *Map) SetAir(blockPos Int3) {
    m.SetBlock(blockPos.X, blockPos.Y, blockPos.Z, NewAirBlock())
}

func (m *Map) ContainsVec(pos mgl32.Vec3) bool {
    x, y, z := pos.X(), pos.Y(), pos.Z()
    return m.Contains(int32(x), int32(y), int32(z))
}

func (m *Map) Contains(x int32, y int32, z int32) bool {
    return x >= 0 && x < m.width*m.ChunkSizeHorizontal && y >= 0 && y < m.height*m.ChunkSizeHeight && z >= 0 && z < m.depth*m.ChunkSizeHorizontal
}

func (m *Map) GetBlockFromVec(pos Int3) *Block {
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

func (m *Map) SetTerrainTexture(texture *glhf.Texture) {
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

func (m *Map) GetTerrainTexture() *glhf.Texture {
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

func (m *Map) FillTorchlight(origin Int3) {
    if !m.ContainsGrid(origin) {
        return
    }
    maxLightLevel := 15

    startBlock, startChunk := m.GetBlockAndChunk(origin)
    startBlock.SetTorchLight(byte(maxLightLevel))
    startChunk.SetDirty()

    spread := func(origin, currentNode, currentNeighbor Int3, costToPrev int) (bool, int) {
        if m.IsSolidBlockAt(currentNeighbor.X, currentNeighbor.Y, currentNeighbor.Z) {
            return false, 0
        }

        nodeLight := m.GetBlockFromVec(currentNode).GetTorchLight()

        neighborBlock, neighborChunk := m.GetBlockAndChunk(currentNeighbor)
        neighborLight := neighborBlock.GetTorchLight()

        if neighborLight < nodeLight-1 {
            neighborBlock.SetTorchLight(nodeLight - 1)
            neighborChunk.SetDirty()
            println(fmt.Sprintf("[Map] Spread torchlight from %s to %s: %d -> %d", currentNode.ToString(), currentNeighbor.ToString(), nodeLight, nodeLight-1))
            return true, costToPrev + 1
        }
        return false, 0
    }

    m.FloodFill(origin, maxLightLevel, spread)
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
