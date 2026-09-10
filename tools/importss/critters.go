package main

// Where a critter's pictures are.
//
// This is the one thing System Shock's data files do not write down. OBJPROP.DAT
// has a per-critter array of frame counts, and in the shipped file it is all
// zeroes - the engine fills it in from the art at load time. So the layout has
// to be derived from the art itself, and it is regular:
//
//   - Every critter, in the order the property table lists them, has a slot.
//     A critter with ViewCount 8 takes eight resources in a multi-view block
//     (one per facing); one with ViewCount 1 takes a single resource.
//   - objart2.res holds five single-view postures first, one resource per
//     critter per posture, posture by posture: 1400 + 37*posture + critter,
//     thirty-seven being the number of critters. Then, from 1585, the
//     multi-view standing block.
//   - objart3.res holds the multi-view walking block, from 1880.
//
// The blocks of one resource are the frames of that one animation, each stored
// as the difference from the one before it.
//
// The derivation was checked against the art: the mutant is critter 0 and its
// walk is objart3 1880-1887, eight facings of the same eight-frame cycle; the
// cyborg is critter 21 and its death, objart2 1495, is four frames of it
// falling apart. The one critter with ViewCount 1 in the middle of the table
// (14/1/4) is why the multi-view blocks are not a flat eight per critter, and
// finding its single resource exactly where the arithmetic says it is - 1984 -
// is what confirms all of this.

import (
	"fmt"
	"image"

	"github.com/inkyblackness/hacked/ss1/content/object"
	"github.com/inkyblackness/hacked/ss1/resource"
)

const (
	// critterClass is which object class the walking things are.
	critterClass = object.Class(14)

	// singleViewBase is the first of the single-view posture resources, and
	// singleViewStride how far apart one posture's block is from the next.
	singleViewBase   = 1400
	singleViewStride = 37
	// standBase and walkBase are the two multi-view blocks: standing in
	// objart2, walking in objart3.
	standBase = 1585
	walkBase  = 1880
)

// The five single-view postures, in the order their blocks appear.
const (
	postureAttack = iota
	postureSecondaryAttack
	postureDeath
	posturePain
	postureNotice
	singleViewPostureCount
)

// Where in a critter's generic properties the fields this importer reads are.
// The layout is in ss-specs; only the ones that reach the game are named.
//
// The primary attack is a 21-byte block starting at 0x01, and these are its own
// offsets folded into the outer one - the damage it does, how close it has to
// be, how long it waits, and the thing it throws if it throws one.
const (
	critterAttackDamage   = 0x05
	critterAttackRange    = 0x0D
	critterAttackReload   = 0x0E
	critterAttackProj     = 0x12
	critterAnimationSpeed = 0x3B
	critterAttackSound    = 0x3C
	critterHurtSound      = 0x3E
	critterDeathSound     = 0x3F
	critterNoticeSound    = 0x40
	critterViewCount      = 0x45
	critterGenericSize    = 0x4B
)

// critter is one walking thing: where it is in the property table, how many
// facings it was drawn from, and where its own art starts.
type critter struct {
	triple    object.Triple
	index     int // position among the critters, which is what indexes the art
	viewStart int // where this critter's own multi-view resources begin
	views     int
	health    int
	name      string
	generic   []byte
}

// critters walks the property table and works out, for every critter, where its
// art is. The running total is the whole trick: a critter's multi-view block
// starts after every earlier critter's, and those are not all the same size.
func (in *installer) critters() []critter {
	var out []critter
	index, viewStart := 0, 0
	in.properties.Iterate(func(triple object.Triple, prop *object.Properties) bool {
		if triple.Class != critterClass || len(prop.Generic) < critterGenericSize {
			return true
		}
		views := int(prop.Generic[critterViewCount])
		if views < 1 {
			views = 1
		}
		out = append(out, critter{
			triple:    triple,
			index:     index,
			viewStart: viewStart,
			views:     views,
			health:    int(prop.Common.Hitpoints),
			name:      in.objectName(triple),
			generic:   prop.Generic,
		})
		index++
		viewStart += views
		return true
	})
	return out
}

