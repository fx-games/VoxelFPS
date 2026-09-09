package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

// mapFile is core.MapLoadResult's on-disk shape: the same fields, but with a
// JSON-friendly concrete color instead of the color.Color interface. Half the
// map format - one struct, marshaled with encoding/json, hand-editable.
//
// The other half is the level's blocks, in the voxel file beside it (see
// voxelFileFor). They are split because they are different kinds of thing, not
// because they are two descriptions of the same one: this says where the player
// starts and what stands in the level, the voxel file says what the level is
// made of, and nothing is described twice.
type mapFile struct {
	PlayerSpawn  mgl32.Vec3
	PlayerDir    mgl32.Vec2
	CeilingColor color.RGBA
	FloorColor   color.RGBA
	Entities     []core.EntitySpawn
	Lights       []core.MapLight
}

func toMapFile(m core.MapLoadResult) mapFile {
	toRGBA := func(c color.Color) color.RGBA {
		if c == nil {
			return color.RGBA{}
		}
		r, g, b, a := c.RGBA()
		return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	return mapFile{
		PlayerSpawn:  m.PlayerSpawn,
		PlayerDir:    m.PlayerDir,
		CeilingColor: toRGBA(m.CeilingColor),
		FloorColor:   toRGBA(m.FloorColor),
		Entities:     m.Entities,
		Lights:       m.Lights,
	}
}

func (f mapFile) toLoadResult() core.MapLoadResult {
	return core.MapLoadResult{
		PlayerSpawn:  f.PlayerSpawn,
		PlayerDir:    f.PlayerDir,
		CeilingColor: f.CeilingColor,
		FloorColor:   f.FloorColor,
		Entities:     f.Entities,
		Lights:       f.Lights,
	}
}

// saveJSONMap writes a map in goray's own format: see mapFile.
func saveJSONMap(filename string, m core.MapLoadResult) error {
	data, err := json.MarshalIndent(toMapFile(m), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

// loadJSONMap reads a map in goray's own format. It returns an error rather
// than panicking because the file watcher re-reads this file on every save: a
// half-written save used to take the running engine down with it, losing
// whatever was not saved on the Go side.
func loadJSONMap(filename string) (core.MapLoadResult, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return core.MapLoadResult{}, fmt.Errorf("open %s: %w", filename, err)
	}
	var f mapFile
	if err := json.Unmarshal(data, &f); err != nil {
		return core.MapLoadResult{}, fmt.Errorf("parse %s: %w", filename, err)
	}
	return f.toLoadResult(), nil
}

// voxelFileFor is where a map's blocks live: beside the map file and named
// after it, so the two halves of a level travel together and a map can never be
// opened against another one's geometry.
func voxelFileFor(mapPath string) string {
	return strings.TrimSuffix(mapPath, filepath.Ext(mapPath)) + ".bin"
}

// loadVoxelFile reads a level's blocks. It is the only way geometry ever comes
// off disk, and a level whose voxel file is missing is a level with nothing in
// it - which is a real answer for a map being started from scratch, not an
// error to stop on.
func loadVoxelFile(path string) *voxel.Map {
	blocks := newLevel(defaultLevelSize)
	if _, err := os.Stat(path); err != nil {
		return blocks
	}
	blocks.LoadFromDisk(path)
	return blocks
}

// defaultLevelSize is how big a level with no voxel file yet is: two chunks
// square and one tall, which is room enough to start building in.
var defaultLevelSize = voxel.Int3{X: 64, Y: 32, Z: 64}
