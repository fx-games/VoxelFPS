package wep

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"io"
	"os"

	"github.com/memmaker/voxellib/tpack"
	"github.com/memmaker/voxellib/tpack/png_chunks"
)

// Writing a weapon folder is the same job whether the frames came out of a mod
// or off a sprite sheet, so both tools do it through here.

// WriteAtlas packs frames into one PNG with the frame table in the chunk the
// engine's atlas reader looks for.
func WriteAtlas(images []tpack.NamedImage, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return writeAtlasTo(images, out)
}

func writeAtlasTo(images []tpack.NamedImage, out io.Writer) error {
	atlas, metadata := tpack.PackImagesIntoAtlasTrimmed(images)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, atlas); err != nil {
		return err
	}
	return png_chunks.InjectChunkIntoPNG(&encoded, out, AtlasChunk, metadata.Bytes())
}

// WriteJSON writes one of the folder's JSON files.
//
// Compact rather than indented: these are generated, and a whole mod's worth of
// them came to 356 megabytes with indentation and 150 without. What makes a
// re-migration reviewable is weapons.lock, which says which files moved.
func WriteJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// HashJSON encodes a value and names it by the hash of the result, which is how
// a shared actor gets a file name that cannot collide or disagree.
func HashJSON(value any) (name string, data []byte, err error) {
	data, err = json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + ".json", append(data, '\n'), nil
}
