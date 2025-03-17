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
    camera                  *core.FPSCamera
    collisionPairsChecked   int
}

func NewCollisionSolver(camera *core.FPSCamera, isSolid func(x int32, y int32, z int32) bool) *PhysicsSystem {
    return &PhysicsSystem{
        isSolid:                 isSolid,
        camera:                  camera,
        objectsOnGrid:           make(map[core.Int3]map[*PhysicsComponent]bool),
        accelerationFromGravity: mgl32.Vec3{0, -9.8, 0},
    }
}

func (c *PhysicsSystem) moveObjectOnGrid(obj *PhysicsComponent, oldPos mgl32.Vec3, newPos mgl32.Vec3) {
    c.removeFromGrid(oldPos, obj)
    c.addToGrid(newPos, obj)
}

const epsilon = 0.001

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

// can lead to stuff leaving the map and goign to infinity
func (c *PhysicsSystem) Update(deltaTime float64) {
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

    c.collisionPairsChecked = 0
    for id := len(c.movingObjects) - 1; id >= 0; id-- {
        obj := c.movingObjects[id]

        if obj.IsDead(deltaTime) {
            c.remove(id, obj)
            continue
        }

        if !obj.IsMovable() {
            continue
        }

        currentPosition := obj.Position()
        //acceleration := obj.Acceleration()
        velocity := obj.Velocity()

        if velocity == zeroVec3 {
            continue
        }
        newPos := currentPosition.Add(velocity.Mul(float32(deltaTime)))
        //newVelocity := velocity
        /*
           if obj.AffectedByGravity() {
               newVelocity = newVelocity.Add(c.accelerationFromGravity)
           }

        */

        //newPos := kinematic(acceleration, velocity, currentPosition, deltaTime)
        //newPos := obj.Position().Add(appliedVelocity)
        //obj.SetPosition(newPos)
        //c.moveObjectOnGrid(obj, previousPos, newPos)

        getCollidersAt := func(x, y, z int32) []LinkedAABB {
            if c.isSolid(x, y, z) {
                return []LinkedAABB{{
                    AABB: voxel.NewAABBFromMin(mgl32.Vec3{float32(x), float32(y), float32(z)}, mgl32.Vec3{1, 1, 1}),
                }}
            }
            mapPos := core.Int3{X: x, Y: y, Z: z}
            colliders, exists := c.objectsOnGrid[mapPos]
            if !exists {
                return nil
            }
            result := make([]LinkedAABB, 0, len(colliders))
            for collider := range colliders {
                if !collider.GameObject().IsAlive() {
                    continue
                }
                result = append(result, LinkedAABB{
                    AABB:       collider.AABB(),
                    GameObject: collider,
                })
            }
            return result
        }

        //projectile := obj.IsProjectile()
        correctedPos, pairsChecked, _ := CheckForCollisions(obj, newPos, c.camera, getCollidersAt)

        c.collisionPairsChecked += pairsChecked

        if obj.IsDead(0) {

            return
        }
        oldPos := obj.Position()
        obj.SetPosition(correctedPos)
        c.moveObjectOnGrid(obj, oldPos, correctedPos)
    }

    //c.updateProjectiles(deltaTime)
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
    c.addToGrid(obj.Position(), obj)
}

func (c *PhysicsSystem) addToGrid(pos mgl32.Vec3, obj *PhysicsComponent) {
    // check all 8 corners of the AABBAt
    aabb := obj.AABBAt(pos)

    corners := aabb.Corners()

    for _, corner := range corners {
        gridPos := core.Int3{X: int32(corner.X()), Y: int32(corner.Y()), Z: int32(corner.Z())}
        if c.objectsOnGrid[gridPos] == nil {
            c.objectsOnGrid[gridPos] = make(map[*PhysicsComponent]bool)
        }
        c.objectsOnGrid[gridPos][obj] = true
    }
}

func (c *PhysicsSystem) removeFromGrid(atPos mgl32.Vec3, obj *PhysicsComponent) {
    aabb := obj.AABBAt(atPos)
    corners := aabb.Corners()
    for _, corner := range corners {
        gridPos := core.Int3{X: int32(corner.X()), Y: int32(corner.Y()), Z: int32(corner.Z())}
        if c.objectsOnGrid[gridPos] != nil {
            delete(c.objectsOnGrid[gridPos], obj)
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

// CheckForCollisions returns the correction vector and whether or not a collision occurred.
// CANNOT BE CALLED WITH HIGH-VELOCITY OBJECTS
func CheckForCollisions(movingEntity *PhysicsComponent, newPos mgl32.Vec3, camera *core.FPSCamera, collidersAt func(x int32, y int32, z int32) []LinkedAABB) (mgl32.Vec3, int, bool) {
    pairsChecked := 0
    hitSomething := false
    // adapted from: https://luisreis.net/blog/aabb_collision_handling/
    prevPos := movingEntity.Position()
    boxFromPos := movingEntity.AABBAt

    for {
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
                        var c voxel.CollisionInfo
                        if collider.GameObject == nil {
                            a := prevBox
                            b := collider.AABB
                            c = voxel.SweepAABB(a, b, velocity)
                        } else { // TODO: THIS DOES NOT WORK
                            c = voxel.CheckAABBVsOBB(prevBox, collider.GameObject.OBB(camera), velocity)
                        }
                        if c.TimeOfImpact < 1 {
                            events.Publish(events.EventCollision, CollisionEvent{
                                ObjectA:       movingEntity,
                                ObjectB:       collider.GameObject,
                                WorldPos:      movingEntity.Position().Add(velocity.Mul(c.TimeOfImpact)),
                                SurfaceNormal: c.SurfaceNormal,
                            })
                            if !movingEntity.GameObject().IsAlive() {
                                return newPos, pairsChecked, hitSomething
                            }
                        }
                        pairsChecked++
                        if c.TimeOfImpact < earliestCollision.TimeOfImpact {
                            earliestCollision = c
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
        hitSomething = true

        BdotB := earliestCollision.SurfaceNormal.Dot(earliestCollision.SurfaceNormal)

        if BdotB != 0 {
            prevPos = newPos
            AdotB := (1 - earliestCollision.TimeOfImpact) * velocity.Dot(earliestCollision.SurfaceNormal)
            //translation = translation.AddFlat(velocity.Mul(1 - r.Result)).Sub(r.Normal.Mul(AdotB / BdotB))
            newPos = newPos.Add(velocity.Mul(1 - earliestCollision.TimeOfImpact).Sub(earliestCollision.SurfaceNormal.Mul(AdotB / BdotB)))
        }

    }

    return newPos, pairsChecked, hitSomething
}
