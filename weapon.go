package main

import (
	"fmt"
	"log"
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

	// atlasPath and soundsPath are where this weapon's pictures and sounds
	// are, loaded the first time it is drawn rather than at startup. A migrated
	// mod brings well over a hundred weapons, and uploading every atlas and
	// decoding every sound before the first frame took minutes.
	atlasPath  string
	soundsPath string
	game       *Game

	// spriteIndex turns a step's frame name into its place in the atlas.
	spriteIndex map[string]int

	runtime *wep.Runtime
	// cursor runs the weapon's own animation and flash runs the overlay
	// A_GunFlash starts over it, at the same time.
	cursor *wep.Cursor
	flash  *wep.Cursor

	uiScale mgl32.Vec2
}

// NewWeapon wires a loaded weapon to where its pictures and sounds are. They
// are not read until the weapon is first held.
func NewWeapon(game *Game, data *wep.Weapon, actors map[string]*wep.Actor, atlasPath, soundsPath string) *Weapon {
	weapon := &Weapon{
		Data:       data,
		Actors:     actors,
		atlasPath:  atlasPath,
		soundsPath: soundsPath,
		game:       game,
		runtime:    wep.NewRuntime(data, rand.New(rand.NewSource(1))),
		uiScale:    game.UIScale,
	}
	weapon.cursor = wep.NewCursor(&data.States, "Ready")
	return weapon
}

// load reads the weapon's atlas and sounds, once.
func (w *Weapon) load() {
	if w.Atlas != nil {
		return
	}
	w.Atlas = w.game.LoadPackedAtlasWithBatch(w.atlasPath)
	w.spriteIndex = make(map[string]int)
	for i := 0; i <= w.Atlas.LastValidIndex(); i++ {
		w.spriteIndex[w.Atlas.NameOfFrame(i)] = i
	}
	w.game.loadWeaponSounds(w)
}

func (w *Weapon) DisplayName() string { return w.Data.Tag }

// Equip puts the weapon in hand, running its raise animation when it has one.
func (w *Weapon) Equip() {
	w.load()
	w.flash = nil
	if w.cursor.Goto("Select") {
		w.cursor.StartRaise()
		return
	}
	w.cursor.Goto("Ready")
}

// Unequip stops the overlay and anything the weapon was still playing. Several
// of these guns hum while they are held, on a looping sound their script starts
// and never stops, so putting one away has to.
func (w *Weapon) Unequip() {
	w.flash = nil
	if w.game != nil && w.game.audio != nil {
		w.game.audio.StopChannels()
	}
}

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
	if step == nil || step.Sprite == "" || w.Atlas == nil {
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
	// Everything the weapons throw into the world is drawn from one atlas,
	// packed from the shared picture store. It is packed here rather than by
	// the migration because which weapons are installed is decided by which
	// folders are present, and kept between runs because that rarely changes.
	shared, err := wep.BuildSharedAtlas(dir)
	if err != nil {
		return fmt.Errorf("weapons: packing the shared pictures: %w", err)
	}
	if shared.Path != "" {
		g.sharedAtlas = g.LoadPackedAtlas(shared.Path)
		g.sharedSpriteIndex = sharedAtlasIndex(g.sharedAtlas)
		if shared.Rebuilt {
			log.Printf("[weapons] packed %d shared pictures into %s", shared.Frames, shared.Path)
		}
	}

	for _, data := range library.Ordered() {
		g.weapons = append(g.weapons, NewWeapon(g, data,
			library.Actors[data.ID], library.AtlasPath(data.ID), library.SoundsPath(data.ID)))
	}
	g.weaponKeys = library.Keys
	g.weaponByID = make(map[string]int, len(g.weapons))
	for index, weapon := range g.weapons {
		g.weaponByID[weapon.Data.ID] = index
	}
	if len(g.weapons) > 0 {
		if index, known := g.weaponByID[g.startWeapon]; known {
			g.currentWeaponIndex = index
		}
		g.currentWeapon().Equip()
	}
	return nil
}

