package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "math"
    "testing"
)

func boxAt(center mgl32.Vec3, halfExtent float32) AABB {
    return NewAABB(
        center.Sub(mgl32.Vec3{halfExtent, halfExtent, halfExtent}),
        center.Add(mgl32.Vec3{halfExtent, halfExtent, halfExtent}),
    )
}

func assertCollision(t *testing.T, got CollisionInfo, wantTimeOfImpact float32, wantNormal mgl32.Vec3) {
    t.Helper()
    if !got.IsCollision {
        t.Fatalf("expected a collision, got %+v", got)
    }
    if math.Abs(float64(got.TimeOfImpact-wantTimeOfImpact)) > 1e-4 {
        t.Errorf("time of impact = %f, want %f", got.TimeOfImpact, wantTimeOfImpact)
    }
    if got.SurfaceNormal.Sub(wantNormal).Len() > 1e-4 {
        t.Errorf("surface normal = %v, want %v", got.SurfaceNormal, wantNormal)
    }
}

// An OBB that is not rotated has to behave exactly like the plain AABB sweep.
func TestCheckAABBVsOBBMatchesSweepAABB(t *testing.T) {
    moving := boxAt(mgl32.Vec3{-3, 0, 0}, 0.5)
    static := boxAt(mgl32.Vec3{}, 0.5)
    obb := OBB{AABB: static, Transform: mgl32.Ident4()}
    velocity := mgl32.Vec3{6, 0, 0}

    swept := SweepAABB(moving, static, velocity)
    oriented := CheckAABBVsOBB(moving, obb, velocity)

    // Centers are 3 apart, they touch after 1 unit of gap has closed at 2 units.
    assertCollision(t, swept, 2.0/6.0, mgl32.Vec3{-1, 0, 0})
    assertCollision(t, oriented, 2.0/6.0, mgl32.Vec3{-1, 0, 0})
}

// A box that is wide in x and thin in z blocks along z once it is turned by 90
// degrees. Using its untransformed extents would report the hit far too early.
func TestCheckAABBVsOBBRotated(t *testing.T) {
    moving := boxAt(mgl32.Vec3{-3, 0, 0}, 0.05)
    wide := NewAABB(mgl32.Vec3{-1, -0.5, -0.25}, mgl32.Vec3{1, 0.5, 0.25})
    velocity := mgl32.Vec3{6, 0, 0}

    unrotated := CheckAABBVsOBB(moving, OBB{AABB: wide, Transform: mgl32.Ident4()}, velocity)
    assertCollision(t, unrotated, (3-1.05)/6, mgl32.Vec3{-1, 0, 0})

    turned := mgl32.HomogRotate3DY(mgl32.DegToRad(90))
    rotated := CheckAABBVsOBB(moving, OBB{AABB: wide, Transform: turned}, velocity)
    assertCollision(t, rotated, (3-0.3)/6, mgl32.Vec3{-1, 0, 0})
}

// The distance has to come back in world units, not in the box' local units.
func TestCheckAABBVsOBBScaled(t *testing.T) {
    moving := boxAt(mgl32.Vec3{-3, 0, 0}, 0.05)
    unit := boxAt(mgl32.Vec3{}, 0.5)
    velocity := mgl32.Vec3{6, 0, 0}

    scaled := CheckAABBVsOBB(moving, OBB{AABB: unit, Transform: mgl32.Scale3D(2, 2, 2)}, velocity)

    // The box now reaches to x = -1, so contact is at x = -1.05.
    assertCollision(t, scaled, (3-1.05)/6, mgl32.Vec3{-1, 0, 0})
}

// A moving box that is not a cube reaches further along some of the rotated
// box' axes than others, which is what the inflation has to account for.
func TestCheckAABBVsOBBElongatedMover(t *testing.T) {
    moving := NewAABB(mgl32.Vec3{-3.5, -0.5, -0.1}, mgl32.Vec3{-2.5, 0.5, 0.1})
    unit := boxAt(mgl32.Vec3{}, 0.5)
    angle := float64(mgl32.DegToRad(30))
    obb := OBB{AABB: unit, Transform: mgl32.HomogRotate3DY(float32(angle))}
    velocity := mgl32.Vec3{6, 0, 0}

    cos, sin := float32(math.Cos(angle)), float32(math.Sin(angle))
    // Reach of the moving box along the local x axis (cos, 0, -sin) of the OBB.
    reachAlongX := cos*0.5 + sin*0.1
    // The ray runs along the local x axis at cos units per world unit.
    distance := (3*cos - (0.5 + reachAlongX)) / cos

    info := CheckAABBVsOBB(moving, obb, velocity)

    assertCollision(t, info, distance/6, mgl32.Vec3{-cos, 0, sin})
}

func TestCheckAABBVsOBBMisses(t *testing.T) {
    moving := boxAt(mgl32.Vec3{-3, 0, 0}, 0.5)
    obb := OBB{AABB: boxAt(mgl32.Vec3{}, 0.5), Transform: mgl32.Ident4()}

    tests := []struct {
        name     string
        velocity mgl32.Vec3
    }{
        {"moving away", mgl32.Vec3{-6, 0, 0}},
        {"too slow to reach", mgl32.Vec3{1, 0, 0}},
        {"passing by", mgl32.Vec3{0, 6, 0}},
        {"standing still", mgl32.Vec3{}},
    }
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            info := CheckAABBVsOBB(moving, obb, test.velocity)
            if info.IsCollision || info.TimeOfImpact != 1 {
                t.Errorf("expected no collision, got %+v", info)
            }
        })
    }
}

// Boxes that already overlap collide immediately instead of reporting a
// negative time of impact, which would move the object backwards.
func TestCheckAABBVsOBBAlreadyOverlapping(t *testing.T) {
    moving := boxAt(mgl32.Vec3{0.1, 0, 0}, 0.5)
    obb := OBB{AABB: boxAt(mgl32.Vec3{}, 0.5), Transform: mgl32.Ident4()}

    info := CheckAABBVsOBB(moving, obb, mgl32.Vec3{6, 0, 0})

    if !info.IsCollision || info.TimeOfImpact != 0 {
        t.Errorf("expected an immediate collision, got %+v", info)
    }
}
