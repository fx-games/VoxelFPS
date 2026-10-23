package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/geometry"
	"github.com/memmaker/voxelfps/platform/glfw"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// openSceneryMenu browses assets/meshes and puts the model that is clicked on
// the crosshair, where a left click drops it - the same ghost the torch rides.
// It is the 3D prop tool's own picker, reached from the menu; the placement key
// opens whichever picker the current mode has - see openModePicker.
func (g *Game) openSceneryMenu() {
	g.setEditorMode(ModeProps3D)
	g.openModePicker()
}

// openSpriteMenu is the flat decorations under assets/sprites/props - a barrel, a
// jet of steam, a corpse - and openPickupMenu the things under items/ that the
// player walks over and takes. Each switches to the tool that places them and
// opens that tool's browser, which is the same thing the placement key does.
func (g *Game) openSpriteMenu() {
	g.setEditorMode(ModeProps2D)
	g.openModePicker()
}

func (g *Game) openPickupMenu() {
	g.setEditorMode(ModePickups)
	g.openModePicker()
}

func (g *Game) openWeaponMenu() {
	g.setEditorMode(ModeWeapons)
	g.openModePicker()
}

// startSceneryPlacement puts a piece on the cursor. The ghost is not solid: a
// ghost you can walk into is a ghost that has already been placed.
func (g *Game) startSceneryPlacement(id string) {
	g.clearGhost()
	g.editor.gameObjectForSpawning = g.spawnScenery(id, mgl32.Vec3{}, false)
}

// openActorMenu is the same for the characters: the folders under
// assets/sprites/actors, browsed by their pictures. Picking one switches to the
// actor tool with that kind selected, which is what a click then places.
func (g *Game) openActorMenu() {
	g.setEditorMode(ModeActors)
	g.openModePicker()
}

func (g *Game) openVoxelEditMenu() {
	if g.editor.voxelBlock == nil {
		return
	}
	previousMouseState := g.setupModal()
	configMenu := g.OpenConfigMenu()

	options := []quads.GuiOption{
		quads.NewGuiOptionAction("Block Textures", g.openCubeTextureMapper),
		quads.NewGuiOptionAction("Paint Tool", func() {
			g.setEditorTool(EditorToolPaint)
		}),
		quads.NewGuiOptionAction("Fill Tool", func() {
			g.setEditorTool(EditorToolFill)
		}),
		quads.NewGuiOptionAction("Dig Tool", func() {
			g.setEditorTool(EditorToolDig)
		}),
		quads.NewGuiOptionAction("Put Tools Away", func() {
			g.setEditorTool(EditorToolNone)
		}),
		quads.NewGuiOptionAction("Choose Paint Texture", func() {
			g.pickTexture(g.MapTextures, func(selectedIndex int) {
				g.editor.currentPaintGunTextureIndex = selectedIndex
			})
		}),
		quads.NewGuiOptionAction("Choose Decoration Texture", g.changeDecorationTexture),
		quads.NewGuiOptionAction("Place 3D Prop", g.openSceneryMenu),
		quads.NewGuiOptionAction("Place 2D Prop", g.openSpriteMenu),
		quads.NewGuiOptionAction("Place Pickup", g.openPickupMenu),
		quads.NewGuiOptionAction("Place Weapon", g.openWeaponMenu),
		quads.NewGuiOptionAction("Place Actor", g.openActorMenu),
		quads.NewGuiOptionAction("Place Torches", func() {
			g.captureMouse(previousMouseState)
			g.setEditorMode(ModeProps3D)
			g.selectProp(torchPaletteSlot)
			g.popFromWidgetStack()
		}),
		quads.NewGuiOptionAction("Save Map", func() {
			g.saveMapWithToast()
			g.popFromWidgetStack()
		}),
		quads.NewGuiOptionSubmenu("Load Map", func() {
			g.pushConfigSubmenu(g.mapOptions())
		}),
		quads.NewGuiOptionAction("New Map", func() {
			g.captureMouse(previousMouseState)
			g.popFromWidgetStack()
			g.openNewMapPanel()
		}),
		quads.NewGuiOptionAction("Clear Map", func() {
			g.CubeMap.ClearAllChunks()
			g.popFromWidgetStack()
		}),
	}
	configMenu.SetOptions(options)
	guiWidth, guiHeight := g.GuiSize()
	configMenu.HandleResize(guiWidth, guiHeight, g.UIScale)

	g.replaceWidgetStack(configMenu)
}

