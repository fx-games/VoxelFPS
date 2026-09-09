package quads

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
)

// maxAtlasSlots is what a texture index can address: the voxel lookup buffer packs
// it into 11 bits alongside the emissive flag and the baked light level (see
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
	onGrow func()
}

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
	return a
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

	rows := (len(used) + columns - 1) / columns
	if rows == 0 {
		rows = 1
	}
	baked := a.sheet.SubImage(image.Rect(0, 0, columns*tileW, rows*tileH))
	if err := writePNG(a.Atlas.SourceFile(), baked); err != nil {
		return nil, err
	}
	return remap, a.savePaths()
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

func writePNG(filename string, img image.Image) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}
