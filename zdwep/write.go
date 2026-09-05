package zdwep

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/memmaker/goray/tpack"
	"github.com/memmaker/goray/wep"
)

// Write puts a compiled weapon into its own folder: the behaviour as JSON, the
// sprites packed into one atlas, the sounds it plays, and the script it came
// from. Nothing in the folder points outside it, so validating one folder
// validates one weapon and removing one cannot break a sibling.
func Write(result *Result, assets *AssetIndex, source *Source, weaponsDir string) error {
	dir := filepath.Join(weaponsDir, result.Weapon.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeActors(result, weaponsDir); err != nil {
		return err
	}
	if err := writeSharedSprites(result, assets, weaponsDir); err != nil {
		return err
	}

	if err := writeAtlas(result, assets, filepath.Join(dir, wep.AtlasFile)); err != nil {
		return err
	}

	sounds, err := writeSounds(result, assets, dir)
	if err != nil {
		return err
	}
	result.Weapon.Sounds = sounds

	if err := wep.WriteJSON(filepath.Join(dir, wep.WeaponFile), result.Weapon); err != nil {
		return err
	}
	// The script the weapon was compiled from, so a re-migration diff says
	// whether upstream moved or the compiler did.
	script, err := source.ReadFile(result.SourceScript)
	if err != nil {
		return fmt.Errorf("reading %s: %w", result.SourceScript, err)
	}
	return os.WriteFile(filepath.Join(dir, wep.SourceFile), script, 0o644)
}

// writeActors puts each actor in the shared store, under the hash of what is in
// it, and notes on the weapon which files it needs. Two weapons that spawn the
// same smoke write the same bytes to the same name, so the second write is a
// no-op and the two cannot drift apart.
//
// The verbatim DECORATE line comes off an actor's ops on the way. It is kept on
// a weapon, which is what somebody reads against the script, and an actor is
// bulk: leaving it on cost another two thirds again across the shared store.
func writeActors(result *Result, weaponsDir string) error {
	if len(result.Actors) == 0 {
		return nil
	}
	store := filepath.Join(weaponsDir, wep.ActorsDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	result.Weapon.Actors = make(map[string]string, len(result.Actors))
	for name, actor := range result.Actors {
		actor.Format = wep.Format
		stripSource(&actor.States)
		file, data, err := wep.HashJSON(actor)
		if err != nil {
			return err
		}
		result.Weapon.Actors[name] = file
		path := filepath.Join(store, file)
		if _, err := os.Stat(path); err == nil {
			continue // the same content is already there, by definition
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// writeSharedSprites puts the frames of the things a weapon spawns into the
// shared picture store, one file each. They are not packed here: which weapons
// are installed is the game's business, not the migration's, so the game packs
// them into one atlas when it starts and keeps it until the set changes.
func writeSharedSprites(result *Result, assets *AssetIndex, weaponsDir string) error {
	if len(result.ActorSprites) == 0 {
		return nil
	}
	store := filepath.Join(weaponsDir, wep.SpritesDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return err
	}
	for _, name := range result.ActorSprites {
		target := filepath.Join(store, name+".png")
		if _, err := os.Stat(target); err == nil {
			continue // another weapon spawns the same thing
		}
		file, ok := assets.Sprite(name)
		if !ok {
			continue // already counted as missing, and already blanked
		}
		data, err := assets.ReadAll(file)
		if err != nil {
			return err
		}
		encoded, err := asPNG(data, name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, encoded, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// asPNG makes sure a picture is one, whatever the mod stored it as.
func asPNG(data []byte, name string) ([]byte, error) {
	decoded, kind, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("sprite %s: %w", name, err)
	}
	if kind == "png" {
		return data, nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, decoded); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func stripSource(machine *wep.StateMachine) {
	for i := range machine.Steps {
		for j := range machine.Steps[i].Ops {
			// A dropped call keeps its line: that one is a decision, and the
			// whole point of recording it is being able to read it back.
			if machine.Steps[i].Ops[j].Op != wep.OpNop {
				machine.Steps[i].Ops[j].Source = ""
			}
		}
	}
}

// writeAtlas packs every frame the weapon and its actors ask for into one PNG,
// with the frame table in the atLS chunk the engine's atlas reader already
// understands.
func writeAtlas(result *Result, assets *AssetIndex, outPath string) error {
	images := make([]tpack.NamedImage, 0, len(result.Sprites))
	var missing []string
	for _, name := range result.Sprites {
		file, ok := assets.Sprite(name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		data, err := assets.ReadAll(file)
		if err != nil {
			return err
		}
		decoded, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("sprite %s (%s): %w", name, file, err)
		}
		images = append(images, tpack.NamedImage{Image: decoded, Name: name})
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s needs sprites this mod does not have: %s",
			result.Weapon.ID, strings.Join(missing, ", "))
	}
	if len(images) == 0 {
		return fmt.Errorf("%s has no sprites at all", result.Weapon.ID)
	}

	return wep.WriteAtlas(images, outPath)
}

// writeSounds copies the weapon's sounds into its folder, keeping the name the
// script plays them by as the path, so the cue the engine loads is the cue the
// compiled op names.
func writeSounds(result *Result, assets *AssetIndex, dir string) (map[string]string, error) {
	if len(result.Sounds) == 0 {
		return nil, nil
	}
	sounds := make(map[string]string, len(result.Sounds))
	var missing []string
	for _, cue := range result.Sounds {
		files, ok := assets.Sounds(cue)
		if !ok {
			missing = append(missing, cue)
			continue
		}
		// A cue the mod gave several sounds keeps them all, named apart by a
		// suffix the loader strips again, so the engine picks between them the
		// way SNDINFO's $random asks it to.
		for index, file := range files {
			data, err := assets.ReadAll(file)
			if err != nil {
				return nil, err
			}
			converted, extension, err := soundBytes(data, file)
			if err != nil {
				return nil, err
			}
			name := filepath.FromSlash(cue)
			if index > 0 {
				name = fmt.Sprintf("%s~%d", name, index+1)
			}
			relative := filepath.Join(wep.SoundsDir, name+extension)
			target := filepath.Join(dir, relative)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, converted, 0o644); err != nil {
				return nil, err
			}
			if index == 0 {
				sounds[cue] = filepath.ToSlash(relative)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s plays sounds this mod does not have: %s",
			result.Weapon.ID, strings.Join(missing, ", "))
	}
	return sounds, nil
}

// soundBytes decides what a sound becomes on the way out.
//
// What it is, not what it is called: a mod stores sounds as lumps with no
// extension, and Final Doomer's are a mix of Doom's own format and ogg files
// under names that say nothing. So the first few bytes decide.
//
// A sound already in a form the player reads is copied through untouched. A
// Doom lump becomes a WAV by arithmetic. Anything else goes through ffmpeg, and
// comes out as a WAV rather than an ogg: which vorbis encoder an ffmpeg build
// has varies, one common build has none and quietly falls back to opus, and the
// player reads neither opus nor a mystery. Uncompressed and correct beats
// compressed and silent.
func soundBytes(data []byte, sourceName string) (out []byte, extension string, err error) {
	switch {
	case bytes.HasPrefix(data, []byte("OggS")):
		return data, ".ogg", nil
	case bytes.HasPrefix(data, []byte("RIFF")):
		return data, ".wav", nil
	case IsDMX(data):
		converted, err := DMXToWAV(data)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", sourceName, err)
		}
		return converted, ".wav", nil
	}
	converted, err := convertToWAV(data, sourceName)
	if err != nil {
		return nil, "", err
	}
	return converted, ".wav", nil
}

// convertToWAV hands a sound in some format we do not read to ffmpeg. It is
// only reached for the handful a mod ships as mp3 or flac; when ffmpeg is not
// installed the migration says so and gives the command to run, rather than
// quietly dropping the sound.
func convertToWAV(data []byte, sourceName string) ([]byte, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("%s is in a format this migration does not read "+
			"and ffmpeg is not installed to convert it", sourceName)
	}
	command := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-y", "-i", "pipe:0", "-c:a", "pcm_s16le", "-f", "wav", "pipe:1")
	command.Stdin = bytes.NewReader(data)
	var out, stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("converting %s: %w: %s",
			sourceName, err, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

// FFmpegVersion reports the ffmpeg the migration would use, for the lock file.
// An empty string means it is not installed, which only matters if a sound
// needs converting.
func FFmpegVersion() string {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	out, err := exec.Command(ffmpeg, "-version").Output()
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(first)
}
