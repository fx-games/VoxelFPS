package zdwep

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"os"
	"os/exec"
	"path"
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
	if err := wep.WriteJSON(filepath.Join(dir, wep.ActorsFile), wep.Actors{
		Format: wep.Format,
		Actors: result.Actors,
	}); err != nil {
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
			if err := writeSound(converted, file, target, extension); err != nil {
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

// soundBytes decides what a sound becomes on the way out. An ogg is copied
// through untouched, and a Doom sound lump is repackaged as a WAV, which is
// arithmetic rather than transcoding and needs nothing installed. Anything else
// is handed to ffmpeg.
func soundBytes(data []byte, sourceName string) (out []byte, extension string, err error) {
	if strings.EqualFold(path.Ext(sourceName), ".ogg") {
		return data, ".ogg", nil
	}
	if strings.EqualFold(path.Ext(sourceName), ".wav") {
		return data, ".wav", nil
	}
	if IsDMX(data) {
		wav, err := DMXToWAV(data)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", sourceName, err)
		}
		return wav, ".wav", nil
	}
	return data, ".ogg", nil
}

// writeSound puts the bytes where they go, running ffmpeg only for a format
// nothing here understands.
func writeSound(data []byte, sourceName, target, extension string) error {
	if extension == ".wav" || strings.EqualFold(path.Ext(sourceName), ".ogg") {
		return os.WriteFile(target, data, 0o644)
	}
	return convertToOgg(data, sourceName, target)
}

// vorbisEncoders are the ways an ffmpeg build might be able to write vorbis.
// Which one a given build has varies, and the common one on macOS has neither
// libvorbis nor a default, so they are tried in turn.
var vorbisEncoders = [][]string{
	{"-c:a", "libvorbis"},
	{"-c:a", "vorbis", "-strict", "-2"},
	{"-c:a", "libopus"},
}

// convertToOgg turns a sound the mod ships in some format we do not read into
// one the player does. ffmpeg does it when it is installed; when it is not, the
// migration says so and gives the command to run, rather than quietly dropping
// the sound.
func convertToOgg(data []byte, sourceName, target string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("%s needs converting and ffmpeg is not installed; "+
			"convert it yourself with: ffmpeg -i %s %s", sourceName, sourceName, target)
	}
	var lastErr error
	for _, encoder := range vorbisEncoders {
		arguments := append([]string{"-hide_banner", "-loglevel", "error", "-y", "-i", "pipe:0"}, encoder...)
		command := exec.Command(ffmpeg, append(arguments, "-f", "ogg", target)...)
		command.Stdin = bytes.NewReader(data)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if err := command.Run(); err == nil {
			return nil
		} else {
			lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	return fmt.Errorf("converting %s: %w", sourceName, lastErr)
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
