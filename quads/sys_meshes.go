package quads

import (
	"image"
	"slices"

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
	// visible, nodes and instances are Draw's scratch, kept so a frame does not
	// allocate them.
	visible   []*MeshComponent
	nodes     []*MeshNode
	instances []float32

	// screenAnims is one flipbook clock per meshIndex, not per placed
	// instance - loadGLTF already shares one CompoundMesh across every
	// instance of the same file, so a screen's animation is a property of
	// the model, the same way its geometry is.
	screenAnims map[int]*ScreenAnimation
}

type MeshComponent struct {
	baseComponent
	MeshIndex int
	IsHidden  bool
	// EmissiveStrength pushes this mesh's bright, colourful texels past white, the
	// way an emissive block face is pushed. 0 - the default - means the mesh only
	// reflects light. A torch sets it so its flame glows and blooms.
	EmissiveStrength float32
	// BakedLight is the voxel light level, 0 to 15, this mesh is lit by. It is
	// sampled once where the piece is placed: scenery does not move, and a door
	// sliding a voxel is not worth sampling every frame.
	BakedLight float32
	// NoShadow keeps this mesh out of every depth pass, the way a sprite's does.
	// A level of a few thousand props under a light that moves every frame - the
	// ark's star turns with its sky - would otherwise draw every one of them twice
	// a frame, once for a shadow nothing it stands in can receive.
	NoShadow bool
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
		screenAnims:        make(map[int]*ScreenAnimation),
	}
}

// EnsureScreenAnimation makes every placed instance of meshIndex's model play
// a looping flipbook of the numbered frames in dir on its textureIndex
// material slot - a computer screen turning on. Safe to call once per spawn
// of the piece: the frames are only ever loaded once per meshIndex, the first
// time this is called for it.
func (q *MeshDrawer) EnsureScreenAnimation(meshIndex, textureIndex int, dir string, frameSeconds float64) error {
	if _, wired := q.screenAnims[meshIndex]; wired {
		return nil
	}
	frames, err := LoadScreenFrames(dir)
	if err != nil {
		return err
	}
	q.screenAnims[meshIndex] = NewScreenAnimation(textureIndex, frameSeconds, frames)
	return nil
}

func (q *MeshDrawer) loadGLTF(fileName string) (meshIndex int) {
	if existingIndex, ok := q.nameCache[fileName]; ok {
		return existingIndex
	}
	meshIndex = q.load(fileName)
	q.nameCache[fileName] = meshIndex
	return meshIndex
}

// load reads a model off disk, uploads it and gives it the next index, cached
// or not.
func (q *MeshDrawer) load(fileName string) (meshIndex int) {
	meshIndex = len(q.loadedMeshes)
	mesh := LoadGLTFWithTextures(fileName)
	q.loadedAABBs = append(q.loadedAABBs, voxel.NewAABB(mesh.FindBounds()))
	q.loadedMeshes = append(q.loadedMeshes, mesh)
	mesh.UploadVertexData(q.shader)
	return meshIndex
}

// NewOwnMesh is NewMesh for a piece whose picture is its own: the model is
// loaded again rather than shared out of the cache, so SetTexture on it changes
// this placement and not every copy in the level. A sign is that piece - what
// is painted on it is what it says.
//
// ponytail: a copy per placement is a load and a draw call each, and the copy
// stays loaded after its sign is deleted. Fine for a level's signage; share one
// atlas across them if a level ever wants hundreds.
func (q *MeshDrawer) NewOwnMesh(gltfFile string) (*MeshComponent, voxel.AABB) {
	meshIndex := q.load(gltfFile)
	return NewMeshComponent(meshIndex), q.loadedAABBs[meshIndex]
}

// SetTexture paints one material slot of a loaded model with a picture.
func (q *MeshDrawer) SetTexture(meshIndex, slot int, picture image.Image) error {
	texture, err := glhf.NewTextureFromImage(picture)
	if err != nil {
		return err
	}
	q.loadedMeshes[meshIndex].SetTexture(slot, texture)
	return nil
}

