// Command mapgen builds a level out of a text file.
//
// A level on disk is two files nothing can hand-edit: the region files hold
// the blocks, map.json is a few hundred lines of generated JSON. Everything that
// wants to make or change a level has therefore had to be the running editor,
// with a mouse in it. This is the other door: build.map, a plain text file of
// one command a line, next to the level it describes.
//
//	go run ./tools/mapgen -gallery              # write the imported gallery's script
//	go run ./tools/mapgen -map station          # build.map -> regions + map.json
//	go run ./tools/mapgen -map station -show 15 # what is solid at height 15
//	go run ./tools/mapgen -map ark -side 56     # what is solid in the x-y plane at z=56
//	go run ./tools/mapgen -help                 # every command, with its arguments
//
// The text is the source and the two files are the output, the same way
// tools/genstairs owns the stairs it draws - until the level is saved from the
// game's editor. That unlinks it: map.json says Edited, the editor is the
// level's source from then on, and a build refuses to overwrite it (-force
// does, and throws the edits away).
//
// Commands, one a line, # to end of line is a comment, blank lines ignored.
// Positional arguments first, then any number of key=value options.
//
//	size W H D                     the level's own size in voxels
//	spawn X Y Z [dir=DX,DZ]        where the player starts, in world units
//	tex NAME PATH [glow]           put a texture from assets/textures in the
//	                               level's own atlas, in the order named, and
//	                               call it NAME. A script with any `tex` in it
//	                               writes map_64x64.png beside the level, so
//	                               nothing it draws depends on the shared one
//	block NAME T [T T]             name a kind of block: wall, floor, ceiling
//	                               textures by `tex` name or atlas slot,
//	                               repeating the last
//	fill NAME x0 y0 z0 x1 y1 z1 [step=DX,DY,DZ times=N]
//	                               make an inclusive box of it solid - N times,
//	                               moved on by step each time, with those two
//	clear x0 y0 z0 x1 y1 z1 [step=DX,DY,DZ times=N]   the same, air
//	cyl NAME cx cz r y0 y1         a solid upright cylinder
//	tube NAME cx cz rOuter rInner y0 y1   the wall of one, hollow inside
//	cone NAME cx cz r0 y0 r1 y1 [shell=N] an upright cylinder whose radius runs
//	                               from r0 at y0 to r1 at y1; shell keeps only
//	                               the outer N voxels of it
//	ball NAME cx cy cz r [shell=N] a sphere, or the skin of one
//	beam NAME x0 y0 z0 x1 y1 z1 [r=N]     every voxel within r of the line
//	                               between two voxel centres (default 0.75)
//	                               Every shape takes `air` for NAME, which
//	                               carves it out instead: a window band out of
//	                               a slanted hull, a shaft through the decks
//	blast cx cy cz r [seed=N] [scorch=NAME] [debris=N]
//	                               a ragged hole: air inside a lumpy sphere,
//	                               whatever is left solid at its edge turned to
//	                               the scorch block, any glass in it broken out,
//	                               and debris loose blocks of it thrown outward,
//	                               the same every build
//	glass cx cz rOuter rInner y     window panes filling the ring a `tube` of
//	                               the same numbers would have walled: they are
//	                               see-through to look at and wall to walk into
//	entity NAME at=X,Y,Z [mesh=M] [sprite=ID] [weapon=ID [ammo=true]] [facing=DX[,DY],DZ] [collider=false] [passthrough=true] [shadow=false]
//	       [step=DX,DY,DZ times=N]
//	                               [code=NNNN]
//	                               sprite=ID is a folder under assets/sprites -
//	                               a character or a piece of scenery drawn as a
//	                               picture. A NAME of "-" takes the folder's own.
//	                               code is the four digits a keypad
//	                               (mesh=keypad) asks for; the door it holds
//	                               shut is whichever one is nearest to it.
//	                               facing is a direction on the floor, or
//	                               X,Y,Z to turn a piece out of that plane:
//	                               facing=0,-1,0 lays a slide door down as a
//	                               floor hatch, and it slides sideways
//	note X Y Z THE WRITING         a sheet of paper on the floor. Everything
//	                               after the three coordinates is what it says,
//	                               spaces and all, and any four digits in it is
//	                               a code the player learns by reading it
//	sign X Y Z FACING THE WORDS     a wall display with writing on it, standing
//	                               on the floor at X Y Z with its face turned to
//	                               FACING (DX,DZ). Lines split at |, and a line
//	                               starting with ! is a warning, written red
//	graffiti X Y Z FACING THE WORDS paint on a wall, placed like a sign and hung
//	                               lower: lines split at |, a line starting
//	                               with ! is red, ~ black, * yellow, else white
//	log X Y Z CUE THE LABEL        an audio log on the floor: a cartridge with a
//	                               recording on it. CUE names the file under
//	                               assets/audio/logs without its extension, and
//	                               the rest of the line is the label on its lid
//	light X Y Z [color=R,G,B] [power=N] [baked] [yaw=N] [pitch=N]
//	                               [angle=N] [reach=N] [step=DX,DY,DZ times=N]
//	sky full                       no horizon: the cubemap covers the sphere
//	spin DEG [axis=X,Y,Z]          degrees a second the sky turns, about Y or
//	                               the axis given
//	sun DX DY DZ [at=X,Y,Z] [dist=N] [depth=N] [angle=N] [color=R,G,B] [power=N]
//	                               the star in the sky, as a light as well as a
//	                               picture: DX DY DZ is the way to it in the
//	                               sky's own frame, so the spin turns it with the
//	                               sky. It stands dist out from at, brackets
//	                               depth of the level with its shadow frustum,
//	                               and power is the brightness where the beam
//	                               lands rather than at the light
//	pressure CX CZ R Y0 Y1 [R1]    the cylinder that has air and gravity in it;
//	                               R1 is its radius at Y1, for a hull that tapers
//	fog START END                  how far the level can be seen through
//	ambient R G B STRENGTH         what an unlit corner looks like
//	ceiling R G B / floor R G B    the two flat colours, 0-255
//	music NAME                     a track under assets/audio, without extension
//
// Voxel coordinates are integers and a box is inclusive of both corners: the
// voxel at x,y,z fills the world cube from x,y,z to x+1,y+1,z+1. Positions in
// world units (spawn, entity, light) are floats.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/quads/voxel"
	"github.com/memmaker/voxellib/tpack/png_chunks"
)

const (
	mapDir      = "assets/maps"
	scriptName  = "build.map"
	mapFileName = "map.json"
	// atlasFileName is the atlas a level's folder draws with, named after the
	// template it would otherwise be given a copy of (quads.UseMapAtlasIn).
	atlasFileName = "map_64x64.png"
	// texRoot is quads.TexLibraryRoot: what a `tex` path is relative to.
	texRoot = "assets/textures"
	// chunkSize is the edge of a chunk on every axis, and has to be the engine's
	// own (see map_build.go's levelChunkSize) or a level built here loads at the
	// wrong size.
	chunkSize = 32
)

