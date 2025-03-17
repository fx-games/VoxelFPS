package voxel

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "math"
)

type CollisionInfo struct {
    // TimeOfImpact is the normalized time of impact, where 0 is the start of the movement and 1 is the end of the movement.
    TimeOfImpact float32
    // SurfaceNormal is the normal of the surface that was hit.
    SurfaceNormal mgl32.Vec3
    IsCollision   bool
}

func NewCollisionInfo(intersection bool, timeOfImpact float32, normalAtImpact mgl32.Vec3) CollisionInfo {
    return CollisionInfo{
        TimeOfImpact:  timeOfImpact,
        SurfaceNormal: normalAtImpact,
        IsCollision:   intersection,
    }
}

type AABB struct {
    minCoords            mgl32.Vec3
    maxCoords            mgl32.Vec3
    vertexesForDebugDraw *glhf.VertexSlice[glhf.GlFloat]
    id                   int
}

func NewAABB(minCoords, maxCoords mgl32.Vec3) AABB {
    return AABB{
        minCoords: minCoords,
        maxCoords: maxCoords,
    }
}

func NewAABBFromOrigin(origin, extents mgl32.Vec3) AABB {
    // extents {1,1,1} && origin {0,0,0} => minCoords {1,1,1} && maxCoords {1,1,1}
    return AABB{
        minCoords: extents.Sub(origin),
        maxCoords: extents.Add(origin),
    }
}

func NewAABBFromMin(min, extents mgl32.Vec3) AABB {
    return AABB{
        minCoords: min,
        maxCoords: min.Add(extents),
    }
}

func (a AABB) Min() mgl32.Vec3 {
    return a.minCoords
}

func (a AABB) Max() mgl32.Vec3 {
    return a.maxCoords
}

func (a AABB) Extents() mgl32.Vec3 {
    return a.maxCoords.Sub(a.minCoords)
}

func (a AABB) HalfExtents() mgl32.Vec3 {
    return a.Extents().Mul(0.5)
}

func (a AABB) InflatedBy(extents mgl32.Vec3) AABB {
    return NewAABB(a.Min().Sub(extents), a.Max().Add(extents))
}

func (a AABB) MinkowskiDifference(other AABB) AABB {
    minM := other.Min().Sub(a.Max())
    extM := a.Extents().Add(other.Extents())
    return NewAABBFromMin(minM, extM)
}

func (a AABB) Draw(shader *glhf.Shader) {
    if a.vertexesForDebugDraw == nil {
        a.vertexesForDebugDraw = glhf.MakeVertexSlice(shader, 3*2*4, 3*2*4) // 2*4 verts for top and bottom, 2*4 verts for sides
        a.vertexesForDebugDraw.SetPrimitiveType(gl.LINES)
        a.vertexesForDebugDraw.Begin()
        a.vertexesForDebugDraw.SetVertexData([]glhf.GlFloat{
            // top
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),

            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),

            // bottom
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),

            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),

            // sides
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Min().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Min().Z()),

            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Max().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),

            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Min().Y()), glhf.GlFloat(a.Max().Z()),
            glhf.GlFloat(a.Min().X()), glhf.GlFloat(a.Max().Y()), glhf.GlFloat(a.Max().Z()),
        })
        a.vertexesForDebugDraw.End()
    }
    a.vertexesForDebugDraw.Begin()
    a.vertexesForDebugDraw.Draw()
    a.vertexesForDebugDraw.End()
}

func (a AABB) Contains(vec3 mgl32.Vec3) bool {
    minVal := a.Min()
    maxVal := a.Max()
    return vec3.X() >= minVal.X() && vec3.X() <= maxVal.X() &&
        vec3.Y() >= minVal.Y() && vec3.Y() <= maxVal.Y() &&
        vec3.Z() >= minVal.Z() && vec3.Z() <= maxVal.Z()
}

func (a AABB) Center() mgl32.Vec3 {
    return a.Min().Add(a.Max()).Mul(0.5)
}

func (a AABB) Add(pos mgl32.Vec3) AABB {
    return NewAABB(a.Min().Add(pos), a.Max().Add(pos))
}

