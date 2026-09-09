package quads

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/memmaker/goray/quads/glhf"
)

// TexLibraryRoot is the folder every texture in the game is chosen from. It is
// read-only as far as the editor is concerned: picking a texture copies it into a
// map's atlas, it never writes back here.
//
// Slash separated, because that is what Disk wants - it is an fs.FS, which takes
// slashes on every platform. filepath.FromSlash is applied where an OS call needs
// it instead.
const TexLibraryRoot = "assets/textures"

// DroppedDir is where a texture dragged onto the window lands: inside the
// library, because the library is the only place an atlas slot can point back
// at, and in a folder of its own so what was dropped in is told apart from what
// shipped with the game.
const DroppedDir = "dropped"

// lastBrowsedDir is where the picker opens next time. Choosing a texture is
// rarely a one-off - a wall, then its trim, then the floor - and all of those are
// in the folder the last one came from.
var lastBrowsedDir string

// browseColumns is how wide a directory's page is. It matches the map atlas so a
// texture is the same size in the browser as it is in the atlas next to it.
const browseColumns = 16

// browseMaxEntries caps one directory at what a texture can hold at this width.
//
// ponytail: a directory past this many files shows the first browseMaxEntries.
// Paging belongs here if the library ever grows a folder that big.
const browseMaxEntries = browseColumns * 64

// TexEntry is one cell of the browser: a subdirectory to walk into, or a texture
// file to pick. Path is relative to TexLibraryRoot, with forward slashes, which is
// also how a picked texture is recorded in an atlas's .paths file.
type TexEntry struct {
	Path  string
	Name  string
	IsDir bool
}

// TexBrowser is one directory of the texture library, uploaded as a fixed-grid
// atlas. That is the whole trick: the picker widget already knows how to draw,
// scroll and hit-test a grid atlas, so a directory becomes one by being packed
// into one, and folders are cells in it like everything else.
//
// ponytail: a directory is decoded and uploaded on every visit, ~450 small PNGs
// at the worst folder in the library. Cache by directory if walking around starts
// to hitch.
type TexBrowser struct {
	root         string
	tileW, tileH int
	dir          string
	entries      []TexEntry
	atlas        *glhf.TextureAtlasFixed
	setupBatch   func(glhf.TextureAtlas)
	dropBatch    func(*glhf.SpriteBatch)
}

// NewTexBrowser opens the library at its root. setupBatch and dropBatch hand a
// directory's atlas in and out of the 2D pass - a browser that never dropped the
// batch of the directory it just left would keep drawing it.
func NewTexBrowser(tileW, tileH int, setupBatch func(glhf.TextureAtlas), dropBatch func(*glhf.SpriteBatch)) *TexBrowser {
	b := &TexBrowser{
		root:       TexLibraryRoot,
		tileW:      tileW,
		tileH:      tileH,
		setupBatch: setupBatch,
		dropBatch:  dropBatch,
	}
	b.Open(lastBrowsedDir)
	return b
}

func (b *TexBrowser) Atlas() glhf.TextureAtlas { return b.atlas }

// AcceptDropped copies dropped PNGs into the library's dropped folder. It
// returns how many landed; anything that is not a PNG is left alone rather than
// refused, because a drop of a folder full of art should take the art. Showing
// the folder afterwards is the caller's - this only touches the filesystem, and
// uploading a directory needs the GL context the widget is already on.
//
// The bytes are copied as they are, after a header check that they really are a
// PNG - re-encoding would be a lossless round trip of somebody else's file for
// no reason, and a .png that is not one has to be caught here rather than become
// a magenta tile nobody can explain.
func (b *TexBrowser) AcceptDropped(names []string) (int, error) {
	dir := filepath.Join(b.root, DroppedDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	saved := 0
	for _, name := range names {
		if !strings.EqualFold(filepath.Ext(name), ".png") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			log.Printf("[textures] dropped %s: %v", name, err)
			continue
		}
		if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
			log.Printf("[textures] dropped %s is not a PNG: %v", name, err)
			continue
		}
		target, err := freeName(dir, filepath.Base(name))
		if err != nil {
			return saved, err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

// freeName is base in dir, or base with a number on it when that is taken.
// Overwriting is the one thing it must not do: a map records a picked texture by
// its library path, so replacing the file under a path already handed out would
// quietly change every slot that points at it.
func freeName(dir, base string) (string, error) {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		candidate := filepath.Join(dir, base)
		if n > 1 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, n, ext))
		}
		_, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
}

