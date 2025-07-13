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
    "regexp"
    "strconv"
    "unsafe"
)

type GlApplication struct {
    Window             *glfw.Window
    TerminateFunc      func()
    UpdateFunc         func(elapsed float64)
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

    CubeMap *voxel.Map

    ShouldDrawAABBs        bool
    ShouldDrawChunkAABBs   bool
    ShouldDrawOBBs         bool
    ShouldDrawPaths        bool
    ShouldDrawMeshes       bool
    ShouldDrawSprites      bool
    ShouldDrawParticles    bool
    ShouldDrawDebugInfo    bool
    ShouldRenderFullscreen bool
    ShouldUseVsync         bool

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

    quadShader *glhf.Shader
    meshShader *glhf.Shader
    cubeShader *glhf.Shader

    quadShaderDepth *glhf.Shader
    cubeShaderDepth *glhf.Shader
    meshShaderDepth *glhf.Shader

    guiShader    *glhf.Shader
    postShader   *glhf.Shader
    lineShader   *glhf.Shader
    skyboxShader *glhf.Shader

    skybox        *glhf.VertexArray[glhf.GlFloat]
    skyboxCubeMap *glhf.Cubemap
    mainUbo       *glhf.UniformBufferObject
    lightsUbo     *LightsUBO

    colorBatch *glhf.SpriteBatch

    shadowBuffer           *glhf.Frame
    postFrameBuffer        *glhf.Frame
    screenQuad             *glhf.VertexArray[glhf.GlFloat]
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
    AtlasMap          *glhf.TextureAtlasFixed
    AtlasSprites      *glhf.TextureAtlasFixed

    UIAtlas *glhf.TextureAtlasFixed
    UIFont  BitmapFont

    spriteBatches    []*glhf.SpriteBatch
    shadowResolution int
}

