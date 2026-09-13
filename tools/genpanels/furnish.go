package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
)

// The ark's furnishings: the plain, built-in things a ship full of people
// needs and that nothing imported from System Shock or Blake Stone is -
// pipes along a ceiling, bunks, lockers, mess tables, pews, crates, the
// barricades the war welded together out of deck plate. Each is a handful of
// boxes, textured out of the same texture library the level's walls are, so
// they sit in the ark rather than on top of it.
//
// They are written to assets/meshes/ark/, a scenery folder each, so the
// editor's palette has them and a level places them like a table:
// `entity - mesh=ark/bunk at=X,Y,Z facing=DX,DZ`.
//
// Like the panels, every piece looks along -Z as modelled and stands on its
// own origin. A piece meant to hang on a wall (the pipes, the duct) is pushed
// to its tile's +Z edge the way genladder pushes the ladder, so `at` is a
// tile's centre and `facing` says which wall.

// part is one box of a piece, in world units where a voxel is 1, and which of
// the piece's textures it wears; front, if not -1, is a different texture for
// the face looking along -Z - a locker's door, a crate's stencil.
type part struct {
	min, max [3]float32
	tile     int
	front    int
}

type furnishing struct {
	name  string
	label string
	// tiles are paths under assets/textures, or "#rrggbb" for a flat,
	// slightly noisy colour: a blanket, a rubber seal.
	tiles []string
	parts []part
}

// box is a part wearing the same texture all round.
func b(x0, y0, z0, x1, y1, z1 float32, tile int) part {
	return part{[3]float32{x0, y0, z0}, [3]float32{x1, y1, z1}, tile, -1}
}

// bf is a part with a different texture on its front.
func bf(x0, y0, z0, x1, y1, z1 float32, tile, front int) part {
	return part{[3]float32{x0, y0, z0}, [3]float32{x1, y1, z1}, tile, front}
}

