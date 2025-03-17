package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "math"
)

type OBB struct {
    AABB
    Transform mgl32.Mat4
}

func IntersectRayOBB(rayOrigin, rayDir mgl32.Vec3, obb OBB) (intersectionDistance float64, isIntersection bool) {
    // Transform ray to object space
    transformMatrix := obb.Transform
    invTransform := transformMatrix.Inv()

    localRayOrigin := invTransform.Mul4x1(mgl32.Vec4{rayOrigin.X(), rayOrigin.Y(), rayOrigin.Z(), 1}).Vec3()

    localRayDir4 := invTransform.Mul4x1(mgl32.Vec4{rayDir.X(), rayDir.Y(), rayDir.Z(), 0})
    localRayDir := localRayDir4.Vec3().Normalize() // Convert to Vec3 before normalizing

    // Intersect transformed ray with AABB
    intersectionDistance, hit := IntersectRayAABB(localRayOrigin, localRayDir, obb.AABB)
    if !hit {
        return -1, false
    }

    return intersectionDistance, true
}

func CheckAABBVsOBB(aabb AABB, obb OBB, aabbVelocity mgl32.Vec3) CollisionInfo {
    lengthOfMovement := float64(aabbVelocity.Len())
    normalizedVelocity := aabbVelocity.Normalize()
    inflatedObb := obb
    inflatedObb.AABB = obb.AABB.InflatedBy(aabb.HalfExtents())
    rayOrigin := aabb.Center()
    distance, intersection := IntersectRayOBB(rayOrigin, normalizedVelocity, inflatedObb)
    if !intersection || distance > lengthOfMovement || math.IsNaN(distance) {
        return CollisionInfo{TimeOfImpact: 1, IsCollision: false}
    }
    intersectionPoint := rayOrigin.Add(aabbVelocity.Mul(float32(distance)))
    intersectionPointTransformed := obb.Transform.Inv().Mul4x1(intersectionPoint.Vec4(1)).Vec3()
    surfaceNormal := obb.AABB.GetNearestFaceNormal(intersectionPointTransformed)
    surfaceNormal = obb.Transform.Mul4x1(surfaceNormal.Vec4(0)).Vec3().Normalize()
    timeOfImpact := float32(distance) / float32(lengthOfMovement)
    return CollisionInfo{TimeOfImpact: timeOfImpact, IsCollision: true, SurfaceNormal: surfaceNormal}
}

func CheckOBBVsOBB(a, b OBB, aVelocity mgl32.Vec3) CollisionInfo {
    b.Transform = b.Transform.Mul4(a.Transform)
    return CheckAABBVsOBB(a.AABB, b, aVelocity)
}
