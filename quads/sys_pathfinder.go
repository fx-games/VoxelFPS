package quads

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxellib/voxel/nav"
)

// ClimbLink is a ladder: standing at Bottom lets a path step straight to Top
// (and back), whatever the height difference. The waypoint it produces
// switches gravity off and drives straight at Top's exact position - see
// PathfindingSystem.Update. Positions are feet positions in world space.
type ClimbLink struct {
	Bottom, Top mgl32.Vec3
}

// WalkLink is a staircase, or anything else that physically bridges an
// elevation change a voxel walk would refuse: standing at Bottom lets a path
// step straight to Top, but - unlike a ClimbLink - the waypoint is an ordinary
// walk. Gravity and the physics step-up carry the climb, as they do for the
// player on the same staircase.
type WalkLink struct {
	Bottom, Top mgl32.Vec3
}

type PathfindingSystem struct {
	movingObjects []*PathingComponent
	graph         *nav.Graph
	climbLinks    []ClimbLink
	walkLinks     []WalkLink
}

// solidFunc adapts the map's solidity test to nav.World.
type solidFunc func(x, y, z int32) bool

func (f solidFunc) Solid(x, y, z int) bool { return f(int32(x), int32(y), int32(z)) }

// NewPathfindingSystem walks the cells isSolid says are open, one cell wide and
// one tall like the actors. An actor walks only between cells of the same
// height (a voxel ledge is a wall to the player too, see pmStepSize); stairs
// and ladders say where they bridge a height change, see AddWalkLink and
// AddClimbLink.
func NewPathfindingSystem(isSolid func(x int32, y int32, z int32) bool) *PathfindingSystem {
	return &PathfindingSystem{graph: &nav.Graph{
		W:        solidFunc(isSolid),
		Size:     [2]int{1, 1},
		Passable: func(from, to nav.Pos) bool { return from.Y == to.Y },
	}}
}

// AddClimbLink registers a ladder. Both directions are added, so a path may
// climb or descend it.
func (c *PathfindingSystem) AddClimbLink(bottom, top mgl32.Vec3) {
	c.climbLinks = append(c.climbLinks, ClimbLink{bottom, top}, ClimbLink{top, bottom})
}

// AddWalkLink registers a staircase (or similar). Both directions are added,
// so a path may go up or down it.
func (c *PathfindingSystem) AddWalkLink(bottom, top mgl32.Vec3) {
	c.walkLinks = append(c.walkLinks, WalkLink{bottom, top}, WalkLink{top, bottom})
}

func (c *PathfindingSystem) AddMovingObject(m *PathingComponent) {
	c.movingObjects = append(c.movingObjects, m)
}

// cellOf is the walkable feet cell at or just below a world position.
func (c *PathfindingSystem) cellOf(pos mgl32.Vec3) (nav.Pos, bool) {
	x, y, z := int(math.Floor(float64(pos.X()))), int(math.Floor(float64(pos.Y()))), int(math.Floor(float64(pos.Z())))
	return c.graph.Ground(x, y-1, z)
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
				m.waypoints = nil
				m.currentWaypoint = -1
				m.arrived = true
				m.GameObject().Sprite().PlayAnimationLooping(int(AnimIdle))
				m.GameObject().Physics().ApplyGravity = true
				continue
			}
		}
		gameObject := m.GameObject()

		if !m.HasPathToDestination() {
			m.SetWaypoints(c.calculatePath(gameObject.Transform().GetPosition(), m.currentDestination))
			// Only on the transition into walking. PlayAnimationLooping restarts
			// the animation from frame 0, so calling it on every re-path froze
			// the legs of anything re-pathing faster than the walk cycle - a
			// companion re-aimed at a moving player four times a second (see
			// followPlayer) never got past its first frame.
			if !gameObject.Sprite().IsPlaying(int(AnimWalk)) {
				gameObject.Sprite().PlayAnimationLooping(int(AnimWalk))
			}
		}

		if !m.HasWaypointsLeft() {
			continue
		}

		wayPoint := m.waypoints[m.currentWaypoint]
		currentPos := gameObject.Transform().GetPosition()
		// A ladder rung is climbed, not walked: gravity gives way to the path's
		// own velocity for exactly as long as this waypoint is the target.
		//
		// ponytail: this is a straight 3D line from the ladder's foot to its
		// landing, not a walk-to-the-rail-then-climb-straight-up animation. It
		// looks like flying diagonally rather than climbing if the two are far
		// apart in x/z. Good enough for "the actor gets up there"; a dedicated
		// climb pose would want the ladder's own line as a mid waypoint.
		gameObject.Physics().ApplyGravity = !wayPoint.Climb
		directionAndSpeed := wayPoint.Pos.Sub(currentPos).Normalize().Mul(m.moveSpeed)
		// Velocity is real units/second, same as everywhere else PhysicsComponent
		// is driven from (q3move, gravity) - physicsSystem.Update multiplies by
		// deltaTime itself when it integrates position. Scaling by deltaTime
		// again here used to leave every actor crawling at moveSpeed/60 or so at
		// 60fps: easy to mistake for "not moving" at a glance, only visible as
		// motion at all because it compounds over many frames.
		gameObject.Physics().SetVelocity(directionAndSpeed)
		gameObject.Transform().SetLookAt2D(wayPoint.Pos)
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

// maxPathNodes caps one search's expansions.
const maxPathNodes = 20000

// calculatePath is a nav.Graph search over the voxels plus the registered
// links, smoothed to straight runs. Ladder waypoints (Climb) pin the actor to
// the ladder's foot and landing; smoothing keeps both ends of any height
// change, so it never cuts across one.
func (c *PathfindingSystem) calculatePath(position mgl32.Vec3, destination mgl32.Vec3) []Waypoint {
	from, ok := c.cellOf(position)
	if !ok {
		return nil
	}
	to, ok := c.cellOf(destination)
	if !ok {
		return nil
	}
	links := make(map[nav.Pos][]nav.Pos, len(c.climbLinks)+len(c.walkLinks))
	climbTop := make(map[[2]nav.Pos]float32, len(c.climbLinks))
	for _, l := range c.climbLinks {
		b, okB := c.cellOf(l.Bottom)
		t, okT := c.cellOf(l.Top)
		if okB && okT {
			links[b] = append(links[b], t)
			climbTop[[2]nav.Pos{b, t}] = l.Top.Y()
		}
	}
	for _, l := range c.walkLinks {
		b, okB := c.cellOf(l.Bottom)
		t, okT := c.cellOf(l.Top)
		if okB && okT {
			links[b] = append(links[b], t)
		}
	}
	c.graph.Links = func(p nav.Pos, buf []nav.Pos) []nav.Pos { return append(buf, links[p]...) }

	path, found := c.graph.Path(from, to, maxPathNodes)
	if !found {
		return nil
	}
	path = c.graph.Smooth(path)
	result := make([]Waypoint, 0, len(path)-1)
	for i := 1; i < len(path); i++ {
		p := path[i]
		if topY, isClimb := climbTop[[2]nav.Pos{path[i-1], p}]; isClimb {
			result = append(result, Waypoint{Pos: mgl32.Vec3{float32(p.X) + 0.5, topY, float32(p.Z) + 0.5}, Climb: true})
			continue
		}
		result = append(result, Waypoint{Pos: mgl32.Vec3{float32(p.X) + 0.5, float32(p.Y), float32(p.Z) + 0.5}})
	}
	return result
}
