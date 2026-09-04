package quads

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
	"math"
)

type MeshCollider struct {
	VertexData             []glhf.GlFloat // vertex data format: posX, posY, posZ, normalX, normalY, normalZ, texU, texV
	TransformFunc          func() mgl32.Mat4
	name                   string
	velocity               mgl32.Vec3
	VertexCount            int
	VertexIndices          []uint32
	VertexFormatComponents uint32
}

func (m *MeshCollider) SetName(name string) {
	m.name = name
}

func (m *MeshCollider) GetName() string {
	return m.name
}
func (m *MeshCollider) IterateTrianglesTransformed(callback func(triangle [3]mgl32.Vec3)) {
	transformMatrix := m.TransformFunc()
	transformVertex := func(x, y, z glhf.GlFloat) mgl32.Vec3 {
		return transformMatrix.Mul4x1(mgl32.Vec4{float32(x), float32(y), float32(z), 1}).Vec3()
	}
	/*
		{Name: "position", Type: glhf.Vec3},
		{Name: "texCoord", Type: glhf.Vec2},
		{Name: "vertexColor", Type: glhf.Vec3},
		{Name: "normal", Type: glhf.Vec3},
	*/

	stride := uint32(m.VertexFormatComponents)
	if m.VertexIndices != nil {
		for i := 0; i < len(m.VertexIndices); i += 3 {
			a := transformVertex(m.VertexData[m.VertexIndices[i]*stride+0], m.VertexData[m.VertexIndices[i]*stride+1], m.VertexData[m.VertexIndices[i]*stride+2])
			b := transformVertex(m.VertexData[m.VertexIndices[i+1]*stride+0], m.VertexData[m.VertexIndices[i+1]*stride+1], m.VertexData[m.VertexIndices[i+1]*stride+2])
			c := transformVertex(m.VertexData[m.VertexIndices[i+2]*stride+0], m.VertexData[m.VertexIndices[i+2]*stride+1], m.VertexData[m.VertexIndices[i+2]*stride+2])
			callback([3]mgl32.Vec3{a, b, c})
		}
	} else {
		for i := 0; i < len(m.VertexData); i += int(stride) * 3 {
			a := transformVertex(m.VertexData[i+0], m.VertexData[i+1], m.VertexData[i+2])
			b := transformVertex(m.VertexData[i+int(stride)+0], m.VertexData[i+int(stride)+1], m.VertexData[i+int(stride)+2])
			c := transformVertex(m.VertexData[i+int(stride)*2+0], m.VertexData[i+int(stride)*2+1], m.VertexData[i+int(stride)*2+2])
			callback([3]mgl32.Vec3{a, b, c})
		}
	}
}
func (m *MeshCollider) IntersectsRay(rayStart, rayEnd mgl32.Vec3) (bool, mgl32.Vec3) {
	//partName := m.name
	//println(fmt.Sprintf("[IntersectsRay] %s", m.name))
	minDist := float32(math.MaxFloat32)
	doesIntersect := false
	nearestIntersection := mgl32.Vec3{0, 0, 0}
	m.IterateTrianglesTransformed(func(triangle [3]mgl32.Vec3) {
		//println(fmt.Sprintf("[IntersectsRay] --> Tri. %v", triangle))
		intersection, atPoint := intersectLineSegmentTriangle(rayStart, rayEnd, triangle[0], triangle[1], triangle[2])
		if intersection {
			doesIntersect = true
			dist := atPoint.Sub(rayStart).Len()
			if dist < minDist {
				minDist = dist
				nearestIntersection = atPoint
			}
		}
	})
	return doesIntersect, nearestIntersection
}

// bench: no allocs, 65ns/op
func intersectLineSegmentTriangle(rayStart, rayEnd mgl32.Vec3, v0, v1, v2 mgl32.Vec3) (bool, mgl32.Vec3) {
	const EPSILON = 0.000001

	direction := rayEnd.Sub(rayStart)
	edge1 := v1.Sub(v0)
	edge2 := v2.Sub(v0)

	h := direction.Cross(edge2)
	a := edge1.Dot(h)

	if a > -EPSILON && a < EPSILON {
		return false, mgl32.Vec3{} // This ray is parallel to this triangle.
	}

	f := 1.0 / a
	s := rayStart.Sub(v0)
	u := f * s.Dot(h)

	if u < 0.0 || u > 1.0 {
		return false, mgl32.Vec3{} // Intersection is outside the triangle
	}

	q := s.Cross(edge1)
	v := f * direction.Dot(q)

	if v < 0.0 || u+v > 1.0 {
		return false, mgl32.Vec3{} // Intersection is outside the triangle
	}

	t := f * edge2.Dot(q)

	if t > EPSILON {
		return true, rayStart.Add(direction.Mul(t)) // Intersection
	}

	return false, mgl32.Vec3{} // This means that there is a line intersection but not a ray intersection.
}

func (m *MeshCollider) Draw() {

}

func (m *MeshCollider) ToString() string {
	return fmt.Sprintf("MeshCollider{FirstVertex = %v, Transformed = %v}", m.VertexData[0:3], m.TransformFunc().Mul4x1(mgl32.Vec4{float32(m.VertexData[0]), float32(m.VertexData[1]), float32(m.VertexData[2]), 1}).Vec3())
}

func (m *MeshCollider) FindFurthestPoint(direction mgl32.Vec3) mgl32.Vec3 {
	var maxPoint mgl32.Vec3
	isSweep := m.velocity.Len() > 0
	var maxDistance = -math.MaxFloat64
	transformMatrix := m.TransformFunc()
	for i := 0; i < len(m.VertexData); i += 8 {
		vertex := mgl32.Vec3{float32(m.VertexData[i]), float32(m.VertexData[i+1]), float32(m.VertexData[i+2])}
		vertex = transformMatrix.Mul4x1(vertex.Vec4(1)).Vec3()
		distance := float64(vertex.Dot(direction))
		if distance > maxDistance {
			maxDistance = distance
			maxPoint = vertex
		}
		if isSweep {
			// translate the vertex by the velocity
			// and check if it is further away
			// than the current maxPoint
			sweptVertex := vertex.Add(m.velocity)
			sweptDistance := float64(sweptVertex.Dot(direction))
			if sweptDistance > maxDistance {
				maxDistance = sweptDistance
				maxPoint = sweptVertex
			}
		}
	}
	return maxPoint
}

func (m *MeshCollider) SetVelocityForSweep(velocity mgl32.Vec3) Collider {
	m.velocity = velocity
	return m
}

func Support(a, b Collider, direction mgl32.Vec3) mgl32.Vec3 {
	// the direction is relative to the origin
	// but our vertices are relative to the center of the object
	// or is the point, that our support is now relative to the origin?
	return a.FindFurthestPoint(direction).Sub(b.FindFurthestPoint(direction.Mul(-1)))
}

type Collider interface {
	FindFurthestPoint(direction mgl32.Vec3) mgl32.Vec3
	ToString() string
	Draw()
	GetName() string
	SetName(name string)
	IntersectsRay(start mgl32.Vec3, end mgl32.Vec3) (bool, mgl32.Vec3)
}
