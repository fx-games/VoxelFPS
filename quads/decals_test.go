package quads

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// A decal stuck to a moving GameObject - a door's own panel, say - has to
// move with it rather than staying wherever it was when it was stuck on,
// which is what parent is for; see the decal type's own doc comment.
func TestDecalOnAMovingParentFollowsIt(t *testing.T) {
	door := NewGameObject("door")
	door.Transform().SetPosition(mgl32.Vec3{5, 0, 5})

	drawer := &SpriteDrawer{}
	hitPos := mgl32.Vec3{5, 0.5, 5.03} // just off the panel's front face
	normal := mgl32.Vec3{0, 0, 1}
	drawer.addDecal(nil, 0, hitPos, normal, 0.2, 0.2, 0, 15, door)
	wantAtSpawn := hitPos.Add(normal.Mul(decalSurfaceOffset)) // AddDecal's own lift off the surface

	if got := len(drawer.decals); got != 1 {
		t.Fatalf("len(decals) = %d, want 1", got)
	}
	_, centerAtSpawn := drawer.decals[0].worldSpace()
	if got := centerAtSpawn.Sub(wantAtSpawn).Len(); got > 0.001 {
		t.Fatalf("decal centre = %v, want it at %v (diff %v)", centerAtSpawn, wantAtSpawn, got)
	}

	door.Transform().SetPosition(mgl32.Vec3{5, 1, 5}) // the door slides up one voxel
	_, centerAfterMove := drawer.decals[0].worldSpace()
	want := wantAtSpawn.Add(mgl32.Vec3{0, 1, 0})
	if got := centerAfterMove.Sub(want).Len(); got > 0.001 {
		t.Errorf("decal centre after the door moved = %v, want %v (it should have moved with the door)", centerAfterMove, want)
	}
}

// A decal with no parent - the overwhelming majority, stuck to a wall - stays
// exactly where it was put, unaffected by anything else's transform.
func TestDecalWithNoParentIsStatic(t *testing.T) {
	drawer := &SpriteDrawer{}
	hitPos := mgl32.Vec3{5, 0.5, 5.03}
	normal := mgl32.Vec3{0, 0, 1}
	drawer.addDecal(nil, 0, hitPos, normal, 0.2, 0.2, 0, 15, nil)
	want := hitPos.Add(normal.Mul(decalSurfaceOffset))

	_, center := drawer.decals[0].worldSpace()
	if got := center.Sub(want).Len(); got > 0.001 {
		t.Fatalf("decal centre = %v, want it at %v (diff %v)", center, want, got)
	}
}
