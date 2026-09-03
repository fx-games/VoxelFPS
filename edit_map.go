package main

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/geometry"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/voxel"
    "path/filepath"
)

func (g *Game) openVoxelBrushMenu() {
    if g.voxelBlock == nil {
        return
    }
    previousMouseState := g.setupModal()
    configMenu := g.OpenConfigMenu()

    options := []quads.GuiOption{
        quads.NewGuiOptionAction("Pen", func() {
            g.captureMouse(previousMouseState)
            g.voxelBrush = &VoxelBrushDrag{}

            if g.voxelSelectionAction == nil {
                g.voxelSelectionAction = g.voxelPlaceBlock
                g.voxelSelectionEmpty = true
            }
            g.Toast("Pen Brush Selected")
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Line", func() {
            g.captureMouse(previousMouseState)
            g.voxelBrush = &VoxelBrushLine{}

            if g.voxelSelectionAction == nil {
                g.voxelSelectionAction = g.voxelPlaceBlock
                g.voxelSelectionEmpty = true
            }

            g.Toast("Line Brush Selected")
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Rect (outline)", func() {
            g.captureMouse(previousMouseState)
            g.voxelBrush = &VoxelBrushRectangle{fill: false}

            if g.voxelSelectionAction == nil {
                g.voxelSelectionAction = g.voxelPlaceBlock
                g.voxelSelectionEmpty = true
            }

            g.Toast("Line Brush Selected")
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Rect (filled)", func() {
            g.captureMouse(previousMouseState)
            g.voxelBrush = &VoxelBrushRectangle{fill: true}

            if g.voxelSelectionAction == nil {
                g.voxelSelectionAction = g.voxelPlaceBlock
                g.voxelSelectionEmpty = true
            }

            g.Toast("Line Brush Selected")
            g.popFromWidgetStack()
        }),
    }
    configMenu.SetOptions(options)
    configMenu.HandleResize(g.WindowWidth, g.WindowHeight, g.UIScale)

    g.pushToWidgetStack(configMenu)
}

func (g *Game) openVoxelEditMenu() {
    if g.voxelBlock == nil {
        return
    }
    previousMouseState := g.setupModal()
    configMenu := g.OpenConfigMenu()

    options := []quads.GuiOption{
        quads.NewGuiOptionAction("Choose Brush", g.openVoxelBrushMenu),
        quads.NewGuiOptionAction("Block Textures", g.openCubeTextureMapper),
        quads.NewGuiOptionAction("Paint Gun", func() {
            g.setWeaponEffect(WeaponEffectHitscanVoxelPaintGun)
        }),
        quads.NewGuiOptionAction("Fill Gun", func() {
            g.setWeaponEffect(WeaponEffectHitscanVoxelPaintGunFill)
        }),
        quads.NewGuiOptionAction("Choose Paint Texture", func() {
            g.pickTexture(g.AtlasMap, func(selectedIndex int) {
                g.currentPaintGunTextureIndex = selectedIndex
            })
        }),
        quads.NewGuiOptionAction("Choose Decoration Texture", g.changeDecorationTexture),
        quads.NewGuiOptionAction("Add Blocks", func() {
            g.captureMouse(previousMouseState)
            if g.voxelBrush == nil {
                g.voxelBrush = &VoxelBrushDrag{}
            }

            g.voxelSelectionAction = g.voxelPlaceBlock
            g.voxelSelectionEmpty = true
            g.Toast("Setting blocks")
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Remove Blocks", func() {
            g.captureMouse(previousMouseState)
            if g.voxelBrush == nil {
                g.voxelBrush = &VoxelBrushDrag{}
            }
            g.voxelSelectionAction = g.voxelRemoveBlock
            g.voxelSelectionEmpty = false
            g.Toast("Removing blocks")
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Save Map", func() {
            saveErr := g.CubeMap.SaveToDisk(filepath.Join("assets", "map.bin"))
            if saveErr != nil {
                g.Toast(saveErr.Error())
            } else {
                g.Toast("Map saved")
            }
            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Load Map", func() {
            g.CubeMap.LoadFromDisk(filepath.Join("assets", "map.bin"))

            g.Toast("Map loaded")

            g.popFromWidgetStack()
        }),
        quads.NewGuiOptionAction("Clear Map", func() {
            g.CubeMap.ClearAllChunks()
            g.CubeMap.SetFloorAtHeight(0, g.voxelBlock)
            g.popFromWidgetStack()
        }),
    }
    configMenu.SetOptions(options)
    configMenu.HandleResize(g.WindowWidth, g.WindowHeight, g.UIScale)

    g.replaceWidgetStack(configMenu)
}

