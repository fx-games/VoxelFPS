package wep

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"

	"github.com/memmaker/goray/png_chunks"
	"github.com/memmaker/goray/tpack"
)

// Writing a weapon folder is the same job whether the frames came out of a mod
// or off a sprite sheet, so both tools do it through here.

// WriteAtlas packs frames into one PNG with the frame table in the chunk the
// engine's atlas reader looks for.
func WriteAtlas(images []tpack.NamedImage, path string) error {
	atlas, metadata := tpack.PackImagesIntoAtlasTrimmed(images)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, atlas); err != nil {
		return err
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png_chunks.InjectChunkIntoPNG(&encoded, out, AtlasChunk, metadata.Bytes())
}

// WriteJSON writes one of the folder's JSON files, indented so a re-migration
// diffs a line at a time.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
