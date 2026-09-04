package core

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/geometry"
    "image/color"
    "math"
)

type MapLoadFunc func(filename string) MapLoadResult
// MapLight is a light the way a map file describes it: authored numbers, no GL and
// no renderer types. quads turns it into a LightInfo on load.
//
// Yaw and Pitch are degrees, and they are here rather than a direction vector
// because a level editor asks a human for angles, not for a unit vector. Yaw 0 with
// Pitch 0 points along -Z, yaw grows towards +X, and positive pitch tilts downwards
// - so a lamp is "yaw whatever, pitch 40" rather than three components that have to
// be normalised by hand.
type MapLight struct {
    // Position is in world units, not tiles. Y is height above the map floor.
    Position mgl32.Vec3
    Yaw      float32
    Pitch    float32

    Color     mgl32.Vec3
    Intensity float32
    // Angle is the full cone angle in degrees, Reach how far the light carries. Reach
    // doubles as the far plane of its shadow frustum, so a light cannot light what it
    // has no depth information for.
    Angle float32
    Reach float32
}

// LookAt points the light at a world position, expressed in the Yaw/Pitch the map
// format stores. Generated maps know what they want lit rather than what angle that
// works out to, and hand-authored ones get the angles from the editor either way.
func (l MapLight) LookAt(target mgl32.Vec3) MapLight {
    direction := target.Sub(l.Position)
    horizontal := mgl32.Vec2{direction.X(), direction.Z()}.Len()
    l.Yaw = mgl32.RadToDeg(float32(math.Atan2(float64(direction.X()), float64(-direction.Z()))))
    l.Pitch = mgl32.RadToDeg(float32(math.Atan2(float64(-direction.Y()), float64(horizontal))))
    return l
}

// Forward is the unit vector the light points along. See the Yaw/Pitch note above.
func (l MapLight) Forward() mgl32.Vec3 {
    yaw := float64(mgl32.DegToRad(l.Yaw))
    pitch := float64(mgl32.DegToRad(l.Pitch))
    cosPitch := float32(math.Cos(pitch))
    return mgl32.Vec3{
        float32(math.Sin(yaw)) * cosPitch,
        -float32(math.Sin(pitch)),
        -float32(math.Cos(yaw)) * cosPitch,
    }.Normalize()
}

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
    // Lights the map brings with it. Loading a map replaces the scene's lights with
    // these, so an empty list means an unlit map rather than the previous map's
    // lights left standing.
    Lights []MapLight
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

    // PatrolTo is the far end of a walking entity's beat. It walks there, then back
    // to where it spawned, forever. Nil means the spawner picks a default.
    PatrolTo *mgl32.Vec3
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