// saveMap writes the whole map out: the voxels to their own file beside the map,
// and everything standing in them - props, torches, actors and the routes they
// walk - back into the map file itself. Both halves or neither: a level whose
// walls were saved without its furniture is half a level.
//
// It bakes the atlas down to the textures the map actually uses in the same
// breath. That has to move with the save: baking renumbers the atlas, so a map
// saved without it - or an atlas baked without saving the map - points every
// wall at the wrong texture. The library itself is never written to; the map's
// atlas holds copies, and this is where the copies that nothing points at any
// more are dropped.
func (g *Game) saveMap() error {
	// A thing in hand is out of the level, so a save would lose it: back it goes.
	if g.editor.grabbed != nil {
		g.dropGrabbed(false)
	}
	// The block list and the block in the brush point at slots too, and a bake
	// that dropped them would leave both painting with whatever moved in.
	designs := g.editor.blocks
	if g.editor.voxelBlock != nil {
		designs = append(slices.Clone(designs), designOf(g.editor.voxelBlock))
	}
	remap, bakeErr := g.MapTextures.Bake(keptThroughBake(g.CubeMap.UsedTextureIndices(), designs))
	if bakeErr != nil {
		return bakeErr
	}
	g.CubeMap.RemapTextures(remap)
	remapDesigns(g.editor.blocks, remap)
	if g.editor.voxelBlock != nil {
		brush := []blockDesign{designOf(g.editor.voxelBlock)}
		remapDesigns(brush, remap)
		g.editor.voxelBlock = brush[0].block()
	}
	if err := g.saveBlocks(); err != nil {
		return err
	}
	if err := g.CubeMap.SaveToDisk(g.voxelFile()); err != nil {
		return err
	}
	// The generated stress rig has no folder to go back to. Its voxels are
	// still saved, so a level can be started from one.
	if g.mapName == "" {
		return nil
	}
	return saveJSONMap(mapPathFor(g.mapName), g.mapWithPlacedEntities())
}

// openNewMapPanel asks for a name and starts a level under it - see createMap -
// then goes there. A name that will not do says why and stays open.
func (g *Game) openNewMapPanel() {
	typed, problem := "", ""
	g.openPanel(func() string {
		return "NEW MAP\n\n" + typed + "_\n\n" + problem + "TYPE A NAME   ENTER creates   ESC cancels"
	}, func(key glfw.Key, mods glfw.ModifierKey) bool {
		switch character, typedOne := typedRune(key, mods); {
		case key == glfw.KeyEnter || key == glfw.KeyKPEnter:
			if err := createMap(typed, g.mapSource); err != nil {
				problem = err.Error() + "\n\n"
				return false
			}
			g.switchToMap(typed)
			return true
		case key == glfw.KeyBackspace && len(typed) > 0:
			typed = typed[:len(typed)-1]
		case typedOne:
			typed += string(character)
		}
		return false
	})
}

