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

// All eight angles of a frame are migrated, in the order the engine asks for
// them: 0 the front, 4 the back, running round the thing's own left, which is
// Doom's rotation number less one. Three of the eight are the mirrored half of
// a lump that carries two, and they are marked so the writer can turn them
// round.
func TestSpriteViewsAreTheEightAnglesInViewOrder(t *testing.T) {
	src, err := NewSourceFS(fstest.MapFS{
		"sprites/MISLA1.png":   &fstest.MapFile{Data: []byte("front")},
		"sprites/MISLA8A2.png": &fstest.MapFile{Data: []byte("front quarter")},
		"sprites/MISLA7A3.png": &fstest.MapFile{Data: []byte("side")},
		"sprites/MISLA6A4.png": &fstest.MapFile{Data: []byte("back quarter")},
		"sprites/MISLA5.png":   &fstest.MapFile{Data: []byte("back")},
	})
	require.NoError(t, err)

	views, several := NewAssetIndex(src).SpriteViews("MISLA0")
	require.True(t, several)
	// Doom names the double lumps with the authored half first, so 6, 7 and 8
	// are the pictures and 2, 3 and 4 are those turned round: what is drawn is
	// the thing's right side, mirrored for its left.
	assert.Equal(t, [Views]SpriteView{
		{File: "sprites/MISLA1.png"},                 // rotation 1, the front
		{File: "sprites/MISLA8A2.png", Mirror: true}, // rotation 2
		{File: "sprites/MISLA7A3.png", Mirror: true}, // rotation 3, its left
		{File: "sprites/MISLA6A4.png", Mirror: true}, // rotation 4
		{File: "sprites/MISLA5.png"},                 // rotation 5, the back
		{File: "sprites/MISLA6A4.png"},               // rotation 6
		{File: "sprites/MISLA7A3.png"},               // rotation 7, its right
		{File: "sprites/MISLA8A2.png"},               // rotation 8
	}, views)
}

// One picture for every angle is not eight views of anything, and neither is a
// set with holes in it: three views of a thing you can walk all the way round
// leaves five angles nothing sensible can be guessed for.
func TestSpriteViewsRefusesOnePictureAndAPartialSet(t *testing.T) {
	src, err := NewSourceFS(fstest.MapFS{
		"sprites/SMOKA0.png":   &fstest.MapFile{Data: []byte("all angles")},
		"sprites/TRACA1.png":   &fstest.MapFile{Data: []byte("front")},
		"sprites/TRACA2A8.png": &fstest.MapFile{Data: []byte("two of them")},
	})
	require.NoError(t, err)
	assets := NewAssetIndex(src)

	_, several := assets.SpriteViews("SMOKA0")
	assert.False(t, several, "the mod draws this one once, for every angle")
	_, several = assets.SpriteViews("TRACA0")
	assert.False(t, several, "three of eight is not a set")

	// And the partial one is what the migration says out loud, because the
	// stored picture keeps the name that was asked for and nothing downstream
	// can tell afterwards that other views existed.
	result := &Result{ActorSprites: []string{"SMOKA0", "TRACA0"}}
	assert.Equal(t, []string{"TRACA0 (3 of 8 views)"}, assets.CollapsedViews(result))
}

// GZDoom's backwards search is a rule about one lump list, and a source stack
// is searched the other way round on purpose: the mod first and doom2.wad
// last, because a mod wants the base game only for what it did not ship
// itself. Run over the whole stack, "the last lump wins" gave the base game
// back every name the mod also uses - Brutal Doom shares 655 of them - so the
// super shotgun, the rocket, the shell casing and four frames of the
// chainsaw's idle came out vanilla, and the idle animation flicked between two
// different pairs of hands three times a second.
func TestAModOutranksTheGameItSitsOn(t *testing.T) {
	mod, err := NewSourceFS(fstest.MapFS{
		"SPRITES/WEAPONS/CHAINSAW/SAWGA0.png": &fstest.MapFile{Data: []byte("brutal doom's chainsaw")},
		"SOUNDS/HD/Chainsaw/DSSAWIDL.lmp":     &fstest.MapFile{Data: []byte("the better take")},
	})
	require.NoError(t, err)
	game, err := NewSourceFS(fstest.MapFS{
		"sprites/SAWGA0":  &fstest.MapFile{Data: []byte("vanilla doom's chainsaw")},
		"sprites/SAWGB0":  &fstest.MapFile{Data: []byte("vanilla doom's chainsaw")},
		"sounds/DSSAWIDL": &fstest.MapFile{Data: []byte("vanilla")},
	})
	require.NoError(t, err)
	mod.layer(game)
	index := NewAssetIndex(mod)

	file, ok := index.Sprite("SAWGA0")
	require.True(t, ok)
	assert.Equal(t, "SPRITES/WEAPONS/CHAINSAW/SAWGA0.png", file, "the mod ships its own")

	file, ok = index.Sprite("SAWGB0")
	require.True(t, ok)
	assert.Equal(t, "sprites/SAWGB0", file, "the mod does not, so the game's stands")

	sounds, ok := index.Sounds("DSSAWIDL")
	require.True(t, ok)
	assert.Equal(t, []string{"SOUNDS/HD/Chainsaw/DSSAWIDL.lmp"}, sounds)
}
