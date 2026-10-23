package voxel

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxelfps/quads/glhf"
)

type CollisionInfo struct {
	// TimeOfImpact is the normalized time of impact, where 0 is the start of the movement and 1 is the end of the movement.
	TimeOfImpact float32
	// SurfaceNormal is the normal of the surface that was hit.
	SurfaceNormal mgl32.Vec3
	// IsCollision reports whether the sweep touched anything, ie. TimeOfImpact < 1.
	IsCollision bool
}

type AABB struct {
	minCoords mgl32.Vec3
	maxCoords mgl32.Vec3
	id        int
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

// The F1 debug boxes - "Show AABBs" and "Show OBBs" - are collected here and
// drawn as one mesh at the end of the pass, rather than one box at a time.
//
// Twelve lines a box is nothing to draw and everything to submit. Per box, one
// at a time, this bound a vertex array - which asks the driver what was bound
// before and waits for the answer - refilled the same buffer the box before it
// was still being drawn out of, and submitted its own draw call, and each of
// those three is a place the CPU stops until the GPU catches up. A room with two
// hundred things in it spent longer on the outlines than on the room.
//
// Each box's transform is folded into its corners here, because one draw call
// has one model matrix. Whoever draws the batch sets that to the identity.
var (
	debugLines     []glhf.GlFloat
	debugLineArray *glhf.VertexArray[glhf.GlFloat]
)

// QueueDebugDraw adds this box, placed by model, to what the next DrawDebugLines
// draws.
func (a AABB) QueueDebugDraw(model mgl32.Mat4) {
	for _, corner := range a.edges() {
		placed := model.Mul4x1(corner.Vec4(1)).Vec3()
		debugLines = append(debugLines,
			glhf.GlFloat(placed.X()), glhf.GlFloat(placed.Y()), glhf.GlFloat(placed.Z()))
	}
}

// DrawDebugLines draws every box queued since the last call, and empties the
// queue. The buffer is kept and refilled, growing to the busiest frame's worth.
func DrawDebugLines(shader *glhf.Shader) {
	if len(debugLines) == 0 {
		return
	}
	if debugLineArray == nil {
		debugLineArray = glhf.NewVertexArray[glhf.GlFloat](shader, true)
		debugLineArray.SetPrimitiveType(gl.LINES)
	}
	debugLineArray.Begin()
	debugLineArray.SetVertexData(debugLines)
	debugLineArray.Draw()
	debugLineArray.End()
	debugLines = debugLines[:0]
}

// edges is the twelve edges of the box, as the pairs of corners they run
// between.
func (a AABB) edges() [24]mgl32.Vec3 {
	minX, minY, minZ := a.Min().X(), a.Min().Y(), a.Min().Z()
	maxX, maxY, maxZ := a.Max().X(), a.Max().Y(), a.Max().Z()
	return [24]mgl32.Vec3{
		// top
		{minX, maxY, minZ}, {maxX, maxY, minZ},
		{maxX, maxY, minZ}, {maxX, maxY, maxZ},
		{maxX, maxY, maxZ}, {minX, maxY, maxZ},
		{minX, maxY, maxZ}, {minX, maxY, minZ},
		// bottom
		{minX, minY, minZ}, {maxX, minY, minZ},
		{maxX, minY, minZ}, {maxX, minY, maxZ},
		{maxX, minY, maxZ}, {minX, minY, maxZ},
		{minX, minY, maxZ}, {minX, minY, minZ},
		// sides
		{minX, minY, minZ}, {minX, maxY, minZ},
		{maxX, minY, minZ}, {maxX, maxY, minZ},
		{maxX, minY, maxZ}, {maxX, maxY, maxZ},
		{minX, minY, maxZ}, {minX, maxY, maxZ},
	}
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

// Scaled is the box with every axis multiplied by its own factor, which is what
// a box around a sprite needs: a sprite's box is in the space of its own quad,
// and the quad is drawn at the size of the picture.
func (a AABB) Scaled(by mgl32.Vec3) AABB {
	perAxis := func(v mgl32.Vec3) mgl32.Vec3 {
		return mgl32.Vec3{v.X() * by.X(), v.Y() * by.Y(), v.Z() * by.Z()}
	}
	return NewAABB(perAxis(a.Min()), perAxis(a.Max()))
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

// Rotated is the smallest axis-aligned box around this one once it has been
// turned. Exact for the quarter turns a piece's facing= ever gives (a multiple
// of 90 degrees stays axis-aligned) and conservative for anything else, which
// is what an axis-aligned box can be about a rotation at all.
//
// It is for the places that hold a box as plain extents and have no transform
// to apply - a frustum test, a debug outline. Anything that carries the
// object's matrix (the collision solver, the picking ray - see OBB) must rotate
// there and not here, or the rotation lands on the box twice.
func (a AABB) Rotated(rotation mgl32.Quat) AABB {
	min := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	max := mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for i := 0; i < 8; i++ {
		corner := mgl32.Vec3{
			a.cornerAxis(i, 0),
			a.cornerAxis(i, 1),
			a.cornerAxis(i, 2),
		}
		turned := rotation.Rotate(corner)
		for axis := 0; axis < 3; axis++ {
			min[axis] = float32(math.Min(float64(min[axis]), float64(turned[axis])))
			max[axis] = float32(math.Max(float64(max[axis]), float64(turned[axis])))
		}
	}
	return NewAABB(min, max)
}

// cornerAxis is one coordinate of one of the box's eight corners: bit `axis` of
// corner index i chooses the min or the max on that axis.
func (a AABB) cornerAxis(corner, axis int) float32 {
	if corner&(1<<axis) != 0 {
		return a.maxCoords[axis]
	}
	return a.minCoords[axis]
}

func (a AABB) Union(b AABB) AABB {
	if a.IsEmpty() {
		return b
	}
	if b.IsEmpty() {
		return a
	}
	perAxis := func(pick func(a, b float64) float64, x, y mgl32.Vec3) mgl32.Vec3 {
		return mgl32.Vec3{
			float32(pick(float64(x.X()), float64(y.X()))),
			float32(pick(float64(x.Y()), float64(y.Y()))),
			float32(pick(float64(x.Z()), float64(y.Z()))),
		}
	}
	return NewAABB(perAxis(math.Min, a.Min(), b.Min()), perAxis(math.Max, a.Max(), b.Max()))
}

// IsEmpty is a box with no room in it along some axis, which is what a frame
// that draws nothing measures.
func (a AABB) IsEmpty() bool {
	for axis := 0; axis < 3; axis++ {
		if a.maxCoords[axis] <= a.minCoords[axis] {
			return true
		}
	}
	return false
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

func Vec3Div(a, b mgl32.Vec3) mgl32.Vec3 {
	return mgl32.Vec3{a.X() / b.X(), a.Y() / b.Y(), a.Z() / b.Z()}
}
func PlaneDistanceToPoint(planeNormal mgl32.Vec3, planePoint mgl32.Vec3, point mgl32.Vec3) float32 {
	return planeNormal.Dot(point.Sub(planePoint))
}
