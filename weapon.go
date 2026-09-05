package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/events"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/wep"
)

// Weapon is what the player holds. Everything about it, from the frames it
// shows to what a shot does, comes out of the weapons folder; nothing about any
// particular gun is written here.
type Weapon struct {
	Data   *wep.Weapon
	Actors map[string]*wep.Actor
	Atlas  *glhf.TextureAtlasPacked

	// spriteIndex turns a step's frame name into its place in the atlas.
	spriteIndex map[string]int

	runtime *wep.Runtime
	// cursor runs the weapon's own animation and flash runs the overlay
	// A_GunFlash starts over it, at the same time.
	cursor *wep.Cursor
	flash  *wep.Cursor

	uiScale mgl32.Vec2
}

// NewWeapon wires a loaded weapon to its atlas.
func NewWeapon(data *wep.Weapon, actors map[string]*wep.Actor, atlas *glhf.TextureAtlasPacked, uiScale mgl32.Vec2) *Weapon {
	indices := make(map[string]int)
	for i := 0; i <= atlas.LastValidIndex(); i++ {
		indices[atlas.NameOfFrame(i)] = i
	}
	weapon := &Weapon{
		Data:        data,
		Actors:      actors,
		Atlas:       atlas,
		spriteIndex: indices,
		runtime:     wep.NewRuntime(data, rand.New(rand.NewSource(1))),
		uiScale:     uiScale,
	}
	weapon.cursor = wep.NewCursor(&data.States, "Ready")
	return weapon
}

func (w *Weapon) DisplayName() string { return w.Data.Tag }

// Equip puts the weapon in hand, running its raise animation when it has one.
func (w *Weapon) Equip() {
	w.flash = nil
	if w.cursor.Goto("Select") {
		w.cursor.StartRaise()
		return
	}
	w.cursor.Goto("Ready")
}

// Unequip stops the overlay; the weapon keeps its inventory, which is the
// player's and not the gun's.
func (w *Weapon) Unequip() { w.flash = nil }

// AmmoLine is what the HUD says about how loaded the weapon is, or empty for a
// weapon that costs nothing to fire.
func (w *Weapon) AmmoLine() string {
	if w.Data.Ammo.Type == "" {
		return ""
	}
	return fmt.Sprintf("%d/%d", w.runtime.Inventory[w.Data.Ammo.Type],
		w.runtime.Caps[w.Data.Ammo.Type])
}

// Update steps the weapon and its overlay and returns what they asked the world
// to do, in the order it was asked.
func (w *Weapon) Update(input wep.Input, elapsed float64) []wep.Op {
	w.runtime.Input = input
	effects := w.cursor.Update(w.runtime, elapsed)
	if w.flash != nil {
		effects = append(effects, w.flash.Update(w.runtime, elapsed)...)
		if w.flash.Stopped() {
			w.flash = nil
		}
	}
	return effects
}

// StartFlash puts the overlay on the label A_GunFlash named.
func (w *Weapon) StartFlash(label string) {
	w.flash = wep.NewCursor(&w.Data.States, label)
}

// Draw paints the weapon and, over it, whatever the flash overlay is showing.
func (w *Weapon) Draw(screenWidth, screenHeight int32) {
	w.drawCursor(w.cursor, screenWidth, screenHeight)
	if w.flash != nil {
		w.drawCursor(w.flash, screenWidth, screenHeight)
	}
}

func (w *Weapon) drawCursor(cursor *wep.Cursor, screenWidth, screenHeight int32) {
	step := cursor.Step()
	if step == nil || step.Sprite == "" {
		return // a frame that shows nothing, which is most of a firing state
	}
	index, known := w.spriteIndex[step.Sprite]
	if !known {
		return // loading rejects this, so only hand-edited data gets here
	}
	scale := mgl32.Vec2{
		w.uiScale.X() * float32(w.Data.Render.Scale[0]),
		w.uiScale.Y() * float32(w.Data.Render.Scale[1]),
	}
	// A weapon being swapped in or out is held part of the way off the bottom
	// of the screen, which is what makes the swap read as one.
	_, top, _, bottom := w.Atlas.GetUVInPx(index)
	swap := float32(cursor.Raised()) * float32(bottom-top) * scale.Y()
	position := mgl32.Vec2{
		float32(screenWidth)/2 + float32(step.OffsetX)*scale.X(),
		float32(screenHeight) + swap + float32(w.Data.Render.YAdjust+step.OffsetY)*scale.Y(),
	}
	source, destination := w.Atlas.GetSourceAndDestRects(index, position, mgl32.Vec2{0.5, 1}, scale)
	w.Atlas.GetSpriteBatch().DrawTextured(source, destination, mgl32.Vec4{1, 1, 1, 1})
}

