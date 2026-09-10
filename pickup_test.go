package main

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// Every weapon that has a pickup picture has to have the box that was measured
// off it, because that is what the thing is the size of in the world. They are
// written by the migration - see wep.MeasurePickups - and a folder that predates
// it is filled in with `go run ./tools/genweapons -measure`.
func TestEveryPickupPictureHasBeenMeasured(t *testing.T) {
	library, err := wep.Load("assets/weapons")
	if err != nil {
		t.Fatal(err)
	}
	for _, weapon := range library.Weapons {
		for _, picture := range []struct {
			what string
			name string
			box  wep.PickupBox
		}{
			{"icon", weapon.Icon, weapon.IconBox},
			{"ammo_icon", weapon.AmmoIcon, weapon.AmmoIconBox},
		} {
			if picture.name == "" {
				continue
			}
			if _, _, measured := picture.box.Size(); !measured {
				t.Errorf("%s: %s is %s and has no box", weapon.ID, picture.what, picture.name)
			}
		}
	}
}

// The selection picks out the picture, not a box around it. A billboard seen
// from a corner is as wide as its picture; an axis aligned box round it is half
// again as wide, and on a shelf what stands in that extra width is the next item
// along. This is the test rayCast does, against the collider a pickup carries.
func TestTheSelectionOnlyPicksOutThePictureItself(t *testing.T) {
	const halfWidth = 0.25

	pickup := standingPickup(mgl32.Vec3{0, 1, 0}, mgl32.Vec2{halfWidth * 2, 0.4})
	// Looking at it down the diagonal, which is where an axis aligned box is at
	// its widest and a picture is not.
	camera := core.NewFPSCamera(mgl32.Vec3{2, 1.2, 2}, 800, 600, 1)
	camera.SetForward2D(mgl32.Vec3{-1, 0, -1}.Normalize())
	origin := camera.GetPosition()
	// Across the view rather than along an axis: the picture turns to face the
	// camera, so this is the direction it has width in.
	across := mgl32.Vec3{1, 0, -1}.Normalize()

	at := func(offset float32) (float64, bool) {
		target := mgl32.Vec3{0, 1.2, 0}.Add(across.Mul(offset))
		return voxel.IntersectRayOBB(origin, target.Sub(origin).Normalize(),
			pickup.object.Physics().OBB(camera))
	}

	away := float64(origin.Sub(mgl32.Vec3{0, 1.2, 0}).Len())
	if distance, hit := at(0); !hit {
		t.Errorf("the selection missed the middle of the picture")
	} else if math.Abs(distance-away) > 0.1 {
		t.Errorf("hit at %v, and the picture is %v away", distance, away)
	}
	if _, hit := at(halfWidth * 0.8); !hit {
		t.Errorf("the selection missed the picture well inside its own edge")
	}
	// Beyond the picture, but inside the box an axis aligned test would use: the
	// diagonal of a square that wide reaches to halfWidth * sqrt(2).
	if _, hit := at(halfWidth * 1.3); hit {
		t.Errorf("the selection picked out a picture the crosshair is beside")
	}
}

// standingPickup is one of these away from the game: a sprite of a given size
// standing at a position, which is all the beam test looks at.
func standingPickup(position mgl32.Vec3, size mgl32.Vec2) *Pickup {
	sprite := quads.NewSpriteFloor(nil, []uint16{0})
	sprite.Size = size
	object := quads.NewGameObject("Pickup")
	object.Transform().SetPosition(position)
	object.SetComponent(sprite)
	object.SetComponent(quads.NewPhysicsComponent(false, pickupPickBox(size)))
	return &Pickup{object: object, sprite: sprite}
}

// The box a thing is picked out by is the size the thing is drawn - once, not
// twice. Every box this engine takes off a picture is scaled by the sprite's own
// size the moment it is measured, and the transform a ray tests it against used
// to scale it by that size again: a kit drawn a third of a unit wide was picked
// out, and outlined, a ninth of a unit wide.
func TestThePickingBoxIsTheSizeThePictureIsDrawn(t *testing.T) {
	const width, height = 0.3125, 0.10416667

	pickup := standingPickup(mgl32.Vec3{0, 1, 0}, mgl32.Vec2{width, height})
	camera := core.NewFPSCamera(mgl32.Vec3{0, 1.05, 2}, 800, 600, 1)
	camera.SetForward2D(mgl32.Vec3{0, 0, -1})
	origin := camera.GetPosition()

	at := func(across, up float32) bool {
		target := mgl32.Vec3{across, 1 + up, 0}
		_, hit := voxel.IntersectRayOBB(origin, target.Sub(origin).Normalize(),
			pickup.object.Physics().OBB(camera))
		return hit
	}

	if !at(width*0.45, height*0.9) {
		t.Errorf("the selection missed the far corner of a picture %v by %v", width, height)
	}
	if at(width*0.6, height*0.5) {
		t.Errorf("the selection picked out a picture the crosshair is beside")
	}
	if at(0, height*1.3) {
		t.Errorf("the selection picked out a picture the crosshair is above")
	}
}

// An item's collider is not as wide as its picture. An axe is drawn wide and
// flat, and a collider that wide reaches into the uprights of the shelf it
// stands on - the solver pushes it out of them, and on the way out it slips off
// the board and falls through the floor. Half a tile is what an item has to
// itself; see the shelf's own colliders in assets/meshes/shelves/scenery.json.
func TestAnItemsColliderFitsOnTheShelfItStandsOn(t *testing.T) {
	axe := pickupCollider(mgl32.Vec2{pickupMaxWidth, 0.1})
	if width := axe.Max().X() - axe.Min().X(); width > 0.5 {
		t.Errorf("a wide picture got a %v wide collider, and its half of a board is 0.5", width)
	}
	if height := axe.Max().Y() - axe.Min().Y(); height != 0.1 {
		t.Errorf("the collider is %v tall and the picture is 0.1", height)
	}
	// A picture smaller than the footprint is not given a box bigger than
	// itself: a single shell standing next to a rifle is a small thing.
	shell := pickupCollider(mgl32.Vec2{0.08, 0.08})
	if width := shell.Max().X() - shell.Min().X(); width > 0.08 {
		t.Errorf("a small picture got a %v wide collider", width)
	}
}
