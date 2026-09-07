package quads

import (
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

	// ConstrainToWorkArea keeps a borderless window inside the desktop's work area,
	// leaving the macOS menu bar and Dock visible on top of it. Off by default:
	// borderless covers the whole screen.
	ConstrainToWorkArea bool

	// Where the window sat before it went fullscreen. GLFW does not keep this:
	// leaving fullscreen means handing SetMonitor a position and a size, and
	// without the real ones there is nothing to go back to.
	windowedX, windowedY          int
	windowedWidth, windowedHeight int
	ShouldUseVsync                bool

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
	// The one light slot that follows a game object around. -1 until something asks
	// for it; the slot is then kept for good, see DetachLight.

	colorBatch *glhf.SpriteBatch

	shadowBuffer       *glhf.Frame
	postFrameBuffer    *glhf.Frame
	bloomBuffers       [2]*glhf.Frame
	bloomShader        *glhf.Shader
	bloomQuad          *glhf.VertexArray[glhf.GlFloat]
	bloomStrength      float32
	bloomThreshold     float32
	emissiveStrength   float32
	emissiveCutoff     float32
	emissiveSaturation float32
	exposure           float32
	gamma              float32
	shadowNormalOffset float32
	shadowDepthBias    float32
	ShouldDrawBloom    bool
	screenQuad         *glhf.VertexArray[glhf.GlFloat]
	UIScale            mgl32.Vec2
	ScreenCoordScale   mgl32.Vec2

	// Multiplies the size of everything drawn in the 2D GUI pass. 1 is the size the
	// GUI has always been drawn at, which is already one window pixel per GUI unit.
	guiScale float32

	// The drawable size in pixels, which is not the window size in screen
	// coordinates on a hidpi display. Only GLFW knows the ratio between them, so
	// it is asked rather than derived from the monitor's content scale.
	FramebufferWidth, FramebufferHeight int
	drawCallCountParticles              int
	benchmark                           *Benchmark
	benchmarkLabel                      string

	// A capture run: the frames to grab, where to write them, and the frame
	// time to pretend every frame took. See Screenshots and FixedStep.
	shotPath             string
	shotFirst            int
	shotEvery            int
	shotCount            int
	shotsTaken           int
	fixedStep            float64
	drawCallCountMeshes  int
	drawCallCountSprites int
	drawCallCountVoxels  int
	drawCallCountGui     int

	minimap *Minimap

	ShouldDrawSkybox  bool
	ShouldDrawMinimap bool
	AtlasMap          *glhf.TextureAtlasFixed
	AtlasSprites      *glhf.TextureAtlasFixed

	UIAtlas *glhf.TextureAtlasFixed
	UIFont  BitmapFont

	spriteBatches    []*glhf.SpriteBatch
	shadowResolution int
	shadowPassCount  int

	guiQuadsDrawn int
	guiDrawCalls  int
}

// GuiDrawnLastFrame is what the 2D pass put on screen: every quad written into a
// sprite batch, whoever wrote it - the GUI drawer, the bitmap fonts, the
// minimap, the toasts and the widgets all share these batches, and one non-empty
// batch is one draw call.
func (a *GlApplication) GuiDrawnLastFrame() (triangles, drawCalls int) {
	return a.guiQuadsDrawn * 2, a.guiDrawCalls
}

