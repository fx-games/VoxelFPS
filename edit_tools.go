package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
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
	// The four placement modes. They were one - PROPS - and one palette of
	// everything placeable, which was fine at a dozen pieces and unusable at a
	// thousand: the models, the flat decorations, the things the player picks up
	// and the guns are four different jobs done at four different times, and a
	// wheel that walks all of them at once is a wheel nobody reaches the end of.
	// Each mode now has its own palette and its own folder to browse - see
	// buildPropPalette and openModePicker.
	ModeProps3D
	ModeProps2D
	ModePickups
	ModeWeapons
	ModeActors
	editorModeCount
)

func (m EditorMode) String() string {
	switch m {
	case ModeProps3D:
		return "3D PROPS"
	case ModeProps2D:
		return "2D PROPS"
	case ModePickups:
		return "PICKUPS"
	case ModeWeapons:
		return "WEAPONS"
	case ModeActors:
		return "ACTORS"
	}
	return "BLOCKS"
}

// placesProps is whether this mode puts a ghost on the crosshair out of a
// palette. The four that do share every key: the number row, the wheel, Tab's
// snap, the turn key and both mouse buttons. Blocks and actors are their own.
func (m EditorMode) placesProps() bool {
	return m >= ModeProps3D && m <= ModeWeapons
}

// propKind is one slot in the placement palette: a piece of scenery by its
// folder name, the torch, or a flat sprite decoration. The list is built once
// at startup from whatever assets/meshes holds, so a new folder with a
// scenery.json in it is a new slot on the number row and not a line of Go.
type propKind struct {
	label   string
	scenery string
	// spriteKind is a folder under assets/sprites/ that is not an actor: a
	// barrel, a jet of steam, a corpse on the floor. It is a slot here rather
	// than in the actor palette because it is furniture that happens to be a
	// picture, and because the barrel the request was about has four frames and
	// has to stay one object.
	spriteKind string
	// weapon is a folder under assets/weapons/ standing on the floor as
	// something to pick up, and ammo is whether it is the gun or its ammunition
	// - the two pictures a weapon brings with it. See spawnPickup, which is what
	// puts one in the world, and core.MapEntity.WeaponName, which is how a map
	// remembers it.
	weapon string
	ammo   bool
	torch  bool
	sprite bool
}

// buildActorPalette fills what the actor tool can place: every folder under
// assets/sprites/ that says it is an actor, in folder order. Each slot is
// everything a map file needs to spawn one again - the kind it animates from,
// how big a frame on its sheet is, and the idle frames it stands on before any
// animation starts. There is no list of actors in Go and there must not be
// one; adding a character is adding a folder.
func (g *Game) buildActorPalette() {
	var palette []core.MapEntity
	for _, id := range g.spriteKindIDs() {
		kind := g.spriteKinds[id]
		if !kind.Actor {
			continue
		}
		palette = append(palette, core.MapEntity{
			Name:                kind.Name,
			TextureIndices:      kind.idleFrames(),
			SpriteSheetName:     kind.ID,
			SpriteSheetTileSize: kind.Tile,
			CreateCollider:      true,
		})
	}
	g.editor.actors = palette
}

// defaultDecorationTexture is the sprite the decoration ghost starts as - the
// barrel. Any other one is a pick out of the sprite atlas, see
// changeDecorationTexture.
const defaultDecorationTexture = 136

// The two palette slots that are not folders, so the keys and menus that place
// one directly do not have to search for them. Both are zero because they are in
// different palettes now - the torch is a model and the raw sprite is a picture -
// and each goes first in its own.
const (
	torchPaletteSlot  = 0
	spritePaletteSlot = 0
)

// buildPropPalette fills the four placement palettes, each out of the folder its
// mode places from: the models under assets/meshes, the flat decorations and the
// things the player picks up under assets/sprites, and the guns under
// assets/weapons. There is no list of pieces in Go and there must not be one -
// adding a piece is adding a folder.
//
// The torch and the raw sprite decoration are the two slots that are not folders,
// and they go first in the mode each belongs to so the keys that place one
// directly do not have to search for it.
func (g *Game) buildPropPalette() {
	models := []propKind{torchPaletteSlot: {label: "Torch", torch: true}}
	for _, id := range g.sceneryIDs() {
		models = append(models, propKind{label: g.scenery[id].Name, scenery: id})
	}
	g.editor.props[ModeProps3D] = models

	flat := []propKind{spritePaletteSlot: {label: "Sprite", sprite: true}}
	var items []propKind
	for _, id := range g.spriteKindIDs() {
		kind := g.spriteKinds[id]
		if kind.Actor {
			continue
		}
		slot := propKind{label: kind.Name, spriteKind: id}
		// Which of the two palettes a picture belongs in is which role folder it
		// is filed under: something the player walks over and takes is placed
		// while dressing a level with loot, not while dressing it with furniture.
		if spriteRole(id) == itemsRole {
			items = append(items, slot)
			continue
		}
		flat = append(flat, slot)
	}
	g.editor.props[ModeProps2D] = flat
	g.editor.props[ModePickups] = items

	var guns []propKind
	for _, weapon := range g.weapons {
		guns = append(guns,
			propKind{label: weapon.Data.Tag, weapon: weapon.Data.ID},
			propKind{label: weapon.Data.Tag + " ammo", weapon: weapon.Data.ID, ammo: true})
	}
	g.editor.props[ModeWeapons] = guns
}

