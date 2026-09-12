package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/events"
	"github.com/memmaker/goray/quads/voxel"
	"math"
)

type PhysicsSystem struct {
	movingObjects           []*PhysicsComponent
	staticObjects           []*PhysicsComponent
	objectsOnGrid           map[core.Int3]map[*PhysicsComponent]bool
	accelerationFromGravity mgl32.Vec3
	isSolid                 func(x int32, y int32, z int32) bool
	collisionPairsChecked   int
	gravityEnabled          bool
	weightlessAt            func(mgl32.Vec3) bool
	colliderScratch         []LinkedAABB
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
		objectsOnGrid:           make(map[core.Int3]map[*PhysicsComponent]bool),
		accelerationFromGravity: mgl32.Vec3{0, -9.8, 0},
	}
}

func (c *PhysicsSystem) moveObjectOnGrid(obj *PhysicsComponent, oldPos mgl32.Vec3, newPos mgl32.Vec3) {
	c.removeFromGrid(oldPos, obj)
	c.addToGrid(newPos, obj)
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

	// One closure and one scratch slice per step instead of per object: the
	// collision test consumes the returned colliders before asking for the next
	// cell, so a single reused buffer is enough.
	getCollidersAt := func(x, y, z int32) []LinkedAABB {
		c.colliderScratch = c.colliderScratch[:0]
		if c.isSolid(x, y, z) {
			c.colliderScratch = append(c.colliderScratch, LinkedAABB{
				AABB: voxel.NewAABBFromMin(mgl32.Vec3{float32(x), float32(y), float32(z)}, mgl32.Vec3{1, 1, 1}),
			})
			return c.colliderScratch
		}
		for collider := range c.objectsOnGrid[core.Int3{X: x, Y: y, Z: z}] {
			if !collider.GameObject().IsAlive() || collider.PassThrough {
				continue
			}
			c.colliderScratch = append(c.colliderScratch, LinkedAABB{
				AABB:       collider.AABB(),
				GameObject: collider,
			})
		}
		return c.colliderScratch
	}

	for id := len(c.movingObjects) - 1; id >= 0; id-- {
		obj := c.movingObjects[id]

		if obj.IsDead(deltaTime) {
			c.remove(id, obj)
			continue
		}

		if !obj.IsMovable() {
			continue
		}

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

		grounded := false
		for step := 0; step < steps; step++ {
			oldPos := obj.Position()
			correctedPos, pairsChecked, hitNormals := CheckForCollisions(obj, oldPos.Add(stepMovement), getCollidersAt)
			c.collisionPairsChecked += pairsChecked

			// A move a wall stopped gets one more chance: over the top of it, if
			// the wall turns out to be low enough to be a stair.
			if c.stepHeight > 0 && obj.AffectedByGravity() && blockedByWall(hitNormals) {
				steppedPos, steppedNormals, stepPairs, stepped := StepOver(obj, stepMovement, correctedPos, c.stepHeight, getCollidersAt)
				c.collisionPairsChecked += stepPairs
				if stepped {
					correctedPos, hitNormals = steppedPos, steppedNormals
				}
			}

			obj.SetPosition(correctedPos)
			c.moveObjectOnGrid(obj, oldPos, correctedPos)

			if obj.IsDead(0) {
				break
			}

			// Movement into a surface is spent, the rest of it keeps going.
			for _, normal := range hitNormals {
				if into := velocity.Dot(normal); into < 0 {
					velocity = velocity.Sub(normal.Mul(into))
				}
				if normal.Y() > groundNormalThreshold {
					grounded = true
				}
			}
			if len(hitNormals) > 0 {
				obj.SetVelocity(velocity)
				stepMovement = velocity.Mul(float32(deltaTime) / float32(steps))
			}
		}
		obj.SetGrounded(grounded)
	}

	//c.updateProjectiles(deltaTime)
}

// blockedByWall reports whether any of these surfaces is steep enough to be a
// wall. A floor underfoot or a ceiling overhead stops a move too, and neither is
// something to try and climb.
func blockedByWall(normals []mgl32.Vec3) bool {
	for _, normal := range normals {
		if math.Abs(float64(normal.Y())) < groundNormalThreshold {
			return true
		}
	}
	return false
}

