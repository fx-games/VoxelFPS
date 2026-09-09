// Command seedemissive writes the emissive slot list into an atlas PNG's emIS
// chunk. A one-off: it put the list that used to be hardcoded in main.go into
// assets/base/map_64x64.png, which is where the game now reads it from. Kept
// because it is also the only way to set the list without the running editor.
//
//	go run ./tools/seedemissive assets/base/map_64x64.png 19,26,27,29,30,45,46,47,49
package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/memmaker/goray/png_chunks"
	"github.com/memmaker/goray/quads"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: seedemissive <atlas.png> <comma separated slots>")
		os.Exit(2)
	}
	path, list := os.Args[1], os.Args[2]
	existing, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	if err := png_chunks.InjectTextChunkIntoPNG(bytes.NewReader(existing), &out, quads.EmissiveChunkKey, list); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, out.Bytes(), 0644); err != nil {
		panic(err)
	}
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	fmt.Printf("%s %s = %q\n", path, quads.EmissiveChunkKey,
		png_chunks.ReadTextChunkFromPNG(file, quads.EmissiveChunkKey))
}
