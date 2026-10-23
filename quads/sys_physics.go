package quads

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/events"
	"github.com/memmaker/voxelfps/quads/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
)

type PhysicsSystem struct {
	movingObjects           []*PhysicsComponent
	staticObjects           []*PhysicsComponent
	bodies                  *collide.Bodies
	accelerationFromGravity mgl32.Vec3
	isSolid                 func(x int32, y int32, z int32) bool
	collisionPairsChecked   int
	gravityEnabled          bool
	weightlessAt            func(mgl32.Vec3) bool
	stepAccumulator         float64
	stepHeight              float32
}

// SetGravityEnabled turns gravity on or off for every object that is affected
// by it. With gravity off, objects keep whatever velocity they are given.
func (c *PhysicsSystem) SetGravityEnabled(enabled bool) {
	c.gravityEnabled = enabled
}

// SetWeightlessAt says where in this world gravity does not reach - outside a
// station's pressure hull, which is the only place so far that has anywhere.
// Gravity is a fact about a place, not a switch on the world: a level with no
// such region (nil, the default) pulls on everything everywhere, the way every
// level that is not a station does.
func (c *PhysicsSystem) SetWeightlessAt(weightless func(mgl32.Vec3) bool) {
	c.weightlessAt = weightless
}

// SetStepHeight is how high a ledge a falling object may walk straight up, as
// Quake 3's STEPSIZE does for stairs. Zero, the default, turns stepping off and
// every ledge is a wall.
func (c *PhysicsSystem) SetStepHeight(height float32) {
	c.stepHeight = height
}

// SetGravity replaces the acceleration gravity applies. The default is roughly
// earth's; a game with its own scale (Quake's, say) wants its own value.
func (c *PhysicsSystem) SetGravity(acceleration mgl32.Vec3) {
	c.accelerationFromGravity = acceleration
}

func (c *PhysicsSystem) GravityEnabled() bool {
	return c.gravityEnabled
}

func (c *PhysicsSystem) weightless(position mgl32.Vec3) bool {
	return c.weightlessAt != nil && c.weightlessAt(position)
}

// NewCollisionSolver is a world whose only walls are the cells isSolid says
// are. Past the edge of a level there are none: it is air, and whatever goes
// out there falls or hovers like anywhere else.
func NewCollisionSolver(isSolid func(x int32, y int32, z int32) bool) *PhysicsSystem {
	return &PhysicsSystem{
		isSolid:                 isSolid,
		bodies:                  collide.NewBodies(),
		accelerationFromGravity: mgl32.Vec3{0, -9.8, 0},
	}
}

func (c *PhysicsSystem) moveObjectOnGrid(obj *PhysicsComponent, oldPos mgl32.Vec3, newPos mgl32.Vec3) {
	if obj.body < 0 {
		return
	}
	c.bodies.Set(obj.body, bodyOf(obj, newPos))
}

// bodyOf is obj standing at pos as a VoxelLib body: its collision box (turned
// ones as their oriented box, enclosed by the turned extents), listed under
// everything it can be picked by.
func bodyOf(obj *PhysicsComponent, pos mgl32.Vec3) collide.Body {
	b := collide.Body{Box: obj.CollisionAABBAt(pos).Collide(), Bounds: obj.gridAABBAt(pos).Collide(), Solid: true, Data: obj}
	if obj.HasOrientedCollider() {
		b.Box = obj.WorldAABB().Collide()
		o := obj.WorldOBB().Collide()
		b.OBB = &o
		b.Bounds = union(b.Bounds, b.Box)
	}
	return b
}

func union(a, b collide.AABB) collide.AABB {
	return collide.AABB{
		MinX: min(a.MinX, b.MinX), MinY: min(a.MinY, b.MinY), MinZ: min(a.MinZ, b.MinZ),
		MaxX: max(a.MaxX, b.MaxX), MaxY: max(a.MaxY, b.MaxY), MaxZ: max(a.MaxZ, b.MaxZ),
	}
}

const epsilon = 0.001

// maxStepDistance is how far an object may move in one collision test.
const maxStepDistance = 1.0

