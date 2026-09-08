package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/dungen"
	"github.com/memmaker/goray/geometry"
	"image/color"
	"math/rand"
	"os"
)

func loadText(mapAsLines []string) core.MapLoadResult {
	// the string is a 2D array of characters
	// each character represents a tile
	// '.' and ' ' are empty tiles by default
	// '#' is a wall
	loadResult := core.MapLoadResult{}

	loadResult.CeilingHeight = 1
	loadResult.IsFloorTextured = true
	loadResult.IsCeilingTextured = true
	loadResult.CeilingColor = color.RGBA{80, 80, 80, 255}
	loadResult.FloorColor = color.RGBA{40, 40, 40, 255}
	mapSizeX := len(mapAsLines[0])
	mapSizeZ := len(mapAsLines)
	mapData := make([]core.TileInfo, mapSizeX*mapSizeZ)
	var playerSpawn mgl32.Vec3

	for y, line := range mapAsLines {
		for x, char := range line {
			mapIndex := y*len(line) + x
			mapData[mapIndex].FloorTex = 2
			mapData[mapIndex].CeilTex = 3
			mapData[mapIndex].WallTex = 0
			switch char {
			case '.':
				// empty space
			case ' ':
				// empty space
			case '#':
				mapData[mapIndex].WallHeight = 1
				if rand.Float64() < 0.1 {
					mapData[mapIndex].WallTex = 1
				}
			case 'P':
				playerSpawn = mgl32.Vec3{float32(x), 1, float32(y)}
			}
		}
	}

	loadResult.MapData = mapData
	loadResult.TileSize = 64
	loadResult.AtlasFilename = "_internal"
	loadResult.PlayerSpawn = playerSpawn
	loadResult.PlayerDir = mgl32.Vec2{0, 1}
	loadResult.MapDimensions = [3]int{mapSizeX, 3, mapSizeZ}

	return loadResult
}

func loadDungeonMap(generatedMap *dungen.DungeonMap) core.MapLoadResult {
	loadResult := core.MapLoadResult{}

	loadResult.CeilingHeight = 1
	loadResult.IsFloorTextured = true
	loadResult.IsCeilingTextured = true
	loadResult.CeilingColor = color.RGBA{80, 80, 80, 255}
	loadResult.FloorColor = color.RGBA{40, 40, 40, 255}
	mapSizeX, mapSizeZ := generatedMap.GetSize()
	mapData := make([]core.TileInfo, mapSizeX*mapSizeZ)
	var playerSpawn mgl32.Vec3

	for y := 0; y < mapSizeZ; y++ {
		for x := 0; x < mapSizeX; x++ {
			mapIndex := y*mapSizeX + x
			mapData[mapIndex].FloorTex = 2
			mapData[mapIndex].CeilTex = 3
			mapData[mapIndex].WallTex = 0
			if generatedMap.IsWallAt(geometry.Point{X: x, Y: y}) {
				mapData[mapIndex].WallHeight = 1
				if rand.Float64() < 0.1 {
					mapData[mapIndex].WallTex = 1
				}
			}
		}
	}

	first, _ := generatedMap.GetFirstAndLastRoom()
	if first == nil {
		spawnPos, _ := generatedMap.GetRandomFiltered(rand.New(rand.NewSource(0)), func(pos geometry.Point) bool {
			return !generatedMap.IsWallAt(pos)
		})
		playerSpawn = mgl32.Vec3{float32(spawnPos.X), 1, float32(spawnPos.Y)}
	} else {
		spawnPos := first.GetCenter()
		playerSpawn = mgl32.Vec3{float32(spawnPos.X), 1, float32(spawnPos.Y)}
	}

	loadResult.MapData = mapData
	loadResult.TileSize = 64
	loadResult.AtlasFilename = "_internal"
	loadResult.PlayerSpawn = playerSpawn
	loadResult.PlayerDir = mgl32.Vec2{0, 1}
	loadResult.MapDimensions = [3]int{mapSizeX, 3, mapSizeZ}

	return loadResult
}

// mapFile is core.MapLoadResult's on-disk shape: the same fields, but with a
// JSON-friendly concrete color instead of the color.Color interface. This is
// the entire map format - one struct, marshaled with encoding/json. No editor,
// no schema, no external tool: a level is a text file you can read and hand-edit.
type mapFile struct {
	TileSize          int
	AtlasFilename     string
	PlayerSpawn       mgl32.Vec3
	PlayerDir         mgl32.Vec2
	CeilingHeight     int
	IsFloorTextured   bool
	IsCeilingTextured bool
	CeilingColor      color.RGBA
	FloorColor        color.RGBA
	MapDimensions     [3]int
	MapData           []core.TileInfo
	Lights            []core.MapLight
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
		TileSize:          m.TileSize,
		AtlasFilename:     m.AtlasFilename,
		PlayerSpawn:       m.PlayerSpawn,
		PlayerDir:         m.PlayerDir,
		CeilingHeight:     m.CeilingHeight,
		IsFloorTextured:   m.IsFloorTextured,
		IsCeilingTextured: m.IsCeilingTextured,
		CeilingColor:      toRGBA(m.CeilingColor),
		FloorColor:        toRGBA(m.FloorColor),
		MapDimensions:     m.MapDimensions,
		MapData:           m.MapData,
		Lights:            m.Lights,
	}
}

func (f mapFile) toLoadResult() core.MapLoadResult {
	return core.MapLoadResult{
		TileSize:          f.TileSize,
		AtlasFilename:     f.AtlasFilename,
		PlayerSpawn:       f.PlayerSpawn,
		PlayerDir:         f.PlayerDir,
		CeilingHeight:     f.CeilingHeight,
		IsFloorTextured:   f.IsFloorTextured,
		IsCeilingTextured: f.IsCeilingTextured,
		CeilingColor:      f.CeilingColor,
		FloorColor:        f.FloorColor,
		MapDimensions:     f.MapDimensions,
		MapData:           f.MapData,
		Lights:            f.Lights,
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
