// Command gentex writes a station's worth of wall textures from nothing but
// numbers. It reads a reference set - a folder of PNGs - only to learn what
// colours a room is painted in, and then draws every texture again from
// scratch: plates and their bolts, conduit runs, hazard stripes, corrosion,
// lava, foliage, screens full of unreadable readouts.
//
// Nothing is copied. What crosses over from a reference picture is its palette
// - five greys from dark to light plus a handful of saturated accents, and even
// those are rotated a few degrees around the colour wheel - and one word saying
// what kind of surface it is (a plate, a console, a grate). Every pixel is
// drawn by the code in this file, seeded by the texture's name so a rerun
// produces the same set.
//
// Run with `go run ./tools/gentex` from the repository root:
//
//	go run ./tools/gentex -src ~/Downloads/sshocktx -out assets/textures/station
//
// Names and sizes follow the reference folder, so the result drops in where the
// reference did.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- colour ---

type col struct{ r, g, b float64 }

func lerp(a, b col, t float64) col {
	return col{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func scale(a col, f float64) col { return col{a.r * f, a.g * f, a.b * f} }

func add(a, b col) col { return col{a.r + b.r, a.g + b.g, a.b + b.b} }

func luma(c col) float64 { return 0.299*c.r + 0.587*c.g + 0.114*c.b }

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func toRGBA(c col) color.RGBA {
	return color.RGBA{uint8(clamp(c.r, 0, 255)), uint8(clamp(c.g, 0, 255)), uint8(clamp(c.b, 0, 255)), 255}
}

func rgbToHSV(c col) (h, s, v float64) {
	r, g, b := c.r/255, c.g/255, c.b/255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	d := max - min
	if max > 0 {
		s = d / max
	}
	if d == 0 {
		return 0, s, v
	}
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func hsvToRGB(h, s, v float64) col {
	h = math.Mod(math.Mod(h, 360)+360, 360)
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return col{(r + m) * 255, (g + m) * 255, (b + m) * 255}
}

// shift moves a colour a few degrees around the wheel and takes a little
// saturation off it. It is what keeps a palette a family resemblance rather
// than a copy.
func shift(c col) col {
	h, s, v := rgbToHSV(c)
	return hsvToRGB(h+7, s*0.93, clamp(v*1.02, 0, 1))
}

// ---------------------------------------------------------------- palette ---

type pal struct {
	ramp   [5]col // dark to light
	accent []col  // saturated, ordered by how much of the picture they cover
}

func (p pal) at(t float64) col {
	t = clamp(t, 0, 1) * 4
	i := int(t)
	if i > 3 {
		i = 3
	}
	return lerp(p.ramp[i], p.ramp[i+1], t-float64(i))
}

// acc returns the accent nearest a wanted hue, or a fallback tinted that way
// when the reference had nothing saturated to offer.
func (p pal) acc(hue float64) col {
	best, bestD := col{}, 1e9
	for _, a := range p.accent {
		h, _, _ := rgbToHSV(a)
		d := math.Abs(h - hue)
		if d > 180 {
			d = 360 - d
		}
		if d < bestD {
			best, bestD = a, d
		}
	}
	if bestD > 45 {
		return hsvToRGB(hue+7, 0.8, 0.85)
	}
	return best
}

func (p pal) accN(i int) col {
	if len(p.accent) == 0 {
		return col{200, 160, 60}
	}
	return p.accent[i%len(p.accent)]
}

func extract(img image.Image) pal {
	type entry struct {
		c col
		n int
	}
	counts := map[[3]uint8]int{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			counts[[3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}]++
		}
	}
	all := make([]entry, 0, len(counts))
	total := 0
	for k, n := range counts {
		all = append(all, entry{col{float64(k[0]), float64(k[1]), float64(k[2])}, n})
		total += n
	}
	byLuma := append([]entry(nil), all...)
	sort.Slice(byLuma, func(i, j int) bool { return luma(byLuma[i].c) < luma(byLuma[j].c) })

	var p pal
	for i, q := range []float64{0.05, 0.27, 0.5, 0.75, 0.96} {
		want := int(float64(total) * q)
		run := 0
		p.ramp[i] = byLuma[len(byLuma)-1].c
		for _, e := range byLuma {
			run += e.n
			if run >= want {
				p.ramp[i] = shift(e.c)
				break
			}
		}
	}
	// A reference is mostly its darkest colours, so the ramp comes out muddy
	// and flat. Keep every hue, but stretch the five steps across a range wide
	// enough for a bevel to read: near-black at one end, a real highlight at
	// the other.
	lo := math.Min(luma(p.ramp[0]), 26)
	hi := math.Max(luma(p.ramp[4]), 170)
	for i := range p.ramp {
		t := float64(i) / 4
		want := lo + (hi-lo)*math.Pow(t, 0.8)
		have := luma(p.ramp[i])
		if have < 1 {
			p.ramp[i] = col{want, want, want}
			continue
		}
		p.ramp[i] = scale(p.ramp[i], want/have)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	for _, e := range all {
		h, s, v := rgbToHSV(e.c)
		if s < 0.45 || v < 0.28 || e.n*400 < total {
			continue
		}
		dupe := false
		for _, a := range p.accent {
			ah, _, _ := rgbToHSV(a)
			d := math.Abs(ah - h)
			if d > 180 {
				d = 360 - d
			}
			if d < 22 {
				dupe = true
			}
		}
		if !dupe {
			p.accent = append(p.accent, shift(e.c))
		}
		if len(p.accent) == 5 {
			break
		}
	}
	return p
}

// ------------------------------------------------------------------ noise ---

func hash2(x, y, seed int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + uint32(seed)*362437
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h) / 4294967295.0
}

// vnoise is value noise on a wrapping grid, so every texture tiles.
func vnoise(x, y float64, period, seed int) float64 {
	xi, yi := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(xi), y-float64(yi)
	sx := fx * fx * (3 - 2*fx)
	sy := fy * fy * (3 - 2*fy)
	w := func(v int) int { return ((v % period) + period) % period }
	a := hash2(w(xi), w(yi), seed)
	b := hash2(w(xi+1), w(yi), seed)
	c := hash2(w(xi), w(yi+1), seed)
	d := hash2(w(xi+1), w(yi+1), seed)
	return (a+(b-a)*sx)*(1-sy) + (c+(d-c)*sx)*sy
}

func fbm(x, y float64, octaves, period, seed int) float64 {
	sum, amp, norm := 0.0, 1.0, 0.0
	p := period
	for i := 0; i < octaves; i++ {
		sum += amp * vnoise(x*float64(p)/float64(period), y*float64(p)/float64(period), p, seed+i*17)
		norm += amp
		amp *= 0.5
		p *= 2
	}
	return sum / norm
}

// ----------------------------------------------------------------- canvas ---

type canvas struct {
	s    int
	px   []col
	seed int
	r    uint32
}

func newCanvas(s, seed int) *canvas {
	c := &canvas{s: s, px: make([]col, s*s), seed: seed, r: uint32(seed)*2654435761 + 1}
	return c
}

// rnd is a small xorshift; the drawing only ever needs cheap randomness.
func (c *canvas) rnd() float64 {
	c.r ^= c.r << 13
	c.r ^= c.r >> 17
	c.r ^= c.r << 5
	return float64(c.r) / 4294967295.0
}

func (c *canvas) rndi(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + int(c.rnd()*float64(hi-lo))
}

func (c *canvas) wrap(x, y int) (int, int) {
	return ((x % c.s) + c.s) % c.s, ((y % c.s) + c.s) % c.s
}

func (c *canvas) set(x, y int, v col) {
	x, y = c.wrap(x, y)
	c.px[y*c.s+x] = v
}

func (c *canvas) get(x, y int) col {
	x, y = c.wrap(x, y)
	return c.px[y*c.s+x]
}

func (c *canvas) blend(x, y int, v col, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	c.set(x, y, lerp(c.get(x, y), v, a))
}

func (c *canvas) fill(v col) {
	for i := range c.px {
		c.px[i] = v
	}
}

func (c *canvas) rect(x0, y0, x1, y1 int, v col) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y, v)
		}
	}
}