func (a AABB) Corners() [8]mgl32.Vec3 {
    minVal := a.Min()
    maxVal := a.Max()
    return [8]mgl32.Vec3{
        mgl32.Vec3{minVal.X(), minVal.Y(), minVal.Z()},
        mgl32.Vec3{maxVal.X(), minVal.Y(), minVal.Z()},
        mgl32.Vec3{maxVal.X(), maxVal.Y(), minVal.Z()},
        mgl32.Vec3{minVal.X(), maxVal.Y(), minVal.Z()},
        mgl32.Vec3{minVal.X(), minVal.Y(), maxVal.Z()},
        mgl32.Vec3{maxVal.X(), minVal.Y(), maxVal.Z()},
        mgl32.Vec3{maxVal.X(), maxVal.Y(), maxVal.Z()},
        mgl32.Vec3{minVal.X(), maxVal.Y(), maxVal.Z()},
    }
}

func (a AABB) GetNearestFaceNormal(pos mgl32.Vec3) mgl32.Vec3 {
    normals := [6]mgl32.Vec3{
        mgl32.Vec3{0, 1, 0},
        mgl32.Vec3{0, -1, 0},
        mgl32.Vec3{1, 0, 0},
        mgl32.Vec3{-1, 0, 0},
        mgl32.Vec3{0, 0, 1},
        mgl32.Vec3{0, 0, -1},
    }
    var distances [6]float32

    distances[0] = PlaneDistanceToPoint(normals[0], a.Max(), pos)
    distances[1] = PlaneDistanceToPoint(normals[1], a.Min(), pos)
    distances[2] = PlaneDistanceToPoint(normals[2], a.Max(), pos)
    distances[3] = PlaneDistanceToPoint(normals[3], a.Min(), pos)
    distances[4] = PlaneDistanceToPoint(normals[4], a.Max(), pos)
    distances[5] = PlaneDistanceToPoint(normals[5], a.Min(), pos)

    smallestDistIndex := -1
    smallestDist := float32(math.MaxFloat32)

    for i, dist := range distances {
        absDist := math.Abs(float64(dist))
        absSmallest := math.Abs(float64(smallestDist))
        if absDist == absSmallest && (dist > 0 && smallestDist < 0) {
            smallestDist = dist
            smallestDistIndex = i

        } else if absDist < absSmallest {
            smallestDist = dist
            smallestDistIndex = i
        }
    }

    return normals[smallestDistIndex]
}

func (a AABB) Extremes() (mgl32.Vec3, mgl32.Vec3) {
    return a.Min(), a.Max()
}

// SweepAABB will return a value between 0 and 1, where 0.5 means "a collision occurred halfway of the AABB's path" and 1 means "there was no collision".
// Notably, if one of the AABB's is completely inside the other, it will return 1, meaning our AABBs will be "hollow".
// It also returns the Normal to the surface that was hit. This will be useful when we implement wall sliding.
func SweepAABB(a, b AABB, vel mgl32.Vec3) CollisionInfo {
    // adapted from: https://luisreis.net/blog/aabb_collision_handling/
    m := a.MinkowskiDifference(b)
    return SweepAABBFromMinkowski(m, vel)
}

