package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// The map editor's three tools, and the one idea that keeps them off menus:
// the editor is always in exactly one mode, one key cycles it, and every other
// key means whatever that mode says it means. The number row, Tab, the wheel
// and the two mouse buttons are the whole interface; drawEditorHUD prints what
// they currently do in the corner, so there is nothing to remember and nothing
// to navigate. The F1/R menus are still there for the things a key cannot
// carry - a texture out of an atlas of hundreds, save, load.

// EditorMode is which family of tools the keys and clicks belong to right now.
type EditorMode int

const (
	// ModeBlocks builds the level itself out of voxels. It is where a session
	// starts, because a map with nothing in it has nowhere to stand.
	ModeBlocks EditorMode = iota
	ModeProps
	ModeActors
	editorModeCount
)

func (m EditorMode) String() string {
	switch m {
	case ModeProps:
		return "PROPS"
	case ModeActors:
		return "ACTORS"
	}
	return "BLOCKS"
}

// propKind is one slot in the placement palette: a piece of scenery by its
// folder name, the torch, or a flat sprite decoration. The list is built once
// at startup from whatever assets/meshes holds, so a new folder with a
// scenery.json in it is a new slot on the number row and not a line of Go.
type propKind struct {
	label   string
	scenery string
	torch   bool
	sprite  bool
}

// actorKinds is what the actor tool can place, and everything a map file needs
// to spawn one again: the sheet it animates from and how big a frame on it is.
// actorNames is derived from it, so a name here is an actor everywhere.
var actorKinds = []core.MapEntity{
	{
		Name:                "Scientist",
		TextureIndices:      []uint16{0, 1, 2, 3, 4, 5, 6, 7},
		SpriteSheetName:     "scientist",
		SpriteSheetTileSize: geometry.Point{X: 64, Y: 64},
		CreateCollider:      true,
	},
}

// defaultDecorationTexture is the sprite the decoration ghost starts as - the
// barrel. Any other one is a pick out of the sprite atlas, see
// changeDecorationTexture.
const defaultDecorationTexture = 136

// The two palette slots that are not folders, so the keys and menus that place
// one directly do not have to search for it.
const (
	torchPaletteSlot = iota
	spritePaletteSlot
)

// buildPropPalette fills the placement palette: the torch and the sprite
// decoration first, because neither is a folder, then the scenery in the
// folder's own order.
func (g *Game) buildPropPalette() {
	palette := []propKind{
		torchPaletteSlot:  {label: "Torch", torch: true},
		spritePaletteSlot: {label: "Sprite", sprite: true},
	}
	for _, id := range g.sceneryIDs() {
		palette = append(palette, propKind{label: g.scenery[id].Name, scenery: id})
	}
	g.editor.props = palette
}

// cycleEditorMode is the mode key: BLOCKS -> PROPS -> ACTORS -> BLOCKS. Three
// modes is short enough to walk one way round.
func (g *Game) cycleEditorMode() {
	g.setEditorMode((g.editor.mode + 1) % editorModeCount)
}

// setEditorMode drops whatever the mode being left was holding - a half-dragged
// voxel shape, the ghost on the crosshair - so a mode is always entered clean,
// and puts the new mode's own ghost back on the crosshair on the way in.

func (g *Game) setEditorMode(mode EditorMode) {
	g.cancelEditorSelection()
	g.clearGhost()
	g.editor.mode = mode
	if mode == ModeProps {
		g.selectProp(g.editor.propIndex)
		return
	}
	g.Toast(mode.String())
}

// editorNumberKey is the number row, which every mode uses for its own slots:
// the voxel shapes and the three paint tools, the placement palette, the kind
// of actor. index is zero based.
func (g *Game) editorNumberKey(index int) {
	switch g.editor.mode {
	case ModeBlocks:
		g.selectBlockSlot(index)
	case ModeProps:
		// Only a slot that exists. The wheel wraps round the palette; a number
		// key that wrapped would quietly pick something else when the row is
		// longer than the palette.
		if index < len(g.editor.props) {
			g.selectProp(index)
		}
	case ModeActors:
		if index < len(actorKinds) {
			g.editor.actorIndex = index
			g.Toast("Place: " + actorKinds[index].Name)
		}
	}
}