func (c *canvas) line(x0, y0, x1, y1 int, v col, w int) {
	dx, dy := x1-x0, y1-y0
	n := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy)))) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		x := x0 + int(math.Round(float64(dx)*t))
		y := y0 + int(math.Round(float64(dy)*t))
		for oy := 0; oy < w; oy++ {
			for ox := 0; ox < w; ox++ {
				c.set(x+ox, y+oy, v)
			}
		}
	}
}

func (c *canvas) disc(cx, cy int, r float64, v col) {
	ri := int(r) + 1
	for y := -ri; y <= ri; y++ {
		for x := -ri; x <= ri; x++ {
			d := math.Hypot(float64(x), float64(y))
			if d <= r {
				c.blend(cx+x, cy+y, v, clamp(r-d, 0, 1))
			}
		}
	}
}

// bevel draws a raised panel: the face, a lit top and left edge, a shadowed
// bottom and right. Everything metal in here is built out of these.
func (c *canvas) bevel(x0, y0, x1, y1 int, face col, lift float64) {
	c.rect(x0, y0, x1, y1, face)
	light := lerp(face, col{255, 255, 255}, 0.28*lift)
	dark := scale(face, 1-0.34*lift)
	for x := x0; x < x1; x++ {
		c.set(x, y0, light)
		c.set(x, y1-1, dark)
	}
	for y := y0; y < y1; y++ {
		c.set(x0, y, light)
		c.set(x1-1, y, dark)
	}
}

func (c *canvas) grain(p pal, amount float64, octaves int) {
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			n := fbm(float64(x)/float64(c.s), float64(y)/float64(c.s), octaves, 8, c.seed) - 0.5
			v := c.get(x, y)
			c.set(x, y, scale(v, 1+amount*n))
		}
	}
}

func (c *canvas) scratches(n int, v col, alpha float64) {
	for i := 0; i < n; i++ {
		x, y := c.rndi(0, c.s), c.rndi(0, c.s)
		l := c.rndi(4, c.s/3)
		ang := c.rnd() * math.Pi
		dx, dy := math.Cos(ang), math.Sin(ang)
		for j := 0; j < l; j++ {
			c.blend(x+int(float64(j)*dx), y+int(float64(j)*dy), v, alpha)
		}
	}
}

func (c *canvas) img() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, c.s, c.s))
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			out.SetRGBA(x, y, toRGBA(c.px[y*c.s+x]))
		}
	}
	return out
}

// ------------------------------------------------------------- generators ---

// Every generator paints a whole texture. They take the palette they must stay
// inside and a phase in [0,1) - the frame number for the animated sets, zero
// for everything else.

func genPlates(c *canvas, p pal, f float64) {
	c.fill(p.at(0.4))
	n := []int{2, 2, 4}[c.rndi(0, 3)]
	cell := c.s / n
	seam := scale(p.at(0.1), 0.8)
	for gy := 0; gy < n; gy++ {
		for gx := 0; gx < n; gx++ {
			face := scale(p.at(0.45), 0.9+0.2*c.rnd())
			c.bevel(gx*cell+1, gy*cell+1, (gx+1)*cell-1, (gy+1)*cell-1, face, 1)
			if cell >= 24 {
				b := lerp(face, col{255, 255, 255}, 0.35)
				for _, o := range [][2]int{{5, 5}, {cell - 6, 5}, {5, cell - 6}, {cell - 6, cell - 6}} {
					c.disc(gx*cell+o[0], gy*cell+o[1], 1.6, b)
				}
			}
		}
	}
	for i := 0; i <= n; i++ {
		c.line(i*cell, 0, i*cell, c.s, seam, 1)
		c.line(0, i*cell, c.s, i*cell, seam, 1)
	}
	c.grain(p, 0.10, 4)
	c.scratches(c.s/16, scale(p.at(0.7), 1.1), 0.25)
}

func genWall(c *canvas, p pal, f float64) {
	// Plain hull wall: a lightly shaded ground, a few tall panels seamed into
	// it, one or two recessed rectangles, a trim band across.
	c.fill(p.at(0.45))
	x := 0
	for x < c.s {
		w := c.rndi(c.s/5, c.s/2)
		tone := 0.4 + 0.18*c.rnd()
		for xx := x; xx < x+w; xx++ {
			t := float64(xx-x) / float64(w)
			shade := 1 + 0.07*math.Cos(t*math.Pi*2)
			cc := scale(p.at(tone), shade)
			for y := 0; y < c.s; y++ {
				c.set(xx, y, cc)
			}
		}
		c.line(x, 0, x, c.s, scale(p.at(0.18), 0.9), 1)
		c.line(x+1, 0, x+1, c.s, lerp(p.at(0.6), col{255, 255, 255}, 0.18), 1)
		x += w
	}
	// Recessed rectangles: lit on the bottom right, which is the opposite of a
	// raised panel and reads as a hollow.
	for i := c.rndi(1, 4); i > 0; i-- {
		w := c.rndi(c.s/5, c.s/2)
		h := c.rndi(c.s/6, c.s/2)
		rx := c.rndi(2, c.s-w-2)
		ry := c.rndi(2, c.s-h-2)
		c.rect(rx, ry, rx+w, ry+h, scale(p.at(0.38), 0.92))
		c.line(rx, ry, rx+w, ry, scale(p.at(0.2), 0.85), 1)
		c.line(rx, ry, rx, ry+h, scale(p.at(0.2), 0.85), 1)
		c.line(rx, ry+h-1, rx+w, ry+h-1, lerp(p.at(0.65), col{255, 255, 255}, 0.2), 1)
		c.line(rx+w-1, ry, rx+w-1, ry+h, lerp(p.at(0.65), col{255, 255, 255}, 0.2), 1)
	}
	if c.rnd() < 0.7 {
		by := c.rndi(c.s/5, c.s*3/4)
		h := c.s / 14
		c.bevel(0, by, c.s, by+h, p.at(0.55), 1)
		if c.rnd() < 0.4 {
			c.line(0, by+h/2, c.s, by+h/2, p.accN(0), 1)
		}
	}
	c.grain(p, 0.13, 5)
	c.scratches(c.s/8, scale(p.at(0.8), 1.05), 0.18)
}