// palette is the current mode's own, and paletteIndex the slot of it on the
// crosshair. Every placement key goes through these rather than through a field,
// so the four modes share one implementation of picking, ghosting and placing.
func (g *Game) palette() []propKind {
	return g.editor.props[g.editor.mode]
}

func (g *Game) paletteIndex() int {
	return g.editor.propIndex[g.editor.mode]
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
	if mode.placesProps() {
		g.selectProp(g.paletteIndex())
	}
}

// editorNumberKey is the number row, which every mode uses for its own slots:
// the voxel shapes and the three paint tools, the placement palette, the kind
// of actor. index is zero based.
func (g *Game) editorNumberKey(index int) {
	if g.editor.mode.placesProps() {
		// Only a slot that exists. The wheel wraps round the palette; a number
		// key that wrapped would quietly pick something else when the row is
		// longer than the palette.
		if index < len(g.palette()) {
			g.selectProp(index)
		}
		return
	}
	switch g.editor.mode {
	case ModeBlocks:
		g.selectBlockSlot(index)
	case ModeActors:
		if index < len(g.editor.actors) {
			g.editor.actorIndex = index
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
		return
	}
	tools := []EditorTool{EditorToolDig, EditorToolPaint, EditorToolFill}
	if slot := index - len(editShapes); slot < len(tools) {
		g.setEditorTool(tools[slot])
		return
	}
	// 9 is not a shape but where every shape aims, so it flips rather than picks.
	g.editor.aimInFront = !g.editor.aimInFront
	g.cancelEditorSelection()
}

// editShapes is the number row's half of EditShape, in the order the keys are
// under a hand.
var editShapes = []EditShape{EditShapeSingle, EditShapeLine, EditShapePlane, EditShapeCube, EditShapeStamp3x3x3}

// editorTabAction is Tab: the one toggle each mode has. Which toggle it is is
// on screen, so the key is "flip this mode's switch" rather than three keys to
// remember.
func (g *Game) editorTabAction() {
	if g.editor.mode == ModeWeapons {
		// A weapon brings two pictures with it - the gun and a box of what it
		// fires - and they sit next to each other in the palette, so the toggle
		// is the slot beside this one. Snap is set from any of the other three
		// placement modes and is shared with them.
		g.selectProp(g.paletteIndex() ^ 1)
		return
	}
	if g.editor.mode.placesProps() {
		g.editor.snapToGrid = !g.editor.snapToGrid
		return
	}
	switch g.editor.mode {
	case ModeBlocks:
		g.editor.addMode = !g.editor.addMode
		g.cancelEditorSelection()
	case ModeActors:
		g.editor.selectedActor = nil
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
	if g.editor.mode.placesProps() {
		g.selectProp(g.paletteIndex() + step)
		return
	}
	switch g.editor.mode {
	case ModeBlocks:
		g.selectBlockSlot((slices.Index(editShapes, g.editor.shapeMethod) + step + len(editShapes)) % len(editShapes))
	case ModeActors:
		if len(g.editor.actors) == 0 {
			return
		}
		g.editor.actorIndex = (g.editor.actorIndex + step + len(g.editor.actors)) % len(g.editor.actors)
	}
}

// selectProp puts palette slot index on the crosshair, wrapping at both ends so
// the wheel never runs out. The ghost is spawned here rather than at click
// time, which is what makes the placement WYSIWYG: what is standing under the
// crosshair is the thing a click leaves behind.
func (g *Game) selectProp(index int) {
	palette := g.palette()
	if len(palette) == 0 {
		g.clearGhost()
		return
	}
	index = (index%len(palette) + len(palette)) % len(palette)
	g.editor.propIndex[g.editor.mode] = index
	kind := palette[index]
	switch {
	case kind.torch:
		g.startTorchPlacement()
	case kind.sprite:
		g.startSpritePlacement()
	case kind.weapon != "":
		g.startPickupPlacement(kind)
	case kind.spriteKind != "":
		g.startSpriteKindPlacement(kind.spriteKind)
	default:
		g.startSceneryPlacement(kind.scenery)
	}
	g.applyGhostFacing()
}

// selectPropByID puts a piece on the crosshair by its folder name, whichever
// folder that is - a model under assets/meshes or a picture under
// assets/sprites. It goes through the palette rather than round it, so the piece
// being placed and the piece a click writes to the map file are always the same
// one.
func (g *Game) selectPropByID(id string) {
	for mode := ModeProps3D; mode <= ModeWeapons; mode++ {
		for index, kind := range g.editor.props[mode] {
			if kind.scenery == id || kind.spriteKind == id {
				g.setEditorMode(mode)
				g.selectProp(index)
				return
			}
		}
	}
}

// selectWeaponPickup is selectPropByID for the guns, which are told apart by two
// things rather than one: which weapon, and whether it is the gun or its ammo.
func (g *Game) selectWeaponPickup(id string, ammo bool) {
	for index, kind := range g.editor.props[ModeWeapons] {
		if kind.weapon == id && kind.ammo == ammo {
			g.setEditorMode(ModeWeapons)
			g.selectProp(index)
			return
		}
	}
}

// selectActorByID puts a character on the actor tool by its folder name, the way
// selectPropByID does for the things that are placed out of a palette. It goes
// through the palette rather than round it, so what the tool says it will place
// and what a click actually places are always the same one.
func (g *Game) selectActorByID(id string) {
	for index, entity := range g.editor.actors {
		if entity.SpriteSheetName == id {
			g.setEditorMode(ModeActors)
			g.editor.actorIndex = index
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

// startPickupPlacement puts a weapon or a box of its ammo on the crosshair. It
// is the pickup itself, built the way a placed one is - see newPickupObject -
// but never registered as one: a ghost the player could walk into and pocket is
// a ghost that disappears the moment they step forward.
func (g *Game) startPickupPlacement(kind propKind) {
	g.clearGhost()
	index, known := g.weaponByID[kind.weapon]
	if !known {
		return
	}
	ghost, sprite := g.newPickupObject(g.weapons[index], kind.ammo, mgl32.Vec3{})
	if ghost == nil {
		g.Toast(kind.label + " has no pickup art")
		return
	}
	sprite.Tint = mgl32.Vec4{1, 1, 1, 0.5}
	g.editor.gameObjectForSpawning = ghost
}

// startSpriteKindPlacement puts a folder's own picture on the crosshair, half
// faded like every other ghost.
func (g *Game) startSpriteKindPlacement(id string) {
	g.clearGhost()
	g.editor.gameObjectForSpawning = g.spawnEntity(mgl32.Vec3{}, g.spriteKindEntity(id, false))
	if g.editor.gameObjectForSpawning != nil {
		g.editor.gameObjectForSpawning.Sprite().Tint = mgl32.Vec4{1, 1, 1, 0.5}
	}
}

// spriteKindEntity is a sprite-kind prop as the map format describes it: the
// folder's own name, its idle frames, and the frame size its sheet is cut at.
// solid is false for the ghost - something you can walk into is not a ghost.
func (g *Game) spriteKindEntity(id string, solid bool) core.MapEntity {
	kind := g.spriteKinds[id]
	if kind == nil {
		return g.decorationEntity(solid)
	}
	return core.MapEntity{
		Name:                kind.Name,
		TextureIndices:      kind.idleFrames(),
		SpriteSheetName:     id,
		SpriteSheetTileSize: kind.Tile,
		CreateCollider:      solid,
		IsTransparent:       !solid,
	}
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
	// A door or a keypad changes which door is nearest which panel, so the
	// pairing is worked out again the moment either one lands.
	if interactableMesh(entity.MeshName) {
		g.bindKeypads()
	}
	// A fresh ghost, because the one that was on the crosshair is now the
	// piece standing on the floor... except for scenery, which spawned a
	// separate solid copy - either way the crosshair gets a new one.
	g.selectProp(g.paletteIndex())
}

// propEntity is the palette's current slot as the map format describes it.
func (g *Game) propEntity() core.MapEntity {
	kind := g.palette()[g.paletteIndex()]
	switch {
	case kind.torch:
		return core.MapEntity{Name: "Torch", MeshName: torchMeshName, Facing: propFacing(g.editor.facing)}
	case kind.sprite:
		return g.decorationEntity(true)
	case kind.weapon != "":
		return core.MapEntity{Name: kind.label, WeaponName: kind.weapon, WeaponAmmo: kind.ammo}
	case kind.spriteKind != "":
		return g.spriteKindEntity(kind.spriteKind, true)
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
	// Same as placing one: removing a keypad has to give its door back, and
	// removing a door has to leave its keypad holding nothing.
	g.bindKeypads()
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
	if g.editor.actorIndex >= len(g.editor.actors) {
		return
	}
	entity := g.editor.actors[g.editor.actorIndex]
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

// mapWithPlacedEntities is the loaded map with its entity list rebuilt from
// what is actually standing in the world right now. Everything is listed fresh,
// so something the editor deleted is gone from the file rather than merely dead
// in memory, and the map the editor started from is not mutated - a failed save
// leaves the running level alone.
//
// A position is a position: the format used to hold a tile index and an offset
// inside the tile, because the geometry was a tile grid, and free placement had
// to be smuggled through the offset. The geometry is voxels now and this is
// simply where the thing stands.
func (g *Game) mapWithPlacedEntities() core.MapLoadResult {
	result := g.mapSource
	// The fill light as the knob left it, so a level keeps what it was lit with.
	result.AmbientColor, result.AmbientStrength = g.ambientColor, g.ambientStrength
	result.Entities = nil
	for _, placed := range g.placed {
		if placed.object != nil && !placed.object.IsAlive() {
			continue
		}
		entity := placed.entity
		if actor := g.actorFor(placed.object); actor != nil && len(actor.patrol) > 1 {
			// The live route, not the one it was spawned with: the path tool
			// edits the actor, and the actor is what the file has to agree with.
			// Point zero is the spawn, which the position already says.
			entity.Patrol = slices.Clone(actor.patrol[1:])
			entity.PatrolTo = nil
		}
		result.Entities = append(result.Entities, core.EntitySpawn{Position: placed.pos, Entity: entity})
	}
	return result
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
	if g.editor.mode.placesProps() {
		placement := "free"
		if g.editor.snapToGrid {
			placement = "snap"
		}
		label := "nothing"
		if palette := g.palette(); len(palette) > 0 {
			label = palette[g.paletteIndex()].label
		}
		line := fmt.Sprintf("%s   %s   facing %s", label, placement, facingNames[g.editor.facing])
		// What the crosshair is on, when it is one of the things that carries
		// writing of its own - so a keypad's code is readable while building
		// rather than only once a panel has been opened on it.
		if status := g.keypadStatus(g.editorHoveredObject()); status != "" {
			line += "   |   " + status
		}
		return line
	}
	switch g.editor.mode {
	case ModeActors:
		if g.editor.actorIndex >= len(g.editor.actors) {
			return "no actors installed"
		}
		kind := g.editor.actors[g.editor.actorIndex].Name
		if actor := g.editor.selectedActor; actor != nil && actor.object.IsAlive() {
			return fmt.Sprintf("%s   editing %s, %d waypoints", kind, actor.Name, len(actor.patrol)-1)
		}
		return kind + "   nothing selected"
	}
	if g.editor.tool != EditorToolNone {
		return "tool: " + g.editor.tool.String()
	}
	line := map[bool]string{true: "ADD", false: "DEL"}[g.editor.addMode] + "   " + g.editor.shapeMethod.String()
	if g.editor.aimInFront {
		line += "   in front of you"
	}
	return line
}

// editorKeyLine is the mode's own keys. Only the mode's own: a legend that
// listed all three modes' keys at once would be the menu this is here to avoid.
func (g *Game) editorKeyLine() string {
	tab := g.keyLabel(ActionEditorTabAction)
	pick := g.keyLabel(ActionEditorOpenPicker)
	if g.editor.mode == ModeWeapons {
		return fmt.Sprintf("1-9/wheel gun   %s browse   %s gun/ammo   LMB place   RMB delete", pick, tab)
	}
	if g.editor.mode.placesProps() {
		return fmt.Sprintf("1-9/wheel piece   %s browse   %s snap   %s turn   LMB place   RMB delete",
			pick, tab, g.keyLabel(ActionEditorRotate))
	}
	switch g.editor.mode {
	case ModeActors:
		return fmt.Sprintf("1-9 kind   %s browse   LMB place/select/waypoint   RMB undo point   %s deselect", pick, tab)
	}
	return fmt.Sprintf("1-5 shape   6-8 dig/paint/fill   9 aim   %s blocks   %s add/del   LMB build   RMB cancel", pick, tab)
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
	case "props", "props3d", "3dprops":
		return ModeProps3D
	case "props2d", "2dprops", "sprites":
		return ModeProps2D
	case "pickups", "items":
		return ModePickups
	case "weapons":
		return ModeWeapons
	case "actors":
		return ModeActors
	}
	return ModeBlocks
}
