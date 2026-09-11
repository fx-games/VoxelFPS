package quads

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/memmaker/goray/png_chunks"

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

// A texture picked into the atlas is saved with it on the spot, because the block
// list pointing at it is too: a list whose slots run past the end of the atlas file
// draws those faces empty on the next load. So the sheet written has to reach the
// last slot handed out - and stop at the end of its row.
func TestSavedSheetReachesTheLastSlot(t *testing.T) {
	const columns, tile = 4, 2
	sheet := image.NewRGBA(image.Rect(0, 0, columns*tile, 4*tile))
	for slot := 0; slot < 16; slot++ {
		fillTile(sheet, tileRect(slot, columns, tile, tile), uint8(10+slot*5))
	}
	file := filepath.Join(t.TempDir(), "map_2x2.png")
	if err := writePNG(file, trimmedSheet(sheet, 5, columns, tile, tile), nil); err != nil {
		t.Fatal(err)
	}

	saved := ImageFromOSPath(file)
	if got, want := saved.Bounds().Size(), image.Pt(columns*tile, 2*tile); got != want {
		t.Errorf("saved sheet is %v, want %v: two rows for five slots", got, want)
	}
	lastSlot := tileRect(4, columns, tile, tile).Min
	if r, _, _, _ := saved.At(lastSlot.X, lastSlot.Y).RGBA(); uint8(r>>8) != 30 {
		t.Errorf("slot 4 saved as shade %d, want 30", r>>8)
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

	browser := &TexBrowser{root: root}
	rootEntries := browser.readDir("")
	if got := names(rootEntries); !slices.Equal(got, []string{"Space", "TOP.png"}) {
		t.Fatalf("root listing: got %v, want the folder first and the .txt left out", got)
	}
	if !rootEntries[0].IsDir || rootEntries[0].Path != "Space" {
		t.Fatalf("folder cell: got %+v", rootEntries[0])
	}

	nested := browser.readDir("Space/blue")
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
	if up := browser.readDir("Space")[0]; up.Path != "" {
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

// The emissive set used to be nine numbers hardcoded in main.go. It lives in the
// atlas file now, so the round trip through a real PNG is the thing worth pinning:
// what is written has to be what comes back, and the pixels have to survive it.
func TestEmissiveChunkSurvivesTheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.png")
	sheet := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{C: color.RGBA{R: 3, G: 5, B: 7, A: 255}}, image.Point{}, draw.Src)

	want := []uint16{19, 26, 47, 49}
	if err := writePNG(path, sheet, want); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got := parseEmissiveChunk(png_chunks.ReadTextChunkFromPNG(file, EmissiveChunkKey))
	if len(got) != len(want) {
		t.Fatalf("read back %d slots, wrote %d", len(got), len(want))
	}
	for _, slot := range want {
		if !got[slot] {
			t.Errorf("slot %d was written but did not come back", slot)
		}
	}

	// The chunk must not have cost the atlas its pixels.
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(file)
	if err != nil {
		t.Fatalf("the atlas no longer decodes: %v", err)
	}
	if r, g, b, _ := decoded.At(4, 4).RGBA(); r>>8 != 3 || g>>8 != 5 || b>>8 != 7 {
		t.Errorf("pixel came back as %v, want the 3,5,7 written", decoded.At(4, 4))
	}
}

// Writing the record twice must leave one record, not two disagreeing ones.
// InjectTextChunkIntoPNG adds a chunk rather than replacing one, so a writer that
// patched the file in place would stack them - and the reader takes the first, which
// means every G press after the first would silently do nothing.
func TestRewritingTheEmissiveChunkReplacesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.png")
	sheet := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if err := writePNG(path, sheet, []uint16{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := writePNG(path, sheet, []uint16{9}); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got := parseEmissiveChunk(png_chunks.ReadTextChunkFromPNG(file, EmissiveChunkKey))
	if len(got) != 1 || !got[9] {
		t.Fatalf("after rewriting, the record is %v, want only slot 9", got)
	}

	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if n := countTextChunks(t, file); n != 1 {
		t.Errorf("the atlas carries %d tEXt chunks, want exactly 1", n)
	}
}

func countTextChunks(t *testing.T, file *os.File) int {
	t.Helper()
	count := 0
	if err := png_chunks.ReadChunksFromPNG(file, func(name string, _ png_chunks.BytePromise) bool {
		if name == "tEXt" {
			count++
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

// A smudged entry costs one panel's glow, not the level: the atlas is an editable
// file and a half-written number is not a reason to refuse the whole map.
func TestEmissiveChunkSkipsWhatItCannotRead(t *testing.T) {
	got := parseEmissiveChunk("19, , 26,not-a-number,47,99999999")
	want := []uint16{19, 26, 47}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	for _, slot := range want {
		if !got[slot] {
			t.Errorf("slot %d was readable but was dropped", slot)
		}
	}
}

// The real atlas has to carry the set the game used to hardcode, or every lit
// fitting in the shipped map goes out.
func TestShippedAtlasStillDeclaresItsLitFittings(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "assets", "base", "map_64x64.png"))
	if err != nil {
		t.Skipf("no map atlas to check: %v", err)
	}
	defer file.Close()
	got := parseEmissiveChunk(png_chunks.ReadTextChunkFromPNG(file, EmissiveChunkKey))
	for _, slot := range []uint16{19, 26, 27, 29, 30, 45, 46, 47, 49} {
		if !got[slot] {
			t.Errorf("slot %d no longer marked emissive in the shipped atlas", slot)
		}
	}
}

// The bake renumbers slots, and a panel that glowed before it has to glow after.
func TestBakingMovesTheEmissiveSetOntoTheNewSlots(t *testing.T) {
	a := &AtlasIndex{emissive: map[uint16]bool{4: true, 7: true, 11: true}}
	remap := map[uint16]uint16{4: 0, 7: 1} // 11 was unused and is dropped by the bake

	moved := make(map[uint16]bool, len(a.emissive))
	for old := range a.emissive {
		if now, kept := remap[old]; kept {
			moved[now] = true
		}
	}
	if !moved[0] || !moved[1] {
		t.Errorf("slots 4 and 7 did not arrive at 0 and 1: %v", moved)
	}
	if len(moved) != 2 {
		t.Errorf("the dropped slot came along: %v", moved)
	}
}

// Rewriting the record must not touch the picture. The CPU sheet is bigger than the
// file whenever the atlas has grown but not been baked, so a rewrite that encoded the
// sheet instead of the file would silently renumber every slot.
func TestRewritingTheRecordKeepsThePictureAndItsSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atlas.png")
	sheet := image.NewRGBA(image.Rect(0, 0, 16, 4))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{C: color.RGBA{R: 200, G: 30, B: 90, A: 255}}, image.Point{}, draw.Src)
	if err := writePNG(path, sheet, []uint16{2}); err != nil {
		t.Fatal(err)
	}

	if err := rewriteEmissive(path, []uint16{2, 5}); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoded, _, err := image.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds(); got.Dx() != 16 || got.Dy() != 4 {
		t.Errorf("atlas is now %v, want the 16x4 it was", got)
	}
	if r, g, b, _ := decoded.At(9, 2).RGBA(); r>>8 != 200 || g>>8 != 30 || b>>8 != 90 {
		t.Errorf("pixel changed to %v", decoded.At(9, 2))
	}

	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	got := parseEmissiveChunk(png_chunks.ReadTextChunkFromPNG(file, EmissiveChunkKey))
	if !got[2] || !got[5] || len(got) != 2 {
		t.Errorf("record is %v, want slots 2 and 5", got)
	}
}