// StepOver is Quake 3's PM_StepSlideMove: a move that ran into a wall is tried
// again from stepHeight higher up and then dropped back down again, which walks
// an object up a stair without needing a ramp to slide along. The entity is
// expected to still be standing where the blocked move started.
//
// It reports false, and changes nothing, unless the retry got further along than
// the blocked move did - otherwise a wall too tall to climb would still nudge
// the climber upwards every frame it is walked into.
func StepOver(movingEntity *PhysicsComponent, movement mgl32.Vec3, blockedPos mgl32.Vec3, stepHeight float32, collidersAt func(x int32, y int32, z int32) []LinkedAABB) (mgl32.Vec3, []mgl32.Vec3, int, bool) {
	startPos := movingEntity.Position()
	up := mgl32.Vec3{0, stepHeight, 0}
	pairs := 0

	// Up, along, and back down. Each leg starts from where the last one ended,
	// so a low ceiling shortens the first leg instead of pushing the entity
	// through it.
	risen, upPairs, _ := CheckForCollisions(movingEntity, startPos.Add(up), collidersAt)
	movingEntity.SetPosition(risen)
	moved, alongPairs, _ := CheckForCollisions(movingEntity, risen.Add(movement), collidersAt)
	movingEntity.SetPosition(moved)
	landed, downPairs, landingNormals := CheckForCollisions(movingEntity, moved.Sub(up), collidersAt)
	movingEntity.SetPosition(startPos)
	pairs = upPairs + alongPairs + downPairs

	if flatDistance(landed, startPos) <= flatDistance(blockedPos, startPos)+epsilon {
		return blockedPos, nil, pairs, false
	}
	return landed, landingNormals, pairs, true
}

// flatDistance is the distance between two points ignoring height.
func flatDistance(a, b mgl32.Vec3) float32 {
	return mgl32.Vec2{a.X() - b.X(), a.Z() - b.Z()}.Len()
}

func kinematic(acceleration mgl32.Vec3, velocity mgl32.Vec3, position mgl32.Vec3, deltaTime float64) mgl32.Vec3 {
	newPos := (acceleration.Mul(float32(0.5 * deltaTime * deltaTime))).Add(velocity.Mul(float32(deltaTime))).Add(position)
	return newPos
}

func (c *PhysicsSystem) PairsChecked() int {
	return c.collisionPairsChecked
}

func (c *PhysicsSystem) AddObject(obj *PhysicsComponent) {
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
// it. voxel.SweepAABB now pushes it back out instead, but over several frames
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
	forEachGridCell(obj.gridAABBAt(pos), func(gridPos core.Int3) {
		if c.objectsOnGrid[gridPos] == nil {
			c.objectsOnGrid[gridPos] = make(map[*PhysicsComponent]bool)
		}
		c.objectsOnGrid[gridPos][obj] = true
	})
}

func (c *PhysicsSystem) removeFromGrid(atPos mgl32.Vec3, obj *PhysicsComponent) {
	forEachGridCell(obj.gridAABBAt(atPos), func(gridPos core.Int3) {
		if c.objectsOnGrid[gridPos] != nil {
			delete(c.objectsOnGrid[gridPos], obj)
			// An empty cell goes too, or something falling off the level
			// leaves a trail of them all the way down.
			if len(c.objectsOnGrid[gridPos]) == 0 {
				delete(c.objectsOnGrid, gridPos)
			}
		}
	})
}

// forEachGridCell visits every cell the box reaches into.
//
// Its eight corners are not enough, which is what this used to walk. A box wider
// than a cell has whole cells in the middle of it that no corner of it lands in,
// and a thing left out of those is a thing the picking ray walks straight
// through: the ray tests what the cell it is in lists, and nothing else. Whether
// that happened at all depended on which cells the ray happened to cross, which
// is why it came and went with the angle you looked from.
//
// Floor rather than a cast, too: a cast truncates towards zero, so everything
// between -1 and 0 landed in cell 0 and the half of the map with negative
// coordinates was listed one cell over.
func forEachGridCell(aabb voxel.AABB, visit func(core.Int3)) {
	cell := func(v float32) int32 { return int32(math.Floor(float64(v))) }
	min, max := aabb.Min(), aabb.Max()
	for x := cell(min.X()); x <= cell(max.X()); x++ {
		for y := cell(min.Y()); y <= cell(max.Y()); y++ {
			for z := cell(min.Z()); z <= cell(max.Z()); z++ {
				visit(core.Int3{X: x, Y: y, Z: z})
			}
		}
	}
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
	c.removeFromGrid(obj.Position(), obj)
	c.movingObjects = append(c.movingObjects[:id], c.movingObjects[id+1:]...)
}

