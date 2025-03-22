package quads

import (
    "fmt"
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/quads/voxel"
    "math"
    "os"
    "path/filepath"
    "unsafe"
)

type GlApplication struct {
    Window             *glfw.Window
    TerminateFunc      func()
    UpdateFunc         func(elapsed float64)
    DrawFunc           func(elapsed float64)
    DrawGuiFunc        func(elapsed float64)
    KeyHandler         func(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)
    MousePosHandler    func(xpos float64, ypos float64)
    MouseButtonHandler func(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)
    ScrollHandler      func(xoff float64, yoff float64)
    ResizeHandler      func(width int, height int)
    DropHandler        func(names []string, x, y float64)
    Ticks              uint64
    FramesPerSecond    float64
    FPSRunningAvg      float64
    FPSMin             float64
    FPSMax             float64
    TimeFactor         float64
    CeilingColor       mgl32.Vec4

    Camera       *core.FPSCamera
    WindowWidth  int
    WindowHeight int
    Sprites      *SpriteDrawer
    Gui          *GuiDrawer
    Meshes       *MeshDrawer
    Pathfinder   *PathfindingSystem
    UnitCube     *UnitCube

    particleSystems map[ParticleName]*glhf.ParticleSystem

    queuedToasts []*GuiToast

    guiShader     *glhf.Shader
    quadShader    *glhf.Shader
    meshShader    *glhf.Shader
    cubeShader    *glhf.Shader
    lineShader    *glhf.Shader
    skyboxShader  *glhf.Shader
    skybox        *glhf.VertexSlice[glhf.GlFloat]
    skyboxCubeMap *glhf.Cubemap
    mainUbo       *glhf.UniformBufferObject

    frameBuffer            *glhf.Frame
    UIScale                mgl32.Vec2
    ScreenCoordScale       mgl32.Vec2
    drawCallCountParticles int
    drawCallCountMeshes    int
    drawCallCountSprites   int
    drawCallCountVoxels    int
    drawCallCountGui       int

    minimap *Minimap

    ShouldDrawSkybox  bool
    ShouldDrawMinimap bool
}

func (a *GlApplication) SetMouseCapture(isCaptured bool) {
    if isCaptured {
        a.Window.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
    } else {
        a.Window.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
    }
}

func (a *GlApplication) MouseButtonCallback(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
    if a.MouseButtonHandler != nil {
        x, y := w.GetCursorPos()
        a.MouseButtonHandler(x, y, button, action, mods)
    }
}

func (a *GlApplication) DropCallback(w *glfw.Window, names []string) {
    if a.DropHandler != nil {
        x, y := w.GetCursorPos()
        glhf.CallNonBlock(func() {
            a.DropHandler(names, x, y)
        })

    }
}

func (a *GlApplication) KeyCallback(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
    if a.KeyHandler != nil {
        a.KeyHandler(
            key,
            scancode,
            action,
            mods,
        )
    }
}
func (a *GlApplication) MousePosCallback(w *glfw.Window, xpos float64, ypos float64) {
    if a.MousePosHandler != nil {
        a.MousePosHandler(xpos, ypos)
    }
}

func (a *GlApplication) ScrollCallback(w *glfw.Window, xoff float64, yoff float64) {
    if a.ScrollHandler != nil {
        a.ScrollHandler(xoff, yoff)
    }
}

func (a *GlApplication) SetupSkybox(cubemap *glhf.Cubemap) {
    a.skyboxCubeMap = cubemap
    a.ShouldDrawSkybox = true
}

func (a *GlApplication) CreateToast(labelText string, font BitmapFont, uiAtlas *glhf.Texture) *GuiToast {
    label := a.NewLabel(font)
    label.SetPosition(mgl32.Vec2{10, 180})
    label.SetText(labelText)
    toast := NewGuiToast(label, a.guiShader, uiAtlas)
    a.queuedToasts = append(a.queuedToasts, toast)
    return toast
}

func (a *GlApplication) SetupUnitCubes() {
    meshFile := filepath.Join("assets", "meshes", "cube.glb")
    mesh := LoadGLTFWithTextures(meshFile)
    mesh.UploadVertexData(a.meshShader)
    a.UnitCube = NewUnitCube(a.meshShader, mesh)
}

