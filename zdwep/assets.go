package zdwep

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strings"
)

// Sprites and sounds are named in the scripts the way Doom named lumps, and
// stored in the mod the way a filesystem stores files. This is the mapping
// between the two, and the one place that knows SNDINFO exists.

// AssetIndex finds a mod's sprites and sounds by the names its scripts use.
type AssetIndex struct {
	src *Source
	// sprites maps a lowercased lump name, without extension, to its path.
	sprites map[string]string
	sounds  map[string]string
	// sndinfo maps a sound name, as A_PlaySound writes it, to the lump that
	// holds it.
	sndinfo map[string]string
}

// NewAssetIndex walks the source once and notes where everything lives.
func NewAssetIndex(src *Source) *AssetIndex {
	index := &AssetIndex{
		src:     src,
		sprites: make(map[string]string),
		sounds:  make(map[string]string),
	}
	for _, name := range src.Names() {
		lower := strings.ToLower(name)
		base := strings.TrimSuffix(path.Base(lower), path.Ext(lower))
		switch {
		case strings.HasPrefix(lower, "sprites/"):
			if _, taken := index.sprites[base]; !taken {
				index.sprites[base] = name
			}
		case strings.HasPrefix(lower, "sounds/"):
			if _, taken := index.sounds[base]; !taken {
				index.sounds[base] = name
			}
		}
	}
	index.sndinfo = readSndInfoFrom(src)
	return index
}

// Sprite finds the file holding a sprite frame such as HSHTA0.
func (a *AssetIndex) Sprite(name string) (string, bool) {
	file, ok := a.sprites[strings.ToLower(name)]
	return file, ok
}

// Sound finds the file holding a sound named the way a script names it, such
// as hellboundshotgun/fire, following SNDINFO to the lump it stands for.
func (a *AssetIndex) Sound(name string) (string, bool) {
	lump := name
	if mapped, ok := a.sndinfo[strings.ToLower(name)]; ok {
		lump = mapped
	}
	base := strings.TrimSuffix(path.Base(lump), path.Ext(lump))
	file, ok := a.sounds[strings.ToLower(base)]
	return file, ok
}

// readSndInfoFrom reads the mod's SNDINFO, which maps the names scripts play to
// the lumps that hold them.
func readSndInfoFrom(src *Source) map[string]string {
	mapping := make(map[string]string)
	file, err := src.Open("SNDINFO")
	if err != nil {
		return mapping
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '$' || line[0] == ';' || strings.HasPrefix(line, "//") {
			continue
		}
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mapping[strings.ToLower(fields[0])] = fields[1]
	}
	return mapping
}

// MissingAssets lists what a compiled weapon needs and the mod does not have.
func (a *AssetIndex) MissingAssets(result *Result) []string {
	var missing []string
	for _, sprite := range result.Sprites {
		if _, ok := a.Sprite(sprite); !ok {
			missing = append(missing, "sprite "+sprite)
		}
	}
	for _, sound := range result.Sounds {
		if _, ok := a.Sound(sound); !ok {
			missing = append(missing, "sound "+sound)
		}
	}
	return missing
}

// ReadAll reads a file out of the source.
func (a *AssetIndex) ReadAll(name string) ([]byte, error) {
	file, err := a.src.Open(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	defer file.Close()
	return io.ReadAll(file)
}