// selectBlockSlot maps the number row onto what a click does to voxels: 1-5
// are the five shapes it builds and digs with, 6-8 the three tools that work a
// surface that is already there. They share the row because they are the same
// question - what does this click do to the block I am looking at - and picking
// a shape puts the tools away rather than leaving a paint gun silently on.
func (g *Game) selectBlockSlot(index int) {
	if index < len(editShapes) {
		g.editor.shapeMethod = editShapes[index]
		g.cancelEditorSelection()
		g.editor.tool = EditorToolNone
		g.Toast(g.editor.shapeMethod.String())
		return
	}
	tools := []EditorTool{EditorToolDig, EditorToolPaint, EditorToolFill}
	if slot := index - len(editShapes); slot < len(tools) {
		g.setEditorTool(tools[slot])
	}
}

// editShapes is the number row's half of EditShape, in the order the keys are
// under a hand.
var editShapes = []EditShape{EditShapeSingle, EditShapeLine, EditShapePlane, EditShapeCube, EditShapeStamp3x3x3}

// editorTabAction is Tab: the one toggle each mode has. Which toggle it is is
// on screen, so the key is "flip this mode's switch" rather than three keys to
// remember.
func (g *Game) editorTabAction() {
	switch g.editor.mode {
	case ModeBlocks:
		g.editor.addMode = !g.editor.addMode
		g.cancelEditorSelection()
		g.Toast(map[bool]string{true: "ADD", false: "DEL"}[g.editor.addMode])
	case ModeProps:
		g.editor.snapToGrid = !g.editor.snapToGrid
		g.Toast(map[bool]string{true: "snap to grid", false: "free placement"}[g.editor.snapToGrid])
	case ModeActors:
		g.editor.selectedActor = nil
		g.Toast("Nothing selected - a click on the floor places an actor")
	}
}

// editorScroll is the wheel, which walks whatever the number row picks: the
// voxel shape, the placement palette, the kind of actor. It exists so the
// palette can be longer than nine slots without a menu.
func (g *Game) editorScroll(yoff float64) {
	step := 1
	if yoff < 0 {
		step = -1
	}
	switch g.editor.mode {
	case ModeBlocks:
		g.selectBlockSlot((slices.Index(editShapes, g.editor.shapeMethod) + step + len(editShapes)) % len(editShapes))
	case ModeProps:
		g.selectProp(g.editor.propIndex + step)
	case ModeActors:
		g.editor.actorIndex = (g.editor.actorIndex + step + len(actorKinds)) % len(actorKinds)
		g.Toast("Place: " + actorKinds[g.editor.actorIndex].Name)
	}
}

// selectProp puts palette slot index on the crosshair, wrapping at both ends so
// the wheel never runs out. The ghost is spawned here rather than at click
// time, which is what makes the placement WYSIWYG: what is standing under the
// crosshair is the thing a click leaves behind.
func (g *Game) selectProp(index int) {
	if len(g.editor.props) == 0 {
		return
	}
	g.editor.propIndex = (index%len(g.editor.props) + len(g.editor.props)) % len(g.editor.props)
	kind := g.editor.props[g.editor.propIndex]
	switch {
	case kind.torch:
		g.startTorchPlacement()
	case kind.sprite:
		g.startSpritePlacement()
	default:
		g.startSceneryPlacement(kind.scenery)
	}
	g.applyGhostFacing()
	g.Toast("Place: " + kind.label)
}

// selectPropByScenery puts a piece on the crosshair by its folder name, for the
// menu that still lists the scenery by hand. It goes through the palette rather
// than round it, so the piece being placed and the piece a click writes to the
// map file are always the same one.
func (g *Game) selectPropByScenery(id string) {
	for index, kind := range g.editor.props {
		if kind.scenery == id {
			g.selectProp(index)
			return
		}
	}
}