func main() {
	name := flag.String("map", "", "level folder under "+mapDir)
	show := flag.Int("show", -1, "instead of building, print what is solid at this height")
	side := flag.Int("side", -1, "instead of building, print what is solid in the x-y plane at this z")
	checkLeaks := flag.Bool("leaks", false, "after building, flood the air from the spawn and say whether it reaches vacuum")
	checkReach := flag.Bool("reach", false, "after building, say whether every sign can be got to from the spawn")
	gallery := flag.Bool("gallery", false, "write the imported gallery's build.map from whatever is installed")
	force := flag.Bool("force", false, "build even over a level the editor has saved, throwing its edits away")
	flag.Parse()

	if *gallery {
		if err := writeGallery(); err != nil {
			fmt.Fprintln(os.Stderr, "mapgen:", err)
			os.Exit(1)
		}
		return
	}

	if *name == "" {
		flag.Usage()
		os.Exit(2)
	}
	dir := filepath.Join(mapDir, *name)

	if *show >= 0 || *side >= 0 {
		blocks := voxel.NewMapWithEmptyChunks(1, 1, 1, chunkSize, chunkSize)
		if err := blocks.LoadRegions(dir); err != nil {
			fatal(err)
		}
		if *side >= 0 {
			fmt.Print(sideSlice(blocks, int32(*side)))
		} else {
			fmt.Print(slice(blocks, int32(*show)))
		}
		return
	}

	// A level saved from the editor is its own source now and build.map no longer
	// describes it, so building would throw the editing away.
	if existing, err := os.ReadFile(filepath.Join(dir, mapFileName)); err == nil && !*force {
		var saved struct{ Edited bool }
		if json.Unmarshal(existing, &saved) == nil && saved.Edited {
			fatal(fmt.Errorf("%s has been saved from the editor, so %s no longer describes it; -force builds over the edits", *name, scriptName))
		}
	}

	script, err := os.ReadFile(filepath.Join(dir, scriptName))
	if err != nil {
		fatal(err)
	}
	level, err := build(string(script))
	if err != nil {
		fatal(err)
	}
	if err := level.blocks.SaveRegions(dir); err != nil {
		fatal(err)
	}
	if len(level.textures) > 0 {
		if err := writeAtlas(dir, level.textures); err != nil {
			fatal(err)
		}
	}
	// core.MapLoadResult is what the engine's own mapFile mirrors field for
	// field, so marshalling the real thing writes the file the engine reads. The
	// colours are concrete color.RGBA rather than nil for the same reason.
	data, err := json.MarshalIndent(level.result, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, mapFileName), data, 0o644); err != nil {
		fatal(err)
	}
	if *checkLeaks {
		report, sealed := leaks(level)
		fmt.Println(report)
		if !sealed {
			defer os.Exit(1)
		}
	}
	if *checkReach {
		report, all := reach(level)
		fmt.Println(report)
		if !all {
			defer os.Exit(1)
		}
	}
	bounds := level.blocks.Bounds()
	fmt.Printf("%s: %dx%dx%d voxels, %d solid, %d entities, %d lights, %d panes\n",
		*name, bounds.X, bounds.Y, bounds.Z, level.solid,
		len(level.result.Entities), len(level.result.Lights), len(level.result.Panes))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mapgen:", err)
	os.Exit(1)
}

// level is a script part-way through being run: the blocks so far, everything
// that is not blocks, and the names the script has given itself.
type level struct {
	blocks *voxel.Map
	result core.MapLoadResult
	kinds  map[string]*voxel.Block
	solid  int
	// textures is the level's own atlas, slot by slot, and slots names them.
	textures []texture
	slots    map[string]uint16
}

// texture is one `tex` line: a file in the library and whether it glows.
type texture struct {
	path string
	glow bool
}

// build runs a whole script. Nothing is written until it has run to the end, so
// a script with a typo on its last line leaves the level it was rebuilding
// exactly as it was.
func build(script string) (*level, error) {
	l := &level{
		kinds: map[string]*voxel.Block{},
		slots: map[string]uint16{},
		result: core.MapLoadResult{
			CeilingColor: color.RGBA{R: 20, G: 20, B: 26, A: 255},
			FloorColor:   color.RGBA{R: 20, G: 20, B: 26, A: 255},
			PlayerDir:    mgl32.Vec2{0, 1},
		},
	}
	l.setSize(voxel.Int3{X: 64, Y: 32, Z: 64})

	scanner := bufio.NewScanner(strings.NewReader(script))
	for line := 1; scanner.Scan(); line++ {
		words, options, err := parseLine(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if len(words) == 0 {
			continue
		}
		if err := l.run(words, options); err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", line, words[0], err)
		}
	}
	l.capGlass()
	return l, scanner.Err()
}

// capGlass puts glass on the top and bottom of every glazed cell that has open
// air there.
//
// A `glass` ring is a course of upright sheets: it glazes the sides of its
// cells and leaves their top and bottom open, which is all a window in a wall
// needs. Where a blast has torn the hull open the player ends up looking along
// the course instead of through it, and an open top is a cell with nothing
// drawn in it at all - a hole in the glass you cannot walk into, because the
// cell still collides. Capping them closes that: the same sheet, laid flat.
//
// It runs once the whole script has, so the cells a later course stacked on top
// and the ones a blast broke out are both already known - a cap goes on what is
// still glass and still exposed, and never between two courses of it. The cells
// are sorted before they are capped: a build has to write the same file twice.
func (l *level) capGlass() {
	glazed := map[core.Int3]bool{}
	for _, pane := range l.result.Panes {
		glazed[pane.Cell()] = true
	}
	cells := make([]core.Int3, 0, len(glazed))
	for cell := range glazed {
		cells = append(cells, cell)
	}
	slices.SortFunc(cells, func(a, b core.Int3) int {
		if a.Y != b.Y {
			return int(a.Y - b.Y)
		}
		if a.X != b.X {
			return int(a.X - b.X)
		}
		return int(a.Z - b.Z)
	})
	for _, cell := range cells {
		for _, up := range []int32{1, -1} {
			if glazed[core.Int3{X: cell.X, Y: cell.Y + up, Z: cell.Z}] {
				continue
			}
			l.result.Panes = append(l.result.Panes, core.Pane{
				Position: mgl32.Vec3{
					float32(cell.X) + 0.5,
					float32(cell.Y) + 0.5 + float32(up)*0.5,
					float32(cell.Z) + 0.5,
				},
				Normal: mgl32.Vec3{0, float32(up), 0},
				Width:  1,
				Height: 1,
			})
		}
	}
}

