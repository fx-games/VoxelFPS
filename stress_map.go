package main

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// StressConfig describes a generated map built to load the renderer, so the
// remaining performance items can be judged on numbers instead of argument. The
// shipped test map draws 816 triangles in 2-3 calls, which is not enough to make
// a difference visible either way (see the Measurement section in README.md).
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
func loadStressMap(cfg StressConfig) (core.MapLoadResult, *voxel.Map) {
	size := int32(cfg.Size)
	// Floor at 0, walls at 1, ceiling at 2 - the shape every one of these maps
	// is, only wider.
	blocks := newLevel(voxel.Int3{X: size, Y: 3, Z: size})
	paintRoom(blocks, roomBlock(0, 2, 3))

	var entities []core.EntitySpawn
	for z := int32(1); z < size-1; z++ {
		for x := int32(1); x < size-1; x++ {
			// The pillars are there on purpose, and so is the alternating wall
			// texture: one constant texture index would not exercise the atlas
			// lookup any more than a flat floor would exercise the mesher.
			if cfg.PillarSpacing > 0 && x%int32(cfg.PillarSpacing) == 0 && z%int32(cfg.PillarSpacing) == 0 {
				pillar := voxel.Int3{X: x, Y: groundLevel, Z: z}
				fillBlocks(blocks, pillar, pillar, roomBlock(uint16((x+z)%2), 2, 3))
				continue
			}

			// The offset is half the spacing, not a literal 2. With a literal 2 any
			// spacing of 1 or 2 asked for a residue the modulo can never produce, so
			// -stress-sprites 2 quietly built a map with no sprites at all - which is
			// exactly the setting someone reaches for to make sprites the bottleneck.
			// Half the spacing is 2 at the default of 4, so the default map is
			// unchanged.
			spriteOffset := int32(cfg.SpriteSpacing / 2)
			if cfg.SpriteSpacing > 0 && x%int32(cfg.SpriteSpacing) == spriteOffset && z%int32(cfg.SpriteSpacing) == spriteOffset {
				entities = append(entities, core.EntitySpawn{
					Position: tileCentre(int(x), int(z), groundLevel),
					Entity: core.MapEntity{
						Name:           "StressBarrel",
						TextureIndices: []uint16{barrelTextureIndex},
						CreateCollider: true,
					},
				})
			}
		}
	}

	// The centre tile is a pillar whenever the spacing happens to divide it, and
	// spawning inside a solid block drops the player through the floor.
	spawn := cfg.Size / 2
	if cfg.PillarSpacing > 0 && spawn%cfg.PillarSpacing == 0 {
		spawn++
	}

	return core.MapLoadResult{
		PlayerSpawn:  mgl32.Vec3{float32(spawn) + 0.5, 1, float32(spawn) + 0.5},
		PlayerDir:    mgl32.Vec2{0, 1},
		CeilingColor: color.RGBA{80, 80, 80, 255},
		FloorColor:   color.RGBA{40, 40, 40, 255},
		Entities:     entities,
		Lights:       stressLights(cfg),
	}, blocks
}

// stressLights spreads shadow casting spotlights over the map, pointing straight
// down. It is a performance rig: the lights are there to cost shadow passes, not to
// be looked at. The map to judge shadows by is the one behind -shadowtest.
func stressLights(cfg StressConfig) []core.MapLight {
	count := cfg.Lights
	// Capped by the shadow atlas, not by MaxLights: these exist to cost shadow
	// passes, and a light past the last layer would cost none. The player's light
	// holds the first layer.
	if count > quads.MaxShadowLayers-1 {
		count = quads.MaxShadowLayers - 1
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
