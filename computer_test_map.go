package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
)

// The computer test map: one room, one Computer prop against the far wall,
// the player spawned facing its screen - proof the screen material actually
// plays its flipbook, not just that the prop loads. `-shot` a few frames
// apart and diff them to see it advance without touching a keyboard.
const (
	computerMapSize   = 10
	computerMapHeight = 4
)

func loadComputerTestMap() core.MapLoadResult {
	size := computerMapSize
	mapData := make([]core.TileInfo, size*size)

	for z := 0; z < size; z++ {
		for x := 0; x < size; x++ {
			tile := &mapData[z*size+x]
			tile.FloorTex = 2
			tile.CeilTex = 3
			tile.CeilingHeight = computerMapHeight
			if x == 0 || z == 0 || x == size-1 || z == size-1 {
				tile.WallHeight = computerMapHeight
			}
		}
	}

	against := &mapData[(size-2)*size+size/2]
	against.Entities = append(against.Entities, core.MapEntity{
		Name:           "Computer",
		MeshName:       "computer",
		CreateCollider: true,
	})

	light := core.MapLight{
		Position:  mgl32.Vec3{float32(size) / 2, float32(computerMapHeight) - 0.5, float32(size) / 2},
		Color:     mgl32.Vec3{0.9, 0.92, 1},
		Intensity: 7, Angle: 140, Reach: 14,
	}.LookAt(mgl32.Vec3{float32(size) / 2, 0, float32(size) / 2})

	return core.MapLoadResult{
		TileSize:          64,
		AtlasFilename:     "_internal",
		PlayerSpawn:       mgl32.Vec3{float32(size) / 2, 1, float32(size) - 4.5},
		PlayerDir:         mgl32.Vec2{0, 1},
		CeilingHeight:     computerMapHeight,
		IsFloorTextured:   true,
		IsCeilingTextured: true,
		CeilingColor:      color.RGBA{60, 60, 70, 255},
		FloorColor:        color.RGBA{40, 40, 40, 255},
		MapDimensions:     [3]int{size, computerMapHeight + 1, size},
		MapData:           mapData,
		Lights:            []core.MapLight{light},
	}
}