// parseLine splits one line into its positional words and its key=value
// options. A value may not contain a space, which is what keeps this a split
// rather than a parser. A bare word once the options have started is a flag -
// `baked`, with nothing to the right of it - which is also what stops a
// positional argument written out of order from being read as one.
func parseLine(text string) ([]string, map[string]string, error) {
	if hash := strings.IndexByte(text, '#'); hash >= 0 {
		text = text[:hash]
	}
	var words []string
	options := map[string]string{}
	for _, field := range strings.Fields(text) {
		if key, value, isOption := strings.Cut(field, "="); isOption {
			if _, twice := options[key]; twice {
				return nil, nil, fmt.Errorf("%s given twice", key)
			}
			options[key] = value
			continue
		}
		if len(options) > 0 {
			options[field] = ""
			continue
		}
		words = append(words, field)
	}
	return words, options, nil
}

func (l *level) run(words []string, options map[string]string) error {
	args := words[1:]
	switch words[0] {
	case "size":
		size, err := ints3(args, 0)
		if err != nil {
			return err
		}
		l.setSize(voxel.Int3{X: size[0], Y: size[1], Z: size[2]})

	case "spawn":
		pos, err := floats3(args, 0)
		if err != nil {
			return err
		}
		l.result.PlayerSpawn = pos
		if dir, given := options["dir"]; given {
			facing, err := vec2(dir)
			if err != nil {
				return err
			}
			l.result.PlayerDir = facing
		}

	case "block":
		if len(args) < 2 {
			return fmt.Errorf("want a name and at least one texture slot")
		}
		textures := make([]uint16, 0, 3)
		for _, arg := range args[1:] {
			slot, named := l.slots[arg]
			if !named {
				number, err := strconv.ParseUint(arg, 10, 16)
				if err != nil {
					return fmt.Errorf("texture %q is neither a slot nor a `tex` name", arg)
				}
				slot = uint16(number)
			}
			textures = append(textures, slot)
		}
		for len(textures) < 3 {
			textures = append(textures, textures[len(textures)-1])
		}
		wall, floor, ceiling := textures[0], textures[1], textures[2]
		l.kinds[args[0]] = voxel.NewBlockWithSides(map[voxel.CubeSide]uint16{
			voxel.North: wall, voxel.South: wall, voxel.East: wall, voxel.West: wall,
			voxel.Top: floor, voxel.Bottom: ceiling,
		})

	case "fill", "clear":
		offset := 1
		block := (*voxel.Block)(nil)
		if words[0] == "clear" {
			offset = 0
		} else {
			kind, known := l.kinds[args[0]]
			if !known {
				return fmt.Errorf("no block called %q - name one with `block` first", args[0])
			}
			block = kind
		}
		box, err := ints3(args, offset)
		if err != nil {
			return err
		}
		far, err := ints3(args, offset+3)
		if err != nil {
			return err
		}
		// step and times lay the same box down again and again, each one moved
		// on by step: a row of cabins is one line rather than fifteen.
		times, err := option(options, "times", 1)
		if err != nil {
			return err
		}
		step := [3]int32{}
		if value, given := options["step"]; given {
			moved, err := vec3(value)
			if err != nil {
				return err
			}
			step = [3]int32{int32(moved.X()), int32(moved.Y()), int32(moved.Z())}
		}
		for range int(times) {
			l.paint(box, far, block)
			for axis := range 3 {
				box[axis] += step[axis]
				far[axis] += step[axis]
			}
		}

	case "cyl", "tube":
		kind, known := l.kinds[args[0]]
		if !known && args[0] != "air" {
			return fmt.Errorf("no block called %q - name one with `block` first", args[0])
		}
		want := 6
		if words[0] == "tube" {
			want = 7
		}
		if len(args) != want {
			return fmt.Errorf("want %d arguments, got %d", want, len(args))
		}
		numbers, err := ints(args[1:])
		if err != nil {
			return err
		}
		centreX, centreZ, outer := numbers[0], numbers[1], numbers[2]
		inner := int32(-1)
		rest := numbers[3:]
		if words[0] == "tube" {
			inner, rest = numbers[3], numbers[4:]
		}
		l.cylinder(kind, centreX, centreZ, outer, inner, rest[0], rest[1])

	case "tex":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "glow") {
			return fmt.Errorf("want NAME PATH [glow]")
		}
		if _, taken := l.slots[args[0]]; taken {
			return fmt.Errorf("%q is named twice", args[0])
		}
		if _, err := os.Stat(filepath.Join(texRoot, filepath.FromSlash(args[1]))); err != nil {
			return err
		}
		l.slots[args[0]] = uint16(len(l.textures))
		l.textures = append(l.textures, texture{path: args[1], glow: len(args) == 3})

	case "cone", "ball", "beam":
		kind, known := l.kinds[args[0]]
		if !known && args[0] != "air" {
			return fmt.Errorf("no block called %q - name one with `block` first", args[0])
		}
		want := map[string]int{"cone": 7, "ball": 5, "beam": 7}[words[0]]
		if len(args) != want {
			return fmt.Errorf("want %d arguments, got %d", want, len(args))
		}
		numbers, err := floats(args[1:])
		if err != nil {
			return err
		}
		shell, err := option(options, "shell", 0)
		if err != nil {
			return err
		}
		switch words[0] {
		case "cone":
			l.cone(kind, numbers[0], numbers[1], numbers[2], numbers[3], numbers[4], numbers[5], shell)
		case "ball":
			l.ball(kind, mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, numbers[3], shell)
		case "beam":
			radius, err := option(options, "r", 0.75)
			if err != nil {
				return err
			}
			l.beam(kind, mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, mgl32.Vec3{numbers[3], numbers[4], numbers[5]}, radius)
		}

	case "blast":
		if len(args) != 4 {
			return fmt.Errorf("want CX CY CZ R")
		}
		numbers, err := floats(args)
		if err != nil {
			return err
		}
		var scorch *voxel.Block
		if name, given := options["scorch"]; given {
			if scorch = l.kinds[name]; scorch == nil {
				return fmt.Errorf("no block called %q", name)
			}
		}
		seed, err := option(options, "seed", 1)
		if err != nil {
			return err
		}
		debris, err := option(options, "debris", 0)
		if err != nil {
			return err
		}
		if debris > 0 && scorch == nil {
			return fmt.Errorf("debris is made of the scorch block - name one with scorch=")
		}
		l.blast(mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, numbers[3], uint32(seed), scorch, int(debris))

	case "glass":
		if len(args) != 5 {
			return fmt.Errorf("want CX CZ ROUTER RINNER Y")
		}
		numbers, err := ints(args)
		if err != nil {
			return err
		}
		l.glazeRing(numbers[0], numbers[1], numbers[2], numbers[3], numbers[4])

	case "entity":
		if len(args) != 1 {
			return fmt.Errorf("want one name")
		}
		at, given := options["at"]
		if !given {
			return fmt.Errorf("needs at=X,Y,Z")
		}
		pos, err := vec3(at)
		if err != nil {
			return err
		}
		entity := core.MapEntity{
			Name:           args[0],
			MeshName:       options["mesh"],
			CreateCollider: options["collider"] != "false",
			PassThrough:    options["passthrough"] == "true",
			NoShadow:       options["shadow"] == "false",
			Code:           options["code"],
		}
		// sprite= names a folder under assets/sprites: a character or a piece
		// of scenery that is a picture rather than a model. The frames and the
		// frame size are read off that folder rather than written here, so a
		// script never repeats what the folder already says.
		if id, given := options["sprite"]; given {
			kind, err := readSpriteKind(id)
			if err != nil {
				return err
			}
			entity.SpriteSheetName = id
			entity.SpriteSheetTileSize = kind.Tile
			entity.TextureIndices = kind.idleFrames()
			if entity.Name == "-" {
				entity.Name = kind.Name
			}
		}
		// weapon= names a folder under assets/weapons: a gun lying on the floor
		// for the player to walk over. ammo=true is the box of what it fires
		// rather than the gun, which is the other picture every weapon brings.
		if id, given := options["weapon"]; given {
			if _, err := os.Stat(filepath.Join("assets", "weapons", id, "weapon.json")); err != nil {
				return fmt.Errorf("no weapon called %q", id)
			}
			entity.WeaponName = id
			entity.WeaponAmmo = options["ammo"] == "true"
			if entity.Name == "-" {
				entity.Name = id
			}
		}
		if turned, given := options["facing"]; given {
			direction, err := facing(turned)
			if err != nil {
				return err
			}
			entity.Facing = direction
		}
		// scale= stands a piece bigger or smaller than its model or picture, as
		// the editor's wheel does - see core.MapEntity.Scale.
		if entity.Scale, err = option(options, "scale", 0); err != nil {
			return err
		}
		// patrol= is the route a walking entity beats, one leg an "X,Y,Z"
		// point and ";" between them - see core.MapEntity.Patrol. An actor
		// spawned this way starts here and walks the list in order.
		if route, given := options["patrol"]; given {
			points, err := vec3List(route)
			if err != nil {
				return err
			}
			entity.Patrol = points
		}
		// step and times stand the same piece up again and again, as fill lays a
		// row of boxes: a ladder the height of a shaft is one line of script.
		times, err := option(options, "times", 1)
		if err != nil {
			return err
		}
		step := mgl32.Vec3{}
		if value, given := options["step"]; given {
			if step, err = vec3(value); err != nil {
				return err
			}
		}
		for range int(times) {
			l.result.Entities = append(l.result.Entities, core.EntitySpawn{Position: pos, Entity: entity})
			pos = pos.Add(step)
		}

	case "note":
		// The one command whose argument is prose, which is why it is a command
		// of its own rather than a `text=` on `entity`: a key=value option may
		// not hold a space, and a note that could not hold a space would be a
		// note that cannot say anything.
		if len(args) < 4 {
			return fmt.Errorf("want X Y Z and something for it to say")
		}
		pos, err := floats3(args, 0)
		if err != nil {
			return err
		}
		l.result.Entities = append(l.result.Entities, core.EntitySpawn{Position: pos, Entity: core.MapEntity{
			Name:           "Note",
			MeshName:       "note",
			CreateCollider: true,
			Text:           strings.Join(args[3:], " "),
		}})

	case "sign", "graffiti":
		// Prose again, like the note: FACING is the one word between the place and
		// the words, the way the reader stands from the wall - 0,1 for a sign on a
		// wall whose open side faces +z.
		if len(args) < 5 {
			return fmt.Errorf("want X Y Z FACING and something for it to say")
		}
		pos, err := floats3(args, 0)
		if err != nil {
			return err
		}
		facing, err := vec2(args[3])
		if err != nil {
			return err
		}
		l.result.Entities = append(l.result.Entities, core.EntitySpawn{Position: pos, Entity: core.MapEntity{
			Name:           strings.ToUpper(words[0][:1]) + words[0][1:],
			MeshName:       words[0],
			CreateCollider: true,
			Facing:         mgl32.Vec3{facing.X(), 0, facing.Y()},
			Text:           strings.Join(args[4:], " "),
		}})

	case "log":
		// The note's twin, and its argument is prose for the same reason - see
		// there. The cue comes first because it is one word and the label is
		// not: a name with no spaces in it can be told from the rest of a line
		// only if it is at a known end of it.
		if len(args) < 5 {
			return fmt.Errorf("want X Y Z CUE and a label")
		}
		pos, err := floats3(args, 0)
		if err != nil {
			return err
		}
		l.result.Entities = append(l.result.Entities, core.EntitySpawn{Position: pos, Entity: core.MapEntity{
			Name:           "AudioLog",
			MeshName:       "audiolog",
			CreateCollider: true,
			Sound:          args[3],
			Text:           strings.Join(args[4:], " "),
		}})

	case "light":
		pos, err := floats3(args, 0)
		if err != nil {
			return err
		}
		light := core.MapLight{
			Position: pos,
			Color:    mgl32.Vec3{1, 1, 1},
			// The engine's own middle-of-the-road lamp, so a light with nothing
			// said about it still lights something.
			Intensity: 6, Angle: 100, Reach: 10, Pitch: 90,
		}
		if value, given := options["color"]; given {
			rgb, err := vec3(value)
			if err != nil {
				return err
			}
			light.Color = rgb
		}
		if _, baked := options["baked"]; baked {
			light.Baked = true
		}
		for key, target := range map[string]*float32{
			"power": &light.Intensity, "yaw": &light.Yaw, "pitch": &light.Pitch,
			"angle": &light.Angle, "reach": &light.Reach,
		} {
			if value, given := options[key]; given {
				number, err := strconv.ParseFloat(value, 32)
				if err != nil {
					return fmt.Errorf("%s=%s: %w", key, value, err)
				}
				*target = float32(number)
			}
		}
		// step and times hang a row of the same lamp, as fill lays a row of boxes.
		times, err := option(options, "times", 1)
		if err != nil {
			return err
		}
		step := mgl32.Vec3{}
		if value, given := options["step"]; given {
			if step, err = vec3(value); err != nil {
				return err
			}
		}
		for range int(times) {
			l.result.Lights = append(l.result.Lights, light)
			light.Position = light.Position.Add(step)
		}

	case "sky":
		if len(args) != 1 || args[0] != "full" {
			return fmt.Errorf("the only sky is `sky full`")
		}
		l.result.SkyFullSphere = true

	case "spin":
		spin, err := float(args, 0)
		if err != nil {
			return err
		}
		l.station().Spin = spin
		if value, given := options["axis"]; given {
			axis, err := vec3(value)
			if err != nil {
				return err
			}
			l.station().SpinAxis = axis
		}

	case "sun":
		// The star the sky already shows, made into a light. The direction is the
		// way to it in the sky's own frame, which is where it is drawn: read it
		// off the cubemap once and the spin takes care of the rest.
		towards, err := floats3(args, 0)
		if err != nil {
			return err
		}
		if towards.Len() == 0 {
			return fmt.Errorf("wants a direction to the star, not a zero vector")
		}
		sun := &core.Sun{Towards: towards.Normalize(), Color: mgl32.Vec3{1, 1, 1}}
		if value, given := options["at"]; given {
			if sun.At, err = vec3(value); err != nil {
				return err
			}
		}
		if value, given := options["color"]; given {
			if sun.Color, err = vec3(value); err != nil {
				return err
			}
		}
		for _, knob := range []struct {
			name     string
			into     *float32
			fallback float32
		}{
			{"dist", &sun.Distance, 150},
			{"depth", &sun.Depth, 200},
			{"angle", &sun.Angle, 26},
			{"power", &sun.Power, 4},
		} {
			if *knob.into, err = option(options, knob.name, knob.fallback); err != nil {
				return err
			}
		}
		l.station().Sun = sun

	case "pressure":
		numbers, err := floats(args)
		if err != nil {
			return err
		}
		if len(numbers) != 5 && len(numbers) != 6 {
			return fmt.Errorf("want CX CZ RADIUS Y0 Y1 [RADIUS1]")
		}
		station := l.station()
		station.CenterX, station.CenterZ, station.Radius = numbers[0], numbers[1], numbers[2]
		station.MinY, station.MaxY = numbers[3], numbers[4]
		if len(numbers) == 6 {
			station.TopRadius = numbers[5]
		}

	case "fog":
		numbers, err := floats(args)
		if err != nil {
			return err
		}
		if len(numbers) != 2 {
			return fmt.Errorf("want START END")
		}
		l.result.FogStart, l.result.FogEnd = numbers[0], numbers[1]

	case "ambient":
		numbers, err := floats(args)
		if err != nil {
			return err
		}
		if len(numbers) != 4 {
			return fmt.Errorf("want R G B STRENGTH")
		}
		l.result.AmbientColor = mgl32.Vec3{numbers[0], numbers[1], numbers[2]}
		l.result.AmbientStrength = numbers[3]

	case "ceiling", "floor":
		rgb, err := ints(args)
		if err != nil {
			return err
		}
		if len(rgb) != 3 {
			return fmt.Errorf("want R G B, 0-255")
		}
		flat := color.RGBA{R: uint8(rgb[0]), G: uint8(rgb[1]), B: uint8(rgb[2]), A: 255}
		if words[0] == "ceiling" {
			l.result.CeilingColor = flat
		} else {
			l.result.FloorColor = flat
		}

	case "music":
		if len(args) != 1 {
			return fmt.Errorf("want one track name")
		}
		l.result.Music = args[0]

	default:
		return fmt.Errorf("no such command")
	}
	return nil
}

