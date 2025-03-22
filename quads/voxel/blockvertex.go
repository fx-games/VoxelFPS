package voxel

var NorthDir = Int3{0, 0, -1}
var SouthDir = Int3{0, 0, 1}
var EastDir = Int3{1, 0, 0}
var WestDir = Int3{-1, 0, 0}
var Up = Int3{0, 1, 0}
var Down = Int3{0, -1, 0}

func NewBlock(id byte) *Block {
    return &Block{
        IsSolid: id != 0,
        Textures: map[CubeSide]uint16{
            Top:    uint16(id),
            Bottom: uint16(id),
            North:  uint16(id),
            South:  uint16(id),
            East:   uint16(id),
            West:   uint16(id),
        },
    }
}
func NewBlockWithSides(textures map[CubeSide]uint16) *Block {
    return &Block{
        IsSolid:  true,
        Textures: textures,
    }
}
func NewAirBlock() *Block {
    return &Block{
        IsSolid: false,
    }
}

// CalculateCornerUVsTerrain returns the UV coordinates for the corners of a tile in a texture atlas
// in this order: top left, top right, bottom right, bottom left
func CalculateCornerUVsTerrain(tileSize, atlasWidth, atlasHeight int, textureIndex int) [4][2]float32 {
    // we need the width height of the whole atlas in pixels
    // we need the width height of each tile in pixels
    // we need the texture textureIndex

    result := [4][2]float32{}

    tileCountX := atlasWidth / tileSize
    //tileCountY := atlasHeight / tileSize

    tileX := textureIndex % tileCountX
    tileY := textureIndex / tileCountX

    // top left
    result[0][0] = float32(tileX*tileSize) / float32(atlasWidth)
    result[0][1] = float32(tileY*tileSize) / float32(atlasHeight)

    // top right
    result[1][0] = float32((tileX+1)*tileSize) / float32(atlasWidth)
    result[1][1] = float32(tileY*tileSize) / float32(atlasHeight)

    // bottom right
    result[2][0] = float32((tileX+1)*tileSize) / float32(atlasWidth)
    result[2][1] = float32((tileY+1)*tileSize) / float32(atlasHeight)

    // bottom left
    result[3][0] = float32(tileX*tileSize) / float32(atlasWidth)
    result[3][1] = float32((tileY+1)*tileSize) / float32(atlasHeight)

    return result
}
