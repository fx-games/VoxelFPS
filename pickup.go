package main

import (
	"fmt"
	"log"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// A pickup is a weapon, or a box of its ammo, standing on a shelf: one sprite
// billboard out of the shared atlas, drawn Doom's way - upright, turning to
// face the player and never pitching - and either a radius the player walks
// into or a box the selection beam picks out, depending on the pickup mode.
//
// The picture is the weapon's own Icon, which the migration put in the shared
// store precisely so this can draw a weapon nobody is holding: only the weapon
// in hand has its own atlas loaded.
type Pickup struct {
	weapon *Weapon
	// ammo marks the box of ammo rather than the weapon itself.
	ammo   bool
	object *quads.GameObject
	sprite *quads.SpriteComponent
}

// How a pickup sits, how close you have to walk and how far you can reach.
// Doom picks a thing up when the two radii overlap - the player's 16 and an
// item's 20, which is a bit over half a voxel here. The size caps keep a
// first-person gun frame, which is what a weapon with no pickup art of its own
// falls back on, from standing taller than the shelf it is on and wider than
// the boards it stands between.
const (
	// Doom's own: the player's radius of 16 plus an item's 20.
	pickupRadius = 36 * worldUnitsPerDoomUnit
	// A little under the shelf's shortest gap, and half the space between its
	// uprights, so a weapon and its ammo stand side by side on one board
	// without either poking through the one above.
	pickupMaxHeight = 0.38
	pickupMaxWidth  = 0.45
	// pickupReach is how far the selection beam picks an item out, in
	// intentional mode. An arm's length, not across the room.
	pickupReach = 2.5
)

// How the player comes by what is lying around. Collision based is Doom's own -
// walk over a thing and it is yours - and is what this has always done.
// Intentional makes it a deliberate act: the selection beam has to be on the
// item, which outlines it, and then E takes it.
const (
	pickupCollision = iota
	pickupIntentional
)

var pickupModeNames = []string{"Collision Based", "Intentional"}

// setPickupMode is the F1 menu's half of it.
func (g *Game) setPickupMode(index int) {
	g.pickupModeIndex = index
	g.Toast("Item pickup: " + pickupModeNames[index])
}

// spawnPickup puts one on the floor, or nothing at all when the weapon has no
// picture for it - a weapon whose pickup art the sources did not have.
func (g *Game) spawnPickup(weapon *Weapon, ammo bool, position mgl32.Vec3) {
	name, box := weapon.Data.Icon, weapon.Data.IconBox
	if ammo {
		name, box = weapon.Data.AmmoIcon, weapon.Data.AmmoIconBox
	}
	index, known := g.sharedSpriteIndex[name]
	if name == "" || !known || g.sharedAtlas == nil {
		return
	}
	sprite := quads.NewSpriteFloor(g.sharedAtlas, []uint16{uint16(index)})
	sprite.Size = pickupSize(g, box, index)
	// Doom lights a thing by the room it stands in, and these never move, so
	// the light where they were put is the light they keep.
	sprite.BakedLight = bakedLightIn(g.CubeMap, position)
	sprite.NoShadow = true

	object := quads.NewGameObject(pickupName(weapon, ammo))
	object.Transform().SetPosition(position)
	object.SetComponent(sprite)
	// The shape of it, which is the picture's own quad - see pickupSlab - and
	// it goes on the physics grid like everything else the player can look at.
	// PassThrough is what keeps it out of the way: the selection ray finds it,
	// the collision solver walks straight through it. Doom's items are things
	// you walk over, not things you bump into.
	shape := quads.NewPhysicsComponent(false, pickupSlab)
	shape.PassThrough = true
	object.SetComponent(shape)
	g.physicsSystem.AddObject(shape)
	g.Sprites.Add(sprite)

	pickup := &Pickup{weapon: weapon, ammo: ammo, object: object, sprite: sprite}
	g.pickups = append(g.pickups, pickup)
	g.pickupByObject[object] = pickup
}

// pickupSize is how big the quad is: the box the migration measured off the
// picture's own pixels, in the same pixels-per-unit everything else a weapon
// draws in the world uses, capped on both axes so it fits the shelf it stands
// on. Both caps scale the whole picture - a gun squeezed on one axis only is a
// gun that reads as the wrong gun.
func pickupSize(g *Game, box wep.PickupBox, index int) mgl32.Vec2 {
	width, height, measured := box.Size()
	if !measured {
		// A weapon.json from before the migration measured these, or one
		// written by hand. The packed atlas trims a picture to the same opaque
		// box, so it answers the same thing at the cost of asking it.
		left, top, right, bottom := g.sharedAtlas.GetUVInPx(index)
		width, height = right-left, bottom-top
	}
	size := mgl32.Vec2{
		float32(width) / spritePixelsPerUnit,
		float32(height) / spritePixelsPerUnit,
	}
	if size.Y() > pickupMaxHeight {
		size = size.Mul(pickupMaxHeight / size.Y())
	}
	if size.X() > pickupMaxWidth {
		size = size.Mul(pickupMaxWidth / size.X())
	}
	return size
}

// pickupSlab is the picture's own quad - bottom centred, a unit wide and a unit
// tall before the sprite's size scales it - with the depth this engine gives
// any box it derives from a picture. A picture is flat, and a ray along the
// plane of a flat thing either misses it or divides by zero.
var pickupSlab = voxel.NewAABB(mgl32.Vec3{-0.5, 0, -0.05}, mgl32.Vec3{0.5, 1, 0.05})

// pickupName is what the outline says about it, and so what the object is
// called: the selection names whatever it has picked out, and this is a thing
// in the world like the barrel beside it.
func pickupName(weapon *Weapon, ammo bool) string {
	if ammo {
		return weapon.DisplayName() + " ammo"
	}
	return weapon.DisplayName()
}

// updatePickups hands over whatever the player is standing next to, which is
// only how it works in collision based mode. In intentional mode nothing is
// taken until it is asked for - see takeHoveredPickup.
//
// ponytail: a distance test rather than a collider, which is what Doom does
// too - a pickup is not something you bump into, it is something you walk
// through. A physics body per pickup would be a body the player collides with,
// and there are a couple of hundred of them on the test range.
func (g *Game) updatePickups() {
	if len(g.pickups) == 0 || g.pickupModeIndex != pickupCollision {
		return
	}
	feet := g.playerFeet()
	kept := g.pickups[:0]
	for _, pickup := range g.pickups {
		// Across the floor only. Doom's own check is a radius and a height
		// span, and the span adds nothing here: a rack is shorter than the
		// player, so both its boards are inside the player's own height
		// whichever one a thing is standing on.
		away := pickup.object.Transform().GetPosition().Sub(feet)
		across := mgl32.Vec2{away.X(), away.Z()}
		if across.Len() > pickupRadius || !pickup.object.IsAlive() {
			kept = append(kept, pickup)
			continue
		}
		g.take(pickup)
	}
	g.pickups = kept
}

// hoveredPickup is the item the selection is on, if it is an item and it is
// close enough to reach. The ray does the finding - a pickup is a collider on
// the grid like anything else you can look at - so there is no second beam and
// no second box: what is outlined is what E takes.
func (g *Game) hoveredPickup() *Pickup {
	if g.pickupModeIndex != pickupIntentional {
		return nil
	}
	object := g.hoveredObj()
	if object == nil {
		return nil
	}
	pickup, isPickup := g.pickupByObject[object]
	if !isPickup {
		return nil
	}
	// An arm's length, not across the room. The ray reaches ten.
	if object.Transform().GetPosition().Sub(g.Camera.GetPosition()).Len() > pickupReach {
		return nil
	}
	return pickup
}

// takeHoveredPickup is the E key in intentional mode: what the beam is on
// changes hands.
func (g *Game) takeHoveredPickup() bool {
	taken := g.hoveredPickup()
	if taken == nil {
		return false
	}
	g.take(taken)
	g.pickups = slices.DeleteFunc(g.pickups, func(p *Pickup) bool { return p == taken })
	return true
}

// take hands one over and clears it out of the world: the weapon joins the
// inventory, or the box of ammo goes into the weapon it belongs to.
//
// ponytail: ammo goes to that one weapon's own runtime, because that is where
// an inventory lives here - every weapon carries its own. A single player
// inventory the weapons draw from is the honest shape and it is a change to
// wep.Runtime, not to this.
func (g *Game) take(pickup *Pickup) {
	pickup.object.Kill()
	g.Sprites.Remove(pickup.sprite)
	delete(g.pickupByObject, pickup.object)
	weapon := pickup.weapon
	if !pickup.ammo {
		if weapon.owned {
			return
		}
		weapon.owned = true
		g.Toast("Picked up " + weapon.DisplayName())
		if g.currentWeapon() == nil {
			g.selectWeapon(g.weaponByID[weapon.Data.ID])
		}
		return
	}
	item := weapon.Data.AmmoPickup
	if item == "" {
		return
	}
	weapon.runtime.Inventory.Give(weapon.runtime.Caps, item, weapon.runtime.Caps[item])
	g.Toast(fmt.Sprintf("Picked up %s (%d)", item, weapon.runtime.Inventory[item]))
}

// spawnWeaponRange furnishes the room the map's WeaponRange entity marks with
// racks of shelves and puts the whole weapons folder on them: a weapon and its
// own ammo side by side on a board, two boards to a rack. It is a test fixture
// and it says so - this is where the pickup, the inventory and the reload are
// looked at with the whole folder in one room - and it is meant to read as
// somebody's weapon room rather than as a car boot sale on the floor.
//
// Every other row is left empty. A rack is a tile wide, so a row of them is a
// wall, and filling every row leaves aisles a tenth of a voxel wider than the
// player - a room you cannot walk through is a room where nothing can be picked
// up either way round.
//
// The tiles are walked from the marker and the solid ones skipped, because the
// room the marker sits in has pillars in it and a rack inside a pillar is a
// rack nobody can reach.
func (g *Game) spawnWeaponRange(corner mgl32.Vec3) {
	remaining := g.weapons
	defer func() {
		log.Printf("[weapons] test range at %v: %d of %d weapons racked",
			corner, len(g.weapons)-len(remaining), len(g.weapons))
	}()
	for row := 0; row < weaponRangeRows && len(remaining) > 0; row += 2 {
		for column := 0; column < weaponRangeColumns && len(remaining) > 0; column++ {
			at := corner.Add(mgl32.Vec3{float32(column), 0, float32(row)})
			grid := voxel.PositionToGridInt3(at)
			if g.CubeMap.IsSolidBlockAt(grid.X, grid.Y, grid.Z) {
				continue
			}
			g.spawnScenery(shelvesMeshName, at, true)
			for _, board := range shelfBoards {
				if len(remaining) == 0 {
					break
				}
				weapon := remaining[0]
				remaining = remaining[1:]
				slot := at.Add(mgl32.Vec3{0, board, 0})
				// A weapon the player always has is not something to pick up -
				// the fists and the pistol, which every one of these mods hands
				// out before the first frame. Its ammo still is, so its half of
				// the board is the box of ammo and nothing else.
				if !weapon.Data.Starting {
					g.spawnPickup(weapon, false, slot.Sub(mgl32.Vec3{shelfSlotBeside, 0, 0}))
				}
				g.spawnPickup(weapon, true, slot.Add(mgl32.Vec3{shelfSlotBeside, 0, 0}))
			}
		}
	}
}

// The shelves the range is built out of. The board heights are the top surfaces
// tools/genshelves draws and scenery/shelves/scenery.json collides with; a thing
// stands on one by standing at that height. shelfSlotBeside splits a board in
// two, the weapon on the left of it and its ammo on the right.
const (
	shelvesMeshName = "shelves"
	shelfSlotBeside = 0.25
)

var shelfBoards = []float32{0.05, 0.60}

// How far the range reaches from its marker, in tiles. The room the marker is
// in is fourteen by thirteen and some of that is pillars, which - at two
// weapons to a rack and a rack on every other row - is enough for the folder as
// it stands and short of it the moment somebody migrates another two mods. The
// weapons that do not fit are simply not laid out.
const (
	weaponRangeColumns = 14
	weaponRangeRows    = 13
)