// startSpritePlacement puts a flat decoration on the crosshair - a barrel, a
// plant, whatever the sprite atlas holds. Which picture it is is the decoration
// texture, which has its own key; this only starts the ghost.
func (g *Game) startSpritePlacement() {
	g.clearGhost()
	g.editor.gameObjectForSpawning = g.spawnEntity(mgl32.Vec3{}, g.decorationEntity(false))
	g.editor.gameObjectForSpawning.Sprite().Tint = mgl32.Vec4{1, 1, 1, 0.5}
}

// decorationEntity is the sprite decoration as the map format describes it.
// solid is false for the ghost: something you can walk into is not a ghost.
func (g *Game) decorationEntity(solid bool) core.MapEntity {
	return core.MapEntity{
		Name:           "Decoration",
		TextureIndices: []uint16{uint16(g.editor.spriteTexture)},
		CreateCollider: solid,
		IsTransparent:  !solid,
	}
}

// clearGhost takes whatever is on the crosshair off it.
func (g *Game) clearGhost() {
	if g.editor.gameObjectForSpawning != nil {
		g.editor.gameObjectForSpawning.Kill()
		g.editor.gameObjectForSpawning = nil
	}
}

// rotateProp is the turn key: a quarter turn of the piece on the crosshair,
// which is the whole of rotation because everything in a voxel map lines up
// with the grid anyway. See propFacing for why turn zero is not a rotation.
func (g *Game) rotateProp() {
	g.editor.facing = (g.editor.facing + 1) % 4
	g.applyGhostFacing()
	g.Toast("Facing " + facingNames[g.editor.facing])
}

var facingNames = [4]string{"as modelled", "east", "south", "west"}

// propFacing is the direction the turn key has turned the ghost to, in the
// shape the map format stores. A turn of zero is the zero vector on purpose: it
// means "however the model was exported", which is what every prop placed
// before there was a turn key looks like, so the default placement is unchanged.
func propFacing(quarterTurns int) mgl32.Vec3 {
	switch quarterTurns % 4 {
	case 1:
		return mgl32.Vec3{1, 0, 0}
	case 2:
		return mgl32.Vec3{0, 0, 1}
	case 3:
		return mgl32.Vec3{-1, 0, 0}
	}
	return mgl32.Vec3{}
}

func (g *Game) applyGhostFacing() {
	ghost := g.editor.gameObjectForSpawning
	if ghost == nil {
		return
	}
	if facing := propFacing(g.editor.facing); facing.Len() > 0 {
		ghost.Transform().SetForward2D(facing)
	} else {
		ghost.Transform().SetRotation(mgl32.QuatIdent())
	}
}

// ghostPosition is where the piece on the crosshair stands: exactly where the
// ray met the floor, or - with snap on - the middle of that floor tile. Snap
// only touches the two horizontal axes; the height stays the surface the ray
// actually hit, so a prop snapped onto a staircase step stands on the step
// rather than sinking to the tile's nominal floor.
func ghostPosition(groundPos mgl32.Vec3, snap bool) mgl32.Vec3 {
	if !snap {
		return groundPos
	}
	return mgl32.Vec3{
		float32(math.Floor(float64(groundPos.X()))) + 0.5,
		groundPos.Y(),
		float32(math.Floor(float64(groundPos.Z()))) + 0.5,
	}
}

// placeProp drops what is on the crosshair into the world for good. The real
// copy is spawned from the same description a map file would spawn it from, so
// everything the piece is - a shelf's own colliders, a ladder's climb volume, a
// torch's light - comes out of the one path that already works, and the
// description is remembered so a save writes it back out. The ghost stays on
// the crosshair for the next one.
func (g *Game) placeProp() {
	ghost := g.editor.gameObjectForSpawning
	if ghost == nil {
		return
	}
	at := ghost.Transform().GetPosition()
	entity := g.propEntity()
	placed := g.spawnEntity(at, entity)
	if placed == nil {
		return
	}
	if entity.MeshName == torchMeshName {
		g.dropTorch(at)
	}
	g.recordPlaced(placed, entity, at)
	// A fresh ghost, because the one that was on the crosshair is now the
	// piece standing on the floor... except for scenery, which spawned a
	// separate solid copy - either way the crosshair gets a new one.
	g.selectProp(g.editor.propIndex)
}