func genRibbed(c *canvas, p pal, f float64) {
	w := c.s / []int{6, 8, 10}[c.rndi(0, 3)]
	horiz := c.rnd() < 0.25
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			u := x
			if horiz {
				u = y
			}
			t := float64(u%w) / float64(w)
			// Off-centre highlight: a cylinder lit from the upper left.
			shade := 0.55 + 0.75*math.Pow(math.Max(0, math.Sin(math.Pi*t)), 1.6)
			if t < 0.08 {
				shade *= 0.5
			}
			c.set(x, y, scale(p.at(0.5), shade))
		}
	}
	for i := 0; i < 3; i++ {
		y := c.rndi(0, c.s)
		c.line(0, y, c.s, y, scale(p.at(0.2), 0.9), 2)
	}
	c.grain(p, 0.07, 4)
}

func genConsole(c *canvas, p pal, f float64) {
	// A console is a wall of equipment: a metal ground, a frame around it, and
	// a column of racked panels that each show something different.
	c.fill(p.at(0.32))
	c.grain(p, 0.10, 5)
	frame := p.at(0.5)
	c.rect(0, 0, c.s, 3, frame)
	c.rect(0, c.s-3, c.s, c.s, frame)
	c.rect(0, 0, 3, c.s, frame)
	c.rect(c.s-3, 0, c.s, c.s, frame)
	c.line(0, 0, c.s, 0, lerp(frame, col{255, 255, 255}, 0.3), 1)
	c.line(0, c.s-1, c.s, c.s-1, scale(frame, 0.6), 1)

	rows := c.rndi(2, 4)
	y := 5
	for r := 0; r < rows && y < c.s-10; r++ {
		h := (c.s - 10) / rows
		cols := c.rndi(1, 4)
		x := 5
		for i := 0; i < cols; i++ {
			w := (c.s-10)/cols - 3
			c.bevel(x, y, x+w, y+h-3, p.at(0.46), 1)
			inx, iny := x+3, y+3
			inw, inh := w-6, h-9
			if inw < 8 || inh < 8 {
				x += w + 3
				continue
			}
			c.rect(inx, iny, inx+inw, iny+inh, scale(p.at(0.04), 0.7))
			switch c.rndi(0, 3) {
			case 0: // readout bars
				for yy := iny + 1; yy+2 < iny+inh; yy += 3 {
					bw := c.rndi(inw/4, inw-1)
					c.rect(inx+1, yy, inx+1+bw, yy+2, p.accN(c.rndi(0, 4)))
				}
			case 1: // indicator lamps
				for yy := iny + 1; yy+3 < iny+inh; yy += 4 {
					for xx := inx + 1; xx+3 < inx+inw; xx += 4 {
						if c.rnd() < 0.65 {
							c.rect(xx, yy, xx+3, yy+3, p.accN(c.rndi(0, 4)))
						}
					}
				}
			default: // a lit screen with a trace running across it
				glow := p.accN(c.rndi(0, 3))
				for yy := iny; yy < iny+inh; yy++ {
					for xx := inx; xx < inx+inw; xx++ {
						d := math.Hypot(float64(xx-inx-inw/2)/float64(inw), float64(yy-iny-inh/2)/float64(inh))
						c.set(xx, yy, scale(glow, 0.14+0.4*math.Max(0, 1-d*1.5)))
					}
				}
				py := iny + inh/2
				for xx := inx; xx < inx+inw; xx++ {
					v := py + int(float64(inh)/3*math.Sin(float64(xx)*0.4+f*6))
					c.set(xx, v, lerp(glow, col{255, 255, 255}, 0.4))
				}
			}
			// A row of buttons under the panel, where a hand would reach.
			for bx := inx; bx+3 < inx+inw && c.rnd() < 0.8; bx += 5 {
				c.rect(bx, y+h-6, bx+3, y+h-4, p.accN(c.rndi(0, 4)))
			}
			x += w + 3
		}
		y += h
	}
}

func genCircuit(c *canvas, p pal, f float64) {
	// Conduit runs stapled to a flat wall: recessed channels, some of them lit,
	// with a junction pad wherever a run ends.
	c.fill(p.at(0.42))
	c.grain(p, 0.12, 4)
	channel := scale(p.at(0.08), 0.9)
	rim := lerp(p.at(0.6), col{255, 255, 255}, 0.15)
	hot := p.accN(0)
	if luma(hot) < 60 {
		hot = p.acc(10)
	}
	routes := c.rndi(5, 10)
	for i := 0; i < routes; i++ {
		x, y := c.rndi(0, c.s), c.rndi(0, c.s)
		dir := c.rndi(0, 4)
		lit := c.rnd() < 0.4
		for step := 0; step < c.rndi(3, 8); step++ {
			l := c.rndi(c.s/8, c.s/2)
			nx, ny := x, y
			switch dir {
			case 0:
				nx += l
			case 1:
				ny += l
			case 2:
				nx -= l
			default:
				ny -= l
			}
			// A slight skew off the axis: the runs are not quite square.
			if c.rnd() < 0.35 {
				ny += l / 7
			}
			c.line(x, y, nx, ny, rim, 5)
			c.line(x+1, y+1, nx+1, ny+1, channel, 3)
			if lit {
				c.line(x+2, y+2, nx+2, ny+2, hot, 1)
			}
			x, y = nx, ny
			if c.rnd() < 0.5 {
				dir = (dir + 1) % 4
			} else {
				dir = (dir + 3) % 4
			}
		}
		c.bevel(x-4, y-4, x+5, y+5, p.at(0.55), 1)
		if c.rnd() < 0.5 {
			c.disc(x, y, 1.6, hot)
		}
	}
	c.scratches(c.s/14, scale(p.at(0.75), 1.05), 0.18)
}

