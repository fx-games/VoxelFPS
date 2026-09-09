package quads

import (
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/memmaker/goray/quads/voxel"
)

// Baking an atlas renumbers every texture in it, and the map has to be moved onto
// the new numbers in the same breath. Get that wrong and nothing errors - every
// wall in the level just quietly draws a different texture.
//
// So this walks the whole round trip without a GL context: paint blocks with
// scattered slots, pack the sheet down to the slots that were used, remap, and
// check every side still lands on the pixels it started on. The tile the block
// pointed at is identified by its colour, which is what makes "same texture"
// checkable rather than "same number".
func TestBakingKeepsEveryBlockOnItsOwnTexture(t *testing.T) {
	const (
		columns = 4
		tile    = 2
		rows    = 4
	)

	// A sheet where every slot is a flat colour, so a tile identifies itself.
	sheet := image.NewRGBA(image.Rect(0, 0, columns*tile, rows*tile))
	for slot := 0; slot < columns*rows; slot++ {
		shade := uint8(10 + slot*5)
		fillTile(sheet, tileRect(slot, columns, tile, tile), shade)
	}

	m := voxel.NewMapWithEmptyChunks(1, 1, 1, 2, 2)
	// Three blocks, sharing some sides, using slots scattered across the sheet so
	// the packing has to actually move things and close gaps.
	placed := map[voxel.Int3]map[voxel.CubeSide]uint16{
		{X: 0, Y: 0, Z: 0}: {voxel.Top: 13, voxel.Bottom: 2, voxel.North: 2, voxel.South: 7, voxel.East: 13, voxel.West: 0},
		{X: 1, Y: 0, Z: 0}: {voxel.Top: 7, voxel.Bottom: 7, voxel.North: 15, voxel.South: 7, voxel.East: 7, voxel.West: 7},
		{X: 1, Y: 1, Z: 1}: {voxel.Top: 4, voxel.Bottom: 4, voxel.North: 4, voxel.South: 4, voxel.East: 4, voxel.West: 4},
	}
	wantColor := make(map[voxel.Int3]map[voxel.CubeSide]uint8)
	for pos, sides := range placed {
		m.SetBlock(pos.X, pos.Y, pos.Z, voxel.NewBlockWithSides(sides))
		wantColor[pos] = make(map[voxel.CubeSide]uint8)
		for side, slot := range sides {
			wantColor[pos][side] = tileShade(sheet, tileRect(int(slot), columns, tile, tile))
		}
	}
	m.SetEmissiveTextures([]uint16{7, 11}) // 11 is on no block, and must not survive

	used := m.UsedTextureIndices()
	if got, want := len(used), 6; got != want {
		t.Fatalf("distinct textures in use: got %d (%v) want %d", got, used, want)
	}

	// Pack, the way Bake does, then move the map onto the packed slots.
	packed := image.NewRGBA(sheet.Bounds())
	remap := make(map[uint16]uint16, len(used))
	for slot, old := range used {
		remap[old] = uint16(slot)
		draw.Draw(packed, tileRect(slot, columns, tile, tile),
			sheet, tileRect(int(old), columns, tile, tile).Min, draw.Src)
	}
	m.RemapTextures(remap)

	for pos, sides := range wantColor {
		block := m.GetBlockFromInt3(pos)
		for side, want := range sides {
			slot := block.GetTexture(side)
			if int(slot) >= len(used) {
				t.Fatalf("%v side %v points past the baked atlas: slot %d of %d", pos, side, slot, len(used))
			}
			if got := tileShade(packed, tileRect(int(slot), columns, tile, tile)); got != want {
				t.Errorf("%v side %v: baked slot %d holds shade %d, want %d", pos, side, slot, got, want)
			}
		}
	}

	if m.EmissiveTextureCount() != 1 {
		t.Errorf("emissive set after baking: %d entries, want just the one still in use", m.EmissiveTextureCount())
	}
}

func fillTile(img *image.RGBA, rect image.Rectangle, shade uint8) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			offset := img.PixOffset(x, y)
			img.Pix[offset], img.Pix[offset+1], img.Pix[offset+2], img.Pix[offset+3] = shade, shade, shade, 255
		}
	}
}

func tileShade(img *image.RGBA, rect image.Rectangle) uint8 {
	return img.Pix[img.PixOffset(rect.Min.X, rect.Min.Y)]
}

// The browser turns a directory into picker cells, and the Path it hands back is
// what a texture is fetched and recorded by. A path composed wrong is not an
// error, it is a wall full of magenta placeholders - so this pins the two things
// that go wrong on a subdirectory: what the ".." row points at, and whether a
// nested file keeps its parent in front of it.
func TestBrowsingASubdirectoryKeepsPathsResolvable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Space/blue/WALL_1A.PNG", "Space/AAA.png", "TOP.png", "notes.txt"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	rootEntries := readTexDir(root, "")
	if got := names(rootEntries); !slices.Equal(got, []string{"Space", "TOP.png"}) {
		t.Fatalf("root listing: got %v, want the folder first and the .txt left out", got)
	}
	if !rootEntries[0].IsDir || rootEntries[0].Path != "Space" {
		t.Fatalf("folder cell: got %+v", rootEntries[0])
	}

	nested := readTexDir(root, "Space/blue")
	if got := names(nested); !slices.Equal(got, []string{"..", "WALL_1A.PNG"}) {
		t.Fatalf("nested listing: got %v", got)
	}
	if up := nested[0]; !up.IsDir || up.Path != "Space" {
		t.Errorf("the .. row should walk to Space, got %q", up.Path)
	}
	if got, want := nested[1].Path, "Space/blue/WALL_1A.PNG"; got != want {
		t.Errorf("nested file path: got %q want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(nested[1].Path))); err != nil {
		t.Errorf("the path handed out does not resolve back to a file: %v", err)
	}

	// One level up from a top-level folder is the root, not "." next to it.
	if up := readTexDir(root, "Space")[0]; up.Path != "" {
		t.Errorf("the .. row out of a top-level folder should be the root, got %q", up.Path)
	}
}

func names(entries []TexEntry) []string {
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.Name
	}
	return got
}