// propEntity is the palette's current slot as the map format describes it.
func (g *Game) propEntity() core.MapEntity {
	kind := g.editor.props[g.editor.propIndex]
	switch {
	case kind.torch:
		return core.MapEntity{Name: "Torch", MeshName: torchMeshName, Facing: propFacing(g.editor.facing)}
	case kind.sprite:
		return g.decorationEntity(true)
	}
	return core.MapEntity{
		Name:           g.scenery[kind.scenery].Name,
		MeshName:       kind.scenery,
		CreateCollider: true,
		Facing:         propFacing(g.editor.facing),
	}
}

// deleteHovered removes the thing the crosshair is on. Killing it is all it
// takes: the mesh, sprite and physics systems all drop a dead object on their
// next pass, and a save skips it - see mapWithPlacedEntities.
func (g *Game) deleteHovered() {
	object := g.editorHoveredObject()
	if object == nil {
		return
	}
	object.Kill()
	// A torch is a light as much as a model, and killing the model does not put
	// the light out: the flood fill lives in the voxels, not on the object.
	if record := g.placedRecord(object); record != nil && record.entity.MeshName == torchMeshName {
		g.CubeMap.RemoveTorchLight(torchLightCell(record.pos))
	}
	if actor := g.actorFor(object); actor != nil && actor == g.editor.selectedActor {
		g.editor.selectedActor = nil
	}
	g.Toast("Removed " + object.DisplayName)
}

// editorHoveredObject is whatever the crosshair is on, furniture included.
// hoveredObj deliberately looks through scenery - a shelf is somewhere to put
// things, not something to pick out - but the editor is exactly where picking
// the shelf itself is the point.
func (g *Game) editorHoveredObject() *quads.GameObject {
	if g.lastHitResult == nil || !g.lastHitResult.HitInfo.Hit {
		return nil
	}
	return g.lastHitResult.HitObject
}

// actorClick is the whole of the actor tool on one button. What it does follows
// from what is under the crosshair and what is already selected, so there is no
// sub-mode to be in: click an actor to select it, click the floor to give the
// selected actor somewhere else to walk, click the floor with nothing selected
// to put a new actor there.
func (g *Game) actorClick() {
	if actor := g.actorFor(g.editorHoveredObject()); actor != nil {
		g.editor.selectedActor = actor
		g.Toast(fmt.Sprintf("%s selected - click the floor to add a waypoint", actor.Name))
		return
	}
	if g.lastHitResult == nil {
		return
	}
	at, onGround := g.lastHitResult.GroundPosition()
	if !onGround {
		return
	}
	if g.editor.selectedActor != nil {
		g.addWaypoint(g.editor.selectedActor, at)
		return
	}
	g.placeActor(at)
}

// placeActor drops an NPC where the crosshair is and selects it, so the clicks
// straight after it lay out the beat it walks.
func (g *Game) placeActor(at mgl32.Vec3) {
	entity := actorKinds[g.editor.actorIndex]
	// The palette's own slice, otherwise: spawnEntity rewrites a texture index
	// it does not like, in place, in the entry every later placement copies.
	entity.TextureIndices = slices.Clone(entity.TextureIndices)
	object := g.spawnEntity(at, entity)
	if object == nil {
		return
	}
	g.recordPlaced(object, entity, at)
	g.editor.selectedActor = g.actorFor(object)
	g.Toast(entity.Name + " placed - click the floor to add a waypoint")
}

// addWaypoint extends the selected actor's beat. It is appended to the live
// patrol rather than to a list kept beside it, so the actor walks the new leg
// as soon as it finishes the one it is on - the route is edited by watching it
// being walked, which is the only way to tell whether it is a route the
// pathfinder can actually follow.
func (g *Game) addWaypoint(actor *Actor, at mgl32.Vec3) {
	actor.patrol = append(actor.patrol, at)
	g.Toast(fmt.Sprintf("%s: %d waypoints", actor.Name, len(actor.patrol)-1))
}

