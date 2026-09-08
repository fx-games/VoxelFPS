package quads

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

// Waypoint is one stop on a path. Climb marks the *arrival* at this stop as a
// ladder ascent/descent rather than a walk - see PathfindingSystem.calculatePath.
type Waypoint struct {
	Pos   mgl32.Vec3
	Climb bool
}

type PathingComponent struct {
	baseComponent
	waypoints          []Waypoint
	currentWaypoint    int
	moveSpeed          float32
	currentDestination mgl32.Vec3
	Enabled            bool
	// arrived is true once the current destination has been reached and stays
	// true until SetDestination gives it a new one. Without this, an idle actor
	// with an empty waypoint list looks identical to one that has never been
	// given a destination, and PathfindingSystem.Update ends up re-running A*
	// against the point it is already standing on, every frame, for as long as
	// it stands there - with a hundred actors idling that is a hundred wasted
	// searches a frame.
	arrived              bool
	vertexesForDebugDraw *glhf.VertexArray[glhf.GlFloat]
}

// defaultMoveSpeed is a brisk walk, well under the player's own run speed
// (pmSpeed in q3move.go, ~5.1 units/second) - a patrolling actor shouldn't
// visibly outrun the player. 16.0 lived here before PathfindingSystem.Update's
// velocity was also being scaled by deltaTime a second time in
// physicsSystem.Update, which made 16.0 actually move at 16.0/60th speed; now
// that only happens once, the old number would have actors sprinting past the
// player instead of patrolling.
const defaultMoveSpeed = 2.5

func NewPathingComponent() *PathingComponent {
	return &PathingComponent{
		moveSpeed: defaultMoveSpeed,
	}
}

func (p *PathingComponent) SetWaypoints(waypoints []Waypoint) {
	p.waypoints = waypoints
	p.currentWaypoint = 0
	p.Enabled = true
	p.arrived = len(waypoints) == 0
}

func (p *PathingComponent) SetDestination(pos mgl32.Vec3) {
	p.currentDestination = pos
	p.waypoints = nil
	p.currentWaypoint = -1
	p.Enabled = true
	p.arrived = false
}

func (p *PathingComponent) Destination() mgl32.Vec3 {
	return p.currentDestination
}

func (p *PathingComponent) HasPathToDestination() bool {
	if p.arrived {
		return true
	}
	if len(p.waypoints) == 0 {
		return false
	}
	// X/Z only: a destination's Y is whatever the caller happened to pass to
	// SetDestination (a map author's PatrolTo, say), while a waypoint's Y comes
	// from the pathfinder's own floor-height scan at that tile. Comparing the
	// full point would keep this false - and calculatePath re-run every single
	// frame, for every actor whose authored Y is not exactly the floor's real
	// height - forever, since neither side of that mismatch ever moves.
	last := p.waypoints[len(p.waypoints)-1].Pos
	const sameColumn = 0.5
	return math.Abs(float64(last.X()-p.currentDestination.X())) < sameColumn &&
		math.Abs(float64(last.Z()-p.currentDestination.Z())) < sameColumn
}

// waypointArrivalRadius is how close counts as "there". It has to clear one
// frame's worth of movement at a sane framerate and moveSpeed, or the object
// steps past the waypoint every frame without ever landing inside a tighter
// radius, and spends forever oscillating across it instead of advancing to
// the next one - motion that looks exactly like standing still. 0.01 was fine
// back when a waypoint move was (wrongly) scaled by deltaTime twice, making a
// frame's step tiny; now that PathfindingSystem.Update moves at the real
// moveSpeed, a frame's step is on the order of moveSpeed/60, comfortably
// under this.
const waypointArrivalRadius = 0.2

func (p *PathingComponent) IsAtCurrentWaypoint() bool {
	if len(p.waypoints) == 0 {
		return false
	}
	if p.currentWaypoint >= len(p.waypoints) || p.currentWaypoint < 0 {
		return false
	}
	wayPoint := p.waypoints[p.currentWaypoint]
	pos := p.resolver.Transform().GetPosition()
	if wayPoint.Climb {
		return pos.ApproxEqualThreshold(wayPoint.Pos, waypointArrivalRadius)
	}
	// X/Z only for the same reason HasPathToDestination is: a walking
	// waypoint's Y is a floor-height scan, not a promise about where physics
	// settles the object to the millimetre.
	dx := float64(pos.X() - wayPoint.Pos.X())
	dz := float64(pos.Z() - wayPoint.Pos.Z())
	return dx*dx+dz*dz < waypointArrivalRadius*waypointArrivalRadius
}

func (p *PathingComponent) HasWaypointsLeft() bool {
	return len(p.waypoints) > 0 && p.currentWaypoint < len(p.waypoints)
}

// HasArrived reports whether the current destination has been reached, so a
// caller (e.g. an Actor picking the next patrol point) can tell "idling
// because it got there" apart from "idling because it has nowhere to go yet".
func (p *PathingComponent) HasArrived() bool {
	return p.arrived
}

func (p *PathingComponent) Type() ComponentType {
	return ComponentTypePathing
}

func (p *PathingComponent) Draw(shader *glhf.Shader) {
	if p.vertexesForDebugDraw == nil {
		var rawVerts []glhf.GlFloat

		for i := 0; i < len(p.waypoints)-1; i++ {
			src := p.waypoints[i].Pos.Add(mgl32.Vec3{0, 0.3, 0})
			dst := p.waypoints[i+1].Pos.Add(mgl32.Vec3{0, 0.3, 0})
			rawVerts = appendLine(rawVerts, src, dst)
		}

		p.vertexesForDebugDraw = NewVertexSliceFromLines(shader, rawVerts)
	}

	p.vertexesForDebugDraw.Begin()
	p.vertexesForDebugDraw.Draw()
	p.vertexesForDebugDraw.End()
}
