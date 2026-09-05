package zdwep

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strings"
)

// A mod takes the game's own data for granted: Final Doomer's weapons spawn
// stock bullet puffs and never ship one. That data lives in an IWAD, which is
// not a zip - it is Doom's own container, a header and a directory of lumps -
// and its pictures are in Doom's own format rather than PNG.
//
// This reads one, and hands its sprites out as PNG so that everything
// downstream neither knows nor cares where a picture came from.

const (
	wadHeaderSize = 12
	wadEntrySize  = 16
	// A Doom picture is a column of runs rather than a rectangle of pixels,
	// which is how a sprite gets its transparent edges.
	postEnd = 0xFF
)

// openWAD reads an IWAD or PWAD as a source. Only its sprites are exposed,
// under the sprites/ prefix the rest of the migration expects, because the
// stock sounds are in a format nothing here decodes and a mod that wants one
// ships its own.
func openWAD(location string) (*Source, error) {
	data, err := os.ReadFile(location)
	if err != nil {
		return nil, err
	}
	if len(data) < wadHeaderSize {
		return nil, fmt.Errorf("%s: too short to be a wad", location)
	}
	magic := string(data[0:4])
	if magic != "IWAD" && magic != "PWAD" {
		return nil, fmt.Errorf("%s: not a wad", location)
	}
	count := int(binary.LittleEndian.Uint32(data[4:8]))
	directory := int(binary.LittleEndian.Uint32(data[8:12]))
	if directory+count*wadEntrySize > len(data) || count < 0 {
		return nil, fmt.Errorf("%s: its directory runs off the end", location)
	}

	lumps := make(map[string][]byte, count)
	palette := paletteFrom(nil)
	source := newSource(func() error { return nil })

	// Sprites sit between marker lumps. Everything outside them is map data,
	// music, wall textures and the like, which the weapons do not reach for.
	inSprites := false
	for i := 0; i < count; i++ {
		entry := data[directory+i*wadEntrySize:]
		start := int(binary.LittleEndian.Uint32(entry[0:4]))
		size := int(binary.LittleEndian.Uint32(entry[4:8]))
		name := strings.ToUpper(strings.TrimRight(string(entry[8:16]), "\x00 "))
		if start < 0 || size < 0 || start+size > len(data) {
			continue
		}
		body := data[start : start+size]

		switch name {
		case "PLAYPAL":
			palette = paletteFrom(body)
			continue
		case "S_START", "SS_START":
			inSprites = true
			continue
		case "S_END", "SS_END":
			inSprites = false
			continue
		}
		if !inSprites || size == 0 {
			continue
		}
		path := "sprites/" + name
		if _, taken := lumps[path]; taken {
			continue // a later wad's lump wins in Doom; the first wins here
		}
		lumps[path] = body
		source.note(path)
	}

	source.layers = []func(string) (io.ReadCloser, error){
		func(real string) (io.ReadCloser, error) {
			body, ok := lumps[real]
			if !ok {
				return nil, fmt.Errorf("%s: no such lump", real)
			}
			encoded, err := pictureToPNG(body, palette)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", real, err)
			}
			return io.NopCloser(bytes.NewReader(encoded)), nil
		},
	}
	return source, nil
}

// paletteFrom reads PLAYPAL's first palette, or falls back to greys when the
// wad has none, so a picture still comes out readable rather than blank.
func paletteFrom(playpal []byte) color.Palette {
	palette := make(color.Palette, 256)
	for i := range palette {
		if len(playpal) >= 768 {
			palette[i] = color.RGBA{playpal[i*3], playpal[i*3+1], playpal[i*3+2], 255}
			continue
		}
		palette[i] = color.RGBA{uint8(i), uint8(i), uint8(i), 255}
	}
	return palette
}

// pictureToPNG turns one Doom picture into a PNG. The format is a header, then
// one offset per column, then for each column a run of posts: where it starts
// down the column, how many pixels, and the pixels. Anything no post covers is
// transparent, which is how a sprite has a shape.
func pictureToPNG(lump []byte, palette color.Palette) ([]byte, error) {
	if len(lump) < 8 {
		return nil, fmt.Errorf("too short to be a picture")
	}
	width := int(int16(binary.LittleEndian.Uint16(lump[0:2])))
	height := int(int16(binary.LittleEndian.Uint16(lump[2:4])))
	if width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return nil, fmt.Errorf("%dx%d is not a picture", width, height)
	}
	if len(lump) < 8+width*4 {
		return nil, fmt.Errorf("its column table runs off the end")
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		offset := int(binary.LittleEndian.Uint32(lump[8+x*4:]))
		for offset+1 < len(lump) {
			top := int(lump[offset])
			if top == postEnd {
				break
			}
			length := int(lump[offset+1])
			// One padding byte before the pixels and one after, left over from
			// how the original engine read them.
			first := offset + 3
			if first+length > len(lump) {
				break
			}
			for i := 0; i < length; i++ {
				y := top + i
				if y < 0 || y >= height {
					continue
				}
				img.Set(x, y, palette[lump[first+i]])
			}
			offset = first + length + 1
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
