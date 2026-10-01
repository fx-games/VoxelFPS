//go:build js

package main

// The browser build reads its assets out of the binary. os on js/wasm reads
// files through Node's fs API; play/index.html implements the read-only half of
// it in JavaScript over the table init() hands it here: every embedded file's
// path and where its bytes sit in wasm memory. Nothing is copied, and no Go
// code runs inside a file read (Go called back from inside a syscall/js call
// corrupted the heap under GC).

import (
	"embed"
	"fmt"
	"os"
	"strings"
	"syscall/js"
	"unsafe"
)

// What the default level (the Ark) touches, plus the weapons keys.txt binds.
// Left out: the other levels, the other 140-odd weapons, the soundfont (no
// music on js), and the texture library and skyboxes the editor browses.
//
//go:embed assets/base assets/maps/ark assets/skybox/blink assets/meshes assets/dialogue
//go:embed assets/sprites/items assets/sprites/props assets/sprites/actors/*/*/*.json assets/sprites/actors/humans/scientist assets/sprites/actors/humans/scientist_hologram
//go:embed all:assets/weapons/_actors assets/weapons/_sprites.png assets/weapons/keys.txt
//go:embed assets/weapons/small_gun assets/weapons/bd_fists assets/weapons/bd_axe assets/weapons/colt45
//go:embed assets/weapons/blacktail_51_connector assets/weapons/fd_borstal_shotgun assets/weapons/fd_machine_gun
//go:embed assets/weapons/bd_plasma_rifle assets/weapons/bd_rocket_launcher
var webAssets embed.FS

// webStubs lists, as "size path" lines, files that are looked at but never read
// in an Ark session: the sheets of actors the Ark does not place and sounds (the sprite loader
// checks they exist) and the shared weapon pictures (the packed atlas is keyed by
// their names and sizes). They are mounted with their size and no bytes, which
// saves 26 MB. build_web.sh writes the list.
//
//go:embed web_stubs.txt
var webStubs string

// embedFile is how the toolchain lays out an embed.FS entry: a name (ending in
// "/" for a directory) and the bytes as a string in the data segment.
type embedFile struct {
	name, data string
	hash       [16]byte
}

func init() {
	files := **(**[]embedFile)(unsafe.Pointer(&webAssets))
	table := make([]any, 0, 3*len(files))
	for _, f := range files {
		table = append(table, f.name, uintptr(unsafe.Pointer(unsafe.StringData(f.data))), len(f.data))
	}
	for _, line := range strings.Split(strings.TrimSpace(webStubs), "\n") {
		var size int
		var name string
		if n, _ := fmt.Sscanf(line, "%d %s", &size, &name); n == 2 {
			table = append(table, name, -1, size)
		}
	}
	js.Global().Call("__mountFS", table)

	// The page's query string is the command line: ?at=16,1.6,3&weapon=colt45
	// runs as -at=16,1.6,3 -weapon=colt45.
	params := js.Global().Get("Array").Call("from", js.Global().Get("URLSearchParams").New(js.Global().Get("location").Get("search")))
	for i := 0; i < params.Length(); i++ {
		os.Args = append(os.Args, "-"+params.Index(i).Index(0).String()+"="+params.Index(i).Index(1).String())
	}
}
