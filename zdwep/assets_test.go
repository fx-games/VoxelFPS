package zdwep

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A script names a spawned thing's frame without a rotation - SMOKA0 - and the
// mod ships the eight views of it. We draw one flat billboard, and the view
// that matters is the one the player gets: what a weapon spawns faces the way
// the player faces, so the player is looking at its back, which Doom numbers
// rotation 5. Brutal Doom authors its muzzle flash blank from behind for
// exactly that reason, and picking the front view instead put a fireball in
// the player's face on every shot.
func TestASpawnedFrameTakesTheViewFromBehind(t *testing.T) {
	files := fstest.MapFS{
		"sprites/PLMZA1.png":   &fstest.MapFile{Data: []byte("front")},
		"sprites/PLMZA2A8.png": &fstest.MapFile{Data: []byte("sides")},
		"sprites/PLMZA3A7.png": &fstest.MapFile{Data: []byte("sides")},
		"sprites/PLMZA4A6.png": &fstest.MapFile{Data: []byte("quarters")},
		"sprites/PLMZA5.png":   &fstest.MapFile{Data: []byte("back")},
	}
	src, err := NewSourceFS(files)
	require.NoError(t, err)
	assets := NewAssetIndex(src)

	file, ok := assets.Sprite("PLMZA0")
	require.True(t, ok)
	assert.Equal(t, "sprites/PLMZA5.png", file, "the back view is what the shooter sees")
}

// A frame the mod draws once for every angle is that one picture, whatever
// rotations the name suggests, and a set with no back view still has to answer
// with something rather than nothing.
func TestASpawnedFrameFallsBackToWhatTheModHas(t *testing.T) {
	src, err := NewSourceFS(fstest.MapFS{
		"sprites/SMOKA0.png": &fstest.MapFile{Data: []byte("all angles")},
	})
	require.NoError(t, err)
	file, ok := NewAssetIndex(src).Sprite("SMOKA0")
	require.True(t, ok)
	assert.Equal(t, "sprites/SMOKA0.png", file)

	src, err = NewSourceFS(fstest.MapFS{
		"sprites/TRACA1.png":   &fstest.MapFile{Data: []byte("front")},
		"sprites/TRACA2A8.png": &fstest.MapFile{Data: []byte("sides")},
	})
	require.NoError(t, err)
	file, ok = NewAssetIndex(src).Sprite("TRACA0")
	require.True(t, ok)
	assert.Equal(t, "sprites/TRACA1.png", file, "no view from behind, so the first one it has")
}

// GZDoom searches its lump list backwards, so the last file with a name wins.
// Keeping the first one handed Brutal Doom's plasma gun a bike part: it ships
// PLSTA0 twice, once under sprites/vehicles/bike and once - the one the plasma
// gun's Fire state means - under sprites/weapons/plasmagun.
func TestTheLastLumpWithANameWins(t *testing.T) {
	src, err := NewSourceFS(fstest.MapFS{
		"sprites/vehicles/bike/PLSTA0.png":     &fstest.MapFile{Data: []byte("exhaust flare")},
		"sprites/weapons/plasmagun/PLSTA0.png": &fstest.MapFile{Data: []byte("the gun firing")},
		"sounds/combat/chainsaw/DSSAWUP.lmp":   &fstest.MapFile{Data: []byte("original")},
		"sounds/hd/chainsaw/DSSAWUP.lmp":       &fstest.MapFile{Data: []byte("the better take")},
	})
	require.NoError(t, err)
	index := NewAssetIndex(src)

	file, ok := index.Sprite("PLSTA0")
	require.True(t, ok)
	assert.Equal(t, "sprites/weapons/plasmagun/PLSTA0.png", file)

	sounds, ok := index.Sounds("DSSAWUP")
	require.True(t, ok)
	assert.Equal(t, []string{"sounds/hd/chainsaw/DSSAWUP.lmp"}, sounds)
}