// station is the level's rotating body, made the first time a command mentions
// one: a level with no spin and no pressure hull stays an ordinary level.
func (l *level) station() *core.Station {
	if l.result.Station == nil {
		l.result.Station = &core.Station{}
	}
	return l.result.Station
}

func (l *level) setSize(size voxel.Int3) {
	chunksFor := func(voxels int32) int32 {
		return int32(math.Ceil(float64(voxels) / chunkSize))
	}
	l.blocks = voxel.NewMapWithEmptyChunks(
		chunksFor(size.X), chunksFor(size.Y), chunksFor(size.Z), chunkSize, chunkSize)
	l.blocks.SetBounds(size)
	l.solid = 0
}

// paint fills the inclusive box between two corners, with a nil block meaning
// air. Out-of-bounds cells are dropped by SetBlock rather than being an error:
// a script that overshoots the edge of the level by a voxel is not worth
// stopping for.
func (l *level) paint(from, to [3]int32, block *voxel.Block) {
	for x := min(from[0], to[0]); x <= max(from[0], to[0]); x++ {
		for y := min(from[1], to[1]); y <= max(from[1], to[1]); y++ {
			for z := min(from[2], to[2]); z <= max(from[2], to[2]); z++ {
				l.set(x, y, z, block)
			}
		}
	}
}

