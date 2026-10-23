package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxelfps/quads/voxel"
)

type UnitCube struct {
	mesh      *CompoundMesh
	shader    *glhf.Shader
	parent    *core.Transform
	positions map[voxel.Int3]mgl32.Vec4
}

func NewUnitCube(shader *glhf.Shader, mesh *CompoundMesh) *UnitCube {
	parent := core.NewDefaultTransform()
	mesh.RootNode.SetParent(parent)
	return &UnitCube{
		mesh:      mesh,
		shader:    shader,
		parent:    parent,
		positions: make(map[voxel.Int3]mgl32.Vec4),
	}
}

func (u *UnitCube) QueueDraw(positions []voxel.Int3) {
	for _, pos := range positions {
		u.positions[pos] = mgl32.Vec4{1, 1, 1, 1}
	}
}

func (u *UnitCube) DrawAllQueued() {
	u.shader.Begin()
	for pos, _ := range u.positions {
		u.parent.SetPosition(pos.ToBlockCenterVec3D())
		u.mesh.Draw()
	}
	u.shader.End()
	clear(u.positions)
}
