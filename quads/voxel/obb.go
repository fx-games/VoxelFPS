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
// oriented box and reports when it first touches, as a fraction of that
// movement, along with the normal of the surface it touched.
//
// This is the separating axis theorem run over time. Two convex boxes are apart
// exactly when there is some direction their shadows on it do not overlap on, so
// each candidate direction gives a window of time during which its two shadows
// do overlap, and the boxes touch at the moment the last of those windows opens.
// Fifteen directions: the three faces of the axis aligned box, the three faces
// of the oriented one, and the nine that run across one edge of each.
//
// Those last nine are the ones that used to be missing. What this did was
// inflate the oriented box by the reach of the moving box along each of its own
// axes and cast a ray from the moving box's centre - which is a box standing in
// for the Minkowski sum of two boxes, and that sum has bevelled edges. So a
// glancing hit near an edge of a rotated box reported slightly early, and a
// mover cornering round one caught on air. An edge-edge contact is exactly what
// a cross axis measures.
func CheckAABBVsOBB(aabb AABB, obb OBB, aabbVelocity mgl32.Vec3) CollisionInfo {
	centerA, halfA := aabb.Center(), aabb.HalfExtents()
	centerB := obb.Transform.Mul4x1(obb.AABB.Center().Vec4(1)).Vec3()

	// The oriented box's own axes and how far it reaches along each of them, in
	// world units. Each column of the transform is one local axis scaled by
	// whatever that axis is scaled by, so its length is the scale and its
	// direction is the axis.
	var axesB [3]mgl32.Vec3
	var halfB mgl32.Vec3
	localHalf := obb.AABB.HalfExtents()
	for axis := 0; axis < 3; axis++ {
		column := obb.Transform.Col(axis).Vec3()
		scale := column.Len()
		if scale == 0 {
			continue // a flattened box has no reach along this one
		}
		axesB[axis] = column.Mul(1 / scale)
		halfB[axis] = scale * localHalf[axis]
	}

	offset := centerA.Sub(centerB)
	earliest, latest := float32(math.Inf(-1)), float32(math.Inf(1))
	shallowest := float32(math.Inf(1))
	var normalAtImpact, wayOut mgl32.Vec3
	apart := false

	// One direction: how far each box reaches along it, how far apart their
	// centres are on it, and how fast that gap is closing. From those, when the
	// shadows start and stop overlapping - and, for boxes that already overlap,
	// how deep this direction's overlap is, since the shallowest of them is the
	// shortest way back out.
	consider := func(direction mgl32.Vec3) {
		if apart || direction.Len() < 1e-6 {
			return // parallel axes cross to nothing and rule nothing out
		}
		direction = direction.Normalize()
		reach := reachAlong(direction, halfA)
		for axis := 0; axis < 3; axis++ {
			reach += abs(direction.Dot(axesB[axis])) * halfB[axis]
		}
		gap := direction.Dot(offset)
		closing := direction.Dot(aabbVelocity)

		if depth := reach - abs(gap); depth < shallowest {
			shallowest, wayOut = depth, direction.Mul(away(gap))
		}

		if closing == 0 {
			apart = abs(gap) > reach // never overlaps on this one, so never at all
			return
		}
		opens, shuts := (-reach-gap)/closing, (reach-gap)/closing
		if opens > shuts {
			opens, shuts = shuts, opens
		}
		if opens > earliest {
			earliest = opens
			normalAtImpact = direction.Mul(away(gap + closing*opens))
		}
		if shuts < latest {
			latest = shuts
		}
		apart = earliest > latest
	}

	for axis := 0; axis < 3; axis++ {
		consider(worldAxis(axis))
	}
	for axis := 0; axis < 3; axis++ {
		consider(axesB[axis])
	}
	for a := 0; a < 3; a++ {
		for b := 0; b < 3; b++ {
			consider(worldAxis(a).Cross(axesB[b]))
		}
	}

	// Apart on some direction, touching only after the move is over, or touching
	// only before it began - which is what moving away from a box looks like: a
	// window of overlap entirely in the past.
	if apart || earliest > 1 || latest < 0 {
		return CollisionInfo{TimeOfImpact: 1}
	}
	if earliest <= 0 {
		// Already overlapping - no window ever opened, they were all open to
		// begin with. Stop right here and point the shortest way out, which is
		// what the axis aligned sweep does with the same case.
		return CollisionInfo{TimeOfImpact: 0, SurfaceNormal: wayOut, IsCollision: true}
	}
	return CollisionInfo{TimeOfImpact: earliest, SurfaceNormal: normalAtImpact, IsCollision: true}
}

// reachAlong is how far a box with these half extents reaches along a direction,
// which for an axis aligned box is its reach on each world axis in turn.
func reachAlong(direction, halfExtents mgl32.Vec3) float32 {
	return abs(direction.X())*halfExtents.X() +
		abs(direction.Y())*halfExtents.Y() +
		abs(direction.Z())*halfExtents.Z()
}

// away is the sign that turns a direction into one pointing from the oriented
// box towards the box moving against it, ie. out of the surface rather than into
// it, given which side of it the mover is on.
func away(side float32) float32 {
	if side < 0 {
		return -1
	}
	return 1
}

func abs(v float32) float32 {
	return float32(math.Abs(float64(v)))
}

func worldAxis(axis int) mgl32.Vec3 {
	var direction mgl32.Vec3
	direction[axis] = 1
	return direction
}
