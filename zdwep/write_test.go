package zdwep

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/memmaker/goray/wep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spritePNG makes a small image to stand in for a Doom sprite. The colour is
// derived from the name so a packed atlas is not one flat block.
func spritePNG(t *testing.T, name string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	shade := uint8(len(name) * 17)
	for y := 1; y < 7; y++ {
		for x := 1; x < 7; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: 128, B: 200, A: 255})
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, img))
	return out.Bytes()
}

// modWithAssets builds a source holding the test mod plus every sprite and
// sound it turns out to need.
func modWithAssets(t *testing.T, sprites []string) *Source {
	t.Helper()
	files := fstest.MapFS{
		"DECORATE": &fstest.MapFile{Data: []byte(testMod)},
		"SNDINFO": &fstest.MapFile{Data: []byte(
			"testshotgun/fire\tTSHTFIRE\nweapons/dryfire\tDRYFIRE\n")},
		// Not real vorbis: the migration copies an ogg through untouched, and
		// loading checks that the file is there, so bytes are enough here.
		"sounds/TSHTFIRE.ogg": &fstest.MapFile{Data: []byte("OggS-fire")},
		"sounds/DRYFIRE.ogg":  &fstest.MapFile{Data: []byte("OggS-dry")},
	}
	for _, sprite := range sprites {
		files["sprites/"+sprite+".png"] = &fstest.MapFile{Data: spritePNG(t, sprite)}
	}
	src, err := NewSourceFS(files)
	require.NoError(t, err)
	return src
}

func writeTestShotgun(t *testing.T) (string, *Result) {
	t.Helper()
	result := compileTestShotgun(t)
	src := modWithAssets(t, result.Sprites)
	assets := NewAssetIndex(src)

	missingSprites, missingSounds := assets.MissingAssets(result)
	require.Empty(t, missingSprites)
	require.Empty(t, missingSounds)

	dir := t.TempDir()
	require.NoError(t, Write(result, assets, src, dir))
	return dir, result
}

func TestWriteProducesALoadableWeaponsFolder(t *testing.T) {
	dir, result := writeTestShotgun(t)

	// The key map is the one file the migration does not write; a folder
	// without it does not load, which is what the tool fills in.
	require.NoError(t, os.WriteFile(filepath.Join(dir, wep.KeyMapFile),
		[]byte("1: "+result.Weapon.ID+"\n"), 0o644))

	library, err := wep.Load(dir)
	require.NoError(t, err)

	require.Len(t, library.Weapons, 1)
	weapon := library.Weapons[0]
	assert.Equal(t, "test_shotgun", weapon.ID)
	assert.Equal(t, "Test Shotgun", weapon.Tag)
	assert.Equal(t, "test_shotgun", library.Keys[1])

	// Every sprite the steps ask for came through into the atlas.
	assert.Contains(t, library.Sprites[weapon.ID], "TSHTA0")
	assert.Contains(t, library.Sprites[weapon.ID], "TSMKA0")

	assert.Contains(t, library.Actors[weapon.ID], "testsmoke")

	// The sounds landed under the names the ops play them by.
	assert.Equal(t, "sounds/testshotgun/fire.ogg", weapon.Sounds["testshotgun/fire"])
	_, err = os.Stat(filepath.Join(dir, weapon.ID, "sounds", "testshotgun", "fire.ogg"))
	assert.NoError(t, err)
}

func TestWriteIsByteIdenticalBetweenRuns(t *testing.T) {
	first, _ := writeTestShotgun(t)
	second, _ := writeTestShotgun(t)

	for _, name := range []string{wep.WeaponFile, wep.AtlasFile} {
		a, err := os.ReadFile(filepath.Join(first, "test_shotgun", name))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(second, "test_shotgun", name))
		require.NoError(t, err)
		assert.True(t, bytes.Equal(a, b), "%s differs between two runs of the same migration", name)
	}
}

func TestLoadCollectsEveryProblemBeforeFailing(t *testing.T) {
	dir, result := writeTestShotgun(t)
	path := filepath.Join(dir, result.Weapon.ID, wep.WeaponFile)

	var weapon wep.Weapon
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &weapon))

	// Three separate problems, so the report has to name all three rather than
	// stopping at the first.
	fire, ok := weapon.States.Label("Fire")
	require.True(t, ok)
	weapon.States.Steps[fire].Ops[0].To = "Nowhere"
	weapon.States.Steps[fire].Ops = append(weapon.States.Steps[fire].Ops,
		wep.Op{Op: "a_overlay"})
	weapon.States.Steps[fire+1].Sprite = "NOPEA0"

	patched, err := json.MarshalIndent(weapon, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, patched, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, wep.KeyMapFile),
		[]byte("1: test_shotgun\n4: no_such_weapon\n"), 0o644))

	_, err = wep.Load(dir)
	require.Error(t, err)
	message := err.Error()
	assert.Contains(t, message, `jumps to "Nowhere"`)
	assert.Contains(t, message, `op "a_overlay"`)
	assert.Contains(t, message, "NOPEA0")
	assert.Contains(t, message, `no weapon folder named "no_such_weapon"`)
}

func TestLoadRejectsAStaleFormat(t *testing.T) {
	dir, result := writeTestShotgun(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, wep.KeyMapFile), []byte("1: test_shotgun\n"), 0o644))

	path := filepath.Join(dir, result.Weapon.ID, wep.WeaponFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["format"] = wep.Format + 1
	patched, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, patched, 0o644))

	_, err = wep.Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this build reads")
}

// Two weapons that spawn the same actor share one file, because the file is
// named by what is in it. This is what took a whole mod's weapons folder from
// 356 megabytes to 43, so it is worth a test that would notice it stopping.
func TestActorsAreSharedBetweenWeapons(t *testing.T) {
	dir := t.TempDir()

	// The same mod compiled twice under two ids, standing in for two weapons
	// that reach the same smoke.
	for _, id := range []string{"first_gun", "second_gun"} {
		result := compileTestShotgun(t)
		result.Weapon.ID = id
		src := modWithAssets(t, result.Sprites)
		require.NoError(t, Write(result, NewAssetIndex(src), src, dir))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, wep.KeyMapFile),
		[]byte("1: first_gun\n2: second_gun\n"), 0o644))

	store, err := os.ReadDir(filepath.Join(dir, wep.ActorsDir))
	require.NoError(t, err)
	assert.Len(t, store, 1, "one actor between them, so one file")

	library, err := wep.Load(dir)
	require.NoError(t, err)
	require.Len(t, library.Weapons, 2)

	// Both weapons resolve it, and to the very same actor.
	first := library.Actors["first_gun"]["testsmoke"]
	second := library.Actors["second_gun"]["testsmoke"]
	require.NotNil(t, first)
	assert.Same(t, first, second, "read once, however many weapons want it")

	// And a weapon that no longer exists leaves its actor file behind rather
	// than taking a sibling's with it. The key map is the player's, so it has
	// to let go of the weapon too; loading says so if it does not.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "second_gun")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, wep.KeyMapFile),
		[]byte("1: first_gun\n"), 0o644))
	library, err = wep.Load(dir)
	require.NoError(t, err)
	assert.Len(t, library.Weapons, 1)
	assert.NotNil(t, library.Actors["first_gun"]["testsmoke"])
}