// terminalVelocity caps how fast gravity can pull something down, so a long
// fall cannot outrun the collision test.
const terminalVelocity = 50.0

// groundNormalThreshold is how upwards a surface normal has to point to count
// as standing on it, roughly 45 degrees.
const groundNormalThreshold = 0.7

type LinkedAABB struct {
	voxel.AABB
	GameObject *PhysicsComponent
}

type CollisionEvent struct {
	ObjectA       *PhysicsComponent
	ObjectB       *PhysicsComponent
	WorldPos      mgl32.Vec3
	SurfaceNormal mgl32.Vec3
}

var zeroVec3 = mgl32.Vec3{0, 0, 0}

// fixedTimeStep is the interval the simulation actually advances by. Raw frame
// time made jump height and fall speed depend on the framerate.
const fixedTimeStep = 1.0 / 60.0

// maxFrameTime caps how much simulation one frame may ask for. Without it a long
// stall (a breakpoint, a window drag) queues up steps that take longer to run than
// the time they simulate, and the accumulator never drains.
const maxFrameTime = 0.25

// Update advances the simulation in fixed steps, consuming deltaTime. Leftover time
// carries into the next frame.
func (c *PhysicsSystem) Update(deltaTime float64) {
	if deltaTime > maxFrameTime {
		deltaTime = maxFrameTime
	}
	c.stepAccumulator += deltaTime
	c.collisionPairsChecked = 0
	for c.stepAccumulator >= fixedTimeStep {
		c.step(fixedTimeStep)
		c.stepAccumulator -= fixedTimeStep
	}
}

// can lead to stuff leaving the map and goign to infinity
func (c *PhysicsSystem) step(deltaTime float64) {
	/*
	   The plan:
	   Collide all moving objects with the world & static objects and themselves
	   Moving objects are either:
	    - Projectiles (modelled as either a point with raytracing or a sphere)
	    - Environmental objects (modelled as AABBs)
	    - NPCs (modelled as AABBs against the world but colliding as spheres/cylinders against each other)
	   Static objects are either:
	    - AABBs or spheres

	    We'll thus have these collision pairs:
	    - Ray vs AABB/OBB/Sphere
	    - AABB vs AABB/Sphere
	    - Sphere vs Sphere
	*/

	// finalPos := (0.5 * accel * deltaTime^2) + (vel * deltaTime) + pos

	solids := collide.SolidsFunc(func(x, y, z int) bool { return c.isSolid(int32(x), int32(y), int32(z)) })

	for id := len(c.movingObjects) - 1; id >= 0; id-- {
		obj := c.movingObjects[id]

		if obj.IsDead(deltaTime) {
			c.remove(id, obj)
			continue
		}

		if !obj.IsMovable() {
			continue
		}
		obj.stepFrom = obj.Position()

		// Before anything is done with it: a collider that changed shape under
		// a thing standing still has left it inside the floor.
		c.keepStandingOnIt(obj)

		velocity := obj.Velocity()
		if c.gravityEnabled && obj.AffectedByGravity() && !c.weightless(obj.Position()) {
			velocity = velocity.Add(c.accelerationFromGravity.Mul(float32(deltaTime)))
			if velocity.Y() < -terminalVelocity {
				velocity = mgl32.Vec3{velocity.X(), -terminalVelocity, velocity.Z()}
			}
			obj.SetVelocity(velocity)
		}

		if velocity == zeroVec3 {
			obj.stepTo = obj.Position()
			continue
		}

		// The collision test walks every grid cell in the bounding box of the
		// move, so a long move is split up: one box per step instead of one
		// box spanning the whole path.
		movement := velocity.Mul(float32(deltaTime))
		steps := 1
		if length := movement.Len(); length > maxStepDistance {
			steps = int(math.Ceil(float64(length / maxStepDistance)))
		}
		stepMovement := movement.Mul(1 / float32(steps))

		mover := c.moverFor(obj, solids)
		grounded := false
		for step := 0; step < steps; step++ {
			oldPos := obj.Position()
			box := obj.CollisionAABBAt(oldPos).Collide()
			move := [3]float64{float64(stepMovement.X()), float64(stepMovement.Y()), float64(stepMovement.Z())}
			var offset [3]float64
			var contacts []collide.Contact
			// A move a wall stopped gets one more chance: over the top of it, if
			// the wall turns out to be low enough to be a stair.
			if c.stepHeight > 0 && obj.AffectedByGravity() {
				offset, contacts = mover.StepSlide(box, move, float64(c.stepHeight))
			} else {
				offset, contacts = mover.Slide(box, move)
			}
			c.collisionPairsChecked += len(contacts)
			correctedPos := oldPos.Add(mgl32.Vec3{float32(offset[0]), float32(offset[1]), float32(offset[2])})

			obj.SetPosition(correctedPos)
			c.moveObjectOnGrid(obj, oldPos, correctedPos)

			if obj.IsDead(0) {
				break
			}

			// Movement into a surface is spent, the rest of it keeps going.
			for _, contact := range contacts {
				normal := mgl32.Vec3{float32(contact.Normal[0]), float32(contact.Normal[1]), float32(contact.Normal[2])}
				if into := velocity.Dot(normal); into < 0 {
					velocity = velocity.Sub(normal.Mul(into))
				}
			}
			if collide.Grounded(contacts) {
				grounded = true
			}
			if len(contacts) > 0 {
				obj.SetVelocity(velocity)
				stepMovement = velocity.Mul(float32(deltaTime) / float32(steps))
			}
		}
		obj.stepTo = obj.Position()
		obj.SetGrounded(grounded)
	}

	//c.updateProjectiles(deltaTime)
}

