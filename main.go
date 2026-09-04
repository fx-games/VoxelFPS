package main

import (
    "flag"
    "fmt"
    "github.com/fsnotify/fsnotify"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/eassets"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
    "github.com/memmaker/goray/zdwep"
    _ "image/png"
    "log"
    "path/filepath"
    "strings"
)

var stringMapData = `################
#              #
#        #     #
#   P          #
#         #    #
################`

func main() {
    defaults := DefaultStressConfig()
    stress := flag.Bool("stress", false, "generate a stress map instead of loading the LDtk map")
    stressSize := flag.Int("stress-size", defaults.Size, "stress map edge length in tiles")
    stressPillars := flag.Int("stress-pillars", defaults.PillarSpacing, "stress map: one pillar every N tiles, 0 for none")
    stressSprites := flag.Int("stress-sprites", defaults.SpriteSpacing, "stress map: one barrel every N tiles, 0 for none")
    stressLightCount := flag.Int("stress-lights", defaults.Lights, "stress map: static spotlights on top of the flashlight")
    flag.Parse()

    var stressConfig *StressConfig
    var mapLoad core.MapLoadResult
    if *stress {
        stressConfig = &StressConfig{
            Size:          *stressSize,
            PillarSpacing: *stressPillars,
            SpriteSpacing: *stressSprites,
            Lights:        *stressLightCount,
        }
        mapLoad = loadStressMap(*stressConfig)
    } else {
        mapLoad = loadLDTK("assets/raymap.ldtk")
        //mapLoad = loadText(strings.Split(stringMapData, "\n"))
    }

    glhf.Run(func() {
        quads.Start(NewGame(stressConfig), mapLoad, 640, 480)
    })
}

// NewGame takes a stress config, or nil for a normal run.
func NewGame(stress *StressConfig) *Game {
    return &Game{
        isMouseCursorCaptured: true,
        stressConfig:          stress,
        currentGuiScaleIndex:  defaultGuiScaleIndex,
    }
}

type Lifekeeper interface {
    IsDead(delta float64) bool
}

type Game struct {
    *quads.GlApplication

    physicsSystem *quads.PhysicsSystem

    textLabel *quads.BitmapFontMesh

    lastMouseX float64
    lastMouseY float64

    crossHair *quads.AnimatedSprite

    uiLabel *quads.BitmapFontMesh
    player  *quads.GameObject

    relativeHitPos mgl32.Vec3
    entityMenu     *quads.GuiList

    widgetStack                []InputReceiver
    widgetsAreBlockingMovement bool
    table                      *quads.GameObject

    fogStart               float32
    fogEnd                 float32
    skyHorizonFade         float32
    exposure               float32
    bloomThreshold         float32
    bloomStrength          float32
    emissiveStrength       float32
    emissiveCutoff         float32
    emissiveSaturation     float32
    currentResolutionIndex int
    currentGuiScaleIndex   int
    currentWindowModeIndex int

    watchFileChanges      bool
    isMouseCursorCaptured bool
    projectileFlyingAnim  int

    projectileImpactAnim int
    lastHitResult        *RaycastResult

    gameObjectForSpawning *quads.GameObject
    gravitationEnabled    bool
    isMouseBeingDragged   bool

    voxelBlock           *voxel.Block
    voxelBrush           VoxelBrush
    voxelSelection       []voxel.Int3
    voxelSelectionEmpty  bool // if voxelSelectionEmpty is set to true, the selection will select the empty grid position right before the collision grid position
    voxelSelectionAction func(pos []voxel.Int3)

    weapons                     []Weapon
    currentWeaponIndex          int
    currentPaintGunTextureIndex int

    // Set only by -stress, nil on a normal run.
    stressConfig *StressConfig
}