func furnishings() []furnishing {
	var pieces []furnishing

	// A run of three water pipes on brackets, high on the wall at the tile's
	// +Z edge, one voxel long so a corridor of it is `step times`.
	pipes := furnishing{name: "pipes", label: "Pipe Run",
		tiles: []string{"Space/_original/METAL_2A.PNG", "Space/_original/STEEL_2A.PNG", "#8a2f22"}}
	for i, y := range []float32{0.93, 0.86, 0.79} {
		tile := 0
		if i == 2 {
			tile = 2 // the hot line, painted red
		}
		pipes.parts = append(pipes.parts, b(-0.5, y-0.025, 0.43-float32(i)*0.005, 0.5, y+0.025, 0.48-float32(i)*0.005, tile))
	}
	for _, x := range []float32{-0.25, 0.25} {
		pipes.parts = append(pipes.parts, b(x-0.015, 0.75, 0.47, x+0.015, 0.97, 0.5, 1))
	}
	pieces = append(pieces, pipes)

	// An air duct, square, under the ceiling against the wall.
	pieces = append(pieces, furnishing{name: "duct", label: "Air Duct",
		tiles: []string{"Space/_original/ZINC_1A.PNG", "Space/_original/VENT_1A.PNG"},
		parts: []part{
			b(-0.5, 0.8, 0.3, 0.5, 0.98, 0.5, 0),
			bf(-0.12, 0.81, 0.29, 0.12, 0.97, 0.3, 1, 1),
		}})

	// A service-deck bunk: two berths in a steel frame, 1.8m long, and the
	// top one well under the one-voxel ceiling.
	bunk := furnishing{name: "bunk", label: "Bunk",
		tiles: []string{"Space/_original/STEEL_1B.PNG", "#4b5563", "#b9b4a2"}}
	for _, x := range []float32{-0.2, 0.18} {
		for _, z := range []float32{-0.45, 0.43} {
			bunk.parts = append(bunk.parts, b(x, 0, z, x+0.02, 0.82, z+0.02, 0))
		}
	}
	for _, y := range []float32{0.12, 0.52} {
		bunk.parts = append(bunk.parts,
			b(-0.2, y, -0.45, 0.2, y+0.03, 0.45, 0),        // the pan
			b(-0.19, y+0.03, -0.44, 0.19, y+0.09, 0.44, 1), // mattress and blanket
			b(-0.17, y+0.09, 0.3, 0.17, y+0.12, 0.42, 2),   // pillow
		)
	}
	pieces = append(pieces, bunk)

	// A tall personal locker, the door on its front.
	pieces = append(pieces, furnishing{name: "locker", label: "Locker",
		tiles: []string{"Space/_original/STEEL_2B.PNG", "Space/_original/SHUTTER_1A.PNG"},
		parts: []part{bf(-0.15, 0, -0.13, 0.15, 0.9, 0.13, 0, 1)}})

	// A row of three lockers, for a wall of them.
	lockers := furnishing{name: "lockers", label: "Locker Row",
		tiles: []string{"Space/_original/STEEL_2B.PNG", "Space/_original/SHUTTER_1A.PNG"}}
	for i := -1; i <= 1; i++ {
		x := float32(i) * 0.31
		lockers.parts = append(lockers.parts, bf(x-0.15, 0, -0.13, x+0.15, 0.9, 0.13, 0, 1))
	}
	pieces = append(pieces, lockers)

	// A mess table with its benches bolted to the deck, seats for eight.
	pieces = append(pieces, furnishing{name: "mess_table", label: "Mess Table",
		tiles: []string{"Space/_original/METAL_1B.PNG", "Space/_original/STEEL_1A.PNG"},
		parts: []part{
			b(-0.6, 0.36, -0.22, 0.6, 0.39, 0.22, 0),
			b(-0.03, 0, -0.03, 0.03, 0.36, 0.03, 1),
			b(-0.5, 0, -0.03, -0.44, 0.36, 0.03, 1),
			b(0.44, 0, -0.03, 0.5, 0.36, 0.03, 1),
			b(-0.6, 0.2, -0.42, 0.6, 0.23, -0.3, 0),
			b(-0.6, 0.2, 0.3, 0.6, 0.23, 0.42, 0),
			b(-0.4, 0, -0.38, -0.36, 0.2, -0.34, 1),
			b(0.36, 0, -0.38, 0.4, 0.2, -0.34, 1),
			b(-0.4, 0, 0.34, -0.36, 0.2, 0.38, 1),
			b(0.36, 0, 0.34, 0.4, 0.2, 0.38, 1),
		}})

	// A pew, or a waiting-room bench: seat and back, facing -Z.
	pieces = append(pieces, furnishing{name: "pew", label: "Pew",
		tiles: []string{"Non-Space/WOOD_1B.PNG", "Space/_original/STEEL_1A.PNG"},
		parts: []part{
			b(-0.55, 0.2, -0.1, 0.55, 0.24, 0.12, 0),
			b(-0.55, 0.24, 0.1, 0.55, 0.5, 0.13, 0),
			b(-0.5, 0, -0.05, -0.46, 0.2, 0.1, 1),
			b(0.46, 0, -0.05, 0.5, 0.2, 0.1, 1),
		}})

	// Cargo crates, one a metre square and one two, stencilled on the front.
	pieces = append(pieces, furnishing{name: "crate", label: "Cargo Crate",
		tiles: []string{"Space/_original/CRATE_1B.PNG", "Space/_original/CRATE_1A.PNG"},
		parts: []part{bf(-0.25, 0, -0.25, 0.25, 0.5, 0.25, 0, 1)}})
	pieces = append(pieces, furnishing{name: "crate_big", label: "Cargo Container",
		tiles: []string{"Space/_original/CRATE_2B.PNG", "Space/_original/CRATE_2A.PNG"},
		parts: []part{bf(-0.45, 0, -0.45, 0.45, 0.9, 0.45, 0, 1)}})

	// A barricade: deck plates and a door leaf welded upright on angle iron,
	// what either side of the war put across a corridor.
	pieces = append(pieces, furnishing{name: "barricade", label: "Barricade",
		tiles: []string{"Space/_original/RUSTY_1A.PNG", "Space/_original/PANEL_2A.PNG", "Space/_original/WARN_2A.PNG"},
		parts: []part{
			b(-0.5, 0, -0.03, -0.05, 0.62, 0.02, 0),
			b(-0.1, 0, -0.06, 0.32, 0.5, -0.02, 1),
			b(0.25, 0, -0.02, 0.5, 0.7, 0.03, 0),
			b(-0.35, 0.3, -0.09, 0.2, 0.42, -0.06, 2),
			b(-0.45, 0, 0.02, -0.4, 0.3, 0.25, 1),
			b(0.35, 0, 0.03, 0.4, 0.3, 0.25, 1),
		}})

	// A ration dispenser: the one machine every deck had, with a slot and a
	// screen on its front.
	pieces = append(pieces, furnishing{name: "dispenser", label: "Ration Dispenser",
		tiles: []string{"Space/_original/METAL_3A.PNG", "shock/130_automatic_teller_machine.png"},
		parts: []part{bf(-0.22, 0, -0.16, 0.22, 0.9, 0.16, 0, 1)}})

	// A cryo pod for cold storage, standing, the glass on its front.
	pieces = append(pieces, furnishing{name: "cryo_pod", label: "Cryo Pod",
		tiles: []string{"Space/_original/LAB_1A.PNG", "shock/045_healing_incubator.png"},
		parts: []part{
			b(-0.25, 0, -0.2, 0.25, 0.08, 0.22, 0),
			bf(-0.22, 0.08, -0.18, 0.22, 0.88, 0.2, 0, 1),
			b(-0.25, 0.88, -0.2, 0.25, 0.95, 0.22, 0),
		}})

	// A washing machine: a box with a round door drawn on its front.
	pieces = append(pieces, furnishing{name: "washer", label: "Washer",
		tiles: []string{"Space/_original/LAB_2A.PNG", "washer"},
		parts: []part{bf(-0.2, 0, -0.2, 0.2, 0.42, 0.2, 0, 1)}})

	// A water tank's valve wheel and gauge, stood on the deck against a tank.
	pieces = append(pieces, furnishing{name: "valve", label: "Valve Stand",
		tiles: []string{"Space/_original/STEEL_1C.PNG", "#a12a1c", "Space/_original/SWITCH_1A.PNG"},
		parts: []part{
			b(-0.04, 0, -0.04, 0.04, 0.5, 0.04, 0),
			b(-0.14, 0.5, -0.08, 0.14, 0.52, -0.06, 1),
			b(-0.02, 0.37, -0.14, 0.02, 0.65, -0.12, 1),
			b(-0.14, 0.5, -0.14, 0.14, 0.52, -0.12, 1),
			bf(-0.08, 0.2, -0.07, 0.08, 0.34, -0.04, 0, 2),
		}})

	return pieces
}

