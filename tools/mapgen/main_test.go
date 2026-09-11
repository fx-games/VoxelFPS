package main

import (
	"os"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/voxel"
)

// One script through the whole of build: the two shapes that are not boxes, the
// options parser, and the hull the gameplay switch reads. A level that comes out
// of this wrong is a level nothing in the game can tell you is wrong.
func TestAScriptBuildsWhatItSays(t *testing.T) {
	built, err := build(`
size 32 16 32
block wall 20 21 22
cyl wall 16 16 6 4 4
tube wall 16 16 6 5 5 6
clear 16 5 10 16 6 10
spawn 16.5 5 16.5 dir=0,-1
pressure 16 16 6 4 8
spin 6
light 16.5 6.5 16.5 color=1,0.5,0.25 power=9 baked
entity SlideDoor mesh=door_slide at=16.5,5,10.5
`)
	if err != nil {
		t.Fatal(err)
	}

	// The floor is a full disc at y=4: solid at the middle and at the rim,
	// nothing a voxel past it.
	for _, at := range [][3]int32{{16, 4, 16}, {16, 4, 22}, {22, 4, 16}} {
		if !built.blocks.IsSolidBlockAt(at[0], at[1], at[2]) {
			t.Errorf("floor %v is not solid", at)
		}
	}
	if built.blocks.IsSolidBlockAt(16, 4, 23) {
		t.Error("the floor reaches past its own radius")
	}

	// The wall is a ring: solid at the rim, open inside it.
	if !built.blocks.IsSolidBlockAt(16, 5, 22) {
		t.Error("the wall is missing at the rim")
	}
	if built.blocks.IsSolidBlockAt(16, 5, 18) {
		t.Error("the tube filled its own middle")
	}
	// ...with the doorway cleared out of it again.
	if built.blocks.IsSolidBlockAt(16, 5, 10) {
		t.Error("clear left the doorway solid")
	}

	if got := built.result.PlayerDir; got.Y() != -1 {
		t.Errorf("dir=0,-1 became %v", got)
	}
	if got := built.result.Lights; len(got) != 1 || !got[0].Baked || got[0].Intensity != 9 {
		t.Errorf("light: %+v", got)
	}
	if got := built.result.Entities; len(got) != 1 || got[0].Entity.MeshName != "door_slide" {
		t.Errorf("entity: %+v", got)
	}

	// The hull is what decides where the suit comes on, so the deck has to be
	// inside it and the space a step past the wall outside it.
	station := built.result.Station
	if station == nil || station.Spin != 6 {
		t.Fatalf("station: %+v", station)
	}
	if !station.Pressurised(built.result.PlayerSpawn) {
		t.Error("the player spawns in vacuum")
	}
	if station.Pressurised(built.result.PlayerSpawn.Add([3]float32{0, 0, 9})) {
		t.Error("outside the hull is pressurised")
	}
}