func genHazard(c *canvas, p pal, f float64) {
	genWall(c, p, f)
	yellow := p.acc(48)
	dark := scale(p.at(0.08), 0.8)
	h := c.s / 3
	y0 := c.rndi(0, c.s-h)
	sw := c.s / 8
	for y := y0; y < y0+h; y++ {
		for x := 0; x < c.s; x++ {
			// 60 degrees, not the usual 45.
			u := x + (y-y0)*2/3
			if ((u%(sw*2))+sw*2)%(sw*2) < sw {
				c.set(x, y, yellow)
			} else {
				c.set(x, y, dark)
			}
		}
	}
	c.rect(0, y0-3, c.s, y0, p.at(0.55))
	c.rect(0, y0+h, c.s, y0+h+3, p.at(0.55))
	c.line(0, y0-3, c.s, y0-3, lerp(p.at(0.7), col{255, 255, 255}, 0.25), 1)
	c.line(0, y0+h+2, c.s, y0+h+2, scale(p.at(0.2), 0.9), 1)
	c.scratches(c.s/8, dark, 0.25)
	c.grain(p, 0.08, 5)
}

func genLava(c *canvas, p pal, f float64) {
	// Cooled crust broken by a web of something still hot underneath. The web
	// is the zero line of a noise field, and it is narrow - a crack, not a
	// glow - with the crust around it left coarse and dark.
	crust := p.at(0.12)
	hot := p.acc(24)
	core := lerp(hot, col{255, 240, 190}, 0.7)
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			u, v := float64(x)/float64(c.s), float64(y)/float64(c.s)
			// Warping the lookup bends the web off the noise grid.
			wu := u + 0.06*(fbm(u, v, 3, 6, c.seed+21)-0.5)
			wv := v + 0.06*(fbm(u, v, 3, 6, c.seed+22)-0.5) + f*0.03
			ridge := math.Abs(fbm(wu, wv, 4, 14, c.seed)*2 - 1)
			t := clamp((0.055-ridge)/0.055, 0, 1)
			// A second, finer web fills the plates in between.
			r2 := math.Abs(fbm(wu*2, wv*2, 3, 40, c.seed+5)*2 - 1)
			t = clamp(t+0.45*clamp((0.03-r2)/0.03, 0, 1), 0, 1)
			// Crust: coarse noise plus a per-pixel speckle, so it reads as
			// something baked and broken rather than an airbrush.
			grit := fbm(u, v, 3, 40, c.seed+11)*0.75 + 0.25*hash2(x, y, c.seed)
			cc := lerp(scale(crust, 0.5), lerp(crust, hot, 0.22), grit*grit)
			cc = lerp(cc, hot, math.Pow(t, 0.55))
			cc = lerp(cc, core, math.Pow(t, 3))
			c.set(x, y, cc)
		}
	}
}

func genPipes(c *canvas, p pal, f float64) {
	genWall(c, p, f)
	n := c.rndi(2, 5)
	for i := 0; i < n; i++ {
		r := c.rndi(c.s/16, c.s/7)
		pos := c.rndi(0, c.s)
		vertical := c.rnd() < 0.6
		tint := p.at(0.5)
		if c.rnd() < 0.35 {
			tint = lerp(tint, p.accN(i), 0.35)
		}
		for a := -r; a <= r; a++ {
			t := float64(a) / float64(r)
			shade := 0.45 + 0.8*math.Sqrt(math.Max(0, 1-t*t))*(0.75-0.35*t)
			cc := scale(tint, shade)
			for b := 0; b < c.s; b++ {
				if vertical {
					c.set(pos+a, b, cc)
				} else {
					c.set(b, pos+a, cc)
				}
			}
		}
		for b := c.rndi(0, 40); b < c.s; b += c.rndi(30, 55) {
			for a := -r - 2; a <= r+2; a++ {
				t := float64(a) / float64(r+2)
				cc := scale(tint, 0.5+0.9*math.Sqrt(math.Max(0, 1-t*t)))
				for k := 0; k < 4; k++ {
					if vertical {
						c.set(pos+a, b+k, cc)
					} else {
						c.set(b+k, pos+a, cc)
					}
				}
			}
		}
	}
	c.grain(p, 0.07, 4)
}

func genGlow(c *canvas, p pal, f float64) {
	// A light fixture or a screen seen from across a room: a hot centre in a
	// recessed housing.
	glow := p.accN(0)
	if luma(glow) < 90 {
		glow = lerp(p.at(1), p.acc(190), 0.5)
	}
	bg := scale(p.at(0.12), 0.7)
	cx, cy := float64(c.s)/2, float64(c.s)/2
	rad := float64(c.s) * (0.26 + 0.06*math.Sin(f*math.Pi*2))
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / rad
			t := math.Exp(-d * d * 1.9)
			cc := lerp(bg, glow, clamp(t*1.25, 0, 1))
			cc = lerp(cc, col{255, 255, 255}, clamp((t-0.7)*3, 0, 1))
			n := fbm(float64(x)/float64(c.s), float64(y)/float64(c.s), 3, 8, c.seed)
			c.set(x, y, scale(cc, 0.93+0.14*n))
		}
	}
	// Housing: a bevelled border and four corner brackets.
	b := c.s / 10
	c.bevel(0, 0, c.s, b, p.at(0.45), 1)
	c.bevel(0, c.s-b, c.s, c.s, p.at(0.45), 1)
	c.bevel(0, 0, b, c.s, p.at(0.45), 1)
	c.bevel(c.s-b, 0, c.s, c.s, p.at(0.45), 1)
	brk := scale(p.at(0.25), 0.9)
	for _, o := range [][2]int{{b, b}, {c.s - b - c.s/6, b}, {b, c.s - b - 2}, {c.s - b - c.s/6, c.s - b - 2}} {
		c.rect(o[0], o[1], o[0]+c.s/6, o[1]+2, brk)
	}
}

func genGrate(c *canvas, p pal, f float64) {
	back := scale(p.at(0.05), 0.7)
	frame := p.at(0.45)
	c.fill(frame)
	cell := c.s / []int{6, 8, 10}[c.rndi(0, 3)]
	hole := cell * 2 / 3
	round := c.rnd() < 0.5
	for gy := 0; gy < c.s; gy += cell {
		for gx := 0; gx < c.s; gx += cell {
			x0, y0 := gx+(cell-hole)/2, gy+(cell-hole)/2
			if round {
				c.disc(x0+hole/2, y0+hole/2, float64(hole)/2, back)
			} else {
				c.rect(x0, y0, x0+hole, y0+hole, back)
			}
			c.line(x0, y0-1, x0+hole, y0-1, scale(frame, 0.7), 1)
			c.line(x0, y0+hole, x0+hole, y0+hole, lerp(frame, col{255, 255, 255}, 0.2), 1)
		}
	}
	c.grain(p, 0.10, 5)
}