// moverFor is the VoxelLib mover obj moves with: blocks, and every body but
// its own that it does not pass through. Each surface hit is published, and a
// thing that died of it stops there.
func (c *PhysicsSystem) moverFor(obj *PhysicsComponent, solids collide.Solids) *collide.Mover {
	return &collide.Mover{
		Shapes: c.bodies.Shapes(solids, obj.body, func(_ int32, b *collide.Body) bool { return passesThrough(obj, b.Data.(*PhysicsComponent)) }),
		OnContact: func(contact collide.Contact) bool {
			var other *PhysicsComponent
			if contact.Body >= 0 {
				other = c.bodies.Get(contact.Body).Data.(*PhysicsComponent)
			}
			at := obj.Position().Add(mgl32.Vec3{float32(contact.Point[0]), float32(contact.Point[1]), float32(contact.Point[2])})
			events.Publish(events.EventPhysicsCollision, CollisionEvent{
				ObjectA:       obj,
				ObjectB:       other,
				WorldPos:      at,
				SurfaceNormal: mgl32.Vec3{float32(contact.Normal[0]), float32(contact.Normal[1]), float32(contact.Normal[2])},
			})
			return obj.GameObject().IsAlive()
		},
	}
}

// passesThrough is whether mover goes through collider rather than stopping.
//
// A thing that is walked through is not a thing that gets shoved. PassThrough
// takes an item out of everybody else's way; the other half of the same
// contract is that the item does not resolve a collision against the player -
// out of a box it is already deep inside, so it goes the shortest way, which is
// downwards through the floor and out of the map. Only against things that
// move under their own steam: the level and its furniture are what an item
// rests on, and it has to keep colliding with those.
func passesThrough(mover, collider *PhysicsComponent) bool {
	return !collider.GameObject().IsAlive() || collider.PassThrough ||
		(mover.PassThrough && collider.AffectedByGravity())
}

func kinematic(acceleration mgl32.Vec3, velocity mgl32.Vec3, position mgl32.Vec3, deltaTime float64) mgl32.Vec3 {
	newPos := (acceleration.Mul(float32(0.5 * deltaTime * deltaTime))).Add(velocity.Mul(float32(deltaTime))).Add(position)
	return newPos
}

// stepFraction is how far the clock is into the next fixed step, 0 to 1.
func (c *PhysicsSystem) stepFraction() float32 {
	return float32(c.stepAccumulator / fixedTimeStep)
}

func (c *PhysicsSystem) PairsChecked() int {
	return c.collisionPairsChecked
}

func (c *PhysicsSystem) AddObject(obj *PhysicsComponent) {
	obj.system = c
	c.movingObjects = append(c.movingObjects, obj)
	obj.colliderBottom = obj.CollisionAABBAt(zeroVec3).Min().Y()
	c.addToGrid(obj.Position(), obj)
}

