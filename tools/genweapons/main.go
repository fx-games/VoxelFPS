// Command genweapons migrates weapons out of a ZDoom mod into the engine's
// weapons folder.
//
//	go run ./tools/genweapons -source ../Brutal-Doom-Community-Expansion -list
//	go run ./tools/genweapons -source ../Brutal-Doom-Community-Expansion FDHellboundShotgun
//
// It reads a pk3 or a directory, indexes every actor in it, and compiles the
// weapons it is asked for into self-contained folders. It never runs while the
// game does.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/goray/wep"
	"github.com/memmaker/goray/zdwep"
)

func main() {
	source := flag.String("source", "", "the mod to migrate from: a .pk3 or a directory. Several, comma separated, are searched in order, so a mod can sit on top of the game data it takes for granted")
	strictAssets := flag.Bool("strict-assets", false, "refuse a weapon whose pictures or sounds the sources do not have, instead of taking it without them")
	outDir := flag.String("out", "weapons", "the weapons folder to write into")
	lockPath := flag.String("lock", "weapons.lock", "where to write the hashes of what was built")
	revision := flag.String("revision", "", "the source's revision, recorded for provenance")
	list := flag.Bool("list", false, "list the weapons in the mod and stop")
	all := flag.Bool("all", false, "migrate every weapon that compiles, and report the rest")
	dry := flag.Bool("dry", false, "compile but write nothing, to see what a mod would need")
	prefix := flag.String("prefix", "", "prepend this to every weapon's folder name")
	scan := flag.Bool("scan", false, "lower every actor in the mod and report the whole vocabulary it needs")
	prune := flag.Bool("prune", false, "delete shared actor files no weapon points at any more, and stop")
	measure := flag.Bool("measure", false, "measure every installed weapon's pickup pictures into its weapon.json, and stop")
	flag.Parse()

	if *measure {
		measured, err := measurePickups(*outDir)
		if err != nil {
			fail("%v", err)
		}
		fmt.Fprintf(os.Stderr, "measured the pickups of %d weapon(s)\n", measured)
		return
	}

	if *prune {
		actors, sprites, err := pruneStores(*outDir)
		if err != nil {
			fail("%v", err)
		}
		fmt.Fprintf(os.Stderr, "removed %d actor file(s) and %d picture(s) nothing points at\n",
			actors, sprites)
		return
	}

	if *source == "" {
		fail("no -source given. Point it at a .pk3 or at a checkout of the mod.")
	}

	src, err := zdwep.OpenSources(strings.Split(*source, ","))
	if err != nil {
		fail("opening %s: %v", *source, err)
	}
	defer src.Close()

	index, err := zdwep.BuildIndex(src)
	if err != nil {
		fail("reading DECORATE: %v", err)
	}
	fmt.Fprintf(os.Stderr, "indexed %d actors from %d files\n", len(index.Actors), len(index.Files))

	if *list {
		weapons := zdwep.Weapons(index)
		if len(weapons) == 0 {
			// Almost always a partial index: the actors are there, but the class
			// they inherit from lives in a file the DECORATE lump did not pull
			// in, so no chain reaches Weapon.
			fmt.Fprintln(os.Stderr, "no weapons found. The classes these actors inherit from "+
				"and this source does not define:")
			for _, root := range unresolvedRoots(index) {
				fmt.Fprintf(os.Stderr, "  %s\n", root)
			}
			return
		}
		for _, name := range weapons {
			actor, _ := index.Actor(name)
			tag := name
			if value, ok := actor.Property("tag"); ok {
				tag = strings.Trim(value, `"`)
			}
			fmt.Printf("%-40s %s\n", name, tag)
		}
		return
	}

	if *scan {
		policy := zdwep.DefaultPolicy()
		policy.Settings = zdwep.LoadSettings(src)
		gaps := newGapReport()
		names := zdwep.AllActorNames(index)
		for _, name := range names {
			if err := zdwep.ScanActor(index, policy, name); err != nil {
				gaps.add(name, err)
			}
		}
		gaps.report(os.Stdout, nil)
		fmt.Fprintf(os.Stderr, "scanned %d actors\n", len(names))
		return
	}

	wanted := flag.Args()
	if *all {
		wanted = zdwep.Weapons(index)
	}
	if len(wanted) == 0 {
		fail("name at least one weapon to migrate, or pass -all, or -list to see what there is.")
	}

	assets := zdwep.NewAssetIndex(src)
	policy := zdwep.DefaultPolicy()
	// The mod's own CVARINFO says what its options default to, so its scripts
	// are read with the author's values rather than ours.
	policy.Settings = zdwep.LoadSettings(src)
	fmt.Fprintf(os.Stderr, "read %d settings from CVARINFO\n", len(policy.Settings.ACS))

	// Migrating a whole mod is a triage loop: most weapons compile, the rest
	// name what the policy has not decided about yet. Failing the run on the
	// first one would hide the other twenty, so they are collected and counted.
	gaps := newGapReport()
	var built, skipped []string
	// Weapon ids come from the tag the player sees, and a mod with several
	// weapon sets has several weapons called Shotgun. Two weapons sharing a
	// folder would mean one quietly overwriting the other, so a name already
	// taken falls back to the actor's own.
	taken := make(map[string]bool)

	for _, name := range wanted {
		compile := zdwep.CompileWeapon
		if *all {
			// Migrating a whole mod takes what a mod gives: a weapon that jumps
			// to a state only its siblings define still compiles, with the jump
			// recorded as going nowhere.
			compile = zdwep.CompileWeaponLenient
		}
		result, err := compile(index, policy, name)
		if err != nil {
			if !*all {
				fail("%v", err)
			}
			gaps.add(name, err)
			skipped = append(skipped, name)
			continue
		}
		result.Weapon.Provenance.Source = *source
		result.Weapon.Provenance.Revision = *revision
		result.Weapon.Provenance.Credit = credit(src)

		result.Weapon.ID = *prefix + uniqueID(taken, result.Weapon.ID, name)

		if collapsed := assets.CollapsedViews(result); len(collapsed) > 0 {
			fmt.Fprintf(os.Stderr, "%s: only one view of %s\n",
				result.Weapon.ID, strings.Join(collapsed, ", "))
		}
		missingSprites, missingSounds := assets.MissingAssets(result)
		if len(missingSprites)+len(missingSounds) > 0 {
			if *strictAssets {
				gaps.addAssets(name, missingSprites, missingSounds)
				skipped = append(skipped, name)
				continue
			}
			// The weapon compiled; it is only some of its pictures and sounds
			// the sources do not have. Those frames show nothing and those
			// calls make no noise, and the weapon records which.
			gaps.addAssets(name, missingSprites, missingSounds)
			result.DropSprites(missingSprites)
			result.DropSounds(missingSounds)
		}
		beforeSprites := len(result.Sprites)
		if dropped := result.PruneDeadStates(); dropped > 0 {
			fmt.Fprintf(os.Stderr, "%s: dropped %d unreachable states, %d of %d sprites\n",
				result.Weapon.ID, dropped, beforeSprites-len(result.Sprites), beforeSprites)
		}
		if len(result.Sprites) == 0 {
			gaps.add(name, fmt.Errorf("none of its pictures are in these sources"))
			skipped = append(skipped, name)
			continue
		}
		if *dry {
			built = append(built, result.Weapon.ID)
			continue
		}
		if err := zdwep.Write(result, assets, src, *outDir); err != nil {
			if !*all {
				fail("%v", err)
			}
			gaps.add(name, err)
			skipped = append(skipped, name)
			continue
		}
		built = append(built, result.Weapon.ID)
		fmt.Fprintf(os.Stderr, "built %s: %d steps, %d sprites, %d sounds, %d actors\n",
			result.Weapon.ID, len(result.Weapon.States.Steps), len(result.Sprites),
			len(result.Sounds), len(result.Actors))
		for _, dropped := range result.Weapon.Dropped {
			fmt.Fprintf(os.Stderr, "  dropped: %s\n", dropped)
		}
	}

	if len(gaps.counts) > 0 {
		gaps.report(os.Stderr, skipped)
	}
	fmt.Fprintf(os.Stderr, "\n%d weapon(s) migrated, %d skipped\n", len(built), len(skipped))

	if *dry {
		return
	}
	if err := updateKeyMap(*outDir, built); err != nil {
		fail("%v", err)
	}
	if err := writeLock(*lockPath, *outDir, *source, *revision); err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *lockPath)
}