// loadWeaponSounds hands the weapon's sounds to the player under the names its
// ops play them by. The migration writes them in the layout the cue names
// imply, so this needs no mapping of its own.
func (g *Game) loadWeaponSounds(weapon *Weapon) {
	if len(weapon.Data.Sounds) == 0 {
		return
	}
	if g.audio == nil {
		g.audio = audio.NewPlayer(audioSampleRate)
	}
	// Cue names are the names the ops play by, which the migration wrote the
	// folder to match, so no mapping is needed here. Two weapons that play the
	// same ZDoom sound share the cue, which is what sharing a name means.
	//
	// A sound that will not decode is reported and the weapon keeps working;
	// what makes a weapon refuse to load is checked at load, and a broken cue
	// is not worth taking the game down for while it is being played.
	if err := g.audio.LoadCueTree(weapon.soundsPath); err != nil {
		log.Printf("[weapons] %s: %v", weapon.Data.ID, err)
	}
	g.audio.SoundsLoaded()
}

// audioSampleRate is what the speaker runs at.
const audioSampleRate = 44100

// cycleWeapon moves to the next weapon in the folder, or the previous one. The
// key map only reaches nine, and a migrated mod brings a hundred, so this is
// how the rest are reachable at all.
func (g *Game) cycleWeapon(step int) {
	if len(g.weapons) < 2 {
		return
	}
	next := (g.currentWeaponIndex + step) % len(g.weapons)
	if next < 0 {
		next += len(g.weapons)
	}
	g.selectWeapon(next)
}

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
	g.updateWorldActors(elapsed)
}

// applyWeaponOp is the whole of what a compiled weapon can ask the world for.
func (g *Game) applyWeaponOp(weapon *Weapon, op wep.Op) {
	switch op.Op {
	case wep.OpFireBullets:
		g.fireBullets(op, weapon.runtime.Rand)
	case wep.OpRailAttack:
		// A beam rather than a spread: one ray straight down the line of fire.
		g.fireHitscan(g.aim())
	case wep.OpCustomPunch, wep.OpSaw:
		g.fireMelee(op)
	case wep.OpExplode:
		g.explode(op)

	case wep.OpGunFlash:
		weapon.StartFlash(op.To)
	case wep.OpFlashLight:
		g.setMuzzleFlash(op.Level)
	case wep.OpRecoil:
		g.Camera.ChangeAngles(0, float32(op.Delta))
	case wep.OpZoom:
		g.setWeaponZoom(op)

	case wep.OpPlaySound:
		g.playWeaponSound(op)
	case wep.OpStopSound:
		if g.audio != nil {
			g.audio.StopChannel(op.Channel)
		}
	case wep.OpPrint:
		if op.Text != "" {
			g.Toast(op.Text)
		}

	case wep.OpSelectWeapon:
		g.selectWeaponByTag(op.Text)

	case wep.OpMakeNoise:
		// Nothing listens yet; there are no enemies to hear it. Compiled and
		// published so wiring AI up later needs no re-migration.
		events.Publish(events.EventWeaponNoise, op)

	case wep.OpSpawnActor, wep.OpFireProjectile, wep.OpSpawnItem:
		g.spawnWorldActor(weapon, op)
	case wep.OpSpawnParticle:
		// A bare coloured dot rather than a sprite, and we have a particle
		// system of our own that does that.
		g.EmitParticlesByName(quads.ParticlesSmoke, g.muzzlePosition(), 1)

	case wep.OpMonsterAction, wep.OpRunScript, wep.OpSetCrosshair,
		wep.OpPlayerFlag, wep.OpQuake, wep.OpRadiusThrust, wep.OpRadiusGive,
		wep.OpBFGSpray, wep.OpChangeVelocity, wep.OpScaleVelocity,
		wep.OpSetSpeed, wep.OpThrust, wep.OpWarp, wep.OpSeekerMissile,
		wep.OpSetAlpha, wep.OpFadeTo, wep.OpSetScale, wep.OpDie,
		wep.OpRemove, wep.OpKillChildren, wep.OpGiveToTarget:
		// Carried, and waiting on the system that would act on it: monster AI,
		// the mod's own scripts, screen shake, and the world sprites that
		// spawned actors need before any of their movement means anything.
	}
}