// keepStandingOnIt puts an object back on top of whatever it was resting on when
// its collider changes shape underneath it.
//
// A sprite's box is measured off the picture it is drawn as, and each animation
// is measured off its own - see PhysicsComponent.AABBAt. The pictures do not all
// leave the same gap under the art, so the instant a guard stops walking and
// starts firing, the box it stands in drops by however much that gap differs: a
// pixel's worth for most of them, most of a tile for a turret's corpse. It is
// inside the floor from that moment, and it used to fall through the world for
// it. The collide mover now pushes it back out instead, but over several frames
// and visibly; moving it up by exactly what the box dropped is neither.
//
// Only the shape of the box is watched, never where it is. A thing walking down
// a slope is getting lower too, and that is it doing its job rather than its
// collider moving out from under it.
//
// ponytail: it goes up whether or not there is room above, on the grounds that
// there was room a moment ago for a box that was no shorter. If a low ceiling
// ever makes that wrong, the sweep pushes back out of it the frame after.
func (c *PhysicsSystem) keepStandingOnIt(obj *PhysicsComponent) {
	bottom := obj.CollisionAABBAt(zeroVec3).Min().Y()
	if dropped := obj.colliderBottom - bottom; obj.IsGrounded() && dropped > 0 {
		oldPos := obj.Position()
		obj.SetPosition(oldPos.Add(mgl32.Vec3{0, dropped, 0}))
		c.moveObjectOnGrid(obj, oldPos, obj.Position())
	}
	obj.colliderBottom = bottom
}

func (c *PhysicsSystem) addToGrid(pos mgl32.Vec3, obj *PhysicsComponent) {
	obj.body = c.bodies.Add(bodyOf(obj, pos))
}

func (c *PhysicsSystem) onProjectileObjectCollision(projectile *PhysicsComponent, collidingObject *PhysicsComponent, worldPosOfCollision mgl32.Vec3) {
	//collidingObject.HitWithProjectile(projectile, worldPosOfCollision)
	projectile.SetVelocity(mgl32.Vec3{0, 0, 0})
	projectile.SetPosition(worldPosOfCollision)
}

func (c *PhysicsSystem) onProjectileMapCollision(projectile *PhysicsComponent, info voxel.HitInfo3D) {
	projectile.SetVelocity(mgl32.Vec3{0, 0, 0})
	projectile.SetPosition(info.CollisionWorldPosition)
}

// RemoveObject drops obj out of the simulation entirely: no longer moved, no
// longer solid, no longer found by a collision test or a picking ray - both
// of those read the same grid this takes it off of. Unlike letting an object
// die on its own (obj.IsDead, checked once a step by the loop in step()),
// this does not touch the GameObject or its sprite at all: a corpse that
// should stay visible on-screen calls this instead of GameObject.Kill(),
// which would also drop it out of SpriteDrawer's draw list and make it
// vanish outright.
func (c *PhysicsSystem) RemoveObject(obj *PhysicsComponent) {
	for i, o := range c.movingObjects {
		if o == obj {
			c.remove(i, obj)
			return
		}
	}
}

func (c *PhysicsSystem) remove(id int, obj *PhysicsComponent) {
	c.bodies.Remove(obj.body)
	obj.body = -1
	c.movingObjects = append(c.movingObjects[:id], c.movingObjects[id+1:]...)
}

// Teleport puts an object somewhere without the simulation having moved it
// there, and - the point of it - re-lists it on the collision grid where it
// now stands. Anything that writes a position itself has to come through
// here rather than through the transform: the grid is what a picking ray
// reads (see Game.rayCast, which only ever looks at GetCollidersAt), so an
// object moved behind the system's back stays pickable at the place it
// started and nowhere else. That is what made a guide unselectable - and so
// unable to be talked to again - the moment it walked its first leg, since
// updateTour drives it by hand rather than through the pathfinder.
func (c *PhysicsSystem) Teleport(obj *PhysicsComponent, pos mgl32.Vec3) {
	oldPos := obj.Position()
	obj.SetPosition(pos)
	c.moveObjectOnGrid(obj, oldPos, pos)
}

