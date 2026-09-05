package zdwep

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"sort"
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
	// rotations maps a sprite and frame - "tfi2" and 'a' - to the files holding
	// it. Doom names a sprite NAMEFR, four letters then a frame letter then a
	// rotation: rotation 0 is one picture used from every angle, and 1 to 8 are
	// the eight views of something you can walk around. A lump can also carry
	// two views at once, NAMEFRFR, the second mirrored.
	//
	// A weapon's own frames are always rotation 0. The things it throws into
	// the world are not, and a script names them without a rotation, so a
	// lookup for NAMEA0 has to find NAMEA1 as well.
	rotations map[string][]string
	sounds    map[string]string
	// sndinfo maps a sound name, as A_PlaySound writes it, to the lumps that
	// hold it. Usually one, but SNDINFO's $random directive gives a name
	// several and lets the engine pick, which is how a mod stops a gunshot
	// sounding identical every time.
	sndinfo map[string][]string
}

// NewAssetIndex walks the source once and notes where everything lives.
func NewAssetIndex(src *Source) *AssetIndex {
	index := &AssetIndex{
		src:       src,
		sprites:   make(map[string]string),
		rotations: make(map[string][]string),
		sounds:    make(map[string]string),
	}
	for _, name := range src.Names() {
		lower := strings.ToLower(name)
		base := strings.TrimSuffix(path.Base(lower), path.Ext(lower))
		switch {
		case strings.HasPrefix(lower, "sprites/"):
			if _, taken := index.sprites[base]; !taken {
				index.sprites[base] = name
			}
			index.noteRotations(base, name)
		case strings.HasPrefix(lower, "sounds/"):
			if _, taken := index.sounds[base]; !taken {
				index.sounds[base] = name
			}
		}
	}
	index.sndinfo = readSndInfoFrom(src)
	return index
}

// noteRotations files a sprite lump under every frame it holds.
func (a *AssetIndex) noteRotations(base, path string) {
	if len(base) < 6 {
		return
	}
	key := func(frame byte) string { return base[:4] + string(frame) }
	a.rotations[key(base[4])] = append(a.rotations[key(base[4])], path)
	if len(base) >= 8 {
		a.rotations[key(base[6])] = append(a.rotations[key(base[6])], path)
	}
}

// Sprite finds the file holding a sprite frame such as HSHTA0, falling back to
// the angled views when the sprite is something you see from outside.
func (a *AssetIndex) Sprite(name string) (string, bool) {
	lower := strings.ToLower(name)
	if file, ok := a.sprites[lower]; ok {
		return file, true
	}
	if len(lower) < 6 {
		return "", false
	}
	files := a.rotations[lower[:5]]
	if len(files) == 0 {
		return "", false
	}
	// Several views of the same frame, and we draw one flat billboard, so take
	// the first: sorted, that is rotation 1, the view from the front.
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	return sorted[0], true
}

// Sounds finds the files holding a sound named the way a script names it, such
// as hellboundshotgun/fire, following SNDINFO to the lumps it stands for. More
// than one means the mod asked for a random pick between them.
func (a *AssetIndex) Sounds(name string) ([]string, bool) {
	lumps := []string{name}
	if mapped, ok := a.sndinfo[strings.ToLower(name)]; ok {
		lumps = mapped
	}
	var files []string
	for _, lump := range lumps {
		base := strings.TrimSuffix(path.Base(lump), path.Ext(lump))
		if file, ok := a.sounds[strings.ToLower(base)]; ok {
			files = append(files, file)
		}
	}
	return files, len(files) > 0
}

// readSndInfoFrom reads the mod's SNDINFO, which maps the names scripts play to
// the lumps that hold them.
//
// Two directives matter. `$random name { a b c }` gives one name several
// sounds to choose between, and `$alias new old` gives an existing sound a
// second name. The rest - reverb, ambient loops, player grunts - are about
// things a weapon does not do.
func readSndInfoFrom(src *Source) map[string][]string {
	mapping := make(map[string][]string)
	file, err := src.Open("SNDINFO")
	if err != nil {
		return mapping
	}
	defer file.Close()

	var randomLists [][2]string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0])
		if line == "" || line[0] == ';' {
			continue
		}
		if line[0] == '$' {
			fields := strings.Fields(line)
			switch {
			case strings.EqualFold(fields[0], "$random") && len(fields) >= 3:
				name := strings.ToLower(fields[1])
				body := line[strings.Index(line, "{")+1:]
				body = strings.TrimSuffix(strings.TrimSpace(body), "}")
				randomLists = append(randomLists, [2]string{name, body})
			case strings.EqualFold(fields[0], "$alias") && len(fields) >= 3:
				mapping[strings.ToLower(fields[1])] = []string{fields[2]}
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mapping[strings.ToLower(fields[0])] = []string{fields[1]}
	}

	// The lists are resolved afterwards, because a $random can name sounds
	// defined below it.
	for _, entry := range randomLists {
		var lumps []string
		for _, member := range strings.Fields(entry[1]) {
			if resolved, ok := mapping[strings.ToLower(member)]; ok {
				lumps = append(lumps, resolved...)
			} else {
				lumps = append(lumps, member)
			}
		}
		if len(lumps) > 0 {
			mapping[entry[0]] = lumps
		}
	}
	return mapping
}

// MissingAssets lists what a compiled weapon needs and the sources do not have,
// split so a caller can either refuse the weapon or take it without them.
func (a *AssetIndex) MissingAssets(result *Result) (sprites, sounds []string) {
	for _, sprite := range result.Sprites {
		if _, ok := a.Sprite(sprite); !ok {
			sprites = append(sprites, sprite)
		}
	}
	for _, sound := range result.Sounds {
		if _, ok := a.Sounds(sound); !ok {
			sounds = append(sounds, sound)
		}
	}
	return sprites, sounds
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
