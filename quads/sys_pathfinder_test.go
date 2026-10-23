package quads

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/geometry"
)

// A 6x6 ground floor (one voxel solid at y=0, so groundHeight is 1 everywhere)
// with a 4-voxel tower at x in [4,5]: too tall for climbStep, so only a
// ClimbLink can reach it.
func towerIsSolid(x, y, z int32) bool {
	if x < 0 || x >= 6 || z < 0 || z >= 6 {
		return false
	}
	height := int32(1)
	if x >= 4 {
		height = 4
	}
	return y < height
}

func TestPathfindingCannotCrossAFourVoxelCliffWithoutAClimbLink(t *testing.T) {
	pf := NewPathfindingSystem(towerIsSolid, 8)
	path := pf.calculatePath(mgl32.Vec3{1.5, 1, 1.5}, mgl32.Vec3{4.5, 4, 1.5})
	if path != nil {
		t.Fatalf("expected no path across a 3-voxel rise with no ladder, got %v", path)
	}
}

func TestPathfindingClimbsARegisteredLadder(t *testing.T) {
	pf := NewPathfindingSystem(towerIsSolid, 8)
	bottom := geometry.Point{X: 3, Y: 1}
	top := geometry.Point{X: 4, Y: 1}
	pf.AddClimbLink(bottom, top, 1, 4)

	path := pf.calculatePath(mgl32.Vec3{1.5, 1, 1.5}, mgl32.Vec3{4.5, 4, 1.5})
	if path == nil {
		t.Fatal("expected a path via the ladder, got none")
	}

	sawClimb := false
	for _, wp := range path {
		if wp.Climb {
			sawClimb = true
			if wp.Pos.Y() != 4 {
				t.Errorf("climb waypoint should land at the ladder's top height 4, got %.1f", wp.Pos.Y())
			}
		}
	}
	if !sawClimb {
		t.Errorf("path %v never uses the ladder", path)
	}

	last := path[len(path)-1]
	if int(last.Pos.X()) != 4 || int(last.Pos.Z()) != 1 {
		t.Errorf("path should end on the tower tile (4,1), ended at %v", last.Pos)
	}
}

// The "arrived" flag is what stops PathfindingSystem.Update from re-running
// A* every frame for an idle actor - see PathingComponent.HasPathToDestination.
func TestPathingComponentStaysArrivedUntilGivenANewDestination(t *testing.T) {
	p := NewPathingComponent()
	p.SetDestination(mgl32.Vec3{5, 1, 5})
	if p.HasArrived() {
		t.Fatal("should not be arrived before a path is even set")
	}
	p.SetWaypoints(nil)
	if !p.HasArrived() {
		t.Fatal("an empty waypoint list from calculatePath (already there) should count as arrived")
	}
	if !p.HasPathToDestination() {
		t.Fatal("arrived should short-circuit HasPathToDestination so Update does not re-path every frame")
	}
	p.SetDestination(mgl32.Vec3{9, 1, 9})
	if p.HasArrived() {
		t.Fatal("a new destination should clear arrived")
	}
}

// A diagonal walk across open flat ground has nothing between the start and
// the end to actually block a straight line, so pruneLineOfSight should
// collapse A*'s zigzag of single-axis grid steps down to just the
// destination - not the handful of intermediate grid-aligned waypoints a raw
// A* path would otherwise produce.
func TestPathfindingPrunesZigzagOnOpenGround(t *testing.T) {
	pf := NewPathfindingSystem(towerIsSolid, 8)
	path := pf.calculatePath(mgl32.Vec3{0.5, 1, 0.5}, mgl32.Vec3{3.5, 1, 3.5})
	if len(path) != 1 {
		t.Fatalf("expected the open-ground diagonal to prune to a single waypoint, got %v", path)
	}
}