func (c *PhysicsSystem) GetCollidersAt(x int32, y int32, z int32) map[*PhysicsComponent]bool {
	var found map[*PhysicsComponent]bool
	c.bodies.Query(collide.Block(int(x), int(y), int(z)), func(_ int32, b *collide.Body) {
		if found == nil {
			found = map[*PhysicsComponent]bool{}
		}
		found[b.Data.(*PhysicsComponent)] = true
	})
	return found
}

/*
Polygon vs Polygon using the separating axis theorem

var overlapping = true;

foreach (Plane plane in planesOfA)

	{
	    var allOut = true;
	    foreach (Vector3 point in pointsOfB)
	    {
	        if (plane.DistanceTo(point) < 0)
	        {
	            allOut = false;
	            break;
	        }
	    }

	    if (allOut)
	    {
	        // a separating plane was found
	        // do not continue testing
	        overlapping = false;
	        break;
	    }
	}

if (overlapping)

	{
	    // only do this check if no separating plane
	    // was found in planes of A
	    foreach (Plane plane in planesOfB)
	    {
	        var allOut = true;
	        foreach (Vector3 point in pointsOfA)
	        {
	            if (plane.DistanceTo(point) < 0)
	            {
	                allOut = false;
	                break;
	            }
	        }

	        if (allOut)
	        {
	            overlapping = false;
	            break;
	        }
	    }
	}

if (overlapping)

	{
	    foreach (Vector3 edgeA in edgesOfA)
	    {
	        foreach (Vector3 edgeB in edgesOfB)
	        {
	            var normal = edgeA.Cross(edgeB);
	            if (normal.Length() == 0)
	            {
	                continue;
	            }

	            var maxA = float.MinValue; // tiny number
	            var minA = float.MaxValue; // huge number

	            // we are using the dot product directly
	            // so we can map a maximum and minimum range
	            // for each polygon, then check if they
	            // overlap.

	            foreach (Vector3 point in pointsOfA)
	            {
	                var distance = normal.Dot(point);
	                maxA = Mathf.Max(maxA, distance);
	                minA = Mathf.Min(minA, distance);
	            }

	            var maxB = float.MinValue; // tiny number
	            var minB = float.MaxValue; // huge number

	            foreach (Vector3 point in pointsOfB)
	            {
	                var distance = normal.Dot(point);
	                maxB = Mathf.Max(maxB, distance);
	                minB = Mathf.Min(minB, distance);
	            }

	            if (minA > maxB || minB > maxA)
	            {
	                // not overlapping!
	                overlapping = false;
	                break;
	            }
	        }

	        if (!overlapping)
	        {
	            break;
	        }

	    }
	}

if (overlapping)

	{
	    GD.Print("Polygons Collided!");
	}
*/
type NormalPlane struct {
	normal mgl32.Vec3
	point  mgl32.Vec3
}

func (n NormalPlane) Normal() mgl32.Vec3 {
	return n.normal
}

func (n NormalPlane) DistanceTo(point mgl32.Vec3) float32 {
	return n.normal.Dot(point.Sub(n.point))
}

type Plane interface {
	DistanceTo(point mgl32.Vec3) float32
	Normal() mgl32.Vec3
}
type Polygon interface {
	Planes() []Plane
	Edges() []mgl32.Vec3
	Vertices() []mgl32.Vec3
}

type obbPolygon struct {
	aabb      voxel.AABB
	transform mgl32.Mat4
}

func (o obbPolygon) Vertices() []mgl32.Vec3 {
	corners := o.aabb.Corners()
	var result []mgl32.Vec3
	for i := 0; i < 8; i++ {
		result = append(result, o.transform.Mul4x1(corners[i].Vec4(1)).Vec3())
	}
	return result
}

