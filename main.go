package main

import (
	"flag"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/eassets"
	"github.com/memmaker/goray/events"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
)

func main() {
	defaults := DefaultStressConfig()
	stress := flag.Bool("stress", false, "generate a stress map instead of loading the default map")
	mapName := flag.String("map", defaultMapName, "load this level, by its folder name under assets/maps")
	// The two shadow knobs are also flags, not only config menu sliders, so a
	// before/after can be run from a script instead of from muscle memory.
	shadowOffset := flag.Float64("shadow-offset", float64(quads.DefaultShadowNormalOffset), "shadow normal offset, in shadow texels")
	shadowBias := flag.Float64("shadow-bias", float64(quads.DefaultShadowDepthBias), "flat shadow depth bias")
	benchSeconds := flag.Float64("bench", 0, "run this many seconds, print frame time percentiles and quit (turns vsync off)")
	startWeapon := flag.String("weapon", "", "start holding this weapon, by its folder name")
	autoFire := flag.Bool("autofire", false, "hold the trigger down, for looking at a weapon or measuring what it costs")
	autoAlt := flag.Bool("altfire", false, "hold the second attack button down, which is what a scope or a bayonet is on")
	autoTap := flag.String("tap", "", "tap the trigger instead of holding it: \"down,up\" in tics, for a weapon that only does something on a fresh press")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile here while the benchmark runs")
	shot := flag.String("shot", "", "save a screenshot here and quit, for looking at what a change did without playing")
	shotFirst := flag.Int("shot-at", 90, "which tick to take the first screenshot on")
	shotEvery := flag.Int("shot-every", 6, "ticks between screenshots when more than one is asked for")
	shotCount := flag.Int("shot-count", 1, "how many screenshots to take, numbered, which is how an animation is reviewed")
	shotFPS := flag.Float64("shot-fps", 60, "the frame time a capture pretends every frame took, so two runs land on the same animation frame")
	timeFactor := flag.Float64("time-factor", 1, "scale how fast time flows: 0.5 runs the whole world at half speed, 2 at double")
	shotPitch := flag.Float64("pitch", 0, "start looking this many degrees down, for a capture of something on the floor")
	shotRoll := flag.Float64("roll", 0, "start with the horizon rolled this many degrees, for a capture of what the suit sees")
	autoWalk := flag.Bool("walk", false, "hold the forward key down, for looking at anything that only happens while moving")
	startEditor := flag.String("editor", "", "start in the map editor, in this mode: blocks, props or actors")
	startPicker := flag.String("pick", "", "open the editor mode's own asset browser on this folder, so a capture can photograph it; \".\" for the top")
	autoCycle := flag.Float64("cycle", 0, "switch to the next weapon every this many seconds, for walking the whole folder in one run")
	width := flag.Int("width", 0, "window width, 0 for the default")
	height := flag.Int("height", 0, "window height, 0 for the default")
	stressSize := flag.Int("stress-size", defaults.Size, "stress map edge length in tiles")
	stressPillars := flag.Int("stress-pillars", defaults.PillarSpacing, "stress map: one pillar every N tiles, 0 for none")
	stressSprites := flag.Int("stress-sprites", defaults.SpriteSpacing, "stress map: one barrel every N tiles, 0 for none")
	stressLightCount := flag.Int("stress-lights", defaults.Lights, "stress map: static spotlights on top of the flashlight")
	showOBBs := flag.Bool("obb", false, "draw every collider, which is the F1 menu's Show OBBs from the command line")
	shockScale := flag.Float64("ss-scale", 1, "scale every System Shock sprite by this, on top of the size its own art implies")
	blakeScale := flag.Float64("bs-scale", 1, "scale every Blake Stone sprite by this, on top of the size its own art implies")
	flag.Parse()
	sourceScale["ss_"] = float32(*shockScale)
	sourceScale["bs_"] = float32(*blakeScale)

	var stressConfig *StressConfig
	var mapLoad core.MapLoadResult
	// The level being played, and the file it came from - empty for the stress
	// rig, which is generated rather than loaded. The name is what the editor
	// saves back to and what the benchmark report calls the run.
	loadedMap := *mapName
	// The level's blocks. There is one description of them: generated here, or
	// read out of the voxel file beside the map. Either way, by this point it
	// is the same thing.
	var blocks *voxel.Map
	if *stress {
		stressConfig = &StressConfig{
			Size:          *stressSize,
			PillarSpacing: *stressPillars,
			SpriteSpacing: *stressSprites,
			Lights:        *stressLightCount,
		}
		mapLoad, blocks = loadStressMap(*stressConfig)
		loadedMap = ""
	} else {
		var err error
		if mapLoad, blocks, err = loadMap(loadedMap); err != nil {
			log.Fatalf("%s (levels in %s: %v)", err, mapDir, mapNames())
		}
	}

	if *cpuProfile != "" {
		file, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		if err := pprof.StartCPUProfile(file); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	glhf.Run(func() {
		game := NewGame(stressConfig)
		game.shadowNormalOffset = float32(*shadowOffset)
		game.shadowDepthBias = float32(*shadowBias)
		game.benchSeconds = *benchSeconds
		game.autoFire = *autoFire
		game.autoAlt = *autoAlt
		fmt.Sscanf(*autoTap, "%d,%d", &game.tapDown, &game.tapUp)
		game.autoWalk = *autoWalk
		game.startOBBs = *showOBBs
		game.autoCycle = *autoCycle
		game.mapName = loadedMap
		game.blocks = blocks
		game.startEditorMode = *startEditor
		game.startPickerDir = *startPicker
		game.startTimeFactor = *timeFactor
		game.startWeapon = *startWeapon
		game.shot = shotRun{
			path: *shot, first: *shotFirst, every: *shotEvery, count: *shotCount,
			step: 1 / *shotFPS, pitch: *shotPitch, roll: *shotRoll,
		}
		windowWidth, windowHeight := 640, 480
		if *width > 0 {
			windowWidth = *width
		}
		if *height > 0 {
			windowHeight = *height
		}
		quads.Start(game, mapLoad, windowWidth, windowHeight)
	})
}

// NewGame takes a stress config, or nil for a normal run.
func NewGame(stress *StressConfig) *Game {
	return &Game{
		isMouseCursorCaptured:  true,
		currentWeaponIndex:     -1,
		pickupModeIndex:        pickupIntentional,
		stressConfig:           stress,
		currentGuiScaleIndex:   defaultGuiScaleIndex,
		currentTimeFactorIndex: defaultTimeFactorIndex,
		editorSunStrength:      defaultEditorSunStrength,
		ambientOcclusion:       true,
		aoStrength:             quads.DefaultAmbientOcclusionStrength,
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
	// beamAtlas is the one-column picture an energy weapon's bolt is drawn
	// with, built the first time one is fired - see beam.go.
	beamAtlas *glhf.TextureAtlasFixed

	// actorsActive is whether the NPCs run their own minds at all. Off by
	// default: a level of imported characters is a level of things that
	// immediately run at you, and most of checking an import is looking at
	// them. The F1 menu's Actors page is the switch - see actor_debug.go.
	actorsActive bool

	// cheats is the F1 menu's own page of them - see cheats.go. Only godMode
	// has to be remembered; the rest are things that happen once.
	cheats cheats

	// hudName and hudAmmo are the two lines the HUD says about the weapon in
	// hand, beside its icon.
	hudName *quads.BitmapFontMesh
	hudAmmo *quads.BitmapFontMesh
	// hudHealth is the one line the HUD says about the player.
	hudHealth *quads.BitmapFontMesh
	// editorHUD is the map editor's key legend, drawn in place of the weapon
	// HUD while the editor is on.
	editorHUD *quads.BitmapFontMesh
	// player is whoever the camera is behind: the world object plus the health
	// that makes being shot at mean something. See combat.go.
	player *Player

	relativeHitPos mgl32.Vec3
	entityMenu     *quads.GuiList

	widgetStack                []InputReceiver
	widgetsAreBlockingMovement bool
	// menuPreviousMouseState is what the cursor was doing before the F1 menu's
	// top level opened, for an action buried in one of its sub-menus (Start
	// Map Hot Reload) that needs to close the whole tree and hand the mouse
	// back exactly the way the top level's own close does, not just pop
	// itself the way a submenu's Escape does.
	menuPreviousMouseState bool

	fogStart         float32
	fogEnd           float32
	skyHorizonFade   float32
	exposure         float32
	bloomThreshold   float32
	bloomStrength    float32
	emissiveStrength float32
	// ambientColor and ambientStrength are the level's own flat fill light,
	// kept here as well as pushed to the shader so the F1 menu has something to
	// turn. A level with no lights in it is lit by this and nothing else.
	ambientColor    mgl32.Vec3
	ambientStrength float32
	// editorSunStrength is the sun the editor builds under, see updateEditorSun.
	editorSunStrength float32
	// ambientOcclusion and aoStrength are the rendering menu's switch and slider
	// for the block faces' AO. The switch keeps the strength it turns off.
	ambientOcclusion       bool
	aoStrength             float32
	emissiveCutoff         float32
	emissiveSaturation     float32
	shadowNormalOffset     float32
	shadowDepthBias        float32
	currentResolutionIndex int
	currentGuiScaleIndex   int
	currentTimeFactorIndex int
	startTimeFactor        float64
	currentWindowModeIndex int

	benchSeconds float64
	// bobTics is the clock the weapon's sway is a function of, in Doom tics.
	bobTics float64
	// bobSpeed is weaponBob's speed input, eased toward the player's actual
	// speed each Update tick rather than read fresh at draw time - see
	// updateWeapon. The physics velocity it comes from is only ever correct
	// on the frames the fixed-timestep physics step actually runs, and on a
	// frame it doesn't, what's left is that tick's own pre-collision wish
	// velocity - full speed, straight into whatever wall stopped it a moment
	// ago. Sampling that raw at draw time is what made the weapon sprite
	// snap between "still" and "full swing" once every couple of frames
	// while walking into a wall; easing it here instead means one such frame
	// only nudges the average, so the sway stays visually smooth regardless
	// of which frames physics actually ran on.
	bobSpeed float64
	// warnedSpinning keeps a weapon whose script cannot settle from saying so
	// every frame for as long as it is held.
	warnedSpinning        map[string]bool
	shot                  shotRun
	watchFileChanges      bool
	isMouseCursorCaptured bool
	projectileFlyingAnim  int

	projectileImpactAnim int
	lastHitResult        *RaycastResult

	gravitationEnabled bool
	move               q3Move

	// glassCells is which voxels the level's window panes fill. The panes
	// themselves are quads in the sprite pass with no collider of their own, so
	// this is what makes a window a wall to walk into - see solidAt.
	glassCells map[core.Int3]bool

	// station is the level's own rotating body, nil for a level that is just a
	// place: what has air in it, and how fast the sky goes round. skySpin is how
	// far round it is so far, in radians, and inEVA is only so that crossing the
	// hull can say so once rather than every frame. See eva.go and core.Station.
	station *core.Station
	skySpin float32
	inEVA   bool

	// The map editor's half of the state. Kept together rather than mixed into the
	// player's, because the two are only ever touched by different halves of the
	// code: nothing in a normal run reads any of it.
	editor Editor

	// mapSource is the map as it was loaded and mapPath the file it came from,
	// empty for a generated one. placed is everything standing in the world
	// that the map file holds - what the map spawned plus what the editor has
	// dropped since. Together they are what a save writes: see
	// mapWithPlacedEntities.
	mapSource core.MapLoadResult
	// mapName is the folder under assets/maps this level came from, empty for
	// the generated stress rig. It is what the editor saves back to.
	mapName string
	placed  []placedEntity
	// blocks is the level's geometry, and the only description of it there is:
	// what LoadVoxelMap is handed and what the editor saves back out.
	blocks *voxel.Map

	weapons            []*Weapon
	currentWeaponIndex int
	// weaponKeys and weaponByID come out of the weapons folder's key map.
	weaponKeys map[int]string
	weaponByID map[string]int
	// playerInventory and playerCaps are the one pool of ammo, chambered rounds
	// and flags every weapon's runtime reads and writes - see docs/adr/0003 and
	// shareInventory. Two guns that both spend Shells share the same Shells.
	playerInventory wep.Inventory
	playerCaps      wep.Caps
	// recentWeapons is which weapons still have their pictures loaded, oldest
	// first. See keepLoaded.
	recentWeapons []*Weapon
	// pickups are the weapons and boxes of ammo waiting to be picked up, and
	// pickupModeIndex is how: walked over, or looked at and asked for. See
	// pickupModeNames. pickupByObject is how the selection tells an item from
	// anything else it has picked out - a pickup is a thing in the world like
	// the barrel beside it, and this is the way back to what it holds.
	pickups         []*Pickup
	pickupModeIndex int
	pickupByObject  map[*quads.GameObject]*Pickup

	// heldItem is the item the player has taken into their hands and is aiming
	// at where it should stand - see carry.go. nil when their hands are empty.
	heldItem *quads.GameObject

	// furniture is every piece of scenery standing in the world. The selection
	// looks straight through it: a shelf is somewhere to put things, not
	// something to pick out, and it used to offer to remove itself instead.
	//
	// ponytail: entries for pieces the editor has removed stay in it. A map of
	// pointers the size of the furniture in one map costs nothing; sweep it
	// where the editor kills one if a map ever holds thousands.
	furniture map[*quads.GameObject]bool

	// scenery is the furniture the world can be furnished with, by folder name.
	// Only the descriptions are here; a model is loaded the first time one is
	// spawned.
	scenery map[string]*Scenery

	// spriteKinds is everything in the world that is a picture rather than a
	// model - the guards, the mutants, the barrels - by folder name. Same shape
	// as scenery and for the same reason; see sprite_kind.go.
	spriteKinds map[string]*SpriteKind

	// keypads and notes are the level's locked doors and the writing that gives
	// their codes away - see keypad.go. knownCodes is every code the player has
	// read off a note, which is what a keypad prints on its own panel when it
	// recognises its own number there.
	keypads    []*Keypad
	notes      []*Note
	signs      []*Sign
	knownCodes map[string]bool

	// audioLogs is the same for the level's recordings - see audiolog.go.
	// lastAudioLog is the one the quickplay key means, which is whichever was
	// picked up most recently. logCuesLoaded and logOrigin are the audio
	// player's half of it: the recordings are loaded the first time one is
	// played and dropped by the next map load, and they play on a sound origin
	// of their own so that putting a weapon away does not silence them.
	audioLogs     []*AudioLog
	lastAudioLog  *AudioLog
	logCuesLoaded bool
	logOrigin     int

	// climbVolumes are the world-space boxes a ladder's Climbable boxes turned
	// into: standing inside one lets pollInput move the player straight up or
	// down instead of falling. See registerClimbable.
	climbVolumes []voxel.AABB

	// actors are every NPC/monster in the level - see actor.go.
	actors []*Actor
	// companionTarget is the actor the player last shot, which is what every
	// companion following the player attacks. nil when the last one died or
	// nothing has been shot yet. See damage() and updateCompanion.
	companionTarget *Actor

	// doors are every slide or swing door in the level - see door.go.
	doors []*Door

	// triggerHeld is a state and reloadPressed is an edge; the weapon's script
	// decides whether either does anything. altHeld is the second attack
	// button, ZDoom's +altattack, which a weapon with an AltFire state of its
	// own reads as a mode of its own.
	triggerHeld   bool
	altHeld       bool
	reloadPressed bool

	audio *audio.Player
	// musicName is the track playing now, so that loading the same level again
	// leaves it playing rather than starting it over - see playMusic.
	musicName string
	// baseFOV is the view the player plays at, which a scope divides. The zoom
	// itself lives on the weapon, the way ZDoom keeps it in Weapon.FOVScale,
	// so putting a scoped weapon away puts the view back on its own.
	baseFOV float32

	// autoWalk holds the forward key down for the whole run. A weapon's sway is
	// a function of how fast the player is going, so it is invisible in a
	// capture that stands still - which every capture does.
	autoWalk bool

	// autoCycle puts the next weapon in hand every so many seconds, and
	// autoCycleClock is how long the one in hand has been held. Cycling the
	// whole folder is what a player does with a mod this size and it is where
	// anything that outlives the weapon that made it shows up.
	autoCycle      float64
	autoCycleClock float64
	// autoAlt does the same for the second attack button.
	autoAlt bool
	// tapDown and tapUp are -tap: how many tics the trigger is held and let go
	// of, over and over. A weapon that only acts on a fresh press - a
	// semi-automatic, a detonator - does nothing at all under a held trigger,
	// and that is exactly the kind that needs looking at.
	tapDown, tapUp int
	tapClock       float64
	// autoFire holds the trigger down for the whole run, which is how a weapon
	// is looked at or measured without a hand on the mouse.
	autoFire bool
	// startWeapon is the weapon to begin holding, by folder name.
	startWeapon string

	// sharedAtlas holds the frames of everything the weapons throw into the
	// world, packed once for all of them, and worldActors are the ones in
	// flight right now.
	sharedAtlas       *glhf.TextureAtlasPacked
	sharedSpriteIndex map[string]int
	worldActors       []*WorldActor
	warnedSprites     map[string]bool
	// lastActorOrigin hands each spawned thing a sound origin of its own, so
	// its channels are its own rather than the player's - see playerOrigin.
	lastActorOrigin int
	// muzzleFlashLit says whether the one tracked light is currently the
	// muzzle flash's, so putting the flash out does not put a projectile's
	// light out with it.
	muzzleFlashLit bool

	// Set only by -stress, nil on a normal run.
	stressConfig *StressConfig

	// startOBBs is -obb: draw every collider from the first frame, which is the
	// F1 menu's Show OBBs without a menu to click. A wrong picking box is only
	// ever obvious when it is drawn.
	startOBBs bool
	// startPickerDir is -pick: the folder to open the current mode's asset
	// browser on once the editor is up, empty for not opening it at all. A
	// capture takes no keystrokes, so a widget only reachable through a key is a
	// widget no screenshot can ever be taken of.
	startPickerDir string
	// startEditorMode is -editor: which editor mode to boot into, empty for a
	// normal run. It is here so a screenshot can be taken of the editor at all
	// - see AGENTS.md, a capture takes no keystrokes.
	startEditorMode string

	// keys is action name -> bound key, loaded from keyBindingsFile. See
	// keybindings.go.
	keys map[string]glfw.Key
}

func (g *Game) Init(app *quads.GlApplication, mapLoadResult core.MapLoadResult) {
	// Basics (Window / Camera)
	g.editor.currentPaintGunTextureIndex = -1
	g.GlApplication = app
	g.ShouldDrawOBBs = g.startOBBs
	g.keys = loadKeyBindings(keyBindingsFile)

	g.SetupUIAtlas(eassets.Embedded.TextureAtlasFixed("ui_sheet.png", 16, 16))
	g.SetupUIFont(eassets.Embedded.BitmapFont("quadratica.png", 8, 14, g.UIScale))
	// The same glyphs again, on the CPU this time: the editor's asset browsers
	// write the name of each thing into the picture of it. quads cannot read the
	// font itself - it is baked into eassets, which imports quads.
	g.setupTileLabelFont()

	if g.startTimeFactor > 0 {
		g.setTimeFactor(nearestTimeFactorIndex(g.startTimeFactor))
	}

	// Weapons and the number keys that select them are data, in the weapons
	// folder. A folder that does not load stops the game with every problem it
	// found, rather than quietly leaving a gun out.
	if err := g.LoadWeapons(weaponsDir); err != nil {
		panic(err)
	}

	// So is the furniture: one folder to a piece under assets/meshes/, read for its
	// name and its colliders here and for its model only when something spawns
	// one.
	scenery, err := loadScenery(meshDir)
	if err != nil {
		panic(err)
	}
	g.scenery = scenery

	// And so are the sprites: one folder to a kind under assets/sprites/, read
	// here for its animations and its name, and for its picture only when
	// something spawns one.
	spriteKinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		panic(err)
	}
	g.spriteKinds = spriteKinds
	g.buildPropPalette()
	g.buildActorPalette()
	g.editor.spriteTexture = defaultDecorationTexture
	g.furniture = make(map[*quads.GameObject]bool)
	g.pickupByObject = make(map[*quads.GameObject]*Pickup)

	g.SetupUnitCubes()
	//g.SetCursor("assets/cursor.png", 7, 7)

	// Load Bitmap Font
	// Setup Text Labels
	g.textLabel = g.NewLabel()
	g.textLabel.SetBackground(mgl32.Vec4{0, 0, 0, 0.9})

	g.uiLabel = g.NewLabel()
	g.uiLabel.SetTintColor(mgl32.Vec4{0.5, 1, 0.5, 0.8})
	g.uiLabel.SetBackground(mgl32.Vec4{0.1, 0.1, 0.1, 0.74})

	g.hudHealth = g.NewLabel()
	g.hudHealth.SetTintColor(mgl32.Vec4{1, 0.5, 0.5, 1})
	g.hudName = g.NewLabel()
	g.hudName.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	g.hudAmmo = g.NewLabel()
	g.hudAmmo.SetTintColor(mgl32.Vec4{0.3, 1, 0.3, 1})

	// The editor's legend. It replaces the weapon HUD while building, and it is
	// the only interface the three tools have - see drawEditorHUD.
	g.editorHUD = g.NewLabel()
	g.editorHUD.SetTintColor(mgl32.Vec4{0.6, 1, 0.6, 1})
	g.editorHUD.SetBackground(mgl32.Vec4{0, 0, 0, 0.7})

	g.entityMenu = g.NewList()
	g.entityMenu.SetTextCentered(true)
	g.entityMenu.SetKeyHandler(func(selectedIndex int, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
		// Same vocabulary as every other menu: cursor keys or WASD to move,
		// Enter or the interact key to act. W/S stay out of it - the menu is
		// not modal and the player is still walking around while it is up.
		switch key {
		case glfw.KeyUp:
			g.entityMenu.SelectPrevious()
			return true
		case glfw.KeyDown:
			g.entityMenu.SelectNext()
			return true
		}
		if key == glfw.KeyEnter || key == glfw.KeyKPEnter || key == g.key(ActionInteract) {
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

	// Load Sprite Animations: six a folder under assets/sprites/, in folder
	// order. Nothing here says which kinds exist - see registerSpriteKinds.
	g.registerSpriteKinds()

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
	g.SetupParticleSystem(quads.ParticlesExplosion, 2000)
	g.SetupParticleSystem(quads.ParticlesSmoke, 2000)
	g.SetupParticleSystem(quads.ParticlesCustom, 2000)
	g.mapLoad(mapLoadResult, false)

	events.Subscribe(events.EventHitScanCollision, g.hitScanCollision)
	events.Subscribe(events.EventPhysicsCollision, g.physicsCollision)
	events.Subscribe(events.EventPlayerInteractWith, g.playerInteractWith)

	if g.benchSeconds > 0 {
		// The debug overlay is a text mesh rebuilt every frame and drawn over the
		// scene, and it is not part of what is being measured. Same for the minimap.
		g.ShouldDrawDebugInfo = false
		g.ShouldDrawMinimap = false
		g.StartBenchmark(quads.NewBenchmark(g.benchSeconds), g.benchmarkLabel())
	}

	if g.shot.path != "" {
		// The overlay would be in the picture, and what a capture is for is
		// looking at the weapon, not at a frame time.
		g.ShouldDrawDebugInfo = false
		g.ShouldDrawMinimap = false
		g.Screenshots(g.shot.path, g.shot.first, g.shot.every, g.shot.count, g.shot.step)
	}
	if g.shot.pitch != 0 {
		// Set outright rather than as a mouse movement: ChangeAngles drops a
		// delta over 200, and at the mouse sensitivity that is any pitch past
		// twenty degrees - which is every capture of something on the floor.
		// The angle only - the camera hangs off the player and stays there;
		// see SetAngles for what handing it a position used to do.
		yaw, _ := g.Camera.GetRotation()
		g.Camera.SetAngles(yaw, float32(-g.shot.pitch))
	}
	if g.shot.roll != 0 {
		g.Camera.SetRoll(float32(g.shot.roll))
	}

	if g.startEditorMode != "" {
		g.toggleEditorMode()
		g.setEditorMode(editorModeByName(g.startEditorMode))
		if g.startPickerDir != "" {
			g.openModePickerAt(g.startPickerDir)
		}
	}

	g.HandleResize(g.WindowWidth, g.WindowHeight)
}

// shotRun is what -shot and its companions asked for: where to write the
// screenshots, which ticks to take them on, and the frame time to pretend
// every frame took so two runs animate identically.
type shotRun struct {
	path  string
	first int
	every int
	count int
	step  float64
	// pitch is how far down to look, in degrees. What a weapon throws into
	// the world often lands on the floor, and a capture that only ever looks
	// at the horizon never sees it.
	pitch float64
	// roll is the horizon's tilt, in degrees - the suit's own, which no
	// keystroke can ask for in a capture. See eva.go.
	roll float64
}

// benchmarkLabel names the run in the report, so two reports pasted next to each
// other say what they measured rather than both saying "benchmark".
func (g *Game) benchmarkLabel() string {
	if g.stressConfig != nil {
		cfg := *g.stressConfig
		return fmt.Sprintf("stress %dx%d, pillars every %d, barrels every %d, %d lights",
			cfg.Size, cfg.Size, cfg.PillarSpacing, cfg.SpriteSpacing, cfg.Lights)
	}
	return fmt.Sprintf("%s, %d entities", g.mapName, len(g.mapSource.Entities))
}

func (g *Game) mapLoad(mapLoad core.MapLoadResult, hotReload bool) {
	g.mapSource = mapLoad
	// A weapon's sounds stay loaded from the moment it is given out until the
	// level that gave it out is gone - this is that moment. Ownership itself
	// lives on the weapon, not the map, so ownership survives a reload and the
	// sounds are loaded straight back for whatever the player is still
	// carrying, rather than staying gone until the next pickup that never
	// comes because it is already owned.
	if g.audio != nil {
		g.audio.UnloadAllCues()
		for _, weapon := range g.weapons {
			if weapon.owned {
				g.loadWeaponSounds(weapon)
			}
		}
		// The characters' cues go back in for the same reason and at the same
		// moment. They have to be here rather than once at startup: the unload
		// above clears every scope, so a guard loaded before the first map ever
		// loaded would be a guard that never says anything.
		g.loadSpriteKindSounds()
	}

	g.Sprites.Clear()
	g.Meshes.Clear()
	// Their sprites went with Sprites.Clear, and the map puts the range back.
	g.pickups = nil
	g.heldItem = nil
	g.climbVolumes = nil
	g.actors = nil
	g.companionTarget = nil
	g.doors = nil
	g.keypads = nil
	g.notes = nil
	g.signs = nil
	g.knownCodes = map[string]bool{}
	// UnloadAllCues just above took the recordings with everything else, so the
	// next log played loads them again.
	g.audioLogs = nil
	g.lastAudioLog = nil
	g.logCuesLoaded = false

	// The level's own atlas, out of its own folder, before the blocks that
	// index into it - and the block list kept beside it.
	if err := g.UseMapAtlasIn(filepath.Join(mapDir, g.savedAs())); err != nil {
		log.Printf("[map] %s keeps the atlas already loaded: %v", g.savedAs(), err)
	}
	g.loadBlocks()

	// Load Map. The blocks came off disk or out of a generator before there was
	// a window; this is where they become the scene's.
	g.LoadVoxelMap(mapLoad, g.blocks)

	// Which wall textures carry a lit fitting is recorded in the atlas itself, in its
	// emIS chunk - see AtlasIndex.EmissiveChunkKey. It is a fact about the pictures
	// (slot 49 glows because of what is painted in slot 49), so it travels with the
	// file rather than sitting in a list here that went stale the moment the editor
	// could add a texture. Press G at any wall to add one; that writes back.
	//
	// Marking a texture does not light the whole tile - only the texels that are both
	// bright and colourful, so the tubes and screens come on and the grey metal panel
	// they are mounted on stays dark.
	g.CubeMap.SetEmissiveTextures(g.MapTextures.Emissive())

	// The frustum has to reach the far corner of whatever just loaded. It is
	// per-map because it is a fact about the map: the gallery is 288 tiles
	// long, and against the default hundred its end wall was clipped away and
	// the sky was visible through it.
	g.Camera.SetFarPlaneDist(mapFarPlane(g.CubeMap.Bounds()))

	/*
	   sphere := physics.NewSphericalHull(5)
	   bulletSDK := bullet.BulletNewPhysicsSDK()
	   world := bullet.BulletNewDynamicWorld(bulletSDK)
	   bullet.BulletAddRigidBody(world, sphere.BulletHandle())
	   bullet.BulletStepSimulation(world, 0.1)
	*/
	// The glass. A pane is drawn as a quad and collides as the voxel it glazes,
	// so a window is see-through without being a way in - see core.Pane and
	// SpriteDrawer.AddPane.
	g.glassCells = make(map[core.Int3]bool, len(mapLoad.Panes))
	for _, pane := range mapLoad.Panes {
		g.glassCells[pane.Cell()] = true
		g.Sprites.AddPane(g.sharedAtlas, uint16(g.sharedSpriteIndex[wep.WindowGlassSprite]),
			pane.Position, pane.Normal, pane.Width, pane.Height, paneLight)
	}

	// Setup Collision Solver
	g.physicsSystem = quads.NewCollisionSolver(g.solidAt)
	g.physicsSystem.SetGravity(mgl32.Vec3{0, -q3Gravity, 0})
	g.physicsSystem.SetStepHeight(pmStepSize)
	// Feet on the floor is the game, flying is building - a level loaded from
	// inside the editor, a new one above all, is loaded flying. F switches.
	g.setGravity(!g.editor.active)
	// Bounds, not VoxelDimensions: the voxel buffer is padded up to a whole
	// number of chunks (a 7-tall level allocates a 32-tall buffer), and every
	// layer above the level itself is chunk padding, not carved-out air - it
	// reads as solid. Scanning up to the padded height made groundHeight find
	// "floor" at y=33 on every column instead of the real surface, and the
	// search only ever explored a stack of one-tile-wide solid ceilings that no
	// path could get down from.
	g.SetupPathfinder(g.CubeMap.IsSolidBlockAt, g.CubeMap.Bounds().Y)

	// Spawn Entities. Each one is remembered as it goes in, so that a save
	// writes the whole level back out rather than only the part of it the
	// editor happened to add this run.
	g.placed = nil
	for _, spawn := range mapLoad.Entities {
		spawnPos, entity := spawn.Position, spawn.Entity
		object := g.spawnEntity(spawnPos, entity)
		if entity.MeshName == torchMeshName {
			// The model is spawnEntity's; the light is not, because the
			// editor's ghost goes through the same call and a ghost that lit
			// the map would flood it from wherever it first appeared.
			g.dropTorch(spawnPos)
		}
		g.recordPlaced(object, entity, spawnPos)
	}
	// Which door each keypad holds shut is worked out once the whole level is
	// standing, because a keypad is perfectly free to load before its door.
	g.bindKeypads()
	if hotReload {
		g.physicsSystem.AddObject(g.player.Physics())
	} else {
		g.player = newPlayer(g, g.spawnPlayer(g.Camera, mapLoad.PlayerSpawn, mapLoad.PlayerDir))
	}
	// A fresh bar every time a level loads, which is also what a hot reload
	// wants: the level is starting again, so the player is too.
	g.player.health = playerMaxHealth
	// The level's own track, which is silence for a level that names none.
	g.playMusic(mapLoad.Music)

	// Setup Ceiling & Fog
	g.CeilingColor = mapLoad.CeilingColorVec4()
	// A map that says nothing about how far it can be seen through gets the
	// engine's own numbers; one that does - the shadow rig, whose fog would
	// otherwise be the same shade of brown as the light pool - gets its own.
	g.fogStart, g.fogEnd = 190, 250
	if mapLoad.FogEnd > 0 {
		g.fogStart, g.fogEnd = mapLoad.FogStart, mapLoad.FogEnd
	}
	// A level with no horizon is one with nothing under it: space. Every other
	// level keeps the horizon it has always had.
	g.skyHorizonFade = 7
	if mapLoad.SkyFullSphere {
		g.skyHorizonFade = 0
	}
	g.station = mapLoad.Station
	// Gravity stops at the hull, for everything and not only for the player: a
	// casing thrown out of an airlock, a grenade let go in vacuum and the
	// sparks off a shot all hang there rather than falling towards a deck that
	// is not under them. See Station.Pressurised and eva.go.
	g.physicsSystem.SetWeightlessAt(g.inVacuum)
	g.skySpin = 0
	g.inEVA = false
	// A new level has a deck underfoot: the mouse turns the player again and
	// the horizon is level, whatever the suit was doing in the last one.
	g.Camera.SetFreeLook(false)
	g.Camera.SetRoll(0)
	g.SetSkyboxSpin(0, mgl32.Vec3{})
	g.exposure = quads.DefaultExposure
	g.bloomThreshold = quads.DefaultBloomThreshold
	g.bloomStrength = quads.DefaultBloomStrength
	g.emissiveStrength = quads.DefaultEmissiveStrength
	g.emissiveCutoff = quads.DefaultEmissiveCutoff
	g.emissiveSaturation = quads.DefaultEmissiveSaturation
	g.SetShadowBias(g.shadowNormalOffset, g.shadowDepthBias)
	g.updateAmbientOcclusion()
	// Every load, not only a level that names one: otherwise the last level's
	// fill light stays on in the next.
	g.ambientColor, g.ambientStrength = levelAmbient(mapLoad)
	g.SetAmbientLight(g.ambientColor, g.ambientStrength)
	g.updateFog()
	g.SetLight(mgl32.Vec3{2, 2, 4}, mgl32.Vec3{1, float32(214) / float32(255), float32(110) / float32(255)})

	//scientist.Sprite().PlayAnimationLooping(int(quads.AnimWalk))
	g.SetupMinimap(minimapRadius*2+1, minimapRadius*2+1, g.colorForMinimap)

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

	// The `true ||` that used to be here made the F1 "Show Debug Info" checkbox do
	// nothing, and kept a text mesh rebuilt and drawn every frame with no way to
	// switch it off - including while measuring frame times. The overlay still
	// defaults to on, it is just switchable again now.
	if g.ShouldDrawDebugInfo {
		g.printDebugInfo()
	}

	g.pollInput(elapsed)

	if g.autoFire {
		g.triggerHeld = true
	}
	g.updateAutoTap(elapsed)
	if g.autoAlt {
		g.altHeld = true
	}
	g.updateAutoCycle(elapsed)
	g.updateWeapon(elapsed)
	g.updateActors(elapsed)
	g.updateDoors(elapsed)
	g.updateStationSpin(elapsed)
	g.physicsSystem.Update(elapsed)
	g.updatePickups()
	g.updateGUI(elapsed)
}

// updateStationSpin turns the sky. Nothing else moves: the station is the frame
// everything in this level is measured in, so the relative motion has to go
// somewhere and the sky is the cheap end of it. See core.Station.
func (g *Game) updateStationSpin(elapsed float64) {
	if g.station == nil || g.station.Spin == 0 {
		return
	}
	turned := float64(g.skySpin) + float64(mgl32.DegToRad(g.station.Spin))*elapsed
	g.skySpin = float32(math.Mod(turned, 2*math.Pi))
	g.SetSkyboxSpin(g.skySpin, g.station.SpinAxis)
}

// paneLight is the baked light level a window pane is drawn at. It is a fixed
// number rather than the light where the pane hangs because the map's light is
// not flooded until the first frame is drawn, which is after the panes are
// placed - and a window is a hole in a wall, lit from both sides, rather than
// something standing in a corner.
const paneLight = 12

// solidAt is the world the physics pushes against: the blocks, plus the voxels
// the glass fills. It is one function rather than a check in the solver because
// the solver is only ever handed "is this cell wall", and glass is.
func (g *Game) solidAt(x, y, z int32) bool {
	return g.CubeMap.IsSolidBlockAt(x, y, z) || g.glassCells[core.Int3{X: x, Y: y, Z: z}]
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
	guiTrisDrawn, guiDrawCalls := g.GuiDrawnLastFrame()
	meshTriangles, meshDrawCalls := g.Meshes.DrawnLastFrame()
	shadowPasses, litLights := g.ShadowPasses()
	meshGen, _ := g.ShadowCacheState()

	rows := []core.TableRow{
		core.NewTableRow("Cam Fwd", "", vec3String(forward)),
		core.NewTableRow("Cam Pos", "", vec3String(pos)),
		core.NewTableRow("Ray Hit", "", rayCastPosString),
		core.NewTableRow("Ray Len", "", lengthOfRay),
		core.NewTableRow("Coll. Pairs", "", fmt.Sprintf("%d", g.physicsSystem.PairsChecked())),
		core.NewTableRow("Map", fmt.Sprintf("%d tris", g.CubeMap.TriangleDrawCount()), fmt.Sprintf("%d", g.CubeMap.DrawCalls())),
		core.NewTableRow("GUI", fmt.Sprintf("%d tris", guiTrisDrawn), fmt.Sprintf("%d", guiDrawCalls)),
		core.NewTableRow("Sprites", fmt.Sprintf("%d tris", spriteTrisDrawn), fmt.Sprintf("%d", spriteDrawCalls)),
		core.NewTableRow("Spawned", fmt.Sprintf("%d live", g.LiveActorCount()), weaponID(g.currentWeapon())),
		core.NewTableRow("Meshes", fmt.Sprintf("%d tris", meshTriangles), fmt.Sprintf("%d", meshDrawCalls)),
		core.NewTableRow("Shadows", fmt.Sprintf("%d/%d lit", shadowPasses, litLights), fmt.Sprintf("gen %d", meshGen)),
	}
	debugLines := core.TableLayout(rows, []core.TextAlignment{core.AlignLeft, core.AlignRight, core.AlignRight})
	g.PrintMulti(debugLines)
}

// weaponID names the weapon in hand on the overlay by its folder name, not its
// tag, or nothing when the player is empty handed. Five weapons are tagged
// "Super Shotgun"; only the id says which of them is in your hands, and the id
// is what -weapon takes to get it back.
func weaponID(weapon *Weapon) string {
	if weapon == nil {
		return ""
	}
	return weapon.Data.ID
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
	if g.heldItem != nil {
		// What is in hand is outlined rather than what is being looked at:
		// there is one thing the carry key acts on and this is it.
		topLeft := g.drawSelectionOutline(g.getHoveredOBB(g.heldItem))
		g.drawSelectionLabel(topLeft, g.heldItem.DisplayName+"  ["+g.keyLabel(ActionCarryItem)+" place]")
	} else if pickup := g.hoveredPickup(); pickup != nil {
		// An item the selection is on, in intentional mode: outlined by its own
		// collider like anything else, named, and waiting for the key that
		// takes it.
		topLeft := g.drawSelectionOutline(g.getHoveredOBB(pickup.object))
		g.drawSelectionLabel(topLeft, pickup.object.DisplayName+"  [E]")
	} else if hoveredObj := g.hoveredObj(); hoveredObj != nil {
		aabb, transform := g.getHoveredOBB(hoveredObj)
		topLeft := g.drawSelectionOutline(aabb, transform)
		hitZone := "body"
		if g.relativeHitPos.Y() > 0.6 && g.relativeHitPos.Y() < 0.8 {
			hitZone = "head"
		}
		if g.drawSelectionLabel(topLeft, fmt.Sprintf("%s (%s)", hoveredObj.DisplayName, hitZone)) &&
			g.entityMenu != nil {
			g.entityMenu.Draw()
		}
	}

	if g.editor.active {
		g.drawEditorHUD()
	} else {
		if weapon := g.currentWeapon(); weapon != nil {
			guiWidth, guiHeight := g.GuiSize()
			bobX, bobY := g.weaponBob()
			weapon.Draw(int32(guiWidth), int32(guiHeight), bobX, bobY)
		}
		g.drawWeaponHUD()
		g.drawHealthHUD()
	}

	// Only while the overlay is on. The check used to be commented out here
	// and left on the half that *builds* the text, so switching the F1
	// checkbox off stopped printDebugInfo updating it and left the last frame
	// of it painted on the screen for the rest of the run - which reads as the
	// checkbox doing nothing at all.
	if g.textLabel != nil && g.ShouldDrawDebugInfo {
		g.textLabel.Draw()
	}

	for _, widget := range g.widgetStack {
		widget.Draw()
	}
}

// drawSelectionOutline puts the four corner brackets round a box in the world
// and reports where its top left corner landed on the screen, which is where a
// label above it goes.
func (g *Game) drawSelectionOutline(aabb voxel.AABB, transform mgl32.Mat4) [2]int {
	guiBatch := g.UIAtlas.GetSpriteBatch()
	topLeft, topRight, bottomLeft, bottomRight := quads.HudCornersFromOBB(aabb, transform, g.Camera.ProjectWorldToScreenPixels)

	borderColor := mgl32.Vec4{0.5, 1, 0.5, 0.8}
	// One tile per corner, each anchored by the corner of itself that meets the
	// box: top left by its own top left, and so round.
	for index, corner := range [4]struct {
		at     [2]int
		anchor mgl32.Vec2
	}{
		{topLeft, mgl32.Vec2{0, 0}},
		{topRight, mgl32.Vec2{1, 0}},
		{bottomRight, mgl32.Vec2{1, 1}},
		{bottomLeft, mgl32.Vec2{0, 1}},
	} {
		source, dest := g.UIAtlas.GetSourceAndDestRects(index,
			mgl32.Vec2{float32(corner.at[0]), float32(corner.at[1])}, corner.anchor, mgl32.Vec2{1, 1})
		guiBatch.DrawTextured(source, dest, borderColor)
	}
	return topLeft
}

// drawSelectionLabel writes what the outlined thing is, just above it.
func (g *Game) drawSelectionLabel(topLeft [2]int, text string) bool {
	if g.uiLabel == nil {
		return false
	}
	g.uiLabel.SetText(text)
	_, fontH := g.uiLabel.Size()
	g.uiLabel.SetPosition(mgl32.Vec2{float32(topLeft[0]), float32(topLeft[1] - fontH)})
	g.uiLabel.Draw()
	return true
}

func (g *Game) hoveredObj() *quads.GameObject {
	if g.lastHitResult == nil {
		return nil
	}

	if !g.lastHitResult.HitInfo.Hit || g.lastHitResult.HitInfo.Distance > 10 || g.lastHitResult.HitObject == nil {
		return nil
	}

	// Furniture is not something to select. The ray still hits a shelf - it
	// stops a bullet and it caps the pickup beam - but the selection looks
	// through it at whatever is standing on it, rather than outlining the shelf
	// and offering to remove it.
	object := g.lastHitResult.HitObject
	if g.furniture[object] {
		return nil
	}
	return object
}

func (g *Game) isHoveringObject() bool {
	return g.hoveredObj() != nil
}
func (g *Game) updateGUI(elapsed float64) {
	if g.widgetsAreBlockingMovement {
		return
	}
	g.lastHitResult = nil

	g.uiLabel.Hide()
	g.entityMenu.Hide()

	var origin, direction mgl32.Vec3
	if g.isMouseCursorCaptured {
		origin, direction = g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
	} else {
		x, y := g.Window.GetCursorPos()
		origin, direction = g.Camera.GetPickingRayFromScreenPosition(
			x/float64(g.WindowWidth), y/float64(g.WindowHeight))
	}
	result := g.rayCast(origin, direction)

	g.lastHitResult = &result

	// The item in hand rides the aim, a block out or on whatever solid thing is
	// nearer than that.
	if g.heldItem != nil {
		g.heldItem.Transform().SetPosition(carryPosition(origin, direction, result))
	}

	if g.editor.active {
		g.updateEditorPreview()
	}

	if ghost := g.editor.gameObjectForSpawning; ghost != nil {
		// A sign hangs on a wall, so its ghost rides the wall the crosshair is on
		// - flat against the face and turned to look out of it, which is the only
		// way a sign is ever mounted. Everything else stands on the floor.
		if wallPos, facing, onWall := g.lastHitResult.WallPosition(); onWall && g.placingWallPiece() {
			ghost.Transform().SetPosition(wallPos)
			ghost.Transform().SetForward2D(facing)
		} else if groundPos, hitGround := g.lastHitResult.GroundPosition(); hitGround {
			ghost.Transform().SetPosition(ghostPosition(groundPos, g.editor.snapToGrid))
		}
	}

	if g.heldItem != nil {
		// While something is in hand it is the one thing outlined, and the
		// label above it is its own.
		g.uiLabel.Show()
		return
	}

	if g.hoveredPickup() != nil {
		// The label belongs to whatever is outlined, and only one thing is.
		g.uiLabel.Show()
		return
	}

	hoveredObj := g.hoveredObj()

	if hoveredObj == nil {
		return
	}

	g.uiLabel.Show()
	// The entity menu edits the world - idle frames, deleting a thing - so it
	// only ever makes sense with the cursor free, and never for a pickup: an
	// item the player can pick up is not one they can also delete this way.
	if !g.isMouseCursorCaptured && g.pickupByObject[hoveredObj] == nil {
		g.entityMenu.Show()
	}
}

// getHoveredOBB is the box the selection outline is drawn round, which has to be
// the same box the ray was tested against - PhysicsComponent.OBB, and nothing
// worked out a second time here. Built separately, it was: the outline kept the
// sprite's full draw transform after the box itself became world units, so the
// brackets round a kit drawn a third of a unit wide closed to a ninth of one
// while the ray that picked it out reached the whole picture.
func (g *Game) getHoveredOBB(hoveredObj *quads.GameObject) (voxel.AABB, mgl32.Mat4) {
	obb := hoveredObj.Physics().OBB(g.Camera)
	return obb.AABB, obb.Transform
}

type RaycastResult struct {
	HitObject *quads.GameObject
	WorldPos  mgl32.Vec3
	Relative  mgl32.Vec3
	HitInfo   voxel.HitInfo3D
	// OnTop is whether the object was hit on the lid of its box - a table top,
	// a crate's upper face - which is somewhere to stand a smaller piece.
	OnTop bool
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
	if !r.HitInfo.Hit {
		return mgl32.Vec3{}, false
	}
	if r.HitObject != nil {
		return r.WorldPos, r.OnTop
	}
	cwp := r.HitInfo.CollisionWorldPosition
	//worldPos := mgl32.Vec3{cwp.X(), float32(math.Floor(float64(cwp.Y()))), cwp.Z()}
	return cwp, r.HitInfo.Side == voxel.Top
}
// signWallGap is how far off the wall a sign hangs, so its face is not in the
// same plane as the block behind it and the two do not flicker against each
// other. It is the gap the hand-written signs in build.map already stand at.
const signWallGap = 0.03

// WallPosition is GroundPosition's twin for the things that go on a wall rather
// than on a floor: a hit on one of the four upright faces of a voxel, reported
// as the spot on the face to hang at and the way that face looks. A hit on a
// prop is not a wall - a sign belongs on the structure, not on a table.
func (r RaycastResult) WallPosition() (mgl32.Vec3, mgl32.Vec3, bool) {
	if !r.HitInfo.Hit || r.HitObject != nil {
		return mgl32.Vec3{}, mgl32.Vec3{}, false
	}
	side := r.HitInfo.Side.AsDirection()
	if side.Y != 0 {
		return mgl32.Vec3{}, mgl32.Vec3{}, false
	}
	outOfTheWall := mgl32.Vec3{float32(side.X), 0, float32(side.Z)}
	return r.HitInfo.CollisionWorldPosition.Add(outOfTheWall.Mul(signWallGap)), outOfTheWall, true
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
	var onTop bool
	hitInfo = voxel.DDARaycast(origin, direction, rayCastRange, func(x, y, z int32) (stopRay bool) {
		if !g.CubeMap.Contains(int32(x), int32(y), int32(z)) {
			return false // outside the map there is nothing to hit, keep going until the ray runs out
		}
		// The nearest thing in this cell, not the first one the map happens to
		// hand over. A cell holds one barrel and it never mattered; a cell of
		// the weapon room holds a shelf's five boxes and the guns standing on
		// them, and picking whichever came out of the map first meant looking
		// at a gun and selecting the board behind it - differently each frame.
		nearest := math.MaxFloat64
		var nearestObj *quads.PhysicsComponent
		for gObj := range g.physicsSystem.GetCollidersAt(x, y, z) {
			// A projectile in flight is not something to look at, interact with or
			// shoot: it is skipped here rather than in hoveredObj, so the HUD frame,
			// the label and the hitscan weapons all lose it at once.
			if gObj.GameObject() == g.player.GameObject || gObj.GameObject() == g.editor.gameObjectForSpawning ||
				gObj.GameObject().HasComponent(quads.ComponentTypeProjectile) {
				continue
			}
			intersectionDistance, intersects := voxel.IntersectRayOBB(origin, direction, gObj.OBB(g.Camera))
			if !intersects || intersectionDistance >= nearest {
				continue
			}
			nearest, nearestObj = intersectionDistance, gObj
		}
		if nearestObj != nil {
			collisionWorldPos := origin.Add(direction.Mul(float32(nearest)))
			hitObject = nearestObj.GameObject()
			worldPos = collisionWorldPos
			relativePos = collisionWorldPos.Sub(nearestObj.Position())
			onTop = onLid(nearestObj.OBB(g.Camera), collisionWorldPos)
			return true
		}
		return g.CubeMap.IsSolidBlockAt(x, y, z)
	})

	return RaycastResult{
		HitObject: hitObject,
		WorldPos:  worldPos,
		Relative:  relativePos,
		HitInfo:   hitInfo,
		OnTop:     onTop,
	}
}

// onLid is whether a point where a ray entered this box lies on its top face.
// The entry point sits on whichever face the ray came in through, so it is the
// lid when its height in the box's own space is the box's top, give or take
// float error - a hair of the box's height, which on a side face means the very
// top edge, where standing a piece on the lid is what was meant anyway.
func onLid(obb voxel.OBB, hit mgl32.Vec3) bool {
	local := obb.Transform.Inv().Mul4x1(hit.Vec4(1)).Vec3()
	height := obb.Max().Y() - obb.Min().Y()
	return math.Abs(float64(obb.Max().Y()-local.Y())) <= float64(height)*1e-3
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
	if g.editor.voxelBlock == nil {
		g.Toast("No voxel block loaded")
		return
	}
	g.ShouldDrawMinimap = false
	g.ShouldDrawDebugInfo = false
	g.SetMouseCapture(false)

	g.widgetsAreBlockingMovement = true
	atlasPicker := g.OpenTextureCube(g.MapTextures, g.editor.voxelBlock.Textures)
	guiWidth, guiHeight := g.GuiSize()
	atlasPicker.HandleResize(guiWidth, guiHeight, g.UIScale)

	// The mapper edits the brush's own sides in place, so what it was opened on
	// is kept to tell whether closing it left a new block to put on the list.
	before := designOf(g.editor.voxelBlock)
	atlasPicker.SetOnClose(func() {
		g.SetMouseCapture(true)
		if after := designOf(g.editor.voxelBlock); after != before {
			g.rememberBlock(after)
		}
	})
	g.replaceWidgetStack(atlasPicker)
}

// pickTexture browses the texture library and hands back the slot the chosen file
// landed in, in whichever atlas it was picked for.
func (g *Game) pickTexture(dest *quads.AtlasIndex, onSelected func(selectedIndex int)) {
	previousMouseState := g.setupModal()
	g.widgetsAreBlockingMovement = true
	atlasPicker := g.OpenTextureBrowser(dest)
	atlasPicker.SetOnIndexClicked(func(selection int) {
		// Close, not just pop: the browser has a directory of the library uploaded
		// and drawn, and popping the widget on its own leaves that behind.
		atlasPicker.Close()
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

// openWizardMenu is F1: four sub-menus rather than one long list, since the
// list had grown past thirty rows - what to show or hide, the window and
// display, what changes how the world runs, and the rendering pipeline's own
// knobs (fog, bloom, emissive, shadows, and the one plain action among them,
// regenerating the terrain's mipmaps).
func (g *Game) openWizardMenu() {
	g.menuPreviousMouseState = g.setupModal()
	top := g.OpenConfigMenu()
	top.SetOptions([]quads.GuiOption{
		quads.NewGuiOptionSubmenu("Load Map", func() { g.pushConfigSubmenu(g.mapOptions()) }),
		quads.NewGuiOptionSubmenu("Show / Hide", func() { g.pushConfigSubmenu(g.showHideOptions()) }),
		quads.NewGuiOptionSubmenu("Video", func() { g.pushConfigSubmenu(g.videoOptions()) }),
		quads.NewGuiOptionSubmenu("Rendering", func() { g.pushConfigSubmenu(g.renderingOptions()) }),
		quads.NewGuiOptionSubmenu("Global", func() { g.pushConfigSubmenu(g.globalOptions()) }),
		quads.NewGuiOptionSubmenu("Actors", func() { g.pushConfigSubmenu(g.actorOptions()) }),
		quads.NewGuiOptionSubmenu("Cheats", func() { g.pushConfigSubmenu(g.cheatOptions()) }),
		quads.NewGuiOptionSubmenu("Music", func() { g.pushConfigSubmenu(g.musicOptions()) }),
	})
	guiWidth, guiHeight := g.GuiSize()
	top.HandleResize(guiWidth, guiHeight, g.UIScale)
	top.SetOnClose(g.closeAllMenus)
	g.pushToWidgetStack(top)
}

// closeAllMenus dismisses the whole F1 tree, however many sub-menus deep it
// is, and hands the mouse back the way it was before the top level opened.
// The top level's own Escape reaches this directly; an action buried in a
// sub-menu (Start Map Hot Reload) calls it too, because starting a watcher
// reads as leaving the menu, not stepping back one level in it.
func (g *Game) closeAllMenus() {
	g.captureMouse(g.menuPreviousMouseState)
	for len(g.widgetStack) > 0 {
		g.popFromWidgetStack()
	}
}

// pushConfigSubmenu opens one of the F1 menu's categories on top of whatever
// is already open. Closing it only pops itself - the parent menu is still on
// the stack underneath and still drawn (DrawGui walks the whole stack), so
// Escape from a submenu reads as going back a level rather than leaving the
// menu entirely, which is what the top level's own close does.
func (g *Game) pushConfigSubmenu(options []quads.GuiOption) {
	menu := g.OpenConfigMenu()
	menu.SetOptions(options)
	guiWidth, guiHeight := g.GuiSize()
	menu.HandleResize(guiWidth, guiHeight, g.UIScale)
	menu.SetOnClose(g.popFromWidgetStack)
	g.pushToWidgetStack(menu)
}

// mapOptions is the levels there are: one row per folder under assets/maps,
// named after the folder. Nothing here knows what levels exist - a new one is a
// folder with a map.json in it, the same way a new weapon or a new piece of
// scenery is.
func (g *Game) mapOptions() []quads.GuiOption {
	names := mapNames()
	if len(names) == 0 {
		return []quads.GuiOption{quads.NewGuiOptionAction("no levels in "+mapDir, func() {})}
	}
	options := make([]quads.GuiOption, 0, len(names))
	for _, name := range names {
		label := name
		if name == g.mapName {
			label += "  (playing)"
		}
		options = append(options, quads.NewGuiOptionAction(label, func() {
			g.closeAllMenus()
			g.switchToMap(name)
		}))
	}
	return options
}

// switchToMap loads a different level while the game is running: both halves of
// it, and the player put down at its own spawn rather than left standing where
// the last level happened to leave them. That is the whole difference from a hot
// reload, which is the same map again and deliberately does not move anyone.
func (g *Game) switchToMap(name string) {
	result, blocks, err := loadMap(name)
	if err != nil {
		g.Toast("Load failed: " + err.Error())
		return
	}
	g.clearGhost()
	g.editor.selectedActor = nil
	// The brush's slots index the level being left's atlas, so it starts over as
	// the default block the editor opens with - see toggleEditorMode.
	g.editor.voxelBlock = voxel.NewBlock(2)
	g.mapName = name
	g.blocks = blocks
	g.mapLoad(result, true)

	// After mapLoad, not before: it builds a new collision solver and puts the
	// player's collider into it, and the velocity to clear is the one on that
	// side of the swap. A player who arrives still falling from the last map
	// lands somewhere else entirely.
	physics := g.player.Physics()
	physics.SetVelocity(mgl32.Vec3{})
	g.player.Transform().SetPosition(result.PlayerSpawn)
	if dir := result.PlayerDir; dir.Len() > 0 {
		g.Camera.SetForward2D(mgl32.Vec3{dir.X(), 0, dir.Y()}.Normalize())
	}
	g.Toast("Loaded " + name)
}

// showHideOptions is every render-pass toggle: the debug overlays and the
// pieces of the scene itself a pass can be switched off to isolate.
func (g *Game) showHideOptions() []quads.GuiOption {
	options := []quads.GuiOption{
		quads.NewGuiOptionBool("Show Debug Info", &g.ShouldDrawDebugInfo),
		quads.NewGuiOptionBool("Show Performance", &g.ShouldShowPerformance),
		quads.NewGuiOptionBool("Show Chunk AABBs", &g.ShouldDrawChunkAABBs),
		quads.NewGuiOptionBool("Show AABBs", &g.ShouldDrawAABBs),
		quads.NewGuiOptionBool("Show OBBs", &g.ShouldDrawOBBs),
		quads.NewGuiOptionBool("Show Paths", &g.ShouldDrawPaths),
		quads.NewGuiOptionBool("Show Sprites", &g.ShouldDrawSprites),
		quads.NewGuiOptionBool("Show Meshes", &g.ShouldDrawMeshes),
		quads.NewGuiOptionBool("Show Skybox", &g.ShouldDrawSkybox),
		quads.NewGuiOptionBool("Show Minimap", &g.ShouldDrawMinimap),
		quads.NewGuiOptionBool("Show Particles", &g.ShouldDrawParticles),
	}
	return options
}

// videoOptions is the window and display: what a settings menu calls
// "Video" everywhere else too, kept apart from the rendering pipeline's own
// tuning below.
func (g *Game) videoOptions() []quads.GuiOption {
	vModes := g.GetVideoModes()
	var resolutionOptions []string
	for _, vMode := range vModes {
		resolutionOptions = append(resolutionOptions, fmt.Sprintf("%dx%d", vMode.Width, vMode.Height))
	}
	options := []quads.GuiOption{
		quads.NewGuiOptionList("Window Mode", quads.WindowModeNames, &g.currentWindowModeIndex, g.setWindowMode),
		quads.NewGuiOptionAction("VSync", g.toggleVsync),
		quads.NewGuiOptionList("Window", resolutionOptions, &g.currentResolutionIndex, g.setWindowResolution),
		quads.NewGuiOptionList("GUI Scale", guiScaleLabels, &g.currentGuiScaleIndex, g.setGuiScale),
	}
	// Only macOS puts a menu bar and Dock over a borderless window, so only
	// there is there anything to choose between.
	if quads.CanConstrainToWorkArea {
		options = append(options, quads.NewGuiOptionBool("Constrain to work area", &g.ConstrainToWorkArea).
			ActivateFunc(g.RefreshWindowMode))
	}
	return options
}

// renderingOptions is the pipeline's own knobs: fog, bloom and tone mapping,
// the emissive pass, shadow bias, and the two things beside them that touch
// the same picture rather than the window it is drawn in.
func (g *Game) renderingOptions() []quads.GuiOption {
	return []quads.GuiOption{
		quads.NewGuiOptionAction("Generate MipMaps for Terrain", func() {
			g.CubeMap.GetTerrainTexture().GenerateMipMaps()
		}),
		quads.NewGuiOptionFloat("Ambient Strength", &g.ambientStrength).StepSize(0.05).ActivateFunc(g.updateAmbient),
		quads.NewGuiOptionFloat("Editor Sun", &g.editorSunStrength).StepSize(0.1).ActivateFunc(g.updateEditorSun),
		quads.NewGuiOptionBool("Ambient Occlusion", &g.ambientOcclusion).ActivateFunc(g.updateAmbientOcclusion),
		quads.NewGuiOptionFloat("AO Strength", &g.aoStrength).StepSize(0.05).ActivateFunc(g.updateAmbientOcclusion),
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
		quads.NewGuiOptionFloat("Shadow Normal Offset", &g.shadowNormalOffset).StepSize(0.5).ActivateFunc(g.updateShadowBias),
		quads.NewGuiOptionFloat("Shadow Depth Bias", &g.shadowDepthBias).StepSize(0.0005).ActivateFunc(g.updateShadowBias),
		quads.NewGuiOptionAction("Drop Light Here", g.dropLightHere),
	}
}

// globalOptions is everything that changes how the world runs rather than
// how it looks or what window it is in: time itself, how a pickup is taken,
// and the map's own dev workflow.
func (g *Game) globalOptions() []quads.GuiOption {
	options := []quads.GuiOption{
		quads.NewGuiOptionList("Time Factor", timeFactorLabels, &g.currentTimeFactorIndex, g.setTimeFactor),
		quads.NewGuiOptionList("Item Pickup", pickupModeNames, &g.pickupModeIndex, g.setPickupMode),
	}
	if !g.watchFileChanges {
		options = append(options, quads.NewGuiOptionAction("Start Map Hot Reload", func() {
			g.closeAllMenus()
			g.Toast("Watching for map changes")
			g.watchFileChanges = true
			go g.watchFileChangesLoop()
		}))
	}
	return options
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
}

// timeFactorSteps are the time factors the menu offers. 1.0 is real time; below it
// the whole world - movement, animation, weapons - runs slower, above it faster.
var timeFactorSteps = []float64{0.1, 0.25, 0.5, 1.0, 2.0, 4.0}

var timeFactorLabels = func() []string {
	labels := make([]string, len(timeFactorSteps))
	for i, factor := range timeFactorSteps {
		labels[i] = strings.TrimSuffix(fmt.Sprintf("%.2f", factor), "0") + "x"
	}
	return labels
}()

// defaultTimeFactorIndex is the entry for 1.0.
const defaultTimeFactorIndex = 3

// nearestTimeFactorIndex is how the -time-factor flag meets the menu: the flag takes
// any number, the menu only holds these, so it opens on the step actually running.
func nearestTimeFactorIndex(factor float64) int {
	nearest := defaultTimeFactorIndex
	for i, step := range timeFactorSteps {
		if math.Abs(step-factor) < math.Abs(timeFactorSteps[nearest]-factor) {
			nearest = i
		}
	}
	return nearest
}

func (g *Game) setTimeFactor(index int) {
	g.TimeFactor = timeFactorSteps[index]
}

func (g *Game) setWindowMode(index int) {
	g.SetWindowMode(quads.WindowMode(index))
}

func (g *Game) setWindowResolution(index int) {
	g.SetWindowSize(g.GetVideoModes()[index])
	g.currentWindowModeIndex = int(quads.WindowModeWindowed)
}
func (g *Game) updateToneMapping() {
	g.SetExposure(g.exposure)
}

func (g *Game) updateBloom() {
	g.SetBloom(g.bloomThreshold, g.bloomStrength)
}

// levelAmbient is the fill light a level is lit with. Most levels name none -
// a black colour at strength zero, which is what an unset field reads as - and
// get white at the default strength. A black colour is always unset: kept, it
// made the strength knob a knob on black, turning a dim level dark.
func levelAmbient(m core.MapLoadResult) (mgl32.Vec3, float32) {
	colour, strength := m.AmbientColor, m.AmbientStrength
	if colour == (mgl32.Vec3{}) {
		colour = mgl32.Vec3{1, 1, 1}
	}
	if strength <= 0 {
		strength = quads.DefaultAmbientStrength
	}
	return colour, strength
}

// updateAmbient is the knob for the flat fill light every surface gets before
// any lamp reaches it. It is the first thing to reach for in a level that is
// too dark to see what is being looked at, which is most of a level built to
// look at imported art in.
func (g *Game) updateAmbient() {
	if g.ambientStrength < 0 {
		g.ambientStrength = 0
	}
	g.SetAmbientLight(g.ambientColor, g.ambientStrength)
}

// editorSunDirection is where the editor's sun stands: high, and off every axis
// by a different amount, so no two faces of a block get the same share of it.
var editorSunDirection = mgl32.Vec3{0.5, 1, 0.3}.Normalize()

const defaultEditorSunStrength = float32(0.8)

// updateEditorSun lights the level from the sky while it is being built and
// never while it is played: a station is lit by its own lamps, and a sun that
// shines through every wall would hide what they do.
func (g *Game) updateEditorSun() {
	if g.editorSunStrength < 0 {
		g.editorSunStrength = 0
	}
	strength := g.editorSunStrength
	if !g.editor.active {
		strength = 0
	}
	g.SetSunLight(editorSunDirection, strength)
}

// updateAmbientOcclusion clamps the slider to 0..1: past 1 a buried corner
// would take light away rather than just not get any.
func (g *Game) updateAmbientOcclusion() {
	g.aoStrength = max(0, min(1, g.aoStrength))
	strength := g.aoStrength
	if !g.ambientOcclusion {
		strength = 0
	}
	g.SetAmbientOcclusion(strength)
}

func (g *Game) updateEmissive() {
	g.SetEmissiveStrength(g.emissiveStrength)
	g.SetEmissiveCutoff(g.emissiveCutoff)
	g.SetEmissiveSaturation(g.emissiveSaturation)
}

// updateShadowBias is the knob for a normal offset that depends
// on the shadow resolution and the map's scale, so it is tuned in the running engine
// rather than guessed at in a constant. Push it well past any plausible value and a
// floor that stays dark means the mechanism is wrong, not the number.
// dropLightHere adds a light where the camera stands, pointing where it looks. It is
// the quick way to find out where a light wants to be; the answer belongs in the map
// file afterwards, as a Light entity, because this one is gone on reload.
func (g *Game) dropLightHere() {
	g.popFromWidgetStack()

	position := g.Camera.GetPosition()
	light := core.MapLight{
		Position:  position,
		Color:     mgl32.Vec3{1, 0.95, 0.85},
		Intensity: 6,
		Angle:     45,
		Reach:     15,
	}.LookAt(position.Add(g.Camera.GetForward()))

	g.AddLights(quads.LightFromMap(light, g.ShadowResolution()))
	g.Toast(fmt.Sprintf("Light at %.1f %.1f %.1f, yaw %.0f pitch %.0f - not saved to the map",
		position.X(), position.Y(), position.Z(), light.Yaw, light.Pitch))
}

func (g *Game) updateShadowBias() {
	g.SetShadowBias(g.shadowNormalOffset, g.shadowDepthBias)
}

func (g *Game) updateFog() {
	g.SetFog(g.CeilingColor.Vec3(), g.fogStart, g.fogEnd)
	g.SetSkyFog(g.skyHorizonFade)
}

func (g *Game) toggleVsync() {
	g.ShouldUseVsync = !g.ShouldUseVsync
	g.SetVSync(g.ShouldUseVsync)
}

func (g *Game) toggleMouseCapture() {
	g.isMouseCursorCaptured = !g.isMouseCursorCaptured
	g.SetMouseCapture(g.isMouseCursorCaptured)
	// Letting go of the mouse has to let go of the trigger with it, or a
	// weapon whose script keeps firing while it is held never stops.
	g.triggerHeld = false
	g.altHeld = false
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
	fileToWatch := mapPathFor(g.savedAs())
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
				// A hand-edited or half-flushed save can land on a truncated file.
				// Keep the map that is loaded and wait for the next event rather
				// than taking the process down over it.
				reloaded, err := loadJSONMap(fileToWatch)
				if err != nil {
					log.Println("hot reload skipped:", err)
					continue
				}
				glhf.Call(func() {
					g.mapLoad(reloaded, true)
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

// minimapRadius is how many blocks around the player the minimap shows. The
// texture is that window, not the whole level, so it scrolls with the player.
const minimapRadius = 20

// colorForMinimap takes texture coords, not world coords: the window is centred
// on the player, so texel (radius, radius) is wherever they are standing.
func (g *Game) colorForMinimap(texX int, texY int) [4]uint8 {
	playerX := int(g.player.GameObject.Transform().GetPosition().X())
	playerY := int(g.player.GameObject.Transform().GetPosition().Z())
	playerHeight := int(g.player.GameObject.Transform().GetPosition().Y())
	x := playerX - minimapRadius + texX
	y := playerY - minimapRadius + texY
	bounds := g.CubeMap.Bounds()
	if x < 0 || y < 0 || x >= int(bounds.X) || y >= int(bounds.Z) {
		return [4]uint8{0, 0, 0, 0}
	}
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

func (g *Game) updateAutoCycle(elapsed float64) {
	if g.autoCycle <= 0 {
		return
	}
	g.autoCycleClock += elapsed
	if g.autoCycleClock < g.autoCycle {
		return
	}
	g.autoCycleClock = 0
	g.cycleWeapon(1)
}

// updateAutoTap is -tap: the trigger pressed and let go of on a fixed cycle.
func (g *Game) updateAutoTap(elapsed float64) {
	if g.tapDown <= 0 {
		return
	}
	cycle := float64(g.tapDown + g.tapUp)
	g.tapClock += elapsed * wep.TicRate
	for g.tapClock >= cycle {
		g.tapClock -= cycle
	}
	g.triggerHeld = g.tapClock < float64(g.tapDown)
}

// mapFarPlane is how far the camera has to see on a given level: to the far
// corner of it from any other corner, which is the diagonal, plus a little so
// nothing sits exactly on the plane. Floored at the old hundred, because a
// small level seeing further than it needs costs nothing.
func mapFarPlane(bounds voxel.Int3) float32 {
	x, y, z := float64(bounds.X), float64(bounds.Y), float64(bounds.Z)
	diagonal := float32(math.Sqrt(x*x+y*y+z*z)) * 1.1
	if diagonal < defaultFarPlane {
		return defaultFarPlane
	}
	return diagonal
}

// defaultFarPlane is what core.NewPerspectiveTransform starts every camera at.
const defaultFarPlane = 100
