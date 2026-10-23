package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// rayCast meets the nearest of the things in front of it and the wall: a
// pickup before the wall wins, one behind it is never seen, and with the near
// one gone the wall's face is what is hit.
func TestRayCastHitsTheNearestOfThingsAndBlocks(t *testing.T) {
	cubes := voxel.NewMapWithEmptyChunks(1, 1, 1, 32, 32)
	for y := int32(0); y < 4; y++ {
		for z := int32(0); z < 4; z++ {
			cubes.SetBlock(5, y, z, voxel.NewBlock(1))
		}
	}
	g := &Game{GlApplication: &quads.GlApplication{}, player: &Player{GameObject: quads.NewGameObject("Player")}}
	g.CubeMap = cubes
	g.physicsSystem = quads.NewCollisionSolver(cubes.IsSolidBlockAt)
	g.Camera = core.NewFPSCamera(mgl32.Vec3{0, 1.2, 0.5}, 800, 600, 1)
	near := standingPickup(mgl32.Vec3{3, 1, 0.5}, mgl32.Vec2{0.5, 0.4})
	behind := standingPickup(mgl32.Vec3{7, 1, 0.5}, mgl32.Vec2{0.5, 0.4})
	g.physicsSystem.AddObject(near.object.Physics())
	g.physicsSystem.AddObject(behind.object.Physics())

	origin, ahead := mgl32.Vec3{0, 1.2, 0.5}, mgl32.Vec3{1, 0, 0}
	if hit := g.rayCast(origin, ahead); hit.HitObject != near.object {
		t.Fatalf("hit %v, not the pickup in front of the wall", hit.HitObject)
	}

	g.physicsSystem.RemoveObject(near.object.Physics())
	hit := g.rayCast(origin, ahead)
	if hit.HitObject != nil {
		t.Fatalf("hit %s through the wall", hit.HitObject.DisplayName)
	}
	info := hit.HitInfo
	if !info.Hit || info.CollisionGridPosition != (voxel.Int3{X: 5, Y: 1}) || info.Side != voxel.West ||
		info.PreviousGridPosition != (voxel.Int3{X: 4, Y: 1}) || info.CollisionWorldPosition.X() != 5 {
		t.Fatalf("wall hit is %+v", info)
	}
}
