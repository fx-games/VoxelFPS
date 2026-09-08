package core

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// identity rotation looks down -Z, so everything in front of the camera has a
// negative Z, and the frustum spans 0.15..100 units along it.
func testCam() *PerspectiveTransform {
	return NewDefaultPerspectiveTransform(800, 600)
}

func TestPointFrustum(t *testing.T) {
	c := testCam()
	cases := []struct {
		name string
		p    mgl32.Vec3
		want bool
	}{
		{"straight ahead", mgl32.Vec3{0, 0, -10}, true},
		{"behind the camera", mgl32.Vec3{0, 0, 10}, false},
		{"in front of the near plane", mgl32.Vec3{0, 0, -0.02}, false},
		{"past the far plane", mgl32.Vec3{0, 0, -200}, false},
		{"far off to the side", mgl32.Vec3{100, 0, -10}, false},
	}
	for _, tc := range cases {
		if got := c.IsInViewFrustum(tc.p); got != tc.want {
			t.Errorf("%s: IsInViewFrustum(%v) = %v, want %v", tc.name, tc.p, got, tc.want)
		}
	}
}

// The regression this replaces: the old test asked whether any corner of the box
// was inside the frustum, so a box enclosing the camera - which is what a voxel
// chunk right in front of you looks like - was culled.
func TestCubeStraddlingFrustumIsVisible(t *testing.T) {
	c := testCam()
	min := mgl32.Vec3{-50, -50, -50}
	max := mgl32.Vec3{50, 50, 50}
	if !c.IsCubeInViewFrustum(min, max) {
		t.Error("a box enclosing the camera must be visible, no corner of it is inside the frustum")
	}
}

func TestCubeFrustum(t *testing.T) {
	c := testCam()
	cases := []struct {
		name     string
		min, max mgl32.Vec3
		want     bool
	}{
		{"small box ahead", mgl32.Vec3{-1, -1, -11}, mgl32.Vec3{1, 1, -9}, true},
		{"small box behind", mgl32.Vec3{-1, -1, 9}, mgl32.Vec3{1, 1, 11}, false},
		{"box beyond the far plane", mgl32.Vec3{-1, -1, -210}, mgl32.Vec3{1, 1, -190}, false},
		{"box far to the right", mgl32.Vec3{500, -1, -11}, mgl32.Vec3{502, 1, -9}, false},
		// clipped by the right plane but still partly inside
		{"box crossing the right plane", mgl32.Vec3{0, -1, -11}, mgl32.Vec3{500, 1, -9}, true},
	}
	for _, tc := range cases {
		if got := c.IsCubeInViewFrustum(tc.min, tc.max); got != tc.want {
			t.Errorf("%s: IsCubeInViewFrustum(%v, %v) = %v, want %v", tc.name, tc.min, tc.max, got, tc.want)
		}
	}
}

func TestSphereFrustum(t *testing.T) {
	c := testCam()
	cases := []struct {
		name   string
		center mgl32.Vec3
		radius float32
		want   bool
	}{
		{"ahead", mgl32.Vec3{0, 0, -10}, 1, true},
		{"behind", mgl32.Vec3{0, 0, 10}, 1, false},
		// just outside the near plane, but big enough to reach across it
		{"straddling the near plane", mgl32.Vec3{0, 0, 1}, 2, true},
		{"off to the side", mgl32.Vec3{100, 0, -10}, 1, false},
	}
	for _, tc := range cases {
		if got := c.IsSphereInViewFrustum(tc.center, tc.radius); got != tc.want {
			t.Errorf("%s: IsSphereInViewFrustum(%v, %v) = %v, want %v", tc.name, tc.center, tc.radius, got, tc.want)
		}
	}
}