func (c *PhysicsSystem) GetCollidersAt(x int32, y int32, z int32) map[*PhysicsComponent]bool {
	mapPos := core.Int3{X: x, Y: y, Z: z}
	m, ok := c.objectsOnGrid[mapPos]
	if !ok {
		return nil
	}
	return m
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

// maxCollisionPasses is how often a single move may be redirected by a surface
// before the rest of it is dropped.
const maxCollisionPasses = 8

// CheckForCollisions moves the entity towards newPos, stopping at and sliding
// along whatever is in the way. It returns the corrected position, how many
// pairs were tested, and the surface normal of every distinct surface that was
// hit, so the caller can take the movement into them out of its velocity.
//
// The cells to test are taken from the bounding box of the whole move, so the
// work grows with the cube of the distance: keep single calls short, see
// PhysicsSystem.Update.
func CheckForCollisions(movingEntity *PhysicsComponent, newPos mgl32.Vec3, collidersAt func(x int32, y int32, z int32) []LinkedAABB) (mgl32.Vec3, int, []mgl32.Vec3) {
	pairsChecked := 0
	var hitNormals []mgl32.Vec3
	// adapted from: https://luisreis.net/blog/aabb_collision_handling/
	prevPos := movingEntity.Position()
	boxFromPos := movingEntity.CollisionAABBAt

	// Each pass slides the rest of the movement along the surface that was hit.
	// A corner can keep producing new surfaces, so the passes are capped
	// instead of trusting the movement to run out.
	for pass := 0; pass < maxCollisionPasses; pass++ {
		velocity := newPos.Sub(prevPos)

		if velocity == zeroVec3 {
			break
		}

		prevBox := boxFromPos(prevPos)
		newBox := boxFromPos(newPos)
		minX := int32(math.Floor(math.Min(float64(newBox.Min().X()), float64(prevBox.Min().X()))))
		maxX := int32(math.Floor(math.Max(float64(newBox.Max().X()), float64(prevBox.Max().X()))))

		minY := int32(math.Floor(math.Min(float64(newBox.Min().Y()), float64(prevBox.Min().Y()))))
		maxY := int32(math.Floor(math.Max(float64(newBox.Max().Y()), float64(prevBox.Max().Y()))))

		minZ := int32(math.Floor(math.Min(float64(newBox.Min().Z()), float64(prevBox.Min().Z()))))
		maxZ := int32(math.Floor(math.Max(float64(newBox.Max().Z()), float64(prevBox.Max().Z()))))

		earliestCollision := voxel.CollisionInfo{
			TimeOfImpact:  1,
			SurfaceNormal: mgl32.Vec3{0, 0, 0},
		}
		var earliestCollider *PhysicsComponent

		for yi := minY; yi <= maxY; yi++ {
			for zi := minZ; zi <= maxZ; zi++ {
				for xi := minX; xi <= maxX; xi++ {
					collidersHere := collidersAt(xi, yi, zi)
					if len(collidersHere) == 0 {
						continue
					}

					for _, collider := range collidersHere {
						if movingEntity == collider.GameObject {
							continue
						}
						// A thing that is walked through is not a thing that
						// gets shoved. PassThrough already takes an item out of
						// everybody else's way; this is the other half of the
						// same contract, and without it walking into an item
						// leaves the *item* resolving a collision against the
						// player - out of a box it is already deep inside, so
						// it goes the shortest way, which is downwards through
						// the floor and out of the map.
						//
						// Only against things that move under their own steam:
						// the level and its furniture are what an item rests
						// on, and it has to keep colliding with those.
						if movingEntity.PassThrough && collider.GameObject != nil &&
							collider.GameObject.AffectedByGravity() {
							continue
						}
						var c voxel.CollisionInfo
						if collider.GameObject != nil && collider.GameObject.HasOrientedCollider() {
							c = voxel.CheckAABBVsOBB(prevBox, collider.GameObject.WorldOBB(), velocity)
						} else {
							c = voxel.SweepAABB(prevBox, collider.AABB, velocity)
						}
						pairsChecked++
						if c.TimeOfImpact < earliestCollision.TimeOfImpact {
							earliestCollision = c
							earliestCollider = collider.GameObject
						}
					}

				}
			}
		}

		epsilon := float32(0.001)
		newPos = prevPos.Add(velocity.Mul(earliestCollision.TimeOfImpact).Add(earliestCollision.SurfaceNormal.Mul(epsilon)))
		if earliestCollision.TimeOfImpact == 1 {
			break
		}

		// One event for the surface that was actually hit, not for every
		// collider that happened to be in the way.
		events.Publish(events.EventPhysicsCollision, CollisionEvent{
			ObjectA:       movingEntity,
			ObjectB:       earliestCollider,
			WorldPos:      prevPos.Add(velocity.Mul(earliestCollision.TimeOfImpact)),
			SurfaceNormal: earliestCollision.SurfaceNormal,
		})
		if !movingEntity.GameObject().IsAlive() {
			return newPos, pairsChecked, hitNormals
		}
		hitNormals = append(hitNormals, earliestCollision.SurfaceNormal)

		BdotB := earliestCollision.SurfaceNormal.Dot(earliestCollision.SurfaceNormal)

		if BdotB != 0 {
			prevPos = newPos
			AdotB := (1 - earliestCollision.TimeOfImpact) * velocity.Dot(earliestCollision.SurfaceNormal)
			//translation = translation.AddFlat(velocity.Mul(1 - r.Result)).Sub(r.Normal.Mul(AdotB / BdotB))
			newPos = newPos.Add(velocity.Mul(1 - earliestCollision.TimeOfImpact).Sub(earliestCollision.SurfaceNormal.Mul(AdotB / BdotB)))
		}

	}

	return newPos, pairsChecked, hitNormals
}
