package zdwep

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/memmaker/voxelfps/audio"
	"github.com/memmaker/voxelfps/png_chunks"
	"github.com/memmaker/voxelfps/tpack"
	"github.com/memmaker/voxelfps/wep"
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
	// The pictures first, because what they end up called is part of what an
	// actor says, and an actor is named by the hash of what it says.
	renamed, views, err := writeSharedSprites(result, assets, weaponsDir)
	if err != nil {
		return err
	}
	renameActorSprites(result, renamed, views)
	// How big the two pickup pictures are is measured here, off the store, so
	// the game never has to work it out from the packed atlas at spawn time.
	if err := wep.MeasurePickups(weaponsDir, result.Weapon); err != nil {
		return err
	}
	if err := writeActors(result, weaponsDir); err != nil {
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
// shared picture store, one file each, and reports what each one is now
// called. They are not packed here: which weapons are installed is the game's
// business, not the migration's, so the game packs them into one atlas when it
// starts and keeps it until the set changes.
//
// The name is the frame's own plus a short hash of the picture, because a
// sprite name is only unique inside the mod that defines it. Brutal Doom and
// Final Doomer both ship a SMOKA0 and they are not the same picture - one is
// 56 pixels square and the other is 141 by 130 - so a store keyed by the bare
// name gave every Brutal Doom weapon Final Doomer's smoke, two and a half
// times too big, purely because Final Doomer happened to be migrated last.
// Two mods that really do ship the same bytes still share one file.
func writeSharedSprites(result *Result, assets *AssetIndex, weaponsDir string) (map[string]string, map[string][]string, error) {
	if len(result.ActorSprites) == 0 {
		return nil, nil, nil
	}
	store := filepath.Join(weaponsDir, wep.SpritesDir)
	if err := os.MkdirAll(store, 0o755); err != nil {
		return nil, nil, err
	}
	renamed := make(map[string]string, len(result.ActorSprites))
	views := make(map[string][]string)
	for _, name := range result.ActorSprites {
		if angles, several := assets.SpriteViews(name); several {
			stored, err := writeSpriteViews(name, angles, assets, store)
			if err != nil {
				return nil, nil, err
			}
			views[name] = stored
			// What a step names is still the view from behind, so a frame that
			// is never looked at from anywhere else draws exactly what it drew
			// before this. The other seven hang off the actor.
			renamed[name] = stored[backView]
			continue
		}
		file, ok := assets.Sprite(name)
		if !ok {
			continue // already counted as missing, and already blanked
		}
		data, err := assets.ReadAll(file)
		if err != nil {
			return nil, nil, err
		}
		encoded, err := asPNG(data, name)
		if err != nil {
			return nil, nil, err
		}
		stored := wep.SharedSpriteName(name, encoded)
		renamed[name] = stored
		if err := writeSharedSprite(store, stored, encoded); err != nil {
			return nil, nil, err
		}
	}
	return renamed, views, nil
}

// backView is where in the eight the picture a step names lives: the view from
// behind, which is what the player who threw the thing is looking at.
const backView = 4

// writeSpriteViews puts all eight angles of one frame in the store and returns
// what each went in as, in view-direction order. Three of the eight are
// commonly the mirrored half of a NAMEFRFR lump, and those are written out
// mirrored here rather than flipped at draw time: it is the same picture every
// frame it is drawn, so turning it round once at migration time costs nothing
// and keeps the draw path unaware that rotations exist.
//
// Each view goes in under its own rotation - MISLE1 through MISLE8 - because
// the store is keyed by name and the eight are eight different pictures.
func writeSpriteViews(name string, angles [Views]SpriteView, assets *AssetIndex, store string) ([]string, error) {
	stored := make([]string, Views)
	for i, angle := range angles {
		data, err := assets.ReadAll(angle.File)
		if err != nil {
			return nil, err
		}
		viewName := name[:5] + string(rune('1'+i))
		encoded, err := asPNG(data, viewName)
		if err != nil {
			return nil, err
		}
		if angle.Mirror {
			if encoded, err = mirrorPNG(encoded, viewName); err != nil {
				return nil, err
			}
		}
		stored[i] = wep.SharedSpriteName(viewName, encoded)
		if err := writeSharedSprite(store, stored[i], encoded); err != nil {
			return nil, err
		}
	}
	return stored, nil
}

// writeSharedSprite puts one picture in the store. The store is keyed by the
// hash of what is in the file, so a picture that is already there is the same
// picture and there is nothing to do.
func writeSharedSprite(store, stored string, encoded []byte) error {
	target := filepath.Join(store, stored+".png")
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	return os.WriteFile(target, encoded, 0o644)
}

// mirrorPNG turns a picture round, hotspot and all. The hotspot is measured
// from the left edge, so mirrored it is measured from what used to be the
// right one - GZDoom does the same arithmetic on a mirrored rotation - and a
// picture that carried no hotspot still carries none.
func mirrorPNG(data []byte, name string) ([]byte, error) {
	decoded, _, err := decodePicture(data, name)
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	flipped := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			flipped.Set(bounds.Dx()-1-x, y, decoded.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, flipped); err != nil {
		return nil, err
	}
	grabX, grabY, has := png_chunks.ReadGrabOffset(bytes.NewReader(data))
	keptX, keptY := tpack.OffsetsFromPNG(data)
	if !has || grabX != keptX || grabY != keptY {
		// No hotspot, or one the atlas packer throws away as implausible.
		// Either way there is nothing to carry over.
		return out.Bytes(), nil
	}
	var withGrab bytes.Buffer
	if err := png_chunks.InjectGrabChunk(bytes.NewReader(out.Bytes()), &withGrab,
		bounds.Dx()-grabX, grabY); err != nil {
		return nil, err
	}
	return withGrab.Bytes(), nil
}

// renameActorSprites points the things a weapon spawns at the names their
// pictures actually went into the store under. Only the spawned actors: a
// weapon's own frames live in its own atlas, where a bare name is unique
// because the folder holds one mod's weapon.
func renameActorSprites(result *Result, renamed map[string]string, views map[string][]string) {
	if len(renamed) == 0 {
		return
	}
	for i, name := range result.ActorSprites {
		if stored, ok := renamed[name]; ok {
			result.ActorSprites[i] = stored
		}
	}
	// The pickup pictures are named the same way and live in the same store,
	// so they are renamed here with everything else rather than left pointing
	// at a name the store never used.
	if stored, ok := renamed[result.Weapon.Icon]; ok {
		result.Weapon.Icon = stored
	}
	if stored, ok := renamed[result.Weapon.AmmoIcon]; ok {
		result.Weapon.AmmoIcon = stored
	}
	for _, actor := range result.Actors {
		for i := range actor.States.Steps {
			step := &actor.States.Steps[i]
			carry := func(name string) string {
				stored, ok := renamed[name]
				if !ok {
					return name
				}
				// The other seven angles ride on the actor, keyed by the name
				// its own step names, so what draws it needs nothing but the
				// picture it was already going to draw and a direction.
				if angles, several := views[name]; several {
					if actor.Views == nil {
						actor.Views = make(map[string][]string)
					}
					actor.Views[stored] = angles
				}
				return stored
			}
			step.Sprite = carry(step.Sprite)
			// A keep marker keeps its own name - it is punctuation, not a
			// picture - but the names it can land on are pictures, and they
			// went into the store under the store's names.
			for j, name := range step.SpriteAlt {
				step.SpriteAlt[j] = carry(name)
			}
		}
	}
}

// asPNG makes sure a picture is one, whatever the mod stored it as.
func asPNG(data []byte, name string) ([]byte, error) {
	decoded, kind, err := decodePicture(data, name)
	if err != nil {
		return nil, err
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

// decodePicture reads a picture a mod shipped, repairing the container when a
// strict decoder will not have it. Doom mods carry PNGs with checksums that do
// not match and transparency chunks the spec forbids; GZDoom reads them anyway
// and so, now, do we.
func decodePicture(data []byte, name string) (image.Image, string, error) {
	decoded, kind, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return decoded, kind, nil
	}
	repaired, changed, repairErr := png_chunks.Repair(data)
	if repairErr != nil || !changed {
		return nil, "", fmt.Errorf("sprite %s: %w", name, err)
	}
	decoded, kind, err = image.Decode(bytes.NewReader(repaired))
	if err != nil {
		return nil, "", fmt.Errorf("sprite %s: %w", name, err)
	}
	return decoded, "repaired", nil
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
		decoded, _, err := decodePicture(data, name+" ("+file+")")
		if err != nil {
			return err
		}
		offsetX, offsetY := tpack.OffsetsFromPNG(data)
		images = append(images, tpack.NamedImage{Image: decoded, Name: name, OffsetX: offsetX, OffsetY: offsetY})
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
		return repairWAV(data), ".wav", nil
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
	return repairWAV(converted), ".wav", nil
}

// repairWAV corrects a data chunk that claims more bytes than the file holds,
// which is the other half of the truncated-sound problem: `endWhenSilent` in
// the player copes with a header that overruns by a few bytes, and this is
// what to do about one that overruns by four gigabytes.
//
// **The broken header is ours.** ffmpeg is handed the sound on a pipe and
// writes the WAV back on one, and a pipe cannot be seeked - so it never goes
// back to patch the two sizes it could not know in advance and leaves both at
// 0xFFFFFFFF, the "read until the stream ends" marker. beep keeps a data size
// in an int32, where that is -1, and its decoder stops before the first
// sample, because `d.pos >= d.h.DataSize` is true at position zero.
//
// So the three sounds these mods ship as FLAC - the BFG10K's idle hum and its
// spin-down, the Unmaker's shot - came out as twenty-seven kilobytes of
// silence each. And an empty buffer is one step away from silencing the whole
// game: beep.Loop asked to loop no samples never returns from Stream, on the
// audio callback's own goroutine.
//
// The real length is what the file actually holds, which is the one thing a
// marker like that says to work out for yourself. Fixing it here rather than
// giving ffmpeg a temp file to seek in keeps the conversion a pipe, which is
// what makes it a pure function of the bytes going in.
func repairWAV(data []byte) []byte {
	const header = 12 // "RIFF", the file size, "WAVE"
	for at := header; at+8 <= len(data); {
		size := int64(binary.LittleEndian.Uint32(data[at+4:]))
		body := int64(at) + 8
		if string(data[at:at+4]) != "data" {
			at = int(body + size + size%2)
			continue
		}
		if body+size <= int64(len(data)) {
			return data // the header is telling the truth
		}
		repaired := make([]byte, len(data))
		copy(repaired, data)
		binary.LittleEndian.PutUint32(repaired[at+4:], uint32(int64(len(data))-body))
		binary.LittleEndian.PutUint32(repaired[4:], uint32(len(data)-8))
		return repaired
	}
	return data
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
		// -ar is the speaker's rate: nothing between a cue and the speaker
		// resamples, so a sound written at any other rate plays at the ratio
		// between the two.
		"-y", "-i", "pipe:0", "-c:a", "pcm_s16le", "-ar", strconv.Itoa(audio.SampleRate),
		"-f", "wav", "pipe:1")
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
