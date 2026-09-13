package quads

import (
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"image"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
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
	TimeFactor         float64
	CeilingColor       mgl32.Vec4

	CubeMap *voxel.Map

	ShouldDrawAABBs      bool
	ShouldDrawChunkAABBs bool
	ShouldDrawOBBs       bool
	ShouldDrawPaths      bool
	ShouldDrawMeshes     bool
	ShouldDrawSprites    bool
	ShouldDrawParticles  bool
	ShouldDrawDebugInfo  bool
	// ShouldShowPerformance gates the frame cost measurement and the title that
	// reports it. Off means the work is not done at all, not merely hidden: the
	// clock read, the ring write and the sort behind the title all stop.
	ShouldShowPerformance  bool
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
	// skyboxSpin is how far the sky has turned, about skyboxAxis - see
	// SetSkyboxSpin.
	skyboxSpin float32
	skyboxAxis mgl32.Vec3
	mainUbo    *glhf.UniformBufferObject
	lightsUbo  *LightsUBO
	// The one light slot that follows a game object around. -1 until something asks
	// for it; the slot is then kept for good, see DetachLight.

	colorBatch *glhf.SpriteBatch

	shadowBuffer    *glhf.Frame
	postFrameBuffer *glhf.Frame
	// thumbBuffer is where a model is drawn on its own for the editor's browser.
	// One buffer for every preview: they are all the same size, and a folder of
	// eighty models is eighty draws into it rather than eighty of it.
	thumbBuffer *glhf.Frame
	// ambient is what SetAmbientLight was last given, so a pass that has to
	// change it - a preview, which is lit flat - can put the level's own back.
	ambient mgl32.Vec4
	// sun is what SetSunLight was last given, kept for the same reason.
	sun                mgl32.Vec4
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
	profile                             frameProfile

	// The last workTimeWindow work times in wrap-around order, plus a scratch
	// slice to sort a copy of them in - see WorkTimePercentiles.
	workTimes     [workTimeWindow]float64
	workTimeCount int
	workTimeSort  []float64

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

	// The two atlases the editor writes into, each with the record of which library
	// texture is in which of its slots. Everything that picks a texture picks it out
	// of TexLibraryRoot and lands it in one of these.
	MapTextures    *AtlasIndex
	SpriteTextures *AtlasIndex
	// templateMapAtlas is the atlas in assets/base, which no level draws with:
	// it is what a level without an atlas of its own is given a copy of.
	templateMapAtlas string

	UIAtlas *glhf.TextureAtlasFixed
	UIFont  BitmapFont

	spriteBatches    []*glhf.SpriteBatch
	shadowResolution int
	shadowPassCount  int
	// casterScratch is gathered once per frame and reused across every light's
	// shadowSceneSignature call, so filtering by reach costs a distance check per
	// caster per light rather than a fresh map walk per light.
	casterScratch []mgl32.Vec3

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

		window, terminateFunc := InitOpenGLWindow(windowTitle, screenW, screenH, false)

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
			ShouldDrawDebugInfo: true, ShouldShowPerformance: true,
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

		glApp.MapTextures = NewAtlasIndex(glApp.AtlasMap)
		glApp.MapTextures.OnGrow(glApp.SetCubeAtlasGridSize)
		glApp.templateMapAtlas = glApp.AtlasMap.SourceFile()
		glApp.SpriteTextures = NewAtlasIndex(glApp.AtlasSprites)

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
		glApp.ambient = mgl32.Vec4{1, 1, 1, DefaultAmbientStrength}
		mainUbo.SetAttribute(UboGlobalLightColor, glApp.ambient)
		mainUbo.SetAttribute(UboToneMapParams, mgl32.Vec2{DefaultExposure, DefaultGamma})
		mainUbo.SetAttribute(UboShadowParams, mgl32.Vec2{DefaultShadowNormalOffset, DefaultShadowDepthBias})
		mainUbo.SetAttribute(UboSunLight, mgl32.Vec4{}) // BufferData left it undefined
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
		// hardcoded here now live in raymap.json as Light entities, where they can be
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

			a.profile.mark("update: toasts, paths, minimap")
			{ // Update BEGIN
				a.updateToasts(deltaTime)

				if a.Pathfinder != nil {
					a.Pathfinder.Update(deltaTime)
				}
				if a.minimap != nil {
					a.minimap.Update()
				}
				a.profile.mark("update: game")
				a.UpdateFunc(deltaTime)

				a.profile.mark("update: sprites, meshes")
				a.Sprites.Update(deltaTime)
				a.Meshes.Update(deltaTime)
				a.profile.mark("update: particles")
				a.UpdateParticles(deltaTime)
				a.profile.mark("update: player lights")
				a.lightsUbo.UpdatePlayerLight()
				a.lightsUbo.UpdateTrackedLight()
				a.profile.mark("update: upload lights")
				a.lightsUbo.Flush()
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

			a.profile.mark("shadow: setup")
			{ // BEGIN SHADOW MAPPING
				gl.Viewport(0, 0, int32(a.shadowResolution), int32(a.shadowResolution))
				a.shadowBuffer.Begin()
				// ponytail: no front-face culling in the depth pass. It is the usual
				// cure for shadow acne, but billboard sprites are single quads, so
				// culling their front faces would drop them out of the shadow map
				// entirely. The slope-scaled bias in ShadowCalculation covers it.

				casterPositions := a.gatherCasterPositions()
				a.shadowPassCount = 0
				lights := a.lightsUbo.Lights()
				for i := range lights {
					light := &lights[i]
					// A light that is off contributes nothing, one with no atlas layer
					// casts no shadow at all, and a light whose frustum and scene are
					// unchanged still has a valid depth map from last frame.
					if light.IsOff() || light.ShadowLayer < 0 {
						continue
					}
					sceneSignature := a.shadowSceneSignature(light, casterPositions)
					if !light.NeedsShadowUpdate(sceneSignature) {
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

			a.profile.mark("scene: clear")
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
				a.profile.mark("scene: particles, debug")
				if a.ShouldDrawParticles {
					a.DrawParticles(a.Camera)
				}

				a.draw3dDebug()

				a.shadowBuffer.DepthTextureArray().End()
				a.postFrameBuffer.End() // will effectively do "gl.BindFramebuffer(gl.FRAMEBUFFER, 0)"
			} // End Render 3D Scene

			a.profile.mark("bloom, post")
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
				glhf.ActiveTexture(gl.TEXTURE0)
			} // End bloom, tone mapping and the scene to the screen

			// DRAW UI ON TOP OF POSTPROCESSING
			a.profile.mark("gui")
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

			if a.ShouldShowPerformance {
				// Everything left in the frame is waiting for the compositor, so the
				// work is measured here rather than at the swap - see WorkTimePercentiles.
				a.recordWorkTime(glfw.GetTime() - time)
				if a.Ticks%titleUpdateTicks == 0 {
					a.Window.SetTitle(a.workTimeTitle())
				}
			} else if a.workTimeCount > 0 {
				// Switched off: drop the window rather than keep it, so turning the
				// readout back on measures the scene it is turned on in, and take the
				// last numbers out of the title rather than leave them there to be
				// read as current.
				a.workTimeCount = 0
				a.Window.SetTitle(windowTitle)
			}

			glhf.CheckGLError("end of frame") // no-op unless built with -tags debugGL

			// Grabbed off the back buffer, so it has to happen before the swap.
			if a.captureDue() {
				a.capture()
				shouldQuit = a.shotsTaken >= a.shotCount
			}

			a.profile.mark("swap, events")
			a.Window.SwapBuffers()
			glfw.PollEvents()
			a.profile.endFrame()
			a.Ticks++

			// Measured after SwapBuffers, so the wait for vblank is inside the frame
			// it belongs to. With vsync on that makes every number the refresh rate,
			// which is why -bench turns vsync off.
			if a.benchmark != nil {
				wasWarm := a.benchmark.warm()
				done := a.benchmark.record(elapsed)
				if !wasWarm && a.benchmark.warm() {
					a.profile.reset()
				}
				if done {
					fmt.Print(a.benchmark.Report(a.benchmarkLabel, a.SceneSummary()))
					fmt.Print(a.profile.report())
					shouldQuit = true
				}
			}
		})
	}
}