// dropLastWaypoint is the undo for the click above. The first point is the
// actor's own spawn and is not a waypoint, so it is never dropped.
func (g *Game) dropLastWaypoint() {
	actor := g.editor.selectedActor
	if actor == nil || len(actor.patrol) <= 1 {
		return
	}
	actor.patrol = actor.patrol[:len(actor.patrol)-1]
	actor.patrolIndex %= len(actor.patrol)
	g.Toast(fmt.Sprintf("%s: %d waypoints", actor.Name, len(actor.patrol)-1))
}

// drawActorPath ghosts the selected actor's beat as a unit cube on every point
// of it, the same cubes the voxel selection uses. A patrol route is otherwise
// invisible: an actor walking one looks exactly like an actor wandering.
func (g *Game) drawActorPath() {
	actor := g.editor.selectedActor
	if actor == nil || !actor.object.IsAlive() {
		return
	}
	cells := make([]voxel.Int3, 0, len(actor.patrol))
	for _, point := range actor.patrol {
		cells = append(cells, voxel.PositionToGridInt3(point))
	}
	g.UnitCube.QueueDraw(cells)
}

// placedEntity is one thing standing in the world that the map file holds: the
// object itself, and the description it would be spawned from again. The list
// of them is what a save writes out, which is how something dropped in the
// editor is still there the next time the map is loaded.
type placedEntity struct {
	// object is nil for an entity that is a marker rather than a thing - the
	// weapon range's corner, say, which spawnEntity acts on and returns
	// nothing for. It still belongs in the file.
	object *quads.GameObject
	entity core.MapEntity
	pos    mgl32.Vec3
}

func (g *Game) recordPlaced(object *quads.GameObject, entity core.MapEntity, at mgl32.Vec3) {
	g.placed = append(g.placed, placedEntity{object: object, entity: entity, pos: at})
}

// placedRecord is what the map file says about an object standing in the world,
// or nil for one that is not in it - a projectile, the player. A linear scan of
// a level's furniture is fine: this runs on a click, not on a frame.
func (g *Game) placedRecord(object *quads.GameObject) *placedEntity {
	for i := range g.placed {
		if g.placed[i].object == object {
			return &g.placed[i]
		}
	}
	return nil
}

// mapWithPlacedEntities is the loaded map with its entity lists rebuilt from
// what is actually standing in the world right now. Every tile's list is
// cleared first, so something the editor deleted is gone from the file rather
// than merely dead in memory, and the map the editor started from is not
// mutated - a failed save leaves the running level alone.
//
// A position becomes a tile plus an offset inside it, because that is the shape
// the format has: see toVoxelMap, which spawns from tile centre + LocalOffset.
// That is what lets free placement round-trip at all - a prop dropped between
// two tile centres comes back exactly where it was put.
func (g *Game) mapWithPlacedEntities() core.MapLoadResult {
	result := g.mapSource
	tiles := slices.Clone(result.MapData)
	for i := range tiles {
		tiles[i].Entities = nil
	}
	width := result.MapDimensions[0]
	for _, placed := range g.placed {
		if placed.object != nil && !placed.object.IsAlive() {
			continue
		}
		tileX := int(math.Floor(float64(placed.pos.X())))
		tileZ := int(math.Floor(float64(placed.pos.Z())))
		index := tileZ*width + tileX
		if tileX < 0 || tileX >= width || index < 0 || index >= len(tiles) {
			continue
		}
		entity := placed.entity
		entity.LocalOffset = placed.pos.Sub(tileSpawnPoint(tileX, tileZ, tiles[index].FloorOffset))
		if actor := g.actorFor(placed.object); actor != nil && len(actor.patrol) > 1 {
			// The live route, not the one it was spawned with: the path tool
			// edits the actor, and the actor is what the file has to agree with.
			// Point zero is the spawn, which the position already says.
			entity.Patrol = slices.Clone(actor.patrol[1:])
			entity.PatrolTo = nil
		}
		tiles[index].Entities = append(tiles[index].Entities, entity)
	}
	result.MapData = tiles
	return result
}

// tileSpawnPoint is where toVoxelMap puts an entity with no offset: the middle
// of the tile, standing on its floor.
func tileSpawnPoint(x, z, floorOffset int) mgl32.Vec3 {
	return mgl32.Vec3{float32(x) + 0.5, float32(floorOffset + 1), float32(z) + 0.5}
}

