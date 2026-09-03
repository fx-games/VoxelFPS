package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "testing"
)

//func CheckAABBVsOBB(aabb AABB, obb OBB, aabbVelocity mgl32.Vec3) CollisionInfo

func TestCheckOBBVsOBB(t *testing.T) {
    aabbUnitA := NewAABB(mgl32.Vec3{-0.5, -0.5, -0.5}, mgl32.Vec3{0.5, 0.5, 0.5})
    obbA := OBB{
        AABB:      aabbUnitA,
        Transform: mgl32.Translate3D(-3, 0, 0),
    }

    aabbUnitB := NewAABB(mgl32.Vec3{-0.5, -0.5, -0.5}, mgl32.Vec3{0.5, 0.5, 0.5})
    obbB := OBB{
        AABB:      aabbUnitB,
        Transform: mgl32.QuatRotate(mgl32.RadToDeg(30), mgl32.Vec3{0, 1, 0}).Mat4(),
    }

    velocity := mgl32.Vec3{6, 0, 0}

    collisionInfo := CheckOBBVsOBB(obbA, obbB, velocity)

    if collisionInfo.IsCollision == false {
        t.Errorf("Expected collision, got no collision")
    }

    if collisionInfo.TimeOfImpact != 0.5 {
        t.Errorf("Expected time of impact to be 0.5, got %f", collisionInfo.TimeOfImpact)
    }
    up := mgl32.Vec3{1, 0, 0}
    if collisionInfo.SurfaceNormal != up {
        t.Errorf("Expected surface normal to be {1, 0, 0}, got %v", collisionInfo.SurfaceNormal)
    }

}
