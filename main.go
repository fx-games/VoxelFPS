package main

import (
    "fmt"
    "github.com/fsnotify/fsnotify"
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/eassets"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
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

    mapLoad := loadLDTK("assets/raymap.ldtk")
    //mapLoad := loadText(strings.Split(stringMapData, "\n"))
    //readWad()
    glhf.Run(func() {
        quads.Start(NewGame(), mapLoad, 640, 480)
    })
}
func NewGame() *Game {
    return &Game{shouldDrawSprites: true, shouldDrawMeshes: true, shouldDrawParticles: true, isMouseCursorCaptured: true, shouldUseVsync: true}
}

type Lifekeeper interface {
    IsDead(delta float64) bool
}

type Game struct {
    *quads.GlApplication

    cubeMap *voxel.Map

    physicsSystem *quads.PhysicsSystem

    textLabel *quads.BitmapFontMesh

    lastMouseX float64
    lastMouseY float64

    crossHair *quads.GuiQuad

    uiRectSpriteTL *quads.GuiQuad
    uiRectSpriteTR *quads.GuiQuad
    uiRectSpriteBL *quads.GuiQuad
    uiRectSpriteBR *quads.GuiQuad

    uiLabel *quads.BitmapFontMesh
    player  *quads.GameObject

    relativeHitPos mgl32.Vec3
    uiAtlas        *glhf.Texture
    entityMenu     *quads.GuiList

    widgetStack                []InputReceiver
    widgetsAreBlockingMovement bool
    table                      *quads.GameObject
    defaultFont                quads.BitmapFont

    shouldDrawAABBs        bool
    shouldDrawChunkAABBs   bool
    shouldDrawOBBs         bool
    shouldDrawPaths        bool
    shouldDrawMeshes       bool
    shouldDrawSprites      bool
    shouldDrawParticles    bool
    shouldDrawDebugInfo    bool
    shouldRenderFullscreen bool
    shouldUseVsync         bool

    fogStart               float32
    fogEnd                 float32
    currentResolutionIndex int

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

    currentWeapon               *Weapon
    weaponQuad                  *quads.GuiQuad
    currentPaintGunTextureIndex int
}

func (g *Game) Init(app *quads.GlApplication, mapLoadResult core.MapLoadResult) {
    // Basics (Window / Camera)
    g.currentPaintGunTextureIndex = -1
    g.GlApplication = app

    g.SetupUnitCubes()
    //g.SetCursor("assets/cursor.png", 7, 7)

    // Load Bitmap Font
    bitmapFont := eassets.LoadEmbeddedFont(g.UIScale)
    g.defaultFont = bitmapFont
    // Setup Text Labels
    g.textLabel = g.NewLabel(bitmapFont)
    g.textLabel.SetBackground(mgl32.Vec4{0, 0, 0, 0.9})

    g.uiLabel = g.NewLabel(bitmapFont)
    g.uiLabel.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
    g.uiLabel.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.74})

    // Load Textures
    g.uiAtlas = eassets.LoadEmbeddedUIAtlas()

    g.entityMenu = g.NewList(g.uiAtlas, bitmapFont)
    g.entityMenu.SetTextCentered(true)
    g.entityMenu.SetKeyHandler(func(selectedIndex int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
        if key == glfw.KeyE {
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
    skybox := quads.LoadCubemap([6]string{
        filepath.Join("assets", "skybox", skyboxDir, "right.png"),
        filepath.Join("assets", "skybox", skyboxDir, "left.png"),
        filepath.Join("assets", "skybox", skyboxDir, "top.png"),
        filepath.Join("assets", "skybox", skyboxDir, "bottom.png"),
        filepath.Join("assets", "skybox", skyboxDir, "front.png"),
        filepath.Join("assets", "skybox", skyboxDir, "back.png"),
    })
    g.SetupSkybox(skybox)

    // Setup HUD

    g.crossHair = g.Gui.Add(g.uiAtlas, 4, int(0), int(0))

    g.uiRectSpriteTL = g.Gui.Add(g.uiAtlas, 0, 0, 0)
    g.uiRectSpriteTR = g.Gui.Add(g.uiAtlas, 1, 0, 0)
    g.uiRectSpriteBR = g.Gui.Add(g.uiAtlas, 2, 0, 0)
    g.uiRectSpriteBL = g.Gui.Add(g.uiAtlas, 3, 0, 0)
    g.uiRectSpriteTL.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
    g.uiRectSpriteTR.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
    g.uiRectSpriteBL.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
    g.uiRectSpriteBR.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})

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

    events.Subscribe(events.EventPlayerWeaponFired, func(id events.EventID, event events.Event) {
        origin, direction := g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
        g.fireWeapon(origin, direction, g.currentWeapon.WeaponEffect)
    })
    events.Subscribe(events.EventHitScanCollision, g.hitScanCollision)
    events.Subscribe(events.EventPhysicsCollision, g.physicsCollision)

    smallGun := g.loadSmallGun()
    g.weaponQuad = g.Gui.Add(smallGun.Texture, 0, 0, 0)
    g.switchWeapon(smallGun)

    g.HandleResize(g.WindowWidth, g.WindowHeight)
}

