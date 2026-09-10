package main

// VSWAP is the Wolf3D-family art container Blake Stone still uses: one flat file
// of chunks, split into three runs by two indices in its own header - walls
// first, then sprites, then pages of digitized sound. Nothing in the file says
// which chunk is which thing beyond those two boundaries; the names come from
// bstone's own tables (see names.go).

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
)

// wallDim is the side of a wall page. Every wall is exactly this, uncompressed,
// stored column-major - which is why the loops below look transposed.
const wallDim = 64

// spriteDim is the side of the square a sprite is drawn into. The stored art is
// only the columns that have anything in them; leftPix/rightPix say where in the
// square those columns sit.
const spriteDim = 64

// pageSize is the size of one sound page. A sound is a run of whole pages and a
// byte count that says how much of the last one is real.
const pageSize = 4096

// pcmRate is the sample rate every digitized sound in the file was recorded at.
// It is not stored anywhere - it is a property of the engine, not of the file.
const pcmRate = 7000

type vswap struct {
	data        []byte
	offsets     []uint32
	lengths     []uint16
	spriteStart int
	soundStart  int
}

func openVSWAP(path string) (*vswap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 6 {
		return nil, fmt.Errorf("%s: too short for a header", path)
	}
	n := int(binary.LittleEndian.Uint16(data[0:]))
	v := &vswap{
		data:        data,
		spriteStart: int(binary.LittleEndian.Uint16(data[2:])),
		soundStart:  int(binary.LittleEndian.Uint16(data[4:])),
		offsets:     make([]uint32, n),
		lengths:     make([]uint16, n),
	}
	if 6+6*n > len(data) {
		return nil, fmt.Errorf("%s: header claims %d chunks, file holds %d bytes", path, n, len(data))
	}
	for i := 0; i < n; i++ {
		v.offsets[i] = binary.LittleEndian.Uint32(data[6+4*i:])
		v.lengths[i] = binary.LittleEndian.Uint16(data[6+4*n+2*i:])
	}
	if v.spriteStart > v.soundStart || v.soundStart > n {
		return nil, fmt.Errorf("%s: nonsense boundaries %d/%d in %d chunks", path, v.spriteStart, v.soundStart, n)
	}
	return v, nil
}

func (v *vswap) chunk(i int) []byte {
	if i < 0 || i >= len(v.offsets) || v.offsets[i] == 0 {
		return nil
	}
	return v.data[v.offsets[i] : v.offsets[i]+uint32(v.lengths[i])]
}

func (v *vswap) wallCount() int   { return v.spriteStart }
func (v *vswap) spriteCount() int { return v.soundStart - v.spriteStart }

// wall decodes one wall page. The source is column-major, so the destination
// index is the transpose of the read order.
func (v *vswap) wall(i int, pal color.Palette) (*image.RGBA, error) {
	src := v.chunk(i)
	if len(src) < wallDim*wallDim {
		return nil, fmt.Errorf("wall %d: %d bytes, want %d", i, len(src), wallDim*wallDim)
	}
	img := image.NewRGBA(image.Rect(0, 0, wallDim, wallDim))
	for x := 0; x < wallDim; x++ {
		for y := 0; y < wallDim; y++ {
			img.Set(x, y, pal[src[x*wallDim+y]])
		}
	}
	return img, nil
}

// sprite decodes one sprite page. The art is stored as a list of vertical posts
// per column: a column's command list is triples of (endY, dataOffset, startY),
// terminated by a zero endY, and the pixels a post covers live at
// chunk[dataOffset+y] - an offset that is relative to the chunk and deliberately
// lets several posts share one run of bytes. Everything not covered by a post is
// transparent, which is the whole point of the format.
func (v *vswap) sprite(i int, pal color.Palette) (*image.RGBA, error) {
	src := v.chunk(v.spriteStart + i)
	if len(src) < 4 {
		return nil, fmt.Errorf("sprite %d: %d bytes, want at least 4", i, len(src))
	}
	left := int(binary.LittleEndian.Uint16(src[0:]))
	right := int(binary.LittleEndian.Uint16(src[2:]))
	if left > right || right >= spriteDim {
		return nil, fmt.Errorf("sprite %d: columns %d..%d outside 0..%d", i, left, right, spriteDim-1)
	}
	img := image.NewRGBA(image.Rect(0, 0, spriteDim, spriteDim))
	for x := left; x <= right; x++ {
		at := 4 + 2*(x-left)
		if at+2 > len(src) {
			return nil, fmt.Errorf("sprite %d: column %d has no offset", i, x)
		}
		cmd := int(binary.LittleEndian.Uint16(src[at:]))
		for {
			// The terminator is two bytes, not six: a column that ends flush
			// with the chunk has nothing after its zero.
			if cmd+2 > len(src) {
				return nil, fmt.Errorf("sprite %d: column %d runs off the chunk", i, x)
			}
			endY := int(binary.LittleEndian.Uint16(src[cmd:])) / 2
			if endY == 0 {
				break
			}
			if cmd+6 > len(src) {
				return nil, fmt.Errorf("sprite %d: column %d has a post with no body", i, x)
			}
			base := int(int16(binary.LittleEndian.Uint16(src[cmd+2:])))
			startY := int(binary.LittleEndian.Uint16(src[cmd+4:])) / 2
			if startY > endY || endY > spriteDim {
				return nil, fmt.Errorf("sprite %d: column %d post %d..%d", i, x, startY, endY)
			}
			for y := startY; y < endY; y++ {
				at := base + y
				if at < 0 || at >= len(src) {
					return nil, fmt.Errorf("sprite %d: column %d reads byte %d of %d", i, x, at, len(src))
				}
				img.Set(x, y, pal[src[at]])
			}
			cmd += 6
		}
	}
	return img, nil
}

// digitized is the table in the file's last chunk: for every digitized sound, the
// page it starts on (counted from soundStart) and how many bytes of it are real.
type digitized struct {
	page, size int
}

func (v *vswap) digitizedTable() []digitized {
	last := v.chunk(len(v.offsets) - 1)
	out := make([]digitized, len(last)/4)
	for i := range out {
		out[i] = digitized{
			page: int(binary.LittleEndian.Uint16(last[4*i:])),
			size: int(binary.LittleEndian.Uint16(last[4*i+2:])),
		}
	}
	return out
}

// samples returns one sound's bytes: unsigned 8-bit mono, read straight out of
// the pages it spans. The pages are contiguous chunks, so this is one slice.
func (v *vswap) samples(d digitized) []byte {
	first := v.soundStart + d.page
	if first >= len(v.offsets) || v.offsets[first] == 0 {
		return nil
	}
	start := int(v.offsets[first])
	if start+d.size > len(v.data) {
		return nil
	}
	return v.data[start : start+d.size]
}
