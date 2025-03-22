package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
)

type MeshDrawer struct {
    shader             *glhf.Shader
    gameObjectsPerMesh map[int]map[*MeshComponent]bool
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
        gameObjectsPerMesh: make(map[int]map[*MeshComponent]bool),
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

func (q *MeshDrawer) Draw(camera *core.FPSCamera, elapsed float64) {
    q.shader.Begin()
    q.drawCalls = 0
    q.trianglesDrawn = 0
    for _, meshes := range q.gameObjectsPerMesh {
        for mesh := range meshes {
            if mesh.IsDead(elapsed) {
                q.Remove(mesh)
                continue
            }
            if mesh.IsHidden || !mesh.GameObject().IsInViewFrustum(camera) {
                continue
            }
            compoundMesh := q.loadedMeshes[mesh.MeshIndex]
            compoundMesh.RootNode.SetParent(mesh.GameObject().Transform())
            compoundMesh.Draw(q.shader, MeshShaderModelTransform)
            q.trianglesDrawn += compoundMesh.TriangleCount()
            q.drawCalls += compoundMesh.DrawCalls()
        }
    }
    q.shader.End()
}

func (q *MeshDrawer) Add(mesh *MeshComponent) {
    if _, ok := q.gameObjectsPerMesh[mesh.MeshIndex]; !ok {
        q.gameObjectsPerMesh[mesh.MeshIndex] = make(map[*MeshComponent]bool)
    }
    q.gameObjectsPerMesh[mesh.MeshIndex][mesh] = true
}

func (q *MeshDrawer) Remove(mesh *MeshComponent) {
    delete(q.gameObjectsPerMesh[mesh.MeshIndex], mesh)
}

func (q *MeshDrawer) DrawAABBs(lineShader *glhf.Shader) {
    for _, meshes := range q.gameObjectsPerMesh {
        for mesh := range meshes {
            if !mesh.GameObject().HasComponent(ComponentTypePhysics) {
                continue
            }
            lineShader.SetUniformAttr(1, mesh.GameObject().Transform().GetTranslationMatrix())
            collObj := mesh.GameObject().Physics()
            collObj.AABBAt(mgl32.Vec3{}).Draw(lineShader)
        }
    }
}

func (q *MeshDrawer) DrawOBBs(lineShader *glhf.Shader) {
    for _, meshes := range q.gameObjectsPerMesh {
        for mesh := range meshes {
            if !mesh.GameObject().HasComponent(ComponentTypePhysics) {
                continue
            }
            collObj := mesh.GameObject().Physics()
            lineShader.SetUniformAttr(1, mesh.Transform())
            collObj.AABBAt(mgl32.Vec3{}).Draw(lineShader)
        }
    }
}

func (q *MeshDrawer) Clear() {
    q.gameObjectsPerMesh = make(map[int]map[*MeshComponent]bool)
}

func (q *MeshDrawer) DrawnLastFrame() (triangles, drawCalls int) {
    return q.trianglesDrawn, q.drawCalls
}