// gapReport gathers what a whole-mod run could not decide, so one pass over the
// mod produces one list of decisions rather than one weapon's worth at a time.
type gapReport struct {
	// counts maps a problem, with the line number stripped off, to how many
	// weapons hit it.
	counts map[string]int
	byGap  map[string]string
}

func newGapReport() *gapReport {
	return &gapReport{counts: map[string]int{}, byGap: map[string]string{}}
}

var lineNumberPrefix = regexp.MustCompile(`^[^:]+:\d+: `)

func (g *gapReport) add(weapon string, err error) {
	seen := map[string]bool{}
	for _, line := range strings.Split(err.Error(), "\n") {
		line = lineNumberPrefix.ReplaceAllString(strings.TrimSpace(line), "")
		if line == "" || strings.HasSuffix(line, ":") || seen[line] {
			continue
		}
		seen[line] = true
		g.counts[line]++
		if _, first := g.byGap[line]; !first {
			g.byGap[line] = weapon
		}
	}
}

func (g *gapReport) addAssets(weapon string, sprites, sounds []string) {
	var missing []string
	for _, name := range sprites {
		missing = append(missing, "sprite "+name)
	}
	for _, name := range sounds {
		missing = append(missing, "sound "+name)
	}
	for _, item := range missing {
		line := "missing asset: " + item
		g.counts[line]++
		if _, first := g.byGap[line]; !first {
			g.byGap[line] = weapon
		}
	}
}