// Start starts the OpenGL application with the given parentApp and screen dimensions.
// Will block until the window is closed.
// Will call the parentApp's Init, Update, HandleKeyEvents, HandleMousePosEvents, HandleMouseButtonEvents, and HandleScrollEvents functions.
func Start(parentApp GlGame, mapLoad core.MapLoadResult, screenW, screenH int) {
	glAppUntyped := glhf.CallVal(func() any {
		mouseSensitivity := float32(0.1)

		window, terminateFunc := InitOpenGLWindow("fps", screenW, screenH, false)

		// No monitor at all is not an error worth crashing on: GLFW answers
		// nil when the display is asleep or the session has none, and a
		// screenshot run wants to go on rendering into the back buffer
		// regardless. One to one is what an unscaled display reports anyway.
		uiScaleX, uiScaleY := float32(1), float32(1)
		if primaryMonitor := glfw.GetPrimaryMonitor(); primaryMonitor != nil {
			uiScaleX, uiScaleY = primaryMonitor.GetContentScale()
		}
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

		mainUboGLSL := mainUbo.GetShaderBlockSource(mainUboBlockName)
		injectedGLSL := mainUboGLSL + lightsGLSLBlock

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
				atlasMap = Disk.TextureAtlasFixed(filepath.Join(baseTexPath, entry.Name()), width, height)
			} else if spriteAtlasRegex.MatchString(entry.Name()) {
				match := spriteAtlasRegex.FindStringSubmatch(entry.Name())
				width, _ = strconv.Atoi(match[1])
				height, _ = strconv.Atoi(match[2])
				atlasSprites = Disk.TextureAtlasFixed(filepath.Join(baseTexPath, entry.Name()), width, height)
			}

		}

		shadowResolution := 512
		glApp := &GlApplication{
			WindowWidth:        winSizeX,
			WindowHeight:       winSizeY,
			Window:             window,
			TerminateFunc:      terminateFunc,
			TimeFactor:         1.0,
			shadowResolution:   shadowResolution,
			Camera:             fpsCamera,
			guiShader:          LoadGuiShader(winSizeX, winSizeY, mgl32.Vec2{1, 1}),
			postShader:         LoadPostShader(mainUboGLSL),
			bloomShader:        LoadBloomShader(),
			bloomStrength:      DefaultBloomStrength,
			bloomThreshold:     DefaultBloomThreshold,
			emissiveStrength:   DefaultEmissiveStrength,
			emissiveCutoff:     DefaultEmissiveCutoff,
			emissiveSaturation: DefaultEmissiveSaturation,
			exposure:           DefaultExposure,
			gamma:              DefaultGamma,
			shadowNormalOffset: DefaultShadowNormalOffset,
			shadowDepthBias:    DefaultShadowDepthBias,
			ShouldDrawBloom:    true,

			quadShader: LoadQuadShader(injectedGLSL),
			cubeShader: LoadCubeShader(injectedGLSL),
			meshShader: LoadMeshShader(injectedGLSL),

			quadShaderDepth: LoadQuadShaderDepthPass(injectedGLSL),
			cubeShaderDepth: LoadCubeShaderDepthPass(injectedGLSL),
			meshShaderDepth: LoadMeshShaderDepthPass(injectedGLSL),

			shadowBuffer: glhf.NewFrameBufferWithDepth(shadowResolution, shadowResolution, MaxShadowLayers),
			// HDR: the scene keeps values past 1.0 all the way to the post pass, which
			// is what lets the bloom threshold tell a blown-out highlight from a merely
			// bright texel, and what lets tone mapping happen once instead of per shader.
			postFrameBuffer: glhf.NewFrameBufferHDR(fbSizeX, fbSizeY, false),

			lineShader:        LoadLineShader(fpsCamera),
			skyboxShader:      LoadSkyboxShader(fpsCamera, mainUboGLSL),
			particleSystems:   make(map[ParticleName]*glhf.ParticleSystem),
			UIScale:           uiScale,
			ScreenCoordScale:  mgl32.Vec2{screenCoordScaleX, screenCoordScaleY},
			FramebufferWidth:  fbSizeX,
			FramebufferHeight: fbSizeY,
			guiScale:          1,
			AtlasMap:          atlasMap,
			AtlasSprites:      atlasSprites,

			ShouldDrawSprites: true, ShouldDrawMeshes: true, ShouldDrawParticles: true, ShouldUseVsync: true,
			ShouldDrawDebugInfo: true,
		}

		// Baseline for the fullscreen toggle, in case the first thing that happens
		// is a switch to fullscreen.
		glApp.rememberWindowedBounds()

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

		glApp.screenQuad = PostQuad(glApp.postShader)
		// The bloom shader needs its own vertex array: glhf resolves attribute
		// locations against the shader the array was built for.
		glApp.bloomQuad = PostQuad(glApp.bloomShader)
		glApp.initBloomBuffers(fbSizeX, fbSizeY)

		skyboxVA := glhf.NewVertexArray[glhf.GlFloat](glApp.skyboxShader, false)
		skyboxVA.Begin()
		skyboxVA.SetVertexData(skyboxVertices())
		skyboxVA.End()
		skyboxVA.DebugName = "SkyBox"
		glApp.skybox = skyboxVA

		// The post shader tone maps, so it reads exposure and gamma from the same UBO
		// the scene shaders use.
		glhf.BindUniformBlockTo(glApp.postShader.ID(), mainUboBlockName, 0)
		glhf.BindUniformBlockTo(glApp.skyboxShader.ID(), mainUboBlockName, 0)
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
		mainUbo.SetAttribute(UboToneMapParams, mgl32.Vec2{DefaultExposure, DefaultGamma})
		mainUbo.SetAttribute(UboShadowParams, mgl32.Vec2{DefaultShadowNormalOffset, DefaultShadowDepthBias})
		mainUbo.End()

		glApp.mainUbo = mainUbo

		glhf.BindUniformBlockTo(glApp.cubeShader.ID(), "Lights", 1)
		glhf.BindUniformBlockTo(glApp.quadShader.ID(), "Lights", 1)
		glhf.BindUniformBlockTo(glApp.meshShader.ID(), "Lights", 1)

		lightsUbo.Begin()

		lightsUbo.BindUBOTo(1)

		lightsUbo.End()

		glApp.lightsUbo = lightsUbo

		// Slot 0 is the player's light and nothing else: it hangs off the camera, is
		// updated by index every frame, and belongs to the player rather than to any
		// map. Every other light comes from the map file - the two that used to be
		// hardcoded here now live in raymap.ldtk as Light entities, where they can be
		// moved without a rebuild and where a second map can have its own.
		glApp.lightsUbo.AttachPlayerLightTo(glApp.Camera, glApp.shadowResolution, PlayerLightOff)

		parentApp.Init(glApp, mapLoad)

		return glApp
	})
	glApp := glAppUntyped.(*GlApplication)
	glApp.StartEventLoop()
}

