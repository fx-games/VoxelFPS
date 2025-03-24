package core

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/geometry"
    "image/color"
    "math"
)

type MapLoadFunc func(filename string) MapLoadResult
type MapLoadResult struct {
    TileSize int
    // AtlasFilename leave empty to use the embedded texture
    AtlasFilename     string
    PlayerSpawn       mgl32.Vec3
    PlayerDir         mgl32.Vec2
    CeilingHeight     int
    IsFloorTextured   bool
    IsCeilingTextured bool
    CeilingColor      color.Color
    FloorColor        color.Color
    MapDimensions     [3]int
    MapData           []TileInfo
}

func (m MapLoadResult) CeilingColorVec4() mgl32.Vec4 {
    ceilingR, ceilingG, ceilingB, _ := m.CeilingColor.RGBA()
    return mgl32.Vec4{float32(ceilingR) / 0xffff, float32(ceilingG) / 0xffff, float32(ceilingB) / 0xffff, 1}
}

type MapEntity struct {
    Name                string
    TextureIndices      []uint16
    MeshName            string
    SpriteSheetName     string
    SpriteSheetTileSize geometry.Point
    IsTransparent       bool
    CreateCollider      bool
}

type TileInfo struct {
    WallTex  int
    FloorTex int
    CeilTex  int

    WallOffset    int
    WallHeight    int
    Entities      []MapEntity
    CeilingHeight int
    FloorOffset   int
}

func (t TileInfo) isWall() bool {
    return t.WallHeight > 0
}

func LineToPlaneIntersection(lineStart, lineEnd, planeCoord, planeNormal mgl32.Vec3) float32 {
    NdotU := planeNormal.Dot(lineEnd)
    if NdotU == 0 {
        return math.MaxFloat32
    }
    return planeNormal.Dot(planeCoord.Sub(lineStart)) / NdotU
}
