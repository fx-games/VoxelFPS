package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
)

// A SpriteKind is a thing in the world that is a picture rather than a model:
// a guard, a mutant, a barrel that flickers. It is a folder under
// assets/sprites/ holding a sheet.png of equal-sized frames, a sprite.json
// saying which frames make which animation, and - for anything that makes a
// noise - a sounds/ folder.
//
// It is assets/meshes/ and assets/weapons/ applied to sprites, and for the same
// reason: what exists is decided by which folders are present, so importing a
// hundred characters out of two DOS games is a hundred folders and not a
// hundred lines of Go. There is deliberately no list of kinds in code.
type SpriteKind struct {
	// ID is the folder's name, and what a map entity's SpriteSheetName says.
	ID string `json:"-"`
	// Name is what the editor's placement menu calls it, and what an entity
	// placed from that menu is named in the map file.
	Name string `json:"name"`
	// Tile is how big one frame on the sheet is. The sheet is a plain grid, so
	// this plus the sheet's own width is the whole layout.
	Tile geometry.Point `json:"tile"`
	// Actor is whether this walks and can be shot. A kind that does is placed
	// with the editor's actor tool and gets an Actor; one that does not is a
	// decoration in the props palette.
	Actor bool `json:"actor"`
	// Health is how much this actor has, or zero for the default. Ignored for a
	// decoration.
	Health int `json:"health"`
	// Hostile is whether this one comes after the player. An actor that is not
	// walks its patrol and can be talked to; one that is chases, shoots and has
	// to be dealt with - see hostile.go. It is on the kind rather than on the
	// map entity because it is a fact about what the thing *is*: a mutant is
	// hostile wherever it is standing.
	Hostile bool `json:"hostile"`
	// Attack is what it does when it gets close enough, or nil for something
	// that has no attack at all. Both games record this about their own
	// characters - what the attack does, how far it reaches, whether it throws
	// something - and both importers read it rather than guessing.
	Attack *SpriteAttack `json:"attack"`
	// Scale multiplies the size the sprite is drawn at, for art that was drawn
	// against a different notion of how tall a person is. Zero means one.
	Scale float32 `json:"scale"`
	// Animations are this kind's own, by the name the engine knows them by:
	// "idle", "walk", "dying", "hit", "prepare_fire", "fire". Only "idle" is
	// required - a barrel has nothing else - and a missing one falls back to
	// idle rather than being an error, so a kind whose source art never had a
	// pain frame still runs.
	Animations map[string]*SpriteAnim `json:"animations"`
	// Sounds are the cues this kind plays, by what they are for: "alert" when
	// it first spawns, "attack" when it fires, "death" when it dies. The value
	// is a cue name under the folder's own sounds/ directory, which is loaded
	// into a scope of this kind's ID - exactly how a weapon's sounds work.
	Sounds map[string]string `json:"sounds"`
	// Provenance records which game and which of its own names this came out
	// of, so a re-import can be checked against what is already on disk.
	Provenance map[string]string `json:"provenance,omitempty"`

	// animBase is where this kind's six animations start in SpriteDrawer's one
	// list, filled in by registerSpriteKinds. A sprite of this kind carries the
	// same number, which is what makes AnimWalk mean this kind's walk.
	animBase int
	// sheetPath is where the picture is, resolved once so a spawn does not have
	// to build it again.
	sheetPath string
	// atlas is the sheet uploaded, kept because it is shared: a room of a
	// hundred guards is one picture, and uploading it once a guard is a hundred
	// copies of the same texture on the card and a hundred draw batches to step
	// through. Loaded the first time one of this kind spawns, so a folder
	// nobody has placed still costs nothing.
	atlas *glhf.TextureAtlasFixed
	// picking is this kind's boxes for the ray, per animation, facing and frame,
	// read the first time one of these spawns and shared by all of them after
	// that. Nil until then; empty for a folder that has no measured frames,
	// which is what tells "not read yet" from "nothing to read".
	picking quads.PickBoxes
}

// Size is how big one of this kind's quads is in world units.
//
// Scale is the height, because that is the measurement that means something
// about a thing standing in a room. The width follows the frame's own shape:
// the two games do not agree about tiles - Blake Stone draws every frame into a
// square 64 and System Shock crops each one to what is in it - so a kind whose
// frames are 140 by 86 drawn on a square quad is a mutant squashed to two
// thirds of its width, with a picking box squashed with it.
func (k *SpriteKind) Size() mgl32.Vec2 {
	if k.Tile.Y <= 0 {
		return mgl32.Vec2{k.Scale, k.Scale}
	}
	return mgl32.Vec2{k.Scale * float32(k.Tile.X) / float32(k.Tile.Y), k.Scale}
}

