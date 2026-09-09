package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

// The computer test map: one room, one Computer prop against the far wall,
// the player spawned facing its screen - proof the screen material actually
// plays its flipbook, not just that the prop loads. `-shot` a few frames
// apart and diff them to see it advance without touching a keyboard.
const (
	computerMapSize   = 10
	computerMapHeight = 4
)

func loadComputerTestMap() (core.MapLoadResult, *voxel.Map) {
	size := computerMapSize
	blocks := newLevel(voxel.Int3{X: int32(size), Y: computerMapHeight + 1, Z: int32(size)})
	paintRoom(blocks, roomBlock(0, 2, 3))

	entities := []core.EntitySpawn{{
		Position: tileCentre(size/2, size-2, groundLevel),
		Entity: core.MapEntity{
			Name:           "Computer",
			MeshName:       "computer",
			CreateCollider: true,
		},
	}}

	light := core.MapLight{
		Position:  mgl32.Vec3{float32(size) / 2, float32(computerMapHeight) - 0.5, float32(size) / 2},
		Color:     mgl32.Vec3{0.9, 0.92, 1},
		Intensity: 7, Angle: 140, Reach: 14,
	}.LookAt(mgl32.Vec3{float32(size) / 2, 0, float32(size) / 2})

	return core.MapLoadResult{
		PlayerSpawn:  mgl32.Vec3{float32(size) / 2, 1, float32(size) - 4.5},
		PlayerDir:    mgl32.Vec2{0, 1},
		CeilingColor: color.RGBA{60, 60, 70, 255},
		FloorColor:   color.RGBA{40, 40, 40, 255},
		Entities:     entities,
		Lights:       []core.MapLight{light},
	}, blocks
}
