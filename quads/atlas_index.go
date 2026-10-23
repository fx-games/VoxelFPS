package quads

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/tpack/png_chunks"

	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// maxAtlasSlots is what a texture index can address: the voxel lookup buffer packs
// it into 10 bits alongside the emissive flag, the baked light and the solid bit (see
// voxel.EmissiveTextureMask). A map cannot hold more distinct textures than this,
// whatever the library has in it.
const maxAtlasSlots = int(voxel.EmissiveTextureMask) + 1

// AtlasIndex is a working atlas plus the record of where each of its tiles came
// from. It is the one thing standing between "choose any texture in the library"
// and "a block stores a uint16": picking a texture copies it into a free slot here
// and hands back that slot's number.
//
// It keeps a CPU copy of the sheet rather than reading tiles back off the GPU.
// That copy is what gets baked, so what lands on disk is the pixels that were put
// in, independent of how the driver hands a texture back.
type AtlasIndex struct {
	Atlas *glhf.TextureAtlasFixed

	sheet  *image.RGBA
	paths  []string // slot -> library-relative source, "" for a slot from the original sheet
	byPath map[string]int
	// emissive is which slots glow, and it is the authored truth about that: it is
	// read from and written back to the atlas file itself. See EmissiveChunkKey.
	emissive map[uint16]bool
	onGrow   func()
}

// EmissiveChunkKey is the tEXt chunk in the atlas PNG listing the slots that glow,
// as decimal numbers separated by commas.
//
// It rides in the atlas rather than in a sidecar or in Go because it is a property
// of the pictures: slot 49 glows because of what is painted in slot 49, so the fact
// travels with the file it is a fact about. A hardcoded list in Go went stale the
// moment the editor could add a texture, and the numbers meant nothing to anyone
// reading them.
const EmissiveChunkKey = "emIS"

// NewAtlasIndex adopts an atlas loaded from disk. Every slot it already has counts
// as occupied - the hand-made sheets are indexed by number by the maps that shipped
// with them, so nothing may be handed out on top of them.
//
// The .paths file next to the atlas is what makes a slot recognisable across
// sessions: without it, re-picking a texture the map already uses would append a
// second copy of it every time the game restarts.
func NewAtlasIndex(atlas *glhf.TextureAtlasFixed) *AtlasIndex {
	a := &AtlasIndex{
		Atlas:  atlas,
		sheet:  scaleNearest(Disk.Image(atlas.SourceFile()), atlas.Width(), atlas.Height()),
		byPath: make(map[string]int),
	}
	a.paths = make([]string, atlas.LastValidIndex()+1)
	a.loadPaths()
	a.loadEmissive()
	return a
}

