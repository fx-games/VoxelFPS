package wep

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/memmaker/goray/png_chunks"
	"github.com/memmaker/goray/tpack"
)

// Everything a weapon throws into the world is drawn from one atlas, packed
// from the shared picture store when the game starts.
//
// It is not packed by the migration, because the migration only ever sees one
// mod at a time and which weapons are installed is decided later, by which
// folders are there. It is not packed every start either: 867 actors' worth of
// frames takes a moment, and the answer only changes when the folder does. So
// it is packed once, kept, and packed again when the store no longer matches
// what it was packed from.
//
// One atlas rather than one per weapon also means the world sprite pass binds
// one texture for every casing, puff and rocket on screen, whoever fired it.

// SharedAtlas is where the packed pictures ended up.
type SharedAtlas struct {
	// Path is the packed atlas, ready to be read as a texture.
	Path string
	// Rebuilt says whether this run had to pack it rather than finding it.
	Rebuilt bool
	// Frames is how many pictures went in.
	Frames int
}

// BuildSharedAtlas packs the shared picture store, or leaves the packed atlas
// alone when it still matches what is in the store.
func BuildSharedAtlas(weaponsDir string) (SharedAtlas, error) {
	store := filepath.Join(weaponsDir, SpritesDir)
	atlasPath := filepath.Join(weaponsDir, SharedAtlasFile)

	names, key, err := sharedSpriteKey(store)
	if err != nil {
		return SharedAtlas{}, err
	}
	if len(names) == 0 {
		return SharedAtlas{}, nil // nothing spawns anything, so nothing to pack
	}
	if matchesKey(atlasPath, key) {
		return SharedAtlas{Path: atlasPath, Frames: len(names)}, nil
	}

	images := make([]tpack.NamedImage, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(store, name+".png"))
		if err != nil {
			return SharedAtlas{}, err
		}
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return SharedAtlas{}, fmt.Errorf("%s: %w", name, err)
		}
		offsetX, offsetY := tpack.OffsetsFromPNG(data)
		images = append(images, tpack.NamedImage{Image: decoded, Name: name, OffsetX: offsetX, OffsetY: offsetY})
	}

	if err := WriteAtlasWithKey(images, atlasPath, key); err != nil {
		return SharedAtlas{}, err
	}
	return SharedAtlas{Path: atlasPath, Rebuilt: true, Frames: len(names)}, nil
}

// sharedSpriteKey lists the store and boils it down to something short that
// changes when the store does. Names and sizes rather than contents: a picture
// that is replaced by a different one of exactly the same size, without its
// name changing, is not a thing a migration does.
func sharedSpriteKey(store string) (names []string, key string, err error) {
	entries, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	hash := sha256.New()
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
	}
	sort.Strings(names)
	for _, name := range names {
		info, err := os.Stat(filepath.Join(store, name+".png"))
		if err != nil {
			return nil, "", err
		}
		fmt.Fprintf(hash, "%s:%d\n", name, info.Size())
	}
	return names, hex.EncodeToString(hash.Sum(nil)), nil
}

// matchesKey reports whether the packed atlas says it was packed from this.
func matchesKey(atlasPath, key string) bool {
	file, err := os.Open(atlasPath)
	if err != nil {
		return false
	}
	defer file.Close()
	return png_chunks.ReadTextChunkFromPNG(file, SharedAtlasKey) == key
}

// WriteAtlasWithKey packs pictures and records what they were, so the next run
// can tell whether it still holds.
func WriteAtlasWithKey(images []tpack.NamedImage, path, key string) error {
	var packed bytes.Buffer
	if err := writeAtlasTo(images, &packed); err != nil {
		return err
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png_chunks.InjectTextChunkIntoPNG(&packed, out, SharedAtlasKey, key)
}