func (g *Game) Init(app *quads.GlApplication, mapLoadResult core.MapLoadResult) {
    // Basics (Window / Camera)
    g.currentPaintGunTextureIndex = -1
    g.GlApplication = app

    g.SetupUIAtlas(eassets.Embedded.TextureAtlasFixed("ui_sheet.png", 16, 16))
    g.SetupUIFont(eassets.Embedded.BitmapFont("quadratica.png", 8, 14, g.UIScale))

    // 1, 2, 3, 4 switch between these.
    g.weapons = []Weapon{
        g.loadDoomWeapon("Ol_Faithful", WeaponEffectKillingProjectile, 0.4),
        g.loadDoomWeapon("Borstal_Shotgun", WeaponEffectHitscanVoxelDestruction, 0.8),
        g.loadDoomWeapon("Charge_Pistol", WeaponEffectHitscanVoxelPaintGun, 0.2),
        g.loadSheetWeapon("Small Gun", "assets/smallgun_sheet.png", 64, 64,
            []uint16{0, 1, 2, 3, 4}, 0.06, WeaponEffectHitscanVoxelDestruction, 0.3),
    }
    g.currentWeapon().Equip()

    g.SetupUnitCubes()
    //g.SetCursor("assets/cursor.png", 7, 7)

    // Load Bitmap Font
    // Setup Text Labels
    g.textLabel = g.NewLabel()
    g.textLabel.SetBackground(mgl32.Vec4{0, 0, 0, 0.9})

    g.uiLabel = g.NewLabel()
    g.uiLabel.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
    g.uiLabel.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.74})

    g.entityMenu = g.NewList()
    g.entityMenu.SetTextCentered(true)
    g.entityMenu.SetKeyHandler(func(selectedIndex int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
        if key == glfw.KeyEnter {
            switch selectedIndex {
            case 0:
                hoveredObj := g.hoveredObj()
                if hoveredObj != nil && hoveredObj.HasComponent(quads.ComponentTypeSprite) {
                    g.openSpriteEditor(hoveredObj)
                }
            case 1:
                hoveredObj := g.hoveredObj()
                if hoveredObj != nil {
                    hoveredObj.Kill()
                }
            }
            return true
        }
        return false
    })

    g.entityMenu.SetItems([]string{
        "Idle Frames",
        "remove entity",
    })

    // Skybox
    skyboxDir := "blink"
    skybox := quads.Disk.Cubemap([6]string{
        filepath.Join("assets", "skybox", skyboxDir, "right.png"),
        filepath.Join("assets", "skybox", skyboxDir, "left.png"),
        filepath.Join("assets", "skybox", skyboxDir, "top.png"),
        filepath.Join("assets", "skybox", skyboxDir, "bottom.png"),
        filepath.Join("assets", "skybox", skyboxDir, "front.png"),
        filepath.Join("assets", "skybox", skyboxDir, "back.png"),
    })
    g.SetupSkybox(skybox)

    // Setup HUD

    g.crossHair = g.Gui.Add(g.UIAtlas, 4, int(0), int(0))

    // Load Sprite Animations
    animations := getWolfCharAnimations()
    for i := quads.AnimIndex(0); i < quads.AnimCount; i += 1 {
        g.Sprites.AddAnimation(animations[i])
    }

    g.projectileFlyingAnim = g.Sprites.AddAnimation(quads.SpriteAnimation{
        Frames:            [][]uint16{{179, 180, 181}},
        ShowTimeInSeconds: []float32{0.1, 0.1, 0.1},
        Loop:              true,
    })

    g.projectileImpactAnim = g.Sprites.AddAnimation(quads.SpriteAnimation{
        Frames:            [][]uint16{{182, 183, 184}},
        ShowTimeInSeconds: []float32{0.1, 0.1, 0.1},
        Loop:              false,
    })

    g.SetupParticleSystem(quads.ParticlesBulletImpact, 2000)
    g.SetupParticleSystem(quads.ParticlesBlood, 2000)
    g.SetupParticleSystem(quads.ParticlesCustom, 2000)
    //midiplayer := audio.NewPlayer(44100)
    //midiStream, _ := audio.RenderSMF("assets/audio/SC-55.sf2", "assets/audio/duke1.mid", 44100)
    /*
       file, err := os.Open("assets/out.raw")
       midiStream, _ := audio.LoadRawSamples(file)
       file.Close()
       if err != nil {
           fmt.Println(err)
       }
       midiplayer.StartStream(midiStream, false)
    */

    g.mapLoad(mapLoadResult, false)

    if g.stressConfig != nil {
        g.AddLights(stressLights(*g.stressConfig, g.ShadowResolution())...)
    }

    events.Subscribe(events.EventPlayerWeaponFired, func(id events.EventID, event events.Event) {
        origin, direction := g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
        g.fireWeapon(origin, direction, event.(WeaponEffect))
    })

    events.Subscribe(events.EventHitScanCollision, g.hitScanCollision)
    events.Subscribe(events.EventPhysicsCollision, g.physicsCollision)
    events.Subscribe(events.EventPlayerInteractWith, g.playerInteractWith)

    g.HandleResize(g.WindowWidth, g.WindowHeight)
}

// loadDoomWeapon reads a Doom weapon sprite atlas plus the ZDoom script that is
// stored alongside it in the PNG, and hands back a weapon that fires with the
// given effect.
func (g *Game) loadDoomWeapon(weaponName string, effect WeaponEffect, delayBetweenShots float64) *DoomWeapon {
    atlas, zscript, err := zdwep.ReadDoomAtlasFromFile(filepath.Join("assets", weaponName+".png"))
    if err != nil {
        panic(err)
    }
    g.SetupSpriteBatch(atlas)

    script, err := zdwep.ParseActorScript(zscript)
    if err != nil {
        panic(err)
    }

    weapon := newDoomWeapon(weaponName, script.ParsedScript)
    weapon.Atlas = atlas
    weapon.Scale = g.UIScale
    weapon.Effect = effect
    weapon.DelayBetweenShots = delayBetweenShots

    spriteMap := make(map[string]int)
    for i := 0; i <= atlas.LastValidIndex(); i++ {
        spriteMap[atlas.NameOfFrame(i)] = i
    }
    weapon.spriteToIndex = spriteMap

    return weapon
}

