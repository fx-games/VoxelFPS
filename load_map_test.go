package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads"
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
		// Every model a level asks for by name has to still be where the name
		// says. Nothing at load time complains about one that is not - the piece
		// is quietly absent - so moving the models into category folders could
		// have emptied the gallery without a word.
		for _, spawn := range result.Entities {
			mesh := spawn.Entity.MeshName
			if mesh == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(meshDir, mesh)); err != nil {
				t.Errorf("%s wants the mesh %q, which is not in %s", name, mesh, meshDir)
			}
		}
	}
}

// A new map is a level that loads: its folder, not one block in it, and the look
// of the map it was made from without that map's contents.
// A name already taken, or one that is not a folder name, is refused.
func TestNewMapIsALevelThatLoads(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(workingDir) })

	look := core.MapLoadResult{
		FogEnd:   40,
		Entities: []core.EntitySpawn{{Entity: core.MapEntity{Name: "Barrel"}}},
	}
	if err := createMap("fresh", look); err != nil {
		t.Fatal(err)
	}
	loaded, blocks, err := loadMap("fresh")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FogEnd != 40 || len(loaded.Entities) != 0 {
		t.Errorf("fog %v and %d entities, want the look and none of the contents", loaded.FogEnd, len(loaded.Entities))
	}
	if used := blocks.UsedTextureIndices(); len(used) != 0 {
		t.Errorf("the new level has blocks in it, drawing %v", used)
	}
	for _, name := range []string{"fresh", "", "../out", "a b"} {
		if createMap(name, look) == nil {
			t.Errorf("created a map called %q", name)
		}
	}
}

// A level that names no fill light is lit white at the default strength, not
// black: the knob scales the colour, and scaling black was a knob that only
// ever made a level darker. What the knob is left at is what a save writes.
func TestAmbientLightDefaultsToWhiteAndIsSaved(t *testing.T) {
	colour, strength := levelAmbient(core.MapLoadResult{})
	if colour != (mgl32.Vec3{1, 1, 1}) || strength != quads.DefaultAmbientStrength {
		t.Errorf("unset ambient is %v at %v, want white at the default", colour, strength)
	}
	blue := mgl32.Vec3{0.1, 0.2, 0.6}
	if colour, strength := levelAmbient(core.MapLoadResult{AmbientColor: blue, AmbientStrength: 0.7}); colour != blue || strength != 0.7 {
		t.Errorf("a level's own ambient came back as %v at %v", colour, strength)
	}

	game := &Game{ambientColor: blue, ambientStrength: 0.45}
	saved := game.mapWithPlacedEntities()
	if saved.AmbientColor != blue || saved.AmbientStrength != 0.45 {
		t.Errorf("save writes %v at %v, want the knob's %v at 0.45", saved.AmbientColor, saved.AmbientStrength, blue)
	}
}