func genFoliage(c *canvas, p pal, f float64) {
	genWall(c, p, f)
	leaf := p.acc(110)
	if luma(leaf) > 150 {
		leaf = scale(leaf, 0.6)
	}
	strands := c.rndi(5, 11)
	for i := 0; i < strands; i++ {
		x := float64(c.rndi(0, c.s))
		y := float64(c.rndi(-c.s/4, c.s/3))
		vx := (c.rnd() - 0.5) * 0.8
		l := c.rndi(c.s/2, c.s)
		stem := scale(leaf, 0.4+0.2*c.rnd())
		for j := 0; j < l; j++ {
			vx = clamp(vx+(c.rnd()-0.5)*0.25, -1.1, 1.1)
			x += vx
			y++
			c.set(int(x), int(y), stem)
			c.set(int(x)+1, int(y), scale(stem, 0.75))
			if j%5 != 0 {
				continue
			}
			// A leaf is a blade: long across the stem, thin along it, and no
			// two the same shade of green.
			side := 1.0
			if c.rnd() < 0.5 {
				side = -1
			}
			ln := 3 + 5*c.rnd()
			lw := 1 + 1.6*c.rnd()
			tint := scale(leaf, 0.6+0.7*c.rnd())
			ang := side*(0.5+0.7*c.rnd()) - 0.3
			for t := 0.0; t < ln; t += 0.5 {
				w := lw * math.Sin(math.Pi*t/ln)
				px := x + t*math.Cos(ang)
				py := y + t*math.Sin(ang)
				for o := -w; o <= w; o += 0.5 {
					c.blend(int(px-o*math.Sin(ang)), int(py+o*math.Cos(ang)), tint, 0.9)
				}
			}
		}
	}
}

func genMarble(c *canvas, p pal, f float64) {
	// Stone: a dark mottled body with thin bright veins cutting through it at
	// one angle, and a scatter of pits.
	vein := p.at(0.9)
	base := p.at(0.28)
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			u, v := float64(x)/float64(c.s), float64(y)/float64(c.s)
			mottle := fbm(u, v, 5, 6, c.seed+3)
			cc := lerp(scale(base, 0.7), p.at(0.5), mottle)
			turb := fbm(u, v, 6, 8, c.seed)
			s := math.Sin((u*2.7 + v*1.3 + turb*3.2) * math.Pi * 2)
			t := math.Pow(math.Max(0, 1-math.Abs(s)*3.2), 6)
			cc = lerp(cc, vein, t)
			s2 := math.Sin((u*1.1 - v*3.4 + turb*2.1) * math.Pi * 2)
			cc = lerp(cc, scale(vein, 0.8), math.Pow(math.Max(0, 1-math.Abs(s2)*5), 8)*0.7)
			if hash2(x, y, c.seed) > 0.985 {
				cc = scale(cc, 0.5)
			}
			c.set(x, y, scale(cc, 0.93+0.14*fbm(u, v, 3, 32, c.seed+9)))
		}
	}
}

func genEmblem(c *canvas, p pal, f float64) {
	// A station marking: a plate cut to a polygon, an accent rim, and one mark
	// inside it. Which polygon and which mark is up to the seed.
	c.fill(p.at(0.3))
	c.grain(p, 0.10, 4)
	mid := c.s / 2
	edge := p.accN(0)
	if luma(edge) < 60 {
		edge = p.at(0.95)
	}
	face := p.at(0.62)
	sides := c.rndi(5, 9)
	r := float64(c.s) * 0.4
	squash := 0.72 + 0.22*c.rnd()
	rot := c.rnd() * 2 * math.Pi
	pts := make([][2]int, sides)
	for i := range pts {
		a := float64(i)/float64(sides)*2*math.Pi + rot
		pts[i] = [2]int{mid + int(r*math.Cos(a)), mid + int(r*squash*math.Sin(a))}
	}
	// Scanline fill of the polygon, so the shape and the outline agree.
	for y := 0; y < c.s; y++ {
		var xs []float64
		for i := range pts {
			j := (i + 1) % len(pts)
			y0, y1 := float64(pts[i][1]), float64(pts[j][1])
			if (y0 <= float64(y)) == (y1 <= float64(y)) {
				continue
			}
			t := (float64(y) - y0) / (y1 - y0)
			xs = append(xs, float64(pts[i][0])+t*(float64(pts[j][0])-float64(pts[i][0])))
		}
		sort.Float64s(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			for x := int(xs[k]); x <= int(xs[k+1]); x++ {
				d := math.Hypot(float64(x-mid)/r, float64(y-mid)/(r*squash))
				c.set(x, y, scale(face, 0.75+0.45*(1-d)))
			}
		}
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		c.line(pts[i][0], pts[i][1], pts[j][0], pts[j][1], edge, 2)
	}
	mark := scale(p.at(0.98), 1)
	switch c.rndi(0, 3) {
	case 0: // stacked chevrons
		for i := 0; i < 3; i++ {
			w := c.s/4 - i*c.s/16
			y := mid - c.s/8 + i*c.s/10
			c.line(mid-w, y, mid, y+c.s/12, mark, 2)
			c.line(mid+w, y, mid, y+c.s/12, mark, 2)
		}
	case 1: // concentric rings, broken at one side
		for i := 1; i <= 3; i++ {
			rr := r * 0.2 * float64(i)
			for a := 0.6; a < 2*math.Pi-0.2; a += 0.05 {
				c.set(mid+int(rr*math.Cos(a)), mid+int(rr*squash*math.Cos(a-math.Pi/2)), mark)
				c.set(mid+int(rr*math.Cos(a)), mid+int(rr*squash*math.Sin(a)), mark)
			}
		}
	default: // a bar and two blocks, like a hazard placard
		c.rect(mid-c.s/5, mid-2, mid+c.s/5, mid+2, mark)
		c.rect(mid-c.s/8, mid-c.s/6, mid-c.s/8+c.s/12, mid-c.s/6+c.s/12, edge)
		c.rect(mid+c.s/16, mid+c.s/12, mid+c.s/16+c.s/12, mid+c.s/12+c.s/12, edge)
	}
}