// mapNamePattern is what a level may be called: it becomes a folder name, so
// nothing that could climb out of assets/maps or trip up a filesystem.
var mapNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// createMap starts a level called name on disk: its folder and a map file with
// the player in the middle of the empty level, and the look of the level it was
// made from - sky, fog, ambient light - without anything that stood in that
// one. There are no blocks to write: a level without a voxel file is an empty
// one, see loadVoxelFile, and the atlas comes with the first load. It refuses a
// name already taken, so a slip on the keyboard never writes over a level.
func createMap(name string, look core.MapLoadResult) error {
	if !mapNamePattern.MatchString(name) {
		return fmt.Errorf("%q will not do: letters, digits, - and _ only", name)
	}
	if _, err := os.Stat(filepath.Join(mapDir, name)); err == nil {
		return fmt.Errorf("there is already a map called %s", name)
	}
	if err := os.MkdirAll(filepath.Join(mapDir, name), 0o755); err != nil {
		return err
	}
	look.PlayerSpawn = mgl32.Vec3{float32(defaultLevelSize.X) / 2, 1, float32(defaultLevelSize.Z) / 2}
	look.PlayerDir = mgl32.Vec2{1, 0}
	look.Entities, look.Lights, look.Panes, look.Station = nil, nil, nil, nil
	return saveJSONMap(mapPathFor(name), look)
}

// voxelFile is where this level's blocks live. The generated stress rig has no
// folder of its own and is saved into one called "scratch", so a level begun by
// carving the rig up can be opened again by name like any other.
func (g *Game) voxelFile() string {
	return voxelPathFor(g.savedAs())
}

// savedAs is the folder this level is written to: its own, or "scratch" for the
// generated rig.
func (g *Game) savedAs() string {
	if g.mapName == "" {
		return "scratch"
	}
	return g.mapName
}

// saveMapWithToast is the save every caller wants: it says on screen what
// happened, because a save whose only sign of failure is a line on a terminal
// nobody is looking at is a save that silently loses a level.
func (g *Game) saveMapWithToast() {
	if err := g.saveMap(); err != nil {
		g.Toast("Save failed: " + err.Error())
		return
	}
	g.Toast(fmt.Sprintf("Saved %s, atlas baked to %d textures", g.voxelFile(), g.MapTextures.Count()))
}

// Editor is the state the map editor keeps: what the brush is holding, what is
// selected, and what a click is currently for. It is a plain struct on Game rather
// than a system of its own, because it has no update of its own to run - it is
// touched by the F1 menus, by the editing keys and by nothing else. A normal run
// never reads it.
type Editor struct {
	// The block ADD paints with, and - while a Line/Plane/Cube drag is
	// mid-way through - the brush dragging it out.
	voxelBlock *voxel.Block
	voxelBrush VoxelBrush
	// blocks is the list the block browser shows - see edit_blocks.go.
	blocks []blockDesign

	// The cells the preview ghosts and the next click would commit.
	voxelSelection []voxel.Int3

	// active is the Minecraft-creative toggle: whether the game is being played
	// or built right now. mode is which family of tools the keys belong to
	// while it is - see EditorMode, and drawEditorHUD, which prints what every
	// key currently does so none of this has to be remembered.
	active bool
	mode   EditorMode

	// addMode is the block mode's Tab state, ADD when true, DEL when false.
	// shapeMethod is the number key's state, which geometry a click applies
	// addMode to. pendingStart is the first corner of a Line/Plane/Cube drag,
	// nil until a click sets it and nil again once the second click or a cancel
	// clears it.
	addMode      bool
	shapeMethod  EditShape
	pendingStart *voxel.Int3
	// aimInFront is the number row's 9: every shape aims at the cell next to
	// the player instead of at what the crosshair is on - see cellInFront.
	aimInFront bool

	// The placement modes' state: a palette per mode, built at startup from the
	// folder that mode places out of, and which slot of each is on the
	// crosshair. One palette per mode rather than one for all four, because a
	// wheel that walks a thousand pieces is a wheel that never arrives - see
	// EditorMode.placesProps.
	//
	// Then: whether a click lands the piece on the tile's centre or exactly
	// where the ray met the floor, how many quarter turns it has been turned,
	// and which sprite the flat decorations are currently showing.
	props         [editorModeCount][]propKind
	propIndex     [editorModeCount]int
	snapToGrid    bool
	facing        int
	spriteTexture int

	// The actor mode's state: the placement palette built from assets/sprites
	// at startup out of every folder that says it is an actor, which slot of it
	// a click places, and which actor the path tool is currently laying a route
	// out for - nil when a floor click makes a new actor instead of a waypoint.
	actors        []core.MapEntity
	actorIndex    int
	selectedActor *Actor

	// The thing the move key has taken out of the level: grabbed is the object
	// it was - dead, and the key to its record until it is put down - grabEntity
	// what putting it down writes, turned and sized by then, and lift how far
	// above the surface under the crosshair it is held. See grabHovered.
	grabbed    *quads.GameObject
	grabEntity core.MapEntity
	lift       float32

	// The ghost following the crosshair: what a click would leave standing
	// there. Which piece it is is the placement palette's own propIndex, so the
	// two can never disagree about what is about to be placed.
	gameObjectForSpawning *quads.GameObject

	currentPaintGunTextureIndex int

	// tool is what a left click does. Off by default, so a click fires the
	// weapon in hand.
	tool EditorTool
}

