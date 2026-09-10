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

	// Update runs fixed steps, so the step after the one that carries the object
	// out of the map is the one that notices - within the same call.
	system.Update(1)
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

// The whole point of the fixed step: one second of falling is one second of
// falling, whether the frames were 60 or 120 per second.
func TestUpdateIsFramerateIndependent(t *testing.T) {
	fall := func(frames int, frameTime float64) mgl32.Vec3 {
		system := NewCollisionSolver(func(x, y, z int32) bool { return false }, alwaysInsideMap)
		system.SetGravityEnabled(true)
		object := mover(t, mgl32.Vec3{0.5, 100, 0.5})
		object.ApplyGravity = true
		system.AddObject(object)
		for i := 0; i < frames; i++ {
			system.Update(frameTime)
		}
		return object.Position()
	}

	at60 := fall(60, 1.0/60.0)
	at120 := fall(120, 1.0/120.0)
	if at60 != at120 {
		t.Errorf("one second of fall differs with the framerate: 60fps %v, 120fps %v", at60, at120)
	}
	if at60.Y() >= 100 {
		t.Errorf("object did not fall at all, y = %v", at60.Y())
	}
}

// Two objects on the collision grid: the mover has to stop at the blocker
// rather than pass through it. Also guards the reused collider scratch buffer
// in step(), which every grid cell now writes into.
func TestUpdateStopsAtObjectOnGrid(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return false }, alwaysInsideMap)

	blocker := mover(t, mgl32.Vec3{3, 0.5, 0.5}) // no velocity, so it never moves
	system.AddObject(blocker)

	moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
	moving.SetVelocity(mgl32.Vec3{6, 0, 0})
	system.AddObject(moving)

	system.Update(1)

	// Both boxes are 0.25 wide, so the centres cannot come closer than 0.5.
	if gap := blocker.Position().X() - moving.Position().X(); gap < 0.49 {
		t.Errorf("moved into the blocker: gap = %f, want >= 0.5", gap)
	}
	if moving.Position().X() < 1 {
		t.Errorf("stopped far too early: x = %f", moving.Position().X())
	}
}

// stairs makes a floor at y < 0 and one ledge of the given height filling
// everything at x >= 2, so a mover walking towards +x runs into it.
func ledge(height float32) func(x, y, z int32) []LinkedAABB {
	return func(x, y, z int32) []LinkedAABB {
		if y < 0 {
			return []LinkedAABB{{AABB: voxel.NewAABBFromMin(
				mgl32.Vec3{float32(x), float32(y), float32(z)}, mgl32.Vec3{1, 1, 1})}}
		}
		if x < 2 || float32(y) >= height {
			return nil
		}
		return []LinkedAABB{{AABB: voxel.NewAABBFromMin(
			mgl32.Vec3{float32(x), 0, float32(z)}, mgl32.Vec3{1, height, 1})}}
	}
}

// walkInto runs one blocked move plus the step retry and reports where the mover
// ended up.
func walkInto(t *testing.T, colliders func(x, y, z int32) []LinkedAABB, stepHeight float32) mgl32.Vec3 {
	t.Helper()
	moving := mover(t, mgl32.Vec3{1.5, 0.25, 0.5})
	movement := mgl32.Vec3{0.5, 0, 0}
	blocked, _, normals := CheckForCollisions(moving, moving.Position().Add(movement), colliders)
	if !blockedByWall(normals) {
		t.Fatalf("expected the ledge to block the move, ended at %v", blocked)
	}
	position, _, _, stepped := StepOver(moving, movement, blocked, stepHeight, colliders)
	if !stepped {
		return blocked
	}
	return position
}

// A ledge lower than the step height is walked up, and the mover ends up
// standing on top of it.
func TestStepOverClimbsALowLedge(t *testing.T) {
	position := walkInto(t, ledge(0.25), 0.3)
	if position.X() <= 1.75 {
		t.Fatalf("did not get past the ledge: x = %f", position.X())
	}
	// Box is half a unit tall, so its center sits at the ledge top plus 0.25.
	if math.Abs(float64(position.Y()-0.5)) > 0.01 {
		t.Errorf("ended at y = %f, want to be standing on the 0.25 ledge", position.Y())
	}
}

// A ledge taller than the step height stays a wall, and does not lift the mover
// even part of the way.
func TestStepOverLeavesATallLedgeAlone(t *testing.T) {
	position := walkInto(t, ledge(0.5), 0.3)
	if position.X() > 1.76 {
		t.Errorf("climbed a ledge taller than the step height: %v", position)
	}
	if position.Y() > 0.26 {
		t.Errorf("was lifted by a wall it cannot climb: y = %f", position.Y())
	}
}

// A pass-through collider is on the grid, so a ray finds it, but nothing walks
// into it. That is what an item lying on the floor is: in Doom you walk over a
// pickup, and before this the only way to be findable by a ray was to be a wall.
func TestPassThroughCollidersAreWalkedThrough(t *testing.T) {
	walkInto := func(passThrough bool) mgl32.Vec3 {
		system := NewCollisionSolver(func(x, y, z int32) bool { return false }, alwaysInsideMap)

		item := mover(t, mgl32.Vec3{2, 0.5, 0.5})
		item.PassThrough = passThrough
		system.AddObject(item)

		walker := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
		walker.SetVelocity(mgl32.Vec3{1, 0, 0})
		system.AddObject(walker)

		// Update clamps a long frame, so this is two seconds of them.
		for frame := 0; frame < 120; frame++ {
			walker.SetVelocity(mgl32.Vec3{1, 0, 0})
			system.Update(1.0 / 60.0)
		}
		return walker.Position()
	}

	if stopped := walkInto(false); stopped.X() > 1.6 {
		t.Errorf("walked to %v through a solid collider at x = 2", stopped)
	}
	if through := walkInto(true); through.X() < 2.4 {
		t.Errorf("stopped at %v, and a pass-through collider is not something to bump into", through)
	}
}

// An item lying on the floor is walked through, not shoved. PassThrough keeps
// it out of the player's way; this is the other half of it, and without it the
// item is the one resolving the overlap - out of a box it is already deep
// inside, which sends it down through the floor and out of the map.
func TestAThingWalkedThroughIsNotShovedByTheWalker(t *testing.T) {
	item := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
	item.PassThrough = true

	walker := mover(t, mgl32.Vec3{0.5, 0.9, 0.5})
	walker.ApplyGravity = true
	walkerInEveryCell := func(x, y, z int32) []LinkedAABB {
		return []LinkedAABB{{AABB: walker.AABB(), GameObject: walker}}
	}

	falling := mgl32.Vec3{0.5, 0.4, 0.5}
	corrected, _, hitNormals := CheckForCollisions(item, falling, walkerInEveryCell)

	if len(hitNormals) > 0 {
		t.Errorf("the item was pushed by the player walking into it: %v", hitNormals)
	}
	if corrected != falling {
		t.Errorf("the item ended at %v, and nothing should have moved it off %v", corrected, falling)
	}
}