// selectWeaponByTag puts a weapon in hand by the name its script uses, which is
// the actor's name rather than the folder's. Nothing matching is a script
// asking for a weapon this folder does not have, which is not worth a fuss.
func (g *Game) selectWeaponByTag(name string) {
	if name == "" {
		return
	}
	for index, weapon := range g.weapons {
		if strings.EqualFold(weapon.Data.Provenance.Actor, name) ||
			strings.EqualFold(weapon.Data.Tag, name) {
			g.selectWeapon(index)
			return
		}
	}
}

// muzzlePosition is roughly where a shot leaves the gun.
func (g *Game) muzzlePosition() mgl32.Vec3 {
	origin, direction := g.aim()
	return origin.Add(direction.Mul(spawnDistanceInFront))
}

// seekTarget is what a homing missile steers at. There are no monsters to lock
// onto, so it is whatever the player is looking at: the first thing the
// crosshair meets, or a point out along it when it meets nothing. A rocket that
// follows the crosshair is what homing looks like from behind the gun anyway.
func (g *Game) seekTarget() mgl32.Vec3 {
	origin, direction := g.aim()
	if hit := g.rayCast(origin, direction); hit.HitInfo.Hit {
		return hit.WorldPosition()
	}
	return origin.Add(direction.Mul(seekReach))
}

// seekReach is how far ahead a missile aims when the crosshair is on nothing.
const seekReach = 64.0

// spawnFrom puts an actor into the world where another one already is, which is
// how a rocket lays a smoke trail behind itself.
func (g *Game) spawnFrom(parent *WorldActor, op wep.Op) {
	weapon := g.currentWeapon()
	if weapon == nil {
		return
	}
	actor, known := weapon.Actors[lowerName(op.Actor)]
	if !known || actor.Stock || g.sharedAtlas == nil {
		return
	}
	// ponytail: the child appears where the parent is and drifts, rather than
	// inheriting its heading. A trail behind a rocket looks right either way,
	// and anything that needs the heading needs seeking first.
	position := parent.object.Transform().GetPosition()
	g.spawnActorAt(weapon, actor, op, position).setMaster(parent)
}

// playSoundAt plays a cue from somewhere in the world, panned and quietened by
// where that is relative to the listener.
func (g *Game) playSoundAt(op wep.Op, position mgl32.Vec3) {
	if g.audio == nil {
		return
	}
	pan, volume := g.listenerFor(position, op.Volume)
	g.audio.PlayCueAt(op.Sound, audio.CueOptions{
		Volume: volume, Pitch: op.Pitch, Looping: op.Looping,
		Channel: op.Channel, Pan: pan,
	})
}

// listenerFor works out how a sound at a place reaches the player: which side
// it is on, and how much of it is left by the time it arrives.
func (g *Game) listenerFor(position mgl32.Vec3, volume float64) (pan, scaled float64) {
	if volume <= 0 {
		volume = 1
	}
	toSource := position.Sub(g.Camera.GetPosition())
	distance := float64(toSource.Len())
	if distance < 0.01 {
		return 0, volume
	}
	// Falls off with distance, and never quite to nothing, so a shot across a
	// room is quiet rather than absent.
	scaled = volume / (1 + distance/soundHalfDistance)

	right := g.Camera.GetForward().Cross(mgl32.Vec3{0, 1, 0}).Normalize()
	return float64(right.Dot(toSource.Normalize())), scaled
}