const (
	torchMeshName = "torch"
	// Far enough past white for the bloom threshold to pick the flame out, the same
	// reason emissive wall panels sit at 7: a torch head is a light source on
	// screen, not a bright decal.
	torchEmissiveStrength = float32(6.0)
	// The torch is 0.625 tall and stands on the floor, so this samples the air
	// block it is standing in rather than whatever the floor boundary rounds to.
	torchLightHeight = float32(0.3)
)

// startTorchPlacement puts a torch on the cursor. It rides the same ghost the
// sprite decorations use - updateGUI moves it to whatever ground the crosshair is
// over, and a left click drops it.
func (g *Game) startTorchPlacement() {
	g.clearGhost()
	g.editor.gameObjectForSpawning = g.spawnTorch(mgl32.Vec3{})
}

// spawnTorch is the model half of a torch: six quads and a flame that glows -
// the glow is spawnEntity's, so a map-loaded torch is as bright as a placed one.
// The light half is CubeMap.AddTorchLight, which only the dropped copy gets - a
// ghost following the crosshair would otherwise re-bake the map every frame.
func (g *Game) spawnTorch(pos mgl32.Vec3) *quads.GameObject {
	return g.spawnEntity(pos, core.MapEntity{Name: "Torch", MeshName: torchMeshName})
}

// dropTorch turns the ghost standing at pos into a placed torch: it stays where it
// is and its light goes into the map, where the next bake floods from it.
func (g *Game) dropTorch(pos mgl32.Vec3) {
	g.CubeMap.AddTorchLight(torchLightCell(pos))
}

// torchLightCell is the voxel a torch standing at pos lights from. Removing a
// torch has to name the same cell adding it did, so both go through here.
func torchLightCell(pos mgl32.Vec3) voxel.Int3 {
	return voxel.PositionToGridInt3(pos.Add(mgl32.Vec3{0, torchLightHeight, 0}))
}

// changeDecorationTexture picks which picture the flat decorations are placed
// with. It is remembered rather than only pushed at the ghost on screen, so the
// next sprite placed is the one that was chosen and not the barrel again.
func (g *Game) changeDecorationTexture() {
	g.pickTexture(g.SpriteTextures, func(selectedIndex int) {
		g.editor.spriteTexture = selectedIndex
		if g.editor.gameObjectForSpawning != nil && g.editor.gameObjectForSpawning.HasComponent(quads.ComponentTypeSprite) {
			g.editor.gameObjectForSpawning.Sprite().TextureIndices = []uint16{uint16(selectedIndex)}
		}
	})
}
func (g *Game) testPackedAtlasPicker() {
	weapon := g.currentWeapon()
	if weapon == nil {
		g.Toast("No packed atlas to pick from")
		return
	}
	picker := g.OpenAtlasPicker(weapon.Atlas)
	picker.SetOnIndexClicked(func(selectedIndex int) {
		println("Selected index", selectedIndex)
	})
	g.replaceWidgetStack(picker)
}

