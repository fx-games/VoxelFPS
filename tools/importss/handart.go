package main

// Where each frame of a weapon sits in the view.
//
// System Shock's hand art carries no hotspot - every frame's anchor box in the
// file is zeroed - and the frames of one weapon are not the same size, so
// nothing in the data says how they line up. The game did not read it from the
// data either: it has a table per weapon, and draws the bitmap's top left
// corner at the offset the table gives. Without them a swing is four pictures
// of a blade at four sizes with nothing holding the hilt still.
//
// The numbers below are that table, out of the source release's hand.c
// (github.com/Interrupt/systemshock, GameSrc/hand.c: pistol_hand_info,
// energy_hand_info and hand2hand_info). The sway that follows the mouse is
// dropped - this engine holds a weapon still - and so is the walking bob, which
// it does itself in weaponBob. What is kept is the part that is about the art:
// which bitmap each step of the animation shows, and where it goes.

import (
	"image"
	"image/draw"
)

// handFrame is one step of a weapon's animation: which of its bitmaps to show
// and where the top left corner of it goes.
type handFrame struct {
	bitmap int
	x, y   int
}

// projectileHands are the seven guns that fire something solid, in the order
// the property table lists them. Two frames each: the weapon at rest and the
// shot going off.
var projectileHands = [][]handFrame{
	{{0, 0, 25}, {1, -4, 19}},  // pistol
	{{0, 0, 25}, {1, -3, 20}},  // dartgun
	{{0, 0, 24}, {1, -2, 18}},  // magnum
	{{0, 0, 23}, {1, -2, 16}},  // assault rifle
	{{0, 0, 23}, {1, -1, 21}},  // riot gun
	{{0, 0, 21}, {1, -15, 11}}, // flechette
	{{0, 0, 23}, {1, -2, 17}},  // skorpion
}

// energyHands are the nine slots after them. Two of the nine are the
// hand-to-hand weapons, which the game fills with zeroes here and describes in
// meleeHands instead - the same two holes are kept so the index still lines up
// with the gun.
var energyHands = [][]handFrame{
	{{0, 0, 22}, {1, -3, 19}},  // magpulse
	{{0, 0, 24}, {1, -4, 16}},  // rail gun
	nil,                        // the pipe, in meleeHands
	nil,                        // the rapier, in meleeHands
	{{0, 0, 24}, {1, 0, 24}},   // sparq beam
	{{0, 0, 24}, {1, -12, 18}}, // blaster
	{{0, 0, 21}, {1, -10, 15}}, // ion rifle
	{{0, 0, 24}, {1, 0, 19}},   // stungun
	{{0, 0, 21}, {1, -16, 10}}, // plasma rifle
}

// meleeHands are the two swings. Five steps out of four bitmaps, and not in
// file order: the rapier starts on its second picture, goes back to the first
// and then out through the other two, which is a blade coming up into view
// before it is swung.
var meleeHands = [][]handFrame{
	{{0, 10, -2}, {1, 0, 7}, {2, -14, 21}, {3, -26, 26}, {1, 3, 7}},      // pr-24 pipe
	{{1, -7, -1}, {0, 10, -20}, {2, -30, 4}, {3, -49, 20}, {1, -16, 19}}, // laser epee
}

// firstEnergyGun is where energyHands starts among the sixteen.
const firstEnergyGun = 7

// handFramesFor is the animation for one gun, by its place among the sixteen.
func handFramesFor(gun int) []handFrame {
	switch {
	case gun < 0:
		return nil
	case gun < firstEnergyGun:
		if gun < len(projectileHands) {
			return projectileHands[gun]
		}
	case gun == 9 || gun == 10:
		return meleeHands[gun-9]
	default:
		if index := gun - firstEnergyGun; index < len(energyHands) {
			return energyHands[index]
		}
	}
	return nil
}

// The view the offsets are measured in. The game hangs hand art from a line
// near the bottom of a two hundred pixel view and lets whatever falls past the
// bottom edge be clipped there - which is most of a gun, and half of a swing.
const (
	handArtBaseY = 168 // FULLSCREEN_HANDART_Y
	handArtViewH = 200 // FULL_VIEW_HEIGHT
)

// layOutHandArt places a weapon's frames on one canvas, each at its own offset,
// with the bottom of the canvas on the bottom of the view. Every frame comes
// out the same size, which is what makes them line up once the engine anchors
// them all the same way - and the engine anchors a weapon's frames by their
// bottom edge, on the same line at 168 this game hangs them from.
//
// Anything below the bottom of the view is cut off rather than kept, because
// that is what the game does with it: the art is drawn into a 320 by 200 view
// and clipped at its edge. Keeping it would push the whole weapon up the screen
// by however far the longest frame overhangs, which is a gun floating in the
// middle of the view with a gap under it.
func layOutHandArt(frames []image.Image, steps []handFrame) []image.Image {
	if len(steps) == 0 {
		return frames
	}
	minX, minY, maxX := 1<<30, 1<<30, -(1 << 30)
	for _, step := range steps {
		if step.bitmap >= len(frames) {
			return frames
		}
		minX = min(minX, step.x)
		minY = min(minY, step.y)
		maxX = max(maxX, step.x+frames[step.bitmap].Bounds().Dx())
	}
	width := maxX - minX
	height := handArtViewH - (handArtBaseY + minY)
	if width < 1 || height < 1 {
		return frames
	}

	out := make([]image.Image, 0, len(steps))
	for _, step := range steps {
		source := frames[step.bitmap]
		canvas := image.NewRGBA(image.Rect(0, 0, width, height))
		at := image.Rect(step.x-minX, step.y-minY,
			step.x-minX+source.Bounds().Dx(), step.y-minY+source.Bounds().Dy())
		// draw.Draw clips to the canvas, which is the overhang going over the
		// bottom edge exactly as it does in the game.
		draw.Draw(canvas, at, source, source.Bounds().Min, draw.Src)
		out = append(out, canvas)
	}
	return out
}
