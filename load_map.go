package main

import (
	hex2 "encoding/hex"
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/dungen"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/ldtk"
	"image/color"
	"io"
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

// loadLDTK reads a map from an LDtk project file. It returns an error rather
// than panicking because the file watcher re-reads this file every time LDtk
// saves it: a half-written or malformed save used to take the running engine
// down with it, losing whatever was not saved on the Go side.
func loadLDTK(filename string) (core.MapLoadResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return core.MapLoadResult{}, fmt.Errorf("open %s: %w", filename, err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return core.MapLoadResult{}, fmt.Errorf("read %s: %w", filename, err)
	}
	ldtkProject, err := ldtk.UnmarshalLdtkJSON(data)
	if err != nil {
		return core.MapLoadResult{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	loadResult := core.MapLoadResult{}

	tileSet := ldtkProject.Defs.Tilesets[0]
	tileSetFilename := tileSet.RelPath
	tileSize := int(tileSet.TileGridSize)
	atlasGridWidth := tileSet.CWid

	level := ldtkProject.Levels[0]

	for _, levelDataField := range level.FieldInstances {
		switch levelDataField.Identifier {
		case "CeilingHeight":
			if ceilingHeight, isOk := levelDataField.Value.(float64); isOk {
				loadResult.CeilingHeight = int(ceilingHeight)
			}
		case "IsFloorTextured":
			if isFloorTextured, isOk := levelDataField.Value.(bool); isOk {
				loadResult.IsFloorTextured = isFloorTextured
			}
		case "IsCeilingTextured":
			if isCeilingTextured, isOk := levelDataField.Value.(bool); isOk {
				loadResult.IsCeilingTextured = isCeilingTextured
			}
		case "CeilingColor":
			if colorHex, isOk := levelDataField.Value.(string); isOk {
				loadResult.CeilingColor = colorFromHex(colorHex)
			}
		case "FloorColor":
			if colorHex, isOk := levelDataField.Value.(string); isOk {
				loadResult.FloorColor = colorFromHex(colorHex)
			}
		}
	}

	layerOne := level.LayerInstances[0]
	mapSizeX := int(layerOne.CWid)
	mapSizeZ := int(layerOne.CHei)
	var spawnPos mgl32.Vec3
	var spawnDir mgl32.Vec2

	mapData := make([]core.TileInfo, mapSizeX*mapSizeZ)

	atlasIndexFromField := func(field ldtk.FieldInstance) int {
		if field.Value == nil {
			return -1
		}
		rectObj, ok := field.Value.(map[string]interface{})
		if !ok {
			return -1
		}
		pixX := int64(rectObj["x"].(float64))
		pixY := int64(rectObj["y"].(float64))
		gridX := pixX / int64(tileSet.TileGridSize)
		gridY := pixY / int64(tileSet.TileGridSize)
		atlasIndex := int64(atlasGridWidth)*gridY + gridX
		return int(atlasIndex)
	}

	for _, layer := range level.LayerInstances {
		layerId := layer.Identifier
		layerType := ldtk.Type(layer.Type)
		switch layerType {
		case ldtk.Entities:
			for _, entity := range layer.EntityInstances {
				gridPos := entity.Grid
				entityId := entity.Identifier
				switch entityId {
				case "PlayerSpawn":
					spawnPos = mgl32.Vec3{float32(gridPos[0]), 1, float32(gridPos[1])}
					if lookDirName, isOk := entity.FieldInstances[0].Value.(string); isOk {
						switch lookDirName {
						case "North":
							spawnDir = mgl32.Vec2{0, 1}
						case "South":
							spawnDir = mgl32.Vec2{0, -1}
						case "East":
							spawnDir = mgl32.Vec2{1, 0}
						case "West":
							spawnDir = mgl32.Vec2{-1, 0}
						}
					}
				case "Light":
					// A light is not a tile entity: it does not sit in mapData, it
					// goes on the load result, and the renderer replaces the scene's
					// lights with the whole list. Grid position plus half a tile puts
					// it in the middle of the tile it was dropped on, and Height is
					// above the map floor.
					light := core.MapLight{
						Position: mgl32.Vec3{float32(gridPos[0]) + 0.5, 2, float32(gridPos[1]) + 0.5},
						Color:    mgl32.Vec3{1, 1, 1},
					}
					for _, field := range entity.FieldInstances {
						value, isNumber := field.Value.(float64)
						switch field.Identifier {
						case "Height":
							if isNumber {
								light.Position[1] = float32(value)
							}
						case "Yaw":
							if isNumber {
								light.Yaw = float32(value)
							}
						case "Pitch":
							if isNumber {
								light.Pitch = float32(value)
							}
						case "Intensity":
							if isNumber {
								light.Intensity = float32(value)
							}
						case "Angle":
							if isNumber {
								light.Angle = float32(value)
							}
						case "Reach":
							if isNumber {
								light.Reach = float32(value)
							}
						case "Color":
							if hex, isString := field.Value.(string); isString {
								light.Color = colorVecFromHex(hex)
							}
						}
					}
					loadResult.Lights = append(loadResult.Lights, light)
				case "Mod":
					mapIndex := int(gridPos[1])*int(mapSizeX) + int(gridPos[0])
					for _, field := range entity.FieldInstances {
						switch field.Identifier {
						case "WallHeight":
							mapData[mapIndex].WallHeight = int(field.Value.(float64))
						case "WallOffset":
							if field.Value != nil {
								mapData[mapIndex].WallOffset = int(field.Value.(float64))
							}
						case "CeilingHeight":
							if field.Value != nil {
								mapData[mapIndex].CeilingHeight = int(field.Value.(float64))
							}
						case "FloorOffset":
							if field.Value != nil {
								mapData[mapIndex].FloorOffset = int(field.Value.(float64))
							}
						case "WallTex":
							if field.Value != nil {
								mapData[mapIndex].WallTex = atlasIndexFromField(field)
							}
						}
					}

				default:
					mapEntity := core.MapEntity{
						Name:           entityId,
						CreateCollider: true,
					}
					for _, field := range entity.FieldInstances {
						switch field.Identifier {
						case "TextureIndex":
							texIndex := atlasIndexFromField(field)
							mapEntity.TextureIndices = []uint16{uint16(texIndex)}
						case "TextureIndices":
							texIndices := field.Value.([]interface{})
							for _, texIndex := range texIndices {
								mapEntity.TextureIndices = append(mapEntity.TextureIndices, uint16(texIndex.(float64)))
							}
						case "MeshName":
							mapEntity.MeshName = field.Value.(string)
						case "SpriteSheetName":
							mapEntity.SpriteSheetName = field.Value.(string)
						case "SpriteSheetTileSize":
							spriteSize := field.Value.([]interface{})
							mapEntity.SpriteSheetTileSize = geometry.Point{X: int(spriteSize[0].(float64)), Y: int(spriteSize[1].(float64))}
						}
					}
					spawnX := gridPos[0]
					spawnY := gridPos[1]
					mapIndex := int(spawnY)*mapSizeX + int(spawnX)

					mapData[mapIndex].Entities = append(mapData[mapIndex].Entities, mapEntity)
				}
			}
		case ldtk.Tiles:
			for _, tile := range layer.GridTiles {
				textureIndex := tile.T
				tileIndex := tile.D[0]
				switch layerId {
				case "Walls":
					mapData[tileIndex].WallTex = int(textureIndex)
					mapData[tileIndex].WallHeight = max(1, mapData[tileIndex].WallHeight)
				case "Floors":
					mapData[tileIndex].FloorTex = int(textureIndex)
				case "Ceilings":
					mapData[tileIndex].CeilTex = int(textureIndex)
				}
			}
		}
	}
	mapSizeY := loadResult.CeilingHeight + 2

	//voxelMap.ChangeMaxChunkHeightForDraw(64)
	//voxelMap.ForceUpload()

	loadResult.MapData = mapData
	loadResult.TileSize = tileSize
	loadResult.AtlasFilename = *tileSetFilename
	loadResult.PlayerSpawn = spawnPos
	loadResult.PlayerDir = spawnDir
	loadResult.MapDimensions = [3]int{mapSizeX, mapSizeY, mapSizeZ}

	return loadResult, nil
}

// colorVecFromHex is the linear-ish 0..1 form of an authored "#RRGGBB", which is what
// a light wants; colorFromHex below returns the color.Color the tile layers want.
func colorVecFromHex(hex string) mgl32.Vec3 {
	rgba, isRGBA := colorFromHex(hex).(color.RGBA)
	if !isRGBA {
		return mgl32.Vec3{1, 1, 1}
	}
	return mgl32.Vec3{float32(rgba.R) / 255, float32(rgba.G) / 255, float32(rgba.B) / 255}
}

func colorFromHex(hex string) color.Color {
	bytes, err := hex2.DecodeString(hex[1:])
	if err != nil {
		return color.RGBA{}
	}
	return color.RGBA{R: bytes[0], G: bytes[1], B: bytes[2], A: 255}
}