// toggleSpawnMode is the sprite decoration's own key, for putting one down
// without going into editor mode at all: it puts the decoration ghost on the
// crosshair, or takes whatever is on it back off.
func (g *Game) toggleSpawnMode() {
	if g.editor.gameObjectForSpawning != nil {
		g.clearGhost()
		return
	}
	g.setEditorMode(ModeProps2D)
	g.selectProp(spritePaletteSlot)
}

// toggleEditorMode is the Minecraft-creative switch: F5 swaps the whole game
// between playing and building. The mouse never comes uncaptured for either -
// building aims with the crosshair, same as firing does.
func (g *Game) toggleEditorMode() {
	g.editor.active = !g.editor.active
	g.editor.pendingStart = nil
	g.editor.voxelBrush = nil
	if g.editor.active {
		if g.editor.voxelBlock == nil {
			g.editor.voxelBlock = voxel.NewBlock(2)
		}
		if g.editor.shapeMethod == 0 {
			g.editor.shapeMethod = EditShapeSingle
		}
		g.editor.addMode = true
		// Back into whichever mode was left, ghost and all - see setEditorMode.
		// The legend in the corner says which one that is, so coming back into
		// the editor is never a guess.
		g.setEditorMode(g.editor.mode)
		// Building starts flying: most of what is built is out of reach of the
		// floor, and a new level has no floor at all. F lands.
		g.setGravity(false)
	} else {
		// Nothing of the editor survives leaving it: no ghost on the crosshair,
		// no paint gun where a weapon should be.
		g.clearGhost()
		g.editor.tool = EditorToolNone
		g.editor.selectedActor = nil
		// Editor mode is the only place flying makes sense; leaving it always
		// puts the player's feet back on the ground.
		g.setGravity(true)
	}
	g.updateEditorSun()
}

// setGravity is the fly switch, set rather than flipped: the player and the
// physics have to agree on it.
//
// Flying is the builder's frame, not the suit's: gravity off means the world's
// own axes, a level horizon and no thrusters, the way a creative mode flies.
// So switching it off ends an EVA rather than leaving the suit's rolled,
// visor-steered frame on a player who is now flying a level around. Switching
// it back on decides nothing here: evaMove looks at where the player is
// standing on the next frame and starts the suit again if that is vacuum.
func (g *Game) setGravity(on bool) {
	g.gravitationEnabled = on
	g.physicsSystem.SetGravityEnabled(on)
	if !on && g.inEVA {
		g.inEVA = false
		g.Camera.SetFreeLook(false)
		g.Camera.SetRoll(0)
	}
}

// EditShape is what the number keys pick in editor mode: the geometry the next
// click applies ADD or DEL to.
type EditShape int

const (
	EditShapeSingle EditShape = iota + 1
	EditShapeLine
	EditShapePlane
	EditShapeCube
	EditShapeStamp3x3x3
)

func (s EditShape) String() string {
	switch s {
	case EditShapeLine:
		return "Line"
	case EditShapePlane:
		return "Plane"
	case EditShapeCube:
		return "Cube"
	case EditShapeStamp3x3x3:
		return "3x3x3 Stamp"
	}
	return "Single"
}

// editorBrushFor is the two-click brushes: click sets the start corner, a
// second click drags out the shape from the current aim. Single and Stamp
// commit on one click, so they never need one of these.
func editorBrushFor(shape EditShape) VoxelBrush {
	switch shape {
	case EditShapeLine:
		return &VoxelBrushLine{}
	case EditShapePlane:
		return &VoxelBrushRectangle{fill: true}
	case EditShapeCube:
		return &VoxelBrushCube{}
	}
	return nil
}