// Start starts the OpenGL application with the given parentApp and screen dimensions.
// Will block until the window is closed.
// Will call the parentApp's Init, Update, HandleKeyEvents, HandleMousePosEvents, HandleMouseButtonEvents, and HandleScrollEvents functions.
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
        fpsCamera.SetMouseInvertedY(true)

        maxTexSize := glQuery(gl.MAX_TEXTURE_SIZE)
        println(fmt.Sprintf("Max texture size: %dx%d", maxTexSize, maxTexSize))

        maxTexLayers := glQuery(gl.MAX_ARRAY_TEXTURE_LAYERS)
        println(fmt.Sprintf("Max texture layers: %d", maxTexLayers))

        // the number of textures that can be accessed by the fragment shader
        maxTexImageUnits := glQuery(gl.MAX_TEXTURE_IMAGE_UNITS)
        println(fmt.Sprintf("Max texture image units: %d", maxTexImageUnits))

        // total number of textures that can be used
        maxCombinedTexImageUnits := glQuery(gl.MAX_COMBINED_TEXTURE_IMAGE_UNITS)
        println(fmt.Sprintf("Max combined texture image units: %d", maxCombinedTexImageUnits))

        mainUboBlockName := "Main"
        mainUbo := mainUBO()
        lightsUbo := NewLightsUBO()

        injectedGLSL := mainUbo.GetShaderBlockSource(mainUboBlockName)
        injectedGLSL += lightsGLSLBlock

        uiScale := mgl32.Vec2{uiScaleX, uiScaleY}

        // load the texture atlas
        // map pattern map_64x64.png
        mapAtlasRegex, _ := regexp.Compile("map_(\\d+)x(\\d+).png")
        spriteAtlasRegex, _ := regexp.Compile("sprites_(\\d+)x(\\d+).png")
        baseTexPath := filepath.Join("assets", "base")
        entries, err := os.ReadDir(baseTexPath)
        if err != nil {
            panic(err)
        }
        var atlasMap *glhf.TextureAtlasFixed
        var atlasSprites *glhf.TextureAtlasFixed
        for _, entry := range entries {
            if entry.IsDir() {
                continue
            }
            var width, height int
            if mapAtlasRegex.MatchString(entry.Name()) {
                match := mapAtlasRegex.FindStringSubmatch(entry.Name())
                width, _ = strconv.Atoi(match[1])
                height, _ = strconv.Atoi(match[2])
                atlasMap = LoadTextureAtlasFixed(filepath.Join(baseTexPath, entry.Name()), width, height)
            } else if spriteAtlasRegex.MatchString(entry.Name()) {
                match := spriteAtlasRegex.FindStringSubmatch(entry.Name())
                width, _ = strconv.Atoi(match[1])
                height, _ = strconv.Atoi(match[2])
                atlasSprites = LoadTextureAtlasFixed(filepath.Join(baseTexPath, entry.Name()), width, height)
            }

        }

        shadowResolution := 512
        glApp := &GlApplication{
            WindowWidth:      winSizeX,
            WindowHeight:     winSizeY,
            Window:           window,
            TerminateFunc:    terminateFunc,
            TimeFactor:       1.0,
            shadowResolution: shadowResolution,
            Camera:           fpsCamera,
            guiShader:        LoadGuiShader(winSizeX, winSizeY, mgl32.Vec2{1, 1}),
            postShader:       LoadPostShader(),

            quadShader: LoadQuadShader(injectedGLSL),
            cubeShader: LoadCubeShader(injectedGLSL),
            meshShader: LoadMeshShader(injectedGLSL),

            quadShaderDepth: LoadQuadShaderDepthPass(injectedGLSL),
            cubeShaderDepth: LoadCubeShaderDepthPass(injectedGLSL),
            meshShaderDepth: LoadMeshShaderDepthPass(injectedGLSL),

            shadowBuffer:    glhf.NewFrameBufferWithDepth(shadowResolution, shadowResolution),
            postFrameBuffer: glhf.NewFrameBufferWithColorDepthStencil(fbSizeX, fbSizeY, false),

            lineShader:       LoadLineShader(fpsCamera),
            skyboxShader:     LoadSkyboxShader(fpsCamera),
            particleSystems:  make(map[ParticleName]*glhf.ParticleSystem),
            UIScale:          uiScale,
            ScreenCoordScale: mgl32.Vec2{screenCoordScaleX, screenCoordScaleY},
            AtlasMap:         atlasMap,
            AtlasSprites:     atlasSprites,

            ShouldDrawSprites: true, ShouldDrawMeshes: true, ShouldDrawParticles: true, ShouldUseVsync: true,
        }

        window.SetKeyCallback(glApp.KeyCallback)
        window.SetCursorPosCallback(glApp.MousePosCallback)

        glApp.SetMouseCapture(true)

        window.SetMouseButtonCallback(glApp.MouseButtonCallback)
        window.SetScrollCallback(glApp.ScrollCallback)
        window.SetDropCallback(glApp.DropCallback)

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
        glApp.colorBatch = glhf.NewSpriteBatch(glApp.guiShader, NewOneWhitePixelTexture(), 10)
        glApp.spriteBatches = []*glhf.SpriteBatch{glApp.colorBatch}
        glApp.AtlasMap.SetGuiShader(glApp.guiShader)
        glApp.AtlasMap.SetMaxSprites(10)
        glApp.AtlasSprites.SetGuiShader(glApp.guiShader)
        glApp.AtlasSprites.SetMaxSprites(10)
        glApp.spriteBatches = append(glApp.spriteBatches, glApp.AtlasMap.GetSpriteBatch())
        glApp.spriteBatches = append(glApp.spriteBatches, glApp.AtlasSprites.GetSpriteBatch())

        postShader := glApp.postShader
        glApp.screenQuad = PostQuad(postShader)

        postShader.Begin()
        postShader.BindSamplerNameToTextureUnit("tex", gl.TEXTURE0)
        //postShader.BindSamplerNameToTextureUnit("depthStencil", gl.TEXTURE1)
        postShader.End()

        skyboxVA := glhf.NewVertexArray[glhf.GlFloat](glApp.skyboxShader, false)
        skyboxVA.Begin()
        skyboxVA.SetVertexData(skyboxVertices())
        skyboxVA.End()
        skyboxVA.DebugName = "SkyBox"
        glApp.skybox = skyboxVA

        glhf.BindUniformBlockTo(glApp.cubeShader.ID(), mainUboBlockName, 0)
        glhf.BindUniformBlockTo(glApp.quadShader.ID(), mainUboBlockName, 0)
        glhf.BindUniformBlockTo(glApp.meshShader.ID(), mainUboBlockName, 0)

        glhf.BindUniformBlockTo(glApp.cubeShaderDepth.ID(), mainUboBlockName, 0)
        glhf.BindUniformBlockTo(glApp.quadShaderDepth.ID(), mainUboBlockName, 0)
        glhf.BindUniformBlockTo(glApp.meshShaderDepth.ID(), mainUboBlockName, 0)

        mainUbo.Begin()

        mainUbo.BindUBOTo(0)

        //ubo.SetAttribute("fogRange", mgl32.Vec2{12, 99})
        mainUbo.SetAttribute(UboGlobalLightColor, mgl32.Vec4{0.2, 0.2, 0.2, 1})
        mainUbo.End()

        glApp.mainUbo = mainUbo

        glhf.BindUniformBlockTo(glApp.cubeShader.ID(), "Lights", 1)
        glhf.BindUniformBlockTo(glApp.quadShader.ID(), "Lights", 1)
        glhf.BindUniformBlockTo(glApp.meshShader.ID(), "Lights", 1)

        lightsUbo.Begin()

        lightsUbo.BindUBOTo(1)

        lightsUbo.End()

        glApp.lightsUbo = lightsUbo

        glApp.lightsUbo.SetLights([]LightInfo{
            {PerspectiveTransform: core.NewPerspectiveTransformForSpotlightFromParent(glApp.Camera, 45, 15, glApp.shadowResolution), Color: mgl32.Vec4{0.7, 0.7, 0.7, 1}},
            {PerspectiveTransform: core.NewPerspectiveTransformForSpotlight(mgl32.Vec3{5.5, 2, 13.5}, mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, mgl32.Vec3{0, 0, -1}), 45, 15, glApp.shadowResolution), Color: mgl32.Vec4{0, 0.7, 0, 1}},
            {PerspectiveTransform: core.NewPerspectiveTransformForSpotlight(mgl32.Vec3{13, 2, 13}, mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, mgl32.Vec3{-1, 0, 0}), 45, 15, glApp.shadowResolution), Color: mgl32.Vec4{0.7, 0, 0, 1}},
        })

        parentApp.Init(glApp, mapLoad)

        return glApp
    })
    glApp := glAppUntyped.(*GlApplication)
    glApp.StartEventLoop()
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
            time = glfw.GetTime()
            elapsed := time - previousTime
            previousTime = time
            deltaTime := elapsed * a.TimeFactor

            { // Update BEGIN
                a.updateToasts(deltaTime)

                if a.Pathfinder != nil {
                    a.Pathfinder.Update(elapsed)
                }
                if a.minimap != nil {
                    a.minimap.Update()
                }
                a.UpdateFunc(deltaTime)

                a.Sprites.Update(deltaTime)
                a.Meshes.Update(deltaTime)
                a.UpdateParticles(deltaTime)
                a.lightsUbo.updateLightAt(0)
            } // Update END

            gl.ClearColor(a.CeilingColor.X(), a.CeilingColor.Y(), a.CeilingColor.Z(), a.CeilingColor.W())

            { // BEGIN SHADOW MAPPING
                gl.Viewport(0, 0, int32(a.shadowResolution), int32(a.shadowResolution))
                a.shadowBuffer.Begin()

                for i := 0; i < len(a.lightsUbo.lights); i++ { // ITERATE ALL LIGHTS
                    a.shadowBuffer.SetDepthLayer(i)
                    gl.Clear(gl.DEPTH_BUFFER_BIT)
                    a.draw3dScene(a.lightsUbo.lights[i], a.cubeShaderDepth, a.meshShaderDepth, a.quadShaderDepth)
                }

                a.shadowBuffer.End()
                gl.Viewport(0, 0, int32(float32(a.WindowWidth)*a.UIScale.X()), int32(float32(a.WindowHeight)*a.UIScale.Y()))
            } // END SHADOW MAPPING

            { // BEGIN RENDER 3D SCENE
                a.postFrameBuffer.Begin()
                a.shadowBuffer.DepthTextureArray().BeginOnUnit(gl.TEXTURE2)

                gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
                a.draw3dScene(a.Camera, a.cubeShader, a.meshShader, a.quadShader)

                if a.skyboxCubeMap != nil && a.ShouldDrawSkybox {
                    a.drawSkybox(a.skyboxCubeMap)
                }

                a.draw3dDebug()

                a.shadowBuffer.DepthTextureArray().End()
                a.postFrameBuffer.End() // will effectively do "gl.BindFramebuffer(gl.FRAMEBUFFER, 0)"
            } // End Render 3D Scene

            { // DRAW FRAMEBUFFER TO SCREEN & POSTPROCESSING
                a.postShader.Begin()

                //a.frameBuffer.Blit(nil, 0, 0, int(float32(a.WindowWidth)*a.UIScale.X()), int(float32(a.WindowHeight)*a.UIScale.Y()), 0, 0, a.WindowWidth*2, a.WindowHeight*2)
                a.postFrameBuffer.Texture().BeginOnUnit(gl.TEXTURE0)
                a.postFrameBuffer.DepthTexture().BeginOnUnit(gl.TEXTURE1)

                a.screenQuad.Begin()
                a.screenQuad.Draw()
                a.screenQuad.End()

                a.postFrameBuffer.DepthTexture().End()
                a.postFrameBuffer.Texture().End()

                a.postShader.End()
                gl.ActiveTexture(gl.TEXTURE0)
            } // End Draw Framebuffer to Screen & Postprocessing

            // DRAW UI ON TOP OF POSTPROCESSING
            { // Begin Draw 2D Scene
                gl.Clear(gl.DEPTH_BUFFER_BIT)
                a.guiShader.Begin()
                glhf.PaintersOffset = 0.0001
                for _, batch := range a.spriteBatches {
                    batch.Begin()
                }

                a.Gui.Draw(deltaTime)

                a.DrawGuiFunc(deltaTime)

                if a.minimap != nil && a.ShouldDrawMinimap {
                    a.minimap.Draw()
                }

                a.drawToasts()

                for _, batch := range a.spriteBatches {
                    batch.End()
                    batch.Render()
                }
                a.guiShader.End()
            } // End Draw 2D Scene

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

