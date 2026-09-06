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
		if got := GetViewDirection(c.observer, at, looking); got != c.want {
			t.Errorf("standing %s: view direction %d, want %d", c.side, got, c.want)
		}
	}
}
