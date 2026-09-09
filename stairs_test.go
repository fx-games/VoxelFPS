package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// stairsMap is the shipped stairs level: the blocks the game would put in the
// world and the staircase standing in them. It is loaded rather than
// reconstructed, so this walks the level that actually ships - the map is an
// asset now, and a test against a copy of it would pass while the asset rotted.
func stairsMap(t *testing.T) (blocks *voxel.Map, staircaseAt mgl32.Vec3) {
	t.Helper()
	result, blocks, err := loadMap("stairs")
	if err != nil {
		t.Fatal(err)
	}
	for _, spawn := range result.Entities {
		if spawn.Entity.MeshName == stairsMeshName {
			return blocks, spawn.Position
		}
	}
	t.Fatalf("no %s entity in the stairs map", stairsMeshName)
	return nil, mgl32.Vec3{}
}

// walkUpTheStairs runs the real physics system: a player sized box is pushed at
// walking speed towards the staircase and its final position is returned.
func walkUpTheStairs(t *testing.T, stepHeight float32) (end mgl32.Vec3, staircaseAt mgl32.Vec3) {
	t.Helper()
	blocks, staircaseAt := stairsMap(t)
	system := quads.NewCollisionSolver(blocks.IsSolidBlockAt, blocks.ContainsVec)
	system.SetGravityEnabled(true)
	system.SetGravity(mgl32.Vec3{0, -q3Gravity, 0})
	system.SetStepHeight(stepHeight)

	// The steps are data now, in assets/meshes/stairs/scenery.json, so this walks the
	// boxes the game itself would put in the world rather than a copy of them.
	library, err := loadScenery(meshDir)
	if err != nil {
		t.Fatal(err)
	}
	stairs, known := library[stairsMeshName]
	if !known {
		t.Fatalf("no %s in %s", stairsMeshName, meshDir)
	}
	if len(stairs.Colliders) != 4 {
		t.Fatalf("stairs have %d colliders, want one per step", len(stairs.Colliders))
	}
	staircase := quads.NewGameObject("Stairs")
	staircase.Transform().SetPosition(staircaseAt)
	for _, step := range stairs.colliders(staircase) {
		system.AddObject(step)
	}

	object := quads.NewGameObject("Player")
	// One tile short of the bottom step, walking at it.
	object.Transform().SetPosition(staircaseAt.Sub(mgl32.Vec3{0, 0, 1}))
	player := quads.NewPhysicsComponent(true, voxel.NewAABB(
		mgl32.Vec3{-0.2, 0, -0.2},
		mgl32.Vec3{0.2, 0.9, 0.2},
	))
	object.SetComponent(player)
	system.AddObject(player)

	// A second and a half at walking speed: up the four steps and a few tiles
	// onto the platform, stopping short of its far edge.
	const tick = 1.0 / 60.0
	for i := 0; i < 90; i++ {
		fall := player.Velocity().Y()
		player.SetVelocity(mgl32.Vec3{0, fall, pmSpeed})
		system.Update(tick)
	}
	return player.Position(), staircaseAt
}

// With the Quake 3 step height the player walks the staircase and ends up on the
// platform, a whole voxel above where it started.
func TestPlayerWalksUpTheStairs(t *testing.T) {
	end, staircaseAt := walkUpTheStairs(t, pmStepSize)

	if math.Abs(float64(end.Y()-2)) > 0.05 {
		t.Errorf("ended at y = %f, want to be standing on the platform at y = 2", end.Y())
	}
	// The platform starts where the two-tile staircase ends.
	if platformZ := staircaseAt.Z() + 2; end.Z() < platformZ {
		t.Errorf("did not reach the platform at z = %f: z = %f", platformZ, end.Z())
	}
}

// Without it every step is a wall, which is what the step-up is there to fix.
func TestWithoutStepHeightTheStairsBlock(t *testing.T) {
	end, staircaseAt := walkUpTheStairs(t, 0)

	if end.Y() > 1.01 {
		t.Errorf("climbed without a step height: y = %f", end.Y())
	}
	if end.Z() > staircaseAt.Z() {
		t.Errorf("walked into the first step and through it: z = %f", end.Z())
	}
}
