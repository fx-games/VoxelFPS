package main

// The bolt of light an energy weapon leaves between its muzzle and whatever it
// hit.
//
// System Shock draws one for its three beam weapons - the Sparq, the Blaster
// and the Ion Beam - and it is not in any of the art: `gamerend.c` draws three
// straight turquoise lines from the weapon to the impact, five and blue when
// the weapon is overloaded. Nothing of that is a picture, so nothing of it came
// out of the import; this is the same thing in three dimensions, which is also
// what the migrated railgun has always been missing.

import (
	"image"
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
)

// What a beam looks like.
const (
	// beamLife is how long it hangs there. Long enough to be seen at sixty
	// frames a second and short enough that a burst reads as separate shots.
	beamLife = 0.09
	// beamDrawDistance is how far in front of the eye the whole beam is drawn,
	// and beamWidth is how wide it is there. Both ends of the quad sit on that
	// sphere, so the quad is a rectangle a fixed number of pixels wide rather
	// than a wedge - see spawnBeam for why that is the whole trick.
	//
	// A quarter of a unit is well clear of the 0.05 near plane, which clips a
	// quad that crosses it into a smear, and close enough that a wall the
	// player is standing against is still behind it.
	beamDrawDistance = 0.25
	// The width is that distance times the angle the line should subtend, so
	// it reads the same on every screen: about a hundredth of a radian, which
	// is three or four pixels in a 480-line view. System Shock drew three
	// lines a pixel wide; anything much thicker is a cone of light with a beam
	// somewhere inside it.
	beamWidth = beamDrawDistance * 0.011
	// beamEmissive is how far past white it is drawn, which is what makes the
	// bloom pass halo it. Lower than a plasma bolt's: the beam is drawn added
	// on top of what is behind it as well, and the two together at a bolt's
	// figure came out white rather than turquoise.
	beamEmissive = 1.4
	// beamLightReach and beamLightIntensity are the light that rides along it,
	// so a shot down a dark corridor lights the corridor.
	beamLightIntensity = 6
	beamLightReach     = 7
)

// beamColour is the turquoise System Shock draws with. It is one colour for
// every beam weapon there, so it is one here.
//
// ponytail: one colour for every beam in the game, the railgun's included. A
// weapon that wants its own needs a colour on wep.Op, which nothing in the
// format carries today.
var beamColour = mgl32.Vec4{0.35, 1.0, 0.95, 1}

// beamTexture is the picture a beam is drawn with: a bright core fading to
// nothing at both edges, one column wide. Built once, because it is the same
// for every shot and there is nothing in the asset folders it could sensibly be.
func beamTexture() *glhf.TextureAtlasFixed {
	const height = 16
	picture := image.NewRGBA(image.Rect(0, 0, 1, height))
	for y := 0; y < height; y++ {
		// Distance from the middle of the strip, 0 at the core and 1 at an
		// edge, squared so the core stays solid and only the rim softens.
		offset := (float64(y) + 0.5) / height * 2 * (1 - 0) // 0..2
		distance := offset - 1
		if distance < 0 {
			distance = -distance
		}
		alpha := 1 - distance*distance
		if alpha < 0 {
			alpha = 0
		}
		picture.Set(0, y, color.RGBA{
			R: 255, G: 255, B: 255,
			A: uint8(alpha * 255),
		})
	}
	texture, err := glhf.NewTextureFromImage(picture)
	if err != nil {
		return nil
	}
	return glhf.NewTextureAtlasFixed(texture, 1, height, "beam")
}

// spawnBeam puts one between two points. It is a sprite like everything else -
// the same quad, the same pass - drawn in the mode that stretches it along a
// line instead of turning it to face you; see SpriteModeBeam.
//
// The two points it is given are a barrel a unit from the eye and an impact
// twenty units away, and a quad of one width stretched between those is a
// wedge: the same 0.05 units that is nineteen pixels across at the muzzle is
// under one at the far end, so the beam was a bright blob over the gun and
// nothing at all along its length. Looking down made it worse, because then
// the whole thing is close and the blob is all there is.
//
// So the beam is not drawn where it happens. Both ends are pushed back along
// their own line of sight to the same short distance from the eye, which
// leaves each of them on exactly the pixel it was already on - any point on a
// ray through a pixel projects to that pixel - and makes the quad a rectangle
// at a constant depth. That is a line of constant width from the barrel to the
// impact, whatever the range and whatever the pitch, and it is also what
// System Shock did: gamerend.c draws its three lines in screen space, not in
// the world.
//
// Being in front of everything is part of the bargain. Nothing between the
// player and what they shot can hide their own beam, which is the behaviour
// the flat lines had.
func (g *Game) spawnBeam(from, to mgl32.Vec3) {
	if g.beamAtlas == nil {
		g.beamAtlas = beamTexture()
		if g.beamAtlas == nil {
			return
		}
	}
	from, to = beamOnScreen(g.Camera.GetPosition(), from, to)
	sprite := quads.NewSprite(g.beamAtlas, []uint16{0})
	sprite.CameraFollowMode = quads.SpriteModeBeam
	sprite.SpriteOrigin = quads.SpriteOriginCenter
	sprite.BeamTo = to
	sprite.Size = mgl32.Vec2{1, beamWidth}
	sprite.Tint = beamColour
	sprite.Emissive = beamEmissive
	// Added rather than blended: a bolt of light brightens what is behind it,
	// and forty overlapping quads of it should read as one bright line rather
	// than a grey band.
	sprite.Additive = true
	// It never casts: a light that lasts a fifteenth of a second would have
	// every shadow map in the level redrawn for it.
	sprite.NoShadow = true

	object := quads.NewGameObject("Beam")
	object.Transform().SetPosition(from)
	object.SetComponent(sprite)
	object.SetLifeTime(beamLife)

	g.Sprites.AddTransparent(sprite)
	g.AttachLightTo(object, beamColour.Vec3(), beamLightIntensity, beamLightReach)
}

