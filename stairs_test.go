package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// The stairs test map in miniature: a floor at y < 1 and the platform the stairs
// lead onto, one voxel tall, starting where the top step ends.
func stairsWorld() (isSolid func(x, y, z int32) bool, isInside func(pos mgl32.Vec3) bool) {
	isSolid = func(x, y, z int32) bool {
		if y < 1 {
			return true
		}
		onPlatform := z >= stairsPlatformZ0 && z <= stairsPlatformZ1 &&
			x >= stairsPlatformX0 && x <= stairsPlatformX1
		return onPlatform && y < 2
	}
	isInside = func(pos mgl32.Vec3) bool {
		return pos.X() > 0 && pos.X() < stairsMapSize && pos.Z() > 0 && pos.Z() < stairsMapSize &&
			pos.Y() > -1 && pos.Y() < stairsCeiling+1
	}
	return
}

// walkUpTheStairs runs the real physics system: a player sized box is pushed at
// walking speed towards the staircase and its final position is
func walkUpTheStairs(t *testing.T, stepHeight float32) mgl32.Vec3 {
	t.Helper()
	isSolid, isInside := stairsWorld()
	system := quads.NewCollisionSolver(isSolid, isInside)
	system.SetGravityEnabled(true)
	system.SetGravity(mgl32.Vec3{0, -q3Gravity, 0})
	system.SetStepHeight(stepHeight)

	// The steps are data now, in scenery/stairs/scenery.json, so this walks the
	// boxes the game itself would put in the world rather than a copy of them.
	library, err := loadScenery(sceneryDir)
	if err != nil {
		t.Fatal(err)
	}
	stairs, known := library[stairsMeshName]
	if !known {
		t.Fatalf("no %s in %s", stairsMeshName, sceneryDir)
	}
	if len(stairs.Colliders) != 4 {
		t.Fatalf("stairs have %d colliders, want one per step", len(stairs.Colliders))
	}
	staircase := quads.NewGameObject("Stairs")
	staircase.Transform().SetPosition(mgl32.Vec3{stairsTileX + 0.5, 1, stairsTileZ + 0.5})
	for _, step := range stairs.colliders(staircase) {
		system.AddObject(step)
	}

	object := quads.NewGameObject("Player")
	object.Transform().SetPosition(mgl32.Vec3{stairsTileX + 0.5, 1, stairsTileZ - 0.5})
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
	return player.Position()
}

// With the Quake 3 step height the player walks the staircase and ends up on the
// platform, a whole voxel above where it started.
func TestPlayerWalksUpTheStairs(t *testing.T) {
	end := walkUpTheStairs(t, pmStepSize)

	if math.Abs(float64(end.Y()-2)) > 0.05 {
		t.Errorf("ended at y = %f, want to be standing on the platform at y = 2", end.Y())
	}
	if end.Z() < stairsPlatformZ0 {
		t.Errorf("did not reach the platform: z = %f", end.Z())
	}
}

// Without it every step is a wall, which is what the step-up is there to fix.
func TestWithoutStepHeightTheStairsBlock(t *testing.T) {
	end := walkUpTheStairs(t, 0)

	if end.Y() > 1.01 {
		t.Errorf("climbed without a step height: y = %f", end.Y())
	}
	if end.Z() > stairsTileZ {
		t.Errorf("walked into the first step and through it: z = %f", end.Z())
	}
}
