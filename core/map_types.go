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
	// Lights the map brings with it. Loading a map replaces the scene's lights with
	// these, so an empty list means an unlit map rather than the previous map's
	// lights left standing.
	Lights []MapLight
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