// maxFrameDelta caps the timestep handed to update and physics. Long frames are
// reported as this instead, so a stalled frame slows the world down rather than
// teleporting everything in it.
const maxFrameDelta = 0.1

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
			// A stall - dragging the window, a breakpoint, alt-tab - otherwise hands
			// the next frame a delta of several seconds, and physics integrates
			// straight through the level in one step.
			elapsed = math.Min(elapsed, maxFrameDelta)
			// A capture run advances the world by the same amount every frame,
			// whatever the machine took to draw it, so tick 90 of one run holds
			// the same animation frame as tick 90 of the next.
			if a.fixedStep > 0 {
				elapsed = a.fixedStep
			}
			deltaTime := elapsed * a.TimeFactor

			{ // Update BEGIN
				a.updateToasts(deltaTime)

				if a.Pathfinder != nil {
					a.Pathfinder.Update(deltaTime)
				}
				if a.minimap != nil {
					a.minimap.Update()
				}
				a.UpdateFunc(deltaTime)

				a.Sprites.Update(deltaTime)
				a.Meshes.Update(deltaTime)
				a.UpdateParticles(deltaTime)
				a.lightsUbo.UpdatePlayerLight()
				a.lightsUbo.UpdateTrackedLight()
			} // Update END

			// The scene buffer is linear HDR, so the authored ceiling colour is lifted to
			// the value that tone maps back to itself. Clearing with the raw colour would
			// come out darker on screen.
			gl.ClearColor(
				HDRFromDisplay(a.CeilingColor.X(), a.exposure, a.gamma),
				HDRFromDisplay(a.CeilingColor.Y(), a.exposure, a.gamma),
				HDRFromDisplay(a.CeilingColor.Z(), a.exposure, a.gamma),
				a.CeilingColor.W(),
			)

			{ // BEGIN SHADOW MAPPING
				gl.Viewport(0, 0, int32(a.shadowResolution), int32(a.shadowResolution))
				a.shadowBuffer.Begin()
				// ponytail: no front-face culling in the depth pass. It is the usual
				// cure for shadow acne, but billboard sprites are single quads, so
				// culling their front faces would drop them out of the shadow map
				// entirely. The slope-scaled bias in ShadowCalculation covers it.

				sceneSignature := a.shadowSceneSignature()
				a.shadowPassCount = 0
				lights := a.lightsUbo.Lights()
				for i := range lights {
					light := &lights[i]
					// A light that is off contributes nothing, one with no atlas layer
					// casts no shadow at all, and a light whose frustum and scene are
					// unchanged still has a valid depth map from last frame.
					if light.IsOff() || light.ShadowLayer < 0 || !light.NeedsShadowUpdate(sceneSignature) {
						continue
					}
					a.shadowBuffer.SetDepthLayer(int(light.ShadowLayer))
					gl.Clear(gl.DEPTH_BUFFER_BIT)
					a.draw3dScene(light, a.cubeShaderDepth, a.meshShaderDepth, a.quadShaderDepth, nil)
					light.MarkShadowRendered(sceneSignature)
					a.shadowPassCount++
				}

				a.shadowBuffer.End()
				gl.Viewport(0, 0, int32(a.FramebufferWidth), int32(a.FramebufferHeight))
			} // END SHADOW MAPPING

			{ // BEGIN RENDER 3D SCENE
				a.postFrameBuffer.Begin()
				a.shadowBuffer.DepthTextureArray().BeginOnUnit(gl.TEXTURE2)

				gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
				a.draw3dScene(a.Camera, a.cubeShader, a.meshShader, a.quadShader, func() {
					if a.skyboxCubeMap != nil && a.ShouldDrawSkybox {
						a.drawSkybox(a.skyboxCubeMap)
					}
				})

				// Particles draw in the main pass only: they need the view and
				// projection matrices separately (the geometry shader billboards in
				// view space), which Observer does not expose, and a shadow pass has
				// no business rasterising them.
				if a.ShouldDrawParticles {
					a.DrawParticles(a.Camera)
				}

				a.draw3dDebug()

				a.shadowBuffer.DepthTextureArray().End()
				a.postFrameBuffer.End() // will effectively do "gl.BindFramebuffer(gl.FRAMEBUFFER, 0)"
			} // End Render 3D Scene

			{ // BLOOM, TONE MAPPING, THEN THE SCENE TO THE SCREEN
				bloomResult := a.renderBloom()

				gl.Viewport(0, 0, int32(a.FramebufferWidth), int32(a.FramebufferHeight))
				a.postShader.Begin()
				a.postShader.SetUniformAttr(PostShaderBloomStrength, a.bloomStrengthInUse())

				a.postFrameBuffer.Texture().BeginOnUnit(gl.TEXTURE0)
				bloomResult.Texture().BeginOnUnit(gl.TEXTURE1)

				a.screenQuad.Begin()
				a.screenQuad.Draw()
				a.screenQuad.End()

				bloomResult.Texture().End()
				a.postFrameBuffer.Texture().End()

				a.postShader.End()
				gl.ActiveTexture(gl.TEXTURE0)
			} // End bloom, tone mapping and the scene to the screen

			// DRAW UI ON TOP OF POSTPROCESSING
			{ // Begin Draw 2D Scene
				gl.Clear(gl.DEPTH_BUFFER_BIT)
				gl.Enable(gl.BLEND)
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

				// A capture is a picture of the weapon, or of the map. Whatever
				// the game last said about the fog is neither.
				if a.shotPath == "" {
					a.drawToasts()
				}

				a.guiQuadsDrawn, a.guiDrawCalls = 0, 0
				for _, batch := range a.spriteBatches {
					batch.End()
					if quads := batch.QuadCount(); quads > 0 {
						a.guiQuadsDrawn += quads
						a.guiDrawCalls++
					}
					batch.Render()
				}
				a.guiShader.End()
				gl.Disable(gl.BLEND)
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

			glhf.CheckGLError("end of frame") // no-op unless built with -tags debugGL

			// Grabbed off the back buffer, so it has to happen before the swap.
			if a.captureDue() {
				a.capture()
				shouldQuit = a.shotsTaken >= a.shotCount
			}

			a.Window.SwapBuffers()
			glfw.PollEvents()
			a.Ticks++

			// Measured after SwapBuffers, so the wait for vblank is inside the frame
			// it belongs to. With vsync on that makes every number the refresh rate,
			// which is why -bench turns vsync off.
			if a.benchmark != nil && a.benchmark.record(elapsed) {
				fmt.Print(a.benchmark.Report(a.benchmarkLabel, a.SceneSummary()))
				shouldQuit = true
			}
		})
	}
}

// shadowSceneSignature hashes everything a depth map depends on: the world mesh
// generation plus the position of every caster. Hashing a few hundred positions
// costs far less than the full scene redraw it lets an unchanged light skip.
// Per-object hashes are summed rather than chained, because the drawers iterate
// maps and the order changes from frame to frame.
func (a *GlApplication) shadowSceneSignature() uint64 {
	signature := a.CubeMap.MeshGeneration() * 0x9E3779B97F4A7C15
	hashPos := func(pos mgl32.Vec3) {
		h := uint64(14695981039346656037)
		for _, f := range [3]float32{pos.X(), pos.Y(), pos.Z()} {
			h = (h ^ uint64(math.Float32bits(f))) * 1099511628211
		}
		signature += h
	}
	a.Sprites.ForEachCaster(hashPos)
	a.Meshes.ForEachCaster(hashPos)
	return signature
}