func (a *GlApplication) SetupParticleSystem(name ParticleName, count int) {
    ps := glhf.NewParticleSystem(count, a.Camera.GetViewMatrix, a.Camera.GetProjectionMatrix)
    a.particleSystems[name] = ps
}
func (a *GlApplication) SetupPathfinder(isSolid func(x, y, z int32) bool) {
    a.Pathfinder = NewPathfindingSystem(isSolid)
}
func (a *GlApplication) SetupMinimap(width, height int, getColor func(x, y int) [4]uint8) {
    scale := float32(4)
    emptyMap := make([]uint8, width*height*4)
    minimapTexture := glhf.NewTexture(width, height, false, emptyMap)
    minimapTexture.SetAtlasItemSize(width, height)
    guiQuadVerts := NewGUIQuadVertsFromSubImage(a.guiShader, minimapTexture, 0, 0, 0, width, height)

    a.minimap = &Minimap{
        Width:    width,
        Height:   height,
        GetColor: getColor,
        Texture:  minimapTexture,
        Pixels:   emptyMap,
        Scale:    scale,
        GuiQuad:  guiQuadVerts,
    }
    a.minimap.Resize(a.WindowWidth, a.WindowHeight)
    a.ShouldDrawMinimap = true
}

func (a *GlApplication) EmitParticlesByName(name ParticleName, pos mgl32.Vec3, count int) {
    ps, ok := a.particleSystems[name]
    if !ok {
        return
    }

    props := Particles[name].WithOrigin(pos)
    ps.Emit(props, count)
}
func (a *GlApplication) EmitParticles(usedSystem ParticleName, props glhf.ParticleProperties, count int) {
    ps, ok := a.particleSystems[usedSystem]
    if !ok {
        return
    }
    ps.Emit(props, count)
}

func (a *GlApplication) GetVideoModes() []*glfw.VidMode {
    monitor := glfw.GetPrimaryMonitor()
    return monitor.GetVideoModes()
}

func (a *GlApplication) SetWindowSize(vMode *glfw.VidMode) {
    a.Window.SetMonitor(nil, 0, 0, vMode.Width, vMode.Height, vMode.RefreshRate)
    a.OnResize(a.Window.GetSize())
}

func (a *GlApplication) SetFullscreen() {
    monitor := glfw.GetPrimaryMonitor()
    videoMode := monitor.GetVideoMode()
    a.Window.SetMonitor(monitor, 0, 0, videoMode.Width, videoMode.Height, videoMode.RefreshRate)
    a.OnResize(a.Window.GetSize())
}

func (a *GlApplication) SetCubeShaderDebugSwitch(debugValue bool) {
    val := 0.0
    if debugValue {
        val = 1.0
    }
    a.cubeShader.Begin()
    a.cubeShader.SetUniformAttr(CubeShaderDebugSwitch, float32(val))
    a.cubeShader.End()
}

func (a *GlApplication) DrawAABBs() {
    a.lineShader.Begin()
    a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
    a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{0.8, 2, 0.8})

    a.Sprites.DrawAABBs(a.lineShader, a.Camera)
    a.Meshes.DrawAABBs(a.lineShader)

    a.lineShader.End()
}

func (a *GlApplication) DrawChunkAABBs(cubeMap *voxel.Map) {
    a.lineShader.Begin()
    a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
    a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{0.8, 2, 0.8})

    cubeMap.DrawAABBs(a.lineShader)

    a.lineShader.End()
}
func (a *GlApplication) DrawOBBs() {
    a.lineShader.Begin()
    a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
    a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{1, 0.498, 0.259})

    a.Sprites.DrawOBBs(a.lineShader, a.Camera)
    a.Meshes.DrawOBBs(a.lineShader)

    a.lineShader.End()
}
func (a *GlApplication) DrawPaths() {
    a.lineShader.Begin()
    a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())

    a.Pathfinder.DrawPathfindingDebug(a.lineShader)
    a.lineShader.End()
}

func (a *GlApplication) DrawParticles(elapsed float64) {
    a.drawCallCountParticles = 0
    for _, ps := range a.particleSystems {
        ps.Draw(elapsed)
        a.drawCallCountParticles += 2
    }
}
func (a *GlApplication) drawSkybox(cubemap *glhf.Cubemap) {
    gl.DepthMask(false)
    a.skyboxShader.Begin()

    projection := a.Camera.GetProjectionMatrix()
    view := a.Camera.GetViewMatrix().Mat3().Mat4() // remove translation
    projView := projection.Mul4(view)

    a.skyboxShader.SetUniformAttr(SkyboxShaderCamProjectionView, projView)
    a.skybox.Begin()
    cubemap.Begin()
    a.skybox.Draw()
    cubemap.End()
    a.skybox.End()
    a.skyboxShader.End()
    gl.DepthMask(true)
}

