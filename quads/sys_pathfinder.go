package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/geometry"
    "github.com/memmaker/goray/quads/glhf"
)

type PathfindingSystem struct {
    movingObjects []*PathingComponent
    isSolid       func(x int32, y int32, z int32) bool
}

func NewPathfindingSystem(isSolid func(x int32, y int32, z int32) bool) *PathfindingSystem {
    return &PathfindingSystem{
        isSolid: isSolid,
    }
}

func (c *PathfindingSystem) AddMovingObject(m *PathingComponent) {
    c.movingObjects = append(c.movingObjects, m)
}

func (c *PathfindingSystem) Update(deltaTime float64) {
    for i := len(c.movingObjects) - 1; i >= 0; i-- {
        m := c.movingObjects[i]
        if m.IsDead(deltaTime) {
            c.movingObjects = append(c.movingObjects[:i], c.movingObjects[i+1:]...)
            continue
        }
        if !m.Enabled {
            continue
        }

        if m.IsAtCurrentWaypoint() {
            m.currentWaypoint++
            if m.currentWaypoint >= len(m.waypoints) {
                m.currentDestination = m.waypoints[0]
                m.waypoints = nil
                m.currentWaypoint = -1
                m.GameObject().Sprite().PlayAnimationLooping(int(AnimIdle))
                continue
            }
        }
        gameObject := m.GameObject()

        if !m.HasPathToDestination() {
            m.SetWaypoints(c.calculatePath(gameObject.Transform().GetPosition(), m.currentDestination))
            gameObject.Sprite().PlayAnimationLooping(int(AnimWalk))
        }

        if !m.HasWaypointsLeft() {
            continue
        }

        wayPoint := m.waypoints[m.currentWaypoint]
        currentPos := gameObject.Transform().GetPosition()
        directionAndSpeed := wayPoint.Sub(currentPos).Normalize().Mul(m.moveSpeed)
        newVelocity := directionAndSpeed.Mul(float32(deltaTime))
        gameObject.Physics().SetVelocity(newVelocity)
        gameObject.Transform().SetLookAt2D(wayPoint)
    }
}

func (c *PathfindingSystem) DrawPathfindingDebug(lineShader *glhf.Shader) {
    for _, m := range c.movingObjects {
        if !m.Enabled {
            continue
        }
        m.Draw(lineShader)
    }
}

type AstarFinder struct {
    neighbours func(geometry.Point) []geometry.Point
}

func (a AstarFinder) Neighbors(point geometry.Point) []geometry.Point {
    return a.neighbours(point)
}

func (a AstarFinder) Cost(point geometry.Point, point2 geometry.Point) int {
    return geometry.DistanceManhattan(point, point2)
}

func (a AstarFinder) Estimation(point geometry.Point, point2 geometry.Point) int {
    return geometry.DistanceSquared(point, point2)
}

func (c *PathfindingSystem) calculatePath(position mgl32.Vec3, destination mgl32.Vec3) []mgl32.Vec3 {
    heightPlaneOrigin := position.Y()
    heightPlaneDestination := destination.Y()
    if heightPlaneOrigin != heightPlaneDestination {
        panic("PathfindingSystem: calculatePath: height difference between origin and destination is not supported")
    }

    pathrange := geometry.NewPathRange(geometry.NewRect(0, 0, 64, 64))
    path := pathrange.AstarPath(AstarFinder{
        neighbours: func(point geometry.Point) []geometry.Point {
            var result []geometry.Point
            for _, p := range []geometry.Point{
                {X: point.X - 1, Y: point.Y},
                {X: point.X + 1, Y: point.Y},
                {X: point.X, Y: point.Y - 1},
                {X: point.X, Y: point.Y + 1},
            } {
                if c.isSolid(int32(p.X), int32(heightPlaneOrigin), int32(p.Y)) {
                    continue
                }
                result = append(result, p)
            }
            return result
        },
    }, topDownPoint(position), topDownPoint(destination))
    if path == nil {
        return nil
    }
    var result []mgl32.Vec3
    for _, p := range path[1:] {
        result = append(result, mgl32.Vec3{float32(p.X) + 0.5, heightPlaneOrigin, float32(p.Y) + 0.5})
    }

    return result
}

func topDownPoint(worldPos mgl32.Vec3) geometry.Point {
    return geometry.Point{X: int(worldPos.X()), Y: int(worldPos.Z())}
}