func (g *Game) mapLoad(mapLoad core.MapLoadResult, hotReload bool) {
    g.Sprites.Clear()
    g.Meshes.Clear()

    // Load Map
    cubeMap, entitySpawns := g.ToVoxelMap(mapLoad)
    g.cubeMap = cubeMap
    /*
       sphere := physics.NewSphericalHull(5)
       bulletSDK := bullet.BulletNewPhysicsSDK()
       world := bullet.BulletNewDynamicWorld(bulletSDK)
       bullet.BulletAddRigidBody(world, sphere.BulletHandle())
       bullet.BulletStepSimulation(world, 0.1)
    */
    // Setup Collision Solver
    g.physicsSystem = quads.NewCollisionSolver(g.Camera, cubeMap.IsSolidBlockAt)
    g.SetupPathfinder(cubeMap.IsSolidBlockAt)

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
    g.fogStart = 90
    g.fogEnd = 98
    g.updateFog()
    g.SetLight(mgl32.Vec3{2, 2, 4}, mgl32.Vec3{1, float32(214) / float32(255), float32(110) / float32(255)})

    //scientist.Sprite().PlayAnimationLooping(int(quads.AnimWalk))
    g.SetupMinimap(mapLoad.MapDimensions[0], mapLoad.MapDimensions[2], g.colorForMinimap)

}