// workTimeWindow is how many frames the title's numbers are taken over, and
// titleUpdateTicks is how often they are recomputed. Two seconds of window at 60
// Hz is long enough that one hitch cannot move the middle of it, and short enough
// to notice walking into a room that costs more to draw. The window is a count of
// frames rather than a span of seconds so that sorting it is a fixed cost.
const (
	workTimeWindow   = 120
	titleUpdateTicks = 30
)

// windowTitle is what the window is called when it is not reporting frame cost.
const windowTitle = "fps"

// recordWorkTime keeps the last workTimeWindow work times, oldest overwritten
// first.
func (a *GlApplication) recordWorkTime(worked float64) {
	a.workTimes[a.workTimeCount%workTimeWindow] = worked
	a.workTimeCount++
}

// WorkTimePercentiles is the middle and the slow tail of the window, in seconds.
//
// Work time is everything this frame did before handing the finished picture over:
// the update, the shadow passes, the scene, the post pass and the GUI. It stops
// short of SwapBuffers on purpose. What SwapBuffers spends is a wait for the
// compositor to take the surface at the next refresh, which is time nothing could
// have used, and measuring it made the readout say more about the window server
// than about the renderer. A whole frame time cannot say whether there is room to
// draw more. This can: it is the part of the refresh interval that is actually
// yours, and the interval it has to fit inside is in the title next to it.
func (a *GlApplication) WorkTimePercentiles() (median, p95 float64) {
	filled := a.filledWorkTimes()
	if filled == 0 {
		return 0, 0
	}
	a.workTimeSort = append(a.workTimeSort[:0], a.workTimes[:filled]...)
	sort.Float64s(a.workTimeSort)
	return percentile(a.workTimeSort, 0.5), percentile(a.workTimeSort, 0.95)
}