func (g *Game) mapLoad(mapLoad core.MapLoadResult, hotReload bool) {
    g.Sprites.Clear()
    g.Meshes.Clear()

    // Load Map
    entitySpawns := g.LoadVoxelMap(mapLoad)

    // The wall textures in the Blake Stone sheet that carry a lit fitting: the cyan tube
    // panels (19, 27) and the console screens (47), which are the three this map actually
    // uses, plus the lit strips (29, 30), FOOD UNIT signs (45, 46) and cyan vent (26) for
    // other maps. Marking a texture does not light the whole tile - only the texels that
    // are both bright and colourful, so the tubes and screens come on and the grey metal
    // panel they are mounted on stays dark.
    //
    // 49 is a brightened copy of the console panel 47, written into the free slot in the
    // wall sheet's last row: same art with its screen readouts lifted, so the screens
    // clear the emissive cutoff by a wide margin and read as lit rather than as painted.
    // raymap.ldtk points the two console walls at it.
    //
    // Indices are literal because this atlas's layout is fixed and indexed directly
    // everywhere else too, see ISSUES.md. Press G at any wall to add another at runtime.
    g.CubeMap.SetEmissiveTextures([]uint16{19, 26, 27, 29, 30, 45, 46, 47, 49})

    /*
       sphere := physics.NewSphericalHull(5)
       bulletSDK := bullet.BulletNewPhysicsSDK()
       world := bullet.BulletNewDynamicWorld(bulletSDK)
       bullet.BulletAddRigidBody(world, sphere.BulletHandle())
       bullet.BulletStepSimulation(world, 0.1)
    */
    // Setup Collision Solver
    g.physicsSystem = quads.NewCollisionSolver(g.CubeMap.IsSolidBlockAt, g.CubeMap.ContainsVec)
    g.SetupPathfinder(g.CubeMap.IsSolidBlockAt)

    // Spawn Entities
    for spawnPos, entity := range entitySpawns {
        g.spawnEntity(spawnPos, entity)
    }
    if hotReload {
        g.physicsSystem.AddObject(g.player.Physics())
    } else {
        g.player = g.spawnPlayer(g.Camera, mapLoad.PlayerSpawn, mapLoad.PlayerDir)
    }
    // Setup Ceiling & Fog
    g.CeilingColor = mapLoad.CeilingColorVec4()
    g.fogStart = 4
    g.fogEnd = 22
    g.skyHorizonFade = 7
    g.exposure = quads.DefaultExposure
    g.bloomThreshold = quads.DefaultBloomThreshold
    g.bloomStrength = quads.DefaultBloomStrength
    g.emissiveStrength = quads.DefaultEmissiveStrength
    g.emissiveCutoff = quads.DefaultEmissiveCutoff
    g.emissiveSaturation = quads.DefaultEmissiveSaturation
    g.updateFog()
    g.SetLight(mgl32.Vec3{2, 2, 4}, mgl32.Vec3{1, float32(214) / float32(255), float32(110) / float32(255)})

    //scientist.Sprite().PlayAnimationLooping(int(quads.AnimWalk))
    g.SetupMinimap(mapLoad.MapDimensions[0], mapLoad.MapDimensions[2], g.colorForMinimap)

}

// HandleResize lays the HUD out in GUI coordinates, not window coordinates, so the
// GUI scale applies to it.
func (g *Game) HandleResize(windowWidth, windowHeight int) {
    guiWidth, guiHeight := g.GuiSize()
    if len(g.widgetStack) > 0 {
        g.focusedWidget().HandleResize(guiWidth, guiHeight, g.UIScale)
    }

    screenCenter := mgl32.Vec2{float32(guiWidth) / 2, float32(guiHeight) / 2}
    listWidth := guiWidth / 3
    listHeight := min(guiHeight/2, 160)

    // center in the right side of the screen
    listX := float32(guiWidth)/2 + (float32(guiWidth) / 4) - (float32(listWidth) / 2)
    g.entityMenu.SetOrigin(listX, screenCenter.Y())
    g.entityMenu.SetBounds(float32(listWidth), float32(listHeight))

    tileW, tileH := g.UIAtlas.GetAtlasItemSize()
    centerX := (float32(guiWidth) / 2) - (float32(tileW) / 2)
    centerY := (float32(guiHeight) / 2) - (float32(tileH) / 2)

    g.crossHair.SetDestRect(mgl32.Vec4{centerX, centerY, centerX + float32(tileW), centerY + float32(tileH)})
    /*
       doubleW := windowWidth / 3
       doubleH := doubleW
       weaponX := (float32(windowWidth) / 2) - (float32(doubleW) / 2) - 12
       weaponY := float32(windowHeight) - float32(doubleH)

       g.weaponQuad.SetSize(doubleW, doubleH)
       g.weaponQuad.SetPosition(mgl32.Vec2{weaponX, weaponY})
    */
}
func (g *Game) focusedWidget() InputReceiver {
    if len(g.widgetStack) == 0 {
        return nil
    }
    return g.widgetStack[len(g.widgetStack)-1]
}
func (g *Game) Update(elapsed float64) {
    if len(g.widgetStack) > 0 {
        // event..
        if g.focusedWidget().ShouldClose() {
            g.popFromWidgetStack()
        }
    }

    if true || g.ShouldDrawDebugInfo {
        g.printDebugInfo()
    }

    g.pollInput(elapsed)

    g.physicsSystem.Update(elapsed)
    g.updateGUI(elapsed)
}

