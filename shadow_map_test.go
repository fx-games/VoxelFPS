package main

import (
    "math"
    "testing"

    "github.com/go-gl/mathgl/mgl32"
)

// The map is only useful if the caster can actually walk its beat and the light
// actually reaches it. Both are easy to break by moving one constant.
func TestShadowTestMapHasAWallAndAClearBeat(t *testing.T) {
    result := loadShadowTestMap()
    size := shadowMapSize

    if len(result.MapData) != size*size {
        t.Fatalf("len(MapData) = %d, want %d", len(result.MapData), size*size)
    }

    for x := shadowWallMinX; x <= shadowWallMaxX; x++ {
        if result.MapData[shadowWallZ*size+x].WallHeight <= 0 {
            t.Fatalf("hole in the back wall at x=%d", x)
        }
    }
    // A wall that spans the room is a room divider, and the caster could not be
    // seen from the player's side of it.
    if result.MapData[shadowWallZ*size+1].WallHeight > 0 {
        t.Error("the back wall reaches the border, so it is not a free standing wall")
    }

    // The pathfinder walks tile to tile in a straight line here; a single solid
    // tile on the beat makes it path around, or fail, and the shadow stops moving.
    for x := shadowPatrolMinX; x <= shadowPatrolMaxX; x++ {
        if result.MapData[shadowPatrolZ*size+x].WallHeight > 0 {
            t.Fatalf("the beat is blocked at x=%d", x)
        }
    }
    // The beat has to be clear of the wall, or the caster stands in it.
    if shadowPatrolZ <= shadowWallZ {
        t.Fatalf("the beat at z=%d is behind the wall at z=%d", shadowPatrolZ, shadowWallZ)
    }

    spawn := result.MapData[shadowPatrolZ*size+shadowPatrolMinX]
    if len(spawn.Entities) != 1 {
        t.Fatalf("%d entities at the spawn tile, want exactly the one caster", len(spawn.Entities))
    }
    caster := spawn.Entities[0]
    // spawnEntity keys the pathing component off this name.
    if caster.Name != "Scientist" {
        t.Errorf("caster is %q, which gets no pathing component and will not move", caster.Name)
    }
    if caster.PatrolTo == nil {
        t.Fatal("caster has no patrol destination, so it stands still")
    }
    if got := int(caster.PatrolTo.Z()); got != shadowPatrolZ {
        t.Errorf("patrol ends at z=%d, want the beat at z=%d", got, shadowPatrolZ)
    }

    spawnX, spawnZ := int(result.PlayerSpawn.X()), int(result.PlayerSpawn.Z())
    if result.MapData[spawnZ*size+spawnX].WallHeight > 0 {
        t.Errorf("player spawns inside a wall at %d,%d", spawnX, spawnZ)
    }
}

func TestShadowTestLightPointsDownAtTheBeat(t *testing.T) {
    light := shadowTestLight()

    pos := light.Position
    if pos.Y() <= 2 {
        t.Errorf("light at height %.1f is not above the caster", pos.Y())
    }
    if pos.Y() >= float32(shadowCeiling) {
        t.Errorf("light at height %.1f is inside the ceiling at %d", pos.Y(), shadowCeiling)
    }

    forward := light.Forward()
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
    coneCosine := float32(math.Cos(float64(mgl32.DegToRad(light.Angle / 2))))
    for _, x := range []int{shadowPatrolMinX, shadowPatrolMaxX} {
        target := mgl32.Vec3{float32(x) + 0.5, 1.5, float32(shadowPatrolZ) + 0.5}
        toTarget := target.Sub(pos)
        if got := forward.Dot(toTarget.Normalize()); got < coneCosine {
            t.Errorf("beat end x=%d is outside the cone: cos %.3f < %.3f", x, got, coneCosine)
        }
        if dist := toTarget.Len(); dist > light.Reach {
            t.Errorf("beat end x=%d is %.1f away, past the light's reach %.1f", x, dist, light.Reach)
        }
    }
}

// The invariant the whole map hangs on, and the one the first version of it got
// wrong: the caster's shadow has to run across some floor AND then reach the wall.
// Too steep a light and the shadow ends on the floor short of the wall - and the
// caster, standing on it, hides it. Too shallow and there is no floor shadow left
// to judge the bias by.
func TestShadowReachesTheFloorAndThenTheWall(t *testing.T) {
    light := shadowTestLight()
    pos := light.Position

    const (
        floorY      = 1.0                        // top of the floor voxels
        casterTopY  = 2.0                        // one unit tall, standing on the floor
        wallFaceZ   = float32(shadowWallZ) + 1   // the wall tile spans z 6..7, so its face is 7
        wallTopY    = 1 + float32(shadowWallHeigh)
    )
    casterZ := float32(shadowPatrolZ) + 0.5

    // Where the ray from the light over the caster's head would meet the floor if
    // the wall were not there.
    head := mgl32.Vec3{pos.X(), casterTopY, casterZ}
    ray := head.Sub(pos)
    toFloor := (floorY - pos.Y()) / ray.Y()
    floorHitZ := pos.Z() + toFloor*ray.Z()

    if floorHitZ >= casterZ {
        t.Fatalf("shadow falls at z=%.2f, on the light's side of the caster at z=%.2f", floorHitZ, casterZ)
    }
    if floorHitZ > wallFaceZ {
        t.Errorf("shadow stops at z=%.2f, short of the wall face at z=%.1f: nothing lands on the wall",
            floorHitZ, wallFaceZ)
    }
    if got := casterZ - wallFaceZ; got < 1 {
        t.Errorf("only %.2f units of floor between caster and wall, too little floor shadow to see", got)
    }

    // And the part that does land on the wall has to land on it, not over the top.
    toWall := (wallFaceZ - pos.Z()) / ray.Z()
    wallHitY := pos.Y() + toWall*ray.Y()
    // A sliver at the skirting board is not a wall shadow anyone can judge a bias
    // by, so this asks for a visible band and not merely for a positive number.
    if wallHitY < floorY+0.3 {
        t.Errorf("the shadow reaches only y=%.2f on the wall, less than 0.3 above the floor at %.1f",
            wallHitY, floorY)
    }
    if wallHitY >= wallTopY {
        t.Errorf("the shadow clears the wall top at y=%.2f, wall ends at %.1f", wallHitY, wallTopY)
    }
}