// soundHalfDistance is how far away a sound has to be to lose half its volume.
const soundHalfDistance = 8.0

// explodeAt marks a blast at a place rather than where the player is looking.
func (g *Game) explodeAt(position mgl32.Vec3, op wep.Op) {
	g.createImpactEffects(wep.PuffExplosion, nil, position, mgl32.Vec3{0, 1, 0})
	events.Publish(events.EventDamage, DamageEvent{Amount: op.Damage, Where: position})
}

// aim is the ray from the middle of the screen.
func (g *Game) aim() (origin, direction mgl32.Vec3) {
	return g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
}

// fireMelee is a swing: one ray, but only as far as an arm reaches.
func (g *Game) fireMelee(op wep.Op) {
	origin, direction := g.aim()
	reach := float64(meleeReach)
	if len(op.Args) > 0 && op.Args[0] > 0 {
		reach = op.Args[0] / worldUnitsPerVoxel
	}
	hit := g.rayCast(origin, direction)
	if !hit.HitInfo.Hit || hit.HitInfo.Distance > reach {
		return
	}
	events.Publish(events.EventHitScanCollision, HitscanEffect{
		Hit: hit, Direction: direction, Shot: op,
	})
}

// explode marks a blast where the player is looking. There is nothing with
// health to hurt, so what lands is the mark and the event.
func (g *Game) explode(op wep.Op) {
	origin, direction := g.aim()
	hit := g.rayCast(origin, direction)
	if !hit.HitInfo.Hit {
		return
	}
	position := hit.WorldPosition()
	g.createImpactEffects(wep.PuffExplosion, hit.HitObject, position,
		hit.HitInfo.Side.AsDirection().ToVec3())
	events.Publish(events.EventDamage, DamageEvent{
		Target: hit.HitObject, Amount: op.Damage, Where: position,
	})
}

// setWeaponZoom narrows the view the way a scope does. A factor of one or less
// puts it back.
func (g *Game) setWeaponZoom(op wep.Op) {
	factor := float32(1)
	if len(op.Args) > 0 && op.Args[0] > 0 {
		factor = float32(op.Args[0])
	}
	if g.baseFOV == 0 {
		g.baseFOV = g.Camera.GetFOV()
	}
	g.Camera.SetFOV(g.baseFOV / factor)
}

// How far a melee swing reaches when the script does not say, and how many of
// ZDoom's units make one of our voxels. Doom's world is 32 units to a wall
// panel, and ours is one.
const (
	meleeReach         = 1.6
	worldUnitsPerVoxel = 32.0
)

// fireBullets casts one ray per pellet, spread the way the script asked. The
// weapon's own source of randomness is used, so a seeded run scatters the same
// way twice.
func (g *Game) fireBullets(op wep.Op, random *rand.Rand) {
	origin, direction := g.aim()
	count := op.Count
	if count < 1 {
		count = 1
	}
	for i := 0; i < count; i++ {
		g.fireHitscanWith(origin, spreadDirection(random, direction, op.SpreadH, op.SpreadV), op)
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
	// is nothing to pan. What a spawned actor plays goes through playSoundAt,
	// which does.
	g.audio.PlayCueAt(op.Sound, audio.CueOptions{
		Volume:  op.Volume,
		Pitch:   op.Pitch,
		Looping: op.Looping,
		Channel: op.Channel,
	})
}

// LoadPackedAtlas reads a packed sprite atlas off disk.
func (g *Game) LoadPackedAtlas(path string) *glhf.TextureAtlasPacked {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	return quads.ReadTextureAtlasPacked(path, file)
}

// LoadPackedAtlasWithBatch is the same, for an atlas drawn as part of the HUD,
// which needs a batch of its own.
func (g *Game) LoadPackedAtlasWithBatch(path string) *glhf.TextureAtlasPacked {
	atlas := g.LoadPackedAtlas(path)
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