// beamOnScreen slides both ends of a beam along their own line of sight until
// they are the same short distance from the eye. Where each one lands on the
// screen does not change; what changes is that they are now at one depth, so a
// quad of one width between them is one width on the screen too.
//
// An end that is on the eye has no line of sight to slide along and is left
// where it is; the quad it makes is degenerate and draws nothing, which is the
// right answer for a beam that starts and ends in the same place.
func beamOnScreen(eye, from, to mgl32.Vec3) (mgl32.Vec3, mgl32.Vec3) {
	return beamAtDrawDistance(eye, from), beamAtDrawDistance(eye, to)
}

func beamAtDrawDistance(eye, point mgl32.Vec3) mgl32.Vec3 {
	along := point.Sub(eye)
	if along.Len() < 1e-5 {
		return point
	}
	return eye.Add(along.Normalize().Mul(beamDrawDistance))
}

// beamMuzzle is where a beam leaves from: the barrel of the weapon in hand,
// which is at the bottom of the view rather than on the line of fire.
//
// That is not a stylistic choice, and System Shock made the same one. A beam
// that started on the line of fire would run straight away from the eye, and a
// line pointing at you projects to a point however it is drawn - which is what
// the first attempt at this looked like, a bright dot at the muzzle and nothing
// else. There has to be an angle between the two, and the gun is where the
// angle comes from.
//
// It is picked in screen space and then put into the world, rather than being
// an offset in the world, because the barrel is a place on the screen: the
// weapon is drawn into a fixed 320x200 space anchored to the bottom of the
// view, so where its muzzle is has nothing to do with which way the player is
// facing or how wide the window is. Take the point along the ray through that
// pixel and the beam starts on the barrel at any field of view, on any screen.
func (g *Game) beamMuzzle(direction mgl32.Vec3) mgl32.Vec3 {
	origin, along := core.GetRayFromCameraPlane(g.Camera, beamMuzzleNDCX, beamMuzzleNDCY)
	if along.Len() < 1e-5 {
		along = direction
	}
	return origin.Add(along.Mul(beamMuzzleForward))
}

// Where the barrel is, in normalized device coordinates: across the view from
// -1 at the left edge to 1 at the right, and up it from -1 at the bottom.
//
// Off System Shock's own numbers. gamerend.c starts its beam lines at
// (deltax + 19 + boff + beamx, deltay + 12 + boff + 6), where deltax and deltay
// are where the hand art was drawn and boff is -12, -8 or -4 for the Sparq, the
// Blaster and the Ion Beam. beamx cancels the sway out of the x, and deltay is
// 168 plus the weapon's own frame offset, so in the 320x200 view the three
// weapons start theirs at (157, 198), (161, 196) and (165, 197): the middle of
// the screen, on its bottom edge, which is where the gun is.
const (
	beamMuzzleNDCX = 2*161.0/320.0 - 1
	beamMuzzleNDCY = 1 - 2*197.0/200.0
)

// beamEnd is where it stops: what the shot hit, or the end of its reach when it
// hit nothing, so a shot into the sky still draws a beam going out.
func beamEnd(origin, direction mgl32.Vec3, hit RaycastResult) mgl32.Vec3 {
	if hit.HitInfo.Hit {
		return hit.WorldPosition()
	}
	return origin.Add(direction.Mul(beamMissReach))
}

const (
	// How far along the ray through the barrel the beam starts. Far enough
	// forward to be clear of the near plane, which clips a quad crossing it
	// into a smear rather than drawing it. Anywhere along that ray is the same
	// pixel, so this is only about the near plane.
	beamMuzzleForward = 1.2
	// beamMissReach is how far a beam that hit nothing is drawn. Shorter than
	// the ray itself, which reaches a hundred units and would draw a beam
	// halfway across a level for a shot at the sky.
	beamMissReach = 40
)