// WorkFramesOverBudget is how many frames in the window cost more than one refresh
// interval, out of how many the window holds so far.
//
// The percentiles say there is room left; this says whether it was ever used up.
// They are not the same question, because a median of 1 ms and a screen missing
// every fourth refresh can be true at once - the p95 only reaches the worst one
// frame in twenty, and a stutter people notice is rarer than that. Zero over
// budget is what a frame rate pinned to the display looks like from the inside.
func (a *GlApplication) WorkFramesOverBudget(budget float64) (over, of int) {
	filled := a.filledWorkTimes()
	if budget <= 0 {
		return 0, filled
	}
	for _, worked := range a.workTimes[:filled] {
		if worked > budget {
			over++
		}
	}
	return over, filled
}

// filledWorkTimes is how much of the ring has been written, which is short of the
// whole window only for the first workTimeWindow frames of a run.
func (a *GlApplication) filledWorkTimes() int {
	if a.workTimeCount > workTimeWindow {
		return workTimeWindow
	}
	return a.workTimeCount
}

// workTimeTitle is the window title: what a frame costs, against what there is to
// spend. The budget is left out when GLFW has no monitor to ask for a refresh
// rate, which is a display asleep or a session with none.
func (a *GlApplication) workTimeTitle() string {
	median, p95 := a.WorkTimePercentiles()
	title := fmt.Sprintf("draw ms median %.2f p95 %.2f", median*1000, p95*1000)
	if budget := refreshInterval(); budget > 0 {
		over, of := a.WorkFramesOverBudget(budget)
		title += fmt.Sprintf(" of %.2f, %d of %d over", budget*1000, over, of)
	}
	return title
}