func (g *Game) printDebugInfo() {
    forward := g.Camera.GetForward()
    pos := g.Camera.GetPosition()
    rayCastPosString := "nil"
    lengthOfRay := "n/a"
    if g.lastHitResult != nil {
        rayCastPosString = vec3String(g.lastHitResult.WorldPosition())
        lengthOfRay = fmt.Sprintf("%.2f", g.lastHitResult.HitInfo.Distance)
    }

    spriteTrisDrawn, spriteDrawCalls := g.Sprites.DrawnLastFrame()
    guiTrisDrawn, guiDrawCalls := g.Gui.DrawnLastFrame()
    meshTriangles, meshDrawCalls := g.Meshes.DrawnLastFrame()
    shadowPasses, litLights := g.ShadowPasses()
    meshGen, sceneHash := g.ShadowCacheState()

    rows := []core.TableRow{
        core.NewTableRow("Cam Fwd", "", vec3String(forward)),
        core.NewTableRow("Cam Pos", "", vec3String(pos)),
        core.NewTableRow("Ray Hit", "", rayCastPosString),
        core.NewTableRow("Ray Len", "", lengthOfRay),
        core.NewTableRow("Coll. Pairs", "", fmt.Sprintf("%d", g.physicsSystem.PairsChecked())),
        core.NewTableRow("Map", fmt.Sprintf("%d tris", g.CubeMap.TriangleDrawCount()), fmt.Sprintf("%d", g.CubeMap.DrawCalls())),
        core.NewTableRow("GUI", fmt.Sprintf("%d tris", guiTrisDrawn), fmt.Sprintf("%d", guiDrawCalls)),
        core.NewTableRow("Sprites", fmt.Sprintf("%d tris", spriteTrisDrawn), fmt.Sprintf("%d", spriteDrawCalls)),
        core.NewTableRow("Meshes", fmt.Sprintf("%d tris", meshTriangles), fmt.Sprintf("%d", meshDrawCalls)),
        core.NewTableRow("Shadows", fmt.Sprintf("%d/%d lit", shadowPasses, litLights), fmt.Sprintf("gen %d", meshGen)),
        core.NewTableRow("Shadow key", fmt.Sprintf("%04x", sceneHash&0xffff), ""),
    }
    debugLines := core.TableLayout(rows, []core.TextAlignment{core.AlignLeft, core.AlignRight, core.AlignRight})
    g.PrintMulti(debugLines)
}

func vec3String(v mgl32.Vec3) string {
    return fmt.Sprintf("%.2f, %.2f, %.2f", v.X(), v.Y(), v.Z())
}

func (g *Game) popFromWidgetStack() {
    if len(g.widgetStack) == 0 {
        return
    }
    g.widgetStack = g.widgetStack[:len(g.widgetStack)-1]
    if len(g.widgetStack) == 0 {
        g.widgetsAreBlockingMovement = false
    }
}

func (g *Game) replaceWidgetStack(widget InputReceiver) {
    g.widgetStack = []InputReceiver{widget}
}

func (g *Game) pushToWidgetStack(widget InputReceiver) {
    for _, w := range g.widgetStack {
        if w == widget {
            g.bringToFront(widget)
            return
        }
    }
    g.widgetStack = append(g.widgetStack, widget)
}

func (g *Game) bringToFront(widget InputReceiver) {
    for i, w := range g.widgetStack {
        if w == widget {
            g.widgetStack = append(g.widgetStack[:i], g.widgetStack[i+1:]...)
            g.widgetStack = append(g.widgetStack, widget)
            return
        }
    }
}

func (g *Game) DrawGui(elapsed float64) {
    hoveredObj := g.hoveredObj()
    if hoveredObj != nil {
        guiBatch := g.UIAtlas.GetSpriteBatch()

        aabb, transform := g.getHoveredOBB(hoveredObj)

        topLeft, topRight, bottomLeft, bottomRight := quads.HudCornersFromOBB(aabb, transform, g.Camera.ProjectWorldToScreenPixels)

        borderColor := mgl32.Vec4{0.5, 1, 0.5, 0.8}

        tlScr, tlDst := g.UIAtlas.GetSourceAndDestRects(0, mgl32.Vec2{float32(topLeft[0]), float32(topLeft[1])}, mgl32.Vec2{0, 0}, mgl32.Vec2{1, 1})
        guiBatch.DrawTextured(tlScr, tlDst, borderColor)

        trScr, trDst := g.UIAtlas.GetSourceAndDestRects(1, mgl32.Vec2{float32(topRight[0]), float32(topRight[1])}, mgl32.Vec2{1, 0}, mgl32.Vec2{1, 1})
        guiBatch.DrawTextured(trScr, trDst, borderColor)

        brScr, brDst := g.UIAtlas.GetSourceAndDestRects(2, mgl32.Vec2{float32(bottomRight[0]), float32(bottomRight[1])}, mgl32.Vec2{1, 1}, mgl32.Vec2{1, 1})
        guiBatch.DrawTextured(brScr, brDst, borderColor)

        blScr, blDst := g.UIAtlas.GetSourceAndDestRects(3, mgl32.Vec2{float32(bottomLeft[0]), float32(bottomLeft[1])}, mgl32.Vec2{0, 1}, mgl32.Vec2{1, 1})
        guiBatch.DrawTextured(blScr, blDst, borderColor)

        if g.uiLabel != nil {
            hitZone := "body"
            if g.relativeHitPos.Y() > 0.6 && g.relativeHitPos.Y() < 0.8 {
                hitZone = "head"
            }
            labelText := fmt.Sprintf("%s (%s)", hoveredObj.DisplayName, hitZone)
            g.uiLabel.SetText(labelText)
            _, fontH := g.uiLabel.Size()
            g.uiLabel.SetPosition(mgl32.Vec2{float32(topLeft[0]), float32(topLeft[1] - fontH)})

            g.uiLabel.Draw()

            if g.entityMenu != nil {
                g.entityMenu.Draw()
            }
        }
    }

    if weapon := g.currentWeapon(); weapon != nil {
        guiWidth, guiHeight := g.GuiSize()
        weapon.Draw(elapsed, int32(guiWidth), int32(guiHeight))
    }

    if g.textLabel != nil { //&& g.shouldDrawDebugInfo {
        g.textLabel.Draw()
    }

    for _, widget := range g.widgetStack {
        widget.Draw()
    }
}

