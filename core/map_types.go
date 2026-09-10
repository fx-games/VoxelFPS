package core

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/geometry"
	"image/color"
	"math"
)

type MapLoadFunc func(filename string) MapLoadResult

// MapLight is a light the way a map file describes it: authored numbers, no GL and
// no renderer types. quads turns it into a LightInfo on load.
//
// Yaw and Pitch are degrees, and they are here rather than a direction vector
// because a level editor asks a human for angles, not for a unit vector. Yaw 0 with
// Pitch 0 points along -Z, yaw grows towards +X, and positive pitch tilts downwards
// - so a lamp is "yaw whatever, pitch 40" rather than three components that have to
// be normalised by hand.
type MapLight struct {
	// Position is in world units, not tiles. Y is height above the map floor.
	Position mgl32.Vec3
	Yaw      float32
	Pitch    float32

	Color     mgl32.Vec3
	Intensity float32
	// Angle is the full cone angle in degrees, Reach how far the light carries. Reach
	// doubles as the far plane of its shadow frustum, so a light cannot light what it
	// has no depth information for. Neither applies to a baked light.
	Angle float32
	Reach float32

	// Baked makes this light part of the level's voxel light instead of a slot in
	// the renderer's light list: it is flood-filled through the map once at load and
	// costs nothing per frame afterwards, but it cannot move, cannot be switched
	// off, has no cone and takes the baked light's colour rather than its own. This
	// is what a torch on a wall is, and it is why a map may hold hundreds of them
	// while the dynamic list holds MaxLights. Yaw, Pitch, Angle and Reach are
	// ignored for one.
	Baked bool
}

// LookAt points the light at a world position, expressed in the Yaw/Pitch the map
// format stores. Generated maps know what they want lit rather than what angle that
// works out to, and hand-authored ones get the angles from the editor either way.
func (l MapLight) LookAt(target mgl32.Vec3) MapLight {
	direction := target.Sub(l.Position)
	horizontal := mgl32.Vec2{direction.X(), direction.Z()}.Len()
	l.Yaw = mgl32.RadToDeg(float32(math.Atan2(float64(direction.X()), float64(-direction.Z()))))
	l.Pitch = mgl32.RadToDeg(float32(math.Atan2(float64(-direction.Y()), float64(horizontal))))
	return l
}

// Forward is the unit vector the light points along. See the Yaw/Pitch note above.
func (l MapLight) Forward() mgl32.Vec3 {
	yaw := float64(mgl32.DegToRad(l.Yaw))
	pitch := float64(mgl32.DegToRad(l.Pitch))
	cosPitch := float32(math.Cos(pitch))
	return mgl32.Vec3{
		float32(math.Sin(yaw)) * cosPitch,
		-float32(math.Sin(pitch)),
		-float32(math.Cos(yaw)) * cosPitch,
	}.Normalize()
}

// MapLoadResult is everything about a level that is not its blocks. The blocks
// are the level's voxel file and nothing else: there is one description of the
// geometry, it is the one the game runs on and the one the editor writes, and
// this holds the things a grid of blocks has no room for.
type MapLoadResult struct {
	PlayerSpawn  mgl32.Vec3
	PlayerDir    mgl32.Vec2
	CeilingColor color.Color
	FloorColor   color.Color
	// Entities are the things standing in the level, at absolute positions.
	Entities []EntitySpawn

	// How far the level can be seen through, and what an unlit corner of it
	// looks like. Both are zero on a map that has nothing to say about either,
	// which leaves the engine's own defaults alone - see Game.mapLoad. They are
	// here because they are look decisions belonging to one level: the shadow
	// rig pushes its fog past the far wall and drops its ambient floor so that
	// unlit really looks unlit, and neither is true of anywhere else.
	FogStart, FogEnd float32
	AmbientColor     mgl32.Vec3
	AmbientStrength  float32
	// Lights the map brings with it. Loading a map replaces the scene's lights with
	// these, so an empty list means an unlit map rather than the previous map's
	// lights left standing.
	Lights []MapLight
	// SkyFullSphere is a level with no horizon: the cubemap covers the whole
	// sphere instead of fading into the fog below eye level. That is what space
	// is, and it is a bool rather than a fade angle of zero because every level
	// written before there was one says nothing at all and must keep its
	// horizon.
	SkyFullSphere bool
	// Station is the rotating body this level is, or nil for a level that is
	// just a place. See Station.
	Station *Station

	// Panes are the level's glass: a window is a hole in the wall with one of
	// these hanging in it. It is a list of quads rather than a kind of block
	// because the blocks are opaque - the chunk pass has no blending and the
	// greedy mesher would cull the faces of whatever a see-through block stood
	// against - and because a quad is what the renderer already has a
	// translucent, world-oriented, one-draw-call pass for.
	Panes []Pane

	// Music is the track this level plays, by file name without the extension,
	// under assets/audio. Empty is silence. It belongs to the level rather than
	// to a table in the engine for the same reason the fog does: which deck you
	// are standing on is what decides it.
	Music string
}

// EntitySpawn is one entity and where it stands, in world units. A slice, not a
// map keyed by position: a level may put two things in the same place - two
// props snapped to the same tile centre is the obvious one - and a map keyed by
// position silently keeps only the last of them.
type EntitySpawn struct {
	Position mgl32.Vec3
	Entity   MapEntity
}

func (m MapLoadResult) CeilingColorVec4() mgl32.Vec4 {
	ceilingR, ceilingG, ceilingB, _ := m.CeilingColor.RGBA()
	return mgl32.Vec4{float32(ceilingR) / 0xffff, float32(ceilingG) / 0xffff, float32(ceilingB) / 0xffff, 1}
}