// refreshInterval is how long one refresh of the primary monitor lasts, and 0 when
// there is no monitor to ask.
func refreshInterval() float64 {
	monitor := glfw.GetPrimaryMonitor()
	if monitor == nil {
		return 0
	}
	if rate := monitor.GetVideoMode().RefreshRate; rate > 0 {
		return 1.0 / float64(rate)
	}
	return 0
}

// gatherCasterPositions collects every caster's world position once per frame,
// reusing casterScratch, so every light's shadowSceneSignature call filters the
// same slice instead of each re-walking the sprite and mesh maps.
func (a *GlApplication) gatherCasterPositions() []mgl32.Vec3 {
	a.casterScratch = a.casterScratch[:0]
	visit := func(pos mgl32.Vec3) { a.casterScratch = append(a.casterScratch, pos) }
	a.Sprites.ForEachCaster(visit)
	a.Meshes.ForEachCaster(visit)
	return a.casterScratch
}

// shadowSceneSignature hashes everything one light's depth map depends on: the
// world mesh generation plus the position of every caster within that light's own
// reach. Filtering by reach is what keeps a mover on one side of the map from
// invalidating a static light's cache on the other side - see
// shadowSceneSignatureFor for why that filter is exact, not approximate.
func (a *GlApplication) shadowSceneSignature(light *LightInfo, casterPositions []mgl32.Vec3) uint64 {
	return shadowSceneSignatureFor(light.GetPosition(), light.GetFarPlaneDist(), a.CubeMap.MeshGeneration(), casterPositions)
}

// draw3dScene draws the world from the observer's point of view. betweenPasses, if
// given, runs after the opaque geometry and before the alpha-blended sprites - that
// is where the skybox goes, so translucent sprites blend against the sky instead of
// the clear colour. The shadow pass passes nil: it has no sky and writes depth only.
// sceneSections and shadowSections name draw3dScene's parts in the frame profile,
// one set per pass, so the camera's meshes and a light's meshes are told apart.
var (
	sceneSections  = [6]string{"scene: map", "scene: meshes", "scene: sprites, decals", "scene: sky", "scene: panes", "scene: transparent"}
	shadowSections = [6]string{"shadow: map", "shadow: meshes", "shadow: sprites", "shadow: (sky)", "shadow: (panes)", "shadow: transparent"}
)

func (a *GlApplication) draw3dScene(observer core.Observer, chunkShader, meshShader, spriteShader *glhf.Shader, betweenPasses func()) {
	// Everything below culls thousands of things against a view that does not
	// move while it draws - see core.FrozenObserver.
	observer = core.Freeze(observer)
	forShadows := spriteShader == a.quadShaderDepth
	sections := &sceneSections
	if forShadows {
		sections = &shadowSections
	}
	a.profile.mark(sections[0])
	a.setMainUBOCam(observer.GetPosition(), observer.GetForward(), observer.GetProjectionViewMatrix())

	// Cull against the observer, not the camera: a shadow pass has to include every
	// caster inside the light's frustum, including the ones behind the player.
	a.CubeMap.DrawWithShader(observer, chunkShader)

	a.profile.mark(sections[1])
	if a.ShouldDrawMeshes {
		a.Meshes.Draw(observer, meshShader, forShadows)
	}
	a.profile.mark(sections[2])
	// Sprites keep blending: the atlases have soft edges and the shader only
	// discards fully transparent texels. Chunks and meshes are opaque and no
	// longer pay for the blend unit.
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

	a.profile.mark(sections[3])
	if betweenPasses != nil {
		betweenPasses()
	}

	a.profile.mark(sections[4])
	// The glass in the windows, after the sky and never in a depth pass: a pane
	// writes no depth, so a sky drawn after it would paint over it, and a pane
	// in a light's depth map would shadow the room it is meant to let light
	// into. See SpriteDrawer.AddPane.
	if !forShadows {
		gl.Enable(gl.BLEND)
		a.Sprites.DrawPanes(observer, spriteShader)
		gl.Disable(gl.BLEND)
	}

	a.profile.mark(sections[5])
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
	// No application is no window, and a game with no window has nowhere to put
	// a toast - which is what a test that drives the game's own logic without
	// opening one is. Saying something to nobody is not an error.
	if a == nil {
		return nil
	}
	label := a.NewLabel()
	label.SetPosition(mgl32.Vec2{10, 180})
	label.SetText(labelText)
	toast := NewGuiToast(a.UiContext(), label)
	a.queuedToasts = append(a.queuedToasts, toast)
	return toast
}