func (g *Game) startWeaponFireAnimation() {
    weapon := g.currentWeapon()
    if weapon == nil || !weapon.Fire(glfw.GetTime()) {
        return
    }
    events.Publish(events.EventPlayerWeaponFired, weapon.FireEffect())
}

func (g *Game) hoveredObj() *quads.GameObject {
    if g.lastHitResult == nil {
        return nil
    }

    if !g.lastHitResult.HitInfo.Hit || g.lastHitResult.HitInfo.Distance > 10 || g.lastHitResult.HitObject == nil {
        return nil
    }

    return g.lastHitResult.HitObject
}

func (g *Game) isHoveringObject() bool {
    if g.lastHitResult == nil {
        return false
    }
    if !g.lastHitResult.HitInfo.Hit || g.lastHitResult.HitInfo.Distance > 10 {
        return false
    }
    if g.lastHitResult.HitObject == nil {
        return false
    }
    return true

}
func (g *Game) updateGUI(elapsed float64) {
    if g.widgetsAreBlockingMovement {
        return
    }
    g.lastHitResult = nil

    g.uiLabel.Hide()
    g.entityMenu.Hide()

    var result RaycastResult
    if g.isMouseCursorCaptured {
        result = g.rayCast(g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5))
    } else {
        result = g.rayCastFromScreenPos(g.Window.GetCursorPos())
        g.UnitCube.QueueDraw(g.voxelSelection)
    }

    g.lastHitResult = &result

    if g.gameObjectForSpawning != nil {
        groundPos, hitGround := g.lastHitResult.GroundPosition()
        if hitGround {
            g.gameObjectForSpawning.Transform().SetPosition(groundPos)
        }
    }

    hoveredObj := g.hoveredObj()

    if hoveredObj == nil {
        return
    }

    g.uiLabel.Show()
    g.entityMenu.Show()
}

func (g *Game) getHoveredOBB(hoveredObj *quads.GameObject) (voxel.AABB, mgl32.Mat4) {
    aabb := hoveredObj.Physics().AABBAt(mgl32.Vec3{})
    var transform mgl32.Mat4
    if hoveredObj.HasComponent(quads.ComponentTypeSprite) {
        transform = hoveredObj.Sprite().Transform(g.Camera)
    } else if hoveredObj.HasComponent(quads.ComponentTypeMesh) {
        transform = hoveredObj.Transform().GetTransformMatrix()
    }
    return aabb, transform
}

type RaycastResult struct {
    HitObject *quads.GameObject
    WorldPos  mgl32.Vec3
    Relative  mgl32.Vec3
    HitInfo   voxel.HitInfo3D
}

func (r RaycastResult) WorldPosition() mgl32.Vec3 {
    if !r.HitInfo.Hit {
        return mgl32.Vec3{}
    }
    if r.HitObject == nil {
        return r.HitInfo.CollisionWorldPosition
    }
    return r.WorldPos
}
func (r RaycastResult) GroundPosition() (mgl32.Vec3, bool) {
    if !r.HitInfo.Hit || r.HitObject != nil {
        return mgl32.Vec3{}, false
    }
    cwp := r.HitInfo.CollisionWorldPosition
    //worldPos := mgl32.Vec3{cwp.X(), float32(math.Floor(float64(cwp.Y()))), cwp.Z()}
    return cwp, r.HitInfo.Side == voxel.Top
}
func (g *Game) rayCastFromScreenPos(screenPosX, screenPosY float64) RaycastResult {
    normalizedX := screenPosX / float64(g.WindowWidth)
    normalizedY := screenPosY / float64(g.WindowHeight)
    origin, dir := g.Camera.GetPickingRayFromScreenPosition(normalizedX, normalizedY)
    return g.rayCast(origin, dir)
}

// rayCastRange is how far picking rays reach, in world units.
const rayCastRange = 100.0

func (g *Game) rayCast(origin, direction mgl32.Vec3) RaycastResult {
    var hitInfo voxel.HitInfo3D
    var hitObject *quads.GameObject
    var worldPos mgl32.Vec3
    var relativePos mgl32.Vec3
    hitInfo = voxel.DDARaycast(origin, direction, rayCastRange, func(x, y, z int32) (stopRay bool) {
        if !g.CubeMap.Contains(int32(x), int32(y), int32(z)) {
            return false // outside the map there is nothing to hit, keep going until the ray runs out
        }
        for gObj := range g.physicsSystem.GetCollidersAt(x, y, z) {
            if gObj.GameObject() == g.player || gObj.GameObject() == g.gameObjectForSpawning {
                continue
            }
            intersectionDistance, intersects := voxel.IntersectRayOBB(origin, direction, gObj.OBB(g.Camera))
            if !intersects {
                continue
            }
            collisionWorldPos := origin.Add(direction.Mul(float32(intersectionDistance)))
            hitObject = gObj.GameObject()
            relativeToObj := collisionWorldPos.Sub(gObj.Position())
            worldPos = collisionWorldPos
            relativePos = relativeToObj
            return true
        }
        return g.CubeMap.IsSolidBlockAt(x, y, z)
    })

    return RaycastResult{
        HitObject: hitObject,
        WorldPos:  worldPos,
        Relative:  relativePos,
        HitInfo:   hitInfo,
    }
}

func (g *Game) Print(text string) {
    // split text into lines
    // to lower
    lines := strings.Split(text, "\n")
    g.textLabel.SetMultilineText(lines)
}