func genOctagon(c *canvas, p pal, f float64) {
	cell := c.s / 4
	c.fill(scale(p.at(0.15), 0.9))
	for gy := 0; gy < c.s; gy += cell {
		for gx := 0; gx < c.s; gx += cell {
			face := scale(p.at(0.5), 0.92+0.16*c.rnd())
			cut := cell / 4
			for y := 0; y < cell; y++ {
				for x := 0; x < cell; x++ {
					// The corner cut is uneven, so the tiles are not regular.
					if x+y < cut || (cell-x)+y < cut+2 || x+(cell-y) < cut+2 || (cell-x)+(cell-y) < cut {
						continue
					}
					shade := 1.0
					if y < 2 || x < 2 {
						shade = 1.2
					} else if y > cell-3 || x > cell-3 {
						shade = 0.75
					}
					c.set(gx+x, gy+y, scale(face, shade))
				}
			}
		}
	}
	c.grain(p, 0.08, 5)
	c.scratches(c.s/14, scale(p.at(0.7), 1.05), 0.2)
}

func genScreen(c *canvas, p pal, f float64) {
	// A wall of readout: a bordered black screen packed with lines of blocks
	// that stand in for text, plus one framed graph.
	c.fill(scale(p.at(0.03), 0.6))
	text := p.accN(0)
	if luma(text) < 70 {
		text = col{120, 240, 140}
	}
	b := 1
	if c.s > 48 {
		b = 2
	}
	c.rect(b, b, c.s-b, b+b, text)
	c.rect(b, c.s-b-b, c.s-b, c.s-b, text)
	c.rect(b, b, b+b, c.s-b, text)
	c.rect(c.s-b-b, b, c.s-b, c.s-b, text)
	pitch := 4
	if c.s > 48 {
		pitch = 6
	}
	off := int(f*float64(pitch)) % pitch
	for y := b + 2 + off; y < c.s-b-3; y += pitch {
		x := b + 2
		for x < c.s-b-3 {
			w := c.rndi(2, c.s/3)
			if x+w > c.s-b-3 {
				w = c.s - b - 3 - x
			}
			if w < 1 {
				break
			}
			c.rect(x, y, x+w, y+pitch-2, scale(text, 0.5+0.65*c.rnd()))
			x += w + c.rndi(2, 5)
		}
	}
	if c.s >= 64 {
		bx, by := c.rndi(6, c.s/2), c.rndi(6, c.s-c.s/3)
		c.rect(bx, by, bx+c.s/3, by+c.s/5, scale(p.at(0.1), 0.8))
		c.line(bx, by, bx+c.s/3, by, p.accN(1), 1)
		py := by + c.s/10
		for xx := bx; xx < bx+c.s/3; xx++ {
			c.set(xx, py+int(float64(c.s)/22*math.Sin(float64(xx)*0.5+f*6)), p.accN(1))
		}
	}
}

func genStatic(c *canvas, p pal, f float64) {
	seed := c.seed + int(f*97)
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			n := hash2(x, y, seed)
			cc := lerp(p.at(0.05), p.at(1), math.Pow(n, 0.8))
			if n > 0.985 {
				cc = p.accN(int(n * 1000))
			}
			c.set(x, y, cc)
		}
	}
	for i := 0; i < 3; i++ {
		y := c.rndi(0, c.s)
		c.rect(0, y, c.s, y+c.rndi(1, 4), scale(p.at(0.05), 0.5))
	}
}

func genMolecule(c *canvas, p pal, f float64) {
	c.fill(scale(p.at(0.02), 0.5))
	n := c.rndi(6, 10)
	xs := make([]int, n)
	ys := make([]int, n)
	for i := range xs {
		xs[i] = c.rndi(c.s/8, c.s*7/8)
		ys[i] = c.rndi(c.s/6, c.s*5/6)
	}
	bond := p.at(0.75)
	for i := 1; i < n; i++ {
		j := c.rndi(0, i)
		wob := int(f * 2)
		c.line(xs[i], ys[i]+wob, xs[j], ys[j], bond, 2)
	}
	for i := range xs {
		c.disc(xs[i], ys[i], float64(c.s)/22+1, p.accN(i))
	}
}

func genPlanet(c *canvas, p pal, f float64) {
	c.fill(scale(p.at(0.1), 0.8))
	c.grain(p, 0.2, 4)
	cx, cy := float64(c.s)/2, float64(c.s)*0.46
	r := float64(c.s) * 0.33
	land := p.acc(120)
	sea := p.acc(200)
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			dx, dy := (float64(x)-cx)/r, (float64(y)-cy)/r
			d2 := dx*dx + dy*dy
			if d2 > 1 {
				continue
			}
			z := math.Sqrt(1 - d2)
			lightAmt := clamp(0.25+0.9*(-dx*0.5-dy*0.5+z*0.7), 0, 1.4)
			n := fbm(dx*0.5+0.5+f*0.15, dy*0.5+0.5, 4, 6, c.seed)
			surf := sea
			if n > 0.52 {
				surf = land
			}
			cc := scale(surf, lightAmt)
			cc = lerp(cc, col{255, 255, 255}, clamp((0.06-d2*0.02)*(1-d2)*2, 0, 0.3))
			c.set(x, y, cc)
		}
	}
}

func genSparkle(c *canvas, p pal, f float64) {
	c.fill(scale(p.at(0.02), 0.4))
	cx, cy := c.s/2, c.s/2
	size := float64(c.s) * (0.55 - 0.45*f)
	bright := p.at(1)
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			dx, dy := math.Abs(float64(x-cx)), math.Abs(float64(y-cy))
			d := math.Hypot(dx, dy)
			spike := math.Exp(-dy*dy/2) + math.Exp(-dx*dx/2)
			t := math.Exp(-d*d/(size*size*0.4)) + 0.45*spike*math.Exp(-d/size)
			c.blend(x, y, bright, clamp(t, 0, 1))
		}
	}
}

func genGrowth(c *canvas, p pal, f float64) {
	// Warped tubes: fbm bent through itself, which reads as something grown
	// rather than built.
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			u, v := float64(x)/float64(c.s), float64(y)/float64(c.s)
			w1 := fbm(u+f*0.03, v, 4, 4, c.seed)
			w2 := fbm(u+w1*0.7, v+w1*0.5, 5, 6, c.seed+31)
			band := 0.5 + 0.5*math.Sin((w2*5+u*2)*math.Pi*2)
			cc := lerp(p.at(0.1), p.at(0.95), math.Pow(band, 1.7))
			if band > 0.93 {
				cc = lerp(cc, p.accN(0), 0.4)
			}
			c.set(x, y, cc)
		}
	}
}

