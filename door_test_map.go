package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
)

// The doors test map: two rooms split by one wall, four doorways cut into it.
// Three are the slide door, one at each depth a door plane can be authored
// at within its tile (front, middle, back - see core.MapEntity.LocalOffset);
// the fourth is the swing door, frame and leaf both. Walk up to any of them
// and press E.
const (
	doorMapSizeX  = 20
	doorMapSizeZ  = 16
	doorMapWallZ  = 8
	doorMapHeight = 4

	doorFrontX = 4
	doorMidX   = 8
	doorBackX  = 12
	doorSwingX = 16

	// doorHalfThickness is half of the door plane's own depth (a sixteenth of
	// a voxel), so a "front" or "back" placement sits flush with its tile's
	// near or far face instead of poking out past it.
	doorHalfThickness = 1.0 / 32.0

	// doorSwingHingeOffset is the swing frame's opening's left edge, relative
	// to its tile centre - see scenery/door_swing_frame's jamb positions and
	// assets/meshes/door_swing_leaf.glb's hinge-at-origin modelling.
	doorSwingHingeOffset = -0.375
)

func loadDoorTestMap() core.MapLoadResult {
	sizeX, sizeZ := doorMapSizeX, doorMapSizeZ
	mapData := make([]core.TileInfo, sizeX*sizeZ)

	isDoorGap := func(x, z int) bool {
		return z == doorMapWallZ && (x == doorFrontX || x == doorMidX || x == doorBackX || x == doorSwingX)
	}

	for z := 0; z < sizeZ; z++ {
		for x := 0; x < sizeX; x++ {
			tile := &mapData[z*sizeX+x]
			tile.FloorTex = 2
			tile.CeilTex = 3
			tile.CeilingHeight = doorMapHeight

			onBorder := x == 0 || z == 0 || x == sizeX-1 || z == sizeZ-1
			onWallRow := z == doorMapWallZ
			if (onBorder || onWallRow) && !isDoorGap(x, z) {
				tile.WallHeight = doorMapHeight
			}
		}
	}

	slideDoor := func(depthOffset float32) core.MapEntity {
		return core.MapEntity{
			Name:           "SlideDoor",
			MeshName:       doorSlideMeshName,
			CreateCollider: true,
			LocalOffset:    mgl32.Vec3{0, 0, depthOffset},
		}
	}
	tileAt := func(x, z int) *core.TileInfo { return &mapData[z*sizeX+x] }

	front := tileAt(doorFrontX, doorMapWallZ)
	front.Entities = append(front.Entities, slideDoor(-0.5+doorHalfThickness))

	mid := tileAt(doorMidX, doorMapWallZ)
	mid.Entities = append(mid.Entities, slideDoor(0))

	back := tileAt(doorBackX, doorMapWallZ)
	back.Entities = append(back.Entities, slideDoor(0.5-doorHalfThickness))

	swing := tileAt(doorSwingX, doorMapWallZ)
	swing.Entities = append(swing.Entities,
		core.MapEntity{
			Name:           "DoorFrame",
			MeshName:       "door_swing_frame",
			CreateCollider: true,
		},
		core.MapEntity{
			Name:           "DoorLeaf",
			MeshName:       doorSwingLeafMeshName,
			CreateCollider: true,
			LocalOffset:    mgl32.Vec3{doorSwingHingeOffset, 0, 0},
		},
	)

	// A Scientist patrolling straight across the swing doorway, spawned on
	// the far side, and back - proof a closed door blocks a pathing NPC's
	// physics exactly the way it blocks the player, not just something to
	// take on faith from watching the player alone.
	patrolNear := mgl32.Vec3{float32(doorSwingX) + 0.5, 1, float32(doorMapWallZ) - 2.5}
	farTile := tileAt(doorSwingX, doorMapWallZ+3)
	farTile.Entities = append(farTile.Entities, core.MapEntity{
		Name:                "Scientist",
		TextureIndices:      []uint16{0, 1, 2, 3, 4, 5, 6, 7},
		SpriteSheetName:     "scientist",
		SpriteSheetTileSize: geometry.Point{X: 64, Y: 64},
		CreateCollider:      true,
		PatrolTo:            &patrolNear,
	})

	return core.MapLoadResult{
		TileSize:          64,
		AtlasFilename:     "_internal",
		PlayerSpawn:       mgl32.Vec3{float32(doorMapSizeX) / 2, 1, 2.5},
		PlayerDir:         mgl32.Vec2{0, 1},
		CeilingHeight:     doorMapHeight,
		IsFloorTextured:   true,
		IsCeilingTextured: true,
		CeilingColor:      color.RGBA{60, 60, 70, 255},
		FloorColor:        color.RGBA{40, 40, 40, 255},
		MapDimensions:     [3]int{sizeX, doorMapHeight + 1, sizeZ},
		MapData:           mapData,
		Lights:            doorTestLights(),
	}
}

// One light over each doorway, aimed down at it, plus one in each room so
// neither side is left dark. Same shape as stairsTestLights - see stairs.go.
func doorTestLights() []core.MapLight {
	overDoor := func(x int) core.MapLight {
		pos := mgl32.Vec3{float32(x) + 0.5, float32(doorMapHeight) - 0.5, float32(doorMapWallZ)}
		return core.MapLight{
			Position: pos, Color: mgl32.Vec3{1, 0.95, 0.85}, Intensity: 6, Angle: 100, Reach: 10,
		}.LookAt(mgl32.Vec3{float32(x) + 0.5, 1, float32(doorMapWallZ)})
	}
	roomLight := func(z int) core.MapLight {
		pos := mgl32.Vec3{float32(doorMapSizeX) / 2, float32(doorMapHeight) - 0.5, float32(z)}
		return core.MapLight{
			Position: pos, Color: mgl32.Vec3{0.9, 0.92, 1}, Intensity: 7, Angle: 140, Reach: 18,
		}.LookAt(mgl32.Vec3{float32(doorMapSizeX) / 2, 0, float32(z)})
	}
	return []core.MapLight{
		overDoor(doorFrontX), overDoor(doorMidX), overDoor(doorBackX), overDoor(doorSwingX),
		roomLight(3), roomLight(doorMapSizeZ - 4),
	}
}
