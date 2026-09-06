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
	// The last file with a lump's name wins, which is GZDoom's rule and not an
	// arbitrary one: FileSystem::CheckNumForName searches the lump list
	// backwards, so a later entry shadows an earlier one of the same name.
	// That is how a mod overrides a sound with a better take in an `hd/`
	// folder, and it is how Brutal Doom means PLSTA0 to be the plasma gun's
	// firing frame in sprites/weapons/plasmagun and not the 43x24 exhaust
	// flare in sprites/vehicles/bike. Keeping the first one instead handed the
	// plasma gun a bike part with a world sprite's hotspot on it, drawn off
	// the left edge of the screen: firing made the gun disappear.
	//
	// It is a narrow rule in practice. Brutal Doom has 11971 sprite lumps and
	// 44 of them are named twice, most being a folder duplicated inside
	// itself.
	//
	// But it is a rule about *one* lump list, and OpenSources stacks several.
	// The stack is searched the other way round on purpose - the mod first and
	// doom2.wad last, because the mods take the base game's sprites and sounds
	// for granted and only want what they did not ship themselves. Run over
	// the whole stack, "last wins" hands every name the base game also uses
	// straight back to the base game. Brutal Doom shares 655 lump names with
	// doom2.wad, and what came out vanilla was not obscure: the whole super
	// shotgun, the rocket, the ejected shell casing, and four frames of the
	// chainsaw's idle - which is why the chainsaw's idle animation was four
	// vanilla frames and three of Brutal Doom's own, jumping between two
	// different pairs of hands.
	//
	// So a lump name belongs to the first layer that has one, and only lumps
	// from that same layer may shadow it afterwards.
	claimed := func(seen map[string]int, base string, layer int) bool {
		if first, taken := seen[base]; taken && first != layer {
			return false
		}
		seen[base] = layer
		return true
	}
	spriteLayer, soundLayer := map[string]int{}, map[string]int{}
	for _, name := range src.Names() {
		lower := strings.ToLower(name)
		base := strings.TrimSuffix(path.Base(lower), path.Ext(lower))
		layer := src.LayerOf(name)
		switch {
		case strings.HasPrefix(lower, "sprites/"):
			if !claimed(spriteLayer, base, layer) {
				continue
			}
			index.sprites[base] = name
			index.noteRotations(base, name)
		case strings.HasPrefix(lower, "sounds/"):
			if !claimed(soundLayer, base, layer) {
				continue
			}
			index.sounds[base] = name
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
	// Several views of the same frame, and we draw one flat billboard, so one
	// of them has to be the picture. It is the back: what a weapon throws into
	// the world is spawned facing the way the player faces, and the player is
	// the only one looking at it, so the view they get is the one from behind.
	// Doom numbers that rotation 5, and GZDoom picks it the same way - the
	// angle from the viewer to the thing, less the thing's own, lands in the
	// fifth of eight sectors.
	//
	// It is not a detail. Brutal Doom's muzzle flash is authored blank from
	// rotations 4 to 6, so in GZDoom the player who fires never sees the
	// flash actor at all - only the flash drawn into the weapon's own frames.
	// Taking the front view instead, which is what sorting these and picking
	// the first does, put a 21 unit fireball in the player's face on every
	// shot of every gun in the mod.
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	for _, wanted := range []byte{'5', '0'} {
		for _, file := range sorted {
			if rotationOf(file) == wanted {
				return file, true
			}
		}
	}
	return sorted[0], true
}

// rotationOf is the rotation a sprite lump holds, as the digit Doom writes
// after the frame letter: NAMEFR, or NAMEFRFR for a lump that holds two views
// at once, the second mirrored. Zero means one picture used from every angle.
func rotationOf(file string) byte {
	base := strings.ToUpper(strings.TrimSuffix(path.Base(file), path.Ext(file)))
	if len(base) < 6 {
		return 0
	}
	if len(base) >= 8 && base[7] == '5' {
		return '5'
	}
	return base[5]
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

// unquote strips a pair of matching double quotes SNDINFO sometimes wraps a
// name in, which strings.Fields itself does not touch.
func unquote(field string) string {
	return strings.Trim(field, `"`)
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
				mapping[strings.ToLower(unquote(fields[1]))] = []string{unquote(fields[2])}
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mapping[strings.ToLower(unquote(fields[0]))] = []string{unquote(fields[1])}
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

// SpriteView is one angle of a frame: the file holding it, and whether the
// file holds it mirrored, which is what the second half of a NAMEFRFR lump is.
// Doom ships five lumps for eight views and lets three of them do double duty
// that way, because a thing seen from its front left and its front right is
// the same picture the other way round.
type SpriteView struct {
	File   string
	Mirror bool
}

// Views is how many angles a frame is drawn from.
const Views = 8

// SpriteViews is the eight angles of a frame, in the order the engine asks for
// them: 0 is the front, 4 the back, and they run round the thing's own left.
// Doom numbers the same eight 1 to 8 in that same order, so the lump for view
// direction N carries rotation N+1 - pinned by core's
// TestGetViewDirectionRunsRoundTheThingsOwnLeft, because getting it backwards
// shows a thing's left side to somebody standing on its right.
//
// It reports false for a frame the mod draws once for every angle, and for a
// set with a hole in it. A partial set is not something to fill in by guessing,
// and Sprite()'s one picture is the honest answer for it.
func (a *AssetIndex) SpriteViews(name string) ([Views]SpriteView, bool) {
	var views [Views]SpriteView
	upper := strings.ToUpper(name)
	if len(upper) < 6 {
		return views, false
	}
	if _, whole := a.sprites[strings.ToLower(name)]; whole {
		// The mod ships this exact frame, which by its own name is the one
		// picture for every angle.
		return views, false
	}
	frame := upper[4]
	for _, file := range a.rotations[strings.ToLower(upper[:5])] {
		base := strings.ToUpper(strings.TrimSuffix(path.Base(file), path.Ext(file)))
		if len(base) < 6 {
			continue
		}
		put := func(rotation byte, mirror bool) {
			if rotation < '1' || rotation > '8' {
				return // rotation 0 is not one of the eight; it is all of them
			}
			views[rotation-'1'] = SpriteView{File: file, Mirror: mirror}
		}
		if base[4] == frame {
			put(base[5], false)
		}
		if len(base) >= 8 && base[6] == frame {
			put(base[7], true)
		}
	}
	for _, view := range views {
		if view.File == "" {
			return [Views]SpriteView{}, false
		}
	}
	return views, true
}

// CollapsedViews names the world sprites this weapon still takes one picture
// of when the sources hold more than one. All eight of a frame are migrated
// now - see SpriteViews - so what is left in here is a partial set: three
// views of a thing you can walk all the way round, which is not something to
// fill in by guessing. Sprite() takes the back one and this says so.
//
// A weapon's own frames are never in here: a psprite is always rotation 0, and
// only the actors a weapon throws are things you can walk around.
func (a *AssetIndex) CollapsedViews(result *Result) []string {
	var collapsed []string
	for _, name := range result.ActorSprites {
		lower := strings.ToLower(name)
		if _, direct := a.sprites[lower]; direct || len(lower) < 6 {
			continue // the sources have this exact frame; nothing was chosen for us
		}
		if _, several := a.SpriteViews(name); several {
			continue // all eight are migrated
		}
		if views := a.views(lower[:5]); views > 1 {
			collapsed = append(collapsed, fmt.Sprintf("%s (%d of 8 views)", name, views))
		}
	}
	sort.Strings(collapsed)
	return collapsed
}

// views is how many distinct angles the sources hold for a frame, counting both
// halves of a NAMEFRFR lump, which holds two.
func (a *AssetIndex) views(frameKey string) int {
	seen := map[byte]bool{}
	for _, file := range a.rotations[frameKey] {
		base := strings.ToUpper(strings.TrimSuffix(path.Base(file), path.Ext(file)))
		if len(base) < 6 {
			continue
		}
		seen[base[5]] = true
		if len(base) >= 8 {
			seen[base[7]] = true
		}
	}
	return len(seen)
}

// MissingAssets lists what a compiled weapon needs and the sources do not have,
// split so a caller can either refuse the weapon or take it without them.
func (a *AssetIndex) MissingAssets(result *Result) (sprites, sounds []string) {
	for _, sprite := range append(append([]string{}, result.Sprites...), result.ActorSprites...) {
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
