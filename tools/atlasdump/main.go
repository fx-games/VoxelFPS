package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/memmaker/voxellib/tpack"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	img, meta, err := tpack.DecodeTrimmedAtlas(f)
	if err != nil {
		panic(err)
	}
	filter := ""
	if len(os.Args) > 2 {
		filter = strings.ToUpper(os.Args[2])
	}
	fmt.Println("atlas", img.Bounds(), "frames", len(meta))
	for i, fr := range meta {
		name := string(fr.SpriteName)
		if filter != "" && !strings.Contains(strings.ToUpper(name), filter) {
			continue
		}
		fmt.Printf("%3d %-12s at=%d,%d src=%dx%d trim=%d,%d %dx%d off=%d,%d\n", i, name,
			fr.X, fr.Y, fr.SourceWidth, fr.SourceHeight,
			fr.TrimRectX, fr.TrimRectY, fr.TrimRectWidth, fr.TrimRectHeight, fr.OffsetX, fr.OffsetY)
	}
}
