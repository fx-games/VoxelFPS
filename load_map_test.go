package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The shipped map's two lights used to be Go literals in the GlApplication
// constructor. They are Light entities in raymap.ldtk now, which means a typo in the
// loader or a stray save from the editor can drop them and the map just goes dark
// with nothing to say so. This is what says so.
func TestLDtkMapCarriesItsLights(t *testing.T) {
	result := loadLDTK("assets/raymap.ldtk")

	if len(result.Lights) < 2 {
		t.Fatalf("raymap.ldtk parsed to %d lights, want the two it is authored with", len(result.Lights))
	}

	for _, light := range result.Lights {
		if light.Intensity <= 0 {
			t.Errorf("light at %v has intensity %.2f, so it contributes nothing", light.Position, light.Intensity)
		}
		if light.Color == (mgl32.Vec3{}) {
			t.Errorf("light at %v is black", light.Position)
		}
		if light.Reach <= 0 || light.Angle <= 0 {
			t.Errorf("light at %v has reach %.1f angle %.1f, both need to be positive",
				light.Position, light.Reach, light.Angle)
		}
		// Inside the room, and off the floor: a light buried in the floor voxels
		// lights nothing and its shadow map is a wall of near plane.
		if light.Position.Y() < 1 {
			t.Errorf("light at %v is at or below the floor", light.Position)
		}
		x, z := int(light.Position.X()), int(light.Position.Z())
		if x < 0 || z < 0 || z*result.MapDimensions[0]+x >= len(result.MapData) {
			t.Errorf("light at %v is outside the map", light.Position)
		}
		// Tilted down at least a little, so the map shows floor shadows. Two
		// perfectly horizontal lights are what made finding #22 so hard to read.
		if light.Pitch <= 0 {
			t.Errorf("light at %v has pitch %.1f: horizontal lights cast no floor shadow worth looking at",
				light.Position, light.Pitch)
		}
	}
}
