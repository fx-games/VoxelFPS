package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/events"
	"github.com/memmaker/voxelfps/quads/voxel"
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

// Past the edge of the level is air, not the end of the world: something that
// goes out there keeps moving, falls under gravity, and stays in the simulation.
// It used to be killed, and the player with it - left standing, unable to move.
func TestObjectsThatLeaveTheMapKeepFalling(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return x >= 0 && x < 10 && y < 1 })
	system.SetGravityEnabled(true)

	escaping := mover(t, mgl32.Vec3{9.5, 3, 0.5})
	escaping.ApplyGravity = true
	escaping.SetVelocity(mgl32.Vec3{10, 0, 0})
	system.AddObject(escaping)

	for range 8 { // two seconds; Update takes a quarter of one at most
		system.Update(maxFrameTime)
	}
	if len(system.movingObjects) != 1 || !escaping.GameObject().IsAlive() {
		t.Fatal("the object that left the map was dropped")
	}
	if pos := escaping.Position(); pos.X() < 15 || pos.Y() > 0 {
		t.Errorf("at %v, want well past the edge and fallen below the floor", pos)
	}
	if cells := len(system.objectsOnGrid); cells > 8 {
		t.Errorf("%d grid cells left behind by one falling box", cells)
	}
}

// floorAt makes every voxel below the given height solid.
func floorAt(height int32) func(x, y, z int32) bool {
	return func(x, y, z int32) bool { return y < height }
}

// A falling object accelerates, lands on the floor and stays there.
func TestUpdateGravityFallsAndLands(t *testing.T) {
	system := NewCollisionSolver(floorAt(1))
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
	system := NewCollisionSolver(floorAt(1))

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
	system := NewCollisionSolver(func(x, y, z int32) bool { return x == 10 })

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
		system := NewCollisionSolver(func(x, y, z int32) bool { return false })
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
	system := NewCollisionSolver(func(x, y, z int32) bool { return false })

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
		system := NewCollisionSolver(func(x, y, z int32) bool { return false })

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

// A thing resting on the floor whose collider grows downwards - which is what a
// sprite changing animation does, since each animation's box is measured off its
// own pictures - is pushed back out of the floor instead of falling through it
// and out of the map.
func TestGrowingColliderDoesNotFallThroughTheFloor(t *testing.T) {
	system := NewCollisionSolver(floorAt(1))
	system.SetGravityEnabled(true)

	standing := mover(t, mgl32.Vec3{0.5, 4, 0.5})
	standing.ApplyGravity = true
	system.AddObject(standing)

	const frame = 1.0 / 60.0
	for i := 0; i < 120; i++ {
		system.Update(frame)
	}
	if !standing.IsGrounded() {
		t.Fatal("expected the object to land first")
	}

	// The next animation is drawn a tenth of a unit lower in its own tile, so
	// the box it stands in reaches that far into the floor.
	standing.SetAABB(voxel.NewAABB(
		mgl32.Vec3{-0.25, -0.35, -0.25},
		mgl32.Vec3{0.25, 0.25, 0.25},
	))

	for i := 0; i < 120; i++ {
		system.Update(frame)
	}

	if len(system.movingObjects) == 0 {
		t.Fatal("the object fell out of the map")
	}
	if bottom := standing.AABB().Min().Y(); bottom < 1-epsilon {
		t.Errorf("sank into the floor: box bottom at y = %f, want >= 1", bottom)
	}
	if !standing.IsGrounded() {
		t.Error("expected the object to still be standing on the floor")
	}
}

// A collider wider than a grid cell is listed in the cells in the middle of it
// too, not only in the ones its corners land in. The picking ray tests what the
// cell it is walking through lists and nothing else, so a gap in the middle is a
// thing the crosshair sits on and does not select.
func TestWideColliderIsOnTheGridCellsItCovers(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return false })

	object := NewGameObject("Cortex")
	object.Transform().SetPosition(mgl32.Vec3{0.5, 0.5, 0.5})
	// Two and a half cells across, so cells 1 and 2 are wholly inside it and
	// neither holds a corner.
	physics := NewPhysicsComponent(false, voxel.NewAABB(
		mgl32.Vec3{-1.25, -0.25, -0.25},
		mgl32.Vec3{1.25, 0.25, 0.25},
	))
	object.SetComponent(physics)
	system.AddObject(physics)

	for x := int32(-1); x <= 1; x++ {
		if system.GetCollidersAt(x, 0, 0)[physics] != true {
			t.Errorf("not listed in cell x = %d", x)
		}
	}
}