// editorAimCell is the cell the next edit applies to: the empty cell just
// before whatever the crosshair hit while ADDing, the solid cell it hit while
// DELeting.
func (g *Game) editorAimCell() (voxel.Int3, bool) {
	if g.editor.aimInFront {
		return cellInFront(g.Camera.GetPosition(), g.Camera.GetForward(), g.player.Physics().AABB()), true
	}
	if g.lastHitResult == nil || !g.lastHitResult.HitInfo.Hit || g.lastHitResult.HitObject != nil {
		return voxel.Int3{}, false
	}
	if g.editor.addMode {
		return g.lastHitResult.HitInfo.PreviousGridPosition, true
	}
	return g.lastHitResult.HitInfo.CollisionGridPosition, true
}

// cellInFront is the voxel next to the eye's along whichever axis the view
// points along most - ahead, or above or below when looking steeply - so a
// structure can be started in empty space, where the crosshair has nothing to
// land on. It steps on past any cell the player's body is in: a block placed
// there would swallow them.
func cellInFront(eye, forward mgl32.Vec3, body voxel.AABB) voxel.Int3 {
	axis := 0
	for i := 1; i < 3; i++ {
		if math.Abs(float64(forward[i])) > math.Abs(float64(forward[axis])) {
			axis = i
		}
	}
	var step [3]int32
	step[axis] = 1
	if forward[axis] < 0 {
		step[axis] = -1
	}
	cell := voxel.PositionToGridInt3(eye)
	for {
		cell = voxel.Int3{X: cell.X + step[0], Y: cell.Y + step[1], Z: cell.Z + step[2]}
		if !overlapsCell(body, cell) {
			return cell
		}
	}
}

func overlapsCell(box voxel.AABB, cell voxel.Int3) bool {
	corner := [3]float32{float32(cell.X), float32(cell.Y), float32(cell.Z)}
	for i := range corner {
		if box.Max()[i] <= corner[i] || box.Min()[i] >= corner[i]+1 {
			return false
		}
	}
	return true
}

// stampAround is the method 5 shape: a 3x3x3 block of cells centered on aim.
func stampAround(center voxel.Int3) []voxel.Int3 {
	cells := make([]voxel.Int3, 0, 27)
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			for dz := int32(-1); dz <= 1; dz++ {
				cells = append(cells, voxel.Int3{X: center.X + dx, Y: center.Y + dy, Z: center.Z + dz})
			}
		}
	}
	return cells
}

// updateEditorPreview keeps the selection the unit cube ghosts every frame:
// the single cell under the crosshair, the shape dragged out so far from a
// pending start, or the stamp's 3x3x3 block. Called every frame editor mode
// is on, captured mouse or not, since the crosshair - not the OS cursor - is
// what aims now.
func (g *Game) updateEditorPreview() {
	// Only the block tools ghost cells. The prop tools have a better preview -
	// the piece itself, standing where it would land - and the actor tools use
	// the same cubes for the selected actor's route instead.
	switch g.editor.mode {
	case ModeActors:
		g.drawActorPath()
		return
	}
	if g.editor.mode.placesProps() {
		return
	}
	aim, hit := g.editorAimCell()
	if !hit {
		g.editor.voxelSelection = nil
		return
	}
	switch g.editor.shapeMethod {
	case EditShapeStamp3x3x3:
		g.editor.voxelSelection = stampAround(aim)
	case EditShapeLine, EditShapePlane, EditShapeCube:
		if g.editor.pendingStart != nil && g.editor.voxelBrush != nil {
			g.editor.voxelSelection = g.editor.voxelBrush.Drag(aim)
		} else {
			g.editor.voxelSelection = []voxel.Int3{aim}
		}
	default:
		g.editor.voxelSelection = []voxel.Int3{aim}
	}
	g.UnitCube.QueueDraw(g.editor.voxelSelection)
}