// Atlas is this kind's sheet, uploaded on first use.
func (k *SpriteKind) Atlas() *glhf.TextureAtlasFixed {
	if k.atlas == nil {
		k.atlas = quads.Disk.TextureAtlasFixed(k.sheetPath, k.Tile.X, k.Tile.Y)
	}
	return k.atlas
}

// SpriteAttack is one character's attack: what it costs whoever is hit, how far
// it reaches, and whether it throws something that travels or hits at once.
type SpriteAttack struct {
	Damage int `json:"damage"`
	// Range is how close it has to be, in world units. A swing is a couple of
	// voxels; a gun is most of a room.
	Range float32 `json:"range"`
	// Projectile makes the attack a thing that flies rather than a shot that
	// lands the instant it is fired - a canister alien's spit, a pod's acid.
	Projectile bool `json:"projectile"`
	// Interval is the wait between attacks, in seconds. Zero takes the
	// engine's own default.
	Interval float64 `json:"interval"`
}

// SpriteAnim is one animation: the frames it plays, in order, per facing.
// Frames is indexed by direction first - eight entries for something you can
// walk round, one for something that looks the same from everywhere, which is
// what a death or an explosion is.
type SpriteAnim struct {
	Frames [][]uint16 `json:"frames"`
	// Box is the part of the tile this animation's frames actually draw on, in
	// the tile's own units - see geometry.Box, and the importers' measureBoxes,
	// which is where it is worked out.
	//
	// It is per animation because a thing's shape is: a guard standing up and
	// the corpse he becomes are not the same box, and neither is a mutant
	// mid-lunge. Measured at spawn there is only ever one frame to measure, so
	// every animation got the idle frame's box and the box was wrong for five
	// of the six. A kind with none written falls back to exactly that.
	Box *geometry.Box `json:"box,omitempty"`
	// FPS is how fast it runs. Zero means the engine's own default rather than
	// an animation that never advances.
	FPS float32 `json:"fps"`
	// Loop is whether it starts again at the end. A walk does; a death does
	// not.
	Loop bool `json:"loop"`
}

// spriteDir is the folder of folders, one to a kind.
const spriteDir = "assets/sprites"

// sourceScale is a multiplier on top of every kind's own scale, by the prefix
// its folder carries: "ss_" is everything out of System Shock, "bs_" everything
// out of Blake Stone.
//
// Each importer works its kinds' heights out from what its game did - a
// Wolfenstein tile is a wall, a System Shock pixel is a ninety-sixth of a map
// square - so the numbers on disk are the originals' own and want no correcting
// on the whole. This is the knob for when they want it anyway: one per import,
// applied to every sprite from that import at once, set by -ss-scale and
// -bs-scale. One means the art as its game drew it.
var sourceScale = map[string]float32{"ss_": 1, "bs_": 1}

// scaleFor is the multiplier this kind's folder falls under, or one for a
// folder that is nobody's import. The prefix is on the folder's own name, not on
// the path: a kind lives under a role and a category now, so the id starts with
// "props/" long before it gets to "ss_".
func scaleFor(id string) float32 {
	name := path.Base(id)
	for prefix, scale := range sourceScale {
		if strings.HasPrefix(name, prefix) {
			return scale
		}
	}
	return 1
}

// The three roles a sprite kind is filed under, which are the three folders at
// the top of assets/sprites: what stands in a room, what the player picks up, and
// what walks. They are the importers' own - see tools/importss/categories.go -
// and they are what the editor's three sprite tools each browse a folder of.
const (
	propsRole  = "props"
	itemsRole  = "items"
	actorsRole = "actors"
)

// spriteRole is which of the three a kind's ID falls under, or "" for a folder
// that predates the sorting and sits at the top level.
func spriteRole(id string) string {
	role, _, nested := strings.Cut(id, "/")
	if !nested {
		return ""
	}
	return role
}

// spriteKindFile is what makes a folder under spriteDir a kind rather than a
// pile of pictures.
const spriteKindFile = "sprite.json"

// spriteSheetFile is the grid of frames every kind's animations index into.
const spriteSheetFile = "sheet.png"

