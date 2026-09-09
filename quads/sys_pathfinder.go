package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads/glhf"
)

// pathGridSize bounds the A* search area: any (x,z) cell outside a square this
// big is unreachable as far as pathfinding is concerned. Big enough for any
// map this engine ships a level for today.
const pathGridSize = 128

// ClimbLink is a ladder: standing at Bottom lets a path step straight to Top
// (and back), whatever the height difference, instead of the ordinary
// same-height check every other neighbour has to pass. The waypoint it
// produces switches gravity off and drives straight at Top's exact position -
// see PathfindingSystem.Update.
type ClimbLink struct {
	Bottom, Top   geometry.Point
	BottomY, TopY float32
}

// WalkLink is a staircase, or anything else that physically bridges an
// elevation change a bare height-delta check would refuse: standing at Bottom
// lets a path step straight to Top even though their floor heights differ,
// but - unlike a ClimbLink - the resulting waypoint is an ordinary walk.
// Gravity and the physics step-up carry the actual climb, the same as they do
// for the player on the same staircase; this only tells the search the two
// tiles are neighbours at all.
type WalkLink struct {
	Bottom, Top geometry.Point
}

type PathfindingSystem struct {
	movingObjects []*PathingComponent
	isSolid       func(x int32, y int32, z int32) bool
	maxHeight     int32
	climbLinks    []ClimbLink
	walkLinks     []WalkLink
}

// NewPathfindingSystem takes the same solidity test the voxel collision
// system uses, plus the map's declared height in voxels (core.MapLoadResult's
// MapDimensions[1], not the voxel buffer's own size - that is padded up to a
// whole chunk, and every layer above what the map actually declared is
// unused padding, not carved-out air. groundHeight scans a column looking for
// the first gap above the floor, and padding above the map's own ceiling
// would never supply one).
func NewPathfindingSystem(isSolid func(x int32, y int32, z int32) bool, maxHeight int32) *PathfindingSystem {
	return &PathfindingSystem{
		isSolid:   isSolid,
		maxHeight: maxHeight,
	}
}

// AddClimbLink registers a ladder. Both directions are added, so a path may
// climb or descend it.
func (c *PathfindingSystem) AddClimbLink(bottom, top geometry.Point, bottomY, topY float32) {
	c.climbLinks = append(c.climbLinks,
		ClimbLink{Bottom: bottom, Top: top, BottomY: bottomY, TopY: topY},
		ClimbLink{Bottom: top, Top: bottom, BottomY: topY, TopY: bottomY},
	)
}

// AddWalkLink registers a staircase (or similar). Both directions are added,
// so a path may go up or down it.
func (c *PathfindingSystem) AddWalkLink(bottom, top geometry.Point) {
	c.walkLinks = append(c.walkLinks,
		WalkLink{Bottom: bottom, Top: top},
		WalkLink{Bottom: top, Top: bottom},
	)
}

func (c *PathfindingSystem) AddMovingObject(m *PathingComponent) {
	c.movingObjects = append(c.movingObjects, m)
}