func genPortrait(c *canvas, p pal, f float64) {
	// The reference set had video of a person here. This is a bio-scan
	// silhouette instead - head, neck, shoulders, scan lines - and no likeness.
	c.fill(scale(p.at(0.12), 0.9))
	skin := p.at(0.72)
	cx := float64(c.s) / 2
	cy := float64(c.s) * 0.42
	rx, ry := float64(c.s)*0.19, float64(c.s)*0.25
	shoulderY := cy + ry*1.5
	for y := int(shoulderY); y < c.s; y++ {
		w := float64(c.s) * (0.22 + 0.9*float64(y-int(shoulderY))/float64(c.s))
		for x := int(cx - w); x < int(cx+w); x++ {
			c.set(x, y, scale(skin, 0.4))
		}
	}
	for y := int(cy); y < int(shoulderY)+1; y++ {
		for x := int(cx - rx*0.42); x < int(cx+rx*0.42); x++ {
			c.set(x, y, scale(skin, 0.55))
		}
	}
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			dx, dy := (float64(x)-cx)/rx, (float64(y)-cy)/ry
			d2 := dx*dx + dy*dy
			if d2 >= 1 {
				continue
			}
			shade := 0.62 + 0.45*math.Sqrt(1-d2) - 0.22*dx
			if dy < -0.25 {
				shade *= 0.55 // hair
			}
			c.set(x, y, scale(skin, shade))
		}
	}
	for y := 0; y < c.s; y += 2 {
		for x := 0; x < c.s; x++ {
			c.set(x, y, scale(c.get(x, y), 0.85))
		}
	}
	sy := int(f * float64(c.s))
	for x := 0; x < c.s; x++ {
		c.blend(x, sy, p.accN(0), 0.55)
	}
	c.grain(p, 0.14, 5)
}

func genCrack(c *canvas, p pal, f float64) {
	// A screen taking a hit: a ragged white burst that grows over the frames.
	c.fill(scale(p.at(0.03), 0.5))
	shard := p.at(1)
	cx, cy := float64(c.s)*0.45, float64(c.s)*0.5
	spread := 0.14 + 0.42*f
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			dx, dy := (float64(x)-cx)/float64(c.s), (float64(y)-cy)/float64(c.s)
			u, v := float64(x)/float64(c.s), float64(y)/float64(c.s)
			warp := fbm(u, v, 4, 5, c.seed) - 0.5
			ang := math.Atan2(dy, dx)
			// A star-shaped edge: the radius wobbles with the angle.
			edge := spread * (0.55 + 0.5*math.Abs(math.Sin(ang*3.5+float64(c.seed%7))) + warp)
			if math.Hypot(dx, dy*1.25) < edge {
				c.set(x, y, shard)
			}
		}
	}
	for i := 0; i < 10; i++ {
		a := c.rnd() * 2 * math.Pi
		l := float64(c.s) * spread * (1.1 + 1.4*c.rnd())
		x, y := cx, cy
		for t := 0.0; t < l; t++ {
			a += (c.rnd() - 0.5) * 0.35
			x += math.Cos(a)
			y += math.Sin(a)
			c.set(int(x), int(y), shard)
		}
	}
	if len(p.accent) > 0 {
		c.disc(int(cx), int(cy), float64(c.s)*spread*0.25, p.accN(0))
	}
}

// grime eats a texture the way the reference set's damaged walls do: a bloom
// of corrosion pulled out of shape by noise, pitted through to the dark
// underneath, with drips running out of the bottom of it.
func grime(c *canvas, p pal) {
	rust := lerp(scale(p.at(0.1), 0.9), p.acc(20), 0.45)
	cx, cy := c.rnd()*float64(c.s), c.rnd()*float64(c.s)
	rx := float64(c.s) * (0.35 + 0.3*c.rnd())
	ry := rx * (0.55 + 0.9*c.rnd())
	for y := 0; y < c.s; y++ {
		for x := 0; x < c.s; x++ {
			dx := math.Abs(float64(x) - cx)
			dy := math.Abs(float64(y) - cy)
			dx = math.Min(dx, float64(c.s)-dx) / rx
			dy = math.Min(dy, float64(c.s)-dy) / ry
			u, v := float64(x)/float64(c.s), float64(y)/float64(c.s)
			warp := fbm(u, v, 4, 4, c.seed+13) - 0.5
			d := math.Hypot(dx, dy) + warp*0.9
			n := fbm(u, v, 5, 6, c.seed+77)
			// The mask is broken up by a second, finer noise, so the edge
			// crumbles into speckle instead of ending in a hard silhouette.
			fine := fbm(u*3, v*3, 4, 24, c.seed+41)
			m := clamp((1-d)*2.1*(0.4+n)-0.2, 0, 1) * (0.3 + 1.15*fine)
			if m <= 0.02 {
				continue
			}
			c.blend(x, y, scale(rust, 0.45+0.8*n), clamp(math.Pow(m, 0.7), 0, 0.92))
			if m > 0.55 && fine < 0.4 {
				c.blend(x, y, scale(p.at(0.06), 0.8), 0.55)
			}
		}
	}
	for i := 0; i < 10; i++ {
		x := int(cx) + int((c.rnd()-0.5)*2*rx)
		y0 := int(cy + ry*0.4*c.rnd())
		l := c.rndi(c.s/8, c.s/2)
		for j := 0; j < l; j++ {
			c.blend(x, y0+j, scale(rust, 0.5), 0.4*(1-float64(j)/float64(l)))
		}
	}
}

// ------------------------------------------------------------------ table ---

type generator func(*canvas, pal, float64)

var kinds = map[string]generator{
	"plates":   genPlates,
	"wall":     genWall,
	"ribbed":   genRibbed,
	"console":  genConsole,
	"circuit":  genCircuit,
	"hazard":   genHazard,
	"lava":     genLava,
	"pipes":    genPipes,
	"glow":     genGlow,
	"grate":    genGrate,
	"foliage":  genFoliage,
	"marble":   genMarble,
	"emblem":   genEmblem,
	"octagon":  genOctagon,
	"screen":   genScreen,
	"static":   genStatic,
	"molecule": genMolecule,
	"planet":   genPlanet,
	"sparkle":  genSparkle,
	"growth":   genGrowth,
	"portrait": genPortrait,
	"crack":    genCrack,
}

// span says what the numbered wall textures are: which surface, and whether
// this one is a damaged copy of it.
type span struct {
	lo, hi int
	kind   string
	dirty  bool
}

