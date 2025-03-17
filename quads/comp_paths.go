package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

type PathingComponent struct {
    baseComponent
    waypoints            []mgl32.Vec3
    currentWaypoint      int
    moveSpeed            float32
    currentDestination   mgl32.Vec3
    Enabled              bool
    vertexesForDebugDraw *glhf.VertexSlice[glhf.GlFloat]
}

func NewPathingComponent() *PathingComponent {
    return &PathingComponent{
        moveSpeed: 16.0,
    }
}

func (p *PathingComponent) SetWaypoints(waypoints []mgl32.Vec3) {
    p.waypoints = waypoints
    p.currentWaypoint = 0
    p.Enabled = true
}

func (p *PathingComponent) SetDestination(pos mgl32.Vec3) {
    p.currentDestination = pos
    p.waypoints = nil
    p.currentWaypoint = -1
    p.Enabled = true
}

func (p *PathingComponent) HasPathToDestination() bool {
    if len(p.waypoints) == 0 {
        return false
    }
    return p.waypoints[len(p.waypoints)-1].ApproxEqualThreshold(p.currentDestination, 0.01)
}

func (p *PathingComponent) IsAtCurrentWaypoint() bool {
    if len(p.waypoints) == 0 {
        return false
    }
    if p.currentWaypoint >= len(p.waypoints) || p.currentWaypoint < 0 {
        return false
    }
    wayPoint := p.waypoints[p.currentWaypoint]
    return p.resolver.Transform().GetPosition().ApproxEqualThreshold(wayPoint, 0.01)
}

func (p *PathingComponent) HasWaypointsLeft() bool {
    return len(p.waypoints) > 0 && p.currentWaypoint < len(p.waypoints)
}

func (p *PathingComponent) Type() ComponentType {
    return ComponentTypePathing
}

func (p *PathingComponent) Draw(shader *glhf.Shader) {
    if p.vertexesForDebugDraw == nil {
        var rawVerts []glhf.GlFloat

        for i := 0; i < len(p.waypoints)-1; i++ {
            src := p.waypoints[i].Add(mgl32.Vec3{0, 0.3, 0})
            dst := p.waypoints[i+1].Add(mgl32.Vec3{0, 0.3, 0})
            rawVerts = appendLine(rawVerts, src, dst)
        }

        p.vertexesForDebugDraw = NewVertexSliceFromLines(shader, rawVerts)
    }

    p.vertexesForDebugDraw.Begin()
    p.vertexesForDebugDraw.Draw()
    p.vertexesForDebugDraw.End()
}