// objectName is what the game calls this object. The short names are what the
// player is shown; a few are blank, and the long name is the fallback.
func (in *installer) objectName(triple object.Triple) string {
	linear := in.linearIndex(triple)
	if linear < len(in.shortNames) && in.shortNames[linear] != "" {
		return in.shortNames[linear]
	}
	if linear < len(in.longNames) && in.longNames[linear] != "" {
		return in.longNames[linear]
	}
	return fmt.Sprintf("Object %d/%d/%d", triple.Class, triple.Subclass, triple.Type)
}

// linearIndex is where this object sits when every class, subclass and type is
// counted off in order - which is how the name lists are indexed.
func (in *installer) linearIndex(want object.Triple) int {
	linear, found := 0, -1
	in.properties.Iterate(func(triple object.Triple, prop *object.Properties) bool {
		if triple == want {
			found = linear
			return false
		}
		linear++
		return true
	})
	return found
}

// posture reads one of the five single-view animations.
func (in *installer) posture(c critter, which int) []image.Image {
	id := resource.ID(singleViewBase + singleViewStride*which + c.index)
	return in.resourceFrames("OBJART2.RES", id)
}

// viewRotation is where this engine's facing zero sits in System Shock's own
// run of eight.
//
// Both games number their views the same way round - the profiles wind in the
// same direction - but they do not start in the same place. This engine's
// facing zero is the front, the view you get standing in front of the thing
// (core.GetViewDirection); System Shock's run starts on the thing's own left
// and reaches the front six steps later. Read off the art: the humanoid mutant
// is a left profile in view 0, its back in view 2, a right profile in view 4
// and face-on in view 6, in objart3's walk block and objart2's stand block
// alike.
//
// Without this, everything imported out of System Shock showed you its side
// when it was walking at you.
const viewRotation = 6

// multiView reads one facing of one of the two multi-view animations, in this
// engine's order rather than the file's - see viewRotation. A critter drawn
// from only one side answers with that one for every facing, which is what the
// engine's single-direction animation already means.
func (in *installer) multiView(c critter, base int, facing int) []image.Image {
	view := (facing + viewRotation) % 8
	if view >= c.views {
		view = 0
	}
	file := "OBJART3.RES"
	if base == standBase {
		file = "OBJART2.RES"
	}
	return in.resourceFrames(file, resource.ID(base+c.viewStart+view))
}

// installCritters writes a folder per critter: the eight-facing walk and stand
// out of the multi-view blocks, and the death, flinch and attack out of the
// single-view ones.
func (in *installer) installCritters() (int, error) {
	written := 0
	for _, c := range in.critters() {
		grid := &sheet{}
		animations := map[string]*spriteAnim{}

		stand := in.facingRows(c, standBase, grid)
		walk := in.facingRows(c, walkBase, grid)
		if len(stand) == 0 && len(walk) == 0 {
			continue
		}
		if len(stand) == 0 {
			stand = firstFrameOf(walk)
		}
		if len(walk) == 0 {
			walk = stand
		}
		animations["idle"] = &spriteAnim{Frames: stand, FPS: critterFPS(c), Loop: true}
		animations["walk"] = &spriteAnim{Frames: walk, FPS: critterFPS(c), Loop: true}

		fallback := [][]uint16{{stand[0][0]}}
		add := func(name string, which int, fps float32) {
			frames := in.posture(c, which)
			if len(frames) == 0 {
				animations[name] = &spriteAnim{Frames: fallback, FPS: fps}
				return
			}
			animations[name] = &spriteAnim{Frames: [][]uint16{grid.addAll(frames)}, FPS: fps}
		}
		add("dying", postureDeath, critterFPS(c))
		add("hit", posturePain, critterFPS(c))
		add("prepare_fire", postureSecondaryAttack, critterFPS(c))
		add("fire", postureAttack, critterFPS(c))

		attack := critterAttack(c)
		kind := &spriteKind{
			Name:       c.name,
			Actor:      true,
			Hostile:    attack != nil,
			Attack:     attack,
			Health:     c.health,
			Animations: animations,
			Provenance: map[string]string{
				"source": "System Shock (1994)",
				"file":   "OBJART2.RES, OBJART3.RES",
				"actor":  fmt.Sprintf("%d/%d/%d", c.triple.Class, c.triple.Subclass, c.triple.Type),
			},
		}
		cues := map[string]int{
			"alert":  int(c.generic[critterNoticeSound]),
			"attack": int(c.generic[critterAttackSound]),
			"death":  int(c.generic[critterDeathSound]),
			"hurt":   int(c.generic[critterHurtSound]),
		}
		id := in.folderFor(c.name, c.triple)
		if err := in.writeKind(id, kind, grid, 8, cues); err != nil {
			return written, fmt.Errorf("%s: %w", id, err)
		}
		written++
	}
	return written, nil
}