// Close takes the directory being shown out of the 2D pass. Without it every visit
// to the picker leaves one more directory being drawn, and holding its texture,
// for the rest of the session.
func (b *TexBrowser) Close() {
	if b.atlas != nil && b.dropBatch != nil {
		b.dropBatch(b.atlas.GetSpriteBatch())
		b.atlas = nil
	}
}

// Dir is the directory being shown, for a breadcrumb.
func (b *TexBrowser) Dir() string {
	return path.Join(path.Base(b.root), b.dir) + "/"
}

// Entry is the cell at a picker index, if there is one there.
func (b *TexBrowser) Entry(index int) (TexEntry, bool) {
	if index < 0 || index >= len(b.entries) {
		return TexEntry{}, false
	}
	return b.entries[index], true
}

// Label names the cell under the cursor. A grid of 64x64 tiles with no names is a
// memory game, and the file name is what the textures in this library are told
// apart by.
func (b *TexBrowser) Label(index int) string {
	entry, ok := b.Entry(index)
	if !ok {
		return ""
	}
	return entry.Name
}

// Up walks to the parent directory, stopping at the library root.
func (b *TexBrowser) Up() {
	if b.dir == "" {
		return
	}
	parent := path.Dir(b.dir)
	if parent == "." {
		parent = ""
	}
	b.Open(parent)
}

// Open shows a directory, given relative to the library root. A directory that
// will not read shows as empty rather than taking the editor down with it - this
// is a folder a user is free to rename or move while the game is running.
func (b *TexBrowser) Open(dir string) {
	b.dir = dir
	lastBrowsedDir = dir
	b.entries = readTexDir(b.root, dir)
	b.upload()
}

// readTexDir lists one directory as browser cells: an ".." row first when there is
// somewhere to go back to, then subdirectories, then textures, each group by name.
func readTexDir(root, dir string) []TexEntry {
	var entries []TexEntry
	if dir != "" {
		parent := path.Dir(dir)
		if parent == "." {
			parent = ""
		}
		entries = append(entries, TexEntry{Path: parent, Name: "..", IsDir: true})
	}

	osEntries, err := os.ReadDir(filepath.FromSlash(path.Join(root, dir)))
	if err != nil {
		return entries
	}

	var dirs, files []TexEntry
	for _, e := range osEntries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child := name
		if dir != "" {
			child = dir + "/" + name
		}
		if e.IsDir() {
			dirs = append(dirs, TexEntry{Path: child, Name: name, IsDir: true})
		} else if strings.EqualFold(filepath.Ext(name), ".png") {
			files = append(files, TexEntry{Path: child, Name: name})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	entries = append(entries, dirs...)
	entries = append(entries, files...)
	if len(entries) > browseMaxEntries {
		entries = entries[:browseMaxEntries]
	}
	return entries
}

// upload packs the current directory into one texture and puts it in the 2D pass,
// taking the previous directory's out of it.
func (b *TexBrowser) upload() {
	b.Close()

	texture, err := glhf.NewTextureFromImage(browseSheet(b.root, b.entries, b.tileW, b.tileH))
	if err != nil {
		panic(err) // an in-memory RGBA that will not upload is a broken GL context
	}
	b.atlas = glhf.NewTextureAtlasFixed(texture, b.tileW, b.tileH, "")
	if b.setupBatch != nil {
		b.setupBatch(b.atlas)
	}
}

// browseSheet lays a directory out as the grid the picker draws: one cell per
// entry, folders first, in the order readTexDir listed them.
func browseSheet(root string, entries []TexEntry, tileW, tileH int) *image.RGBA {
	rows := max((len(entries)+browseColumns-1)/browseColumns, 1)
	sheet := image.NewRGBA(image.Rect(0, 0, browseColumns*tileW, rows*tileH))
	folder := folderIcon(tileW, tileH)
	for i, entry := range entries {
		tile := folder
		if entry.Name == ".." {
			tile = folderUpIcon(tileW, tileH)
		} else if !entry.IsDir {
			tile = TileImage(root+"/"+entry.Path, tileW, tileH)
		}
		draw.Draw(sheet, tileRect(i, browseColumns, tileW, tileH), tile, image.Point{}, draw.Src)
	}
	return sheet
}

// TileImage reads a texture off disk at exactly one atlas cell. filePath is a
// slash separated path from the working directory, the way Disk wants it. The library is
// mostly 64x64 but not entirely, and an atlas cell is one size, so anything else
// is resampled rather than rejected - a texture the picker refuses to show is a
// texture nobody can find out is the wrong size.
func TileImage(filePath string, w, h int) *image.RGBA {
	return scaleNearest(Disk.Image(filePath), w, h)
}

// scaleNearest is nearest neighbour on purpose: these are pixel-art tiles, and
// interpolating them muddies exactly the hard edges they are drawn for.
func scaleNearest(src image.Image, w, h int) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if bounds.Dx() == w && bounds.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
		return dst
	}
	for y := 0; y < h; y++ {
		sourceY := bounds.Min.Y + y*bounds.Dy()/h
		for x := 0; x < w; x++ {
			dst.Set(x, y, src.At(bounds.Min.X+x*bounds.Dx()/w, sourceY))
		}
	}
	return dst
}

