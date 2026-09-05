package main

import (
	"fmt"
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// The staircase assets/meshes/stairs.glb is built from. Must match the constants
// in tools/genstairs, which draws the same boxes.
//
// The rise is what the whole thing is about: at 0.25 it is under the physics
// system's step height (18 Quake units, 0.289 here), so the player walks up it.
// Raise it past that and the stairs become a wall, which is the point of having
// a step height at all.
const (
	stairsMeshName = "stairs"
	stairStepRise  = 0.25
	stairStepRun   = 0.5
	stairStepCount = 4
	stairWidth     = 1.0
	// The front face of the bottom step, so a staircase spawned at a tile centre
	// spans two whole tiles and its top step ends on the tile boundary.
	stairFrontZ = -0.5
)

// stairColliders is the physics half of the stairs mesh: one box per step,
// reaching from the floor to that step's top. The mesh drawer would hand
// spawnEntity a single box around the whole staircase, and a single box is a
// wall - each step has to be its own collider for the step-up to have anything
// to climb.
func stairColliders(origin mgl32.Vec3) []*quads.PhysicsComponent {
	steps := make([]*quads.PhysicsComponent, 0, stairStepCount)
	for i := 0; i < stairStepCount; i++ {
		step := quads.NewGameObject(fmt.Sprintf("Stair %d", i))
		step.Transform().SetPosition(origin)
		physics := quads.NewPhysicsComponent(false, voxel.NewAABB(
			mgl32.Vec3{-stairWidth / 2, 0, stairFrontZ + float32(i)*stairStepRun},
			mgl32.Vec3{stairWidth / 2, float32(i+1) * stairStepRise, stairFrontZ + float32(i+1)*stairStepRun},
		))
		step.SetComponent(physics)
		steps = append(steps, physics)
	}
	return steps
}

// The stairs test map: one platform a whole voxel above the floor, one staircase
// leading up onto it, and every other side of the platform left bare. Walk up the
// stairs and the step-up carries you; walk into the platform anywhere else and it
// blocks, because a full voxel is far taller than the step height.
const (
	stairsMapSize    = 16
	stairsCeiling    = 4
	stairsPlatformZ0 = 6 // the platform, in tiles
	stairsPlatformZ1 = 12
	stairsPlatformX0 = 5
	stairsPlatformX1 = 11
	// The staircase tile. Its top step ends where the platform starts.
	stairsTileX = 8
	stairsTileZ = stairsPlatformZ0 - 2
)

func loadStairsTestMap() core.MapLoadResult {
	size := stairsMapSize
	mapData := make([]core.TileInfo, size*size)

	for z := 0; z < size; z++ {
		for x := 0; x < size; x++ {
			tile := &mapData[z*size+x]
			tile.FloorTex = 2
			tile.CeilTex = 3
			tile.WallTex = 0
			tile.CeilingHeight = stairsCeiling

			if x == 0 || z == 0 || x == size-1 || z == size-1 {
				tile.WallHeight = stairsCeiling
				continue
			}
			onPlatform := x >= stairsPlatformX0 && x <= stairsPlatformX1 &&
				z >= stairsPlatformZ0 && z <= stairsPlatformZ1
			if onPlatform {
				tile.WallHeight = 1
			}
		}
	}

	stairsTile := &mapData[stairsTileZ*size+stairsTileX]
	stairsTile.Entities = append(stairsTile.Entities, core.MapEntity{
		Name:     "Stairs",
		MeshName: stairsMeshName,
	})

	return core.MapLoadResult{
		TileSize:      64,
		AtlasFilename: "_internal",
		// A few tiles back from the stairs, looking straight at them.
		PlayerSpawn:       mgl32.Vec3{float32(stairsTileX) + 0.5, 1, 2.5},
		PlayerDir:         mgl32.Vec2{0, 1},
		CeilingHeight:     stairsCeiling,
		IsFloorTextured:   true,
		IsCeilingTextured: true,
		CeilingColor:      color.RGBA{60, 60, 70, 255},
		FloorColor:        color.RGBA{40, 40, 40, 255},
		MapDimensions:     [3]int{size, stairsCeiling + 1, size},
		MapData:           mapData,
		Lights:            stairsTestLights(),
	}
}

// Two lights: one over the stairs so the steps read as steps, one over the far
// end of the platform so there is somewhere lit to walk to.
func stairsTestLights() []core.MapLight {
	overStairs := mgl32.Vec3{float32(stairsTileX) + 0.5, 3.5, float32(stairsTileZ)}
	overPlatform := mgl32.Vec3{float32(stairsTileX) + 0.5, 3.5, stairsPlatformZ1 - 1}
	stairsLight := core.MapLight{
		Position: overStairs, Color: mgl32.Vec3{1, 0.95, 0.85},
		Intensity: 6, Angle: 90, Reach: 16,
	}.LookAt(mgl32.Vec3{float32(stairsTileX) + 0.5, 1, float32(stairsTileZ) + 1})
	platformLight := core.MapLight{
		Position: overPlatform, Color: mgl32.Vec3{0.85, 0.9, 1},
		Intensity: 6, Angle: 90, Reach: 16,
	}.LookAt(mgl32.Vec3{float32(stairsTileX) + 0.5, 2, stairsPlatformZ1})
	return []core.MapLight{stairsLight, platformLight}
}