func (g *Game) HandleResize(windowWidth, windowHeight int) {
    if len(g.widgetStack) > 0 {
        g.focusedWidget().HandleResize(windowWidth, windowHeight, g.UIScale)
    }

    screenCenter := mgl32.Vec2{float32(windowWidth) / 2, float32(windowHeight) / 2}
    listWidth := windowWidth / 3
    listHeight := min(windowHeight/2, 160)

    // center in the right side of the screen
    listX := float32(windowWidth)/2 + (float32(windowWidth) / 4) - (float32(listWidth) / 2)
    g.entityMenu.SetOrigin(listX, screenCenter.Y())
    g.entityMenu.SetBounds(float32(listWidth), float32(listHeight))

    tileW, tileH := g.uiAtlas.GetAtlasItemSize()
    centerX := (float32(windowWidth) / 2) - (float32(tileW) / 2)
    centerY := (float32(windowHeight) / 2) - (float32(tileH) / 2)

    g.crossHair.SetPosition(mgl32.Vec2{centerX, centerY})

    doubleW := windowWidth / 3
    doubleH := doubleW
    weaponX := (float32(windowWidth) / 2) - (float32(doubleW) / 2) - 12
    weaponY := float32(windowHeight) - float32(doubleH)

    g.weaponQuad.SetSize(doubleW, doubleH)
    g.weaponQuad.SetPosition(mgl32.Vec2{weaponX, weaponY})
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

    if g.shouldDrawDebugInfo {
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
    if g.lastHitResult != nil {
        rayCastPosString = vec3String(g.lastHitResult.WorldPosition())
    }

    spriteTrisDrawn, spriteDrawCalls := g.Sprites.DrawnLastFrame()
    guiTrisDrawn, guiDrawCalls := g.Gui.DrawnLastFrame()
    meshTriangles, meshDrawCalls := g.Meshes.DrawnLastFrame()
    rows := []core.TableRow{
        core.NewTableRow("Cam Fwd", "", vec3String(forward)),
        core.NewTableRow("Cam Pos", "", vec3String(pos)),
        core.NewTableRow("Ray Hit", "", rayCastPosString),
        core.NewTableRow("Coll. Pairs", "", fmt.Sprintf("%d", g.physicsSystem.PairsChecked())),
        core.NewTableRow("Map", fmt.Sprintf("%d tris", g.cubeMap.TriangleDrawCount()), fmt.Sprintf("%d", g.cubeMap.DrawCalls())),
        core.NewTableRow("GUI", fmt.Sprintf("%d tris", guiTrisDrawn), fmt.Sprintf("%d", guiDrawCalls)),
        core.NewTableRow("Sprites", fmt.Sprintf("%d tris", spriteTrisDrawn), fmt.Sprintf("%d", spriteDrawCalls)),
        core.NewTableRow("Meshes", fmt.Sprintf("%d tris", meshTriangles), fmt.Sprintf("%d", meshDrawCalls)),
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

func (g *Game) Draw(elapsed float64) {
    g.cubeMap.Draw(g.Camera)

    gl.ActiveTexture(gl.TEXTURE0)

    if g.UnitCube != nil {
        g.UnitCube.DrawAllQueued()
    }

    if g.shouldDrawMeshes {
        g.Meshes.Draw(g.Camera, elapsed)
    }

    if g.shouldDrawSprites {
        g.Sprites.Draw(g.Camera, elapsed)
    }
    if g.shouldDrawParticles {
        g.DrawParticles(elapsed)
    }
    if g.shouldDrawAABBs {
        g.DrawAABBs()
    }
    if g.shouldDrawChunkAABBs {
        g.DrawChunkAABBs(g.cubeMap)
    }
    if g.shouldDrawOBBs {
        g.DrawOBBs()
    }
    if g.shouldDrawPaths {
        g.DrawPaths()
    }
}
func (g *Game) DrawGui(elapsed float64) {
    if g.textLabel != nil && g.shouldDrawDebugInfo {
        g.textLabel.Draw()
    }

    if g.uiLabel != nil && g.hoveredObj() != nil {
        g.uiLabel.Draw()

        if g.entityMenu != nil {
            g.entityMenu.Draw()
        }
    }

    for _, widget := range g.widgetStack {
        widget.Draw()
    }
}

func (g *Game) startWeaponFireAnimation() {
    now := glfw.GetTime()
    if g.currentWeapon.DelayBetweenShots > 0 && now-g.currentWeapon.LastShotTime < g.currentWeapon.DelayBetweenShots {
        return
    }
    g.weaponQuad.PlayAnimation(g.currentWeapon.Animation)
    g.currentWeapon.LastShotTime = now
}

func (g *Game) hoveredObj() *quads.GameObject {
    if g.lastHitResult == nil {
        return nil
    }

    if g.lastHitResult.HitInfo.Distance > 10 || g.lastHitResult.HitObject == nil {
        return nil
    }

    return g.lastHitResult.HitObject
}

func (g *Game) updateGUI(elapsed float64) {
    if g.widgetsAreBlockingMovement {
        return
    }
    g.lastHitResult = nil

    g.uiRectSpriteTL.Hide()
    g.uiRectSpriteTR.Hide()
    g.uiRectSpriteBL.Hide()
    g.uiRectSpriteBR.Hide()
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

    g.uiRectSpriteTL.Show()
    g.uiRectSpriteTR.Show()
    g.uiRectSpriteBL.Show()
    g.uiRectSpriteBR.Show()
    g.uiLabel.Show()
    g.entityMenu.Show()

    tileW, _ := g.uiAtlas.GetAtlasItemSize()
    aabb := hoveredObj.Physics().AABBAt(mgl32.Vec3{})
    var transform mgl32.Mat4
    if hoveredObj.HasComponent(quads.ComponentTypeSprite) {
        transform = hoveredObj.Sprite().Transform(g.Camera)
    } else if hoveredObj.HasComponent(quads.ComponentTypeMesh) {
        transform = hoveredObj.Transform().GetTransformMatrix()
    }

    topLeft, topRight, bottomLeft, bottomRight := quads.HudCornersFromOBB(aabb, transform, g.Camera.ProjectWorldToScreenPixels)

    topRight[0] -= tileW
    bottomLeft[1] -= tileW

    bottomRight[0] -= tileW
    bottomRight[1] -= tileW

    g.uiRectSpriteTL.SetPosition(mgl32.Vec2{float32(topLeft[0]), float32(topLeft[1])})
    g.uiRectSpriteTR.SetPosition(mgl32.Vec2{float32(topRight[0]), float32(topRight[1])})
    g.uiRectSpriteBL.SetPosition(mgl32.Vec2{float32(bottomLeft[0]), float32(bottomLeft[1])})
    g.uiRectSpriteBR.SetPosition(mgl32.Vec2{float32(bottomRight[0]), float32(bottomRight[1])})

    if g.uiLabel != nil {
        hitZone := "body"
        if g.relativeHitPos.Y() > 0.6 && g.relativeHitPos.Y() < 0.8 {
            hitZone = "head"
        }
        labelText := fmt.Sprintf("%s (%s)", hoveredObj.DisplayName, hitZone)
        g.uiLabel.SetText(labelText)
        _, fontH := g.uiLabel.Size()
        g.uiLabel.SetPosition(mgl32.Vec2{float32(topLeft[0]), float32(topLeft[1] - fontH)})
    }
}

type RaycastResult struct {
    HitObject *quads.GameObject
    WorldPos  mgl32.Vec3
    Relative  mgl32.Vec3
    HitInfo   voxel.HitInfo3D
}

func (r RaycastResult) WorldPosition() mgl32.Vec3 {
    if r.HitObject == nil {
        return r.HitInfo.CollisionWorldPosition
    }
    return r.WorldPos
}
func (r RaycastResult) GroundPosition() (mgl32.Vec3, bool) {
    if r.HitObject != nil {
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
func (g *Game) rayCast(origin, direction mgl32.Vec3) RaycastResult {
    var hitInfo voxel.HitInfo3D
    var hitObject *quads.GameObject
    var worldPos mgl32.Vec3
    var relativePos mgl32.Vec3
    hitInfo = voxel.DDARaycast(origin, direction, func(x, y, z int32) (stopRay bool) {
        if !g.cubeMap.Contains(int32(x), int32(y), int32(z)) {
            return true
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
        return g.cubeMap.IsSolidBlockAt(x, y, z)
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
    g.shouldDrawDebugInfo = false
    g.SetMouseCapture(false)

    g.widgetsAreBlockingMovement = true
    atlasPicker := g.OpenTextureCube(g.uiAtlas, g.defaultFont, g.AtlasMap, g.voxelBlock.Textures)
    atlasPicker.HandleResize(g.WindowWidth, g.WindowHeight, g.UIScale)

    atlasPicker.SetOnClose(func() {
        g.SetMouseCapture(true)
    })
    g.replaceWidgetStack(atlasPicker)
}

func (g *Game) pickTexture(atlas *glhf.Texture, onSelected func(selectedIndex int)) {
    previousMouseState := g.setupModal()
    g.widgetsAreBlockingMovement = true
    atlasPicker := g.OpenAtlasPicker(g.uiAtlas, g.defaultFont, atlas)
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
    configMenu := g.OpenConfigMenu(g.uiAtlas, g.defaultFont)

    options := []quads.GuiOption{
        quads.NewGuiOptionAction("Continue", g.popFromWidgetStack),
        quads.NewGuiOptionAction("Quit", g.quitGame),
    }
    configMenu.SetOptions(options)
    configMenu.HandleResize(g.WindowWidth, g.WindowHeight, g.UIScale)
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
    configMenu := g.OpenConfigMenu(g.uiAtlas, g.defaultFont)
    vModes := g.GetVideoModes()
    var resolutionOptions []string
    for _, vMode := range vModes {
        resolutionOptions = append(resolutionOptions, fmt.Sprintf("%dx%d", vMode.Width, vMode.Height))
    }
    options := []quads.GuiOption{
        quads.NewGuiOptionBool("Draw Debug Info", &g.shouldDrawDebugInfo),
        quads.NewGuiOptionAction("Toggle Voxel Renderer", func() {
            core.BigDebugSwitch = !core.BigDebugSwitch
            //g.SetCubeShaderDebugSwitch(core.BigDebugSwitch)
            //g.cubeMap.ForceReupload()
        }),
        quads.NewGuiOptionAction("Fullscreen", g.toggleFullscreen),
        quads.NewGuiOptionAction("VSync", g.toggleVsync),

        quads.NewGuiOptionList("Window", resolutionOptions, &g.currentResolutionIndex, g.setWindowMode),
        quads.NewGuiOptionBool("Draw Chunk AABBs", &g.shouldDrawChunkAABBs),
        quads.NewGuiOptionBool("Draw AABBs", &g.shouldDrawAABBs),
        quads.NewGuiOptionBool("Draw OBBs", &g.shouldDrawOBBs),
        quads.NewGuiOptionBool("Draw Paths", &g.shouldDrawPaths),
        quads.NewGuiOptionBool("Draw Sprites", &g.shouldDrawSprites),
        quads.NewGuiOptionBool("Draw Meshes", &g.shouldDrawMeshes),
        quads.NewGuiOptionBool("Draw Skybox", &g.ShouldDrawSkybox),
        quads.NewGuiOptionBool("Draw Minimap", &g.ShouldDrawMinimap),
        quads.NewGuiOptionBool("Draw Particles", &g.shouldDrawParticles),
        quads.NewGuiOptionFloat("Fog Start", &g.fogStart).StepSize(5).ActivateFunc(g.updateFog),
        quads.NewGuiOptionFloat("Fog End", &g.fogEnd).StepSize(5).ActivateFunc(g.updateFog),
    }

    if !g.watchFileChanges {
        options = append(options, quads.NewGuiOptionAction("Start Map Hot Reload", func() {
            g.popFromWidgetStack()
            g.CreateToast("Watching for map changes", g.defaultFont, g.uiAtlas)
            g.watchFileChanges = true
            go g.watchFileChangesLoop()
        }))
    }
    configMenu.SetOptions(options)
    configMenu.HandleResize(g.WindowWidth, g.WindowHeight, g.UIScale)
    configMenu.SetOnClose(func() {
        g.captureMouse(previousMouseState)
        g.popFromWidgetStack()
    })
    g.pushToWidgetStack(configMenu)
}
func (g *Game) setWindowMode(index int) {
    g.shouldRenderFullscreen = false
    vMode := g.GetVideoModes()[index]
    g.SetWindowSize(vMode)
}
func (g *Game) updateFog() {
    toastMessage := fmt.Sprintf("Fog: %.0f - %.0f", g.fogStart, g.fogEnd)
    g.Toast(toastMessage)
    g.SetFog(g.CeilingColor.Vec3(), g.fogStart, g.fogEnd)
}

func (g *Game) Toast(toastMessage string) *quads.GuiToast {
    return g.CreateToast(toastMessage, g.defaultFont, g.uiAtlas)
}

func (g *Game) toggleFullscreen() {
    if g.shouldRenderFullscreen {
        g.SetWindowSize(g.GetVideoModes()[g.currentResolutionIndex])
    } else {
        g.SetFullscreen()
    }
    g.shouldRenderFullscreen = !g.shouldRenderFullscreen
}

func (g *Game) toggleVsync() {
    g.shouldUseVsync = !g.shouldUseVsync
    g.SetVSync(g.shouldUseVsync)
    g.Toast(fmt.Sprintf("VSync: %t", g.shouldUseVsync))
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
        if g.focusedWidget().HandleDrop(names, screenDropX, screenDropY) {
            return
        }
    }

    if len(names) == 1 {

        rayHit := g.rayCastFromScreenPos(screenDropX, screenDropY)
        worldPos, hitGround := rayHit.GroundPosition()
        if hitGround {
            g.spawnDecorationFromFile(worldPos, names[0])
        }

        return
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
               loadedText := quads.LoadTexture(name)
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
    blockAt := g.cubeMap.GetGlobalBlock(int32(x), int32(playerHeight), int32(y))
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
