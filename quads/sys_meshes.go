package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
)

type MeshDrawer struct {
	shader             *glhf.Shader
	gameObjectsPerMesh map[int][]*MeshComponent
	loadedMeshes       []*CompoundMesh
	loadedAABBs        []voxel.AABB
	nameCache          map[string]int
	drawCalls          int
	trianglesDrawn     int
}

type MeshComponent struct {
	baseComponent
	MeshIndex int
	IsHidden  bool
	// EmissiveStrength pushes this mesh's bright, colourful texels past white, the
	// way an emissive block face is pushed. 0 - the default - means the mesh only
	// reflects light. A torch sets it so its flame glows and blooms.
	EmissiveStrength float32
}

func NewMeshComponent(meshIndex int) *MeshComponent {
	return &MeshComponent{
		MeshIndex: meshIndex,
	}
}

func (m *MeshComponent) Type() ComponentType {
	return ComponentTypeMesh
}

func (m *MeshComponent) Transform() mgl32.Mat4 {
	return m.GameObject().Transform().GetTransformMatrix()
}

func NewMeshDrawer(shader *glhf.Shader) *MeshDrawer {
	return &MeshDrawer{
		shader:             shader,
		gameObjectsPerMesh: make(map[int][]*MeshComponent),
		nameCache:          make(map[string]int),
	}
}

func (q *MeshDrawer) loadGLTF(fileName string) (meshIndex int) {
	if existingIndex, ok := q.nameCache[fileName]; ok {
		return existingIndex
	}
	meshIndex = len(q.loadedMeshes)

	mesh := LoadGLTFWithTextures(fileName)
	q.loadedAABBs = append(q.loadedAABBs, voxel.NewAABB(mesh.FindBounds()))

	q.loadedMeshes = append(q.loadedMeshes, mesh)

	q.nameCache[fileName] = meshIndex

	mesh.UploadVertexData(q.shader)

	return meshIndex
}

func (q *MeshDrawer) NewMesh(gltfFile string) (*MeshComponent, voxel.AABB) {
	meshIndex := q.loadGLTF(gltfFile)
	meshComponent := NewMeshComponent(meshIndex)

	return meshComponent, q.loadedAABBs[meshIndex]
}

func (q *MeshDrawer) Update(elapsed float64) {
	for index, meshes := range q.gameObjectsPerMesh {
		for i := len(meshes) - 1; i >= 0; i-- {
			if meshes[i].IsDead(elapsed) {
				meshes = swapRemove(meshes, i)
			}
		}
		q.gameObjectsPerMesh[index] = meshes
	}
}

func (q *MeshDrawer) Draw(frustum core.Culler, meshShader *glhf.Shader) {
	meshShader.Begin()
	q.drawCalls = 0
	q.trianglesDrawn = 0
	for _, meshes := range q.gameObjectsPerMesh {
		for _, mesh := range meshes {
			if mesh.IsHidden || !mesh.GameObject().IsInViewFrustum(frustum) {
				continue
			}
			if meshShader.HasUniform(MeshShaderEmissiveStrength) {
				meshShader.SetUniformAttr(MeshShaderEmissiveStrength, mesh.EmissiveStrength)
			}
			compoundMesh := q.loadedMeshes[mesh.MeshIndex]
			compoundMesh.RootNode.SetParent(mesh.GameObject().Transform())
			compoundMesh.Draw(meshShader, MeshShaderModelTransform)
			q.trianglesDrawn += compoundMesh.TriangleCount()
			q.drawCalls += compoundMesh.DrawCalls()
		}
	}
	meshShader.End()
}

// ForEachCaster visits the world position of every mesh that ends up in a shadow
// map. Used to notice when the casting scene changed.
func (q *MeshDrawer) ForEachCaster(visit func(pos mgl32.Vec3)) {
	for _, meshes := range q.gameObjectsPerMesh {
		for _, mesh := range meshes {
			if mesh.IsHidden {
				continue
			}
			visit(mesh.GameObject().Transform().GetPosition())
		}
	}
}

func (q *MeshDrawer) Add(mesh *MeshComponent) {
	q.gameObjectsPerMesh[mesh.MeshIndex] = append(q.gameObjectsPerMesh[mesh.MeshIndex], mesh)
}

func (q *MeshDrawer) Remove(mesh *MeshComponent) {
	list := q.gameObjectsPerMesh[mesh.MeshIndex]
	for i, candidate := range list {
		if candidate == mesh {
			q.gameObjectsPerMesh[mesh.MeshIndex] = swapRemove(list, i)
			return
		}
	}
}

func (q *MeshDrawer) DrawAABBs(frustum core.Culler, lineShader *glhf.Shader) {
	defer voxel.DrawDebugLines(lineShader)
	for _, meshes := range q.gameObjectsPerMesh {
		for _, mesh := range meshes {
			if !hasDebugBox(mesh, frustum) {
				continue
			}
			mesh.GameObject().Physics().AABBAt(mgl32.Vec3{}).
				QueueDebugDraw(mesh.GameObject().Transform().GetTranslationMatrix())
		}
	}
}

func (q *MeshDrawer) DrawOBBs(frustum core.Culler, lineShader *glhf.Shader) {
	defer voxel.DrawDebugLines(lineShader)
	for _, meshes := range q.gameObjectsPerMesh {
		for _, mesh := range meshes {
			if !hasDebugBox(mesh, frustum) {
				continue
			}
			mesh.GameObject().Physics().AABBAt(mgl32.Vec3{}).QueueDebugDraw(mesh.Transform())
		}
	}
}

// hasDebugBox is whether this mesh has a box to draw and is somewhere the player
// can see it - the same two questions the mesh pass itself asks.
func hasDebugBox(mesh *MeshComponent, frustum core.Culler) bool {
	return !mesh.IsHidden &&
		mesh.GameObject().HasComponent(ComponentTypePhysics) &&
		mesh.GameObject().IsInViewFrustum(frustum)
}

func (q *MeshDrawer) Clear() {
	q.gameObjectsPerMesh = make(map[int][]*MeshComponent)
}

func (q *MeshDrawer) DrawnLastFrame() (triangles, drawCalls int) {
	return q.trianglesDrawn, q.drawCalls
}