var spans = []span{
	{1, 1, "plates", false}, {2, 2, "plates", true},
	{3, 4, "console", false}, {5, 5, "lava", false}, {6, 8, "console", false},
	{9, 9, "ribbed", false}, {10, 10, "circuit", false}, {11, 11, "ribbed", false},
	{12, 12, "console", false}, {13, 14, "circuit", false}, {15, 17, "hazard", false},
	{18, 18, "wall", false}, {19, 19, "pipes", false}, {20, 20, "glow", false},
	{21, 22, "console", false}, {23, 23, "grate", false}, {24, 25, "circuit", false},
	{26, 26, "wall", false}, {27, 31, "console", false}, {32, 32, "wall", false},
	{33, 35, "glow", false}, {36, 37, "wall", false}, {38, 43, "console", false},
	{44, 47, "wall", true}, {48, 48, "wall", false}, {49, 49, "glow", false},
	{50, 50, "plates", true}, {51, 51, "grate", false}, {52, 52, "console", true},
	{53, 53, "wall", false}, {54, 54, "glow", false}, {55, 55, "plates", true},
	{56, 57, "console", false}, {58, 58, "wall", false}, {59, 59, "emblem", false},
	{60, 60, "wall", false}, {61, 61, "wall", true}, {62, 63, "console", false},
	{64, 67, "console", true}, {68, 69, "wall", false}, {70, 71, "circuit", false},
	{72, 72, "wall", true}, {73, 75, "console", false}, {76, 76, "planet", false},
	{77, 77, "planet", true}, {78, 78, "octagon", false}, {79, 79, "plates", true},
	{80, 80, "plates", false}, {81, 81, "plates", true}, {82, 83, "wall", false},
	{84, 85, "pipes", false}, {86, 88, "console", false}, {89, 89, "wall", true},
	{90, 90, "pipes", false}, {91, 91, "pipes", true}, {92, 94, "grate", false},
	{95, 96, "glow", false}, {97, 98, "hazard", false}, {99, 103, "wall", false},
	{104, 109, "hazard", false}, {110, 110, "wall", false}, {111, 112, "marble", false},
	{113, 113, "circuit", false}, {114, 114, "plates", true}, {115, 115, "console", false},
	{116, 116, "console", true}, {117, 120, "circuit", false}, {121, 122, "wall", true},
	{123, 123, "screen", false}, {124, 124, "wall", false}, {125, 127, "ribbed", false},
	{128, 128, "circuit", false}, {129, 129, "plates", false}, {130, 130, "glow", false},
	{131, 132, "emblem", false}, {133, 134, "marble", false}, {135, 136, "emblem", false},
	{137, 137, "ribbed", false}, {138, 138, "glow", false}, {139, 139, "grate", true},
	{140, 140, "wall", false}, {141, 143, "console", false}, {144, 144, "console", true},
	{145, 149, "console", false}, {150, 150, "wall", false}, {151, 154, "console", true},
	{155, 156, "console", false}, {157, 158, "console", true}, {159, 160, "emblem", false},
	{161, 161, "marble", false}, {162, 168, "console", false}, {169, 169, "glow", false},
	{170, 170, "grate", false}, {171, 177, "console", false}, {178, 181, "console", true},
	{182, 182, "wall", false}, {183, 193, "foliage", false}, {194, 195, "glow", false},
	{196, 197, "octagon", false}, {198, 198, "plates", false}, {199, 199, "plates", true},
	{200, 201, "plates", false}, {202, 202, "plates", true}, {203, 203, "plates", false},
	{204, 204, "plates", true}, {205, 205, "plates", false}, {206, 206, "plates", true},
	{207, 207, "glow", false}, {208, 208, "marble", false}, {209, 214, "lava", false},
	{215, 215, "ribbed", false}, {216, 217, "emblem", false}, {218, 218, "screen", false},
	{219, 219, "grate", false}, {220, 220, "wall", false}, {221, 221, "emblem", false},
	{222, 222, "screen", false}, {223, 225, "emblem", false}, {226, 226, "console", false},
	{227, 228, "marble", false}, {229, 240, "circuit", false},
}

// prefixes covers the animated sets, which are named by what they are.
var prefixes = []struct {
	prefix string
	kind   string
}{
	{"CMP", "screen"}, {"TEXT", "screen"}, {"STAT", "static"}, {"PLN", "planet"},
	{"MOLECUL", "molecule"}, {"LOCK", "console"}, {"FACE", "portrait"},
	{"GROWTH", "growth"}, {"OFF", "sparkle"}, {"SCR_BRK", "crack"}, {"LOGO", "emblem"},
}

var trailingDigits = func(s string) (int, string) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0, s
	}
	n, _ := strconv.Atoi(s[i:])
	return n, strings.TrimRight(s[:i], "_")
}

// recipe picks the surface and the frame phase for one texture name.
func recipe(name string) (generator, bool, float64) {
	num, base := trailingDigits(name)
	phase := 0.0
	if num > 0 {
		phase = float64((num-1)%8) / 8
	}
	if strings.HasPrefix(base, "SSTX") {
		for _, s := range spans {
			if num >= s.lo && num <= s.hi {
				return kinds[s.kind], s.dirty, 0
			}
		}
		return genWall, false, 0
	}
	// CMP1_2 -> base "CMP1"; strip the set number too.
	trimmed := strings.TrimRight(base, "0123456789")
	for _, pre := range prefixes {
		if strings.HasPrefix(trimmed, pre.prefix) || strings.HasPrefix(base, pre.prefix) {
			return kinds[pre.kind], false, phase
		}
	}
	return genWall, false, phase
}

func seedOf(name string) int {
	h := uint32(2166136261)
	for i := 0; i < len(name); i++ {
		h = (h ^ uint32(name[i])) * 16777619
	}
	return int(h & 0x7fffffff)
}

// ------------------------------------------------------------------- main ---

func main() {
	src := flag.String("src", "", "folder of reference PNGs; only their palettes and names are read")
	out := flag.String("out", "assets/textures/station", "folder to write the generated set into")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "gentex: -src is required")
		os.Exit(2)
	}
	entries, err := os.ReadDir(*src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gentex:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gentex:", err)
		os.Exit(1)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		f, err := os.Open(filepath.Join(*src, e.Name()))
		if err != nil {
			fmt.Fprintln(os.Stderr, "gentex:", err)
			os.Exit(1)
		}
		ref, err := png.Decode(f)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "gentex:", e.Name(), err)
			os.Exit(1)
		}
		p := extract(ref)
		size := ref.Bounds().Dx()
		gen, dirty, phase := recipe(name)
		c := newCanvas(size, seedOf(name))
		gen(c, p, phase)
		if dirty {
			grime(c, p)
		}
		w, err := os.Create(filepath.Join(*out, name+".png"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "gentex:", err)
			os.Exit(1)
		}
		if err := png.Encode(w, c.img()); err != nil {
			fmt.Fprintln(os.Stderr, "gentex:", err)
			os.Exit(1)
		}
		w.Close()
		n++
	}
	fmt.Printf("gentex: wrote %d textures to %s\n", n, *out)
}
