package voxel

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/stretchr/testify/assert"
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

func TestMPR(t *testing.T) {

    //size_t i;
    //ccd_t ccd;
    /*
       CCD_BOX(box1);
       CCD_BOX(box2);
    */
    //int res;
    /*
       CCD_INIT(&ccd);
       ccd.support1 = ccdSupport;
       ccd.support2 = ccdSupport;
       ccd.center1  = ccdObjCenter;
       ccd.center2  = ccdObjCenter;
    */

    /*
       box1.x = 1;
       box1.y = 2;
       box1.z = 1;
       box2.x = 2;
       box2.y = 1;
       box2.z = 2;
    */
    box1 := NewBox(1, 2, 1)
    box2 := NewBox(2, 1, 2)
    /*
       ccdVec3Set(&box1.pos, -5., 0., 0.);
       ccdVec3Set(&box2.pos, 0., 0., 0.);
       ccdQuatSet(&box1.quat, 0., 0., 0., 1.);
       ccdQuatSet(&box2.quat, 0., 0., 0., 1.);
    */

    box1.Transform.SetPosition(mgl32.Vec3{-5, 0, 0})
    box2.Transform.SetPosition(mgl32.Vec3{0, 0, 0})
    box1.Transform.SetRotation(mgl32.QuatIdent())
    box2.Transform.SetRotation(mgl32.QuatIdent())
    for i := 0; i < 100; i++ {
        res := ccdMPRIntersect(box1, box2)
        if i < 35 || i > 65 {
            //assertFalse(res);
            assert.False(t, res, "Expected false, got true")
        } else if i != 35 && i != 65 {
            //assertTrue(res);
            assert.True(t, res, "Expected true, got false")
        }

        //box1.pos.v[0] += 0.1;
        box1.Transform.SetPosition(box1.Transform.GetPosition().Add(mgl32.Vec3{0.1, 0, 0}))
    }

}
