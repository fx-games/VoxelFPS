package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "math"
)

type OBB struct {
    AABB
    Transform mgl32.Mat4
}

// IntersectRayOBB intersects a world space ray with an oriented box. The
// returned distance is in world units, ie. along the unnormalized rayDir, even
// when the box transform scales.
func IntersectRayOBB(rayOrigin, rayDir mgl32.Vec3, obb OBB) (intersectionDistance float64, isIntersection bool) {
    // Transform ray to object space
    invTransform := obb.Transform.Inv()

    localRayOrigin := invTransform.Mul4x1(mgl32.Vec4{rayOrigin.X(), rayOrigin.Y(), rayOrigin.Z(), 1}).Vec3()

    localRayDir := invTransform.Mul4x1(mgl32.Vec4{rayDir.X(), rayDir.Y(), rayDir.Z(), 0}).Vec3()
    // How many local units one world unit of the ray covers.
    localUnitsPerWorldUnit := float64(localRayDir.Len()) / float64(rayDir.Len())
    if localUnitsPerWorldUnit == 0 || math.IsNaN(localUnitsPerWorldUnit) {
        return -1, false
    }

    // Intersect transformed ray with AABB
    localDistance, hit := IntersectRayAABB(localRayOrigin, localRayDir.Normalize(), obb.AABB)
    if !hit {
        return -1, false
    }

    return localDistance / localUnitsPerWorldUnit, true
}

// CheckAABBVsOBB sweeps a world space AABB along aabbVelocity against an
// oriented box and reports when it first touches. The OBB is inflated by the
// reach of the AABB along each of its own axes, so the sweep becomes a ray cast
// from the AABB's center.
// ponytail: that inflation is a box, while the exact Minkowski sum of two boxes
// has bevelled edges, so a glancing hit near an edge of a rotated box reports
// slightly too early. Replace with a proper SAT sweep if that ever shows.
func CheckAABBVsOBB(aabb AABB, obb OBB, aabbVelocity mgl32.Vec3) CollisionInfo {
    lengthOfMovement := float64(aabbVelocity.Len())
    if lengthOfMovement == 0 {
        return CollisionInfo{TimeOfImpact: 1}
    }
    inflatedObb := obb
    inflatedObb.AABB = obb.AABB.InflatedBy(localInflation(obb.Transform, aabb.HalfExtents()))

    rayOrigin := aabb.Center()
    direction := aabbVelocity.Mul(float32(1 / lengthOfMovement))
    distance, intersection := IntersectRayOBB(rayOrigin, direction, inflatedObb)
    if !intersection || math.IsNaN(distance) || distance > lengthOfMovement {
        return CollisionInfo{TimeOfImpact: 1}
    }
    if distance < 0 {
        distance = 0 // the boxes already overlap, stop right here
    }

    // The impact point sits on the surface of the inflated box, so take the
    // normal from that box and rotate it back into world space.
    impactPoint := rayOrigin.Add(direction.Mul(float32(distance)))
    localImpactPoint := obb.Transform.Inv().Mul4x1(impactPoint.Vec4(1)).Vec3()
    localNormal := inflatedObb.AABB.GetNearestFaceNormal(localImpactPoint)
    surfaceNormal := obb.Transform.Mul4x1(localNormal.Vec4(0)).Vec3().Normalize()

    return CollisionInfo{
        TimeOfImpact:  float32(distance / lengthOfMovement),
        SurfaceNormal: surfaceNormal,
        IsCollision:   true,
    }
}

// localInflation returns how far the given half extents reach along each local
// axis of the transform, expressed in the local units of that transform. That
// is what an oriented box has to grow by to swallow an axis aligned box of
// those half extents, rotation and scale of the transform included.
func localInflation(transform mgl32.Mat4, halfExtents mgl32.Vec3) mgl32.Vec3 {
    var inflation mgl32.Vec3
    for axis := 0; axis < 3; axis++ {
        localAxis := transform.Col(axis).Vec3()
        scale := localAxis.Len() // world units per local unit along this axis
        if scale == 0 {
            continue
        }
        unitAxis := localAxis.Mul(1 / scale)
        reach := float32(0)
        for component := 0; component < 3; component++ {
            reach += float32(math.Abs(float64(unitAxis[component]))) * halfExtents[component]
        }
        inflation[axis] = reach / scale
    }
    return inflation
}
