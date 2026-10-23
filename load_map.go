package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads/voxel"
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

	FogStart, FogEnd float32
	AmbientColor     mgl32.Vec3
	AmbientStrength  float32

	Entities []core.EntitySpawn
	Lights   []core.MapLight
	Music    string

	SkyFullSphere bool
	Station       *core.Station
	Panes         []core.Pane
	Edited        bool `json:",omitempty"`
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
		PlayerSpawn:     m.PlayerSpawn,
		PlayerDir:       m.PlayerDir,
		CeilingColor:    toRGBA(m.CeilingColor),
		FloorColor:      toRGBA(m.FloorColor),
		FogStart:        m.FogStart,
		FogEnd:          m.FogEnd,
		AmbientColor:    m.AmbientColor,
		AmbientStrength: m.AmbientStrength,
		Entities:        m.Entities,
		Lights:          m.Lights,
		Music:           m.Music,
		SkyFullSphere:   m.SkyFullSphere,
		Station:         m.Station,
		Panes:           m.Panes,
		Edited:          m.Edited,
	}
}

func (f mapFile) toLoadResult() core.MapLoadResult {
	return core.MapLoadResult{
		PlayerSpawn:     f.PlayerSpawn,
		PlayerDir:       f.PlayerDir,
		CeilingColor:    f.CeilingColor,
		FloorColor:      f.FloorColor,
		FogStart:        f.FogStart,
		FogEnd:          f.FogEnd,
		AmbientColor:    f.AmbientColor,
		AmbientStrength: f.AmbientStrength,
		Entities:        f.Entities,
		Lights:          f.Lights,
		Music:           f.Music,
		SkyFullSphere:   f.SkyFullSphere,
		Station:         f.Station,
		Panes:           f.Panes,
		Edited:          f.Edited,
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

// mapDir is the folder of folders, one to a level: a map.json and the region
// files beside it. Which levels exist is decided by which folders are present, the
// same way the weapons and the scenery decide what exists - a new level is a
// folder, not a line of Go, which is the whole reason the test maps stopped
// being Go in the first place.
const mapDir = "assets/maps"

// defaultMapName is the level a run with no -map starts in.
const defaultMapName = "ark"

const mapFileName = "map.json"

// mapPathFor and voxelPathFor are the two halves of the level called name.
func mapPathFor(name string) string   { return filepath.Join(mapDir, name, mapFileName) }
func voxelPathFor(name string) string { return filepath.Join(mapDir, name) }

// voxelFileFor is where the blocks of the map at this path live: beside it, so
// the two halves of a level travel together and a map can never be opened
// against another one's geometry.
func voxelFileFor(mapPath string) string {
	return filepath.Dir(mapPath)
}

// mapNames is every level under mapDir, in a fixed order, for a menu and for
// the -map flag to be checked against. A folder with no map.json in it is not a
// level - a stray directory is not an error to stop the game with.
func mapNames() []string {
	entries, err := os.ReadDir(mapDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(mapPathFor(entry.Name())); err != nil {
			continue
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}

// loadMap reads the level called name: both halves, or an error naming the one
// that would not open. The blocks are allowed to be missing - that is an empty
// level, which is where building one starts - but the map file is not.
func loadMap(name string) (core.MapLoadResult, *voxel.Map, error) {
	result, err := loadJSONMap(mapPathFor(name))
	if err != nil {
		return core.MapLoadResult{}, nil, err
	}
	return result, loadVoxelFile(voxelPathFor(name)), nil
}

// loadVoxelFile reads a level's blocks. It is the only way geometry ever comes
// off disk, and a level whose voxel file is missing is a level with nothing in
// it - which is a real answer for a map being started from scratch, not an
// error to stop on.
func loadVoxelFile(path string) *voxel.Map {
	blocks := newLevel(defaultLevelSize)
	if !voxel.HasRegions(path) {
		return blocks
	}
	if err := blocks.LoadRegions(path); err != nil {
		panic(err)
	}
	return blocks
}

// defaultLevelSize is how big a level with no voxel file yet is: two chunks
// square and one tall, which is room enough to start building in.
var defaultLevelSize = voxel.Int3{X: 64, Y: 32, Z: 64}