// writeFurnishings writes every piece and its scenery.json.
func writeFurnishings() {
	for _, piece := range furnishings() {
		atlas, rects := furnishingAtlas(piece.tiles)
		var positions, normals [][3]float32
		var uvs [][2]float32
		var indices []uint16
		for _, p := range piece.parts {
			front := p.tile
			if p.front >= 0 {
				front = p.front
			}
			texturedBox(p.min, p.max, rects[p.tile], rects[front], &positions, &normals, &uvs, &indices)
		}
		name := "ark/" + piece.name
		write(name, encode(atlas), positions, normals, uvs, indices)
		json := fmt.Sprintf("{\n  \"name\": %s\n}\n", strconv.Quote(piece.label))
		if err := os.WriteFile(filepath.Join("assets", "meshes", name, "scenery.json"), []byte(json), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}

// furnishingAtlas lays a piece's textures out in a grid of 128-pixel cells and
// says which UV rectangle each one is, inset half a texel from its edges so
// nearest sampling never reads the neighbour.
func furnishingAtlas(tiles []string) (*image.NRGBA, [][4]float32) {
	const cell = 128
	columns := int(math.Ceil(math.Sqrt(float64(len(tiles)))))
	size := columns * cell
	atlas := image.NewNRGBA(image.Rect(0, 0, size, size))
	var rects [][4]float32
	for i, tile := range tiles {
		x, y := i%columns*cell, i/columns*cell
		area := image.Rect(x, y, x+cell, y+cell)
		drawTile(atlas, area, tile, int64(i))
		inset := float32(1)
		rects = append(rects, [4]float32{
			(float32(x) + inset) / float32(size), (float32(y) + inset) / float32(size),
			(float32(x+cell) - inset) / float32(size), (float32(y+cell) - inset) / float32(size),
		})
	}
	return atlas, rects
}

func drawTile(atlas *image.NRGBA, area image.Rectangle, tile string, seed int64) {
	random := rand.New(rand.NewSource(seed + 5))
	switch {
	case tile == "washer":
		for py := area.Min.Y; py < area.Max.Y; py++ {
			for px := area.Min.X; px < area.Max.X; px++ {
				dx, dy := float64(px-area.Min.X-64), float64(py-area.Min.Y-70)
				r := math.Hypot(dx, dy)
				shade := uint8(200 + random.Intn(12))
				c := color.NRGBA{shade, shade, shade - 4, 255}
				switch {
				case r < 34:
					c = color.NRGBA{40, 52, 60, 255}
				case r < 42:
					c = color.NRGBA{120, 124, 128, 255}
				case py-area.Min.Y < 20:
					c = color.NRGBA{150, 154, 160, 255}
				}
				atlas.SetNRGBA(px, py, c)
			}
		}
	case tile[0] == '#':
		value, err := strconv.ParseUint(tile[1:], 16, 32)
		if err != nil {
			log.Fatalf("colour %q: %v", tile, err)
		}
		base := color.NRGBA{uint8(value >> 16), uint8(value >> 8), uint8(value), 255}
		for py := area.Min.Y; py < area.Max.Y; py++ {
			for px := area.Min.X; px < area.Max.X; px++ {
				n := random.Intn(14) - 7
				atlas.SetNRGBA(px, py, color.NRGBA{clamp(int(base.R) + n), clamp(int(base.G) + n), clamp(int(base.B) + n), 255})
			}
		}
	default:
		file, err := os.Open(filepath.Join("assets", "textures", tile))
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		picture, _, err := image.Decode(file)
		if err != nil {
			log.Fatalf("%s: %v", tile, err)
		}
		// Nearest-neighbour up or down to the cell, the way the level draws it.
		bounds := picture.Bounds()
		for py := area.Min.Y; py < area.Max.Y; py++ {
			for px := area.Min.X; px < area.Max.X; px++ {
				sx := bounds.Min.X + (px-area.Min.X)*bounds.Dx()/area.Dx()
				sy := bounds.Min.Y + (py-area.Min.Y)*bounds.Dy()/area.Dy()
				atlas.Set(px, py, picture.At(sx, sy))
			}
		}
	}
}

func clamp(v int) uint8 { return uint8(max(0, min(255, v))) }

// texturedBox is box with a texture rectangle per face: front on the face
// looking along -Z, side on the other five.
func texturedBox(min, max [3]float32, side, front [4]float32,
	positions, normals *[][3]float32, uvs *[][2]float32, indices *[]uint16) {
	corners := [8][3]float32{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]},
		{max[0], min[1], max[2]}, {min[0], min[1], max[2]},
		{min[0], max[1], min[2]}, {max[0], max[1], min[2]},
		{max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	faces := []face{
		{[4]int{7, 3, 2, 6}, [3]float32{0, 0, 1}, side},
		{[4]int{5, 1, 0, 4}, [3]float32{0, 0, -1}, front},
		{[4]int{6, 2, 1, 5}, [3]float32{1, 0, 0}, side},
		{[4]int{4, 0, 3, 7}, [3]float32{-1, 0, 0}, side},
		{[4]int{4, 7, 6, 5}, [3]float32{0, 1, 0}, side},
		{[4]int{3, 0, 1, 2}, [3]float32{0, -1, 0}, side},
	}
	for _, f := range faces {
		base := uint16(len(*positions))
		for _, c := range f.corner {
			*positions = append(*positions, corners[c])
			*normals = append(*normals, f.normal)
		}
		u0, v0, u1, v1 := f.uv[0], f.uv[1], f.uv[2], f.uv[3]
		*uvs = append(*uvs, [2]float32{u0, v0}, [2]float32{u0, v1}, [2]float32{u1, v1}, [2]float32{u1, v0})
		*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)
	}
}
