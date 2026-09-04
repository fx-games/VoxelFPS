package core

import (
    "testing"

    "github.com/go-gl/mathgl/mgl32"
)

// Yaw and Pitch are what the map file stores and Forward is what the renderer uses,
// so the two have to be the same rotation read two ways.
func TestLookAtAndForwardAreTheSameRotation(t *testing.T) {
    targets := []mgl32.Vec3{
        {0, 0, -1},   // straight ahead, the zero of the convention
        {1, 0, 0},    // +X, yaw 90
        {-1, 0, 0},   // -X
        {0, -1, 0},   // straight down, pitch 90
        {3, -2, -5},  // the shape a lamp over a walkway actually has
        {-4, -1, 2},  // behind and to the left
    }
    origin := mgl32.Vec3{10, 4, 13}

    for _, offset := range targets {
        light := MapLight{Position: origin}.LookAt(origin.Add(offset))
        want := offset.Normalize()
        if got := light.Forward(); !got.ApproxEqualThreshold(want, 1e-3) {
            t.Errorf("LookAt(%v) then Forward() = %v, want %v (yaw %.1f pitch %.1f)",
                offset, got, want, light.Yaw, light.Pitch)
        }
    }
}

func TestForwardIsTheDocumentedConvention(t *testing.T) {
    forward := MapLight{}.Forward()
    if !forward.ApproxEqualThreshold(mgl32.Vec3{0, 0, -1}, 1e-3) {
        t.Errorf("yaw 0 pitch 0 points %v, want -Z", forward)
    }
    if got := (MapLight{Yaw: 90}).Forward(); !got.ApproxEqualThreshold(mgl32.Vec3{1, 0, 0}, 1e-3) {
        t.Errorf("yaw 90 points %v, want +X", got)
    }
    if got := (MapLight{Pitch: 90}).Forward(); !got.ApproxEqualThreshold(mgl32.Vec3{0, -1, 0}, 1e-3) {
        t.Errorf("pitch 90 points %v, want straight down", got)
    }
}
