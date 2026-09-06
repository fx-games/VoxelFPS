package wep

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/goray/tpack"
)

// The weapons folder is data a person can edit, so it is checked on load even
// though the migration that normally writes it already guarantees all of this.
// Every problem is collected and then the load fails: skipping a broken weapon
// would silently change the loadout and leave the player to find out, and
// stopping at the first problem would mean one run per typo.

// Library is a loaded weapons folder.
type Library struct {
	Dir     string
	Weapons []*Weapon
	ByID    map[string]*Weapon
	// Actors holds each weapon's own actors, keyed by weapon id then by
	// lowercased actor name.
	Actors map[string]map[string]*Actor
	// Keys binds number keys to weapon ids: Keys[1] is what pressing 1 selects.
	Keys map[int]string
	// Sprites lists the atlas frame names each weapon's atlas holds, in atlas
	// order, so the engine can turn a step's sprite name into an atlas index.
	Sprites map[string][]string
}

// AtlasPath is where a weapon's packed sprites live.
func (l *Library) AtlasPath(id string) string {
	return filepath.Join(l.Dir, id, AtlasFile)
}

// SoundsPath is where a weapon's sounds live.
func (l *Library) SoundsPath(id string) string {
	return filepath.Join(l.Dir, id, SoundsDir)
}

// Ordered lists the weapons in key order, then the rest alphabetically, which
// is the order the player cycles through them in.
func (l *Library) Ordered() []*Weapon {
	var ordered []*Weapon
	taken := make(map[string]bool)
	for _, key := range sortedKeys(l.Keys) {
		if weapon, ok := l.ByID[l.Keys[key]]; ok && !taken[weapon.ID] {
			ordered = append(ordered, weapon)
			taken[weapon.ID] = true
		}
	}
	for _, weapon := range l.Weapons {
		if !taken[weapon.ID] {
			ordered = append(ordered, weapon)
		}
	}
	return ordered
}

// Load reads and checks a weapons folder.
func Load(dir string) (*Library, error) {
	library := &Library{
		Dir:     dir,
		ByID:    make(map[string]*Weapon),
		Actors:  make(map[string]map[string]*Actor),
		Keys:    make(map[int]string),
		Sprites: make(map[string][]string),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("weapons: %w", err)
	}

	// Actors are shared between weapons and named by the hash of what is in
	// them, so they are read once however many weapons point at one. Their
	// pictures are shared too, in their own directory.
	shared := newActorStore(filepath.Join(dir, ActorsDir))
	sharedSprites := readSharedSprites(filepath.Join(dir, SpritesDir))

	var problems []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ActorsDir || entry.Name() == SpritesDir {
			continue
		}
		weapon, actors, sprites, issues := loadWeapon(
			filepath.Join(dir, entry.Name()), entry.Name(), shared, sharedSprites)
		problems = append(problems, issues...)
		if weapon == nil {
			continue
		}
		library.Weapons = append(library.Weapons, weapon)
		library.ByID[weapon.ID] = weapon
		library.Actors[weapon.ID] = actors
		library.Sprites[weapon.ID] = sprites
	}
	sort.Slice(library.Weapons, func(i, j int) bool {
		return library.Weapons[i].ID < library.Weapons[j].ID
	})

	keys, keyProblems := loadKeyMap(filepath.Join(dir, KeyMapFile), library.ByID)
	library.Keys = keys
	problems = append(problems, keyProblems...)

	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("weapons: %d problem(s)\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
	if len(library.Weapons) == 0 {
		return nil, fmt.Errorf("weapons: %s holds no weapons", dir)
	}
	return library, nil
}

// actorStore reads the shared actor files, keeping what it has already read.
type actorStore struct {
	dir    string
	loaded map[string]*Actor
}

func newActorStore(dir string) *actorStore {
	return &actorStore{dir: dir, loaded: make(map[string]*Actor)}
}

// get reads one actor file, or hands back the copy already read.
func (s *actorStore) get(file string) (*Actor, error) {
	if actor, ok := s.loaded[file]; ok {
		return actor, nil
	}
	var actor Actor
	if err := readJSON(filepath.Join(s.dir, file), &actor); err != nil {
		return nil, err
	}
	if actor.Format != Format {
		return nil, fmt.Errorf("%s was built for format %d, this build reads %d",
			file, actor.Format, Format)
	}
	s.loaded[file] = &actor
	return &actor, nil
}

// readSharedSprites lists the frames in the shared picture store. They are
// packed into one atlas when the game starts; here they only have to be there.
func readSharedSprites(dir string) map[string]bool {
	names := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))] = true
	}
	return names
}

