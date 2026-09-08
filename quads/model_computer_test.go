package quads

import (
	"path/filepath"
	"testing"

	"github.com/qmuntal/gltf"
)

// TestComputerScreenIsItsOwnMaterial is the load-bearing assumption behind
// the whole animated-screen feature: the Screen cube has to compile to a
// different glTF material/texture index than the housing it sits on, or
// MeshDrawer.EnsureScreenAnimation would end up repainting the whole prop
// instead of just its screen. See AGENTS.md's Blockbench section for how
// easy this kind of thing is to get wrong invisibly.
func TestComputerScreenIsItsOwnMaterial(t *testing.T) {
	path := filepath.Join("..", "scenery", "computer", "mesh.gltf")
	mesh := LoadGLTF(path, nil, nil)
	if mesh == nil {
		t.Fatal("scenery/computer/mesh.gltf did not load")
	}
	if got := mesh.TriangleCount(); got == 0 {
		t.Fatal("computer mesh has no triangles")
	}

	doc, err := gltf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Scenes[0].Nodes) != 1 {
		t.Fatalf("scene has %d top-level nodes, want 1 (everything should be grouped under one root)", len(doc.Scenes[0].Nodes))
	}
	if got := len(doc.Materials); got != 2 {
		t.Fatalf("materials: got %d, want 2 (body and screen)", got)
	}

	screenNode, found := mesh.GetNodeByName("Screen")
	if !found {
		t.Fatal("no node named Screen")
	}
	housingNode, found := mesh.GetNodeByName("Housing")
	if !found {
		t.Fatal("no node named Housing")
	}
	// mesh.SubMeshes, not drawPairs: ConvertVertexData (which fills drawPairs)
	// uploads to the GPU, and this test has no GL context. SubMeshes already
	// carries the material's texture index straight off the parsed document.
	if screenNode.mesh == nil || housingNode.mesh == nil || len(screenNode.mesh.SubMeshes) != 1 || len(housingNode.mesh.SubMeshes) != 1 {
		t.Fatalf("expected one submesh each, got screen=%v housing=%v", screenNode.mesh, housingNode.mesh)
	}
	screenTextureIndex := screenNode.mesh.SubMeshes[0].TextureIndex
	housingTextureIndex := housingNode.mesh.SubMeshes[0].TextureIndex
	if screenTextureIndex == housingTextureIndex {
		t.Fatalf("Screen and Housing share texture index %d - EnsureScreenAnimation would animate the whole prop", screenTextureIndex)
	}

	// scenery.json declares "material": 1 for the screen slot - keep that
	// number honest against what the model actually compiled to.
	const declaredScreenMaterial = 1
	if screenTextureIndex != declaredScreenMaterial {
		t.Errorf("Screen's texture index is %d, but scenery/computer/scenery.json declares material %d", screenTextureIndex, declaredScreenMaterial)
	}
}
