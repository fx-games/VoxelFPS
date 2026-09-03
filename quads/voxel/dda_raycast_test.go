package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "math"
    "testing"
)

const testRange = 1000.0

// solidAt returns a stopRay function for a world where only the given positions
// are solid.
func solidAt(t *testing.T, solid ...Int3) func(x, y, z int32) bool {
    t.Helper()
    return func(x, y, z int32) bool {
        for _, s := range solid {
            if s.X == x && s.Y == y && s.Z == z {
                return true
            }
        }
        return false
    }
}

func TestDDARaycastAxisAligned(t *testing.T) {
    tests := []struct {
        name         string
        direction    mgl32.Vec3
        solid        Int3
        wantSide     CubeSide
        wantPrevious Int3
        wantDistance float64
    }{
        {"east", mgl32.Vec3{1, 0, 0}, Int3{3, 0, 0}, West, Int3{2, 0, 0}, 2.5},
        {"west", mgl32.Vec3{-1, 0, 0}, Int3{-2, 0, 0}, East, Int3{-1, 0, 0}, 1.5},
        {"up", mgl32.Vec3{0, 1, 0}, Int3{0, 2, 0}, Bottom, Int3{0, 1, 0}, 1.5},
        {"down", mgl32.Vec3{0, -1, 0}, Int3{0, -3, 0}, Top, Int3{0, -2, 0}, 2.5},
        {"south", mgl32.Vec3{0, 0, 1}, Int3{0, 0, 4}, North, Int3{0, 0, 3}, 3.5},
        {"north", mgl32.Vec3{0, 0, -1}, Int3{0, 0, -1}, South, Int3{0, 0, 0}, 0.5},
    }
    start := mgl32.Vec3{0.5, 0.5, 0.5}
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            hit := DDARaycast(start, test.direction, testRange, solidAt(t, test.solid))
            if !hit.Hit {
                t.Fatal("expected a hit")
            }
            if hit.CollisionGridPosition != test.solid {
                t.Errorf("hit voxel = %v, want %v", hit.CollisionGridPosition, test.solid)
            }
            if hit.Side != test.wantSide {
                t.Errorf("side = %v, want %v", hit.Side, test.wantSide)
            }
            if hit.PreviousGridPosition != test.wantPrevious {
                t.Errorf("previous voxel = %v, want %v", hit.PreviousGridPosition, test.wantPrevious)
            }
            if math.Abs(hit.Distance-test.wantDistance) > 1e-5 {
                t.Errorf("distance = %f, want %f", hit.Distance, test.wantDistance)
            }
            // The entered face must be the one the ray came from.
            entered := test.wantSide.AsDirection()
            if test.solid.Add(entered) != test.wantPrevious {
                t.Errorf("side %v points to %v, but the ray came from %v", test.wantSide, test.solid.Add(entered), test.wantPrevious)
            }
        })
    }
}

// The ray starts inside a solid voxel: it hits immediately, at distance 0.
func TestDDARaycastStartsInsideSolid(t *testing.T) {
    hit := DDARaycast(mgl32.Vec3{0.5, 0.5, 0.5}, mgl32.Vec3{1, 0, 0}, testRange, solidAt(t, Int3{0, 0, 0}))
    if !hit.Hit || hit.Distance != 0 {
        t.Fatalf("expected an immediate hit at distance 0, got %+v", hit)
    }
    if hit.CollisionGridPosition != (Int3{0, 0, 0}) {
        t.Errorf("hit voxel = %v, want origin voxel", hit.CollisionGridPosition)
    }
}

// A diagonal ray reports a distance in ray-direction units, so the reported
// world position must be start + direction * distance and sit on the surface of
// the voxel that was hit.
func TestDDARaycastDiagonal(t *testing.T) {
    start := mgl32.Vec3{0.5, 0.5, 0.5}
    direction := mgl32.Vec3{1, 0, 1}.Normalize()
    solid := Int3{3, 0, 3}

    hit := DDARaycast(start, direction, testRange, solidAt(t, solid))

    if !hit.Hit {
        t.Fatal("expected a hit")
    }
    if hit.CollisionGridPosition != solid {
        t.Fatalf("hit voxel = %v, want %v", hit.CollisionGridPosition, solid)
    }
    want := start.Add(direction.Mul(float32(hit.Distance)))
    if hit.CollisionWorldPosition.Sub(want).Len() > 1e-5 {
        t.Errorf("world position = %v, want %v", hit.CollisionWorldPosition, want)
    }
    // Entering the voxel at {3,0,3} means touching its x=3 or z=3 plane.
    p := hit.CollisionWorldPosition
    onFace := math.Abs(float64(p.X())-3) < 1e-5 || math.Abs(float64(p.Z())-3) < 1e-5
    if !onFace {
        t.Errorf("world position %v is not on a face of voxel %v", p, solid)
    }
    if p.X() < 3 && p.Z() < 3 {
        t.Errorf("world position %v lies before the hit voxel %v", p, solid)
    }
}

// A ray through empty space stops at maxDistance instead of looping forever,
// and a solid voxel beyond that distance is not reported as a hit.
func TestDDARaycastMiss(t *testing.T) {
    start := mgl32.Vec3{0.5, 0.5, 0.5}
    direction := mgl32.Vec3{1, 0, 0}

    if hit := DDARaycast(start, direction, 10, solidAt(t)); hit.Hit {
        t.Errorf("expected a miss in an empty world, got %+v", hit)
    }
    if hit := DDARaycast(start, direction, 10, solidAt(t, Int3{50, 0, 0})); hit.Hit {
        t.Errorf("expected a miss for a voxel beyond the range, got %+v", hit)
    }
    if hit := DDARaycast(start, direction, 10, solidAt(t, Int3{5, 0, 0})); !hit.Hit {
        t.Error("expected a hit for a voxel within the range")
    }
}

// Every side must map to the direction of the neighbour on that side.
func TestCubeSideAsDirection(t *testing.T) {
    opposites := map[CubeSide]CubeSide{East: West, Top: Bottom, South: North}
    for side, opposite := range opposites {
        sum := side.AsDirection().Add(opposite.AsDirection())
        if sum != (Int3{}) {
            t.Errorf("%v and %v are not opposite: %v", side, opposite, sum)
        }
    }
}