func (a *GlApplication) NewLabel(font BitmapFont) *BitmapFontMesh {
    return NewBitmapFontMesh(a.guiShader, font)
}

func (a *GlApplication) NewList(uiAtlas *glhf.Texture, font BitmapFont) *GuiList {
    return NewGuiList(a.guiShader, uiAtlas, font)
}
func (a *GlApplication) OpenTextureCube(uiAtlas *glhf.Texture, font BitmapFont, atlasForSelection *glhf.Texture, preselection map[voxel.CubeSide]uint16) *GuiTexturePickerCube {
    return NewGuiTexturePickerCube(a.guiShader, uiAtlas, font, atlasForSelection, preselection)
}
func (a *GlApplication) OpenConfigMenu(uiAtlas *glhf.Texture, font BitmapFont) *GuiConfig {
    return NewGuiConfig(a.guiShader, uiAtlas, font)
}
func (a *GlApplication) SetFog(colour mgl32.Vec3, near, far float32) {
    a.mainUbo.Begin()
    a.mainUbo.SetAttribute(UboFogColor, colour.Vec4(1))
    a.mainUbo.SetAttribute(UboFogRange, mgl32.Vec2{near, far})
    a.mainUbo.End()
}

func (a *GlApplication) SetLight(pos, colour mgl32.Vec3) {
    a.mainUbo.Begin()
    a.mainUbo.SetAttribute(UboCamLightColor, colour.Vec4(1))
    a.mainUbo.End()
}

func (a *GlApplication) SetVSync(enabled bool) {
    if enabled {
        glfw.SwapInterval(1)
    } else {
        glfw.SwapInterval(0)
    }
}

func (a *GlApplication) StartEventLoop() {
    defer glhf.Call(a.TerminateFunc)
    // Start Render Loop
    shouldQuit := false

    previousTime := glfw.GetTime()
    time := glfw.GetTime()

    for !shouldQuit {
        glhf.Call(func() {
            if a.Window.ShouldClose() {
                shouldQuit = true
            }

            // Clear the window.
            //gl.ClearColor(0, 0, 0, 1)
            gl.ClearColor(a.CeilingColor.X(), a.CeilingColor.Y(), a.CeilingColor.Z(), a.CeilingColor.W())
            gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

            // Update
            time = glfw.GetTime()
            elapsed := time - previousTime
            previousTime = time
            deltaTime := elapsed * a.TimeFactor

            a.updateToasts(deltaTime)

            if a.Pathfinder != nil {
                a.Pathfinder.Update(elapsed)
            }
            if a.minimap != nil {
                a.minimap.Update()
            }
            a.UpdateFunc(deltaTime)

            a.updateUBO()

            // DRAW BEGIN

            /* Framebuffer is doing depth testing right now..
               a.frameBuffer.Begin()

               buffers := []uint32{gl.COLOR_ATTACHMENT0}
               gl.DrawBuffers(1, &buffers[0])
            */
            a.DrawFunc(deltaTime)

            if a.skyboxCubeMap != nil && a.ShouldDrawSkybox {
                a.drawSkybox(a.skyboxCubeMap)
            }

            a.guiShader.Begin()
            gl.Disable(gl.DEPTH_TEST)
            a.Gui.Draw(deltaTime)
            a.guiShader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
            a.DrawGuiFunc(deltaTime)
            gl.Enable(gl.DEPTH_TEST)

            if a.minimap != nil && a.ShouldDrawMinimap {
                a.minimap.Draw(a.guiShader)
            }

            a.drawToasts()
            a.guiShader.End()

            /*
               a.frameBuffer.End()

               a.frameBuffer.Blit(nil, 0, 0, a.WindowWidth*2, a.WindowHeight*2, 0, 0, a.WindowWidth*2, a.WindowHeight*2)
            */
            // DRAW END

            a.FramesPerSecond = 1.0 / elapsed
            if a.Ticks%60 == 0 {
                sixtyTicksAverage := a.FPSRunningAvg
                a.Window.SetTitle(fmt.Sprintf("FPS: %.0f (Avg: %.0f, Min: %.0f, Max: %.0f) / Elapsed: %.3f", a.FramesPerSecond, sixtyTicksAverage, a.FPSMin, a.FPSMax, elapsed*1000))
                a.FPSRunningAvg = 0 + a.FramesPerSecond*(1.0/60.0)
                a.FPSMin = math.MaxFloat64
                a.FPSMax = 0
            } else {
                a.FPSRunningAvg = a.FPSRunningAvg + a.FramesPerSecond*(1.0/60.0)
                if a.FramesPerSecond < a.FPSMin {
                    a.FPSMin = a.FramesPerSecond
                }
                if a.FramesPerSecond > a.FPSMax {
                    a.FPSMax = a.FramesPerSecond
                }
            }

            a.Window.SwapBuffers()
            glfw.PollEvents()
            a.Ticks++
        })
    }
}
func mat4ptr(mat mgl32.Mat4) unsafe.Pointer {
    return unsafe.Pointer(&mat[0])
}