// The glass has to close the ring a wall of the same numbers would have closed.
// It is drawn as quads and collides as the voxels those quads are derived back
// into (core.Pane.Cell), so a gap in it is a window you can fly in through, and
// a pane whose cell is outside the ring is a wall in the middle of the corridor.
func TestTheGlassClosesTheRingTheWallLeftOpen(t *testing.T) {
	walled, err := build(`
size 32 16 32
block wall 20
tube wall 16 16 8 7 4 4
`)
	if err != nil {
		t.Fatal(err)
	}
	glazed, err := build(`
size 32 16 32
glass 16 16 8 7 4
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(glazed.result.Panes) == 0 {
		t.Fatal("no panes")
	}

	glass := map[core.Int3]bool{}
	for _, pane := range glazed.result.Panes {
		cell := pane.Cell()
		if cell.Y != 4 {
			t.Fatalf("pane %v sits at height %d, not 4", pane.Position, cell.Y)
		}
		if !walled.blocks.IsSolidBlockAt(cell.X, cell.Y, cell.Z) {
			t.Errorf("pane %v fills %v, which is not in the ring", pane.Position, cell)
		}
		glass[cell] = true
	}

	// Flood in from a corner with only the glass in the way. Four-connected,
	// because two cells meeting at a corner leave a gap of no width and nothing
	// with a box around it gets through one.
	reached := map[core.Int3]bool{}
	queue := []core.Int3{{X: 0, Y: 4, Z: 0}}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if at.X < 0 || at.X > 31 || at.Z < 0 || at.Z > 31 || reached[at] || glass[at] {
			continue
		}
		reached[at] = true
		queue = append(queue,
			core.Int3{X: at.X + 1, Y: 4, Z: at.Z}, core.Int3{X: at.X - 1, Y: 4, Z: at.Z},
			core.Int3{X: at.X, Y: 4, Z: at.Z + 1}, core.Int3{X: at.X, Y: 4, Z: at.Z - 1})
	}
	if reached[core.Int3{X: 16, Y: 4, Z: 16}] {
		t.Error("the glass has a gap: outside reaches the middle of the ring")
	}

	// Nothing solid may be invisible. A pane collides as the whole voxel it
	// fills, so every face of a glass voxel that borders something you can stand
	// in has to have a pane drawn on it, or that face is an invisible wall.
	drawn := map[[2]core.Int3]bool{}
	for _, pane := range glazed.result.Panes {
		normal := core.Int3{
			X: int32(pane.Normal.X()), Y: int32(pane.Normal.Y()), Z: int32(pane.Normal.Z()),
		}
		drawn[[2]core.Int3{pane.Cell(), normal}] = true
	}
	for cell := range glass {
		for _, side := range []core.Int3{{X: 1}, {X: -1}, {Z: 1}, {Z: -1}} {
			beyond := core.Int3{X: cell.X + side.X, Y: cell.Y, Z: cell.Z + side.Z}
			if glass[beyond] || drawn[[2]core.Int3{cell, side}] {
				continue
			}
			t.Errorf("the %v face of the glass at %v is solid and not drawn", side, cell)
		}
	}
}

// A tapering hull, the pressure test tapering with it, textures by name, and a
// blast that blows the same hole every build - a rebuild that moved a hole
// would move every room cut next to it.
func TestTheConeAndItsDamage(t *testing.T) {
	script := `
size 40 40 40
tex plate Space/_original/METAL_1A.PNG
tex strip Space/_original/LIGHT_3A.PNG glow
block hull plate plate strip
cone hull 20 20 4 5 12 25 shell=1.5
pressure 20 20 3 5 25 11
blast 28 15 20 3 seed=7 scorch=hull
`
	// mapgen runs from the repository root, which is where a `tex` path is read.
	here, _ := os.Getwd()
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(here)

	built, err := build(script)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := build(script)

	if !built.blocks.IsSolidBlockAt(24, 5, 20) || built.blocks.IsSolidBlockAt(26, 5, 20) {
		t.Error("the cone's narrow end is not radius 4")
	}
	if !built.blocks.IsSolidBlockAt(32, 25, 20) || built.blocks.IsSolidBlockAt(20, 25, 20) {
		t.Error("the cone's wide end is not a radius-12 shell")
	}
	if got := built.kinds["hull"].GetTexture(voxel.Bottom); got != 1 || !built.textures[1].glow {
		t.Errorf("the ceiling texture is slot %d, want the glowing strip in slot 1", got)
	}

	station := built.result.Station
	if !station.Pressurised(mgl32.Vec3{30, 25, 20}) || station.Pressurised(mgl32.Vec3{30, 5, 20}) {
		t.Error("the pressure hull does not taper with the cone")
	}

	if built.blocks.IsSolidBlockAt(28, 15, 20) {
		t.Error("the blast left the wall it hit")
	}
	for x := int32(20); x < 36; x++ {
		for y := int32(8); y < 23; y++ {
			for z := int32(12); z < 28; z++ {
				if built.blocks.IsSolidBlockAt(x, y, z) != again.blocks.IsSolidBlockAt(x, y, z) {
					t.Fatalf("two builds blew different holes at %d,%d,%d", x, y, z)
				}
			}
		}
	}
}

func TestABadLineNamesItself(t *testing.T) {
	for _, script := range []string{
		"fill nosuchblock 0 0 0 1 1 1",
		"cyl wall 1 2 3",
		"entity Thing",
		"wobble 1 2 3",
	} {
		if _, err := build(script); err == nil {
			t.Errorf("%q built without complaint", script)
		} else if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%q: %v does not say which line", script, err)
		}
	}
}

// A room the flood cannot get out of is sealed, the same room with one hole in
// its wall is not, and a repeated fill lays down the row it says.
func TestTheLeakCheckFindsTheGap(t *testing.T) {
	room := `
size 16 8 16
block wall 20
pressure 8 8 7 1 6
cyl wall 8 8 5 1 1
tube wall 8 8 5 4 2 4
cyl wall 8 8 5 5 5
spawn 8.5 2 8.5
`
	sealed, err := build(room)
	if err != nil {
		t.Fatal(err)
	}
	if report, ok := leaks(sealed); !ok {
		t.Error(report)
	}
	holed, _ := build(room + "clear 12 2 8 13 2 8\n")
	if report, ok := leaks(holed); ok {
		t.Errorf("a hole in the wall was called sealed: %s", report)
	}

	row, err := build("size 16 4 16\nblock wall 20\nfill wall 0 0 0 0 0 0 step=3,0,0 times=4\n")
	if err != nil {
		t.Fatal(err)
	}
	for x := int32(0); x < 12; x++ {
		if want := x%3 == 0; row.blocks.IsSolidBlockAt(x, 0, 0) != want {
			t.Errorf("x=%d solid=%v, want %v", x, !want, want)
		}
	}
}
