package voxel

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/collide"
)

type OBB struct {
	AABB
	Transform mgl32.Mat4
}

// Collide is the box as VoxelLib's collide.OBB: the transform's columns are the
// scaled axes and its translation the position.
func (o OBB) Collide() collide.OBB {
	t := o.Transform
	return collide.OBB{
		Local: o.AABB.Collide(),
		Axes: [3][3]float64{
			{float64(t[0]), float64(t[1]), float64(t[2])},
			{float64(t[4]), float64(t[5]), float64(t[6])},
			{float64(t[8]), float64(t[9]), float64(t[10])},
		},
		Pos: [3]float64{float64(t[12]), float64(t[13]), float64(t[14])},
	}
}

// Collide is the box as VoxelLib's collide.AABB.
func (a AABB) Collide() collide.AABB {
	return collide.AABB{
		MinX: float64(a.minCoords.X()), MinY: float64(a.minCoords.Y()), MinZ: float64(a.minCoords.Z()),
		MaxX: float64(a.maxCoords.X()), MaxY: float64(a.maxCoords.Y()), MaxZ: float64(a.maxCoords.Z()),
	}
}

func vec64(v mgl32.Vec3) [3]float64 {
	return [3]float64{float64(v.X()), float64(v.Y()), float64(v.Z())}
}

func vec32(v [3]float64) mgl32.Vec3 { return mgl32.Vec3{float32(v[0]), float32(v[1]), float32(v[2])} }

// IntersectRayOBB intersects a world space ray with an oriented box
// (collide.IntersectRayOBB). The distance is in world units even when the box
// transform scales.
func IntersectRayOBB(rayOrigin, rayDir mgl32.Vec3, obb OBB) (float64, bool) {
	dist, _, ok := collide.IntersectRayOBB(vec64(rayOrigin), vec64(rayDir), obb.Collide())
	if !ok {
		return -1, false
	}
	return dist, true
}

// IntersectRayAABB is the distance along the ray, in multiples of rayDir, at
// which it enters the box (0 when it starts inside).
func IntersectRayAABB(rayOrigin, rayDir mgl32.Vec3, aabb AABB) (float64, bool) {
	t, _, ok := collide.RayAABB(vec64(rayOrigin), vec64(rayDir), aabb.Collide())
	if !ok {
		return -1, false
	}
	return t, true
}

// CheckAABBVsOBB sweeps a world space AABB along aabbVelocity against an
// oriented box (collide.SweepAABBVsOBB).
func CheckAABBVsOBB(aabb AABB, obb OBB, aabbVelocity mgl32.Vec3) CollisionInfo {
	return collisionInfo(collide.SweepAABBVsOBB(aabb.Collide(), obb.Collide(), vec64(aabbVelocity)))
}

// SweepAABB sweeps a along vel against b. Boxes that already overlap collide at
// 0, along the shallowest way out.
func SweepAABB(a, b AABB, vel mgl32.Vec3) CollisionInfo {
	box := collide.OBB{Local: b.Collide(), Axes: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
	return collisionInfo(collide.SweepAABBVsOBB(a.Collide(), box, vec64(vel)))
}

func collisionInfo(s collide.OBBSweep) CollisionInfo {
	return CollisionInfo{TimeOfImpact: float32(s.T), SurfaceNormal: vec32(s.Normal), IsCollision: s.Hit}
}
