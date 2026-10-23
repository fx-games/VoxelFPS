package eassets

import (
	"embed"
	"github.com/memmaker/voxelfps/quads"
	"io/fs"
)

//go:embed files/*
var assets embed.FS

// Embedded serves the assets baked into the binary.
var Embedded = quads.Assets{FS: subDir()}

func subDir() fs.FS {
	files, err := fs.Sub(assets, "files")
	if err != nil {
		panic(err)
	}
	return files
}