func (g *Game) PrintMulti(text []string) {
    g.textLabel.SetMultilineText(text)
}

func (g *Game) openSpriteEditor(obj *quads.GameObject) {
    //TODO
}
func (g *Game) openCubeTextureMapper() {
    if g.voxelBlock == nil {
        g.Toast("No voxel block loaded")
        return
    }
    g.ShouldDrawMinimap = false
    g.ShouldDrawDebugInfo = false
    g.SetMouseCapture(false)

    g.widgetsAreBlockingMovement = true
    atlasPicker := g.OpenTextureCube(g.AtlasMap, g.voxelBlock.Textures)
    guiWidth, guiHeight := g.GuiSize()
    atlasPicker.HandleResize(guiWidth, guiHeight, g.UIScale)

    atlasPicker.SetOnClose(func() {
        g.SetMouseCapture(true)
    })
    g.replaceWidgetStack(atlasPicker)
}

func (g *Game) pickTexture(atlas glhf.TextureAtlas, onSelected func(selectedIndex int)) {
    previousMouseState := g.setupModal()
    g.widgetsAreBlockingMovement = true
    atlasPicker := g.OpenAtlasPicker(atlas)
    atlasPicker.SetOnIndexClicked(func(selection int) {
        g.popFromWidgetStack()
        onSelected(selection)
    })
    atlasPicker.SetOnClose(func() {
        g.captureMouse(previousMouseState)
    })
    g.replaceWidgetStack(atlasPicker)
}
func (g *Game) openEscapeMenu() {
    previousMouseState := g.setupModal()
    configMenu := g.OpenConfigMenu()

    options := []quads.GuiOption{
        quads.NewGuiOptionAction("Continue", g.popFromWidgetStack),
        quads.NewGuiOptionAction("Quit", g.quitGame),
    }
    configMenu.SetOptions(options)
    guiWidth, guiHeight := g.GuiSize()
    configMenu.HandleResize(guiWidth, guiHeight, g.UIScale)
    configMenu.SetOnClose(func() {
        g.captureMouse(previousMouseState)
        g.popFromWidgetStack()
    })
    g.pushToWidgetStack(configMenu)
}

func (g *Game) setupModal() bool {
    g.player.Physics().SetVelocity(mgl32.Vec3{})
    g.widgetsAreBlockingMovement = true
    previousMouseState := g.isMouseCursorCaptured
    g.captureMouse(false)
    return previousMouseState
}
func (g *Game) openWizardMenu() {
    previousMouseState := g.setupModal()
    configMenu := g.OpenConfigMenu()
    vModes := g.GetVideoModes()
    var resolutionOptions []string
    for _, vMode := range vModes {
        resolutionOptions = append(resolutionOptions, fmt.Sprintf("%dx%d", vMode.Width, vMode.Height))
    }
    options := []quads.GuiOption{
        quads.NewGuiOptionBool("Show Debug Info", &g.ShouldDrawDebugInfo),
        quads.NewGuiOptionAction("Generate MipMaps for Terrain", func() {
            //core.BigDebugSwitch = !core.BigDebugSwitch
            g.CubeMap.GetTerrainTexture().GenerateMipMaps()
        }),
        quads.NewGuiOptionList("Window Mode", quads.WindowModeNames, &g.currentWindowModeIndex, g.setWindowMode),
        quads.NewGuiOptionAction("VSync", g.toggleVsync),

        quads.NewGuiOptionList("Window", resolutionOptions, &g.currentResolutionIndex, g.setWindowResolution),
        quads.NewGuiOptionList("GUI Scale", guiScaleLabels, &g.currentGuiScaleIndex, g.setGuiScale),
    }

    // Only macOS puts a menu bar and Dock over a borderless window, so only there is
    // there anything to choose between.
    if quads.CanConstrainToWorkArea {
        options = append(options, quads.NewGuiOptionBool("Constrain to work area", &g.ConstrainToWorkArea).
            ActivateFunc(g.RefreshWindowMode))
    }

    options = append(options,
        quads.NewGuiOptionBool("Show Chunk AABBs", &g.ShouldDrawChunkAABBs),
        quads.NewGuiOptionBool("Show AABBs", &g.ShouldDrawAABBs),
        quads.NewGuiOptionBool("Show OBBs", &g.ShouldDrawOBBs),
        quads.NewGuiOptionBool("Show Paths", &g.ShouldDrawPaths),
        quads.NewGuiOptionBool("Show Sprites", &g.ShouldDrawSprites),
        quads.NewGuiOptionBool("Show Meshes", &g.ShouldDrawMeshes),
        quads.NewGuiOptionBool("Show Skybox", &g.ShouldDrawSkybox),
        quads.NewGuiOptionBool("Show Minimap", &g.ShouldDrawMinimap),
        quads.NewGuiOptionBool("Show Particles", &g.ShouldDrawParticles),
        quads.NewGuiOptionFloat("Fog Start", &g.fogStart).StepSize(5).ActivateFunc(g.updateFog),
        quads.NewGuiOptionFloat("Fog End", &g.fogEnd).StepSize(5).ActivateFunc(g.updateFog),
        quads.NewGuiOptionFloat("Sky Horizon Fade", &g.skyHorizonFade).StepSize(1).ActivateFunc(g.updateFog),
        quads.NewGuiOptionBool("Bloom", &g.ShouldDrawBloom),
        quads.NewGuiOptionFloat("Exposure", &g.exposure).StepSize(0.1).ActivateFunc(g.updateToneMapping),
        quads.NewGuiOptionFloat("Bloom Threshold", &g.bloomThreshold).StepSize(0.1).ActivateFunc(g.updateBloom),
        quads.NewGuiOptionFloat("Bloom Strength", &g.bloomStrength).StepSize(0.05).ActivateFunc(g.updateBloom),
        quads.NewGuiOptionFloat("Emissive Strength", &g.emissiveStrength).StepSize(0.5).ActivateFunc(g.updateEmissive),
        quads.NewGuiOptionFloat("Emissive Cutoff", &g.emissiveCutoff).StepSize(0.05).ActivateFunc(g.updateEmissive),
        quads.NewGuiOptionFloat("Emissive Saturation", &g.emissiveSaturation).StepSize(0.05).ActivateFunc(g.updateEmissive),
    )

    if !g.watchFileChanges {
        options = append(options, quads.NewGuiOptionAction("Start Map Hot Reload", func() {
            g.popFromWidgetStack()
            g.Toast("Watching for map changes")
            g.watchFileChanges = true
            go g.watchFileChangesLoop()
        }))
    }
    configMenu.SetOptions(options)
    guiWidth, guiHeight := g.GuiSize()
    configMenu.HandleResize(guiWidth, guiHeight, g.UIScale)
    configMenu.SetOnClose(func() {
        g.captureMouse(previousMouseState)
        g.popFromWidgetStack()
    })
    g.pushToWidgetStack(configMenu)
}
// guiScaleSteps are the GUI scale factors the menu offers, with their labels.
// 1.0 is the size the GUI has always been drawn at.
var guiScaleSteps = []float32{0.25, 0.5, 1.0, 2.0, 3.0, 4.0, 6.0, 8.0}