func vec4ptr(vec mgl32.Vec4) unsafe.Pointer {
    return unsafe.Pointer(&vec[0])
}

func (a *GlApplication) updateUBO() {

    a.mainUbo.Begin()
    a.mainUbo.SetAttribute(UboCamPosition, a.Camera.GetPosition().Vec4(1))
    a.mainUbo.SetAttribute(UboCamDirection, a.Camera.GetForward().Vec4(0))
    a.mainUbo.SetAttribute(UboCamProjectionView, a.Camera.GetProjectionViewMatrix())
    a.mainUbo.End()
}

func (a *GlApplication) ToVoxelMap(loadResult core.MapLoadResult, atlas *glhf.Texture) (*voxel.Map, map[mgl32.Vec3]core.MapEntity) {
    return toVoxelMap(loadResult, atlas, a.cubeShader)
}
func (a *GlApplication) OnResize(width int, height int) {
    a.WindowWidth = width
    a.WindowHeight = height
    a.Camera.SetScreenSize(width, height)
    projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(width, height)
    a.guiShader.Begin()
    a.guiShader.SetUniformAttr(GuiShaderProjection, projectionMatrix)
    a.guiShader.End()
    if a.minimap != nil {
        a.minimap.Resize(width, height)
    }
    a.ResizeHandler(width, height)
}

func (a *GlApplication) SetCursor(cursorImageFile string, hotX, hotY int) {
    img := LoadImage(cursorImageFile)
    if img != nil {
        a.Window.SetCursor(glfw.CreateCursor(img, hotX, hotY))
    }
}

func (a *GlApplication) updateToasts(deltaTime float64) {
    if len(a.queuedToasts) == 0 {
        return
    }
    first := a.queuedToasts[0]
    first.Update(deltaTime)
    if first.ShouldClose() {
        a.queuedToasts = a.queuedToasts[1:]
    }
}

func (a *GlApplication) drawToasts() {
    if len(a.queuedToasts) == 0 {
        return
    }

    first := a.queuedToasts[0]
    first.Draw()
}