// Mesh is the loaded model and the box around it, loading it if this is the
// first time it has been asked for. The preview renderer wants the model without
// anything placed in the world holding it - see GlApplication.MeshThumbnail - and
// it wants it out of the same cache, so browsing a folder and then placing a
// piece out of it costs one load between them and not two.
//
// The box comes from here rather than from CompoundMesh.FindBounds because a
// model that has been uploaded no longer has its vertices to be measured: the
// bounds are taken once on load, before the upload, and kept.
func (q *MeshDrawer) Mesh(gltfFile string) (*CompoundMesh, voxel.AABB) {
	index := q.loadGLTF(gltfFile)
	return q.loadedMeshes[index], q.loadedAABBs[index]
}

func (q *MeshDrawer) NewMesh(gltfFile string) (*MeshComponent, voxel.AABB) {
	meshIndex := q.loadGLTF(gltfFile)
	meshComponent := NewMeshComponent(meshIndex)

	return meshComponent, q.loadedAABBs[meshIndex]
}

func (q *MeshDrawer) Update(elapsed float64) {
	for meshIndex, anim := range q.screenAnims {
		anim.tick(q.loadedMeshes[meshIndex], elapsed)
	}
	for index, meshes := range q.gameObjectsPerMesh {
		for i := len(meshes) - 1; i >= 0; i-- {
			if meshes[i].IsDead(elapsed) {
				meshes = swapRemove(meshes, i)
			}
		}
		q.gameObjectsPerMesh[index] = meshes
	}
}

// Draw draws every copy of a model part in view with one call. The part's
// texture and vertex array are bound once, each visible copy's transform and
// light go into the part's instance buffer, and the part is drawn that many times
// over. It used to be a draw call and three uniforms per copy - 1800 calls on the
// ark's deck 6 - and on macOS a GL call is paid for in the driver, whatever the
// GPU does with it.
func (q *MeshDrawer) Draw(frustum core.Culler, meshShader *glhf.Shader, forShadows bool) {
	meshShader.Begin()
	q.drawCalls = 0
	q.trianglesDrawn = 0
	for meshIndex, meshes := range q.gameObjectsPerMesh {
		q.visible = q.visible[:0]
		for _, mesh := range meshes {
			if mesh.IsHidden || (forShadows && mesh.NoShadow) || !mesh.GameObject().IsInViewFrustum(frustum) {
				continue
			}
			q.visible = append(q.visible, mesh)
		}
		if len(q.visible) == 0 {
			continue
		}
		model := q.loadedMeshes[meshIndex]
		q.nodes = model.RootNode.appendDrawn(q.nodes[:0])
		for _, node := range q.nodes {
			if len(node.drawPairs) == 0 {
				continue
			}
			q.instances = q.instances[:0]
			for _, mesh := range q.visible {
				model.RootNode.SetParent(mesh.GameObject().Transform())
				transform := node.GetTransformMatrix()
				q.instances = append(q.instances, transform[:]...)
				q.instances = append(q.instances, mesh.EmissiveStrength, mesh.BakedLight)
			}
			for _, pair := range node.drawPairs {
				texture := model.textures[pair.TextureIndex]
				texture.Begin()
				pair.VertexData.Begin()
				if !slices.Equal(pair.uploaded, q.instances) {
					pair.VertexData.SetInstanceData(q.instances)
					pair.uploaded = append(pair.uploaded[:0], q.instances...)
				}
				pair.VertexData.DrawInstanced(int32(len(q.visible)))
				pair.VertexData.End()
				texture.End()
				q.drawCalls++
				q.trianglesDrawn += pair.VertexData.TriangleCount() * len(q.visible)
			}
		}
	}
	meshShader.End()
}

// ForEachCaster visits the world position of every mesh that ends up in a shadow
// map. Used to notice when the casting scene changed.
func (q *MeshDrawer) ForEachCaster(visit func(pos mgl32.Vec3)) {
	for _, meshes := range q.gameObjectsPerMesh {
		for _, mesh := range meshes {
			if mesh.IsHidden || mesh.NoShadow {
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
			// The box as the frustum test sees it, turned and placed - see
			// PhysicsComponent.WorldAABB - so what this draws is what culls.
			mesh.GameObject().Physics().WorldAABB().QueueDebugDraw(mgl32.Ident4())
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