// report prints the decisions still to be made, most widely felt first, since
// one policy line near the top unblocks the most weapons.
func (g *gapReport) report(out io.Writer, skipped []string) {
	if len(skipped) > 0 {
		fmt.Fprintf(out, "\n%d weapon(s) did not compile: %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	fmt.Fprintf(out, "\n%d thing(s) the policy has not decided about, widest first:\n", len(g.counts))

	lines := make([]string, 0, len(g.counts))
	for line := range g.counts {
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool {
		if g.counts[lines[i]] != g.counts[lines[j]] {
			return g.counts[lines[i]] > g.counts[lines[j]]
		}
		return lines[i] < lines[j]
	})
	for _, line := range lines {
		fmt.Fprintf(out, "  %3d weapons  %s  (first: %s)\n", g.counts[line], line, g.byGap[line])
	}
}

// updateKeyMap gives every new weapon the first free number key, and leaves
// bindings that are already there alone. The file is the player's, not ours.
func updateKeyMap(outDir string, built []string) error {
	path := filepath.Join(outDir, wep.KeyMapFile)
	lines, bound, err := readKeyMap(path)
	if err != nil {
		return err
	}
	for _, id := range built {
		if _, already := bound[id]; already {
			continue
		}
		key := firstFreeKey(bound)
		if key == 0 {
			fmt.Fprintf(os.Stderr, "no free number key for %s, bind it yourself in %s\n", id, path)
			continue
		}
		bound[id] = key
		lines = append(lines, fmt.Sprintf("%d: %s", key, id))
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i] < lines[j] })
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func readKeyMap(path string) ([]string, map[string]int, error) {
	bound := make(map[string]int)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, bound, nil
		}
		return nil, nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
		left, right, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if key, err := strconv.Atoi(strings.TrimSpace(left)); err == nil {
			bound[strings.TrimSpace(right)] = key
		}
	}
	return lines, bound, scanner.Err()
}

func firstFreeKey(bound map[string]int) int {
	taken := make(map[int]bool, len(bound))
	for _, key := range bound {
		taken[key] = true
	}
	for key := 1; key <= 9; key++ {
		if !taken[key] {
			return key
		}
	}
	return 0
}