// drawEditorHUD is the editor's whole interface, and the reason none of it
// needs a menu: it prints, in the corner, which mode is on, what the mode is
// currently holding, and what every live key does. The keys are read out of the
// bindings rather than written down here, so a rebind moves the legend with it.
func (g *Game) drawEditorHUD() {
	if g.editorHUD == nil {
		return
	}
	g.editorHUD.SetMultilineText([]string{
		g.editorModeLine(),
		g.editorStateLine(),
		g.editorKeyLine(),
		fmt.Sprintf("%s mode   %s menu   %s fly   Ctrl+S save   %s play",
			g.keyLabel(ActionEditorCycleMode), g.keyLabel(ActionOpenVoxelEditMenu),
			g.keyLabel(ActionToggleGravity), g.keyLabel(ActionToggleEditorMode)),
	})
	_, guiHeight := g.GuiSize()
	_, panelHeight := g.editorHUD.Size()
	g.editorHUD.SetPosition(mgl32.Vec2{hudMargin, float32(guiHeight-hudMargin) - float32(panelHeight)})
	g.editorHUD.Draw()
}

// editorModeLine is the three modes with the live one in brackets, so which
// one the keys belong to is readable at a glance rather than remembered.
func (g *Game) editorModeLine() string {
	line := "EDITOR "
	for mode := ModeBlocks; mode < editorModeCount; mode++ {
		if mode == g.editor.mode {
			line += " [" + mode.String() + "]"
			continue
		}
		line += "  " + mode.String()
	}
	return line
}

// editorStateLine is what the current mode is holding: the block a click would
// lay and the shape it would lay it in, the piece on the crosshair and how it
// will land, the actor whose route is being drawn.
func (g *Game) editorStateLine() string {
	switch g.editor.mode {
	case ModeProps:
		placement := "free"
		if g.editor.snapToGrid {
			placement = "snap"
		}
		label := "nothing"
		if len(g.editor.props) > 0 {
			label = g.editor.props[g.editor.propIndex].label
		}
		return fmt.Sprintf("%s   %s   facing %s", label, placement, facingNames[g.editor.facing])
	case ModeActors:
		kind := actorKinds[g.editor.actorIndex].Name
		if actor := g.editor.selectedActor; actor != nil && actor.object.IsAlive() {
			return fmt.Sprintf("%s   editing %s, %d waypoints", kind, actor.Name, len(actor.patrol)-1)
		}
		return kind + "   nothing selected"
	}
	if g.editor.tool != EditorToolNone {
		return "tool: " + g.editor.tool.String()
	}
	return map[bool]string{true: "ADD", false: "DEL"}[g.editor.addMode] + "   " + g.editor.shapeMethod.String()
}

// editorKeyLine is the mode's own keys. Only the mode's own: a legend that
// listed all three modes' keys at once would be the menu this is here to avoid.
func (g *Game) editorKeyLine() string {
	tab := g.keyLabel(ActionEditorTabAction)
	switch g.editor.mode {
	case ModeProps:
		return fmt.Sprintf("1-9/wheel piece   %s snap   %s turn   LMB place   RMB delete",
			tab, g.keyLabel(ActionEditorRotate))
	case ModeActors:
		return fmt.Sprintf("1-9 kind   LMB place/select/waypoint   RMB undo point   %s deselect", tab)
	}
	return fmt.Sprintf("1-5 shape   6-8 dig/paint/fill   %s add/del   LMB build   RMB cancel", tab)
}

// keyLabel names the key an action is bound to. The legend is the whole
// interface, so it has to say what is actually bound rather than what the
// defaults were - see keybindings.txt.
func (g *Game) keyLabel(action string) string {
	bound := g.key(action)
	for name, key := range keyByName {
		if key == bound {
			return name
		}
	}
	return "?"
}

// editorModeByName is -editor's argument. Anything unrecognised is the block
// tools, which is where the editor starts anyway.
func editorModeByName(name string) EditorMode {
	switch name {
	case "props":
		return ModeProps
	case "actors":
		return ModeActors
	}
	return ModeBlocks
}