// inTube is which columns a cylinder or a ring covers, measured from the middle
// of a voxel to the middle of the cylinder - so a radius of r is r voxels of
// wall out from the centre column in every direction rather than r-and-a-bit on
// one side and r-less-a-bit on the other. An inner radius below zero is a solid
// disc. Both the blocks and the glass go through it, which is what lets a ring
// of panes be told to fill exactly the ring a wall left open.
func inTube(centreX, centreZ, outer, inner, x, z int32) bool {
	dx, dz := float64(x-centreX), float64(z-centreZ)
	distance := dx*dx + dz*dz
	return distance <= float64(outer)*float64(outer) &&
		(inner < 0 || distance >= float64(inner)*float64(inner))
}

// cylinder fills an upright cylinder between two heights, inclusive. An inner
// radius of zero or more leaves everything inside it alone, which is the
// difference between a pillar and a wall.
func (l *level) cylinder(block *voxel.Block, centreX, centreZ, outer, inner, y0, y1 int32) {
	for x := centreX - outer; x <= centreX+outer; x++ {
		for z := centreZ - outer; z <= centreZ+outer; z++ {
			if !inTube(centreX, centreZ, outer, inner, x, z) {
				continue
			}
			for y := min(y0, y1); y <= max(y0, y1); y++ {
				l.set(x, y, z, block)
			}
		}
	}
}

// glazeRing glazes the outer skin of a ring: every face of it that borders
// something that is not also glass, inside and out.
//
// Every face, not only the ones looking outwards, because a pane is drawn as a
// quad and collides as the whole voxel it fills (core.Pane.Cell). Glazing only
// the outer face left the inner one solid and invisible - an invisible wall a
// voxel short of the window, with the unglazed side faces of the neighbouring
// cells reading as gaps between the panes. Draw every solid face and there is
// nothing you can touch that you cannot see.
//
// The skin, not the whole ring, because a ring rasterised from two radii is two
// voxels thick where it crosses an axis, and glazing both would tint those
// stretches of window twice as dark as the rest. What is behind the skin there
// is an open reveal - visible geometry, not a surprise.
func (l *level) glazeRing(centreX, centreZ, outer, inner, y int32) {
	sides := []voxel.Int3{{X: 1}, {X: -1}, {Z: 1}, {Z: -1}}
	skin := func(x, z int32) bool {
		if !inTube(centreX, centreZ, outer, inner, x, z) {
			return false
		}
		for _, side := range sides {
			if inTube(centreX, centreZ, outer, inner, x+side.X, z+side.Z) {
				continue
			}
			if side.X*(x-centreX)+side.Z*(z-centreZ) > 0 {
				return true // an open face looking away from the middle
			}
		}
		return false
	}
	for x := centreX - outer; x <= centreX+outer; x++ {
		for z := centreZ - outer; z <= centreZ+outer; z++ {
			if !skin(x, z) {
				continue
			}
			for _, side := range sides {
				if skin(x+side.X, z+side.Z) {
					continue
				}
				l.result.Panes = append(l.result.Panes, core.Pane{
					Position: mgl32.Vec3{
						float32(x) + 0.5 + float32(side.X)*0.5,
						float32(y) + 0.5,
						float32(z) + 0.5 + float32(side.Z)*0.5,
					},
					Normal: mgl32.Vec3{float32(side.X), 0, float32(side.Z)},
					Width:  1,
					Height: 1,
				})
			}
		}
	}
}

