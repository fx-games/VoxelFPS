// Command gensheet builds a weapon folder out of a plain grid sprite sheet, for
// weapons that were never in a Doom mod.
//
//	go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
//	    -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 3
//
// The format does not care where a weapon came from, and this is the proof: what
// it writes is the same weapon.json the migration writes, only by hand. The work
// is wep.WriteGridWeapon's, which the DOS-game importers call directly for the
// same reason - a Wolfenstein-family weapon is a grid of frames and nothing else.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"

	"github.com/memmaker/goray/wep"
)

func main() {
	sheet := flag.String("sheet", "", "the sprite sheet to cut up")
	id := flag.String("id", "", "the weapon's folder name")
	tag := flag.String("tag", "", "the name the player sees")
	prefix := flag.String("prefix", "", "four letters to name the frames with, Doom style")
	size := flag.String("size", "64x64", "one frame's size, as WxH")
	frames := flag.String("frames", "", "the frames of the firing animation, in order, by index")
	tics := flag.Int("tics", 2, "how long each firing frame lasts")
	scale := flag.Int("scale", 1, "magnify every frame this many times, nearest neighbour")
	damage := flag.Int("damage", 5, "damage per shot")
	outDir := flag.String("out", "assets/weapons", "the weapons folder to write into")
	flag.Parse()

	if *sheet == "" || *id == "" || *prefix == "" || *frames == "" {
		fail("need at least -sheet, -id, -prefix and -frames")
	}
	if len(*prefix) != 4 {
		fail("-prefix has to be four letters, the way Doom names sprites")
	}

	frameWidth, frameHeight := parseSize(*size)
	spec := wep.GridWeapon{
		Sheet:      readImage(*sheet),
		FrameW:     frameWidth,
		FrameH:     frameHeight,
		Frames:     parseFrames(*frames),
		ID:         *id,
		Tag:        *tag,
		Prefix:     *prefix,
		Tics:       *tics,
		Scale:      *scale,
		Damage:     *damage,
		Source:     *sheet,
		SourceFile: *sheet,
	}
	if err := wep.WriteGridWeapon(*outDir, spec); err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s/%s: %d frames\n", *outDir, *id, len(spec.Frames))
}

func readImage(path string) image.Image {
	file, err := os.Open(path)
	if err != nil {
		fail("%v", err)
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		fail("%s: %v", path, err)
	}
	return decoded
}

func parseSize(text string) (int, int) {
	width, height, found := strings.Cut(strings.ToLower(text), "x")
	if !found {
		fail("-size wants WxH, got %q", text)
	}
	w, err := strconv.Atoi(width)
	if err != nil {
		fail("-size: %v", err)
	}
	h, err := strconv.Atoi(height)
	if err != nil {
		fail("-size: %v", err)
	}
	return w, h
}

func parseFrames(text string) []int {
	var indices []int
	for _, field := range strings.Split(text, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			fail("-frames: %v", err)
		}
		indices = append(indices, value)
	}
	if len(indices) == 0 {
		fail("-frames named none")
	}
	return indices
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gensheet: "+format+"\n", args...)
	os.Exit(1)
}