var guiScaleLabels = func() []string {
    labels := make([]string, len(guiScaleSteps))
    for i, scale := range guiScaleSteps {
        // 0.25x keeps both decimals, 1.00x reads better as 1.0x.
        labels[i] = strings.TrimSuffix(fmt.Sprintf("%.2f", scale), "0") + "x"
    }
    return labels
}()

// defaultGuiScaleIndex is the entry for 1.0.
const defaultGuiScaleIndex = 2

func (g *Game) setGuiScale(index int) {
    g.SetGuiScale(guiScaleSteps[index])
    g.Toast(fmt.Sprintf("GUI scale: %s", guiScaleLabels[index]))
}

func (g *Game) setWindowMode(index int) {
    g.SetWindowMode(quads.WindowMode(index))
    g.Toast(fmt.Sprintf("Window mode: %s", quads.WindowModeNames[index]))
}

func (g *Game) setWindowResolution(index int) {
    g.SetWindowSize(g.GetVideoModes()[index])
    g.currentWindowModeIndex = int(quads.WindowModeWindowed)
}
func (g *Game) updateToneMapping() {
    g.SetExposure(g.exposure)
    g.Toast(fmt.Sprintf("Exposure: %.2f", g.exposure))
}

func (g *Game) updateBloom() {
    g.SetBloom(g.bloomThreshold, g.bloomStrength)
    g.Toast(fmt.Sprintf("Bloom: threshold %.2f, strength %.2f", g.bloomThreshold, g.bloomStrength))
}

func (g *Game) updateEmissive() {
    g.SetEmissiveStrength(g.emissiveStrength)
    g.SetEmissiveCutoff(g.emissiveCutoff)
    g.SetEmissiveSaturation(g.emissiveSaturation)
    g.Toast(fmt.Sprintf("Emissive: strength %.1f, cutoff %.2f, sat %.2f", g.emissiveStrength, g.emissiveCutoff, g.emissiveSaturation))
}

func (g *Game) updateFog() {
    toastMessage := fmt.Sprintf("Fog: %.0f - %.0f, horizon %.0f deg", g.fogStart, g.fogEnd, g.skyHorizonFade)
    g.Toast(toastMessage)
    g.SetFog(g.CeilingColor.Vec3(), g.fogStart, g.fogEnd)
    g.SetSkyFog(g.skyHorizonFade)
}


func (g *Game) toggleVsync() {
    g.ShouldUseVsync = !g.ShouldUseVsync
    g.SetVSync(g.ShouldUseVsync)
    g.Toast(fmt.Sprintf("VSync: %t", g.ShouldUseVsync))
}

func (g *Game) toggleMouseCapture() {
    g.isMouseCursorCaptured = !g.isMouseCursorCaptured
    g.SetMouseCapture(g.isMouseCursorCaptured)
}

func (g *Game) HandleDrop(names []string, screenDropX, screenDropY float64) {
    if len(names) == 0 {
        return
    }

    if len(g.widgetStack) > 0 {
        guiX, guiY := g.GuiCoords(screenDropX, screenDropY)
        if g.focusedWidget().HandleDrop(names, guiX, guiY) {
            return
        }
    }

    /*
       rectSize := 64
       padding := 6
       //var loadedTextures []*glhf.Texture
       drawX := 30
       drawY := 30
       for _, name := range names {
           ext := filepath.Ext(name)
           if strings.ToLower(ext) == ".png" {
               loadedText := quads.Disk.Texture(name)
               drawX += rectSize + padding
               if drawX > g.WindowWidth-rectSize {
                   drawX = 30
                   drawY += rectSize + padding
               }

               baseName := filepath.Base(name)
           }
       }

    */
}

