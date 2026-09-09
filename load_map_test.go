package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The shipped map's two lights used to be Go literals in the GlApplication
// constructor. They are Light entries in its map file now, which means a typo in
// the loader or a stray save from the editor can drop them and the map just goes
// dark with nothing to say so. This is what says so.
func TestJSONMapCarriesItsLights(t *testing.T) {
	result, err := loadJSONMap(mapPathFor(defaultMapName))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Lights) < 2 {
		t.Fatalf("raymap.json parsed to %d lights, want the two it is authored with", len(result.Lights))
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
		// Tilted down at least a little, so the map shows floor shadows. Two
		// perfectly horizontal lights are what made finding #22 so hard to read.
		if light.Pitch <= 0 {
			t.Errorf("light at %v has pitch %.1f: horizontal lights cast no floor shadow worth looking at",
				light.Position, light.Pitch)
		}
	}
}

// The file watcher hands loadJSONMap whatever is on disk at the moment it was
// saved, which can be a truncated or malformed file. It has to come back as an
// error - a panic here kills the running engine and the session with it.
func TestLoadJSONMapReportsBadFilesInsteadOfPanicking(t *testing.T) {
	dir := t.TempDir()

	truncated := filepath.Join(dir, "truncated.json")
	full, err := os.ReadFile(mapPathFor(defaultMapName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(truncated, full[:len(full)/2], 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{truncated, filepath.Join(dir, "missing.json")} {
		if _, err := loadJSONMap(name); err == nil {
			t.Errorf("loadJSONMap(%s) returned no error", name)
		}
	}
}

// The Load Map menu and the -map flag both ask this what levels exist, and
// nothing else knows: a level is a folder with a map.json in it. A folder that
// stops being listed is a level that quietly vanishes from the menu.
func TestMapNamesListsEveryShippedLevel(t *testing.T) {
	names := mapNames()
	if !slices.Contains(names, defaultMapName) {
		t.Fatalf("levels are %v, want the default map %q among them", names, defaultMapName)
	}
	for _, name := range names {
		result, blocks, err := loadMap(name)
		if err != nil {
			t.Errorf("%s is listed but will not load: %s", name, err)
			continue
		}
		// A level whose blocks did not come with it is an empty box, and the
		// player falls out of the bottom of it.
		if bounds := blocks.Bounds(); bounds.Y <= 1 {
			t.Errorf("%s has bounds %v, so it has no room to stand in", name, bounds)
		}
		if result.PlayerSpawn == (mgl32.Vec3{}) {
			t.Errorf("%s spawns the player at the origin, which is inside the floor", name)
		}
	}
}