// The grid listing covers the picking boxes as well as the collision box. They
// are different shapes - the collision box is one median over every facing, the
// picking box is the picture actually on the screen - and a ray only ever tests
// what the cell it is in lists.
func TestGridListingCoversThePickingBoxes(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return false })

	object := NewGameObject("Cortex")
	object.Transform().SetPosition(mgl32.Vec3{0.5, 0.5, 0.5})
	physics := NewPhysicsComponent(false, voxel.NewAABB(
		mgl32.Vec3{-0.25, -0.25, -0.25},
		mgl32.Vec3{0.25, 0.25, 0.25},
	))
	object.SetComponent(physics)
	// Seen from one facing it is drawn twice as wide as the box it collides by.
	physics.SetPickBoxes(PickBoxes{AnimIdle: {
		{voxel.NewAABB(mgl32.Vec3{-0.25, -0.25, -0.25}, mgl32.Vec3{0.25, 0.25, 0.25})},
		{voxel.NewAABB(mgl32.Vec3{-1.25, -0.25, -0.25}, mgl32.Vec3{1.25, 0.25, 0.25})},
	}})
	system.AddObject(physics)

	for x := int32(-1); x <= 1; x++ {
		if system.GetCollidersAt(x, 0, 0)[physics] != true {
			t.Errorf("not listed in cell x = %d", x)
		}
	}
}

// Moving takes the listing with it: the cells it was in are the cells it is
// taken out of, whatever animation happens to be playing by then.
func TestMovingLeavesNoStaleGridCells(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return false })

	moving := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
	moving.SetVelocity(mgl32.Vec3{3, 0, 0})
	system.AddObject(moving)
	moving.SetAABBForAnimation(AnimIdle, voxel.NewAABB(
		mgl32.Vec3{-0.25, -0.25, -0.25},
		mgl32.Vec3{0.25, 0.25, 0.25},
	))

	// Update caps how much it simulates in one call, so this is several
	// seconds' worth: well clear of the cells it started in.
	for i := 0; i < 10; i++ {
		system.Update(1)
	}

	if moving.Position().X() < 3 {
		t.Fatalf("expected it to have moved away, x = %f", moving.Position().X())
	}
	if system.GetCollidersAt(0, 0, 0)[moving] {
		t.Error("still listed in the cell it started in")
	}
}

// A billboard's box is swung round its own position to face the camera, so the
// cells it is listed in have to be the ones it can reach into from any angle.
// Face on it is wide and a sliver deep; from the side those swap over.
func TestSwungRoundReachesBothWays(t *testing.T) {
	facingBox := voxel.NewAABB(mgl32.Vec3{-0.7, 0, -0.07}, mgl32.Vec3{0.7, 1.4, 0.07})

	swung := swungRound(facingBox)

	if reach := swung.Max().Z(); reach < 0.7 {
		t.Errorf("depth reaches %f, want the width it has when turned side on", reach)
	}
	if swung.Max().X() != swung.Max().Z() || swung.Min().X() != swung.Min().Z() {
		t.Errorf("expected the same reach either way, got %v", swung)
	}
	if swung.Min().Y() != 0 || swung.Max().Y() != 1.4 {
		t.Errorf("turning about the vertical axis should leave the height alone, got %v", swung)
	}
}

