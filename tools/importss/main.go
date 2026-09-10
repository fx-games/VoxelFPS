// Command importss pulls System Shock's art and sound out of its RES containers
// and files it the way this project files assets.
//
//	go run ./tools/importss -data ~/SSHOCK/DATA -dump /tmp/ss
//
// The container format, its LZW-ish compression and the RLE the bitmaps inside
// it use are all read by inkyblackness/hacked, which is the reference
// implementation of the format; there is no reason to write a second one. What
// is here is the part hacked does not do: turning resources into PNGs and WAVs
// under the names the rest of this repo expects.
//
// -dump writes every bitmap and every sound as its own file, named
// <RESFILE>/<resource id>_<block>.png. It is the raw form, for looking at what
// is in there; the grouped output the game actually loads is written by the
// other modes.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/inkyblackness/hacked/ss1/content/audio/voc"
	"github.com/inkyblackness/hacked/ss1/content/bitmap"
	"github.com/inkyblackness/hacked/ss1/resource"
	"github.com/inkyblackness/hacked/ss1/resource/lgres"
	"github.com/inkyblackness/hacked/ss1/serial"
	"github.com/memmaker/goray/audio"
)

// gamePalette is the resource holding the palette every in-game bitmap is drawn
// with. It lives in GAMEPAL.RES; nothing in OBJART or TEXTURE carries its own.
const gamePalette = resource.ID(0x02BC)

func main() {
	dataDir := flag.String("data", "", "path to System Shock's DATA directory")
	dumpDir := flag.String("dump", "", "write every bitmap and sound here, one file each")
	install := flag.Bool("install", false, "file everything into this repo's own asset folders")
	flag.Parse()

	if *dataDir == "" {
		fail(fmt.Errorf("-data is required"))
	}
	opaque, keyed, err := loadPalette(filepath.Join(*dataDir, "GAMEPAL.RES"))
	if err != nil {
		fail(err)
	}
	if *dumpDir != "" {
		if err := dumpAll(*dataDir, *dumpDir, opaque, keyed); err != nil {
			fail(err)
		}
	}
	if *install {
		in, err := newInstaller(*dataDir, opaque, keyed)
		if err != nil {
			fail(err)
		}
		textures, err := in.installTextures()
		if err != nil {
			fail(err)
		}
		critters, err := in.installCritters()
		if err != nil {
			fail(err)
		}
		objects, err := in.installObjects()
		if err != nil {
			fail(err)
		}
		weapons, err := in.installWeapons()
		if err != nil {
			fail(err)
		}
		models, err := in.installModels()
		if err != nil {
			fail(err)
		}
		fmt.Printf("installed %d textures, %d critters, %d objects, %d weapons, %d models\n",
			textures, critters, objects, weapons, models)
	}
}

// loadPalette reads the game palette. It is 256 RGB triples at VGA's six bits a
// channel, which is why every colour is scaled on the way out - taken raw, the
// whole game renders in the darkest quarter of the range.
func loadPalette(path string) (opaque, keyed color.Palette, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r, err := lgres.ReaderFrom(f)
	if err != nil {
		return nil, nil, err
	}
	view, err := r.View(gamePalette)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: no palette resource %v: %w", path, gamePalette, err)
	}
	block, err := view.Block(0)
	if err != nil {
		return nil, nil, err
	}
	var raw bitmap.Palette
	decoder := serial.NewDecoder(block)
	decoder.Code(&raw)
	if err := decoder.FirstError(); err != nil {
		return nil, nil, err
	}
	// Index 0 is the transparent one for a sprite and plain black for a wall,
	// so the palette is built both ways once and the caller picks.
	opaque = make(color.Palette, len(raw))
	keyed = make(color.Palette, len(raw))
	for i, c := range raw {
		opaque[i] = color.RGBA{R: c.Red, G: c.Green, B: c.Blue, A: 0xFF}
		keyed[i] = opaque[i]
	}
	keyed[0] = color.RGBA{}
	return opaque, keyed, nil
}

