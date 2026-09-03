package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads/voxel"
    "math"
    "testing"
)

// mover is a half unit cube that starts at the given position.
func mover(t *testing.T, position mgl32.Vec3) *PhysicsComponent {
    t.Helper()
    object := NewGameObject("Mover")
    object.Transform().SetPosition(position)
    physics := NewPhysicsComponent(false, voxel.NewAABB(
        mgl32.Vec3{-0.25, -0.25, -0.25},
        mgl32.Vec3{0.25, 0.25, 0.25},
    ))
    object.SetComponent(physics)
    return physics
}

// wall makes every voxel with x >= wallX solid.
func wall(wallX int32) func(x, y, z int32) []LinkedAABB {
    return func(x, y, z int32) []LinkedAABB {
        if x < wallX {
            return nil
        }
        return []LinkedAABB{{
            AABB: voxel.NewAABBFromMin(mgl32.Vec3{float32(x), float32(y), float32(z)}, mgl32.Vec3{1, 1, 1}),
        }}
    }
}

func TestCheckForCollisionsStopsAtWall(t *testing.T) {
    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})

    corrected, _, hitNormals := CheckForCollisions(moving, mgl32.Vec3{5, 0.5, 0.5}, wall(2))

    if len(hitNormals) == 0 {
        t.Fatal("expected a collision with the wall")
    }
    // The box is 0.25 wide, so its center stops just short of x = 2.
    if corrected.X() > 1.75 {
        t.Errorf("moved into the wall: x = %f, want <= 1.75", corrected.X())
    }
    if corrected.X() < 1.7 {
        t.Errorf("stopped too early: x = %f, want close to 1.75", corrected.X())
    }
}

// Movement into a wall at an angle keeps the part that runs along it.
func TestCheckForCollisionsSlidesAlongWall(t *testing.T) {
    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})

    corrected, _, hitNormals := CheckForCollisions(moving, mgl32.Vec3{5, 0.5, 3.5}, wall(2))

    if len(hitNormals) == 0 {
        t.Fatal("expected a collision with the wall")
    }
    if corrected.X() > 1.75 {
        t.Errorf("moved into the wall: x = %f", corrected.X())
    }
    if corrected.Z() < 1 {
        t.Errorf("did not slide along the wall: z = %f, want most of the 3 units of z movement", corrected.Z())
    }
}

// A move into a corner is redirected by both surfaces and still terminates.
func TestCheckForCollisionsInCornerTerminates(t *testing.T) {
    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
    corner := func(x, y, z int32) []LinkedAABB {
        if x < 2 && z < 2 {
            return nil
        }
        return []LinkedAABB{{
            AABB: voxel.NewAABBFromMin(mgl32.Vec3{float32(x), float32(y), float32(z)}, mgl32.Vec3{1, 1, 1}),
        }}
    }

    corrected, _, _ := CheckForCollisions(moving, mgl32.Vec3{5, 0.5, 5}, corner)

    if corrected.X() > 1.75 || corrected.Z() > 1.75 {
        t.Errorf("moved into the corner: %v", corrected)
    }
    if math.IsNaN(float64(corrected.X())) || math.IsNaN(float64(corrected.Z())) {
        t.Errorf("corrected position is not a number: %v", corrected)
    }
}

// A collider that carries its own rotation is swept as an oriented box, so a
// plank turned sideways only blocks the space it actually occupies.
func TestCheckForCollisionsAgainstRotatedObject(t *testing.T) {
    plank := func(rotation mgl32.Quat) func(x, y, z int32) []LinkedAABB {
        object := NewGameObject("Plank")
        object.Transform().SetPosition(mgl32.Vec3{3, 0.5, 0.5})
        object.Transform().SetRotation(rotation)
        physics := NewPhysicsComponent(false, voxel.NewAABB(
            mgl32.Vec3{-1, -0.5, -0.1},
            mgl32.Vec3{1, 0.5, 0.1},
        ))
        object.SetComponent(physics)
        return func(x, y, z int32) []LinkedAABB {
            return []LinkedAABB{{AABB: physics.AABB(), GameObject: physics}}
        }
    }

    across := stoppedAtX(t, plank(mgl32.QuatIdent()))
    if math.Abs(float64(across-1.75)) > 0.05 {
        t.Errorf("stopped at x = %f, want the plank to block at 1.75", across)
    }

    edgeOn := stoppedAtX(t, plank(mgl32.QuatRotate(mgl32.DegToRad(90), mgl32.Vec3{0, 1, 0})))
    if math.Abs(float64(edgeOn-2.65)) > 0.05 {
        t.Errorf("stopped at x = %f, want the turned plank to block at 2.65", edgeOn)
    }
}

// stoppedAtX moves a box from x = 0.5 towards x = 5 and reports where it ended up.
func stoppedAtX(t *testing.T, collidersAt func(x, y, z int32) []LinkedAABB) float32 {
    t.Helper()
    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
    corrected, _, _ := CheckForCollisions(moving, mgl32.Vec3{5, 0.5, 0.5}, collidersAt)
    return corrected.X()
}

// Nothing in the way means the move goes through unchanged.
func TestCheckForCollisionsWithoutColliders(t *testing.T) {
    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
    empty := func(x, y, z int32) []LinkedAABB { return nil }

    target := mgl32.Vec3{3, 0.5, 0.5}
    corrected, _, hitNormals := CheckForCollisions(moving, target, empty)

    if len(hitNormals) > 0 {
        t.Error("expected no collision")
    }
    if corrected != target {
        t.Errorf("position = %v, want %v", corrected, target)
    }
}