// draw3dScene draws the world from the observer's point of view. betweenPasses, if
// given, runs after the opaque geometry and before the alpha-blended sprites - that
// is where the skybox goes, so translucent sprites blend against the sky instead of
// the clear colour. The shadow pass passes nil: it has no sky and writes depth only.
func (a *GlApplication) draw3dScene(observer core.Observer, chunkShader, meshShader, spriteShader *glhf.Shader, betweenPasses func()) {
	a.setMainUBOCam(observer.GetPosition(), observer.GetForward(), observer.GetProjectionViewMatrix())

	// Cull against the observer, not the camera: a shadow pass has to include every
	// caster inside the light's frustum, including the ones behind the player.
	a.CubeMap.DrawWithShader(observer, chunkShader)

	if a.ShouldDrawMeshes {
		a.Meshes.Draw(observer, meshShader)
	}
	// Sprites keep blending: the atlases have soft edges and the shader only
	// discards fully transparent texels. Chunks and meshes are opaque and no
	// longer pay for the blend unit.
	forShadows := spriteShader == a.quadShaderDepth
	if a.ShouldDrawSprites {
		gl.Enable(gl.BLEND)
		a.Sprites.Draw(observer, spriteShader, forShadows)
		// Decals are skipped in the shadow pass on purpose: they sit a centimetre
		// off the wall they are stuck to, so a depth map holding them would shadow
		// that wall with the bullet hole's own outline.
		if !forShadows {
			a.Sprites.DrawDecals(observer, spriteShader)
		}
		gl.Disable(gl.BLEND)
	}

	if betweenPasses != nil {
		betweenPasses()
	}

	if a.ShouldDrawSprites {
		gl.Enable(gl.BLEND)
		a.Sprites.DrawTransparent(observer, spriteShader, forShadows)
		gl.Disable(gl.BLEND)
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
	atlasFixed := Disk.TextureAtlasFixed(filename, tileW, tileH)
	atlasFixed.SetGuiShader(a.guiShader)
	atlasFixed.SetMaxSprites(maxSprites)
	a.spriteBatches = append(a.spriteBatches, atlasFixed.GetSpriteBatch())
	return atlasFixed
}
func (a *GlApplication) LoadPackedGuiAtlasWithBatch(filename string, maxSprites int) *glhf.TextureAtlasPacked {
	atlasFixed := Disk.TextureAtlasPacked(filename)
	atlasFixed.SetGuiShader(a.guiShader)
	atlasFixed.SetMaxSprites(maxSprites)
	a.spriteBatches = append(a.spriteBatches, atlasFixed.GetSpriteBatch())
	return atlasFixed
}

// DropSpriteBatch takes a batch out of the 2D pass. Everything that owns one
// is drawn every frame for as long as it is in the list, so something loaded
// on demand - a weapon's own atlas - has to be able to leave again.
func (a *GlApplication) DropSpriteBatch(batch *glhf.SpriteBatch) {
	for i, candidate := range a.spriteBatches {
		if candidate == batch {
			a.spriteBatches = append(a.spriteBatches[:i], a.spriteBatches[i+1:]...)
			return
		}
	}
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
	a.minimap.Resize(a.GuiSize())
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

// WindowMode is how the window covers the screen.
type WindowMode int

const (
	WindowModeWindowed WindowMode = iota
	WindowModeBorderless
	WindowModeFullscreen
)

// WindowModeNames are the labels for the modes, indexed by WindowMode.
var WindowModeNames = []string{"Windowed", "Borderless", "Fullscreen"}

// WindowMode reports the current mode by asking the window, so there is no second
// copy of this state to go stale. Undecorated means borderless: nothing else takes
// the decoration off.
func (a *GlApplication) WindowMode() WindowMode {
	switch {
	case a.Window.GetMonitor() != nil:
		return WindowModeFullscreen
	case a.Window.GetAttrib(glfw.Decorated) == glfw.False:
		return WindowModeBorderless
	}
	return WindowModeWindowed
}

// IsFullscreen is true for both of the screen-covering modes.
func (a *GlApplication) IsFullscreen() bool {
	return a.WindowMode() != WindowModeWindowed
}

// rememberWindowedBounds records the current position and size, so going back to
// windowed can put the window where it was.
func (a *GlApplication) rememberWindowedBounds() {
	if a.IsFullscreen() {
		return // the saved bounds are already the windowed ones
	}
	a.windowedX, a.windowedY = a.Window.GetPos()
	a.windowedWidth, a.windowedHeight = a.Window.GetSize()
}

// nativeVideoMode is the panel's own resolution: the largest mode the display
// offers. A display's mode list tops out at what the hardware actually has, so the
// biggest entry is the native one. Not monitor.GetVideoMode(), which on macOS
// reports the desktop's scaled UI size instead - see SetWindowMode.
func nativeVideoMode(monitor *glfw.Monitor) *glfw.VidMode {
	modes := monitor.GetVideoModes()
	best := monitor.GetVideoMode()
	for _, mode := range modes {
		if mode.Width*mode.Height > best.Width*best.Height {
			best = mode
		}
	}
	return best
}

// SetWindowMode switches between windowed, borderless and real fullscreen.
//
// The two screen-covering modes differ in what they render into, and on macOS the
// numbers are not the ones the API names suggest. Measured on a Mac with a 2880x1864
// panel whose desktop is set to "looks like 1710x1107":
//
//   - Borderless matches the desktop: a 3420x2214 framebuffer (1710x1107 at a
//     backing scale of 2), which the window server then downsamples to the panel.
//     That is 39% more fragments than the panel has, spent on supersampling.
//   - Fullscreen takes the panel's own 2880x1864, one fragment per physical pixel,
//     and asks the window server to resample nothing.
//
// The desktop's 1710x1107 is not in the mode list and cannot be: on Apple Silicon a
// scaled desktop is not a display mode, it is the compositor rendering large and
// downsampling. The panel's real mode, 2880x1864, is listed - it is the last entry.
func (a *GlApplication) SetWindowMode(mode WindowMode) {
	if mode == a.WindowMode() {
		return
	}
	a.applyWindowMode(mode)
}

// RefreshWindowMode re-applies the current mode, for when something it depends on
// has changed - ConstrainToWorkArea, say.
func (a *GlApplication) RefreshWindowMode() {
	a.applyWindowMode(a.WindowMode())
}

func (a *GlApplication) applyWindowMode(mode WindowMode) {
	a.rememberWindowedBounds()

	// Leave whatever the window is in now, so each case starts from a plain window.
	if a.Window.GetMonitor() != nil {
		a.Window.SetMonitor(nil, a.windowedX, a.windowedY, a.windowedWidth, a.windowedHeight, glfw.DontCare)
	}
	showSystemChrome()
	a.Window.SetAttrib(glfw.Decorated, glfw.True)
	a.Window.SetAttrib(glfw.Resizable, glfw.False)

	monitor := glfw.GetPrimaryMonitor()
	switch mode {
	case WindowModeWindowed:
		a.Window.SetPos(a.windowedX, a.windowedY)
		a.Window.SetSize(a.windowedWidth, a.windowedHeight)
	case WindowModeBorderless:
		if a.ConstrainToWorkArea {
			// Sit inside the work area and leave the menu bar and Dock alone. The
			// clamp below would force this anyway; asking for it is clearer than
			// being overruled.
			areaX, areaY, areaWidth, areaHeight := monitor.GetWorkarea()
			a.Window.SetAttrib(glfw.Decorated, glfw.False)
			a.Window.SetPos(areaX, areaY)
			a.Window.SetSize(areaWidth, areaHeight)
			break
		}
		videoMode := monitor.GetVideoMode()
		monitorX, monitorY := monitor.GetPos()
		// Resizable, only while borderless. Cocoa clamps a non-resizable window to
		// the work area, which starts below the menu bar, so the window landed 29
		// points down: an undrawn strip at the top and the bottom hanging off the
		// screen. There is nothing to drag on an undecorated window anyway.
		a.Window.SetAttrib(glfw.Resizable, glfw.True)
		a.Window.SetAttrib(glfw.Decorated, glfw.False)
		a.Window.SetPos(monitorX, monitorY)
		a.Window.SetSize(videoMode.Width, videoMode.Height)
		hideSystemChrome()
	case WindowModeFullscreen:
		videoMode := nativeVideoMode(monitor)
		a.Window.SetMonitor(monitor, 0, 0, videoMode.Width, videoMode.Height, videoMode.RefreshRate)
	}

	a.ShouldRenderFullscreen = a.IsFullscreen()
	a.OnResize(a.Window.GetSize())
}

// SetWindowSize applies a windowed resolution, leaving any screen-covering mode.
func (a *GlApplication) SetWindowSize(vMode *glfw.VidMode) {
	a.SetWindowMode(WindowModeWindowed)
	a.windowedWidth, a.windowedHeight = vMode.Width, vMode.Height
	// Keep the window where it is. Moving it to 0,0 puts the title bar under the
	// menu bar on macOS, and then it cannot be dragged back.
	a.Window.SetSize(vMode.Width, vMode.Height)
	a.OnResize(a.Window.GetSize())
}

func (a *GlApplication) DrawAABBs() {
	a.lineShader.Begin()
	a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
	a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{0.8, 2, 0.8})
	// The boxes are batched into one mesh in world space - see
	// voxel.QueueDebugDraw - so there is nothing left for the shader to place.
	a.lineShader.SetUniformAttr(LineShaderModelTransform, mgl32.Ident4())

	a.Sprites.DrawAABBs(a.lineShader, a.Camera)
	a.Meshes.DrawAABBs(a.Camera, a.lineShader)

	a.lineShader.End()
}

func (a *GlApplication) DrawChunkAABBs(cubeMap *voxel.Map) {
	a.lineShader.Begin()
	a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
	a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{0.8, 2, 0.8})
	a.lineShader.SetUniformAttr(LineShaderModelTransform, mgl32.Ident4())

	cubeMap.DrawAABBs(a.lineShader)

	a.lineShader.End()
}
func (a *GlApplication) DrawOBBs() {
	a.lineShader.Begin()
	a.lineShader.SetUniformAttr(0, a.Camera.GetProjectionViewMatrix())
	a.lineShader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{1, 0.498, 0.259})
	a.lineShader.SetUniformAttr(LineShaderModelTransform, mgl32.Ident4())

	a.Sprites.DrawOBBs(a.lineShader, a.Camera)
	a.Meshes.DrawOBBs(a.Camera, a.lineShader)

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

