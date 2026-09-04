package main

import "testing"

func TestStressMapIsWalledPillaredAndPopulated(t *testing.T) {
    cfg := DefaultStressConfig()
    result := loadStressMap(cfg)

    size := cfg.Size
    if got := result.MapDimensions; got != [3]int{size, 3, size} {
        t.Errorf("dimensions = %v, want {%d 3 %d}", got, size, size)
    }
    if len(result.MapData) != size*size {
        t.Fatalf("len(MapData) = %d, want %d", len(result.MapData), size*size)
    }

    sprites := 0
    for z := 0; z < size; z++ {
        for x := 0; x < size; x++ {
            tile := result.MapData[z*size+x]
            solid := tile.WallHeight > 0
            onBorder := x == 0 || z == 0 || x == size-1 || z == size-1
            onPillar := x%cfg.PillarSpacing == 0 && z%cfg.PillarSpacing == 0

            if onBorder && !solid {
                t.Fatalf("hole in the border wall at %d,%d", x, z)
            }
            if solid != (onBorder || onPillar) {
                t.Fatalf("solid = %v at %d,%d, want %v", solid, x, z, onBorder || onPillar)
            }
            if solid && len(tile.Entities) > 0 {
                t.Fatalf("sprite inside a wall at %d,%d", x, z)
            }
            sprites += len(tile.Entities)
        }
    }

    // The point of the map is that there is a lot to draw.
    if sprites < 500 {
        t.Errorf("only %d sprites, that is not a stress map", sprites)
    }

    // Spawning inside a pillar drops the player through the floor.
    spawnX, spawnZ := int(result.PlayerSpawn.X()), int(result.PlayerSpawn.Z())
    if result.MapData[spawnZ*size+spawnX].WallHeight > 0 {
        t.Errorf("player spawns inside a wall at %d,%d", spawnX, spawnZ)
    }
}

// Same config, same map: a measurement you cannot repeat is not a measurement.
func TestStressMapIsDeterministic(t *testing.T) {
    first := loadStressMap(DefaultStressConfig())
    second := loadStressMap(DefaultStressConfig())

    for i := range first.MapData {
        if first.MapData[i].WallHeight != second.MapData[i].WallHeight ||
            first.MapData[i].WallTex != second.MapData[i].WallTex ||
            len(first.MapData[i].Entities) != len(second.MapData[i].Entities) {
            t.Fatalf("tile %d differs between two generations", i)
        }
    }
}

func TestStressLightsStayInsideTheMapAndUnderTheCap(t *testing.T) {
    cfg := DefaultStressConfig()
    lights := stressLights(cfg)

    if len(lights) != cfg.Lights {
        t.Fatalf("generated %d lights, want %d", len(lights), cfg.Lights)
    }
    for _, light := range lights {
        pos := light.Position
        if pos.X() <= 0 || pos.X() >= float32(cfg.Size) || pos.Z() <= 0 || pos.Z() >= float32(cfg.Size) {
            t.Errorf("light at %v is outside the map", pos)
        }
    }

    // Slot 0 is the flashlight, so the static lights have one less to fill.
    cfg.Lights = 999
    if got := len(stressLights(cfg)); got > 15 {
        t.Errorf("generated %d lights, want at most 15", got)
    }
}