// Objects that leave the map are dropped instead of falling forever.
func TestUpdateRemovesObjectsThatLeaveTheMap(t *testing.T) {
    noSolids := func(x, y, z int32) bool { return false }
    insideMap := func(pos mgl32.Vec3) bool { return pos.X() >= 0 && pos.X() < 10 }
    system := NewCollisionSolver(noSolids, insideMap)

    escaping := mover(t, mgl32.Vec3{9, 0.5, 0.5})
    escaping.SetVelocity(mgl32.Vec3{10, 0, 0})
    system.AddObject(escaping)

    system.Update(1) // moves it to x = 19, outside the map
    if len(system.movingObjects) != 1 {
        t.Fatalf("object should still be tracked right after the move, have %d", len(system.movingObjects))
    }

    system.Update(1) // now it is noticed as gone
    if len(system.movingObjects) != 0 {
        t.Errorf("expected the escaped object to be removed, %d left", len(system.movingObjects))
    }
    if escaping.GameObject().IsAlive() {
        t.Error("expected the escaped object to be dead")
    }
}

// floorAt makes every voxel below the given height solid.
func floorAt(height int32) func(x, y, z int32) bool {
    return func(x, y, z int32) bool { return y < height }
}

func alwaysInsideMap(pos mgl32.Vec3) bool { return true }

// A falling object accelerates, lands on the floor and stays there.
func TestUpdateGravityFallsAndLands(t *testing.T) {
    system := NewCollisionSolver(floorAt(1), alwaysInsideMap)
    system.SetGravityEnabled(true)

    falling := mover(t, mgl32.Vec3{0.5, 4, 0.5})
    falling.ApplyGravity = true
    system.AddObject(falling)

    const frame = 1.0 / 60.0
    system.Update(frame)
    if falling.Velocity().Y() >= 0 {
        t.Fatalf("expected a downwards velocity after one frame, got %v", falling.Velocity())
    }
    if falling.IsGrounded() {
        t.Error("expected the object to still be in the air")
    }

    for i := 0; i < 120; i++ {
        system.Update(frame)
    }

    // The box is 0.25 high, so its center comes to rest just above y = 1.
    if math.Abs(float64(falling.Position().Y()-1.25)) > 0.05 {
        t.Errorf("came to rest at y = %f, want 1.25", falling.Position().Y())
    }
    if !falling.IsGrounded() {
        t.Error("expected the object to be grounded")
    }
    if math.Abs(float64(falling.Velocity().Y())) > 1 {
        t.Errorf("velocity into the floor should be spent, got %v", falling.Velocity())
    }
}

// Without gravity nothing falls.
func TestUpdateWithoutGravity(t *testing.T) {
    system := NewCollisionSolver(floorAt(1), alwaysInsideMap)

    floating := mover(t, mgl32.Vec3{0.5, 4, 0.5})
    floating.ApplyGravity = true
    system.AddObject(floating)

    system.Update(1)

    if floating.Position().Y() != 4 {
        t.Errorf("position = %v, want it to stay at y = 4", floating.Position())
    }
}

// A fast object is moved in steps, so it neither passes through a wall nor
// walks the grid cells of the whole path at once.
func TestUpdateFastObjectStopsAtWall(t *testing.T) {
    system := NewCollisionSolver(func(x, y, z int32) bool { return x == 10 }, alwaysInsideMap)

    fast := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
    fast.SetVelocity(mgl32.Vec3{600, 0, 0})
    system.AddObject(fast)

    system.Update(1) // 600 units in one frame

    if fast.Position().X() > 9.75 {
        t.Errorf("tunneled through the wall to x = %f", fast.Position().X())
    }
    if fast.Position().X() < 9.7 {
        t.Errorf("stopped short of the wall at x = %f", fast.Position().X())
    }
    if fast.Velocity().X() != 0 {
        t.Errorf("velocity into the wall should be spent, got %v", fast.Velocity())
    }
}

// blocker is a static collider of its own, sitting at the given position.
func blocker(name string, position mgl32.Vec3) *PhysicsComponent {
    object := NewGameObject(name)
    object.Transform().SetPosition(position)
    physics := NewPhysicsComponent(false, voxel.NewAABB(
        mgl32.Vec3{-0.5, -0.5, -0.5},
        mgl32.Vec3{0.5, 0.5, 0.5},
    ))
    object.SetComponent(physics)
    return physics
}

// Only the surface that was actually hit is reported, once, even though
// several colliders lie on the path.
func TestCheckForCollisionsPublishesOneEvent(t *testing.T) {
    var collisions []CollisionEvent
    events.Subscribe(events.EventPhysicsCollision, func(id events.EventID, event events.Event) {
        collisions = append(collisions, event.(CollisionEvent))
    })

    near := blocker("Near", mgl32.Vec3{3, 0.5, 0.5})
    far := blocker("Far", mgl32.Vec3{5, 0.5, 0.5})
    both := func(x, y, z int32) []LinkedAABB {
        return []LinkedAABB{
            {AABB: near.AABB(), GameObject: near},
            {AABB: far.AABB(), GameObject: far},
        }
    }

    moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
    collisions = nil
    CheckForCollisions(moving, mgl32.Vec3{8, 0.5, 0.5}, both)

    if len(collisions) != 1 {
        t.Fatalf("expected one collision event, got %d: %+v", len(collisions), collisions)
    }
    if collisions[0].ObjectB != near {
        t.Errorf("reported the wrong object: %s", collisions[0].ObjectB.GameObject().DisplayName)
    }
    // The mover is 0.25 wide and the near blocker starts at x = 2.5.
    if math.Abs(float64(collisions[0].WorldPos.X()-2.25)) > 0.01 {
        t.Errorf("collision position = %v, want the mover touching the first blocker", collisions[0].WorldPos)
    }
}
