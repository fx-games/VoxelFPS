// Command tinttex recolors neutral gray textures into System Shock hues.
//
//	go run ./tools/tinttex -in ~/Downloads/RETRO_TEXTURES_V28/TEXTURES \
//	    -out assets/base/tex/medical -family teal
//
// The ramps are not invented: each one is the mean color of every System Shock
// pixel in that hue family, binned by luminance (sshocktx.wad, ~4M pixels), with
// the highlight end extrapolated because SS textures are too dark to measure it.
// A texture is gradient-mapped through its ramp by luminance, and the result is
// mixed back in proportion to how gray the pixel was, so painted detail that is
// already colored -- hazard stripes, LEDs, blood -- survives the pass.
package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ramps holds 9 anchors per family, evenly spaced over luminance 0..255.
var ramps = map[string][9][3]float64{
	"rust":   {{0, 0, 0}, {56, 20, 20}, {101, 44, 42}, {150, 65, 56}, {188, 87, 69}, {224, 116, 97}, {255, 148, 129}, {255, 183, 165}, {255, 220, 205}},
	"amber":  {{0, 0, 0}, {54, 30, 15}, {95, 53, 18}, {138, 78, 34}, {174, 111, 57}, {215, 147, 43}, {241, 179, 88}, {255, 212, 175}, {255, 249, 228}},
	"olive":  {{0, 0, 0}, {41, 32, 2}, {82, 63, 3}, {120, 95, 4}, {157, 136, 5}, {208, 177, 8}, {240, 212, 27}, {249, 243, 84}, {255, 255, 176}},
	"green":  {{0, 0, 0}, {18, 44, 20}, {33, 81, 34}, {48, 123, 45}, {58, 171, 44}, {79, 212, 53}, {113, 242, 87}, {153, 255, 130}, {199, 255, 180}},
	"teal":   {{0, 0, 0}, {4, 49, 48}, {4, 91, 92}, {4, 135, 136}, {4, 178, 179}, {4, 225, 227}, {50, 252, 252}, {130, 255, 255}, {202, 255, 255}},
	"blue":   {{0, 0, 0}, {30, 28, 71}, {62, 61, 102}, {89, 88, 132}, {114, 113, 191}, {153, 153, 241}, {195, 197, 255}, {226, 228, 255}, {255, 255, 255}},
	"violet": {{0, 0, 0}, {55, 6, 86}, {98, 5, 176}, {151, 31, 219}, {191, 70, 217}, {228, 98, 255}, {255, 130, 255}, {255, 166, 255}, {255, 206, 255}},
}

// autoMix is how often each family shows up in System Shock's own textures, by
// chromatic pixel share. -family auto deals textures out in these proportions.
var autoMix = []struct {
	name   string
	weight int
}{{"rust", 38}, {"amber", 25}, {"blue", 16}, {"teal", 8}, {"green", 8}, {"olive", 3}, {"violet", 2}}

func main() {
	in := flag.String("in", "", "source texture directory")
	out := flag.String("out", "", "destination directory (created if missing)")
	family := flag.String("family", "rust", "ramp name, a comma-separated list to alternate, or \"auto\" for the System Shock mix")
	strength := flag.Float64("strength", 0.75, "how far a fully gray pixel moves onto the ramp, 0..1")
	preserve := flag.Float64("preserve", 1.6, "how hard already-colored pixels resist tinting; higher keeps more of the original")
	gamma := flag.Float64("gamma", 1.25, "luminance gamma before lookup; >1 darkens, matching System Shock's murk")
	list := flag.Bool("list", false, "print the ramp names and exit")
	flag.Parse()

	if *list {
		for _, n := range sortedRamps() {
			fmt.Println(n)
		}
		return
	}
	if *in == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	fams, err := parseFamilies(*family)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(*in, *out, fams, *strength, *preserve, *gamma); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sortedRamps() []string {
	names := make([]string, 0, len(ramps))
	for n := range ramps {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func parseFamilies(s string) ([]string, error) {
	if s == "auto" {
		var fams []string
		for _, e := range autoMix {
			for i := 0; i < e.weight; i++ {
				fams = append(fams, e.name)
			}
		}
		return fams, nil
	}
	fams := strings.Split(s, ",")
	for i, f := range fams {
		fams[i] = strings.TrimSpace(f)
		if _, ok := ramps[fams[i]]; !ok {
			return nil, fmt.Errorf("unknown family %q, have %s", fams[i], strings.Join(sortedRamps(), " "))
		}
	}
	return fams, nil
}

// pickFamily hashes the filename so a given texture keeps the same hue on every
// run -- a re-tint must not reshuffle a level's walls.
func pickFamily(fams []string, name string) string {
	if len(fams) == 1 {
		return fams[0]
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	return fams[int(h.Sum32()%uint32(len(fams)))]
}

func run(in, out string, fams []string, strength, preserve, gamma float64) error {
	entries, err := os.ReadDir(in)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	luts := map[string]*[256][3]float64{}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		fam := pickFamily(fams, e.Name())
		if luts[fam] == nil {
			luts[fam] = buildLUT(ramps[fam], gamma)
		}
		if err := tintFile(filepath.Join(in, e.Name()), filepath.Join(out, e.Name()), luts[fam], strength, preserve); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		n++
	}
	fmt.Printf("tinted %d textures into %s\n", n, out)
	return nil
}

// buildLUT expands the 9 anchors into a 256-entry table, applying the gamma to
// the input luminance so the darkening happens once per family, not per pixel.
func buildLUT(anchors [9][3]float64, gamma float64) *[256][3]float64 {
	var lut [256][3]float64
	for i := 0; i < 256; i++ {
		l := math.Pow(float64(i)/255, gamma) * 255
		t := l * float64(len(anchors)-1) / 255
		k := int(t)
		if k > len(anchors)-2 {
			k = len(anchors) - 2
		}
		u := t - float64(k)
		for c := 0; c < 3; c++ {
			lut[i][c] = anchors[k][c]*(1-u) + anchors[k+1][c]*u
		}
	}
	return &lut
}

func tintFile(src, dst string, lut *[256][3]float64, strength, preserve float64) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	// NRGBA keeps alpha unpremultiplied, so a tinted pixel under a transparent
	// one cannot bleed into the color channels.
	b := img.Bounds()
	m := image.NewNRGBA(b)
	draw.Draw(m, b, img, b.Min, draw.Src)
	tint(m, lut, strength, preserve)

	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := png.Encode(o, m); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func tint(m *image.NRGBA, lut *[256][3]float64, strength, preserve float64) {
	for i := 0; i < len(m.Pix); i += 4 {
		r, g, b := float64(m.Pix[i]), float64(m.Pix[i+1]), float64(m.Pix[i+2])
		l := int(0.299*r + 0.587*g + 0.114*b)

		hi := math.Max(r, math.Max(g, b))
		lo := math.Min(r, math.Min(g, b))
		sat := 0.0
		if hi > 0 {
			sat = (hi - lo) / hi
		}
		// A gray pixel takes the full tint; a saturated one is left as painted.
		w := strength * math.Pow(1-sat, preserve)

		m.Pix[i] = mix(r, lut[l][0], w)
		m.Pix[i+1] = mix(g, lut[l][1], w)
		m.Pix[i+2] = mix(b, lut[l][2], w)
	}
}

func mix(a, b, w float64) uint8 {
	v := a*(1-w) + b*w
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}
