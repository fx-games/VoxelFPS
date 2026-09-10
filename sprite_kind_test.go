package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// Every folder under assets/sprites has to be loadable and has to index frames
// its own sheet actually has. Six hundred of these folders were written by two
// importers off two DOS games' own data, and the failure mode that matters is
// silent: a frame index one past the end of the grid draws the wrong picture,
// or nothing, and nobody notices until a particular guard happens to die on
// screen. This walks the installed set once and checks every index against the
// picture's real size, which costs a second and covers the whole import.
func TestEveryInstalledSpriteKindIndexesFramesItsSheetHas(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	if len(kinds) == 0 {
		t.Fatalf("%s holds no sprite kinds", spriteDir)
	}

	for id, kind := range kinds {
		config, err := imageConfig(kind.sheetPath)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if kind.Tile.X <= 0 || kind.Tile.Y <= 0 {
			t.Errorf("%s: tile size %dx%d", id, kind.Tile.X, kind.Tile.Y)
			continue
		}
		columns := config.Width / kind.Tile.X
		rows := config.Height / kind.Tile.Y
		if columns == 0 || rows == 0 {
			t.Errorf("%s: a %dx%d sheet holds no %dx%d tiles",
				id, config.Width, config.Height, kind.Tile.X, kind.Tile.Y)
			continue
		}
		last := uint16(columns*rows - 1)
		for name, anim := range kind.Animations {
			if len(anim.Frames) == 0 {
				t.Errorf("%s: animation %q has no facings", id, name)
			}
			for facing, frames := range anim.Frames {
				if len(frames) == 0 {
					t.Errorf("%s: animation %q facing %d has no frames", id, name, facing)
				}
				for _, frame := range frames {
					if frame > last {
						t.Errorf("%s: animation %q facing %d names frame %d, and the sheet holds %d",
							id, name, facing, frame, last+1)
					}
				}
			}
		}
	}
}

// An actor needs all six animations the engine plays, because nothing checks at
// the point of use: spawnActor reads the last frame of AnimDying to size the
// collider a corpse gets, and updateActors sends the cursor into AnimHit and
// AnimFire without asking whether they exist. A kind that is missing one still
// loads - the loader falls back to the idle - and this is what says out loud
// which ones fell back rather than being imported.
func TestEveryInstalledActorHasAllSixAnimations(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	for id, kind := range kinds {
		if !kind.Actor {
			continue
		}
		for anim := quads.AnimIndex(0); anim < quads.AnimCount; anim++ {
			name := animOrder[anim]
			if kind.Animations[name] == nil {
				t.Errorf("%s is an actor with no %q animation", id, name)
			}
		}
		// A directional idle is what the map file's own TextureIndices come
		// from, and the engine reads TextureIndices[viewDirection] with no
		// bounds check of its own: anything but one or eight is a crash the
		// first time somebody walks round it.
		if facings := len(kind.Animations["idle"].Frames); facings != 1 && facings != 8 {
			t.Errorf("%s idles from %d facings, want 1 or 8", id, facings)
		}
	}
}

// imageConfig reads a PNG's size without decoding the pixels, which for six
// hundred sheets is the difference between a test and a coffee break.
func imageConfig(path string) (image.Config, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return image.Config{}, err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	return config, err
}

// A hostile with no usable attack is an enemy that walks up to you and stands
// there. Both importers read the source game's own numbers, and a zero in any
// of them - a reach of nothing, damage of nothing - is an import that went
// wrong rather than a character that is meant to be harmless: harmless ones are
// not marked hostile in the first place.
func TestEveryHostileKindCanActuallyAttack(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	hostiles := 0
	for id, kind := range kinds {
		if !kind.Hostile {
			continue
		}
		hostiles++
		if !kind.Actor {
			t.Errorf("%s is hostile but is not an actor", id)
		}
		if kind.Attack == nil {
			t.Errorf("%s is hostile with no attack", id)
			continue
		}
		if kind.Attack.Damage <= 0 {
			t.Errorf("%s attacks for %d", id, kind.Attack.Damage)
		}
		if kind.Attack.Range <= 0 {
			t.Errorf("%s attacks from %v away", id, kind.Attack.Range)
		}
	}
	if hostiles == 0 {
		t.Error("nothing installed is hostile, so nothing shoots back")
	}
}

// An actor whose kind named no attack still has to have a reach and a cooldown,
// or the one branch that decides whether it is close enough is comparing
// against zero and it never attacks at all.
func TestAnActorWithNoAttackStillHasAReachAndACooldown(t *testing.T) {
	bare := &Actor{}
	if got := bare.attackRange(); got != hostileMeleeRange {
		t.Errorf("reach with no kind is %v, want %v", got, hostileMeleeRange)
	}
	if got := bare.attackInterval(); got != hostileAttackInterval {
		t.Errorf("cooldown with no kind is %v, want %v", got, hostileAttackInterval)
	}

	named := &Actor{kind: &SpriteKind{Attack: &SpriteAttack{Range: 9, Interval: 0.5}}}
	if got := named.attackRange(); got != 9 {
		t.Errorf("reach is %v, want the kind's own 9", got)
	}
	if got := named.attackInterval(); got != 0.5 {
		t.Errorf("cooldown is %v, want the kind's own 0.5", got)
	}
}