// Emissive is the slots that glow, in order, for seeding the voxel map's own set.
func (a *AtlasIndex) Emissive() []uint16 {
	slots := make([]uint16, 0, len(a.emissive))
	for slot := range a.emissive {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	return slots
}

// SetEmissive records that a slot does or does not glow and writes it back to the
// atlas. Called by the G key, which is the only way a texture is marked today, so
// what the editor did survives the session that did it.
func (a *AtlasIndex) SetEmissive(slot uint16, glowing bool) error {
	if a.emissive == nil {
		a.emissive = make(map[uint16]bool)
	}
	if glowing {
		a.emissive[slot] = true
	} else {
		delete(a.emissive, slot)
	}
	return a.saveEmissive()
}

// OnGrow is called when a pick made the atlas bigger. The cube shader is told the
// atlas grid size once and reads it per fragment, so it has to hear about it.
func (a *AtlasIndex) OnGrow(callback func()) { a.onGrow = callback }

// Count is how many slots are handed out - the size of the atlas a save would bake
// if the map used every one of them.
func (a *AtlasIndex) Count() int { return len(a.paths) }

// SourcePath is the library texture in a slot, empty for the ones that came from
// the original sheet.
func (a *AtlasIndex) SourcePath(index int) string {
	if index < 0 || index >= len(a.paths) {
		return ""
	}
	return a.paths[index]
}

// Tile is one slot's pixels, for drawing a picture of something built out of
// it. A slot past the end of the sheet is an empty tile rather than a panic.
func (a *AtlasIndex) Tile(slot int) image.Image {
	tileW, tileH := a.Atlas.GetAtlasItemSize()
	rect := tileRect(slot, a.Atlas.GridWidth(), tileW, tileH)
	if slot < 0 || !rect.In(a.sheet.Bounds()) {
		return image.NewRGBA(image.Rect(0, 0, tileW, tileH))
	}
	return a.sheet.SubImage(rect)
}

// Import puts a library texture in the atlas and returns its slot, reusing the one
// it already has if it is in there. libPath is relative to TexLibraryRoot.
func (a *AtlasIndex) Import(libPath string) (int, error) {
	if slot, alreadyIn := a.byPath[libPath]; alreadyIn {
		return slot, nil
	}
	slot := len(a.paths)
	if slot >= maxAtlasSlots {
		return -1, fmt.Errorf("atlas full at %d textures", maxAtlasSlots)
	}
	if err := a.growTo(slot); err != nil {
		return -1, err
	}

	tileW, tileH := a.Atlas.GetAtlasItemSize()
	tile := TileImage(filepath.Join(TexLibraryRoot, filepath.FromSlash(libPath)), tileW, tileH)
	a.writeTile(slot, tile)

	a.paths = append(a.paths, libPath)
	a.byPath[libPath] = slot
	// On disk straight away, not at the next bake: the block list is saved the
	// moment a block is added, and a list pointing past the end of the atlas file
	// draws those faces empty once the level is loaded again. Import only ever
	// appends, so no slot the saved map already uses moves.
	if err := a.save(); err != nil {
		return slot, fmt.Errorf("texture in slot %d, but the atlas was not saved: %w", slot, err)
	}
	return slot, nil
}

// Bake rewrites the atlas as exactly the slots in used, in that order, and writes
// it back over the file it was loaded from. The library itself is never touched:
// baking copies out of the working sheet, which was copied out of the library.
//
// Returns the old slot -> new slot mapping the map has to be moved onto.
func (a *AtlasIndex) Bake(used []uint16) (map[uint16]uint16, error) {
	tileW, tileH := a.Atlas.GetAtlasItemSize()
	columns := a.Atlas.GridWidth()
	if columns == 0 {
		return nil, fmt.Errorf("atlas %s has no usable grid", a.Atlas.SourceFile())
	}

	remap := make(map[uint16]uint16, len(used))
	packed := image.NewRGBA(a.sheet.Bounds())
	paths := make([]string, len(used))
	for slot, old := range used {
		remap[old] = uint16(slot)
		paths[slot] = a.SourcePath(int(old))
		draw.Draw(packed, tileRect(slot, columns, tileW, tileH),
			a.sheet, tileRect(int(old), columns, tileW, tileH).Min, draw.Src)
	}

	// The emissive set is keyed by slot, so it has to move with the slots or a panel
	// that glowed before the bake stops glowing after it.
	moved := make(map[uint16]bool, len(a.emissive))
	for old := range a.emissive {
		if now, kept := remap[old]; kept {
			moved[now] = true
		}
	}
	a.emissive = moved

	a.sheet = packed
	a.paths = paths
	a.byPath = make(map[string]int, len(paths))
	for slot, libPath := range paths {
		if libPath != "" {
			a.byPath[libPath] = slot
		}
	}
	for slot := range used {
		a.uploadTile(slot)
	}
	return remap, a.save()
}

// save writes the working sheet and its paths record over the atlas's files,
// cut down to the rows the handed-out slots reach.
func (a *AtlasIndex) save() error {
	tileW, tileH := a.Atlas.GetAtlasItemSize()
	sheet := trimmedSheet(a.sheet, len(a.paths), a.Atlas.GridWidth(), tileW, tileH)
	if err := writePNG(a.Atlas.SourceFile(), sheet, a.Emissive()); err != nil {
		return err
	}
	return a.savePaths()
}

// trimmedSheet is the part of a sheet holding its first count slots: whole rows,
// and at least one, so an atlas with nothing in it is still an image.
func trimmedSheet(sheet *image.RGBA, count, columns, tileW, tileH int) image.Image {
	rows := max(1, (count+columns-1)/columns)
	return sheet.SubImage(image.Rect(0, 0, columns*tileW, rows*tileH))
}

// growTo makes room for a slot, expanding the atlas as often as it takes. The CPU
// sheet is regrown alongside it: Expand keeps the old content at the top left of
// the new texture, so a copy into the same corner mirrors it.
func (a *AtlasIndex) growTo(slot int) error {
	grew := false
	for slot > a.Atlas.LastValidIndex() {
		before := a.Atlas.Width() * a.Atlas.Height()
		a.Atlas.Expand()
		if a.Atlas.Width()*a.Atlas.Height() <= before {
			return fmt.Errorf("atlas %s will not grow past %d textures", a.Atlas.SourceFile(), a.Atlas.LastValidIndex()+1)
		}
		grew = true
	}
	if !grew {
		return nil
	}
	grown := image.NewRGBA(image.Rect(0, 0, a.Atlas.Width(), a.Atlas.Height()))
	draw.Draw(grown, a.sheet.Bounds(), a.sheet, image.Point{}, draw.Src)
	a.sheet = grown
	if a.onGrow != nil {
		a.onGrow()
	}
	return nil
}

// writeTile puts one tile into both copies of the atlas, the CPU sheet that gets
// baked and the texture that gets drawn.
func (a *AtlasIndex) writeTile(slot int, tile *image.RGBA) {
	tileW, tileH := a.Atlas.GetAtlasItemSize()
	draw.Draw(a.sheet, tileRect(slot, a.Atlas.GridWidth(), tileW, tileH), tile, image.Point{}, draw.Src)
	a.uploadTile(slot)
}

func (a *AtlasIndex) uploadTile(slot int) {
	tileW, tileH := a.Atlas.GetAtlasItemSize()
	rect := tileRect(slot, a.Atlas.GridWidth(), tileW, tileH)
	pixels := make([]uint8, tileW*tileH*4)
	for row := 0; row < tileH; row++ {
		start := a.sheet.PixOffset(rect.Min.X, rect.Min.Y+row)
		copy(pixels[row*tileW*4:], a.sheet.Pix[start:start+tileW*4])
	}
	a.Atlas.SetPixelsAtIndex(slot, pixels)
}

func tileRect(slot, columns, tileW, tileH int) image.Rectangle {
	return image.Rect(0, 0, tileW, tileH).
		Add(image.Pt((slot%columns)*tileW, (slot/columns)*tileH))
}

// pathsFile is the record of which library texture is in which slot, kept next to
// the atlas it describes.
func pathsFile(atlasFile string) string {
	return strings.TrimSuffix(atlasFile, filepath.Ext(atlasFile)) + ".paths"
}

func (a *AtlasIndex) loadPaths() {
	content, err := os.ReadFile(pathsFile(a.Atlas.SourceFile()))
	if err != nil {
		return // no record yet: every slot is an original, unnamed one
	}
	a.paths = nil
	trimmed := strings.TrimRight(string(content), "\n")
	if trimmed == "" {
		return // a record of an atlas with nothing handed out yet
	}
	for _, line := range strings.Split(trimmed, "\n") {
		libPath := strings.TrimSpace(line)
		if libPath != "" {
			a.byPath[libPath] = len(a.paths)
		}
		a.paths = append(a.paths, libPath)
	}
}

func (a *AtlasIndex) savePaths() error {
	var content string
	if len(a.paths) > 0 {
		content = strings.Join(a.paths, "\n") + "\n"
	}
	return os.WriteFile(pathsFile(a.Atlas.SourceFile()), []byte(content), 0644)
}

func (a *AtlasIndex) loadEmissive() {
	file, err := os.Open(a.Atlas.SourceFile())
	if err != nil {
		return // no atlas to read a set out of: nothing glows
	}
	defer file.Close()
	a.emissive = parseEmissiveChunk(png_chunks.ReadTextChunkFromPNG(file, EmissiveChunkKey))
}

// parseEmissiveChunk reads the chunk's slot list, skipping anything that is not a
// number rather than refusing the atlas: this is metadata on an editable file, and a
// smudged entry should cost one panel's glow, not the level.
func parseEmissiveChunk(content string) map[uint16]bool {
	set := make(map[uint16]bool)
	for _, field := range strings.Split(content, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		slot, err := strconv.ParseUint(field, 10, 16)
		if err != nil {
			log.Printf("[atlas] %s: ignoring emissive entry %q: %v", EmissiveChunkKey, field, err)
			continue
		}
		set[uint16(slot)] = true
	}
	return set
}

func emissiveChunk(slots []uint16) string {
	fields := make([]string, len(slots))
	for i, slot := range slots {
		fields[i] = strconv.FormatUint(uint64(slot), 10)
	}
	return strings.Join(fields, ",")
}

// saveEmissive rewrites the atlas with the current set in its chunk. It decodes the
// file and re-encodes it rather than patching the bytes in place, because
// InjectTextChunkIntoPNG *adds* a chunk - it copies every chunk already there through
// untouched and puts the new one before IEND. Patching a file that already carries an
// emIS would leave two of them, and the reader takes the first, so the record would
// freeze at whatever it said the first time and the file would grow on every G press.
//
// Decoding gives back the pixels and the dimensions that are actually on disk, which
// is what makes this safe: the CPU sheet is bigger than the file whenever the atlas
// has grown but not been baked, and writing that would move every slot.
func (a *AtlasIndex) saveEmissive() error {
	return rewriteEmissive(a.Atlas.SourceFile(), a.Emissive())
}

// rewriteEmissive re-encodes an atlas with a new emissive record, keeping the pixels
// and the dimensions it finds on disk.
func rewriteEmissive(filename string, slots []uint16) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(file)
	file.Close()
	if err != nil {
		return err
	}
	return writePNG(filename, img, slots)
}

// writePNG encodes img over filename with the emissive record in its chunk. It is the
// only writer of an atlas, and it always encodes from scratch, so the file it leaves
// has exactly one emIS in it. A bake rewrites the pixels, and a rewrite that dropped
// the record would put every lit fitting in the level out.
func writePNG(filename string, img image.Image, emissive []uint16) error {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return err
	}
	var out bytes.Buffer
	if err := png_chunks.InjectTextChunkIntoPNG(&encoded, &out, EmissiveChunkKey, emissiveChunk(emissive)); err != nil {
		return err
	}
	return os.WriteFile(filename, out.Bytes(), 0644)
}