// spriteSoundsDir is where a kind's cues live, if it has any.
const spriteSoundsDir = "sounds"

// defaultSpriteFPS is how fast an animation runs that does not say. Five frames
// a second is Wolfenstein's own walk, which is where most of this art is from.
const defaultSpriteFPS = 5

// animOrder is the six animations the engine knows, in the order SpriteDrawer
// is handed them - so a kind's own AnimWalk is always animBase+AnimWalk. A kind
// that has not got one of these is registered with its idle in that slot, which
// is why nothing downstream has to check whether an animation exists.
var animOrder = [quads.AnimCount]string{
	quads.AnimIdle:        "idle",
	quads.AnimWalk:        "walk",
	quads.AnimDying:       "dying",
	quads.AnimHit:         "hit",
	quads.AnimPrepareFire: "prepare_fire",
	quads.AnimFire:        "fire",
}

// loadSpriteKinds reads every folder under dir that has a sprite.json in it. A
// folder without one is skipped rather than being an error, the same way a
// mesh folder without a scenery.json is a plain model.
//
// It walks the whole tree rather than the top level: a kind is filed under a
// role and a category - see tools/importss/categories.go - so a kind's ID is its
// path below dir with forward slashes, which is what a map's SpriteSheetName
// holds and what the editor's browsers hand back.
func loadSpriteKinds(dir string) (map[string]*SpriteKind, error) {
	library := map[string]*SpriteKind{}
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() || entry.Name() != spriteKindFile {
			return nil
		}
		folder := filepath.Dir(p)
		id, err := filepath.Rel(dir, folder)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		kind := &SpriteKind{ID: filepath.ToSlash(id)}
		if err := json.Unmarshal(raw, kind); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if kind.Name == "" {
			kind.Name = kind.ID
		}
		if kind.Scale == 0 {
			kind.Scale = 1
		}
		kind.Scale *= scaleFor(kind.ID)
		kind.sheetPath = filepath.Join(folder, spriteSheetFile)
		if _, err := os.Stat(kind.sheetPath); err != nil {
			return fmt.Errorf("%s names no %s", kind.ID, spriteSheetFile)
		}
		if kind.Animations["idle"] == nil || len(kind.Animations["idle"].Frames) == 0 {
			return fmt.Errorf("%s has no idle animation", kind.ID)
		}
		library[kind.ID] = kind
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sprites: %w", err)
	}
	return library, nil
}

