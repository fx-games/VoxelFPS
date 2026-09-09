package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads/voxel"
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
	// to its tile centre - see assets/meshes/door_swing_frame's jamb positions and
	// assets/meshes/door_swing_leaf/mesh.glb's hinge-at-origin modelling.
	doorSwingHingeOffset = -0.375
)

func loadDoorTestMap() (core.MapLoadResult, *voxel.Map) {
	sizeX, sizeZ := doorMapSizeX, doorMapSizeZ
	blocks := newLevel(voxel.Int3{X: int32(sizeX), Y: doorMapHeight + 1, Z: int32(sizeZ)})
	block := roomBlock(0, 2, 3)
	paintRoom(blocks, block)

	// The wall that splits the map in two, with a gap left in it for each door.
	isDoorGap := func(x int) bool {
		return x == doorFrontX || x == doorMidX || x == doorBackX || x == doorSwingX
	}
	for x := int32(1); x < int32(sizeX)-1; x++ {
		if isDoorGap(int(x)) {
			continue
		}
		fillBlocks(blocks,
			voxel.Int3{X: x, Y: groundLevel, Z: doorMapWallZ},
			voxel.Int3{X: x, Y: doorMapHeight - 1, Z: doorMapWallZ},
			block)
	}

	// A door plane is a full voxel wide and tall but a sixteenth as deep, so
	// where in the depth of its opening it sits is the thing being tested:
	// against the front face, in the middle, against the back.
	slideDoor := func(x int, depthOffset float32) core.EntitySpawn {
		return core.EntitySpawn{
			Position: tileCentre(x, doorMapWallZ, groundLevel).Add(mgl32.Vec3{0, 0, depthOffset}),
			Entity: core.MapEntity{
				Name:           "SlideDoor",
				MeshName:       doorSlideMeshName,
				CreateCollider: true,
			},
		}
	}

	entities := []core.EntitySpawn{
		slideDoor(doorFrontX, -0.5+doorHalfThickness),
		slideDoor(doorMidX, 0),
		slideDoor(doorBackX, 0.5-doorHalfThickness),
		{
			Position: tileCentre(doorSwingX, doorMapWallZ, groundLevel),
			Entity: core.MapEntity{
				Name:           "DoorFrame",
				MeshName:       "door_swing_frame",
				CreateCollider: true,
			},
		},
		{
			Position: tileCentre(doorSwingX, doorMapWallZ, groundLevel).Add(mgl32.Vec3{doorSwingHingeOffset, 0, 0}),
			Entity: core.MapEntity{
				Name:           "DoorLeaf",
				MeshName:       doorSwingLeafMeshName,
				CreateCollider: true,
			},
		},
	}

	// A Scientist patrolling straight across the swing doorway, spawned on
	// the far side, and back - proof a closed door blocks a pathing NPC's
	// physics exactly the way it blocks the player, not just something to
	// take on faith from watching the player alone.
	patrolNear := mgl32.Vec3{float32(doorSwingX) + 0.5, 1, float32(doorMapWallZ) - 2.5}
	entities = append(entities, core.EntitySpawn{
		Position: tileCentre(doorSwingX, doorMapWallZ+3, groundLevel),
		Entity: core.MapEntity{
			Name:                "Scientist",
			TextureIndices:      []uint16{0, 1, 2, 3, 4, 5, 6, 7},
			SpriteSheetName:     "scientist",
			SpriteSheetTileSize: geometry.Point{X: 64, Y: 64},
			CreateCollider:      true,
			PatrolTo:            &patrolNear,
		},
	})

	return core.MapLoadResult{
		PlayerSpawn:  mgl32.Vec3{float32(doorMapSizeX) / 2, 1, 2.5},
		PlayerDir:    mgl32.Vec2{0, 1},
		CeilingColor: color.RGBA{60, 60, 70, 255},
		FloorColor:   color.RGBA{40, 40, 40, 255},
		Entities:     entities,
		Lights:       doorTestLights(),
	}, blocks
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
