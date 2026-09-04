package quads

import "testing"

func meshComponent(t *testing.T, name string, meshIndex int) *MeshComponent {
    t.Helper()
    object := NewGameObject(name)
    mesh := NewMeshComponent(meshIndex)
    object.SetComponent(mesh)
    return mesh
}

// The draw lists are slices now, removed by swapping in the last element.
// Removing anything but the last one is where that goes wrong.
func TestMeshDrawerRemoveKeepsTheRest(t *testing.T) {
    drawer := &MeshDrawer{gameObjectsPerMesh: make(map[int][]*MeshComponent)}

    first := meshComponent(t, "first", 0)
    second := meshComponent(t, "second", 0)
    third := meshComponent(t, "third", 0)
    for _, mesh := range []*MeshComponent{first, second, third} {
        drawer.Add(mesh)
    }

    drawer.Remove(second)

    list := drawer.gameObjectsPerMesh[0]
    if len(list) != 2 {
        t.Fatalf("len = %d, want 2", len(list))
    }
    for _, mesh := range list {
        if mesh == second {
            t.Error("the removed mesh is still in the draw list")
        }
    }

    // Removing something that is not in the list must not touch it.
    drawer.Remove(second)
    if len(drawer.gameObjectsPerMesh[0]) != 2 {
        t.Errorf("len = %d after removing twice, want 2", len(drawer.gameObjectsPerMesh[0]))
    }
}

// Update drops dead meshes, and dropping one must not skip its neighbour.
func TestMeshDrawerUpdateDropsDeadMeshes(t *testing.T) {
    drawer := &MeshDrawer{gameObjectsPerMesh: make(map[int][]*MeshComponent)}

    alive := meshComponent(t, "alive", 0)
    deadA := meshComponent(t, "deadA", 0)
    deadB := meshComponent(t, "deadB", 0)
    for _, mesh := range []*MeshComponent{alive, deadA, deadB} {
        drawer.Add(mesh)
    }
    deadA.GameObject().Kill()
    deadB.GameObject().Kill()

    drawer.Update(0.016)

    list := drawer.gameObjectsPerMesh[0]
    if len(list) != 1 || list[0] != alive {
        t.Fatalf("draw list = %v, want just the living mesh", list)
    }
}