// spriteKindIDs is the folders in a fixed order, so the editor's palette and
// the number row mean the same thing from one run to the next. Go's map order
// does not, and a placement key that moved between runs would be unusable.
func (g *Game) spriteKindIDs() []string {
	ids := make([]string, 0, len(g.spriteKinds))
	for id := range g.spriteKinds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// registerSpriteKinds hands every kind's animations to the sprite drawer and
// remembers where each kind's block landed. It runs once, after the drawer
// exists and before any map is loaded, because a sprite carries its base
// number and a base handed out later would not reach a sprite already in the
// world.
//
// The engine's own six animations are registered first, at base zero, so
// anything that predates per-kind animations - the projectile effects, the
// scientist as it was - keeps working with the numbers it already uses.
func (g *Game) registerSpriteKinds() {
	for _, id := range g.spriteKindIDs() {
		kind := g.spriteKinds[id]
		kind.animBase = len(g.Sprites.Animations())
		for anim := quads.AnimIndex(0); anim < quads.AnimCount; anim++ {
			g.Sprites.AddAnimation(kind.spriteAnimation(animOrder[anim]))
		}
	}
}

// spriteAnimation turns one entry of the JSON into what SpriteDrawer wants. A
// name this kind has not got falls back to its idle: every kind has one, and a
// barrel asked for its death animation should stand there rather than crash.
func (k *SpriteKind) spriteAnimation(name string) quads.SpriteAnimation {
	anim := k.Animations[name]
	if anim == nil || len(anim.Frames) == 0 {
		anim = k.Animations["idle"]
	}
	fps := anim.FPS
	if fps <= 0 {
		fps = defaultSpriteFPS
	}
	// Every direction of one animation runs at the same speed and has the same
	// number of frames - SpriteDrawer times an animation off direction zero -
	// so the frame times come off the longest of them.
	longest := 0
	for _, frames := range anim.Frames {
		if len(frames) > longest {
			longest = len(frames)
		}
	}
	showTimes := make([]float32, longest)
	for i := range showTimes {
		showTimes[i] = 1 / fps
	}
	frames := make([][]uint16, len(anim.Frames))
	for i, direction := range anim.Frames {
		frames[i] = append([]uint16(nil), direction...)
	}
	return quads.SpriteAnimation{Frames: frames, ShowTimeInSeconds: showTimes, Loop: anim.Loop}
}

// box is the box one of this kind's animations deserves, at the size the quad
// is drawn - or false for a kind whose folder was written before boxes were
// measured, which falls back to measuring the frame off the card.
func (k *SpriteKind) box(name string) (voxel.AABB, bool) {
	anim := k.Animations[name]
	if anim == nil || anim.Box == nil || anim.Box.Empty() {
		return voxel.AABB{}, false
	}
	return quads.ScaledSpriteBox(*anim.Box, k.Size()), true
}

// pickBoxes is what this kind is picked out by: the box of the very picture on
// the screen, by animation, then by the angle it is seen from, then by the frame
// it is on.
//
// The box in sprite.json is one shape for a whole animation from every angle,
// because a solver must not care where the player is looking or which frame a
// walk cycle is on - see geometry.BoxAcrossDirections. A ray does care: it is
// testing a picture, and System Shock's avian mutant is twice as wide with its
// wings out as it is with them folded, from the same angle, one frame apart.
//
// The numbers come off the folder rather than off the picture - measured once at
// import, see geometry.SpriteBoxesFile - and are read on first spawn, so a kind
// nobody has placed costs nothing and a room of thirty of them reads once.
func (k *SpriteKind) pickBoxes() quads.PickBoxes {
	if k.picking != nil {
		return k.picking
	}
	k.picking = quads.PickBoxes{}
	frameBoxes, err := geometry.ReadFrameBoxes(filepath.Dir(k.sheetPath))
	if err != nil {
		log.Printf("sprites: %s has no usable %s: %s", k.ID, geometry.SpriteBoxesFile, err)
		return k.picking
	}
	size := k.Size()
	for anim := quads.AnimIndex(0); anim < quads.AnimCount; anim++ {
		facings := k.spriteAnimation(animOrder[anim]).Frames
		boxes := make([][]voxel.AABB, 0, len(facings))
		measured := false
		for _, frames := range facings {
			perFrame := make([]voxel.AABB, 0, len(frames))
			for _, cell := range frames {
				if int(cell) >= len(frameBoxes) {
					perFrame = append(perFrame, voxel.AABB{})
					continue
				}
				box := frameBoxes[cell].Box()
				if box.Empty() {
					// A frame that draws nothing has nothing to be picked out
					// by; the thing falls back to its animation's own box for
					// as long as it is on that frame.
					perFrame = append(perFrame, voxel.AABB{})
					continue
				}
				measured = true
				perFrame = append(perFrame, quads.ScaledSpriteBox(box, size))
			}
			boxes = append(boxes, perFrame)
		}
		if measured {
			k.picking[anim] = boxes
		}
	}
	return k.picking
}

// idleFrames is what a map entity of this kind carries as its TextureIndices:
// the first frame of its idle, one per direction. That is what the sprite falls
// back to before any animation is playing, and what the editor's ghost draws.
func (k *SpriteKind) idleFrames() []uint16 {
	idle := k.Animations["idle"]
	out := make([]uint16, 0, len(idle.Frames))
	for _, direction := range idle.Frames {
		if len(direction) == 0 {
			continue
		}
		out = append(out, direction[0])
	}
	return out
}

// loadSpriteKindSounds gives every kind that has a sounds/ folder its own cue
// scope, named for the kind. It is the same call a weapon makes; the scope
// keeps two kinds' "death" apart.
func (g *Game) loadSpriteKindSounds() {
	if g.audio == nil {
		// The same lazy construction the soundtrack, the logs and the weapons
		// make: whichever of them runs first builds the player.
		g.audio = audio.NewPlayer(audioSampleRate)
	}
	for _, id := range g.spriteKindIDs() {
		kind := g.spriteKinds[id]
		if len(kind.Sounds) == 0 {
			continue
		}
		root := filepath.Join(spriteDir, kind.ID, spriteSoundsDir)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		if err := g.audio.LoadCueTree(kind.ID, root); err != nil {
			log.Printf("[sprites] %s: %v", kind.ID, err)
			continue
		}
		g.audio.SoundsLoaded(kind.ID)
	}
}