func (g *Game) watchFileChangesLoop() {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        log.Fatal(err)
    }
    defer watcher.Close()
    // Add a path.
    fileToWatch := filepath.Join("assets", "raymap.ldtk")
    err = watcher.Add(fileToWatch)
    if err != nil {
        log.Fatal(err)
    }
    for {
        select {
        case event, ok := <-watcher.Events:
            if !ok {
                return
            }
            log.Println("event:", event)
            if event.Has(fsnotify.Write) {
                log.Println("modified file:", event.Name)
                glhf.Call(func() {
                    g.mapLoad(loadLDTK(fileToWatch), true)
                })
            }
        case err, ok := <-watcher.Errors:
            if !ok {
                return
            }
            log.Println("error:", err)
        }
    }
}

func (g *Game) colorForMinimap(x int, y int) [4]uint8 {
    playerX := int(g.player.GameObject().Transform().GetPosition().X())
    playerY := int(g.player.GameObject().Transform().GetPosition().Z())
    playerHeight := int(g.player.GameObject().Transform().GetPosition().Y())
    if x == playerX && y == playerY {
        return [4]uint8{80, 255, 255, 255}
    }
    blockAt := g.CubeMap.GetGlobalBlock(int32(x), int32(playerHeight), int32(y))
    if blockAt.IsSolid {
        return [4]uint8{100, 244, 100, 255}
    }
    collidersAt := g.physicsSystem.GetCollidersAt(int32(x), int32(playerHeight), int32(y))
    for physics := range collidersAt {
        if physics.GameObject().DisplayName == "Scientist" {
            return [4]uint8{255, 20, 20, 255}
        }
    }
    alpha := 160
    if len(collidersAt) > 0 {
        alpha = 255
    }
    return [4]uint8{10, 10, uint8(10 + (len(collidersAt) * 120)), uint8(alpha)}
}

func getWolfCharAnimations() []quads.SpriteAnimation {
    // 6 fps at 60 fps = 0.1 seconds
    // 8 fps at 60 fps = 0.125 seconds
    getSameDelays := func(frameCount int, delay float32) []float32 {
        delays := make([]float32, frameCount)
        for i := 0; i < frameCount; i += 1 {
            delays[i] = delay
        }
        return delays
    }
    idleAnim := quads.SpriteAnimation{
        Frames: [][]uint16{
            {0}, // direction 0
            {1}, // direction 1
            {2}, // direction 2
            {3}, // direction 3
            {4}, // direction 4
            {5}, // direction 5
            {6}, // direction 6
            {7}, // direction 7
        },
        ShowTimeInSeconds: getSameDelays(1, 0.2),
    }
    walkAnim := quads.SpriteAnimation{
        Frames: [][]uint16{
            {8, 16, 24, 32},  // direction 0
            {9, 17, 25, 33},  // direction 1
            {10, 18, 26, 34}, // direction 2
            {11, 19, 27, 35}, // direction 3
            {12, 20, 28, 36}, // direction 4
            {13, 21, 29, 37}, // direction 5
            {14, 22, 30, 38}, // direction 6
            {15, 23, 31, 39}, // direction 7
        },
        ShowTimeInSeconds: getSameDelays(4, 0.2),
    }

    dyingAnim := quads.SpriteAnimation{
        Frames:            [][]uint16{{40, 41, 42, 43, 44}},
        ShowTimeInSeconds: getSameDelays(5, 0.2),
    }
    hitAnim := quads.SpriteAnimation{
        Frames:            [][]uint16{{45}},
        ShowTimeInSeconds: getSameDelays(1, 0.2),
    }
    prepareFireAnim := quads.SpriteAnimation{
        Frames:            [][]uint16{{46, 47}},
        ShowTimeInSeconds: getSameDelays(2, 0.22),
    }
    fireAnim := quads.SpriteAnimation{
        Frames:            [][]uint16{{48, 47, 46}},
        ShowTimeInSeconds: getSameDelays(3, 0.22),
    }

    /*
              // front=ft, back=bk, left=lt, right=rt
              // front_left=fl, front_right=fr, back_left=bl, back_right=br
              //

       id: idle
       delays: 0.2
       dir_ft: 0
       dir_fl: 1
       dir_lt: 2
       dir_bl: 3
       dir_bk: 4
       dir_br: 5
       dir_rt: 6
       dir_fr: 7

       id: walk
       delays: 0.2, 0.2, 0.2, 0.2
       dir_ft: 8, 16, 24, 32
       dir_fl: 9, 17, 25, 33
       dir_lt: 10, 18, 26, 34
       dir_bl: 11, 19, 27, 35
       dir_bk: 12, 20, 28, 36
       dir_br: 13, 21, 29, 37
       dir_rt: 14, 22, 30, 38
       dir_fr: 15, 23, 31, 39

         id: dying
            delays: 0.2, 0.2, 0.2, 0.2, 0.2
            dir_ft: 40, 41, 42, 43, 44

            id: hit
            delays: 0.2
            dir_ft: 45

            id: prepare_fire
            delays: 0.22, 0.22
            dir_ft: 46, 47

            id: fire
            delays: 0.22, 0.22, 0.22
            dir_ft: 48, 47, 46
    */
    animations := []quads.SpriteAnimation{
        idleAnim,        // 0
        walkAnim,        // 1
        dyingAnim,       // 2
        hitAnim,         // 3
        prepareFireAnim, // 4
        fireAnim,        // 5
    }
    return animations
}