func (o obbPolygon) Planes() []Plane {
	planes := make([]Plane, 6)
	corners := o.aabb.Corners()
	for i := 0; i < 6; i++ {
		normal := mgl32.Vec3{}
		point := mgl32.Vec3{}
		switch i {
		case 0:
			normal = mgl32.Vec3{1, 0, 0}
			point = corners[0]
		case 1:
			normal = mgl32.Vec3{-1, 0, 0}
			point = corners[1]
		case 2:
			normal = mgl32.Vec3{0, 1, 0}
			point = corners[0]
		case 3:
			normal = mgl32.Vec3{0, -1, 0}
			point = corners[2]
		case 4:
			normal = mgl32.Vec3{0, 0, 1}
			point = corners[0]
		case 5:
			normal = mgl32.Vec3{0, 0, -1}
			point = corners[4]
		}

		point = o.transform.Mul4x1(point.Vec4(1)).Vec3()
		normal = o.transform.Mul4x1(normal.Vec4(0)).Vec3()

		planes[i] = NormalPlane{
			normal: normal,
			point:  point,
		}
	}
	return planes
}

func (o obbPolygon) Edges() []mgl32.Vec3 {
	corners := o.Vertices()
	edges := make([]mgl32.Vec3, 12)
	for i := 0; i < 4; i++ {
		edges[i] = corners[i+1].Sub(corners[i])
		edges[i+4] = corners[i+5].Sub(corners[i+1])
		edges[i+8] = corners[i+4].Sub(corners[i])
	}
	return edges
}

func PolygonFromObb(aabb voxel.AABB, transform mgl32.Mat4) Polygon {
	return &obbPolygon{
		aabb:      aabb,
		transform: transform,
	}
}

func PolygonPolygonCollisionV1(polyA, polyB Polygon) bool {
	overlapping := true
	for _, plane := range polyA.Planes() {
		allOut := true
		for _, point := range polyB.Edges() {
			if plane.DistanceTo(point) < 0 {
				allOut = false
				break
			}
		}
		if allOut {
			overlapping = false
			break
		}
	}
	if overlapping {
		for _, plane := range polyB.Planes() {
			allOut := true
			for _, point := range polyA.Edges() {
				if plane.DistanceTo(point) < 0 {
					allOut = false
					break
				}
			}
			if allOut {
				overlapping = false
				break
			}
		}
	}
	if overlapping {
		for _, edgeA := range polyA.Edges() {
			for _, edgeB := range polyB.Edges() {
				normal := edgeA.Cross(edgeB)
				if normal.Len() == 0 {
					continue
				}
				maxA := float32(math.MinInt32)
				minA := float32(math.MaxInt32)
				for _, point := range polyA.Edges() {
					distance := normal.Dot(point)
					maxA = max(maxA, distance)
					minA = min(minA, distance)
				}
				maxB := float32(math.MinInt32)
				minB := float32(math.MaxInt32)
				for _, point := range polyB.Edges() {
					distance := normal.Dot(point)
					maxB = max(maxB, distance)
					minB = min(minB, distance)
				}
				if minA > maxB || minB > maxA {
					overlapping = false
					break
				}
			}
			if !overlapping {
				break
			}
		}
	}
	return overlapping

}

func PolygonPolygonCollisionGPT(prevPolyA, newPolyA, polyB Polygon, velocity mgl32.Vec3) (bool, float32, mgl32.Vec3) {
	var mtv mgl32.Vec3
	toi := float32(1) // Start with max value (meaning no collision)
	minPenetration := float32(math.MaxFloat32)

	// Function to check projections and update MTV and TOI
	checkSeparationAxis := func(normal mgl32.Vec3) bool {
		if normal.Len() == 0 {
			return false // Skip degenerate axes
		}
		normal = normal.Normalize()

		minA, maxA := projectPolygon(prevPolyA, normal)
		minB, maxB := projectPolygon(polyB, normal)

		// Check for separation in the initial position
		if minA > maxB || minB > maxA {
			return false // No collision
		}

		// Compute penetration depth
		penetration := min(maxA-minB, maxB-minA)
		if penetration < minPenetration {
			minPenetration = penetration
			mtv = normal.Mul(penetration)
		}

		// Check time of impact using velocity projection
		velocityAlongAxis := velocity.Dot(normal)
		if velocityAlongAxis < 0 { // Moving towards the separation axis
			toiAxis := (maxB - minA) / -velocityAlongAxis
			if toiAxis < toi {
				toi = toiAxis
			}
		}

		return true
	}

	// Check polygon A's planes
	for _, plane := range newPolyA.Planes() {
		if !checkSeparationAxis(plane.Normal()) {
			return false, 1, mgl32.Vec3{}
		}
	}

	// Check polygon B's planes
	for _, plane := range polyB.Planes() {
		if !checkSeparationAxis(plane.Normal()) {
			return false, 1, mgl32.Vec3{}
		}
	}

	// Check edge cross-products as potential separating axes
	for _, edgeA := range newPolyA.Edges() {
		for _, edgeB := range polyB.Edges() {
			normal := edgeA.Cross(edgeB)
			if !checkSeparationAxis(normal) {
				return false, 1, mgl32.Vec3{}
			}
		}
	}

	return true, toi, mtv
}