// LoadWeapons reads the weapons folder and puts every weapon in it into the
// player's hands. A folder that does not load stops the game rather than
// quietly leaving a gun out.
func (g *Game) LoadWeapons(dir string) error {
	library, err := wep.Load(dir)
	if err != nil {
		return err
	}
	var soundProblems []string
	for _, data := range library.Ordered() {
		atlas := g.LoadPackedAtlasWithBatch(library.AtlasPath(data.ID))
		g.weapons = append(g.weapons, NewWeapon(data, library.Actors[data.ID], atlas, g.UIScale))
		if err := g.loadWeaponSounds(library, data); err != nil {
			soundProblems = append(soundProblems, err.Error())
		}
	}
	if len(soundProblems) > 0 {
		// Every problem, then refuse, the same way wep.Load reports the folder
		// itself. One weapon's unreadable sounds must not hide the next one's.
		return fmt.Errorf("weapons: %d sound problem(s)\n  %s",
			len(soundProblems), strings.Join(soundProblems, "\n  "))
	}
	g.weaponKeys = library.Keys
	g.weaponByID = make(map[string]int, len(g.weapons))
	for index, weapon := range g.weapons {
		g.weaponByID[weapon.Data.ID] = index
	}
	if len(g.weapons) > 0 {
		g.currentWeapon().Equip()
	}
	return nil
}

// loadWeaponSounds hands the weapon's sounds to the player under the names its
// ops play them by. The migration writes them in the layout the cue names
// imply, so this needs no mapping of its own.
func (g *Game) loadWeaponSounds(library *wep.Library, data *wep.Weapon) error {
	if len(data.Sounds) == 0 {
		return nil
	}
	if g.audio == nil {
		g.audio = audio.NewPlayer(audioSampleRate)
	}
	// Cue names are the names the ops play by, which the migration wrote the
	// folder to match, so no mapping is needed here. Two weapons that play the
	// same ZDoom sound share the cue, which is what sharing a name means.
	if err := g.audio.LoadCueTree(library.SoundsPath(data.ID)); err != nil {
		return fmt.Errorf("%s: %w", data.ID, err)
	}
	g.audio.SoundsLoaded()
	return nil
}

// audioSampleRate is what the speaker runs at.
const audioSampleRate = 44100

// selectWeaponByKey puts the weapon bound to a number key in the player's
// hands. An unbound key does nothing.
func (g *Game) selectWeaponByKey(key int) {
	id, bound := g.weaponKeys[key]
	if !bound {
		return
	}
	if index, known := g.weaponByID[id]; known {
		g.selectWeapon(index)
	}
}

// updateWeapon steps the weapon in hand and acts on what it asks for.
func (g *Game) updateWeapon(elapsed float64) {
	weapon := g.currentWeapon()
	if weapon == nil {
		return
	}
	input := wep.Input{Trigger: g.triggerHeld, Reload: g.reloadPressed}
	g.reloadPressed = false // a reload is a press, not a state

	for _, op := range weapon.Update(input, elapsed) {
		g.applyWeaponOp(weapon, op)
	}
}

// applyWeaponOp is the whole of what a compiled weapon can ask the world for.
func (g *Game) applyWeaponOp(weapon *Weapon, op wep.Op) {
	switch op.Op {
	case wep.OpFireBullets:
		g.fireBullets(op, weapon.runtime.Rand)
	case wep.OpGunFlash:
		weapon.StartFlash(op.To)
	case wep.OpFlashLight:
		g.setMuzzleFlash(op.Level)
	case wep.OpPlaySound:
		g.playWeaponSound(op)
	case wep.OpRecoil:
		g.Camera.ChangeAngles(0, float32(op.Delta))
	case wep.OpMakeNoise:
		// Nothing listens yet; there are no enemies to hear it. Compiled and
		// published so wiring AI up later needs no re-migration.
		events.Publish(events.EventWeaponNoise, op)
	case wep.OpSpawnActor, wep.OpFireProjectile:
		// ponytail: compiled and carried, not yet drawn. World sprites come
		// from a fixed grid atlas and a weapon's frames are packed, so putting
		// the mod's own smoke and casings in the air needs the sprite system to
		// take a packed atlas first.
	}
}