// bloomDownscale is how much smaller the bloom chain runs than the scene. A glow is low
// frequency by definition, so a quarter resolution costs a sixteenth of the fragments,
// the free bilinear upsample at the end widens it further, and every blur tap covers
// four times as much screen as it would at full resolution - which is most of what
// makes the glow reach far enough to read as light bleeding.
const bloomDownscale = 4

func (a *GlApplication) initBloomBuffers(width, height int) {
	w, h := max(width/bloomDownscale, 1), max(height/bloomDownscale, 1)
	for i := range a.bloomBuffers {
		// smooth: the blur taps and the upsample into the post pass both want linear
		// filtering, which is half the reason the chain can afford to be this small.
		a.bloomBuffers[i] = glhf.NewFrameBufferHDR(w, h, true)
	}
}

// bloomStrengthInUse is 0 when bloom is switched off, which turns the composite in the
// post shader into a plain tone map without needing a second shader for the case.
func (a *GlApplication) bloomStrengthInUse() float32 {
	if !a.ShouldDrawBloom {
		return 0
	}
	return a.bloomStrength
}

// renderBloom extracts the over-threshold part of the HDR scene into a half resolution
// buffer and blurs it, ping-ponging between the two bloom buffers: horizontal into one,
// vertical back into the other. Returns whichever buffer holds the result.
//
// ponytail: one blur width, no mip chain. A mip pyramid gives a softer, wider glow for
// more passes and more buffers; worth it only if this reads too tight.
func (a *GlApplication) renderBloom() *glhf.Frame {
	source := a.bloomBuffers[0]
	target := a.bloomBuffers[1]

	if !a.ShouldDrawBloom {
		// Still has to be a black texture rather than a stale one: the post shader
		// multiplies it by a strength of zero, but sampling a buffer that was never
		// cleared this session is asking for trouble.
		source.Begin()
		gl.Viewport(0, 0, int32(source.Texture().Width()), int32(source.Texture().Height()))
		gl.ClearColor(0, 0, 0, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		source.End()
		return source
	}

	gl.Disable(gl.DEPTH_TEST)
	defer gl.Enable(gl.DEPTH_TEST)

	a.bloomShader.Begin()
	a.bloomQuad.Begin()
	gl.Viewport(0, 0, int32(source.Texture().Width()), int32(source.Texture().Height()))

	// Extract: HDR scene -> bloomBuffers[0]
	a.bloomShader.SetUniformAttr(BloomShaderMode, BloomModeExtract)
	a.bloomShader.SetUniformAttr(BloomShaderThreshold, a.bloomThreshold)
	source.Begin()
	a.postFrameBuffer.Texture().BeginOnUnit(gl.TEXTURE0)
	a.bloomQuad.Draw()
	a.postFrameBuffer.Texture().End()
	source.End()

	// Blur: alternate horizontal and vertical, ping-ponging between the two buffers.
	// Each horizontal+vertical pair doubles the tap spacing, so three pairs reach far
	// wider than three passes at a fixed radius would, for the same six draws.
	a.bloomShader.SetUniformAttr(BloomShaderMode, BloomModeBlur)
	radius := bloomBaseBlurRadius
	for pass := 0; pass < bloomBlurPasses*2; pass++ {
		direction := mgl32.Vec2{1, 0}
		if pass%2 == 1 {
			direction = mgl32.Vec2{0, 1}
		}
		a.bloomShader.SetUniformAttr(BloomShaderTexelDirection, direction)
		a.bloomShader.SetUniformAttr(BloomShaderBlurRadius, radius)
		if pass%2 == 1 {
			radius *= 2
		}

		target.Begin()
		source.Texture().BeginOnUnit(gl.TEXTURE0)
		a.bloomQuad.Draw()
		source.Texture().End()
		target.End()

		source, target = target, source
	}

	a.bloomQuad.End()
	a.bloomShader.End()

	// source is the buffer last written to, after the final swap.
	return source
}

// SetExposure and SetGamma drive the tone map for every shader at once, through the
// main UBO. Exposure is the knob that decides how far into the highlights the image
// rolls before it clips.
func (a *GlApplication) SetExposure(exposure float32) {
	a.exposure = exposure
	a.updateToneMapping()
}

func (a *GlApplication) SetGamma(gamma float32) {
	a.gamma = gamma
	a.updateToneMapping()
}

func (a *GlApplication) Exposure() float32 { return a.exposure }
func (a *GlApplication) Gamma() float32    { return a.gamma }

// SetShadowBias takes the two knobs of the normal-offset shadow bias: how many shadow
// texels the sample position is pushed along the surface normal, and the flat depth
// bias left over for depth-buffer quantisation. Exposed because the useful value
// depends on the shadow resolution and the scale of the map.
// Screenshots turns this run into a capture: it saves the finished frame at
// tick `first`, then one every `every` ticks after that until it has `count`
// of them, and quits. More than one screenshot writes them as name-00.png,
// name-01.png and so on, which is what makes a weapon's animation reviewable
// as a strip rather than as one lucky frame.
//
// `step` is the frame time every frame is told it took. A capture is compared
// against another capture, so it has to be the same run twice; with the real
// clock a slower machine lands on a different animation frame. Pass 0 to keep
// the real one.
func (a *GlApplication) Screenshots(path string, first, every, count int, step float64) {
	if every < 1 {
		every = 1
	}
	if count < 1 {
		count = 1
	}
	a.shotPath, a.shotFirst, a.shotEvery, a.shotCount = path, first, every, count
	a.fixedStep = step
	// Nothing waits for vblank in a capture: the frame time is fixed anyway, so
	// waiting only makes the run take longer.
	a.ShouldUseVsync = false
	a.SetVSync(false)
}

func (a *GlApplication) captureDue() bool {
	if a.shotPath == "" || a.shotsTaken >= a.shotCount {
		return false
	}
	tick := int(a.Ticks)
	return tick >= a.shotFirst && (tick-a.shotFirst)%a.shotEvery == 0
}

func (a *GlApplication) capture() {
	path := a.shotPath
	if a.shotCount > 1 {
		extension := filepath.Ext(path)
		path = fmt.Sprintf("%s-%02d%s", strings.TrimSuffix(path, extension), a.shotsTaken, extension)
	}
	if err := a.SaveScreenshot(path); err != nil {
		log.Printf("[capture] %s: %v", path, err)
	} else {
		fmt.Printf("[capture] tick %d -> %s\n", a.Ticks, path)
	}
	a.shotsTaken++
}

// StartBenchmark makes this run a measurement: it quits after the benchmark's warm
// up plus duration, having printed one report. VSync goes off, because a frame time
// capped at the refresh rate measures the monitor and not the renderer.
func (a *GlApplication) StartBenchmark(bench *Benchmark, label string) {
	a.benchmark = bench
	a.benchmarkLabel = label
	a.ShouldUseVsync = false
	a.SetVSync(false)
}

// SceneSummary is what was on screen, in the terms the render path is optimised in.
// A frame time on its own is not comparable to another frame time; a frame time next
// to a draw call count is.
func (a *GlApplication) SceneSummary() string {
	spriteTris, spriteCalls := a.Sprites.DrawnLastFrame()
	meshTris, meshCalls := a.Meshes.DrawnLastFrame()
	shadowPasses, litLights := a.ShadowPasses()

	mapTris, mapCalls := 0, 0
	if a.CubeMap != nil {
		mapTris, mapCalls = a.CubeMap.TriangleDrawCount(), a.CubeMap.DrawCalls()
	}
	return fmt.Sprintf(
		"last frame    map %d tris / %d calls, sprites %d tris / %d calls, meshes %d tris / %d calls, %d of %d lights redrew their shadow map",
		mapTris, mapCalls, spriteTris, spriteCalls, meshTris, meshCalls, shadowPasses, litLights)
}

// SetAmbientLight is the floor under everything: the colour, and in the alpha the
// strength, that a surface gets no matter which lights reach it. Turning it down is
// how a scene is made to show what its lights actually do.
func (a *GlApplication) SetAmbientLight(colour mgl32.Vec3, strength float32) {
	a.mainUbo.Begin()
	a.mainUbo.SetAttribute(UboGlobalLightColor, colour.Vec4(strength))
	a.mainUbo.End()
}

func (a *GlApplication) SetShadowBias(normalOffset, depthBias float32) {
	a.shadowNormalOffset = normalOffset
	a.shadowDepthBias = depthBias
	a.mainUbo.Begin()
	a.mainUbo.SetAttribute(UboShadowParams, mgl32.Vec2{normalOffset, depthBias})
	a.mainUbo.End()
}

func (a *GlApplication) updateToneMapping() {
	a.mainUbo.Begin()
	a.mainUbo.SetAttribute(UboToneMapParams, mgl32.Vec2{a.exposure, a.gamma})
	a.mainUbo.End()
}

// SetBloom takes the threshold above which a fragment starts to glow and how much of
// the blurred result is added back. Threshold is in HDR units, so 1.0 is "as bright as
// white" and anything under it never blooms.
// SetEmissiveStrength is how far past white a face marked emissive is pushed. It is
// the other half of the bloom knob: this decides what glows, the bloom threshold
// decides how much of it blooms.
func (a *GlApplication) SetEmissiveStrength(strength float32) {
	a.emissiveStrength = strength
	a.cubeShader.Begin()
	a.cubeShader.SetUniformAttr(CubeShaderEmissiveStrength, strength)
	a.cubeShader.End()
}

// SetEmissiveCutoff is the luminance above which a texel of an emissive texture glows.
// Turn it down towards 0 to light the whole tile, up to pick out only the brightest
// screens on a busy panel.
func (a *GlApplication) SetEmissiveCutoff(cutoff float32) {
	a.emissiveCutoff = cutoff
	a.cubeShader.Begin()
	a.cubeShader.SetUniformAttr(CubeShaderEmissiveCutoff, cutoff)
	a.cubeShader.End()
}

// SetEmissiveSaturation is how colourful a texel also has to be before it glows. It is
// what keeps bare metal dark: raise it to light only strongly coloured fittings, drop it
// to 0 to go back to a pure brightness mask.
func (a *GlApplication) SetEmissiveSaturation(saturation float32) {
	a.emissiveSaturation = saturation
	a.cubeShader.Begin()
	a.cubeShader.SetUniformAttr(CubeShaderEmissiveSaturation, saturation)
	a.cubeShader.End()
}

func (a *GlApplication) EmissiveSaturation() float32 { return a.emissiveSaturation }

func (a *GlApplication) EmissiveStrength() float32 { return a.emissiveStrength }
func (a *GlApplication) EmissiveCutoff() float32   { return a.emissiveCutoff }

// ToggleEmissiveTextureAt makes every face using the texture on this block side glow,
// or stops it. Returns the texture index and its new state.
func (a *GlApplication) ToggleEmissiveTextureAt(gridPos voxel.Int3, side voxel.CubeSide) (uint16, bool, bool) {
	block := a.CubeMap.GetGlobalBlock(gridPos.X, gridPos.Y, gridPos.Z)
	if block == nil || block.IsAir() {
		return 0, false, false
	}
	textureIndex := block.GetTexture(side)
	return textureIndex, a.CubeMap.ToggleEmissiveTexture(textureIndex), true
}

func (a *GlApplication) SetBloom(threshold, strength float32) {
	a.bloomThreshold = threshold
	a.bloomStrength = strength
}

func (a *GlApplication) BloomThreshold() float32 { return a.bloomThreshold }
func (a *GlApplication) BloomStrength() float32  { return a.bloomStrength }

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

func (a *GlApplication) NewMessageBox() *GuiMessageBox {
	return NewGuiMessageBox(a.UiContext())
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
	guiWidth, guiHeight := a.GuiSize()
	picker.HandleResize(guiWidth, guiHeight, a.UIScale)
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

// TogglePlayerLight switches the light the player carries off, or back on as
// whatever it was last. CyclePlayerLight picks which kind that is.
func (a *GlApplication) TogglePlayerLight() PlayerLightKind {
	return a.lightsUbo.TogglePlayerLight()
}

// CyclePlayerLight steps through off, flashlight and torch. One slot holds all
// three, so the player is never lit by two of their own lights at once.
func (a *GlApplication) CyclePlayerLight() PlayerLightKind {
	return a.lightsUbo.CyclePlayerLight()
}

func (a *GlApplication) PlayerLight() PlayerLightKind {
	return a.lightsUbo.PlayerLight()
}

// SetSkyFog sets how many degrees above the horizon the sky fades in over, 0 for a hard horizon.
func (a *GlApplication) SetSkyFog(degrees float32) {
	a.skyboxShader.Begin()
	a.skyboxShader.SetUniformAttr(SkyboxShaderHorizonFade, degrees)
	a.skyboxShader.End()
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

// SaveScreenshot grabs the default framebuffer rather than the scene buffer. The scene
// buffer is linear HDR now, so reading it back as bytes would clip every highlight and
// skip the tone map; the screen has the finished image, GUI included.
func (a *GlApplication) SaveScreenshot(filename string) error {
	width, height := a.FramebufferWidth, a.FramebufferHeight
	pixels := make([]uint8, width*height*4)

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gl.ReadBuffer(gl.BACK)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))

	return glhf.WritePixelsToPNG(filename, pixels, width, height)
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

	// The map owns its lighting. This replaces rather than appends, so a hot reload
	// or a second map does not leave the previous map's lights burning, and a map
	// with no lights is dark instead of inheriting someone else's.
	//
	// A baked light never reaches the light list: it seeds the voxel flood fill,
	// where it costs one bake at load and nothing per frame afterwards. That is what
	// a torch is, and it is why a map can hold hundreds of them while the dynamic
	// list is capped at MaxLights.
	lights, torches := splitMapLights(loadResult.Lights, a.shadowResolution)
	for _, torch := range torches {
		cubeMap.AddTorchLight(torch)
	}
	if dropped := a.SetStaticLights(lights...); dropped > 0 {
		// Silently dropping them is how a map ends up mysteriously darker than it
		// was authored. Bake the ones that do not move instead.
		a.Toast(fmt.Sprintf("%d of the map's %d dynamic lights did not fit under the %d slot cap",
			dropped, len(lights), MaxLights))
	}

	return entitySpawns
}

// OnResize takes the new window size in screen coordinates. Everything measured in
// pixels - the viewport, the scissor box, the offscreen buffers - is asked of GLFW
// rather than derived from the monitor's content scale, because that ratio is not a
// constant. Measured on a retina Mac: a 640x480 window has a 1280x960 framebuffer,
// but the same window fullscreen is 1920x1200 with a 1920x1200 framebuffer. Scaling
// the window size by the content scale of 2 asked for a 3840x2400 viewport on that
// 1920x1200 framebuffer, which is why fullscreen showed one quarter of the scene.
// GuiScale is the current GUI scale factor.
func (a *GlApplication) GuiScale() float32 {
	return a.guiScale
}

// SetGuiScale resizes the GUI coordinate space, which scales everything drawn in
// it: text, the crosshair, the menus, the minimap and the weapon.
func (a *GlApplication) SetGuiScale(scale float32) {
	if scale <= 0 {
		return
	}
	a.guiScale = scale
	a.OnResize(a.WindowWidth, a.WindowHeight)
}

// GuiSize is the GUI coordinate space: the window divided by the GUI scale. Lay
// GUI elements out in these, not in window coordinates, or they stop scaling.
func (a *GlApplication) GuiSize() (int, int) {
	return int(float32(a.WindowWidth) / a.guiScale), int(float32(a.WindowHeight) / a.guiScale)
}

// GuiCoords converts a window position - a mouse event, say - into GUI coordinates,
// so hit testing lands where the element was drawn.
func (a *GlApplication) GuiCoords(x, y float64) (float64, float64) {
	return x / float64(a.guiScale), y / float64(a.guiScale)
}

func (a *GlApplication) OnResize(width int, height int) {
	a.WindowWidth = width
	a.WindowHeight = height
	a.FramebufferWidth, a.FramebufferHeight = a.Window.GetFramebufferSize()

	// Viewport and scissor box together: SCISSOR_TEST is on for the whole run.
	glhf.Bounds(0, 0, a.FramebufferWidth, a.FramebufferHeight)

	println(fmt.Sprintf("Resize: window %dx%d, framebuffer %dx%d, mode %s",
		width, height, a.FramebufferWidth, a.FramebufferHeight, WindowModeNames[a.WindowMode()]))

	a.Camera.SetScreenSize(width, height)

	guiWidth, guiHeight := a.GuiSize()
	projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(guiWidth, guiHeight)
	a.guiShader.Begin()
	a.guiShader.SetUniformAttr(GuiShaderProjection, projectionMatrix)
	a.guiShader.End()
	if a.postFrameBuffer != nil {
		a.postFrameBuffer.Resize(a.FramebufferWidth, a.FramebufferHeight)
		a.initBloomBuffers(a.FramebufferWidth, a.FramebufferHeight)
	}
	if a.minimap != nil {
		a.minimap.Resize(guiWidth, guiHeight)
	}
	a.ResizeHandler(width, height)
}

func (a *GlApplication) SetCursor(cursorImageFile string, hotX, hotY int) {
	img := Disk.Image(cursorImageFile)
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

// AttachLightTo parents a point light to a game object: it moves with the object
// and switches itself off the frame the object dies.
func (a *GlApplication) AttachLightTo(obj *GameObject, color mgl32.Vec3, intensity, reach float32) {
	a.lightsUbo.AttachLightTo(obj, color, intensity, reach)
}

// DetachLight switches the tracked light off but keeps its slot.
func (a *GlApplication) DetachLight() {
	a.lightsUbo.DetachLight()
}

// AddLights appends to the scene's lights, keeping the ones already there. Returns
// how many did not fit under MaxLights.
func (a *GlApplication) AddLights(infos ...LightInfo) int {
	return a.lightsUbo.AddLights(infos...)
}

// SetStaticLights replaces every light in the scene except the player's, which
// lives in slot 0. A map that brings its own lighting calls this on load.
func (a *GlApplication) SetStaticLights(infos ...LightInfo) int {
	return a.lightsUbo.SetStaticLights(infos...)
}

// ShadowResolution is the edge length of one layer of the shadow atlas. Callers
// that build their own lights need it to size the light's projection.
func (a *GlApplication) ShadowResolution() int {
	return a.shadowResolution
}

// ShadowPasses reports how many lights actually re-rendered their depth map last
// frame, out of how many are switched on. Lights whose frustum and scene are
// unchanged reuse the map from the previous frame.
func (a *GlApplication) ShadowPasses() (rendered, lit int) {
	return a.shadowPassCount, a.lightsUbo.LitCount()
}

// ShadowCacheState exposes the two inputs the shadow cache keys on. If rendered
// stays at the lit count while standing still, one of these is churning: a
// climbing mesh generation means the world keeps re-meshing, a changing scene
// hash with a steady generation means a caster is moving.
func (a *GlApplication) ShadowCacheState() (meshGeneration uint64, sceneHash uint64) {
	return a.CubeMap.MeshGeneration(), a.shadowSceneSignature()
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
		// Blending stays off by default: only the transparent sprite pass and the
		// 2D GUI need it. Left on, every opaque chunk and mesh fragment went
		// through the blend unit for nothing.
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		gl.BlendEquation(gl.FUNC_ADD)
		gl.Disable(gl.BLEND)
		gl.Enable(gl.SCISSOR_TEST)
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
	// The voxel lookup buffer is strided by the chunk-aligned grid size, not by the
	// dimensions LDtk reports, so the uniform has to come from the map itself.
	gridX, gridY, gridZ := voxelMap.VoxelDimensions()
	shader.SetUniformAttr(CubeShaderMapSize, glhf.IntVec4{gridX, gridY, gridZ, 0})
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
				// No light is seeded here. Torch light is owned by
				// Map.bakeEmissiveTorchlight, which clears before it floods, and
				// nothing reads sun light at all - GetSunLight has no callers. A
				// level of 14 on every block in the map was a trap waiting for the
				// first load where the bake had not run yet.
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