func (g *Game) changeDecorationTexture() {
    g.pickTexture(g.AtlasSprites, func(selectedIndex int) {
        if g.gameObjectForSpawning != nil {
            g.gameObjectForSpawning.Sprite().TextureIndices = []uint16{uint16(selectedIndex)}
        }
    })
}
func (g *Game) testPackedAtlasPicker() {
    weapon, isDoomWeapon := g.currentWeapon().(*DoomWeapon)
    if !isDoomWeapon {
        g.Toast("No packed atlas to pick from")
        return
    }
    g.pickTexture(weapon.Atlas, func(selectedIndex int) {
        println("Selected index", selectedIndex)
    })
}

func (g *Game) toggleSpawnMode() {
    if g.gameObjectForSpawning != nil {
        g.gameObjectForSpawning.Kill()
        g.gameObjectForSpawning = nil
        return
    }
    if g.lastHitResult == nil {
        return
    }
    spawnPos, hitGround := g.lastHitResult.GroundPosition()
    if !hitGround {
        return
    }
    entityDesc := core.MapEntity{
        Name:           "Barrel",
        TextureIndices: []uint16{136},
        CreateCollider: false,
        IsTransparent:  true,
    }
    g.gameObjectForSpawning = g.spawnEntity(spawnPos, entityDesc)
    g.gameObjectForSpawning.Sprite().Tint = mgl32.Vec4{1, 1, 1, 0.5}
}

func (g *Game) startVoxelEditing() {
    g.captureMouse(false)
    g.voxelBrush = &VoxelBrushRectangle{fill: false}
    g.voxelBlock = voxel.NewBlock(2)
    g.voxelSelectionAction = g.voxelPlaceBlock
    g.voxelSelectionEmpty = true
}

func (g *Game) voxelPlaceBlock(pos []voxel.Int3) {
    g.CubeMap.SetBlocks(pos, g.voxelBlock.Copy())
}

func (g *Game) voxelRemoveBlock(pos []voxel.Int3) {
    g.CubeMap.SetAirBlocks(pos)
}

type VoxelBrush interface {
    StartDrag(pos voxel.Int3) []voxel.Int3
    Drag(cur voxel.Int3) []voxel.Int3
}

type VoxelBrushDrag struct {
    draggedOver []voxel.Int3
}

func (v *VoxelBrushDrag) StartDrag(pos voxel.Int3) []voxel.Int3 {
    v.draggedOver = []voxel.Int3{pos}
    return []voxel.Int3{pos}
}

func (v *VoxelBrushDrag) Drag(cur voxel.Int3) []voxel.Int3 {
    v.draggedOver = append(v.draggedOver, cur)
    return v.draggedOver
}

type VoxelBrushLine struct {
    start voxel.Int3
}

func (v *VoxelBrushLine) StartDrag(pos voxel.Int3) []voxel.Int3 {
    v.start = pos
    return []voxel.Int3{pos}
}

func (v *VoxelBrushLine) Drag(cur voxel.Int3) []voxel.Int3 {
    var line []voxel.Int3
    geometry.Bresenham3D(int(v.start.X), int(v.start.Y), int(v.start.Z), int(cur.X), int(cur.Y), int(cur.Z), func(x, y, z int) bool {
        line = append(line, voxel.Int3{X: int32(x), Y: int32(y), Z: int32(z)})
        return true
    })
    return line
}

type VoxelBrushRectangle struct {
    start voxel.Int3
    fill  bool
}

func (v *VoxelBrushRectangle) StartDrag(pos voxel.Int3) []voxel.Int3 {
    v.start = pos
    return []voxel.Int3{pos}
}

func (v *VoxelBrushRectangle) Drag(cur voxel.Int3) []voxel.Int3 {
    // force same height
    cur = voxel.Int3{cur.X, v.start.Y, cur.Z}
    minX := min(v.start.X, cur.X)
    maxX := max(v.start.X, cur.X)

    minZ := min(v.start.Z, cur.Z)
    maxZ := max(v.start.Z, cur.Z)
    var rect []voxel.Int3

    if v.fill {
        for x := int(minX); x <= int(maxX); x++ {
            for z := int(minZ); z <= int(maxZ); z++ {
                rect = append(rect, voxel.Int3{int32(x), v.start.Y, int32(z)})
            }
        }
    } else {
        for x := int(minX); x <= int(maxX); x++ {
            rect = append(rect, voxel.Int3{int32(x), v.start.Y, minZ})
            rect = append(rect, voxel.Int3{int32(x), v.start.Y, maxZ})
        }
        for z := int(minZ); z <= int(maxZ); z++ {
            rect = append(rect, voxel.Int3{minX, v.start.Y, int32(z)})
            rect = append(rect, voxel.Int3{maxX, v.start.Y, int32(z)})
        }
    }

    return rect
}
