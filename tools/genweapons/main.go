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
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/goray/wep"
	"github.com/memmaker/goray/zdwep"
)

func main() {
	source := flag.String("source", "", "the mod to migrate from: a .pk3 or a directory")
	outDir := flag.String("out", "weapons", "the weapons folder to write into")
	lockPath := flag.String("lock", "weapons.lock", "where to write the hashes of what was built")
	revision := flag.String("revision", "", "the source's revision, recorded for provenance")
	list := flag.Bool("list", false, "list the weapons in the mod and stop")
	flag.Parse()

	if *source == "" {
		fail("no -source given. Point it at a .pk3 or at a checkout of the mod.")
	}

	src, err := zdwep.OpenSource(*source)
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
		for _, name := range zdwep.Weapons(index) {
			actor, _ := index.Actor(name)
			tag := name
			if value, ok := actor.Property("tag"); ok {
				tag = strings.Trim(value, `"`)
			}
			fmt.Printf("%-40s %s\n", name, tag)
		}
		return
	}

	wanted := flag.Args()
	if len(wanted) == 0 {
		fail("name at least one weapon to migrate, or pass -list to see what there is.")
	}

	assets := zdwep.NewAssetIndex(src)
	policy := zdwep.DefaultPolicy()

	var built []string
	for _, name := range wanted {
		result, err := zdwep.CompileWeapon(index, policy, name)
		if err != nil {
			fail("%v", err)
		}
		result.Weapon.Provenance.Source = *source
		result.Weapon.Provenance.Revision = *revision
		result.Weapon.Provenance.Credit = credit(src)

		if missing := assets.MissingAssets(result); len(missing) > 0 {
			fail("%s is missing assets in this mod:\n  %s",
				result.Weapon.ID, strings.Join(missing, "\n  "))
		}
		if err := zdwep.Write(result, assets, src, *outDir); err != nil {
			fail("%v", err)
		}
		built = append(built, result.Weapon.ID)
		fmt.Fprintf(os.Stderr, "built %s: %d steps, %d sprites, %d sounds, %d actors\n",
			result.Weapon.ID, len(result.Weapon.States.Steps), len(result.Sprites),
			len(result.Sounds), len(result.Actors))
		for _, dropped := range result.Weapon.Dropped {
			fmt.Fprintf(os.Stderr, "  dropped: %s\n", dropped)
		}
	}

	if err := updateKeyMap(*outDir, built); err != nil {
		fail("%v", err)
	}
	if err := writeLock(*lockPath, *outDir, *source, *revision); err != nil {
		fail("%v", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *lockPath)
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