// writeLock records what the migration produced, so re-running it and comparing
// says whether upstream moved, the compiler moved, or nothing did. It holds no
// mod content, only hashes, so it can be checked in whatever the mod's terms
// turn out to be.
func writeLock(lockPath, outDir, source, revision string) error {
	var lines []string
	lines = append(lines,
		"# Written by tools/genweapons. Re-run the migration and diff this file",
		"# to see whether the source, the compiler or nothing at all changed.",
		fmt.Sprintf("source\t%s", source),
		fmt.Sprintf("revision\t%s", revision),
		fmt.Sprintf("format\t%d", wep.Format),
	)
	if sum := sourceHash(source); sum != "" {
		lines = append(lines, fmt.Sprintf("source-sha256\t%s", sum))
	}
	if version := zdwep.FFmpegVersion(); version != "" {
		lines = append(lines, fmt.Sprintf("ffmpeg\t%s", version))
	}

	var files []string
	err := filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Base(path) == wep.KeyMapFile {
			return nil // the player's file, not the migration's output
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(outDir, path)
		if err != nil {
			return err
		}
		files = append(files, fmt.Sprintf("%s\tsha256:%s", filepath.ToSlash(relative), sum))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	lines = append(lines, files...)
	return os.WriteFile(lockPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// pruneActors removes the shared actor files no weapon names any more. They
// are left behind when a weapon is re-migrated or removed, and they cost disk
// and nothing else, so sweeping them is a thing you ask for rather than
// something that happens while you are trying to build something.
// pruneStores deletes what the shared stores hold that no installed weapon
// points at any more. Both stores are content addressed, so a re-migration
// that changes anything writes a new file and leaves the old one behind; this
// is what stops the folder growing a copy of every version it ever had.
func pruneStores(weaponsDir string) (actors, sprites int, err error) {
	library, err := wep.Load(weaponsDir)
	if err != nil {
		return 0, 0, err
	}
	wantedActors := make(map[string]bool)
	wantedSprites := make(map[string]bool)
	// The engine's own decal pictures live in the same store and no weapon.json
	// names them, so the sweep below would take them every single time.
	for _, name := range wep.EngineDecalSprites {
		wantedSprites[name+".png"] = true
	}
	for _, weapon := range library.Weapons {
		for _, file := range weapon.Actors {
			wantedActors[file] = true
		}
		for _, name := range weapon.Pictures() {
			wantedSprites[name+".png"] = true
		}
	}
	for _, actor := range library.Actors {
		for _, one := range actor {
			for _, name := range one.Pictures() {
				wantedSprites[name+".png"] = true
			}
		}
	}
	actors, err = pruneStore(filepath.Join(weaponsDir, wep.ActorsDir), wantedActors)
	if err != nil {
		return actors, 0, err
	}
	sprites, err = pruneStore(filepath.Join(weaponsDir, wep.SpritesDir), wantedSprites)
	return actors, sprites, err
}

func pruneStore(store string, wanted map[string]bool) (int, error) {
	entries, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || wanted[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(store, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// uniqueID keeps two weapons out of one folder.
func uniqueID(taken map[string]bool, id, actorName string) string {
	if !taken[id] {
		taken[id] = true
		return id
	}
	fallback := zdwep.SnakeName(actorName)
	for suffix := 2; taken[fallback]; suffix++ {
		fallback = fmt.Sprintf("%s_%d", zdwep.SnakeName(actorName), suffix)
	}
	taken[fallback] = true
	return fallback
}

// sourceHash pins a pk3 by its bytes. A checkout is pinned by its revision
// instead: hashing a whole working tree is slow and says less than the commit
// it came from.
func sourceHash(source string) string {
	info, err := os.Stat(source)
	if err != nil || info.IsDir() {
		return ""
	}
	sum, err := hashFile(source)
	if err != nil {
		return ""
	}
	return sum
}

// unresolvedRoots lists the classes actors inherit from that this source does
// not define, sorted and deduplicated. When nothing looks like a weapon, this is
// what says why.
func unresolvedRoots(index *zdwep.Index) []string {
	seen := make(map[string]bool)
	for name := range index.Actors {
		if root := zdwep.UnresolvedRoot(index, name); root != "" {
			seen[root] = true
		}
	}
	var roots []string
	for root := range seen {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

// credit picks up the mod's own credits lump if it ships one, so a compiled
// weapon carries its attribution rather than losing it on the way out.
func credit(src *zdwep.Source) string {
	for _, name := range []string{"CREDITS", "CREDITS.txt", "README", "README.txt"} {
		if real, ok := src.Resolve(name); ok {
			return real
		}
	}
	return ""
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "genweapons: "+format+"\n", args...)
	os.Exit(1)
}

// measurePickups fills in the pickup boxes of weapons that were migrated before
// there were any. The migration does this itself now; this is the one-off for a
// folder full of weapons nobody is going to migrate again, and it is the same
// measurement off the same store, so running it twice writes the same numbers.
func measurePickups(weaponsDir string) (int, error) {
	library, err := wep.Load(weaponsDir)
	if err != nil {
		return 0, err
	}
	measured := 0
	for _, weapon := range library.Weapons {
		before := fmt.Sprint(weapon.IconBox, weapon.AmmoIconBox)
		if err := wep.MeasurePickups(weaponsDir, weapon); err != nil {
			return measured, err
		}
		if fmt.Sprint(weapon.IconBox, weapon.AmmoIconBox) == before {
			continue
		}
		path := filepath.Join(weaponsDir, weapon.ID, wep.WeaponFile)
		if err := wep.WriteJSON(path, weapon); err != nil {
			return measured, err
		}
		measured++
	}
	return measured, nil
}