func InitOpenGLWindow(title string, width, height int, fullScreen bool) (*glfw.Window, func()) {
    var win *glfw.Window
    glErr := glfw.Init()
    if glErr != nil {
        println("glfw: ", glErr.Error())
        panic(glErr)
        return nil, nil
    }
    glfw.WindowHint(glfw.ContextVersionMajor, 3)
    glfw.WindowHint(glfw.ContextVersionMinor, 3)
    glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
    glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
    glfw.WindowHint(glfw.Resizable, glfw.False)
    glfw.WindowHint(glfw.OpenGLDebugContext, glfw.True)
    glfw.WindowHint(glfw.Visible, glfw.True)
    glfw.WindowHint(glfw.CocoaRetinaFramebuffer, glfw.True)
    glfw.WindowHint(glfw.DoubleBuffer, glfw.True)
    glfw.WindowHint(glfw.Decorated, glfw.True)

    var err error
    var monitor *glfw.Monitor
    if fullScreen {
        monitor = glfw.GetPrimaryMonitor()
        videoMode := monitor.GetVideoMode()
        width, height = videoMode.Width, videoMode.Height
    }

    win, err = glfw.CreateWindow(width, height, title, monitor, nil)
    if err != nil {
        panic(err)
    }
    win.SetFramebufferSizeCallback(func(w *glfw.Window, width int, height int) {
        gl.Viewport(0, 0, int32(width), int32(height))
        gl.Scissor(0, 0, int32(width), int32(height))
        gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
    })
    win.MakeContextCurrent()

    glfw.SwapInterval(1) // enable (1) vsync

    // Init initializes OpenGL by loading function pointers from the active OpenGL context.
    // This function must be manually run inside the main thread (using "github.com/faiface/mainthread"
    // package).
    //
    // It must be called under the presence of an active OpenGL context, e.g., always after calling
    // window.MakeContextCurrent(). Also, always call this function when switching contexts.
    {
        initErr := gl.Init()
        if initErr != nil {
            panic(initErr)
        }
        gl.Enable(gl.BLEND)
        gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
        gl.Enable(gl.SCISSOR_TEST)
        gl.BlendEquation(gl.FUNC_ADD)
    }

    version := gl.GoStr(gl.GetString(gl.VERSION))
    fmt.Println("OpenGL version", version)

    gl.DepthFunc(gl.LEQUAL)
    gl.Enable(gl.DEPTH_TEST)

    gl.Enable(gl.CULL_FACE)
    gl.CullFace(gl.BACK)

    /*
    	if runtime.GOOS != "darwin" {
    		gl.Enable(gl.DEBUG_OUTPUT)
    		gl.DebugMessageCallback(gl.DebugProc(glErrorHandler), gl.Ptr(nil))
    	}
    */

    return win, func() {
        glfw.DetachCurrentContext()
        glfw.Terminate()
    }
}

func glErrorHandler(source uint32, gltype uint32, id uint32, severity uint32, length int32, message string, param unsafe.Pointer) {
    errorMessage := fmt.Sprintf("source: %d, type: %d, id: %d, severity: %d, length: %d, param: %d, message:\n%s", source, gltype, id, severity, length, param, message)
    println(errorMessage)
}

func Get2DPixelCoordOrthographicProjectionMatrix(width, height int) mgl32.Mat4 {
    // we want 0,0 to be at the top left
    // left, right, bottom, top
    return mgl32.Ortho2D(0, float32(width), float32(height), 0)
}

type GlGame interface {
    Init(app *GlApplication, mapLoad core.MapLoadResult)
    Draw(elapsed float64)
    DrawGui(elapsed float64)
    Update(elapsed float64)
    HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)
    HandleMousePosEvents(xpos float64, ypos float64)
    HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)
    HandleScrollEvents(xoff float64, yoff float64)
    HandleResize(width int, height int)
    HandleDrop(names []string, x, y float64)
}