// Helper function to project a polygon onto an axis
func projectPolygon(poly Polygon, axis mgl32.Vec3) (float32, float32) {
	minProj := float32(math.MaxFloat32)
	maxProj := float32(math.SmallestNonzeroFloat32)

	for _, point := range poly.Vertices() {
		projection := axis.Dot(point)
		minProj = min(minProj, projection)
		maxProj = max(maxProj, projection)
	}

	return minProj, maxProj
}

type Ray struct {
	Origin, Direction mgl32.Vec3
}

type Sphere struct {
	Center mgl32.Vec3
	Radius float32
}

func IntersectRaySphere(ray Ray, sphere Sphere) (bool, float32, float32) {
	CO := ray.Origin.Sub(sphere.Center)
	a := ray.Direction.Dot(ray.Direction)
	b := 2.0 * CO.Dot(ray.Direction)
	c := CO.Dot(CO) - (sphere.Radius * sphere.Radius)
	discriminant := b*b - 4.0*a*c

	if discriminant < 0.0 {
		return false, 0, 0
	}

	tmin := (-b - float32(math.Sqrt(float64(discriminant)))) / (2.0 * a)
	tmax := (-b + float32(math.Sqrt(float64(discriminant)))) / (2.0 * a)
	if tmin > tmax {
		tmin, tmax = tmax, tmin
	}

	return true, tmin, tmax
}

type Capsule struct {
	SegmentStart, SegmentEnd mgl32.Vec3
	Radius                   float64
}

func IntersectRayCapsule(ray Ray, capsule Capsule) (bool, mgl32.Vec3, mgl32.Vec3) {
	AB := capsule.SegmentEnd.Sub(capsule.SegmentStart)
	AO := ray.Origin.Sub(capsule.SegmentStart)

	ABDotD := AB.Dot(ray.Direction)
	ABDotAO := AB.Dot(AO)
	ABDotAB := AB.Dot(AB)

	m := ABDotD / ABDotAB
	n := ABDotAO / ABDotAB

	Q := ray.Direction.Sub(AB.Mul(m))
	R := AO.Sub(AB.Mul(n))

	a := Q.Dot(Q)
	b := 2.0 * Q.Dot(R)
	c := R.Dot(R) - float32(capsule.Radius*capsule.Radius)

	if a == 0.0 {
		// Special case: parallel ray and capsule
		return false, mgl32.Vec3{}, mgl32.Vec3{}
	}

	discriminant := b*b - 4.0*a*c
	if discriminant < 0.0 {
		return false, mgl32.Vec3{}, mgl32.Vec3{}
	}

	tmin := (-b - float32(math.Sqrt(float64(discriminant)))) / (2.0 * a)
	tmax := (-b + float32(math.Sqrt(float64(discriminant)))) / (2.0 * a)
	if tmin > tmax {
		tmin, tmax = tmax, tmin
	}

	return true, ray.Origin.Add(ray.Direction.Mul(tmin)), ray.Origin.Add(ray.Direction.Mul(tmax))
}