// artFiles are the containers holding pictures, and whether what is in them is
// a surface (index 0 is black) or a sprite (index 0 is see-through). Everything
// else in DATA is text, movies, or the level maps.
var artFiles = []struct {
	name  string
	keyed bool
}{
	{"TEXTURE.RES", false},
	{"CITMAT.RES", false},
	{"OBJART.RES", true},
	{"OBJART2.RES", true},
	{"OBJART3.RES", true},
	{"HANDART.RES", true},
	{"SIDEART.RES", true},
	{"MFDART.RES", true},
}

func dumpAll(dataDir, outDir string, opaque, keyed color.Palette) error {
	for _, art := range artFiles {
		name := art.name
		pal := opaque
		if art.keyed {
			pal = keyed
		}
		n, err := dumpArt(filepath.Join(dataDir, name), filepath.Join(outDir, strings.TrimSuffix(name, ".RES")), pal)
		if err != nil {
			return err
		}
		fmt.Printf("%-14s %d bitmaps\n", name, n)
	}
	n, err := dumpSounds(filepath.Join(dataDir, "DIGIFX.RES"), filepath.Join(outDir, "DIGIFX"))
	if err != nil {
		return err
	}
	fmt.Printf("%-14s %d sounds\n", "DIGIFX.RES", n)
	return nil
}

func dumpArt(resPath, outDir string, pal color.Palette) (int, error) {
	f, err := os.Open(resPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r, err := lgres.ReaderFrom(f)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for _, id := range r.IDs() {
		view, err := r.View(id)
		if err != nil || view.ContentType() != resource.Bitmap {
			continue
		}
		// A frame of an animation is stored as the difference from the frame
		// before it, so a compound resource has to be decoded in order and each
		// block handed the one before as its reference. Decoded standalone, every
		// frame after the first comes out as holes.
		var previous []byte
		for i := 0; i < view.BlockCount(); i++ {
			block, err := view.Block(i)
			if err != nil {
				continue
			}
			bmp, err := bitmap.DecodeReferenced(block, func(w, h int16) ([]byte, error) {
				if len(previous) == int(w)*int(h) {
					out := make([]byte, len(previous))
					copy(out, previous)
					return out, nil
				}
				return make([]byte, int(w)*int(h)), nil
			})
			if err != nil {
				continue
			}
			previous = bmp.Pixels
			img := toImage(bmp, pal)
			if img == nil {
				continue
			}
			name := fmt.Sprintf("%04d_%02d.png", id.Value(), i)
			if err := writePNG(filepath.Join(outDir, name), img); err != nil {
				return written, err
			}
			written++
		}
	}
	return written, nil
}

// toImage paints a decoded bitmap with the game palette. A bitmap carrying its
// own palette uses that one instead - a few menu pictures do.
func toImage(bmp *bitmap.Bitmap, pal color.Palette) *image.RGBA {
	w, h := int(bmp.Header.Width), int(bmp.Header.Height)
	if w <= 0 || h <= 0 || bmp.Header.Stride == 0 {
		return nil
	}
	use := pal
	if bmp.Palette != nil {
		use = make(color.Palette, len(bmp.Palette))
		for i, c := range bmp.Palette {
			use[i] = color.RGBA{R: c.Red, G: c.Green, B: c.Blue, A: 0xFF}
		}
		if _, _, _, a := pal[0].RGBA(); a == 0 {
			use[0] = color.RGBA{}
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	stride := int(bmp.Header.Stride)
	for y := 0; y < h; y++ {
		row := y * stride
		if row+w > len(bmp.Pixels) {
			break
		}
		for x := 0; x < w; x++ {
			img.Set(x, y, use[bmp.Pixels[row+x]])
		}
	}
	return img
}

func dumpSounds(resPath, outDir string) (int, error) {
	f, err := os.Open(resPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r, err := lgres.ReaderFrom(f)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	written := 0
	for _, id := range r.IDs() {
		view, err := r.View(id)
		if err != nil || view.ContentType() != resource.Sound {
			continue
		}
		block, err := view.Block(0)
		if err != nil {
			continue
		}
		sound, err := voc.Load(block)
		if err != nil || len(sound.Samples) == 0 {
			continue
		}
		name := fmt.Sprintf("%04d.wav", id.Value())
		if err := os.WriteFile(filepath.Join(outDir, name), audio.WAVFromPCM8(sound.Samples, int(sound.SampleRate)), 0o644); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "importss:", err)
	os.Exit(1)
}
