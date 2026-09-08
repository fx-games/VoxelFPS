package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
)

// The actor pathfinding test map: one big ground floor, one platform a single
// voxel up (reached by an ordinary staircase - see stairs.go), and one
// platform four voxels up (too tall for the physics step height stairs rely
// on, reached only by a ladder). A hundred Scientist entities patrol it, most
// of them on the ground, a few routed up onto each platform so both the
// stairs and the ladder connection actually get walked.
const (
	actorMapSize    = 40
	actorMapCeiling = 6

	platformAX0, platformAX1 = 4, 12
	platformAZ0, platformAZ1 = 4, 12
	// The staircase mesh is 2 tiles long (tools/genstairs: stepCount * stepRun)
	// and its top step has to end exactly where the platform starts, the same
	// spacing stairs.go uses - one tile short of that and it lands embedded in
	// the platform's own voxel block instead of flush against it.
	stairsAX, stairsAZ = 8, platformAZ0 - 2

	platformBX0, platformBX1 = 27, 34
	platformBZ0, platformBZ1 = 27, 34
	// The ladder stands one tile in front of platform B's cliff face, and
	// lands on the first platform tile right behind it - the shortest possible
	// hop once the climb is done, so there is something to step onto rather
	// than empty air.
	ladderX, ladderZ       = 30, platformBZ0 - 1
	ladderTopX, ladderTopZ = 30, platformBZ0

	platformAHeight = 1 // within climbStep, so ordinary stairs can bridge it
	platformBHeight = 4 // taller than climbStep: a ladder, not a stair, has to bridge it

	// groundSurfaceY is the walkable height above a tile with no WallHeight at
	// all: the map always has one solid floor slab under it (see
	// quads/gl_application.go's toVoxelMap), so a platform's own walkable
	// surface is this plus its WallHeight, not WallHeight alone.
	groundSurfaceY    = 1
	platformASurfaceY = groundSurfaceY + platformAHeight
	platformBSurfaceY = groundSurfaceY + platformBHeight

	actorPopulation = 100
)