func IntersectCapsuleCapsule(capsule1, capsule2 Capsule, velocity1, velocity2 mgl32.Vec3) (bool, float32, mgl32.Vec3) {
	deltaV := velocity1.Sub(velocity2)                         // Relative velocity
	deltaP := capsule1.SegmentStart.Sub(capsule2.SegmentStart) // Initial separation

	// Quadratic coefficients for closest approach
	a := deltaV.Dot(deltaV)
	b := 2 * deltaP.Dot(deltaV)
	c := deltaP.Dot(deltaP) - float32(math.Pow(capsule1.Radius+capsule2.Radius, 2))

	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return false, 0, mgl32.Vec3{}
	}

	timeOfImpact := (-b - float32(math.Sqrt(float64(discriminant)))) / (2 * a)
	if timeOfImpact < 0 || timeOfImpact > 1 {
		return false, 0, mgl32.Vec3{}
	}

	// Compute impact point normal
	impactPoint1 := capsule1.SegmentStart.Add(velocity1.Mul(timeOfImpact))
	impactPoint2 := capsule2.SegmentStart.Add(velocity2.Mul(timeOfImpact))
	normal := impactPoint1.Sub(impactPoint2)
	normal.Normalize()

	return true, timeOfImpact, normal
}

// CheckForCollisions moves the entity towards newPos with the same VoxelLib
// mover the system uses, against the colliders collidersAt lists per cell. It
// returns the corrected position, how many surfaces were touched, and their
// normals. The system itself reads its body index instead; this is for tests
// and callers with their own idea of what is in the way.
func CheckForCollisions(movingEntity *PhysicsComponent, newPos mgl32.Vec3, collidersAt func(x int32, y int32, z int32) []LinkedAABB) (mgl32.Vec3, int, []mgl32.Vec3) {
	old := movingEntity.Position()
	m := cellMover(movingEntity, collidersAt)
	d := newPos.Sub(old)
	offset, contacts := m.Slide(movingEntity.CollisionAABBAt(old).Collide(), [3]float64{float64(d.X()), float64(d.Y()), float64(d.Z())})
	normals := make([]mgl32.Vec3, len(contacts))
	for i, c := range contacts {
		normals[i] = mgl32.Vec3{float32(c.Normal[0]), float32(c.Normal[1]), float32(c.Normal[2])}
	}
	return old.Add(mgl32.Vec3{float32(offset[0]), float32(offset[1]), float32(offset[2])}), len(contacts), normals
}

// cellMover is a mover over collidersAt, publishing hits as the system does.
func cellMover(obj *PhysicsComponent, collidersAt func(x, y, z int32) []LinkedAABB) *collide.Mover {
	var others []*PhysicsComponent
	m := &collide.Mover{Shapes: func(area collide.AABB, out []collide.Shape) []collide.Shape {
		others = others[:0]
		seen := map[*PhysicsComponent]bool{}
		for x := int32(math.Floor(area.MinX)); x <= int32(math.Floor(area.MaxX)); x++ {
			for y := int32(math.Floor(area.MinY)); y <= int32(math.Floor(area.MaxY)); y++ {
				for z := int32(math.Floor(area.MinZ)); z <= int32(math.Floor(area.MaxZ)); z++ {
					for _, l := range collidersAt(x, y, z) {
						s := collide.Shape{Box: l.AABB.Collide(), Body: -1}
						if l.GameObject != nil {
							if l.GameObject == obj || seen[l.GameObject] || passesThrough(obj, l.GameObject) {
								continue
							}
							seen[l.GameObject] = true
							if l.GameObject.HasOrientedCollider() {
								o := l.GameObject.WorldOBB().Collide()
								s.OBB = &o
							}
							s.Body = int32(len(others))
							others = append(others, l.GameObject)
						}
						out = append(out, s)
					}
				}
			}
		}
		return out
	}}
	m.OnContact = func(c collide.Contact) bool {
		var other *PhysicsComponent
		if c.Body >= 0 {
			other = others[c.Body]
		}
		events.Publish(events.EventPhysicsCollision, CollisionEvent{
			ObjectA:       obj,
			ObjectB:       other,
			WorldPos:      obj.Position().Add(mgl32.Vec3{float32(c.Point[0]), float32(c.Point[1]), float32(c.Point[2])}),
			SurfaceNormal: mgl32.Vec3{float32(c.Normal[0]), float32(c.Normal[1]), float32(c.Normal[2])},
		})
		return obj.GameObject().IsAlive()
	}
	return m
}