func loadWeapon(dir, id string, shared *actorStore, sharedSprites map[string]bool) (*Weapon, map[string]*Actor, []string, []string) {
	var problems []string
	fail := func(format string, args ...any) (*Weapon, map[string]*Actor, []string, []string) {
		return nil, nil, nil, append(problems, id+"/: "+fmt.Sprintf(format, args...))
	}

	var weapon Weapon
	if err := readJSON(filepath.Join(dir, WeaponFile), &weapon); err != nil {
		return fail("%v", err)
	}
	if weapon.Format != Format {
		return fail("%s was built for format %d, this build reads %d",
			WeaponFile, weapon.Format, Format)
	}
	if weapon.ID != id {
		problems = append(problems, fmt.Sprintf("%s/: names itself %q", id, weapon.ID))
	}

	actors := make(map[string]*Actor, len(weapon.Actors))
	for name, file := range weapon.Actors {
		actor, err := shared.get(file)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s/: %s: %v", id, name, err))
			continue
		}
		actors[strings.ToLower(name)] = actor
	}

	sprites, err := atlasFrames(filepath.Join(dir, AtlasFile))
	if err != nil {
		return fail("%v", err)
	}
	known := make(map[string]bool, len(sprites))
	for _, sprite := range sprites {
		known[sprite] = true
	}

	problems = append(problems, checkStates(id, WeaponFile, weapon.States, known, actors, weapon.Sounds, dir)...)
	for _, name := range sortedActorNames(actors) {
		actor := actors[name]
		if actor.Stock {
			continue // the engine supplies it, so there are no states to check
		}
		problems = append(problems,
			checkStates(id, "actor "+actor.Name, actor.States, sharedSprites, actors, weapon.Sounds, dir)...)
		// The other seven angles are not named by any step, so checkStates
		// never sees them.
		for name, views := range actor.Views {
			if len(views) != ViewDirections {
				problems = append(problems, fmt.Sprintf("%s/: actor %s: %s has %d views, want %d",
					id, actor.Name, name, len(views), ViewDirections))
			}
			for _, view := range views {
				if !sharedSprites[view] {
					problems = append(problems, fmt.Sprintf("%s/: actor %s: %s is a view of %s, and there is no such picture",
						id, actor.Name, view, name))
				}
			}
		}
	}

	if len(problems) > 0 {
		return nil, nil, nil, problems
	}
	return &weapon, actors, sprites, nil
}

// checkStates walks one state machine: every jump lands on a label that exists,
// every op is one this build knows, every sprite is in the atlas, every sound
// is a file, and every actor referenced was compiled into this folder.
func checkStates(id, what string, machine StateMachine, sprites map[string]bool,
	actors map[string]*Actor, sounds map[string]string, dir string) []string {
	var problems []string
	note := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s/%s: ", id, what)+fmt.Sprintf(format, args...))
	}

	for index, step := range machine.Steps {
		// A keep marker names no picture of its own - what it shows is
		// whatever the actor is showing - so it is its alternatives that have
		// to be there.
		if step.Sprite != "" && !IsKeepMarker(step.Sprite) && !sprites[step.Sprite] {
			note("step %d shows %s, and there is no such picture", index, step.Sprite)
		}
		for _, name := range step.SpriteAlt {
			if !sprites[name] {
				note("step %d can show %s, and there is no such picture", index, name)
			}
		}
		if step.Tics < -1 {
			note("step %d lasts %d tics", index, step.Tics)
		}
		for _, op := range step.Ops {
			if !KnownOps[op.Op] {
				note("step %d uses the op %q, which this build does not know", index, op.Op)
				continue
			}
			if op.To != "" {
				if _, ok := machine.Label(op.To); !ok {
					note("step %d jumps to %q, and there is no such label", index, op.To)
				}
			}
			if op.Sound != "" {
				file, ok := sounds[op.Sound]
				if !ok {
					note("step %d plays %q, which the weapon does not list", index, op.Sound)
				} else if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(file))); err != nil {
					note("step %d plays %q, and %s is missing", index, op.Sound, file)
				}
			}
			if op.Actor != "" {
				if _, ok := actors[strings.ToLower(op.Actor)]; !ok {
					note("step %d spawns %q, which the weapon does not list", index, op.Actor)
				}
			}
		}
	}
	return problems
}

// atlasFrames reads the frame table out of a packed atlas without needing a
// graphics context, which is what lets loading check sprite names before
// anything is uploaded.
func atlasFrames(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_, metadata, err := tpack.DecodeTrimmedAtlas(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", AtlasFile, err)
	}
	names := make([]string, len(metadata))
	for i := range metadata {
		names[i] = metadata.Name(i)
	}
	return names, nil
}

// loadKeyMap reads `1: borstal_shotgun` lines. It is the one file in the
// weapons folder a person writes by hand, so it is plain text and its problems
// are reported with line numbers.
func loadKeyMap(path string, weapons map[string]*Weapon) (map[int]string, []string) {
	keys := make(map[int]string)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return keys, []string{fmt.Sprintf("%s: missing", KeyMapFile)}
		}
		return keys, []string{fmt.Sprintf("%s: %v", KeyMapFile, err)}
	}
	defer file.Close()

	var problems []string
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		note := func(format string, args ...any) {
			problems = append(problems,
				fmt.Sprintf("%s:%d: ", KeyMapFile, lineNumber)+fmt.Sprintf(format, args...))
		}
		left, right, found := strings.Cut(line, ":")
		if !found {
			note("expected `<key>: <weapon>`, got %q", line)
			continue
		}
		key, err := strconv.Atoi(strings.TrimSpace(left))
		if err != nil || key < 1 || key > 9 {
			note("%q is not a number key from 1 to 9", strings.TrimSpace(left))
			continue
		}
		id := strings.TrimSpace(right)
		if existing, taken := keys[key]; taken {
			note("key %d is already %s", key, existing)
			continue
		}
		if _, ok := weapons[id]; !ok {
			note("no weapon folder named %q", id)
			continue
		}
		keys[key] = id
	}
	return keys, problems
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

func sortedActorNames(actors map[string]*Actor) []string {
	names := make([]string, 0, len(actors))
	for name := range actors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(keys map[int]string) []int {
	out := make([]int, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Ints(out)
	return out
}