// A character drawn from eight sides has to name eight different frames for
// them. The two games number their views from different starting points - see
// viewRotation in tools/importss - and every mistake in lining one up with the
// other shows up here first: a rotation done with the wrong modulus, or a
// clamp where there should have been a wrap, lands two facings on one picture
// and the character stops turning through part of its circle.
func TestADirectionalIdleNamesADifferentFrameForEveryFacing(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	for id, kind := range kinds {
		idle := kind.Animations["idle"]
		if idle == nil || len(idle.Frames) != 8 {
			continue
		}
		seen := map[uint16]int{}
		for facing, frames := range idle.Frames {
			if len(frames) == 0 {
				continue
			}
			if first, taken := seen[frames[0]]; taken {
				t.Errorf("%s: facings %d and %d both idle on frame %d",
					id, first, facing, frames[0])
				break
			}
			seen[frames[0]] = facing
		}
	}
}

// A frame drawn on a quad of a different shape is a picture stretched. The two
// games disagree about frames - Blake Stone draws every one into a square 64,
// System Shock crops each to what is in it - so the quad has to take its shape
// from the tile rather than from a single number, or every System Shock sprite
// is squashed to the width of a Blake Stone one, and the box it is picked and
// bumped into by is squashed with it.
func TestEveryKindsQuadIsTheShapeOfItsFrames(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	for id, kind := range kinds {
		if kind.Tile.X <= 0 || kind.Tile.Y <= 0 {
			t.Errorf("%s: tile %dx%d", id, kind.Tile.X, kind.Tile.Y)
			continue
		}
		size := kind.Size()
		if size.X() <= 0 || size.Y() <= 0 {
			t.Errorf("%s: drawn at %v", id, size)
			continue
		}
		tileAspect := float32(kind.Tile.X) / float32(kind.Tile.Y)
		quadAspect := size.X() / size.Y()
		if diff := tileAspect - quadAspect; diff > 0.001 || diff < -0.001 {
			t.Errorf("%s: %dx%d frames on a quad of %.3f:1, want %.3f:1 - its texels are not square",
				id, kind.Tile.X, kind.Tile.Y, quadAspect, tileAspect)
		}
	}
}

// A thing drawn from eight angles and half a dozen frames is picked out by the
// picture on the screen, not by an average of all of them. Its one collision box
// is the middle of every view of the whole animation - see
// geometry.BoxAcrossDirections, and the solver that must not care where the
// player looks - and for a bird twice as wide with its wings out as with them
// folded, that box is most of a wingspan beside what you are aiming at.
func TestADirectionalKindCarriesABoxPerFrame(t *testing.T) {
	kinds, err := loadSpriteKinds(spriteDir)
	if err != nil {
		t.Fatalf("loading %s: %v", spriteDir, err)
	}
	kind := kinds["ss_avian_mut"]
	if kind == nil {
		t.Skip("the avian mutant is not installed")
	}
	facings := kind.pickBoxes()[quads.AnimIdle]
	idle := kind.Animations["idle"]
	if len(facings) != len(idle.Frames) {
		t.Fatalf("%d facings drawn and %d measured", len(idle.Frames), len(facings))
	}
	width := func(box voxel.AABB) float32 { return box.Max().X() - box.Min().X() }
	median, ok := kind.box("idle")
	if !ok {
		t.Fatal("the kind has no measured box of its own")
	}

	narrowest, widest := width(facings[0][0]), width(facings[0][0])
	sameWithinAFacing := true
	for facing, frames := range facings {
		if len(frames) != len(idle.Frames[facing]) {
			t.Errorf("facing %d is %d frames and %d boxes", facing, len(idle.Frames[facing]), len(frames))
		}
		for frame, box := range frames {
			if box.Min().Y() != 0 || box.Max().Y() <= 0 {
				t.Errorf("facing %d frame %d stands from %v to %v", facing, frame, box.Min().Y(), box.Max().Y())
			}
			if w := width(box); w < narrowest {
				narrowest = w
			} else if w > widest {
				widest = w
			}
			if width(box) != width(frames[0]) {
				sameWithinAFacing = false
			}
		}
	}
	if narrowest >= width(median) {
		t.Errorf("the narrowest picture is %v wide and the one box is %v - nothing was measured per frame",
			narrowest, width(median))
	}
	if widest <= width(median) {
		t.Errorf("the widest picture is %v wide and the one box is %v - the one box is supposed to be the middle",
			widest, width(median))
	}
	if sameWithinAFacing {
		t.Errorf("every frame of every facing is the same width - the boxes are not per frame")
	}
}