type MapEntity struct {
	Name                string
	TextureIndices      []uint16
	MeshName            string
	SpriteSheetName     string
	SpriteSheetTileSize geometry.Point
	IsTransparent       bool
	CreateCollider      bool

	// PassThrough makes the collider one the world moves straight through:
	// still on the physics grid, so the crosshair finds it and it can be
	// selected and shot at, but skipped by the collision solver so nothing is
	// stopped by it. It is what an exhibit standing in a row of five hundred
	// wants to be, and what a note lying on the floor already is.
	PassThrough bool `json:",omitempty"`

	// PatrolTo is the far end of a walking entity's beat. It walks there, then back
	// to where it spawned, forever. Nil means the spawner picks a default.
	//
	// Patrol supersedes it: a whole route, walked in order and then round to the
	// first point again, which is what the editor's path tool writes. An entity
	// with both keeps Patrol; one with neither gets the spawner's default.
	PatrolTo *mgl32.Vec3
	Patrol   []mgl32.Vec3

	// Facing is which way this entity is turned, as a direction on the floor.
	// The zero vector is "however the model was exported", which is what
	// everything placed before there was a rotate key is. See the editor's
	// prop rotation.
	Facing mgl32.Vec3

	// ClimbTo is the landing at the top of a ladder entity: the point pathfinding
	// steps straight to from the ladder's foot, whatever the height difference.
	// Nil means this entity is not a ladder. See Scenery.Climbable.
	ClimbTo *mgl32.Vec3

	// Code is the four digits a keypad asks for, on the keypad that asks for
	// them. It is on the entity rather than on the door because the keypad is
	// what the level author places and gives a number to; which door it
	// secures is the nearest one to it - see Game.bindKeypads.
	Code string

	// Text is what a note says. A note is a sheet of paper lying in the level
	// with a line of writing on it, and this is the writing: reading one puts
	// it in the player's inventory and learns any four-digit code in it.
	Text string

	// Sound is the recording an audio log carries, by its file name under
	// assets/audio/logs without the extension. It is a name rather than the
	// writing itself because a log says what it plays, and Text is only the
	// label on its lid - see AudioLog.
	Sound string

	// StepTo is the far side of a physically-bridged elevation change this
	// entity provides - a staircase's platform, say. Pathfinding otherwise
	// requires two neighbouring tiles to have exactly the same floor height:
	// even one voxel of difference is taller than the physics step height can
	// carry a walker over unassisted (see quads.climbStep's absence - there
	// isn't one), so without this a tile a staircase leads to is unreachable
	// from its foot exactly like any other wall. Unlike ClimbTo this is an
	// ordinary walk: gravity and the physics step-up carry the moment across,
	// the same as they do for the player on the same staircase.
	StepTo *mgl32.Vec3
}

func LineToPlaneIntersection(lineStart, lineEnd, planeCoord, planeNormal mgl32.Vec3) float32 {
	NdotU := planeNormal.Dot(lineEnd)
	if NdotU == 0 {
		return math.MaxFloat32
	}
	return planeNormal.Dot(planeCoord.Sub(lineStart)) / NdotU
}

// Station is a level that is one rotating body - a space station - described in
// the only frame anything here is ever measured in: its own. Nothing rotates.
// The blocks never move, gravity inside stays -Y, and walking and jumping
// inside are the ordinary ones, which is the whole point: movement is relative
// to the station because the station is the frame. What turns is the sky, at
// Spin, which is the same relative motion a turning hull under fixed stars
// shows - through a window, past a hull, either way.
//
// ponytail: the station is the rest frame, so a player floating free outside
// turns with it rather than watching it turn. An inertial EVA needs the
// centrifugal and Coriolis terms on the free player, or a world that really
// rotates - which is a different engine.
type Station struct {
	// Spin is degrees a second, about Y - the axis a level's own up already is.
	Spin float32

	// The pressurised part of the level, as an upright cylinder: inside it there
	// is air, a floor and gravity, outside it there is vacuum and the suit. An
	// airlock is not a switch, it is only where the hull lets you cross this.
	CenterX, CenterZ float32
	Radius           float32
	MinY, MaxY       float32
}

// Pressurised reports whether a point is inside the hull. It is a test on the
// position and nothing else - no state, no entering and leaving - so there is
// no way to end up on the wrong side of it.
func (s Station) Pressurised(pos mgl32.Vec3) bool {
	if pos.Y() < s.MinY || pos.Y() > s.MaxY {
		return false
	}
	dx, dz := pos.X()-s.CenterX, pos.Z()-s.CenterZ
	return dx*dx+dz*dz <= s.Radius*s.Radius
}

// Pane is one sheet of glass: where it hangs, which way it faces and how big
// it is, in world units. It is solid to walk into and see-through to look at,
// so the cell it stands in counts as wall for the physics - see Pane.Cell.
type Pane struct {
	Position mgl32.Vec3
	Normal   mgl32.Vec3
	Width    float32
	Height   float32
}

// Cell is the voxel this pane glazes: the pane sits on that voxel's own face,
// so stepping back half a unit along the normal lands in the middle of it.
// Deriving it rather than storing it is what keeps a level from being able to
// say the glass is in one place and the wall it fills in another.
func (p Pane) Cell() Int3 {
	middle := p.Position.Sub(p.Normal.Normalize().Mul(0.5))
	return Int3{
		X: int32(math.Floor(float64(middle.X()))),
		Y: int32(math.Floor(float64(middle.Y()))),
		Z: int32(math.Floor(float64(middle.Z()))),
	}
}