func loadActorTestMap() core.MapLoadResult {
	size := actorMapSize
	mapData := make([]core.TileInfo, size*size)

	onPlatformA := func(x, z int) bool {
		return x >= platformAX0 && x <= platformAX1 && z >= platformAZ0 && z <= platformAZ1
	}
	onPlatformB := func(x, z int) bool {
		return x >= platformBX0 && x <= platformBX1 && z >= platformBZ0 && z <= platformBZ1
	}

	for z := 0; z < size; z++ {
		for x := 0; x < size; x++ {
			tile := &mapData[z*size+x]
			tile.FloorTex = 2
			tile.CeilTex = 3
			tile.CeilingHeight = actorMapCeiling

			if x == 0 || z == 0 || x == size-1 || z == size-1 {
				tile.WallHeight = actorMapCeiling
				continue
			}
			if onPlatformA(x, z) {
				tile.WallHeight = platformAHeight
			}
			if onPlatformB(x, z) {
				tile.WallHeight = platformBHeight
			}
		}
	}

	stairsTop := mgl32.Vec3{float32(stairsAX) + 0.5, platformASurfaceY, float32(platformAZ0) + 0.5}
	mapData[stairsAZ*size+stairsAX].Entities = append(mapData[stairsAZ*size+stairsAX].Entities, core.MapEntity{
		Name: "Stairs", MeshName: stairsMeshName, CreateCollider: true, StepTo: &stairsTop,
	})

	landing := mgl32.Vec3{float32(ladderTopX) + 0.5, platformBSurfaceY, float32(ladderTopZ) + 0.5}
	mapData[ladderZ*size+ladderX].Entities = append(mapData[ladderZ*size+ladderX].Entities, core.MapEntity{
		Name: "Ladder", MeshName: "ladder", CreateCollider: true, ClimbTo: &landing,
	})

	isGroundTile := func(x, z int) bool {
		return x > 1 && x < size-2 && z > 1 && z < size-2 &&
			!onPlatformA(x, z) && !onPlatformB(x, z) &&
			!(x == stairsAX && z == stairsAZ) && !(x == ladderX && z == ladderZ)
	}
	var ground []mgl32.Vec2
	for z := 2; z < size-2; z += 2 {
		for x := 2; x < size-2; x += 2 {
			if isGroundTile(x, z) {
				ground = append(ground, mgl32.Vec2{float32(x), float32(z)})
			}
		}
	}

	// Most actors patrol two ground points opposite each other across the
	// list; a handful are routed up onto each platform so the stairs and the
	// ladder are both something a hundred-actor population actually uses,
	// not just the player.
	platformATarget := mgl32.Vec3{platformAX0 + 2.5, platformASurfaceY, platformAZ0 + 2.5}
	platformBTarget := mgl32.Vec3{platformBX0 + 2.5, platformBSurfaceY, platformBZ0 + 2.5}
	half := len(ground) / 2
	for i := 0; i < actorPopulation && i < len(ground); i++ {
		spawn := ground[i]
		far := ground[(i+half)%len(ground)]
		patrolTo := mgl32.Vec3{far.X() + 0.5, groundSurfaceY, far.Y() + 0.5}
		switch {
		case i%20 == 0:
			patrolTo = platformATarget
		case i%37 == 0:
			patrolTo = platformBTarget
		}
		entity := core.MapEntity{
			Name:                "Scientist",
			TextureIndices:      []uint16{0, 1, 2, 3, 4, 5, 6, 7},
			SpriteSheetName:     "scientist",
			SpriteSheetTileSize: geometry.Point{X: 64, Y: 64},
			CreateCollider:      true,
			PatrolTo:            &patrolTo,
		}
		idx := int(spawn.Y())*size + int(spawn.X())
		mapData[idx].Entities = append(mapData[idx].Entities, entity)
	}

	return core.MapLoadResult{
		TileSize:          64,
		AtlasFilename:     "_internal",
		PlayerSpawn:       mgl32.Vec3{float32(size) / 2, 1, 2.5},
		PlayerDir:         mgl32.Vec2{0, 1},
		CeilingHeight:     actorMapCeiling,
		IsFloorTextured:   true,
		IsCeilingTextured: true,
		CeilingColor:      color.RGBA{60, 60, 70, 255},
		FloorColor:        color.RGBA{40, 40, 40, 255},
		MapDimensions:     [3]int{size, actorMapCeiling + 1, size},
		MapData:           mapData,
		Lights:            actorTestLights(size),
	}
}

func actorTestLights(size int) []core.MapLight {
	center := core.MapLight{
		Position: mgl32.Vec3{float32(size) / 2, float32(actorMapCeiling) - 0.5, float32(size) / 2},
		Color:    mgl32.Vec3{1, 0.97, 0.9}, Intensity: 8, Angle: 140, Reach: float32(size),
	}.LookAt(mgl32.Vec3{float32(size) / 2, 0, float32(size) / 2})
	overStairs := core.MapLight{
		Position: mgl32.Vec3{stairsAX + 0.5, 3.5, stairsAZ},
		Color:    mgl32.Vec3{1, 0.95, 0.85}, Intensity: 6, Angle: 90, Reach: 16,
	}.LookAt(mgl32.Vec3{stairsAX + 0.5, 1, stairsAZ + 1})
	overLadder := core.MapLight{
		Position: mgl32.Vec3{ladderX + 0.5, 5.5, ladderZ},
		Color:    mgl32.Vec3{0.85, 0.9, 1}, Intensity: 6, Angle: 90, Reach: 16,
	}.LookAt(mgl32.Vec3{ladderX + 0.5, 2, ladderZ})
	return []core.MapLight{center, overStairs, overLadder}
}
