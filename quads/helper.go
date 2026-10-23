package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxelfps/quads/voxel"
	"math"
)

func NewUInt32QuadIndexedVerts() ([]uint32, []glhf.GlUInt) {
	var indices []uint32
	offset := 0
	indices = append(indices, uint32(offset+1), uint32(offset+0), uint32(offset+2), uint32(offset+2), uint32(offset+0), uint32(offset+3))
	quadVertices := []glhf.GlUInt{
		0, 1, 2, 3,
	}
	return indices, quadVertices
}

func TransformVertex(x, y float32, cam core.Camera, projViewInverted mgl32.Mat4) mgl32.Vec3 {
	normalizedNearPos := mgl32.Vec4{x, y, cam.GetNearPlaneDist(), 1}
	// project point from camera space to world space
	nearWorldPos := projViewInverted.Mul4x1(normalizedNearPos)
	// perspective divide
	correctedNearWorldPos := nearWorldPos.Vec3().Mul(1 / nearWorldPos.W())
	return correctedNearWorldPos
}

func HudCornersFromOBB(aabb voxel.AABB, objectTransformationMatrix mgl32.Mat4, projectWorldToScreenPixels func(mgl32.Vec3) (int, int)) ([2]int, [2]int, [2]int, [2]int) {
	// Extract translation and rotation (ignore scaling if uniform)
	rotation := objectTransformationMatrix.Mat3()           // Extracts upper-left 3x3 matrix
	translation := objectTransformationMatrix.Col(3).Vec3() // Extracts translation vector

	// Compute the AABB center and extents
	center := (aabb.Min().Add(aabb.Max())).Mul(0.5)
	extents := (aabb.Max().Sub(aabb.Min())).Mul(0.5)

	// Transform center to world space
	worldCenter := rotation.Mul3x1(center).Add(translation)

	// Generate transformed screen-space coordinates for each corner
	minX, minY := math.MaxInt, math.MaxInt
	maxX, maxY := math.MinInt, math.MinInt

	for i := 0; i < 8; i++ {
		offset := mgl32.Vec3{
			extents.X() * float32(1-2*((i>>0)&1)), // Flip X
			extents.Y() * float32(1-2*((i>>1)&1)), // Flip Y
			extents.Z() * float32(1-2*((i>>2)&1)), // Flip Z
		}

		worldPos := worldCenter.Add(rotation.Mul3x1(offset)) // Transform offset
		x, y := projectWorldToScreenPixels(worldPos)

		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	return [2]int{minX, minY}, [2]int{maxX, minY}, [2]int{minX, maxY}, [2]int{maxX, maxY}
}

const emptyFragmentShaderSource = `#version 400 core
void main() {}`