// facingRows reads all eight facings of one multi-view animation into the grid
// and gives back the frame indices, one row a facing - which is exactly the
// shape a directional animation has in sprite.json.
//
// The frames of one animation are laid out facing by facing rather than
// interleaved, because a facing's frame count can differ from its neighbour's:
// the game reuses art between views, and three of the mutant's eight standing
// views have five frames where the other five have two.
func (in *installer) facingRows(c critter, base int, grid *sheet) [][]uint16 {
	rows := make([][]uint16, 0, 8)
	longest := 0
	perFacing := make([][]image.Image, 8)
	for facing := 0; facing < 8; facing++ {
		frames := in.multiView(c, base, facing)
		perFacing[facing] = frames
		if len(frames) > longest {
			longest = len(frames)
		}
	}
	if longest == 0 {
		return nil
	}
	for facing := 0; facing < 8; facing++ {
		frames := perFacing[facing]
		if len(frames) == 0 {
			frames = perFacing[0]
		}
		row := grid.addAll(frames)
		// Every facing of one animation has to run the same number of frames -
		// the engine times an animation off facing zero - so a short facing
		// holds its last frame rather than looping early.
		for len(row) < longest {
			row = append(row, row[len(row)-1])
		}
		rows = append(rows, row)
	}
	return rows
}

// firstFrameOf is an animation reduced to the pose it starts in, for a critter
// the game drew walking but never standing still.
func firstFrameOf(rows [][]uint16) [][]uint16 {
	out := make([][]uint16, len(rows))
	for i, row := range rows {
		out[i] = []uint16{row[0]}
	}
	return out
}

// spriteAttack is what this critter does when it gets to you, out of its own
// primary attack block. Nothing here is invented: the damage, the reach, the
// wait and whether it throws something are all the game's own numbers.
func critterAttack(c critter) *spriteAttack {
	g := c.generic
	damage := int(int16(uint16(g[critterAttackDamage]) | uint16(g[critterAttackDamage+1])<<8))
	if damage <= 0 {
		return nil
	}
	// System Shock's player has about two hundred and fifty hit points and this
	// engine's has a hundred, so the damage is on that scale rather than its
	// own - otherwise every one of these kills in two hits.
	damage = damage * playerHealthScale / shockPlayerHealth
	if damage < 1 {
		damage = 1
	}
	reach := float32(g[critterAttackRange])
	if reach < 2 {
		// A reach of one is a claw, and one world unit is inside the player's
		// own collider - close enough never to be reached.
		reach = 2
	}
	reload := int(uint16(g[critterAttackReload]) | uint16(g[critterAttackReload+1])<<8)
	interval := float64(reload) / reloadUnitsPerTic * ticSeconds
	if interval <= 0 {
		interval = 1
	}
	projectile := uint32(g[critterAttackProj]) | uint32(g[critterAttackProj+1])<<8 |
		uint32(g[critterAttackProj+2])<<16 | uint32(g[critterAttackProj+3])<<24
	return &spriteAttack{
		Damage:     damage,
		Range:      reach,
		Projectile: projectile != 0,
		Interval:   interval,
	}
}

// The two conversions above, named: what the source game's own numbers are on
// the scale of, and what this one's are.
const (
	shockPlayerHealth = 255
	playerHealthScale = 100
	reloadUnitsPerTic = 256.0
	ticSeconds        = 0.9
)

// critterFPS is the game's own animation speed, which it stores as the time one
// frame is held: 0xFF is about nine tenths of a second.
func critterFPS(c critter) float32 {
	speed := float32(c.generic[critterAnimationSpeed])
	if speed <= 0 {
		return 8
	}
	return 255 / (speed * 0.9)
}