// applyEditorEdit is what a left click does in editor mode: single and stamp
// commit immediately, the drag shapes take a click for the start corner and a
// second click - anywhere, the crosshair is still aiming - to commit.
func (g *Game) applyEditorEdit() {
	aim, hit := g.editorAimCell()
	if !hit {
		return
	}
	switch g.editor.shapeMethod {
	case EditShapeLine, EditShapePlane, EditShapeCube:
		if g.editor.pendingStart == nil {
			g.editor.voxelBrush = editorBrushFor(g.editor.shapeMethod)
			g.editor.voxelBrush.StartDrag(aim)
			start := aim
			g.editor.pendingStart = &start
			return
		}
		g.commitEditorEdit(g.editor.voxelBrush.Drag(aim))
		g.editor.pendingStart = nil
		g.editor.voxelBrush = nil
	case EditShapeStamp3x3x3:
		g.commitEditorEdit(stampAround(aim))
	default:
		g.commitEditorEdit([]voxel.Int3{aim})
	}
}

// cancelEditorSelection drops a pending start corner - the right click's job
// while editor mode is on, since it fires nothing while empty handed.
func (g *Game) cancelEditorSelection() {
	g.editor.pendingStart = nil
	g.editor.voxelBrush = nil
}

func (g *Game) commitEditorEdit(cells []voxel.Int3) {
	if g.editor.addMode {
		g.voxelPlaceBlock(cells)
	} else {
		g.voxelRemoveBlock(cells)
	}
}

func (g *Game) voxelPlaceBlock(pos []voxel.Int3) {
	g.CubeMap.SetBlocks(pos, g.editor.voxelBlock.Copy())
}

func (g *Game) voxelRemoveBlock(pos []voxel.Int3) {
	g.CubeMap.SetAirBlocks(pos)
}

type VoxelBrush interface {
	StartDrag(pos voxel.Int3) []voxel.Int3
	Drag(cur voxel.Int3) []voxel.Int3
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
	cur = voxel.Int3{X: cur.X, Y: v.start.Y, Z: cur.Z}
	minX := min(v.start.X, cur.X)
	maxX := max(v.start.X, cur.X)

	minZ := min(v.start.Z, cur.Z)
	maxZ := max(v.start.Z, cur.Z)
	var rect []voxel.Int3

	if v.fill {
		for x := int(minX); x <= int(maxX); x++ {
			for z := int(minZ); z <= int(maxZ); z++ {
				rect = append(rect, voxel.Int3{X: int32(x), Y: v.start.Y, Z: int32(z)})
			}
		}
	} else {
		for x := int(minX); x <= int(maxX); x++ {
			rect = append(rect, voxel.Int3{X: int32(x), Y: v.start.Y, Z: minZ})
			rect = append(rect, voxel.Int3{X: int32(x), Y: v.start.Y, Z: maxZ})
		}
		for z := int(minZ); z <= int(maxZ); z++ {
			rect = append(rect, voxel.Int3{X: minX, Y: v.start.Y, Z: int32(z)})
			rect = append(rect, voxel.Int3{X: maxX, Y: v.start.Y, Z: int32(z)})
		}
	}

	return rect
}

// VoxelBrushCube is method 4: a filled box between two arbitrary corners, the
// only brush that varies all three axes.
type VoxelBrushCube struct {
	start voxel.Int3
}

func (v *VoxelBrushCube) StartDrag(pos voxel.Int3) []voxel.Int3 {
	v.start = pos
	return []voxel.Int3{pos}
}

func (v *VoxelBrushCube) Drag(cur voxel.Int3) []voxel.Int3 {
	minX, maxX := min(v.start.X, cur.X), max(v.start.X, cur.X)
	minY, maxY := min(v.start.Y, cur.Y), max(v.start.Y, cur.Y)
	minZ, maxZ := min(v.start.Z, cur.Z), max(v.start.Z, cur.Z)
	var box []voxel.Int3
	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			for z := minZ; z <= maxZ; z++ {
				box = append(box, voxel.Int3{X: x, Y: y, Z: z})
			}
		}
	}
	return box
}