// inShell is whether a squared distance lands inside a radius and, with a shell
// above zero, within that many voxels of its outside.
func inShell(distance2, radius, shell float32) bool {
	inner := radius - shell
	return distance2 <= radius*radius && (shell <= 0 || inner <= 0 || distance2 >= inner*inner)
}

// cone is cylinder with the radius worked out again on every layer, running
// straight from r0 at y0 to r1 at y1. Distances are from voxel to centre the
// way inTube measures them, so a cone with r0 == r1 is the cylinder `cyl` draws.
func (l *level) cone(block *voxel.Block, centreX, centreZ, r0, y0, r1, y1, shell float32) {
	reach := int32(math.Ceil(float64(max(r0, r1))))
	for y := int32(min(y0, y1)); y <= int32(max(y0, y1)); y++ {
		radius := r0
		if y1 != y0 {
			radius += (r1 - r0) * (float32(y) - y0) / (y1 - y0)
		}
		for x := int32(centreX) - reach; x <= int32(centreX)+reach; x++ {
			for z := int32(centreZ) - reach; z <= int32(centreZ)+reach; z++ {
				dx, dz := float32(x)-centreX, float32(z)-centreZ
				if inShell(dx*dx+dz*dz, radius, shell) {
					l.set(x, y, z, block)
				}
			}
		}
	}
}

func (l *level) ball(block *voxel.Block, centre mgl32.Vec3, radius, shell float32) {
	l.around(centre, radius, func(x, y, z int32, offset mgl32.Vec3) {
		if inShell(offset.Dot(offset), radius, shell) {
			l.set(x, y, z, block)
		}
	})
}

// beam is every voxel whose centre is within radius of the segment between two
// others, which is a line with no gaps in it at any angle.
func (l *level) beam(block *voxel.Block, from, to mgl32.Vec3, radius float32) {
	along := to.Sub(from)
	length2 := along.Dot(along)
	l.around(from.Add(along.Mul(0.5)), along.Len()/2+radius, func(x, y, z int32, _ mgl32.Vec3) {
		at := mgl32.Vec3{float32(x), float32(y), float32(z)}
		t := float32(0)
		if length2 > 0 {
			t = min(max(at.Sub(from).Dot(along)/length2, 0), 1)
		}
		if at.Sub(from.Add(along.Mul(t))).Len() <= radius {
			l.set(x, y, z, block)
		}
	})
}

// blast is battle damage: a hole whose outline is lumpy rather than a sphere's,
// a scorched rim on whatever it left standing, and loose pieces thrown round
// it. Everything random in it is drawn from the seed in a fixed order, so a
// rebuild blows exactly the same hole and another seed blows a different one.
func (l *level) blast(centre mgl32.Vec3, radius float32, seed uint32, scorch *voxel.Block, debris int) {
	random := rand.New(rand.NewPCG(uint64(seed), 0x5eed))
	var phase [3]float64
	for i := range phase {
		phase[i] = random.Float64() * 2 * math.Pi
	}
	// Three lobes a side, from where the direction points: the hole reaches a
	// third further out along some of them and a third less along others.
	outline := func(offset mgl32.Vec3) float32 {
		length := offset.Len()
		if length == 0 {
			return radius
		}
		u := offset.Mul(1 / length)
		lump := math.Sin(3*float64(u.X())+phase[0]) + math.Sin(3*float64(u.Y())+phase[1]) + math.Sin(3*float64(u.Z())+phase[2])
		return radius * (1 + 0.35*float32(lump/3))
	}
	l.around(centre, radius*1.35+2.5, func(x, y, z int32, offset mgl32.Vec3) {
		distance := offset.Len()
		edge := outline(offset) + (random.Float32()-0.5)*1.2
		switch {
		case distance < edge:
			l.set(x, y, z, nil)
		case scorch != nil && distance < edge+1.5 && l.blocks.IsSolidBlockAt(x, y, z):
			l.set(x, y, z, scorch)
		}
	})
	// Glass in the hole goes with the wall it was set in. Tested against the
	// outline without the jitter, which would take a draw from the seed and so
	// move every piece of debris after it.
	l.result.Panes = slices.DeleteFunc(l.result.Panes, func(pane core.Pane) bool {
		cell := pane.Cell()
		offset := mgl32.Vec3{float32(cell.X), float32(cell.Y), float32(cell.Z)}.Sub(centre)
		return offset.Len() < outline(offset)
	})

	// The pieces go out, away from the hull's axis, and never land where there
	// is air: a blast on the hull throws them into space rather than into a
	// room, and a piece in a shaft is a shaft nobody can climb.
	hull := l.result.Station
	for range debris {
		direction := mgl32.Vec3{random.Float32() - 0.5, random.Float32() - 0.5, random.Float32() - 0.5}
		distance := radius * (1.4 + 1.6*random.Float32())
		if direction.Len() == 0 {
			continue
		}
		at := centre.Add(direction.Normalize().Mul(distance))
		x, y, z := int32(math.Round(float64(at.X()))), int32(math.Round(float64(at.Y()))), int32(math.Round(float64(at.Z())))
		cell := mgl32.Vec3{float32(x) + 0.5, float32(y) + 0.5, float32(z) + 0.5}
		if hull != nil {
			outward := direction.X()*(centre.X()+0.5-hull.CenterX) + direction.Z()*(centre.Z()+0.5-hull.CenterZ)
			if outward < 0 || hull.Pressurised(cell) {
				continue
			}
		}
		if !l.blocks.IsSolidBlockAt(x, y, z) {
			l.set(x, y, z, scorch)
		}
	}
}

