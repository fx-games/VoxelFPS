package main

import (
	"testing"

	"github.com/memmaker/voxelfps/quads"
)

func TestStressMapIsWalledPillaredAndPopulated(t *testing.T) {
	cfg := DefaultStressConfig()
	result, blocks := loadStressMap(cfg)

	size := int32(cfg.Size)
	if got := blocks.Bounds(); got.X != size || got.Y != 3 || got.Z != size {
		t.Errorf("bounds = %v, want {%d 3 %d}", got, size, size)
	}

	for z := int32(0); z < size; z++ {
		for x := int32(0); x < size; x++ {
			// Everything is judged at the one height a wall occupies: floor
			// below it and ceiling above it are solid everywhere by design.
			solid := blocks.IsSolidBlockAt(x, groundLevel, z)
			onBorder := x == 0 || z == 0 || x == size-1 || z == size-1
			onPillar := x%int32(cfg.PillarSpacing) == 0 && z%int32(cfg.PillarSpacing) == 0

			if onBorder && !solid {
				t.Fatalf("hole in the border wall at %d,%d", x, z)
			}
			if solid != (onBorder || onPillar) {
				t.Fatalf("solid = %v at %d,%d, want %v", solid, x, z, onBorder || onPillar)
			}
		}
	}

	for _, spawn := range result.Entities {
		x, z := int32(spawn.Position.X()), int32(spawn.Position.Z())
		if blocks.IsSolidBlockAt(x, groundLevel, z) {
			t.Fatalf("sprite inside a wall at %d,%d", x, z)
		}
	}

	// The point of the map is that there is a lot to draw.
	if len(result.Entities) < 500 {
		t.Errorf("only %d sprites, that is not a stress map", len(result.Entities))
	}

	// Spawning inside a pillar drops the player through the floor.
	spawnX, spawnZ := int32(result.PlayerSpawn.X()), int32(result.PlayerSpawn.Z())
	if blocks.IsSolidBlockAt(spawnX, groundLevel, spawnZ) {
		t.Errorf("player spawns inside a wall at %d,%d", spawnX, spawnZ)
	}
}

// A spacing the modulo cannot satisfy builds an empty map, and the flag that does it
// is the one anybody reaches for to make sprites the bottleneck.
func TestTightSpriteSpacingStillPlacesSprites(t *testing.T) {
	for _, spacing := range []int{1, 2, 3, 4, 8} {
		cfg := StressConfig{Size: 32, PillarSpacing: 0, SpriteSpacing: spacing}
		result, _ := loadStressMap(cfg)
		if len(result.Entities) == 0 {
			t.Errorf("-stress-sprites %d placed no sprites at all", spacing)
		}
	}
}

// Same config, same map: a measurement you cannot repeat is not a measurement.
func TestStressMapIsDeterministic(t *testing.T) {
	first, firstBlocks := loadStressMap(DefaultStressConfig())
	second, secondBlocks := loadStressMap(DefaultStressConfig())

	size := int32(DefaultStressConfig().Size)
	for z := int32(0); z < size; z++ {
		for x := int32(0); x < size; x++ {
			for y := int32(0); y < 3; y++ {
				if firstBlocks.IsSolidBlockAt(x, y, z) != secondBlocks.IsSolidBlockAt(x, y, z) {
					t.Fatalf("block %d,%d,%d differs between two generations", x, y, z)
				}
			}
		}
	}
	if len(first.Entities) != len(second.Entities) {
		t.Fatalf("%d entities one time and %d the next", len(first.Entities), len(second.Entities))
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

	// Slot 0 is the player's light and it holds the first shadow layer, so the
	// static lights have one less to fill.
	cfg.Lights = 999
	if got := len(stressLights(cfg)); got > quads.MaxShadowLayers-1 {
		t.Errorf("generated %d lights, want at most 15", got)
	}
}
