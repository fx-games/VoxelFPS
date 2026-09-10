package main

// The gallery: a build.map written from whatever is installed rather than by
// hand, because what is installed is six hundred folders and counting.
//
//	go run ./tools/mapgen -gallery      # write assets/maps/imported/build.map
//	go run ./tools/mapgen -map imported # and compile it, as usual
//
// It is a level like any other once written - a plain text file the ordinary
// commands, which anybody can edit afterwards. Re-running this overwrites it,
// which is the point: re-import a game and the hall has the new folders in it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// galleryName is the level this writes.
const galleryName = "imported"

// The hall. It is four bays deep, in the order you walk into them, and wide
// enough that the last bay is not in the first one's line of sight.
// The ceiling is six because a voxel is about two metres of room - the station
// runs its corridors two voxels from floor to ceiling and its people are 0.875
// of one - so a hall four voxels of air tall is already a big hall. It was
// fourteen, from when everything imported was drawn at twice its size, and next
// to art at the size its own game drew it that reads as a cathedral with a
// jumble sale in it.
const (
	galleryWidth  = 160
	galleryHeight = 6
	galleryDepth  = 272
)

// meshDir and spriteDir are the two folders of folders the hall is filled from.
const (
	galleryMeshDir   = "assets/meshes"
	gallerySpriteDir = "assets/sprites"
)

// writeGallery lays the whole thing out and writes the script.
func writeGallery() error {
	props, friendly, hostile, err := galleryKinds()
	if err != nil {
		return err
	}
	models, err := galleryModels()
	if err != nil {
		return err
	}

	var out []string
	line := func(format string, args ...any) {
		out = append(out, fmt.Sprintf(format, args...))
	}

	line("# The imported gallery: one of everything the two DOS-game importers")
	line("# brought in, in a hall you can walk the length of. It is the check that an")
	line("# import worked, and the range the hostile AI is tried out in.")
	line("#")
	line("# Written by `go run ./tools/mapgen -gallery` out of whatever is installed,")
	line("# then compiled by `go run ./tools/mapgen -map %s` like any other level.", galleryName)
	line("#")
	line("# Four bays, in the order you meet them: the scenery, standing in rows with")
	line("# room to walk between; the characters that do not fight; everything that")
	line("# does; and the game's own 3D models at the far end. F1 -> Cheats -> Give")
	line("# All Weapons before going past the second bay.")
	line("")
	line("size %d %d %d", galleryWidth, galleryHeight, galleryDepth)
	// Bright: the point of the hall is to look at what is standing in it, and
	// a level lit for atmosphere is a level where half the exhibits are a
	// silhouette. The F1 menu's Rendering page turns it down again.
	line("ambient 0.75 0.75 0.82 0.85")
	line("ceiling 26 26 34")
	line("floor 18 18 24")
	line("fog 150 420")
	line("")
	line("block hall 21 22 20")
	line("fill hall 0 0 0 %d %d %d", galleryWidth-1, galleryHeight-1, galleryDepth-1)
	line("clear 2 1 2 %d %d %d", galleryWidth-3, galleryHeight-3, galleryDepth-3)
	line("")
	line("spawn 8.5 1.5 8.5 dir=0,1")
	line("")

	line("# --- light ------------------------------------------------------------------")
	for z := 12; z < galleryDepth-8; z += 20 {
		for x := 12; x < galleryWidth-8; x += 20 {
			line("light %d.5 %d %d.5 color=1,0.92,0.80 power=6 baked", x, galleryHeight-3, z)
		}
	}
	line("")

	z := 14.0
	bay := func(title string, ids []string, columns int, step float64, place func(id string, x, z float64) string) {
		line("# --- %s", title)
		for i, id := range ids {
			out = append(out, place(id,
				6.5+float64(i%columns)*step,
				z+float64(i/columns)*step))
		}
		rows := (len(ids) + columns - 1) / columns
		z += float64(rows)*step + 10
		line("")
	}

	// galleryFloor is the top of the slab the hall is cut out of - the fill runs
	// from y=0 and the clear from y=1, so the first bit of air is at one. A
	// sprite is anchored at its feet and a model at its base, so both stand flat
	// on the floor at exactly that. Anything above it hovers, which is what 1.2
	// was doing to every exhibit in the place.
	const galleryFloor = 1.0

	sprite := func(id string, x, z float64) string {
		return fmt.Sprintf("entity - at=%g,%g,%g sprite=%s", x, galleryFloor, z, id)
	}
	// The exhibits are walked through rather than round: five hundred solid
	// pieces in rows is a wall across the hall, and the point of the bay is to
	// see them, not to be stopped by them.
	//
	// Walked through, not absent: passthrough keeps them on the physics grid, so
	// the crosshair finds them and every one of them is outlined and named by
	// its own box. collider=false left them off the grid entirely, which is why
	// nothing in the first or the last bay could be looked at.
	exhibit := func(id string, x, z float64) string {
		return fmt.Sprintf("entity - at=%g,%g,%g sprite=%s passthrough=true", x, galleryFloor, z, id)
	}
	mesh := func(id string, x, z float64) string {
		return fmt.Sprintf("entity - at=%g,%g,%g mesh=%s passthrough=true", x, galleryFloor, z, id)
	}

	bay("bay one: the scenery, every frame it has ------------------------------", props, 34, 2, exhibit)
	bay("bay two: the characters that do not fight -----------------------------", friendly, 12, 6, sprite)
	bay("bay three: everything that shoots back --------------------------------", hostile, 12, 11, sprite)
	bay("bay four: the game's own 3D models ------------------------------------", models, 12, 13, mesh)

	if int(z) > galleryDepth {
		return fmt.Errorf("the hall needs to be %d deep and is %d", int(z), galleryDepth)
	}
	path := filepath.Join(mapDir, galleryName, scriptName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d scenery, %d friendly, %d hostile, %d models, %d deep of %d\n",
		path, len(props), len(friendly), len(hostile), len(models), int(z), galleryDepth)
	return nil
}

// galleryKinds sorts every installed sprite folder into the three bays it can
// go in: scenery, a character that does not fight, and one that does.
func galleryKinds() (props, friendly, hostile []string, err error) {
	entries, err := os.ReadDir(gallerySpriteDir)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(gallerySpriteDir, entry.Name(), "sprite.json"))
		if err != nil {
			continue
		}
		var kind struct {
			Actor   bool `json:"actor"`
			Hostile bool `json:"hostile"`
		}
		if err := json.Unmarshal(raw, &kind); err != nil {
			continue
		}
		switch {
		case kind.Hostile:
			hostile = append(hostile, entry.Name())
		case kind.Actor:
			friendly = append(friendly, entry.Name())
		default:
			props = append(props, entry.Name())
		}
	}
	sort.Strings(props)
	sort.Strings(friendly)
	sort.Strings(hostile)
	return props, friendly, hostile, nil
}

// galleryModels is the imported 3D models, which are the only mesh folders the
// hall stands out: the engine's own doors and ladders belong in a level that
// uses them, not in a row of exhibits.
func galleryModels() ([]string, error) {
	entries, err := os.ReadDir(galleryMeshDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "ss3d_") {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
