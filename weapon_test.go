package main

import (
	"math"
	"testing"

	"github.com/memmaker/goray/wep"
)

// A vanilla weapon frame's own offset is (width/2 - 160, height - 168). Every
// weapon sprite in every one of the migrated mods is authored against that: it
// is what puts a gun in the middle of the screen with its bottom edge on the
// screen's bottom edge, and a mod's recoil frame is a few pixels off it. If
// this stops holding, every gun in the folder is drawn somewhere it should not
// be, which is exactly the state this replaced.
func TestAVanillaWeaponFrameFillsTheBottomOfTheScreen(t *testing.T) {
	const viewWidth, viewHeight = 640, 480

	for _, frame := range []struct{ width, height int }{
		{256, 64},  // Brutal Doom's purist chaingun
		{320, 168}, // its minigun, on the full canvas
		{128, 128},
		{59, 55}, // and its super shotgun, on a small one
	} {
		hotspotX, hotspotY := frame.width/2-160, frame.height-168
		scale := psprScale(viewHeight)
		corner := psprCorner(viewWidth, viewHeight, 0, wep.WeaponTop, hotspotX, hotspotY, scale)

		bottom := corner.Y() + float32(frame.height)*scale.Y()
		if math.Abs(float64(bottom-viewHeight)) > 0.5 {
			t.Errorf("%dx%d frame ends at y=%.1f, want the bottom of the screen at %d",
				frame.width, frame.height, bottom, viewHeight)
		}

		// Half a source pixel of slack: an odd-width frame's offset is
		// width/2 rounded down, so it cannot be exactly centred and Doom's
		// own sprites are not.
		middle := corner.X() + float32(frame.width)/2*scale.X()
		if math.Abs(float64(middle-viewWidth/2)) > float64(scale.X())/2 {
			t.Errorf("%dx%d frame is centred on x=%.1f, want the middle of the screen at %d",
				frame.width, frame.height, middle, viewWidth/2)
		}
	}
}

// The 320x200 space is scaled by the view's height alone, so the same gun is
// the same size on a wide screen and a narrow one - a wider window shows more
// room either side of it rather than a wider gun.
func TestTheWeaponIsTheSameSizeOnAWiderScreen(t *testing.T) {
	narrow := psprCorner(640, 480, 0, wep.WeaponTop, -32, -104, psprScale(480))
	wide := psprCorner(1920, 480, 0, wep.WeaponTop, -32, -104, psprScale(480))

	if narrow.Y() != wide.Y() {
		t.Errorf("height moved from %.1f to %.1f on a wider screen", narrow.Y(), wide.Y())
	}
	if got, want := wide.X()-960, narrow.X()-320; math.Abs(float64(got-want)) > 0.001 {
		t.Errorf("offset from the middle went from %.1f to %.1f", want, got)
	}
}

// A frame that writes offset() moves the gun and it stays moved: that is how a
// recoil the script never mentions again still ends, at A_WeaponReady.
func TestOffsetsPersistUntilAWeaponReadyPutsThemBack(t *testing.T) {
	machine := wep.StateMachine{
		Steps: []wep.Step{
			{Sprite: "GUNA0", Tics: 1, OffsetY: 40},
			{Sprite: "GUNA0", Tics: 1},
			{Sprite: "GUNA0", Tics: 1, Ops: []wep.Op{{Op: wep.OpWeaponReady}}},
		},
		Labels: map[string]int{"fire": 0},
	}
	cursor := wep.NewCursor(&machine, "Fire")
	runtime := &wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}, Vars: map[string]float64{}}

	if _, y := cursor.Offsets(); y != wep.WeaponTop {
		t.Fatalf("a weapon starts held at %v, not %v", wep.WeaponTop, y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto the offset frame
	if _, y := cursor.Offsets(); y != 40 {
		t.Errorf("offset(0, 40) put the gun at %v, want 40", y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto a frame that writes nothing
	if _, y := cursor.Offsets(); y != 40 {
		t.Errorf("the next frame moved the gun to %v; a zero offset leaves it alone", y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto A_WeaponReady
	if _, y := cursor.Offsets(); y != wep.WeaponTop {
		t.Errorf("A_WeaponReady left the gun at %v, want it back at %v", y, wep.WeaponTop)
	}
}

// Doom's weapon sway: a full circle across for two dips down, sixteen units at
// its widest, never above where the gun is held, and nothing at all when the
// player stands still. Every one of those is something a player notices the
// moment it is wrong - a gun that rises above where it rests reads as
// floating, and one that sways while standing still reads as drunk.
func TestTheWeaponSwaysTheWayDoomsDoes(t *testing.T) {
	const running = 16.0 // Doom units a tic, which is a run

	if x, y := weaponBobAt(0, 17); x != 0 || y != 0 {
		t.Errorf("standing still swayed to (%v, %v)", x, y)
	}

	// A whole cycle, sampled finely enough to catch the turns.
	const steps = 640
	xs := make([]float64, steps+1)
	ys := make([]float64, steps+1)
	for i := range xs {
		tic := weaponBobCycleTics * float64(i) / steps
		xs[i], ys[i] = weaponBobAt(running, tic)
		if ys[i] < 0 {
			t.Fatalf("tic %.1f swung the gun %v above where it rests", tic, -ys[i])
		}
		if math.Abs(xs[i]) > maxWeaponBob+0.001 || ys[i] > maxWeaponBob+0.001 {
			t.Fatalf("tic %.1f swung to (%v, %v), past the %v cap", tic, xs[i], ys[i], maxWeaponBob)
		}
	}

	if got := peaks(ys); got != 2 {
		t.Errorf("the gun reached the bottom of a dip %d times in one cycle, want 2", got)
	}
	// One sweep across, the long way: right at the start of the cycle, left at
	// the middle of it.
	for _, at := range []struct {
		tic  float64
		want float64
	}{{0, maxWeaponBob}, {weaponBobCycleTics / 2, -maxWeaponBob}} {
		if x, _ := weaponBobAt(running, at.tic); math.Abs(x-at.want) > 0.001 {
			t.Errorf("tic %v is %v across, want %v", at.tic, x, at.want)
		}
	}

	// And the cycle really is one: it comes back to where it started.
	if math.Abs(xs[0]-xs[steps]) > 0.001 || math.Abs(ys[0]-ys[steps]) > 0.001 {
		t.Errorf("after one cycle the gun is at (%v, %v), not (%v, %v)",
			xs[steps], ys[steps], xs[0], ys[0])
	}
}

func peaks(values []float64) int {
	count := 0
	for i := 1; i < len(values)-1; i++ {
		if values[i] > values[i-1] && values[i] >= values[i+1] {
			count++
		}
	}
	return count
}
