package core

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// Which of the eight views of a thing the observer is looking at. It is worth
// pinning because the migration writes a spawned actor's frames in this order
// and Doom numbers the same eight differently: Doom's rotation is this index
// plus one, and getting that backwards shows a thing's left side to somebody
// standing on its right.
//
// The sprite sits at the origin looking down +X. Its own left is +Y x +X,
// which is -Z, so the observer standing at +Z is looking at its right side.
func TestGetViewDirectionRunsRoundTheThingsOwnLeft(t *testing.T) {
	looking := mgl32.Vec2{1, 0} // down +X, in the (X, Z) plane the sprites use
	at := mgl32.Vec3{}

	for _, c := range []struct {
		observer mgl32.Vec3
		want     int
		side     string
	}{
		{mgl32.Vec3{10, 0, 0}, 0, "in front of it"},
		{mgl32.Vec3{7, 0, -7}, 1, "off its front left"},
		{mgl32.Vec3{0, 0, -10}, 2, "on its left"},
		{mgl32.Vec3{-7, 0, -7}, 3, "off its back left"},
		{mgl32.Vec3{-10, 0, 0}, 4, "behind it"},
		{mgl32.Vec3{-7, 0, 7}, 5, "off its back right"},
		{mgl32.Vec3{0, 0, 10}, 6, "on its right"},
		{mgl32.Vec3{7, 0, 7}, 7, "off its front right"},
	} {
		// Every observer here stands well clear of the thing, so which way they
		// happen to be facing never comes into it.
		if got := GetViewDirection(c.observer, mgl32.Vec3{0, 0, -1}, at, looking); got != c.want {
			t.Errorf("standing %s: view direction %d, want %d", c.side, got, c.want)
		}
	}
}

// A thing standing where the viewer does has no side to be seen from: the
// player's own muzzle flash hangs a hand's breadth from the eye, and the scrap
// of horizontal offset left when the gun points steeply up or down is noise.
// Taken literally it says "you are looking at this from the side", and the
// flash - whose side view is a long thin sliver of art - is drawn as a bright
// bar lying across the screen. Which way the viewer faces answers it instead.
func TestAThingAtTheViewerIsSeenFromBehindNotFromTheSide(t *testing.T) {
	looking := mgl32.Vec2{1, 0} // the flash points away down +X, as the shot does
	eye := mgl32.Vec3{0, 2, 0}

	// Level, the flash is a little way down the line of fire: seen from behind.
	ahead := mgl32.Vec3{0.8, 2, 0}
	if got := GetViewDirection(eye, mgl32.Vec3{1, 0, 0}, ahead, looking); got != 4 {
		t.Errorf("flash out in front: view %d, want 4 - the view from behind it", got)
	}

	// Pointed almost straight down, the same flash is under the eye with a
	// couple of centimetres of horizontal offset left over.
	below := mgl32.Vec3{0.04, 1.2, -0.09}
	if got := GetViewDirection(eye, mgl32.Vec3{0.17, -0.98, 0}, below, looking); got != 4 {
		t.Errorf("flash under the eye: view %d, want 4 - the same view from behind", got)
	}

	// And straight down, where the viewer has no flat facing either.
	if got := GetViewDirection(eye, mgl32.Vec3{0, -1, 0}, below, looking); got != 4 {
		t.Errorf("viewer looking straight down: view %d, want 4", got)
	}
}