// groundHeight is the world Y of the walkable surface at (x,z): the first gap
// directly above the floor slab every tile is built on, and whether there is
// a floor there at all - a column with none is a hole, not a floor
// pathfinding can stand on.
//
// This has to scan up from the floor rather than down from the ceiling: a
// room is floor, then air, then ceiling, and a scan starting at the top hits
// the ceiling first - every column in an ordinary roofed room, reporting the
// underside of the ceiling as "the floor" and turning the whole map into one
// flat plane nothing could actually stand on.
//
// ponytail: one floor per (x,z) column. Two floors stacked over the same tile
// (a mezzanine with open space above and below it) would need a proper 3D
// navmesh instead of a per-column height scan; every map that ships today
// keeps its floors apart in x/z instead, ladders and stairs included.
func (c *PathfindingSystem) groundHeight(x, z int32) (int32, bool) {
	if !c.isSolid(x, 0, z) {
		return 0, false
	}
	for y := int32(1); y < c.maxHeight; y++ {
		if !c.isSolid(x, y, z) {
			return y, true
		}
	}
	return c.maxHeight, true
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
			if gameObject.Sprite().CurrentAnimation() != int(AnimWalk) {
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

// calculatePath is a 2D grid search whose walkability test looks up each
// cell's own floor height instead of assuming one height for the whole map:
// that is what makes a route across a staircase or up a ladder look like an
// ordinary A* neighbour list rather than a separate 3D search.
//
// Two adjacent cells are neighbours only when they share the exact same floor
// height, or a WalkLink/ClimbLink explicitly bridges them - there is no
// tolerance for "just one voxel of rise, that is probably fine". A real
// staircase gets there over four treads each well under the physics step
// height; a single bare voxel of unassisted rise is a wall to the player too,
// and pathfinding treating every such ledge as free to walk up was an actor
// climbing terrain the player could not. Two floors with no stairs and no
// ClimbLink between them are simply not neighbours, and the search returns no
// path - same contract as before, a nil path the caller already knows how to
// sit out.
func (c *PathfindingSystem) calculatePath(position mgl32.Vec3, destination mgl32.Vec3) []Waypoint {
	origin := topDownPoint(position)
	target := topDownPoint(destination)

	climbFrom := make(map[geometry.Point][]ClimbLink, len(c.climbLinks))
	climbEdge := make(map[[2]geometry.Point]ClimbLink, len(c.climbLinks))
	for _, link := range c.climbLinks {
		climbFrom[link.Bottom] = append(climbFrom[link.Bottom], link)
		climbEdge[[2]geometry.Point{link.Bottom, link.Top}] = link
	}
	walkFrom := make(map[geometry.Point][]geometry.Point, len(c.walkLinks))
	for _, link := range c.walkLinks {
		walkFrom[link.Bottom] = append(walkFrom[link.Bottom], link.Top)
	}

	pathrange := geometry.NewPathRange(geometry.NewRect(0, 0, pathGridSize, pathGridSize))
	path := pathrange.AstarPath(AstarFinder{
		neighbours: func(point geometry.Point) []geometry.Point {
			height, hasFloor := c.groundHeight(int32(point.X), int32(point.Y))
			if !hasFloor {
				return nil
			}
			var result []geometry.Point
			for _, p := range []geometry.Point{
				{X: point.X - 1, Y: point.Y},
				{X: point.X + 1, Y: point.Y},
				{X: point.X, Y: point.Y - 1},
				{X: point.X, Y: point.Y + 1},
			} {
				neighbourHeight, ok := c.groundHeight(int32(p.X), int32(p.Y))
				if !ok || neighbourHeight != height {
					continue
				}
				if c.isSolid(int32(p.X), neighbourHeight, int32(p.Y)) {
					continue
				}
				result = append(result, p)
			}
			for _, link := range climbFrom[point] {
				result = append(result, link.Top)
			}
			result = append(result, walkFrom[point]...)
			return result
		},
	}, origin, target)
	if path == nil {
		return nil
	}

	result := make([]Waypoint, 0, len(path)-1)
	for i := 1; i < len(path); i++ {
		prev, p := path[i-1], path[i]
		if link, isClimb := climbEdge[[2]geometry.Point{prev, p}]; isClimb {
			result = append(result, Waypoint{
				Pos:   mgl32.Vec3{float32(p.X) + 0.5, link.TopY, float32(p.Y) + 0.5},
				Climb: true,
			})
			continue
		}
		height, _ := c.groundHeight(int32(p.X), int32(p.Y))
		result = append(result, Waypoint{Pos: mgl32.Vec3{float32(p.X) + 0.5, float32(height), float32(p.Y) + 0.5}})
	}

	return c.pruneLineOfSight(position, result)
}

// losSampleStep is how finely pruneLineOfSight samples a candidate straight
// line before trusting it. Fine enough that a single solid grid cell astride
// the line cannot slip between two samples.
const losSampleStep = 0.5

// pruneLineOfSight collapses runs of waypoints a straight line already
// crosses cleanly. A* only ever steps N/S/E/W, so an unobstructed diagonal
// walk across open ground turns into a staircase of those single-axis hops
// with nothing actually in the way between the ends - this is the standard
// "string pulling" cleanup for a grid A* path.
//
// Ladder waypoints (Climb) are never skipped over, and never skipped past:
// they pin the actor to the ladder's exact foot and landing, which a cut
// straight line would miss. Splitting the waypoint list on them and pruning
// each climb-free run independently keeps that guarantee for free.
func (c *PathfindingSystem) pruneLineOfSight(origin mgl32.Vec3, waypoints []Waypoint) []Waypoint {
	result := make([]Waypoint, 0, len(waypoints))
	fromPos := origin
	segStart := 0
	for i, wp := range waypoints {
		if wp.Climb {
			result = append(result, c.pruneRun(fromPos, waypoints[segStart:i])...)
			result = append(result, wp)
			fromPos = wp.Pos
			segStart = i + 1
		}
	}
	result = append(result, c.pruneRun(fromPos, waypoints[segStart:])...)
	return result
}

// pruneRun greedily keeps the farthest waypoint in a climb-free run that is
// still in a straight, flat, unobstructed line from the last kept point -
// classic string pulling.
func (c *PathfindingSystem) pruneRun(fromPos mgl32.Vec3, run []Waypoint) []Waypoint {
	if len(run) < 2 {
		return run
	}
	result := make([]Waypoint, 0, len(run))
	anchor := fromPos
	i := 0
	for i < len(run) {
		j := len(run) - 1
		for j > i && !c.hasLineOfSight(anchor, run[j].Pos) {
			j--
		}
		result = append(result, run[j])
		anchor = run[j].Pos
		i = j + 1
	}
	return result
}

// hasLineOfSight is whether every grid cell the straight line from a to b
// crosses is open floor at exactly the same height as both ends - the same
// same-height requirement calculatePath's own neighbour search already
// enforces for an ordinary walk, just tested along a line instead of a grid
// step.
func (c *PathfindingSystem) hasLineOfSight(a, b mgl32.Vec3) bool {
	height, ok := c.groundHeight(int32(a.X()), int32(a.Z()))
	if !ok {
		return false
	}
	if h, ok := c.groundHeight(int32(b.X()), int32(b.Z())); !ok || h != height {
		return false
	}
	delta := b.Sub(a)
	dist := mgl32.Vec2{delta.X(), delta.Z()}.Len()
	steps := int(dist/losSampleStep) + 1
	for i := 0; i <= steps; i++ {
		t := float32(i) / float32(steps)
		p := a.Add(delta.Mul(t))
		x, z := int32(p.X()), int32(p.Z())
		h, ok := c.groundHeight(x, z)
		if !ok || h != height || c.isSolid(x, height, z) {
			return false
		}
	}
	return true
}

func topDownPoint(worldPos mgl32.Vec3) geometry.Point {
	return geometry.Point{X: int(worldPos.X()), Y: int(worldPos.Z())}
}