func (a *GlApplication) draw3dScene(observer core.Projector, chunkShader, meshShader, spriteShader *glhf.Shader) {
    a.setMainUBOCam(observer.GetPosition(), observer.GetForward(), observer.GetProjectionViewMatrix())

    a.CubeMap.DrawWithShader(a.Camera, chunkShader) // camera only used for frustum culling

    if a.ShouldDrawMeshes {
        a.Meshes.Draw(a.Camera, meshShader) // camera only used for frustum culling
    }
    if a.ShouldDrawSprites {
        a.Sprites.Draw(a.Camera, observer, spriteShader) // this one actually changes viewdirection depending on the camera..
    }
    if a.ShouldDrawParticles {
        //a.DrawParticles(cam)
    }
}

func (a *GlApplication) draw3dDebug() {
    if a.UnitCube != nil {
        a.UnitCube.DrawAllQueued()
    }

    if a.ShouldDrawAABBs {
        a.DrawAABBs()
    }
    if a.ShouldDrawChunkAABBs {
        a.DrawChunkAABBs(a.CubeMap)
    }
    if a.ShouldDrawOBBs {
        a.DrawOBBs()
    }
    if a.ShouldDrawPaths {
        a.DrawPaths()
    }
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

func (a *GlApplication) LoadFixedGuiAtlasWithBatch(filename string, tileW int, tileH int, maxSprites int) *glhf.TextureAtlasFixed {
    atlasFixed := LoadTextureAtlasFixed(filename, tileW, tileH)
    atlasFixed.SetGuiShader(a.guiShader)
    atlasFixed.SetMaxSprites(maxSprites)
    a.spriteBatches = append(a.spriteBatches, atlasFixed.GetSpriteBatch())
    return atlasFixed
}
func (a *GlApplication) LoadPackedGuiAtlasWithBatch(filename string, maxSprites int) *glhf.TextureAtlasPacked {
    atlasFixed := LoadTextureAtlasPacked(filename)
    atlasFixed.SetGuiShader(a.guiShader)
    atlasFixed.SetMaxSprites(maxSprites)
    a.spriteBatches = append(a.spriteBatches, atlasFixed.GetSpriteBatch())
    return atlasFixed
}

func (a *GlApplication) SetupSpriteBatch(atlas glhf.TextureAtlas) {
    atlas.SetGuiShader(a.guiShader)
    atlas.SetMaxSprites(6)
    a.spriteBatches = append(a.spriteBatches, atlas.GetSpriteBatch())
}
func (a *GlApplication) SetupSkybox(cubemap *glhf.Cubemap) {
    a.skyboxCubeMap = cubemap
    a.ShouldDrawSkybox = true
}

func (a *GlApplication) Toast(labelText string) *GuiToast {
    label := a.NewLabel()
    label.SetPosition(mgl32.Vec2{10, 180})
    label.SetText(labelText)
    toast := NewGuiToast(a.UiContext(), label)
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
    ps := glhf.NewParticleSystem(count)
    a.particleSystems[name] = ps
}
func (a *GlApplication) SetupPathfinder(isSolid func(x, y, z int32) bool) {
    a.Pathfinder = NewPathfindingSystem(isSolid)
}
func (a *GlApplication) SetupMinimap(width, height int, getColor func(x, y int) [4]uint8) {
    a.minimap = NewMiniMap(a.guiShader, width, height, getColor)
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
func (a *GlApplication) UpdateParticles(deltaTime float64) {
    for _, ps := range a.particleSystems {
        ps.Update(deltaTime)
    }
}
func (a *GlApplication) DrawParticles(cam core.Camera) {
    a.drawCallCountParticles = 0
    for _, ps := range a.particleSystems {
        ps.Draw(cam)
        a.drawCallCountParticles += 2
    }
}
func (a *GlApplication) drawSkybox(cubemap *glhf.Cubemap) {
    projection := a.Camera.GetProjectionMatrix()
    view := a.Camera.GetViewMatrix().Mat3().Mat4() // remove translation
    projView := projection.Mul4(view)

    gl.DepthMask(false)

    a.skyboxShader.Begin()
    a.skyboxShader.SetUniformAttr(SkyboxShaderCamProjectionView, projView)
    cubemap.Begin()
    a.skybox.Begin()
    a.skybox.Draw()
    a.skybox.End()
    cubemap.End()
    a.skyboxShader.End()

    gl.DepthMask(true)
}

func (a *GlApplication) NewLabel() *BitmapFontMesh {
    return NewBitmapFontMesh(a.guiShader, a.UIFont, a.colorBatch)
}

func (a *GlApplication) NewList() *GuiList {
    return NewGuiList(a.UiContext())
}

func (a *GlApplication) SetupUIAtlas(atlas *glhf.TextureAtlasFixed) {
    a.UIAtlas = atlas
    a.UIAtlas.SetGuiShader(a.guiShader)
    a.UIAtlas.SetMaxSprites(20)
    a.spriteBatches = append(a.spriteBatches, a.UIAtlas.GetSpriteBatch())
}

func (a *GlApplication) SetupUIFont(font BitmapFont) {
    a.UIFont = font
    a.UIFont.Texture.SetGuiShader(a.guiShader)
    a.UIFont.Texture.SetMaxSprites(512)
    a.spriteBatches = append(a.spriteBatches, a.UIFont.Texture.GetSpriteBatch())
}
func (a *GlApplication) OpenTextureCube(atlasForSelection *glhf.TextureAtlasFixed, preselection map[voxel.CubeSide]uint16) *GuiTexturePickerCube {
    return NewGuiTexturePickerCube(a.UiContext(), atlasForSelection, preselection, func(s string) {
        a.Toast(s)
    }, func() {
        gridSize := glhf.IntVec2{int32(atlasForSelection.GridWidth()), int32(atlasForSelection.GridHeight())}
        a.cubeShader.Begin()
        a.cubeShader.SetUniformAttr(CubeShaderAtlasGridSize, gridSize)
        a.cubeShader.End()
    })
}

func (a *GlApplication) OpenAtlasPicker(atlasForSelection glhf.TextureAtlas) *GuiAtlasPicker {
    picker := NewGuiAtlasPicker(a.UiContext(), atlasForSelection)
    picker.HandleResize(a.WindowWidth, a.WindowHeight, a.UIScale)
    return picker
}
func (a *GlApplication) OpenConfigMenu() *GuiConfig {
    return NewGuiConfig(a.UiContext())
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

func mat4ptr(mat mgl32.Mat4) unsafe.Pointer {
    return unsafe.Pointer(&mat[0])
}

func vec4ptr(vec mgl32.Vec4) unsafe.Pointer {
    return unsafe.Pointer(&vec[0])
}

func (a *GlApplication) SaveScreenshot(filename string) error {
    return a.postFrameBuffer.Texture().SaveAsPNG(filename)
}
func (a *GlApplication) SaveShadowMaps(filename string) error {
    //near, far := a.Camera.GetNearAndFar()
    return a.shadowBuffer.DepthTextureArray().SaveAsPNGs(0.15, 15, filename)
}

func (a *GlApplication) updateMainUBO() {
    a.setMainUBOCam(a.Camera.GetPosition(), a.Camera.GetForward(), a.Camera.GetProjectionViewMatrix())
}

func (a *GlApplication) setMainUBOCam(pos, dir mgl32.Vec3, projectionView mgl32.Mat4) {
    a.mainUbo.Begin()
    a.mainUbo.SetAttribute(UboCamDirection, dir.Vec4(0))
    a.mainUbo.SetAttribute(UboCamPosition, pos.Vec4(1))
    a.mainUbo.SetAttribute(UboCamProjectionView, projectionView)
    a.mainUbo.End()
}

func (a *GlApplication) LoadVoxelMap(loadResult core.MapLoadResult) map[mgl32.Vec3]core.MapEntity {
    cubeMap, entitySpawns := toVoxelMap(loadResult, a.AtlasMap, a.cubeShader)
    a.CubeMap = cubeMap
    return entitySpawns
}
func (a *GlApplication) OnResize(width int, height int) {
    a.WindowWidth = width
    a.WindowHeight = height
    a.Camera.SetScreenSize(width, height)
    projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(width, height)
    a.guiShader.Begin()
    a.guiShader.SetUniformAttr(GuiShaderProjection, projectionMatrix)
    a.guiShader.End()
    if a.postFrameBuffer != nil {
        a.postFrameBuffer.Resize(int(float32(width)*a.UIScale.X()), int(float32(height)*a.UIScale.Y()))
    }
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

func (a *GlApplication) UiContext() UIContext {
    return UIContext{
        Shader:     a.guiShader,
        Atlas:      a.UIAtlas,
        Font:       a.UIFont,
        ColorBatch: a.colorBatch,
    }
}

func (a *GlApplication) SetLights(infos []LightInfo) {

}

type UIContext struct {
    Shader     *glhf.Shader
    Atlas      *glhf.TextureAtlasFixed
    Font       BitmapFont
    ColorBatch *glhf.SpriteBatch
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
        glhf.Bounds(0, 0, width, height)
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
    gl.DepthRangef(0, 1)
    gl.Enable(gl.DEPTH_TEST)
    gl.ClearDepth(1.0)

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
    Update(elapsed float64)
    HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey)
    HandleMousePosEvents(xpos float64, ypos float64)
    HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey)
    HandleScrollEvents(xoff float64, yoff float64)
    HandleResize(width int, height int)
    HandleDrop(names []string, x, y float64)
    DrawGui(elapsed float64)
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

func toVoxelMap(loadResult core.MapLoadResult, atlas *glhf.TextureAtlasFixed, shader *glhf.Shader) (*voxel.Map, map[mgl32.Vec3]core.MapEntity) {
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
    shader.SetUniformAttr(CubeShaderAtlasGridSize, glhf.IntVec2{int32(atlas.GridWidth()), int32(atlas.GridHeight())})
    shader.End()

    voxelMap.SetTerrainTexture(atlas)

    entitySpawns := make(map[mgl32.Vec3]core.MapEntity)

    safeTex := func(tex int) uint16 {
        if tex < 0 {
            return 0
        }
        if tex > atlas.LastValidIndex() {
            return 0
        }
        return uint16(tex)
    }

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
                cube.Textures[voxel.North] = safeTex(tileInfo.WallTex)
                cube.Textures[voxel.South] = safeTex(tileInfo.WallTex)
                cube.Textures[voxel.East] = safeTex(tileInfo.WallTex)
                cube.Textures[voxel.West] = safeTex(tileInfo.WallTex)
                cube.Textures[voxel.Top] = safeTex(tileInfo.FloorTex)
                cube.Textures[voxel.Bottom] = safeTex(tileInfo.CeilTex)
            }
        }
    }
    return voxelMap, entitySpawns
}
