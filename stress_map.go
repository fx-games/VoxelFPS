package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
)

// StressConfig describes a generated map built to load the renderer, so the
// remaining performance items can be judged on numbers instead of argument. The
// shipped test map draws 816 triangles in 2-3 calls, which is not enough to make
// a difference visible either way (see the Measurement section in findings.md).
//
// Everything here is deterministic: same config, same map, same frame. A
// measurement that cannot be repeated is not a measurement.
type StressConfig struct {
	Size          int // map is Size x Size tiles
	PillarSpacing int // a one tile pillar every N tiles, 0 for none
	SpriteSpacing int // a barrel sprite every N tiles, 0 for none
	Lights        int // static spotlights, on top of the flashlight
}

// DefaultStressConfig is what -stress alone gives: 128x128 is 16 chunks against
// the test map's 2-3, and the barrel grid puts about a thousand sprites in the
// world, each of which is its own draw call today.
func DefaultStressConfig() StressConfig {
	return StressConfig{Size: 128, PillarSpacing: 6, SpriteSpacing: 4, Lights: 8}
}

// barrelTextureIndex is the barrel in the Blake Stone sprite sheet. The sheet is
// indexed directly everywhere else too, see ISSUES.md.
const barrelTextureIndex = 136

// loadStressMap builds an open hall of the given size with a grid of pillars and
// a grid of barrels. The pillars are there on purpose: a flat floor and four
// outer walls collapse into a handful of quads under greedy meshing, which would
// stress nothing.
func loadStressMap(cfg StressConfig) core.MapLoadResult {
	size := cfg.Size
	mapData := make([]core.TileInfo, size*size)

	for z := 0; z < size; z++ {
		for x := 0; x < size; x++ {
			tile := &mapData[z*size+x]
			tile.FloorTex = 2
			tile.CeilTex = 3
			// Alternating wall texture so the atlas lookup is not one constant.
			tile.WallTex = (x + z) % 2

			isBorder := x == 0 || z == 0 || x == size-1 || z == size-1
			isPillar := cfg.PillarSpacing > 0 && x%cfg.PillarSpacing == 0 && z%cfg.PillarSpacing == 0
			if isBorder || isPillar {
				tile.WallHeight = 1
				continue
			}

			// The offset is half the spacing, not a literal 2. With a literal 2 any
			// spacing of 1 or 2 asked for a residue the modulo can never produce, so
			// -stress-sprites 2 quietly built a map with no sprites at all - which is
			// exactly the setting someone reaches for to make sprites the bottleneck.
			// Half the spacing is 2 at the default of 4, so the default map is
			// unchanged.
			spriteOffset := cfg.SpriteSpacing / 2
			if cfg.SpriteSpacing > 0 && x%cfg.SpriteSpacing == spriteOffset && z%cfg.SpriteSpacing == spriteOffset {
				tile.Entities = append(tile.Entities, core.MapEntity{
					Name:           "StressBarrel",
					TextureIndices: []uint16{barrelTextureIndex},
					CreateCollider: true,
				})
			}
		}
	}

	// The centre tile is a pillar whenever the spacing happens to divide it, and
	// spawning inside a solid block drops the player through the floor.
	spawn := size / 2
	if cfg.PillarSpacing > 0 && spawn%cfg.PillarSpacing == 0 {
		spawn++
	}

	return core.MapLoadResult{
		TileSize:          64,
		AtlasFilename:     "_internal",
		PlayerSpawn:       mgl32.Vec3{float32(spawn) + 0.5, 1, float32(spawn) + 0.5},
		PlayerDir:         mgl32.Vec2{0, 1},
		CeilingHeight:     1,
		IsFloorTextured:   true,
		IsCeilingTextured: true,
		CeilingColor:      color.RGBA{80, 80, 80, 255},
		FloorColor:        color.RGBA{40, 40, 40, 255},
		MapDimensions:     [3]int{size, 3, size},
		MapData:           mapData,
		Lights:            stressLights(cfg),
	}
}

// stressLights spreads shadow casting spotlights over the map, pointing straight
// down. It is a performance rig: the lights are there to cost shadow passes, not to
// be looked at. The map to judge shadows by is the one behind -shadowtest.
func stressLights(cfg StressConfig) []core.MapLight {
	count := cfg.Lights
	if count > quads.MaxLights-1 { // the flashlight holds slot 0
		count = quads.MaxLights - 1
	}
	if count <= 0 {
		return nil
	}

	// A square grid of `perSide` lights, spread evenly, inset by half a cell so
	// none of them sits in the border wall.
	perSide := 1
	for perSide*perSide < count {
		perSide++
	}
	spacing := float32(cfg.Size) / float32(perSide)

	var lights []core.MapLight
	for row := 0; row < perSide && len(lights) < count; row++ {
		for column := 0; column < perSide && len(lights) < count; column++ {
			position := mgl32.Vec3{
				(float32(column) + 0.5) * spacing,
				2,
				(float32(row) + 0.5) * spacing,
			}
			lights = append(lights, core.MapLight{
				Position:  position,
				Pitch:     90, // straight down
				Color:     mgl32.Vec3{0.7, 0.7, 0.7},
				Intensity: 6,
				Angle:     45,
				Reach:     15,
			})
		}
	}
	return lights
}