// fireBullets casts one ray per pellet, spread the way the script asked. The
// weapon's own source of randomness is used, so a seeded run scatters the same
// way twice.
func (g *Game) fireBullets(op wep.Op, random *rand.Rand) {
	origin, direction := g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
	count := op.Count
	if count < 1 {
		count = 1
	}
	for i := 0; i < count; i++ {
		g.fireHitscan(origin, spreadDirection(random, direction, op.SpreadH, op.SpreadV), op)
	}
}

// spreadDirection scatters a pellet within the horizontal and vertical spread,
// which are half angles in degrees the way ZDoom writes them.
func spreadDirection(random *rand.Rand, direction mgl32.Vec3, spreadH, spreadV float64) mgl32.Vec3 {
	if spreadH == 0 && spreadV == 0 {
		return direction
	}
	right, up := spreadBasis(direction)
	yaw := (randomSpan(random)) * spreadH * math.Pi / 180
	pitch := (randomSpan(random)) * spreadV * math.Pi / 180
	scattered := direction.
		Add(right.Mul(float32(math.Tan(yaw)))).
		Add(up.Mul(float32(math.Tan(pitch))))
	return scattered.Normalize()
}

// spreadBasis gives two axes across the line of fire. Crossing with world up
// collapses when the player is looking straight up or down, which would scatter
// every pellet to NaN, so that case crosses with something else.
func spreadBasis(direction mgl32.Vec3) (right, up mgl32.Vec3) {
	reference := mgl32.Vec3{0, 1, 0}
	if math.Abs(float64(direction.Y())) > 0.999 {
		reference = mgl32.Vec3{0, 0, 1}
	}
	right = direction.Cross(reference).Normalize()
	return right, right.Cross(direction).Normalize()
}

// randomSpan is a number from -1 to 1.
func randomSpan(random *rand.Rand) float64 {
	if random == nil {
		return rand.Float64()*2 - 1
	}
	return random.Float64()*2 - 1
}

func (g *Game) playWeaponSound(op wep.Op) {
	if g.audio == nil {
		return
	}
	// A weapon's own sounds come from where the listener is standing, so there
	// is nothing to pan. Attenuation is compiled and carried for the day a
	// spawned actor makes a noise somewhere else.
	g.audio.PlayCueAt(op.Sound, audio.CueOptions{
		Volume:  op.Volume,
		Pitch:   op.Pitch,
		Looping: op.Looping,
		Channel: op.Channel,
	})
}

// LoadPackedAtlasWithBatch reads a weapon's packed sprite atlas off disk and
// gives it a batch to draw through.
func (g *Game) LoadPackedAtlasWithBatch(path string) *glhf.TextureAtlasPacked {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	atlas := quads.ReadTextureAtlasPacked(path, file)
	g.SetupSpriteBatch(atlas)
	return atlas
}

// How the muzzle flash lights the room: A_Light1 and A_Light2 are the two
// levels Doom weapons ask for, and A_Light0 puts it out.
const (
	muzzleFlashIntensity = float32(14)
	muzzleFlashReach     = float32(7)
)

var muzzleFlashColor = mgl32.Vec3{1.0, 0.85, 0.55}

// setMuzzleFlash lights the room from the player for as long as the weapon's
// flash overlay says. This is A_Light0/1/2: not a sector light, but the brief
// world brightening that goes with a muzzle flash.
//
// ponytail: rides the single tracked light slot, which projectiles also use, so
// firing while a plasma bolt is in the air takes the light off the bolt. One
// slot each needs the light system to hand out more than one tracked slot.
func (g *Game) setMuzzleFlash(level int) {
	if level <= 0 || g.player == nil {
		g.DetachLight()
		return
	}
	g.AttachLightTo(g.player, muzzleFlashColor,
		muzzleFlashIntensity*float32(level), muzzleFlashReach)
}

// weaponsDir is where the engine looks for weapons: a folder of that name in
// the directory the game is started from.
const weaponsDir = "weapons"
