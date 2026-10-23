package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// The shadow rig, as it ships. It exists so shadow bias can be judged by eye,
// and it only works if three things hold at once: the caster can walk its beat,
// the light reaches the whole of it, and the shadow crosses some floor before it
// climbs the wall. All three were easy to break by moving a constant, and are
// now easy to break by hand-editing assets/maps/shadowtest - which is what these
// read, rather than a copy of the numbers.
type shadowRig struct {
	map_   core.MapLoadResult
	blocks *voxel.Map
	light  core.MapLight
	// The caster's beat, from where it spawns to where it patrols.
	from, to mgl32.Vec3
	// The free standing wall the shadow climbs: the z of its row, the z of the
	// face pointing at the caster, and the height of its top.
	wallZ, wallFaceZ, wallTopY float32
}

func loadShadowRig(t *testing.T) shadowRig {
	t.Helper()
	result, blocks, err := loadMap("shadowtest")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entities) != 1 {
		t.Fatalf("%d entities in the rig, want exactly the one caster", len(result.Entities))
	}
	caster := result.Entities[0]
	// spawnEntity keys the pathing component off this name.
	if caster.Entity.Name != "Scientist" {
		t.Errorf("caster is %q, which gets no pathing component and will not move", caster.Entity.Name)
	}
	if caster.Entity.PatrolTo == nil {
		t.Fatal("caster has no patrol destination, so it stands still")
	}
	if len(result.Lights) != 1 {
		t.Fatalf("%d lights in the rig, want exactly the one being judged", len(result.Lights))
	}

	rig := shadowRig{
		map_:   result,
		blocks: blocks,
		light:  result.Lights[0],
		from:   caster.Position,
		to:     *caster.Entity.PatrolTo,
	}
	rig.wallZ, rig.wallTopY = findFreeStandingWall(t, blocks)
	rig.wallFaceZ = rig.wallZ + 1
	return rig
}

// findFreeStandingWall looks for the one row of interior blocks the rig has in
// it, and reports which row and how tall. A wall that reached the border would
// be a room divider, and the caster could not be seen from the player's side of
// it - so a row that touches either edge is not the wall, it is the border.
func findFreeStandingWall(t *testing.T, blocks *voxel.Map) (wallZ, topY float32) {
	t.Helper()
	size := blocks.Bounds()
	for z := int32(1); z < size.Z-1; z++ {
		var width, top int32
		for x := int32(1); x < size.X-1; x++ {
			if !blocks.IsSolidBlockAt(x, groundLevel, z) {
				continue
			}
			width++
			for y := int32(groundLevel); y < size.Y-1 && blocks.IsSolidBlockAt(x, y, z); y++ {
				top = max(top, y)
			}
		}
		if width > 0 {
			if width >= size.X-2 {
				t.Fatalf("the wall at z=%d spans the room, so it is a divider and not a free standing wall", z)
			}
			return float32(z), float32(top + 1)
		}
	}
	t.Fatal("no free standing wall in the rig: there is nothing to cast a shadow on")
	return 0, 0
}

func TestShadowRigHasAWallAndAClearBeat(t *testing.T) {
	rig := loadShadowRig(t)

	// The pathfinder walks tile to tile in a straight line here; a single solid
	// tile on the beat makes it path around, or fail, and the shadow stops moving.
	if int32(rig.from.Z()) != int32(rig.to.Z()) {
		t.Fatalf("the beat runs from z=%.1f to z=%.1f, so it is not the straight line the rig assumes",
			rig.from.Z(), rig.to.Z())
	}
	beatZ := int32(rig.from.Z())
	minX, maxX := int32(min(rig.from.X(), rig.to.X())), int32(max(rig.from.X(), rig.to.X()))
	for x := minX; x <= maxX; x++ {
		if rig.blocks.IsSolidBlockAt(x, groundLevel, beatZ) {
			t.Fatalf("the beat is blocked at x=%d", x)
		}
	}

	// The beat has to be clear of the wall, or the caster stands in it.
	if float32(beatZ) <= rig.wallZ {
		t.Fatalf("the beat at z=%d is behind the wall at z=%.0f", beatZ, rig.wallZ)
	}

	spawnX, spawnZ := int32(rig.map_.PlayerSpawn.X()), int32(rig.map_.PlayerSpawn.Z())
	if rig.blocks.IsSolidBlockAt(spawnX, groundLevel, spawnZ) {
		t.Errorf("player spawns inside a wall at %d,%d", spawnX, spawnZ)
	}
}