func SweepAABBFromMinkowski(m AABB, vel mgl32.Vec3) CollisionInfo {
    // adapted from: https://luisreis.net/blog/aabb_collision_handling/
    containsOrigin := m.Contains(mgl32.Vec3{})
    h := float32(1.0)
    nx := 0
    ny := 0
    nz := 0
    var s float32
    nullVec := mgl32.Vec3{}

    // X Min
    s = LineToPlaneIntersection(nullVec, vel, m.Min(), mgl32.Vec3{-1, 0, 0})
    if s >= 0 && vel.X() > 0 && s < h && InRange(s*vel.Y(), m.Min().Y(), m.Max().Y()) && InRange(s*vel.Z(), m.Min().Z(), m.Max().Z()) {
        nx = -1
        h = s
        ny = 0
        nz = 0
    }

    // X Max
    s = LineToPlaneIntersection(nullVec, vel, mgl32.Vec3{m.Max().X(), m.Min().Y(), m.Min().Z()}, mgl32.Vec3{1, 0, 0})
    if s >= 0 && vel.X() < 0 && s < h && InRange(s*vel.Y(), m.Min().Y(), m.Max().Y()) && InRange(s*vel.Z(), m.Min().Z(), m.Max().Z()) {
        nx = 1
        h = s
        ny = 0
        nz = 0
    }

    // Y Min
    s = LineToPlaneIntersection(nullVec, vel, m.Min(), mgl32.Vec3{0, -1, 0})
    if s >= 0 && vel.Y() > 0 && s < h && InRange(s*vel.X(), m.Min().X(), m.Max().X()) && InRange(s*vel.Z(), m.Min().Z(), m.Max().Z()) {
        nx = 0
        h = s
        ny = -1
        nz = 0
    }

    // Y Max
    s = LineToPlaneIntersection(nullVec, vel, mgl32.Vec3{m.Min().X(), m.Max().Y(), m.Min().Z()}, mgl32.Vec3{0, 1, 0})
    if s >= 0 && vel.Y() < 0 && s < h && InRange(s*vel.X(), m.Min().X(), m.Max().X()) && InRange(s*vel.Z(), m.Min().Z(), m.Max().Z()) {
        nx = 0
        h = s
        ny = 1
        nz = 0
    }

    // Z Min
    s = LineToPlaneIntersection(nullVec, vel, m.Min(), mgl32.Vec3{0, 0, -1})
    if s >= 0 && vel.Z() > 0 && s < h && InRange(s*vel.X(), m.Min().X(), m.Max().X()) && InRange(s*vel.Y(), m.Min().Y(), m.Max().Y()) {
        nx = 0
        h = s
        ny = 0
        nz = -1
    }

    // Z Max
    s = LineToPlaneIntersection(nullVec, vel, mgl32.Vec3{m.Min().X(), m.Min().Y(), m.Max().Z()}, mgl32.Vec3{0, 0, 1})
    if s >= 0 && vel.Z() < 0 && s < h && InRange(s*vel.X(), m.Min().X(), m.Max().X()) && InRange(s*vel.Y(), m.Min().Y(), m.Max().Y()) {
        nx = 0
        h = s
        ny = 0
        nz = 1
    }

    return CollisionInfo{
        TimeOfImpact:  h,
        SurfaceNormal: mgl32.Vec3{float32(nx), float32(ny), float32(nz)},
        IsCollision:   containsOrigin,
    }
}

func InRange(x, min, max float32) bool {
    return x >= min && x <= max
}

func LineToPlaneIntersection(lineStart, lineEnd, planeCoord, planeNormal mgl32.Vec3) float32 {
    NdotU := planeNormal.Dot(lineEnd)
    if NdotU == 0 {
        return math.MaxFloat32
    }
    return planeNormal.Dot(planeCoord.Sub(lineStart)) / NdotU
}

func IntersectRayAABB(rayOrigin, rayDir mgl32.Vec3, aabb AABB) (intersectionDistance float64, intersection bool) {
    tMin := aabb.Min().Sub(rayOrigin)
    tMax := aabb.Max().Sub(rayOrigin)
    tMin = Vec3Div(tMin, rayDir)
    tMax = Vec3Div(tMax, rayDir)
    t1 := mgl32.Vec3{float32(min(float64(tMin.X()), float64(tMax.X()))), float32(min(float64(tMin.Y()), float64(tMax.Y()))), float32(min(float64(tMin.Z()), float64(tMax.Z())))}
    t2 := mgl32.Vec3{float32(max(float64(tMin.X()), float64(tMax.X()))), float32(max(float64(tMin.Y()), float64(tMax.Y()))), float32(max(float64(tMin.Z()), float64(tMax.Z())))}
    tNear := max(max(float64(t1.X()), float64(t1.Y())), float64(t1.Z()))
    tFar := min(min(float64(t2.X()), float64(t2.Y())), float64(t2.Z()))
    if tNear > tFar || tFar < 0 {
        return -1, false
    }
    return tNear, true
}

func Vec3Div(a, b mgl32.Vec3) mgl32.Vec3 {
    return mgl32.Vec3{a.X() / b.X(), a.Y() / b.Y(), a.Z() / b.Z()}
}
func PlaneDistanceToPoint(planeNormal mgl32.Vec3, planePoint mgl32.Vec3, point mgl32.Vec3) float32 {
    return planeNormal.Dot(point.Sub(planePoint))
}
