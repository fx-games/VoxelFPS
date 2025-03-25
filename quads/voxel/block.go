package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "maps"
)

type CollidingObject interface {
    Position() mgl32.Vec3
    SetPosition(pos mgl32.Vec3)
    Velocity() mgl32.Vec3
    Acceleration() mgl32.Vec3
    SetVelocity(newVelocity mgl32.Vec3)
    IsDead(elapsed float64) bool
    AffectedByGravity() bool
    AABBAt(pos mgl32.Vec3) AABB
    AABB() AABB
    Transform(cam *core.FPSCamera) mgl32.Mat4
    IsMovable() bool
    IsProjectile() bool
}
type Block struct {
    IsSolid    bool
    Textures   map[CubeSide]uint16
    lightLevel byte
}

func (b *Block) SetTorchLight(lightLevel byte) {
    b.lightLevel = (b.lightLevel & 0xF0) | lightLevel
}

func (b *Block) GetTorchLight() byte {
    if b == nil {
        return 0
    }
    return b.lightLevel & 0xF
}
func (b *Block) SetSunLight(lightLevel byte) {
    b.lightLevel = (b.lightLevel & 0xF) | (lightLevel << 4)
}

func (b *Block) GetSunLight() byte {
    if b == nil {
        return 0
    }
    return b.lightLevel >> 4
}

func (b *Block) IsAir() bool {
    return !b.IsSolid
}

func (b *Block) RemoveUnit(unit MapObject) {

}

func (b *Block) AddUnit(unit MapObject) {

}

func (b *Block) GetTexture(side CubeSide) uint16 {
    text, ok := b.Textures[side]
    if !ok {
        return 0
    }
    return text
}

func (b *Block) Copy() *Block {
    return &Block{
        IsSolid:    b.IsSolid,
        Textures:   maps.Clone(b.Textures),
        lightLevel: b.lightLevel,
    }
}

/*

func (s CubeSide) AsDirection() Int3 {
    switch s {
    case North:
        return Int3{0, 0, -1}
    case East:
        return Int3{1, 0, 0}
    case South:
        return Int3{0, 0, 1}
    case West:
        return Int3{-1, 0, 0}
    case Top:
        return Int3{0, 1, 0}
    case Bottom:
        return Int3{0, -1, 0}
    }
    return Int3{}
}

*/
func (b *Block) XYTextureIndexNeg() uint16 {
    return b.Textures[North]
}

func (b *Block) XYTextureIndexPos() uint16 {
    return b.Textures[South]
}

func (b *Block) ZYTextureIndexNeg() uint16 {
    return b.Textures[West]
}

func (b *Block) ZYTextureIndexPos() uint16 {
    return b.Textures[East]
}

func (b *Block) XZTextureIndexNeg() uint16 {
    return b.Textures[Bottom]
}

func (b *Block) XZTextureIndexPos() uint16 {
    return b.Textures[Top]
}