func (a *GlApplication) SetupUnitCubes() {
	meshFile := filepath.Join("assets", "meshes", "cube", "mesh.glb")
	mesh := LoadGLTFWithTextures(meshFile)
	mesh.UploadVertexData(a.meshShader)
	a.UnitCube = NewUnitCube(a.meshShader, mesh)
}

func (a *GlApplication) SetupParticleSystem(name ParticleName, count int) {
	ps := glhf.NewParticleSystem(count)
	a.particleSystems[name] = ps
}
func (a *GlApplication) SetupPathfinder(isSolid func(x, y, z int32) bool, maxHeight int32) {
	a.Pathfinder = NewPathfindingSystem(isSolid, maxHeight)
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
	a.profile.start()
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
	a.ambient = colour.Vec4(strength)
	a.mainUbo.Begin()
	a.mainUbo.SetAttribute(UboGlobalLightColor, a.ambient)
	a.mainUbo.End()
}

// SetSunLight is the directional light from the sky: towards is the unit vector
// pointing at the sun from the ground, and a strength of 0 switches it off.
func (a *GlApplication) SetSunLight(towards mgl32.Vec3, strength float32) {
	a.sun = towards.Vec4(strength)
	a.mainUbo.Begin()
	a.mainUbo.SetAttribute(UboSunLight, a.sun)
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

// SetEmissiveCutoff is the luminance above which a texel of an emissive surface glows -
// a wall face and a mesh alike, which is why it is set on both shaders. Turn it down
// towards 0 to light the whole tile, up to pick out only the brightest screens on a
// busy panel.
func (a *GlApplication) SetEmissiveCutoff(cutoff float32) {
	a.emissiveCutoff = cutoff
	setShaderFloat(a.cubeShader, CubeShaderEmissiveCutoff, cutoff)
	setShaderFloat(a.meshShader, MeshShaderEmissiveCutoff, cutoff)
}

// SetEmissiveSaturation is how colourful a texel also has to be before it glows. It is
// what keeps bare metal dark: raise it to light only strongly coloured fittings, drop it
// to 0 to go back to a pure brightness mask.
func (a *GlApplication) SetEmissiveSaturation(saturation float32) {
	a.emissiveSaturation = saturation
	setShaderFloat(a.cubeShader, CubeShaderEmissiveSaturation, saturation)
	setShaderFloat(a.meshShader, MeshShaderEmissiveSaturation, saturation)
}

// setShaderFloat is the Begin/set/End every one of these needs. The thresholds live
// in the shared fragment header but a uniform is per program, so each shader that
// masks emissive texels has to be handed the same number.
// DefaultAmbientOcclusionStrength is how dark a corner enclosed on both sides gets:
// the light from everywhere drops to 1 minus this.
const DefaultAmbientOcclusionStrength = float32(0.6)

// SetAmbientOcclusion is the block faces' ambient occlusion at this strength, 0 for
// none. 0 skips the neighbour reads altogether, so it is also the way to measure
// what they cost.
func (a *GlApplication) SetAmbientOcclusion(strength float32) {
	setShaderFloat(a.cubeShader, CubeShaderAmbientOcclusion, strength)
}

func setShaderFloat(shader *glhf.Shader, uniform int, value float32) {
	shader.Begin()
	shader.SetUniformAttr(uniform, value)
	shader.End()
}

func (a *GlApplication) EmissiveSaturation() float32 { return a.emissiveSaturation }

func (a *GlApplication) EmissiveStrength() float32 { return a.emissiveStrength }
func (a *GlApplication) EmissiveCutoff() float32   { return a.emissiveCutoff }

// ToggleEmissiveTextureAt makes every face using the texture on this block side glow,
// or stops it. Returns the texture index and its new state.
//
// The map holds the set the renderer reads every frame; the atlas holds the record of
// it. Both are written here, so a texture marked with G is still marked next time the
// atlas is loaded instead of being forgotten with the session.
func (a *GlApplication) ToggleEmissiveTextureAt(gridPos voxel.Int3, side voxel.CubeSide) (uint16, bool, bool) {
	block := a.CubeMap.GetGlobalBlock(gridPos.X, gridPos.Y, gridPos.Z)
	if block == nil || block.IsAir() {
		return 0, false, false
	}
	textureIndex := block.GetTexture(side)
	nowGlowing := a.CubeMap.ToggleEmissiveTexture(textureIndex)
	if err := a.MapTextures.SetEmissive(textureIndex, nowGlowing); err != nil {
		// The wall in front of you glows either way; only the record failed.
		log.Printf("[atlas] could not record texture %d as emissive: %v", textureIndex, err)
	}
	return textureIndex, nowGlowing, true
}

func (a *GlApplication) SetBloom(threshold, strength float32) {
	a.bloomThreshold = threshold
	a.bloomStrength = strength
}

func (a *GlApplication) BloomThreshold() float32 { return a.bloomThreshold }
func (a *GlApplication) BloomStrength() float32  { return a.bloomStrength }

// SetSkyboxSpin turns the whole sky about an axis, in radians. It is how a level
// that is itself a turning body shows that it turns: the blocks stay put and the
// stars go round, which is the same relative motion and costs one matrix. A zero
// axis is Y.
func (a *GlApplication) SetSkyboxSpin(radians float32, axis mgl32.Vec3) {
	if axis.Len() == 0 {
		axis = mgl32.Vec3{0, 1, 0}
	}
	a.skyboxSpin, a.skyboxAxis = radians, axis.Normalize()
}

func (a *GlApplication) drawSkybox(cubemap *glhf.Cubemap) {
	projection := a.Camera.GetProjectionMatrix()
	view := a.Camera.GetViewMatrix().Mat3().Mat4() // remove translation
	if a.skyboxSpin != 0 {
		view = view.Mul4(mgl32.HomogRotate3D(a.skyboxSpin, a.skyboxAxis))
	}
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

// SetCubeAtlasGridSize re-tells the cube shader how the terrain atlas is laid out.
// The shader turns a texture index into UVs itself, so a picked texture that made
// the atlas taller has to reach it or every wall in the map samples the old grid.
func (a *GlApplication) SetCubeAtlasGridSize() {
	gridSize := glhf.IntVec2{int32(a.AtlasMap.GridWidth()), int32(a.AtlasMap.GridHeight())}
	a.cubeShader.Begin()
	a.cubeShader.SetUniformAttr(CubeShaderAtlasGridSize, gridSize)
	a.cubeShader.End()
}

// NewTexBrowser opens the texture library. Each directory it shows is a texture of
// its own, so it is handed in and out of the 2D pass as the user walks around.
func (a *GlApplication) NewTexBrowser(dest *AtlasIndex) *TexBrowser {
	tileW, tileH := dest.Atlas.GetAtlasItemSize()
	return NewTexBrowser(tileW, tileH, a.SetupSpriteBatch, a.DropSpriteBatch)
}

func (a *GlApplication) OpenTextureCube(dest *AtlasIndex, preselection map[voxel.CubeSide]uint16) *GuiTexturePickerCube {
	return NewGuiTexturePickerCube(a.UiContext(), dest, a.NewTexBrowser(dest), preselection, func(s string) {
		a.Toast(s)
	})
}

func (a *GlApplication) OpenAtlasPicker(atlasForSelection glhf.TextureAtlas) *GuiAtlasPicker {
	picker := NewGuiAtlasPicker(a.UiContext(), atlasForSelection)
	guiWidth, guiHeight := a.GuiSize()
	picker.HandleResize(guiWidth, guiHeight, a.UIScale)
	return picker
}

// OpenTextureBrowser is the plain "pick one texture" widget: the library, browsed,
// with the chosen file copied into dest and its slot handed to the caller.
func (a *GlApplication) OpenTextureBrowser(dest *AtlasIndex) *GuiAtlasPicker {
	picker := NewGuiAtlasPicker(a.UiContext(), nil)
	// The browser first: it is what the picker has an atlas to lay out at all.
	picker.SetBrowser(a.NewTexBrowser(dest), dest, func(s string) { a.Toast(s) })
	guiWidth, guiHeight := a.GuiSize()
	picker.HandleResize(guiWidth, guiHeight, a.UIScale)
	return picker
}

// OpenAssetBrowser is the "pick a thing out of a folder of things" widget: a
// tree of assets browsed the way the texture library is, with each cell a picture
// of the asset in it rather than a folder icon. Choosing one is choosing its
// folder, so the index a caller gets back is turned into a name through the
// browser it is handed.
//
// isAsset and tile are the caller's, because what makes a folder an asset is the
// caller's business and so is what one looks like: a model is a scenery.json
// beside a mesh and its picture is a render of it, a sprite kind is a sprite.json
// beside a sheet and its picture is one frame off that sheet, a weapon is a
// weapon.json and its picture is the icon it lies on the floor as.
// startDir is the folder to open at, or "" for wherever this root was left last.
func (a *GlApplication) OpenAssetBrowser(root, startDir string, isAsset func(dir string) bool, tile func(dir string, w, h int) *image.RGBA) (*GuiAtlasPicker, *TexBrowser) {
	browser := &TexBrowser{
		root:       root,
		tileW:      ThumbSize,
		tileH:      ThumbSize,
		setupBatch: a.SetupSpriteBatch,
		dropBatch:  a.DropSpriteBatch,
		pick:       func(p string, isDir bool) bool { return isDir && isAsset(root+"/"+p) },
		tile:       func(p string, w, h int) *image.RGBA { return tile(root+"/"+p, w, h) },
	}
	if startDir == "" {
		startDir = lastBrowsedDir[root]
	}
	browser.Open(path.Clean("/" + startDir)[1:]) // no leading slash, and "." is the root

	picker := NewGuiAtlasPicker(a.UiContext(), nil)
	picker.SetBrowser(browser, nil, func(s string) { a.Toast(s) })
	guiWidth, guiHeight := a.GuiSize()
	picker.HandleResize(guiWidth, guiHeight, a.UIScale)
	return picker, browser
}

// OpenListBrowser is OpenAssetBrowser over a list the caller keeps rather than
// a folder on disk: one cell per name, drawn by tile, and the index a click
// hands back is the position in that list. title is the breadcrumb.
func (a *GlApplication) OpenListBrowser(title string, names []string, tile func(index, w, h int) *image.RGBA) *GuiAtlasPicker {
	browser := &TexBrowser{
		root:       title,
		tileW:      ThumbSize,
		tileH:      ThumbSize,
		setupBatch: a.SetupSpriteBatch,
		dropBatch:  a.DropSpriteBatch,
		tile: func(p string, w, h int) *image.RGBA {
			index, _ := strconv.Atoi(p)
			return tile(index, w, h)
		},
	}
	for index, name := range names {
		browser.entries = append(browser.entries, TexEntry{Path: strconv.Itoa(index), Name: name})
	}
	browser.upload()

	picker := NewGuiAtlasPicker(a.UiContext(), nil)
	picker.SetBrowser(browser, nil, func(s string) { a.Toast(s) })
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

	glhf.BindDefaultFramebuffer()
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

// UseMapAtlasIn makes the atlas in a level's folder the one its voxels draw
// with and the editor picks textures into, so every level bakes its own and none
// renumbers another's. A folder without one is given a copy of the template
// first - its .paths record too - which is also how a level that indexed into
// the old shared atlas keeps drawing what it drew. The same file again is left
// as it is: a hot reload must not drop textures picked since the last save.
func (a *GlApplication) UseMapAtlasIn(dir string) error {
	file := path.Join(filepath.ToSlash(dir), path.Base(a.templateMapAtlas))
	if a.AtlasMap.SourceFile() == file {
		return nil
	}
	if _, err := os.Stat(file); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := copyFile(a.templateMapAtlas, file); err != nil {
			return err
		}
		if err := copyFile(pathsFile(a.templateMapAtlas), pathsFile(file)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	tileW, tileH := a.AtlasMap.GetAtlasItemSize()
	a.DropSpriteBatch(a.AtlasMap.GetSpriteBatch())
	a.AtlasMap = Disk.TextureAtlasFixed(file, tileW, tileH)
	a.AtlasMap.SetGuiShader(a.guiShader)
	a.AtlasMap.SetMaxSprites(10)
	a.spriteBatches = append(a.spriteBatches, a.AtlasMap.GetSpriteBatch())
	a.MapTextures = NewAtlasIndex(a.AtlasMap)
	a.MapTextures.OnGrow(a.SetCubeAtlasGridSize)
	return nil
}

func copyFile(from, to string) error {
	content, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, content, 0o644)
}

// LoadVoxelMap takes the level's blocks - read off disk or painted by a
// generator, they are the same thing by the time they get here - and makes them
// the scene's: the shader and the atlas they draw with, and the lights the map
// file says burn in them.
//
// There is no other description of the geometry. There used to be a tile grid in
// the map file that this inflated into voxels on every load, which meant a level
// had two shapes that could disagree and only one of them was editable.
func (a *GlApplication) LoadVoxelMap(loadResult core.MapLoadResult, cubeMap *voxel.Map) {
	a.CubeMap = cubeMap
	cubeMap.SetShader(a.cubeShader)
	cubeMap.SetModelTransformAttrIndex(CubeShaderModelTransform)
	cubeMap.SetTerrainTexture(a.AtlasMap)

	a.cubeShader.Begin()
	a.cubeShader.SetUniformAttr(CubeShaderAtlasGridSize, glhf.IntVec2{int32(a.AtlasMap.GridWidth()), int32(a.AtlasMap.GridHeight())})
	// The voxel lookup buffer is strided by the chunk-aligned grid size, not by
	// the level's own bounds, so the uniform has to come from the map itself.
	gridX, gridY, gridZ := cubeMap.VoxelDimensions()
	a.cubeShader.SetUniformAttr(CubeShaderMapSize, glhf.IntVec4{gridX, gridY, gridZ, 0})
	a.cubeShader.End()

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

// AddMovableLight appends a light and returns the slot it landed in, for MoveLight.
func (a *GlApplication) AddMovableLight(info LightInfo) int {
	return a.lightsUbo.AddMovableLight(info)
}

// MoveLight stands the light in a slot somewhere else and points it somewhere else.
func (a *GlApplication) MoveLight(index int, position, forward mgl32.Vec3) {
	a.lightsUbo.MoveLight(index, position, forward)
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

// ShadowCacheState exposes the two inputs the shadow cache keys on, for the debug
// overlay. If rendered stays at the lit count while standing still, one of these
// is churning: a climbing mesh generation means the world keeps re-meshing, a
// changing scene hash with a steady generation means some caster is moving.
// sceneHash sums every caster unfiltered, unlike the per-light signature the
// cache itself uses - a diagnostic total, not a cache key.
func (a *GlApplication) ShadowCacheState() (meshGeneration uint64, sceneHash uint64) {
	sceneHash = a.CubeMap.MeshGeneration() * 0x9E3779B97F4A7C15
	for _, pos := range a.gatherCasterPositions() {
		sceneHash += hashPos(pos)
	}
	return a.CubeMap.MeshGeneration(), sceneHash
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