// around visits every voxel in the cube that holds a sphere, with its offset
// from the centre, for the shapes that are distances from a point.
func (l *level) around(centre mgl32.Vec3, radius float32, visit func(x, y, z int32, offset mgl32.Vec3)) {
	lo := func(v float32) int32 { return int32(math.Floor(float64(v - radius))) }
	hi := func(v float32) int32 { return int32(math.Ceil(float64(v + radius))) }
	for x := lo(centre.X()); x <= hi(centre.X()); x++ {
		for y := lo(centre.Y()); y <= hi(centre.Y()); y++ {
			for z := lo(centre.Z()); z <= hi(centre.Z()); z++ {
				visit(x, y, z, mgl32.Vec3{float32(x), float32(y), float32(z)}.Sub(centre))
			}
		}
	}
}

// writeAtlas writes the level's own atlas from its `tex` lines: the sheet the
// engine draws with, the .paths record the editor reads to know which library
// texture sits in which slot, and the list of the ones that glow. It is the same
// three things quads.AtlasIndex.Bake writes, so the editor opens a scripted
// level's atlas as if it had baked it.
func writeAtlas(dir string, textures []texture) error {
	const tile, columns = 64, 16 // map_64x64, sixteen across like assets/base
	rows := (len(textures) + columns - 1) / columns
	sheet := image.NewRGBA(image.Rect(0, 0, columns*tile, rows*tile))
	paths := make([]string, len(textures))
	var glowing []string
	for slot, texture := range textures {
		file, err := os.Open(filepath.Join(texRoot, filepath.FromSlash(texture.path)))
		if err != nil {
			return err
		}
		picture, _, err := image.Decode(file)
		file.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", texture.path, err)
		}
		// Nearest neighbour, like quads.TileImage: pixel art keeps its edges.
		at, from := image.Pt(slot%columns*tile, slot/columns*tile), picture.Bounds()
		for y := 0; y < tile; y++ {
			for x := 0; x < tile; x++ {
				sheet.Set(at.X+x, at.Y+y, picture.At(from.Min.X+x*from.Dx()/tile, from.Min.Y+y*from.Dy()/tile))
			}
		}
		paths[slot] = texture.path
		if texture.glow {
			glowing = append(glowing, strconv.Itoa(slot))
		}
	}
	var encoded, out bytes.Buffer
	if err := png.Encode(&encoded, sheet); err != nil {
		return err
	}
	// "emIS" is quads.EmissiveChunkKey, spelled out so a command-line tool does
	// not link the renderer for one constant.
	if err := png_chunks.InjectTextChunkIntoPNG(&encoded, &out, "emIS", strings.Join(glowing, ",")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, atlasFileName), out.Bytes(), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, strings.TrimSuffix(atlasFileName, ".png")+".paths"),
		[]byte(strings.Join(paths, "\n")+"\n"), 0o644)
}

// leaks floods the air from the spawn, six ways, through everything that is not
// a block, a pane or a door, and says whether it reaches vacuum - somewhere
// outside the pressure hull, or off the edge of the level. A door counts as
// shut, which is what makes this the question a level wants answered: is the
// part you start in a sealed room, or is there a gap in it to space?
//
// The hull is a shape, not a flood (core.Station.Pressurised), so nothing in
// the game ever asks this; a gap is only a gap to the eye. This is the eye.
func leaks(l *level) (string, bool) {
	hull := l.result.Station
	if hull == nil {
		return "leaks: no pressure hull, so nothing to leak", true
	}
	seen, escaped := floodAir(l, spawnVoxel(l), hull)
	if escaped != nil {
		from := seen[*escaped]
		return fmt.Sprintf("leaks: the air from the spawn reaches vacuum at %d,%d,%d, through %d,%d,%d",
			escaped.X, escaped.Y, escaped.Z, from.X, from.Y, from.Z), false
	}
	return fmt.Sprintf("leaks: sealed - %d voxels of air round the spawn, none of them vacuum", len(seen)), true
}

// reach says whether every sign in the level can be got to from the spawn. A
// sign stands where a deck is entered and names it, so a sign the flood cannot
// reach is a deck with no way in - which is the thing worth being told about,
// since a level builds happily with one walled off.
//
// ponytail: connectivity through air, with no hull to stop at, which is the
// truth outside the pressure hull - out there the suit flies and a hole in a
// deck is a way into it. Inside the hull it is generous: it will walk up a well
// a player would need a ladder for. Model the climb if that ever bites.
func reach(l *level) (string, bool) {
	seen, _ := floodAir(l, spawnVoxel(l), nil)
	var missed []string
	signs := 0
	for _, spawn := range l.result.Entities {
		if spawn.Entity.MeshName != "sign" {
			continue
		}
		signs++
		at := voxelOf(spawn.Position)
		if _, been := seen[at]; !been {
			name, _, _ := strings.Cut(spawn.Entity.Text, "|")
			missed = append(missed, fmt.Sprintf("%s at %d,%d,%d", strings.TrimPrefix(name, "!"), at.X, at.Y, at.Z))
		}
	}
	if len(missed) > 0 {
		return fmt.Sprintf("reach: %d of %d signs cannot be got to from the spawn:\n  %s",
			len(missed), signs, strings.Join(missed, "\n  ")), false
	}
	return fmt.Sprintf("reach: all %d signs can be got to from the spawn, through %d voxels of air", signs, len(seen)), true
}

// floodAir spreads through the level's air from one voxel and hands back how it
// got to each one it reached. Glass is always shut - a pane is a wall you can
// see through - and a door is shut or open depending on which question is being
// asked, which is what the hull argument decides.
//
// With a hull it stops the moment it leaves it and says where, with every door
// shut, because that is a leak: air escaping a ship nobody is opening anything
// on. With none it floods the whole level through open doors, vacuum included,
// which is what asking whether somewhere can be got to means on a ship with a
// suit and an airlock on it.
func floodAir(l *level, start voxel.Int3, hull *core.Station) (seen map[voxel.Int3]voxel.Int3, escaped *voxel.Int3) {
	shut := map[voxel.Int3]bool{}
	for _, pane := range l.result.Panes {
		cell := pane.Cell()
		shut[voxel.Int3{X: cell.X, Y: cell.Y, Z: cell.Z}] = true
	}
	for _, spawn := range l.result.Entities {
		if strings.HasPrefix(spawn.Entity.MeshName, "door_") && hull != nil {
			shut[voxelOf(spawn.Position)] = true
		}
	}
	bounds := l.blocks.Bounds()
	seen = map[voxel.Int3]voxel.Int3{start: start}
	queue := []voxel.Int3{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		outside := at.X < 0 || at.Y < 0 || at.Z < 0 || at.X >= bounds.X || at.Y >= bounds.Y || at.Z >= bounds.Z
		if hull != nil {
			centre := mgl32.Vec3{float32(at.X) + 0.5, float32(at.Y) + 0.5, float32(at.Z) + 0.5}
			if outside || !hull.Pressurised(centre) {
				return seen, &at
			}
		} else if outside {
			continue
		}
		for _, side := range []voxel.Int3{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}} {
			next := at.Add(side)
			if _, been := seen[next]; been || shut[next] || l.blocks.IsSolidBlockAt(next.X, next.Y, next.Z) {
				continue
			}
			seen[next] = at
			queue = append(queue, next)
		}
	}
	return seen, nil
}