// Start starts the OpenGL application with the given parentApp and screen dimensions.
// Will block until the window is closed.
// Will call the parentApp's Init, Draw, DrawGui, Update, HandleKeyEvents, HandleMousePosEvents, HandleMouseButtonEvents, and HandleScrollEvents functions.
func Start(parentApp GlGame, mapLoad core.MapLoadResult, screenW, screenH int) {
    glAppUntyped := glhf.CallVal(func() any {
        mouseSensitivity := float32(0.1)

        window, terminateFunc := InitOpenGLWindow("fps", screenW, screenH, false)

        primaryMonitor := glfw.GetPrimaryMonitor()
        uiScaleX, uiScaleY := primaryMonitor.GetContentScale()
        println(fmt.Sprintf("UI scale: %.2f, %.2f", uiScaleX, uiScaleY))

        winSizeX, winSizeY := window.GetSize()
        fbSizeX, fbSizeY := window.GetFramebufferSize()

        screenCoordScaleX := float32(fbSizeX) / (float32(winSizeX) * uiScaleX)
        screenCoordScaleY := float32(fbSizeY) / (float32(winSizeY) * uiScaleY)
        println(fmt.Sprintf("Screen coord scale: %.2f, %.2f", screenCoordScaleX, screenCoordScaleY))

        fpsCamera := core.NewFPSCamera(mgl32.Vec3{0, 0, 0}, winSizeX, winSizeY, mouseSensitivity)
        fpsCamera.SetUIProjectionScale(mgl32.Vec2{uiScaleX, uiScaleY})
        fpsCamera.SetMouseInvertedY(true)

        maxTexSize := glQuery(gl.MAX_TEXTURE_SIZE)

        println(fmt.Sprintf("Max texture size: %dx%d", maxTexSize, maxTexSize))

        uboBlockName := "Main"
        ubo := mainUBO()

        shaderBlockSource := ubo.GetShaderBlockSource(uboBlockName)

        uiScale := mgl32.Vec2{uiScaleX, uiScaleY}
        glApp := &GlApplication{
            WindowWidth:     winSizeX,
            WindowHeight:    winSizeY,
            Window:          window,
            TerminateFunc:   terminateFunc,
            TimeFactor:      1.0,
            Camera:          fpsCamera,
            guiShader:       LoadGuiShader(winSizeX, winSizeY),
            quadShader:      LoadQuadShader(shaderBlockSource),
            cubeShader:      LoadCubeShader(shaderBlockSource),
            meshShader:      LoadMeshShader(shaderBlockSource),
            lineShader:      LoadLineShader(fpsCamera),
            skyboxShader:    LoadSkyboxShader(fpsCamera),
            particleSystems: make(map[ParticleName]*glhf.ParticleSystem),
            //frameBuffer:      glhf.NewFrame(fbSizeX, fbSizeY, false),
            UIScale:          uiScale,
            ScreenCoordScale: mgl32.Vec2{screenCoordScaleX, screenCoordScaleY},
        }
        window.SetKeyCallback(glApp.KeyCallback)
        window.SetCursorPosCallback(glApp.MousePosCallback)

        glApp.SetMouseCapture(true)

        window.SetMouseButtonCallback(glApp.MouseButtonCallback)
        window.SetScrollCallback(glApp.ScrollCallback)
        window.SetDropCallback(glApp.DropCallback)

        glApp.DrawFunc = parentApp.Draw
        glApp.DrawGuiFunc = parentApp.DrawGui
        glApp.UpdateFunc = parentApp.Update
        glApp.KeyHandler = parentApp.HandleKeyEvents
        glApp.MousePosHandler = parentApp.HandleMousePosEvents
        glApp.MouseButtonHandler = parentApp.HandleMouseButtonEvents
        glApp.ScrollHandler = parentApp.HandleScrollEvents
        glApp.ResizeHandler = parentApp.HandleResize
        glApp.DropHandler = parentApp.HandleDrop
        glApp.Sprites = NewSpriteDrawer(glApp.quadShader)
        glApp.Gui = NewGuiDrawer(glApp.guiShader)
        glApp.Meshes = NewMeshDrawer(glApp.meshShader)

        skyboxVA := glhf.MakeVertexSlice(glApp.skyboxShader, 36, 36)
        skyboxVA.Begin()
        skyboxVA.SetVertexData(skyboxVertices())
        skyboxVA.End()
        glApp.skybox = skyboxVA

        ubo.BindUniformBlockTo(glApp.cubeShader.ID(), uboBlockName, 0)
        ubo.BindUniformBlockTo(glApp.quadShader.ID(), uboBlockName, 0)
        ubo.BindUniformBlockTo(glApp.meshShader.ID(), uboBlockName, 0)

        ubo.Begin()

        ubo.BindUBOTo(0)

        //ubo.SetAttribute("fogRange", mgl32.Vec2{12, 99})
        ubo.SetAttribute(UboGlobalLightColor, mgl32.Vec4{0.5, 0.5, 0.5, 1})
        ubo.End()

        glApp.mainUbo = ubo

        parentApp.Init(glApp, mapLoad)

        return glApp
    })
    glApp := glAppUntyped.(*GlApplication)
    glApp.StartEventLoop()
}

func glQuery(value uint32) int32 {
    var queryResult int32
    gl.GetIntegerv(value, &queryResult)
    return int32(queryResult)
}

func FileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}
func skyboxVertices() []glhf.GlFloat {
    return []glhf.GlFloat{
        // positions          
        -1.0, 1.0, -1.0,
        -1.0, -1.0, -1.0,
        1.0, -1.0, -1.0,
        1.0, -1.0, -1.0,
        1.0, 1.0, -1.0,
        -1.0, 1.0, -1.0,

        -1.0, -1.0, 1.0,
        -1.0, -1.0, -1.0,
        -1.0, 1.0, -1.0,
        -1.0, 1.0, -1.0,
        -1.0, 1.0, 1.0,
        -1.0, -1.0, 1.0,

        1.0, -1.0, -1.0,
        1.0, -1.0, 1.0,
        1.0, 1.0, 1.0,
        1.0, 1.0, 1.0,
        1.0, 1.0, -1.0,
        1.0, -1.0, -1.0,

        -1.0, -1.0, 1.0,
        -1.0, 1.0, 1.0,
        1.0, 1.0, 1.0,
        1.0, 1.0, 1.0,
        1.0, -1.0, 1.0,
        -1.0, -1.0, 1.0,

        -1.0, 1.0, -1.0,
        1.0, 1.0, -1.0,
        1.0, 1.0, 1.0,
        1.0, 1.0, 1.0,
        -1.0, 1.0, 1.0,
        -1.0, 1.0, -1.0,

        -1.0, -1.0, -1.0,
        -1.0, -1.0, 1.0,
        1.0, -1.0, -1.0,
        1.0, -1.0, -1.0,
        -1.0, -1.0, 1.0,
        1.0, -1.0, 1.0,
    }
}

func toVoxelMap(loadResult core.MapLoadResult, atlas *glhf.Texture, shader *glhf.Shader) (*voxel.Map, map[mgl32.Vec3]core.MapEntity) {
    chunkSize := 32
    mapData := loadResult.MapData
    mapSizeX, mapSizeY, mapSizeZ := loadResult.MapDimensions[0], loadResult.MapDimensions[1], loadResult.MapDimensions[2]

    chunksNeededX := int32(math.Ceil(float64(mapSizeX) / float64(chunkSize)))
    chunksNeededY := int32(math.Ceil(float64(mapSizeY) / float64(chunkSize)))
    chunksNeededZ := int32(math.Ceil(float64(mapSizeZ) / float64(chunkSize)))

    voxelMap := voxel.NewMapWithEmptyChunks(chunksNeededX, chunksNeededY, chunksNeededZ, int32(chunkSize), int32(chunkSize))
    voxelMap.SetShader(shader)
    voxelMap.SetModelTransformAttrIndex(CubeShaderModelTransform)

    shader.Begin()
    shader.SetUniformAttr(CubeShaderAtlasGridSize, mgl32.Vec2{atlas.GridWidth(), atlas.GridHeight()})
    shader.End()

    voxelMap.SetTerrainTexture(atlas)

    entitySpawns := make(map[mgl32.Vec3]core.MapEntity)

    for z := 0; z < mapSizeZ; z++ {
        for x := 0; x < mapSizeX; x++ {
            mapIndex := z*mapSizeX + x
            tileInfo := mapData[mapIndex]

            floorStartsAt := tileInfo.FloorOffset
            ceilingStartsAt := mapSizeY - 1
            if tileInfo.CeilingHeight > 0 {
                ceilingStartsAt = tileInfo.CeilingHeight
            }

            wallStartsAt := 0
            wallEndsAt := 0

            if tileInfo.WallHeight > 0 {
                wallStartsAt = tileInfo.WallOffset + 1
                wallEndsAt = wallStartsAt + tileInfo.WallHeight
            }

            for y := 0; y < mapSizeY; y++ {
                // 0 = floor layer
                // 1 = ground layer
                // mapSizeY - 1 = ceiling layer
                cube := voxelMap.GetGlobalBlock(int32(x), int32(y), int32(z))
                isFloorLevel := y == floorStartsAt+1

                for _, entity := range tileInfo.Entities {
                    if isFloorLevel {
                        entitySpawnPos := mgl32.Vec3{float32(x) + 0.5, float32(y), float32(z) + 0.5}
                        entitySpawns[entitySpawnPos] = entity
                    }
                }
                isCeiling := y == ceilingStartsAt && loadResult.IsCeilingTextured
                isWall := y <= floorStartsAt || y >= wallStartsAt && y < wallEndsAt
                cube.IsSolid = isWall || isCeiling
                cube.Textures = make(map[voxel.CubeSide]uint16)
                cube.SetSunLight(14)
                cube.SetTorchLight(14)
                cube.Textures[voxel.North] = uint16(tileInfo.WallTex)
                cube.Textures[voxel.South] = uint16(tileInfo.WallTex)
                cube.Textures[voxel.East] = uint16(tileInfo.WallTex)
                cube.Textures[voxel.West] = uint16(tileInfo.WallTex)
                cube.Textures[voxel.Top] = uint16(tileInfo.FloorTex)
                cube.Textures[voxel.Bottom] = uint16(tileInfo.CeilTex)
            }
        }
    }
    return voxelMap, entitySpawns
}