func TestShadowRigLightPointsDownAtTheBeat(t *testing.T) {
	rig := loadShadowRig(t)
	pos := rig.light.Position

	if pos.Y() <= 2 {
		t.Errorf("light at height %.1f is not above the caster", pos.Y())
	}
	if ceiling := float32(rig.blocks.Bounds().Y - 1); pos.Y() >= ceiling {
		t.Errorf("light at height %.1f is inside the ceiling at %.0f", pos.Y(), ceiling)
	}

	forward := rig.light.Forward()
	// Diagonally down: dropping, but not straight down, or the wall gets nothing.
	if forward.Y() >= -0.3 || forward.Y() <= -0.9 {
		t.Errorf("forward %v is not a diagonal, want y between -0.9 and -0.3", forward)
	}
	// Pointing at the wall side of the room, so the caster's shadow runs away from
	// the player and up the wall rather than towards the camera.
	if forward.Z() >= 0 {
		t.Errorf("forward %v points away from the wall", forward)
	}

	// The whole beat has to sit inside the cone, or half the patrol is unlit and
	// casts nothing.
	coneCosine := float32(math.Cos(float64(mgl32.DegToRad(rig.light.Angle / 2))))
	for _, end := range []mgl32.Vec3{rig.from, rig.to} {
		target := mgl32.Vec3{end.X(), 1.5, end.Z()}
		toTarget := target.Sub(pos)
		if got := forward.Dot(toTarget.Normalize()); got < coneCosine {
			t.Errorf("beat end %v is outside the cone: cos %.3f < %.3f", end, got, coneCosine)
		}
		if dist := toTarget.Len(); dist > rig.light.Reach {
			t.Errorf("beat end %v is %.1f away, past the light's reach %.1f", end, dist, rig.light.Reach)
		}
	}
}

// The invariant the whole rig hangs on, and the one the first version of it got
// wrong: the caster's shadow has to run across some floor AND then reach the wall.
// Too steep a light and the shadow ends on the floor short of the wall - and the
// caster, standing on it, hides it. Too shallow and there is no floor shadow left
// to judge the bias by.
func TestShadowReachesTheFloorAndThenTheWall(t *testing.T) {
	rig := loadShadowRig(t)
	pos := rig.light.Position

	const (
		floorY     = 1.0 // top of the floor voxels
		casterTopY = 2.0 // one unit tall, standing on the floor
	)
	casterZ := rig.from.Z()

	// Where the ray from the light over the caster's head would meet the floor if
	// the wall were not there.
	head := mgl32.Vec3{pos.X(), casterTopY, casterZ}
	ray := head.Sub(pos)
	toFloor := (floorY - pos.Y()) / ray.Y()
	floorHitZ := pos.Z() + toFloor*ray.Z()

	if floorHitZ >= casterZ {
		t.Fatalf("shadow falls at z=%.2f, on the light's side of the caster at z=%.2f", floorHitZ, casterZ)
	}
	if floorHitZ > rig.wallFaceZ {
		t.Errorf("shadow stops at z=%.2f, short of the wall face at z=%.1f: nothing lands on the wall",
			floorHitZ, rig.wallFaceZ)
	}
	if got := casterZ - rig.wallFaceZ; got < 1 {
		t.Errorf("only %.2f units of floor between caster and wall, too little floor shadow to see", got)
	}

	// And the part that does land on the wall has to land on it, not over the top.
	toWall := (rig.wallFaceZ - pos.Z()) / ray.Z()
	wallHitY := pos.Y() + toWall*ray.Y()
	// A sliver at the skirting board is not a wall shadow anyone can judge a bias
	// by, so this asks for a visible band and not merely for a positive number.
	if wallHitY < floorY+0.3 {
		t.Errorf("the shadow reaches only y=%.2f on the wall, less than 0.3 above the floor at %.1f",
			wallHitY, floorY)
	}
	if wallHitY >= rig.wallTopY {
		t.Errorf("the shadow clears the wall top at y=%.2f, wall ends at %.1f", wallHitY, rig.wallTopY)
	}
}