// A collider that drops under a thing standing still takes the thing with it,
// in the same step, rather than leaving it to be pushed back out of the floor
// over the next few. This is a sprite changing animation: each one's box is
// measured off its own pictures and they do not all leave the same gap under
// the art.
func TestColliderDroppingUnderAGroundedThingTakesItAlong(t *testing.T) {
	system := NewCollisionSolver(floorAt(1))
	system.SetGravityEnabled(true)

	standing := mover(t, mgl32.Vec3{0.5, 4, 0.5})
	standing.ApplyGravity = true
	system.AddObject(standing)

	const frame = 1.0 / 60.0
	for i := 0; i < 120; i++ {
		system.Update(frame)
	}
	landedAt := standing.Position().Y()

	// The next animation's box reaches a tenth of a unit further below the
	// thing's own position than this one's did.
	standing.SetAABB(voxel.NewAABB(
		mgl32.Vec3{-0.25, -0.35, -0.25},
		mgl32.Vec3{0.25, 0.25, 0.25},
	))
	system.Update(frame)

	if risen := standing.Position().Y() - landedAt; math.Abs(float64(risen-0.1)) > 0.01 {
		t.Errorf("moved up by %f, want the 0.1 the box dropped by", risen)
	}
	if bottom := standing.AABB().Min().Y(); bottom < 1-epsilon {
		t.Errorf("box bottom at y = %f, want it still on the floor at 1", bottom)
	}
}

// Getting lower is not the same as the collider getting lower: a thing walking
// down off a ledge is not held up by this.
func TestWalkingDownIsNotMistakenForADroppingCollider(t *testing.T) {
	system := NewCollisionSolver(floorAt(1))
	system.SetGravityEnabled(true)

	falling := mover(t, mgl32.Vec3{0.5, 4, 0.5})
	falling.ApplyGravity = true
	system.AddObject(falling)
	falling.SetGrounded(true) // as if it had just stepped off something

	const frame = 1.0 / 60.0
	for i := 0; i < 120; i++ {
		system.Update(frame)
	}

	if math.Abs(float64(falling.Position().Y()-1.25)) > 0.05 {
		t.Errorf("came to rest at y = %f, want 1.25", falling.Position().Y())
	}
}

// Gravity is a fact about a place. A station's hull has air and a deck inside
// it and vacuum outside, so the same casing falls in one and hangs in the
// other - and nothing had to be told which it was when it was thrown.
func TestNothingFallsWhereThereIsNoGravity(t *testing.T) {
	system := NewCollisionSolver(floorAt(0))
	system.SetGravityEnabled(true)
	// Everything above y=10 is outside the hull.
	system.SetWeightlessAt(func(at mgl32.Vec3) bool { return at.Y() > 10 })

	outside := mover(t, mgl32.Vec3{0.5, 20, 0.5})
	outside.ApplyGravity = true
	system.AddObject(outside)

	inside := mover(t, mgl32.Vec3{4.5, 5, 0.5})
	inside.ApplyGravity = true
	system.AddObject(inside)

	for range 8 {
		system.Update(maxFrameTime)
	}
	if pos := outside.Position(); pos != (mgl32.Vec3{0.5, 20, 0.5}) {
		t.Errorf("weightless object moved to %v, want it hanging where it was", pos)
	}
	if pos := inside.Position(); pos.Y() >= 5 {
		t.Errorf("object inside the hull is at %v, want it fallen", pos)
	}
}

// Drawn between steps, a thing walking at a steady speed covers the same ground
// every frame. Drawn where the last step left it, a 120 Hz frame covers none and
// then twice as much, which is what walking felt like on a ProMotion panel.
func TestRenderPositionMovesEveryFrame(t *testing.T) {
	system := NewCollisionSolver(func(x, y, z int32) bool { return false })
	object := mover(t, mgl32.Vec3{0.5, 0.5, 0.5})
	system.AddObject(object)

	const frame, speed = 1.0 / 120.0, 6.0
	last := object.RenderPosition().X()
	for tick := 0; tick < 24; tick++ {
		object.SetVelocity(mgl32.Vec3{speed, 0, 0})
		system.Update(frame)
		x := object.RenderPosition().X()
		if moved := float64(x - last); tick > 2 && math.Abs(moved-speed*frame) > 0.002 {
			t.Errorf("frame %d moved %.4f, want %.4f", tick, moved, speed*frame)
		}
		last = x
	}

	// Put somewhere by something other than the solver: drawn there, not
	// slid across the level to it.
	object.SetPosition(mgl32.Vec3{40, 0.5, 0.5})
	if drawn := object.RenderPosition(); drawn != object.Position() {
		t.Errorf("teleported to %v, drawn at %v", object.Position(), drawn)
	}
}