// folderIcon is the cell a subdirectory gets: a manila folder, drawn rather than
// loaded, so navigation costs the library no asset of its own. Warm and saturated
// because everything around it is wall texture - the folders have to read as
// "not a texture" at a glance down a grid of 200 of them.
func folderIcon(w, h int) *image.RGBA {
	icon := image.NewRGBA(image.Rect(0, 0, w, h))
	x := func(f float64) int { return int(f * float64(w)) }
	y := func(f float64) int { return int(f * float64(h)) }
	fill := func(rect image.Rectangle, c color.RGBA) {
		draw.Draw(icon, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
	}

	edge := color.RGBA{R: 0x2A, G: 0x1E, B: 0x08, A: 0xFF}
	body := color.RGBA{R: 0xE8, G: 0xB0, B: 0x44, A: 0xFF}
	back := color.RGBA{R: 0xB8, G: 0x84, B: 0x24, A: 0xFF}

	// The back panel and its tab, then the front panel over them, each drawn as a
	// dark rectangle with a lighter one inset inside it - two rects per outline.
	fill(image.Rect(x(0.08), y(0.18), x(0.48), y(0.34)), edge)
	fill(image.Rect(x(0.11), y(0.21), x(0.45), y(0.34)), back)
	fill(image.Rect(x(0.08), y(0.28), x(0.92), y(0.50)), edge)
	fill(image.Rect(x(0.11), y(0.31), x(0.89), y(0.50)), back)
	fill(image.Rect(x(0.08), y(0.40), x(0.92), y(0.82)), edge)
	fill(image.Rect(x(0.11), y(0.43), x(0.89), y(0.79)), body)
	return icon
}

// folderUpIcon is the ".." cell. It is the one cell in a directory that does the
// opposite of every other folder in it, and it is hit more than any of them, so it
// gets an arrow rather than being told apart by hovering it for its name.
func folderUpIcon(w, h int) *image.RGBA {
	icon := folderIcon(w, h)
	arrow := color.RGBA{R: 0x2A, G: 0x1E, B: 0x08, A: 0xFF}
	// A left pointing triangle with a stem: the head widens over the first eighth of
	// the width, then the stem carries on at a fixed height.
	midY, half := h/2+h/16, h/8
	for step := 0; step < half; step++ {
		for y := midY - step; y <= midY+step; y++ {
			set(icon, w/4+step, y, arrow)
		}
	}
	for x := w / 4; x < w-w/4; x++ {
		for y := midY - h/24; y <= midY+h/24; y++ {
			set(icon, x, y, arrow)
		}
	}
	return icon
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.SetRGBA(x, y, c)
	}
}