func spawnVoxel(l *level) voxel.Int3 {
	return voxelOf(l.result.PlayerSpawn)
}

func voxelOf(pos mgl32.Vec3) voxel.Int3 {
	return voxel.Int3{X: int32(pos.X()), Y: int32(pos.Y()), Z: int32(pos.Z())}
}

// option is a number given as key=value, or the fallback when it is not.
func option(options map[string]string, key string, fallback float32) (float32, error) {
	value, given := options[key]
	if !given {
		return fallback, nil
	}
	number, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return 0, fmt.Errorf("%s=%s: %w", key, value, err)
	}
	return float32(number), nil
}

func (l *level) set(x, y, z int32, block *voxel.Block) {
	if block == nil {
		l.blocks.SetAir(voxel.Int3{X: x, Y: y, Z: z})
		return
	}
	l.blocks.SetBlock(x, y, z, block.Copy())
	l.solid++
}

// slice is a level read back: one character a voxel at one height, so what a
// script drew can be looked at without starting the game. The ruler down the
// side and along the top is what makes a coordinate in it usable as a
// coordinate in the script.
func slice(blocks *voxel.Map, y int32) string {
	bounds := blocks.Bounds()
	if y < 0 || y >= bounds.Y {
		return fmt.Sprintf("y=%d is outside the level (0..%d)\n", y, bounds.Y-1)
	}
	header := fmt.Sprintf("y=%d  %dx%d  # is solid, . is air, x runs right, z runs down", y, bounds.X, bounds.Z)
	return plane(header, bounds.X, 0, bounds.Z-1, func(x, z int32) bool { return blocks.IsSolidBlockAt(x, y, z) })
}

// sideSlice is the same read back standing up: the x-y plane at one z, top of
// the level first, which is the one view of a tall level that shows its floors.
func sideSlice(blocks *voxel.Map, z int32) string {
	bounds := blocks.Bounds()
	if z < 0 || z >= bounds.Z {
		return fmt.Sprintf("z=%d is outside the level (0..%d)\n", z, bounds.Z-1)
	}
	header := fmt.Sprintf("z=%d  %dx%d  # is solid, . is air, x runs right, y runs up", z, bounds.X, bounds.Y)
	return plane(header, bounds.X, bounds.Y-1, 0, func(x, y int32) bool { return blocks.IsSolidBlockAt(x, y, z) })
}

// plane prints rows first to last with an x ruler along the top and the row's
// own coordinate down the side.
func plane(header string, width, first, last int32, solid func(x, row int32) bool) string {
	var out strings.Builder
	out.WriteString(header + "\n     ")
	for x := int32(0); x < width; x++ {
		if x%10 == 0 {
			out.WriteString(strconv.Itoa(int(x / 10 % 10)))
		} else {
			out.WriteByte(' ')
		}
	}
	out.WriteByte('\n')

	step := int32(1)
	if last < first {
		step = -1
	}
	for row := first; row != last+step; row += step {
		fmt.Fprintf(&out, "%4d ", row)
		for x := int32(0); x < width; x++ {
			if solid(x, row) {
				out.WriteByte('#')
			} else {
				out.WriteByte('.')
			}
		}
		out.WriteByte('\n')
	}
	return out.String()
}

// ---------------------------------------------------------------- numbers ---

func ints(args []string) ([]int32, error) {
	numbers := make([]int32, len(args))
	for i, arg := range args {
		number, err := strconv.ParseInt(arg, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", arg)
		}
		numbers[i] = int32(number)
	}
	return numbers, nil
}

func floats(args []string) ([]float32, error) {
	numbers := make([]float32, len(args))
	for i, arg := range args {
		number, err := strconv.ParseFloat(arg, 32)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", arg)
		}
		numbers[i] = float32(number)
	}
	return numbers, nil
}

func ints3(args []string, from int) ([3]int32, error) {
	if len(args) < from+3 {
		return [3]int32{}, fmt.Errorf("want three whole numbers from argument %d", from+1)
	}
	numbers, err := ints(args[from : from+3])
	if err != nil {
		return [3]int32{}, err
	}
	return [3]int32{numbers[0], numbers[1], numbers[2]}, nil
}

func floats3(args []string, from int) (mgl32.Vec3, error) {
	if len(args) < from+3 {
		return mgl32.Vec3{}, fmt.Errorf("want three numbers from argument %d", from+1)
	}
	numbers, err := floats(args[from : from+3])
	if err != nil {
		return mgl32.Vec3{}, err
	}
	return mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, nil
}

func float(args []string, at int) (float32, error) {
	if len(args) <= at {
		return 0, fmt.Errorf("want a number")
	}
	numbers, err := floats(args[at : at+1])
	if err != nil {
		return 0, err
	}
	return numbers[0], nil
}

func vec3(value string) (mgl32.Vec3, error) {
	numbers, err := floats(strings.Split(value, ","))
	if err != nil {
		return mgl32.Vec3{}, err
	}
	if len(numbers) != 3 {
		return mgl32.Vec3{}, fmt.Errorf("%q is not X,Y,Z", value)
	}
	return mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, nil
}

// vec3List reads a route: a semicolon-separated list of X,Y,Z points, one leg
// an entry, in the order they are walked - what `entity`'s patrol= wants.
func vec3List(value string) ([]mgl32.Vec3, error) {
	parts := strings.Split(value, ";")
	points := make([]mgl32.Vec3, len(parts))
	for i, part := range parts {
		point, err := vec3(part)
		if err != nil {
			return nil, err
		}
		points[i] = point
	}
	return points, nil
}

// facing reads a direction a piece is turned to: X,Z for the ordinary case of
// something standing on the floor, or X,Y,Z for one turned out of that plane -
// facing=0,-1,0 lays a slide door down as a floor hatch.
func facing(value string) (mgl32.Vec3, error) {
	numbers, err := floats(strings.Split(value, ","))
	if err != nil {
		return mgl32.Vec3{}, err
	}
	switch len(numbers) {
	case 2:
		return mgl32.Vec3{numbers[0], 0, numbers[1]}, nil
	case 3:
		return mgl32.Vec3{numbers[0], numbers[1], numbers[2]}, nil
	}
	return mgl32.Vec3{}, fmt.Errorf("%q is not X,Z or X,Y,Z", value)
}

func vec2(value string) (mgl32.Vec2, error) {
	numbers, err := floats(strings.Split(value, ","))
	if err != nil {
		return mgl32.Vec2{}, err
	}
	if len(numbers) != 2 {
		return mgl32.Vec2{}, fmt.Errorf("%q is not X,Z", value)
	}
	return mgl32.Vec2{numbers[0], numbers[1]}, nil
}
