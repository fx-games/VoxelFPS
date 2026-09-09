package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads/voxel"
)

// The shadow test map. Its whole reason to exist: shadows showed up on walls
// and never on the floor, and neither the LDtk map nor
// the stress map could settle why. The LDtk map's lights are horizontal at head
// height, which turns every floor shadow into a grazing smear that looks the same
// whether the bias is right or wrong; the stress map is a thousand barrels under
// eight lights, which is a performance rig, not a microscope.
//
// So: one caster, one light, one wall, nothing else. The scientist walks a straight
// beat parallel to the wall, the light hangs above and behind him pointing down at
// about 40 degrees, and a correct renderer therefore owes two shadows at once - one
// stretched across the floor and one bent up the wall - from a single caster that
// keeps moving. A bias that is too large loses the floor half first, which is
// exactly the failure this map is here to make obvious.
//
// Deterministic, like the stress map: same geometry every run, so two screenshots
// taken a week apart are comparable.
const (
	shadowMapSize = 20 // tiles per side, walls included

	shadowWallZ     = 6  // the back wall the shadow climbs
	shadowWallMinX  = 4  // its extent, so it is a wall and not the room's border
	shadowWallMaxX  = 16 //
	shadowWallHeigh = 4  // tall enough that the whole caster fits on it

	// The beat sits one clear tile in front of the wall, and that distance is the
	// whole design of the map rather than a round number. A caster one unit tall
	// throws a shadow 1/tan(elevation) long; whatever of that does not fit on the
	// floor before the wall is what climbs the wall. Put the beat far out and the
	// shadow lands entirely on the floor; put it against the wall and there is no
	// floor shadow to lose. 1.5 units in front of the wall face, under a light at
	// about 27 degrees, splits it: 1.5 units across the floor, the rest up the wall.
	shadowPatrolZ    = 8  // tiles are 1 wide, so this is z 8.5, and the wall face is z 7
	shadowPatrolMinX = 7  //
	shadowPatrolMaxX = 13 //

	shadowCeiling = 6 // the light hangs below this
)

// scientistSheet is the walking NPC used as the caster. Same sheet and frame list
// the LDtk map gives its Scientist, because spawnEntity keys the pathing component
// off that name.
func scientistEntity(patrolTo mgl32.Vec3) core.MapEntity {
	return core.MapEntity{
		Name:                "Scientist",
		SpriteSheetName:     "scientist",
		SpriteSheetTileSize: geometry.Point{X: 64, Y: 64},
		TextureIndices:      []uint16{0, 1, 2, 3, 4, 5, 6, 7},
		CreateCollider:      true,
		PatrolTo:            &patrolTo,
	}
}

func loadShadowTestMap() (core.MapLoadResult, *voxel.Map) {
	size := int32(shadowMapSize)
	blocks := newLevel(voxel.Int3{X: size, Y: shadowCeiling + 1, Z: size})
	wall := roomBlock(0, 2, 3)
	paintRoom(blocks, wall)

	// The one thing in the room besides the caster: a wall for its shadow to
	// climb, short enough to see over from the corner the player stands in.
	fillBlocks(blocks,
		voxel.Int3{X: shadowWallMinX, Y: groundLevel, Z: shadowWallZ},
		voxel.Int3{X: shadowWallMaxX, Y: shadowWallHeigh, Z: shadowWallZ},
		wall)

	// The caster spawns at one end of its beat and walks to the other. The pathfinder
	// sends it back to where it started once it arrives, so this is a patrol without
	// anything else having to drive it.
	caster := core.EntitySpawn{
		Position: tileCentre(shadowPatrolMinX, shadowPatrolZ, groundLevel),
		Entity: scientistEntity(
			mgl32.Vec3{float32(shadowPatrolMaxX) + 0.5, 1, float32(shadowPatrolZ) + 0.5},
		),
	}

	return core.MapLoadResult{
		// Off to one side on purpose. Head on, the caster stands between the camera
		// and its own floor shadow and hides the very thing the map is for. From the
		// corner the shadow rakes across the floor and up the wall in full view.
		PlayerSpawn:  mgl32.Vec3{15.5, 1, 13.5},
		PlayerDir:    mgl32.Vec2{-4.5, -5.5},
		CeilingColor: color.RGBA{60, 60, 70, 255},
		FloorColor:   color.RGBA{40, 40, 40, 255},
		Entities:     []core.EntitySpawn{caster},
		Lights:       []core.MapLight{shadowTestLight()},
	}, blocks
}

// shadowTestLight is the single light the map is judged by: above and behind the
// beat, pointing down at it at about 26 degrees. Both of the things a depth bias
// trades off are in one frame - the floor it grazes and the wall it faces.
//
// The elevation is not free. Steeper looks more like a lamp but shortens the shadow
// until it no longer reaches the wall at all, which was the first version of this
// map and it showed nothing: the caster stood on its own shadow and hid it. This
// angle is the shallowest that still reads as "from above".
func shadowTestLight() core.MapLight {
	position := mgl32.Vec3{float32(shadowMapSize) / 2, 4, 13.6}
	aimAt := mgl32.Vec3{float32(shadowMapSize) / 2, 1.5, float32(shadowPatrolZ) + 0.5}

	return core.MapLight{
		Position:  position,
		Color:     mgl32.Vec3{1, 0.95, 0.85},
		Intensity: 8,
		// 70 degrees covers the whole beat from here with room to spare, without
		// spreading 512 texels so thin that this becomes a test of the shadow
		// resolution instead. TestShadowTestLightPointsDownAtTheBeat holds it to that.
		Angle: 70,
		Reach: 20,
	}.LookAt(aimAt)
}
