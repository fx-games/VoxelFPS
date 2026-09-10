// Command mapgen builds a level out of a text file.
//
// A level on disk is two files nothing can hand-edit: map.bin is gzipped
// chunks, map.json is a few hundred lines of generated JSON. Everything that
// wants to make or change a level has therefore had to be the running editor,
// with a mouse in it. This is the other door: build.map, a plain text file of
// one command a line, next to the level it describes.
//
//	go run ./tools/mapgen -map station          # build.map -> map.bin + map.json
//	go run ./tools/mapgen -map station -show 15 # what is solid at height 15
//	go run ./tools/mapgen -help                 # every command, with its arguments
//
// The text is the source and the two files are the output, the same way
// tools/genstairs owns the stairs it draws. Editing a built level in the game
// and saving writes map.bin back, and the next build overwrites it - so a level
// is authored in one place or the other, not both.
//
// Commands, one a line, # to end of line is a comment, blank lines ignored.
// Positional arguments first, then any number of key=value options.
//
//	size W H D                     the level's own size in voxels
//	spawn X Y Z [dir=DX,DZ]        where the player starts, in world units
//	block NAME T [T T]             name a kind of block: wall, floor, ceiling
//	                               textures by atlas slot, repeating the last
//	fill NAME x0 y0 z0 x1 y1 z1    make an inclusive box of it solid
//	clear x0 y0 z0 x1 y1 z1        make an inclusive box air
//	cyl NAME cx cz r y0 y1         a solid upright cylinder
//	tube NAME cx cz rOuter rInner y0 y1   the wall of one, hollow inside
//	glass cx cz rOuter rInner y     window panes filling the ring a `tube` of
//	                               the same numbers would have walled: they are
//	                               see-through to look at and wall to walk into
//	entity NAME at=X,Y,Z [mesh=M] [facing=DX,DZ] [collider=false] [code=NNNN]
//	                               code is the four digits a keypad
//	                               (mesh=keypad) asks for; the door it holds
//	                               shut is whichever one is nearest to it
//	note X Y Z THE WRITING         a sheet of paper on the floor. Everything
//	                               after the three coordinates is what it says,
//	                               spaces and all, and any four digits in it is
//	                               a code the player learns by reading it
//	log X Y Z CUE THE LABEL        an audio log on the floor: a cartridge with a
//	                               recording on it. CUE names the file under
//	                               assets/audio/logs without its extension, and
//	                               the rest of the line is the label on its lid
//	light X Y Z [color=R,G,B] [power=N] [baked] [yaw=N] [pitch=N]
//	                               [angle=N] [reach=N]
//	sky full                       no horizon: the cubemap covers the sphere
//	spin DEG                       degrees a second the sky turns, about Y
//	pressure CX CZ R Y0 Y1         the cylinder that has air and gravity in it
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
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

const (
	mapDir        = "assets/maps"
	scriptName    = "build.map"
	voxelFileName = "map.bin"
	mapFileName   = "map.json"
	// chunkSize is the edge of a chunk on every axis, and has to be the engine's
	// own (see map_build.go's levelChunkSize) or a level built here loads at the
	// wrong size.
	chunkSize = 32
)

func main() {
	name := flag.String("map", "", "level folder under "+mapDir)
	show := flag.Int("show", -1, "instead of building, print what is solid at this height")
	flag.Parse()

	if *name == "" {
		flag.Usage()
		os.Exit(2)
	}
	dir := filepath.Join(mapDir, *name)

	if *show >= 0 {
		blocks := voxel.NewMapWithEmptyChunks(1, 1, 1, chunkSize, chunkSize)
		blocks.LoadFromDisk(filepath.Join(dir, voxelFileName))
		fmt.Print(slice(blocks, int32(*show)))
		return
	}

	script, err := os.ReadFile(filepath.Join(dir, scriptName))
	if err != nil {
		fatal(err)
	}
	level, err := build(string(script))
	if err != nil {
		fatal(err)
	}
	if err := level.blocks.SaveToDisk(filepath.Join(dir, voxelFileName)); err != nil {
		fatal(err)
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
}

// build runs a whole script. Nothing is written until it has run to the end, so
// a script with a typo on its last line leaves the level it was rebuilding
// exactly as it was.
func build(script string) (*level, error) {
	l := &level{
		kinds: map[string]*voxel.Block{},
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
	return l, scanner.Err()
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
			slot, err := strconv.ParseUint(arg, 10, 16)
			if err != nil {
				return fmt.Errorf("texture slot %q: %w", arg, err)
			}
			textures = append(textures, uint16(slot))
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
		l.paint(box, far, block)

	case "cyl", "tube":
		kind, known := l.kinds[args[0]]
		if !known {
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
			Code:           options["code"],
		}
		if facing, given := options["facing"]; given {
			flat, err := vec2(facing)
			if err != nil {
				return err
			}
			entity.Facing = mgl32.Vec3{flat.X(), 0, flat.Y()}
		}
		l.result.Entities = append(l.result.Entities, core.EntitySpawn{Position: pos, Entity: entity})

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
		l.result.Lights = append(l.result.Lights, light)

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

	case "pressure":
		numbers, err := floats(args)
		if err != nil {
			return err
		}
		if len(numbers) != 5 {
			return fmt.Errorf("want CX CZ RADIUS Y0 Y1")
		}
		station := l.station()
		station.CenterX, station.CenterZ, station.Radius = numbers[0], numbers[1], numbers[2]
		station.MinY, station.MaxY = numbers[3], numbers[4]

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
	var out strings.Builder
	fmt.Fprintf(&out, "y=%d  %dx%d  # is solid, . is air, x runs right, z runs down\n", y, bounds.X, bounds.Z)

	out.WriteString("     ")
	for x := int32(0); x < bounds.X; x++ {
		if x%10 == 0 {
			out.WriteString(strconv.Itoa(int(x / 10 % 10)))
		} else {
			out.WriteByte(' ')
		}
	}
	out.WriteByte('\n')

	for z := int32(0); z < bounds.Z; z++ {
		fmt.Fprintf(&out, "%4d ", z)
		for x := int32(0); x < bounds.X; x++ {
			if blocks.IsSolidBlockAt(x, y, z) {
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
