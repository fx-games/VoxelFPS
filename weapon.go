package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/audio"
	"github.com/memmaker/voxelfps/events"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxelfps/wep"
)

// Weapon is what the player holds. Everything about it, from the frames it
// shows to what a shot does, comes out of the weapons folder; nothing about any
// particular gun is written here.
type Weapon struct {
	Data   *wep.Weapon
	Actors map[string]*wep.Effect
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
	// shots counts what this weapon has fired, for DEBUG_WEAPON. A rate of
	// fire is only measurable against a clock, and this is the numerator.
	shots int
	// cursor runs the weapon's own animation and flash runs the overlay
	// A_GunFlash starts over it, at the same time.
	cursor *wep.Cursor
	flash  *wep.Cursor
	// zoom is how far this weapon's own scope magnifies the view, 0 or 1 for
	// one that does not. ZDoom keeps it as Weapon.FOVScale, on the weapon.
	zoom float32
	// owned says the player has picked this one up. The folder is the whole
	// library of a hundred and thirty four weapons, not the loadout: what a
	// player can hold is what they have walked over.
	owned bool
}

// NewWeapon wires a loaded weapon to where its pictures and sounds are. They
// are not read until the weapon is first held.
func NewWeapon(game *Game, data *wep.Weapon, actors map[string]*wep.Effect, atlasPath, soundsPath string) *Weapon {
	weapon := &Weapon{
		Data:       data,
		Actors:     actors,
		atlasPath:  atlasPath,
		soundsPath: soundsPath,
		game:       game,
		runtime:    wep.NewRuntime(data, rand.New(rand.NewSource(1))),
	}
	weapon.cursor = wep.NewCursor(&data.States, "Ready")
	game.shareInventory(weapon.runtime)
	return weapon
}

// shareInventory folds a freshly built weapon's inventory and caps into the
// player's one shared pool and repoints the runtime at it, so ammo two guns
// both spend - Doom's Clip, Shell, Cell, Rocket, and a migrated mod's own
// names for the same thing - is one count instead of each gun's own copy. See
// docs/adr/0003-single-inventory-map.md.
//
// A weapon-private counter, like a chambered-round count or an overload flag,
// is namespaced with the weapon's own name by the migration, so it never
// collides with another weapon's key - it lives in the same shared map without
// being shared by anything.
func (g *Game) shareInventory(runtime *wep.Runtime) {
	if g.playerInventory == nil {
		g.playerInventory = make(wep.Inventory)
		g.playerCaps = make(wep.Caps)
	}
	for item, amount := range runtime.Caps {
		g.playerCaps[item] = amount
	}
	for item, amount := range runtime.Inventory {
		// The most any weapon would start with, not the first weapon's figure.
		// A pool one gun spends is often declared by another that only tops it
		// up, and that one declares it empty: the Hellbound magnums carry
		// FDHellboundBullets as a reserve at zero, the Zaiger rifle fires them
		// and starts with a hundred, and first-wins by folder order handed the
		// Zaiger the magnums' zero. It could never fire, and it dry-fired for
		// ever because it has no reload to get out of that with.
		//
		// Safe because this only ever runs while a weapon is being built, and
		// every weapon is built before the player can spend anything - see the
		// only call site, in NewWeapon.
		if existing, already := g.playerInventory[item]; !already || amount > existing {
			g.playerInventory[item] = amount
		}
	}
	runtime.Inventory = g.playerInventory
	runtime.Caps = g.playerCaps
}

// load reads the weapon's atlas, once, and keeps it for the life of the game.
// Its sounds are not this function's business - see give.
func (w *Weapon) load() {
	if w.Atlas != nil {
		return
	}
	if debugWeapon {
		started := time.Now()
		defer func() { log.Printf("[weapon] %s loaded in %v", w.Data.ID, time.Since(started)) }()
	}
	w.Atlas = w.game.LoadPackedAtlasWithBatch(w.atlasPath)
	w.spriteIndex = make(map[string]int)
	for i := 0; i <= w.Atlas.LastValidIndex(); i++ {
		w.spriteIndex[w.Atlas.NameOfFrame(i)] = i
	}
}

// give hands the weapon to whoever is about to carry it - a pickup for the
// player today, however an NPC comes to one tomorrow - loading its sounds the
// first time and never again on its own account: they stay loaded, alongside
// every other weapon anyone is carrying, until the level that handed them out
// is gone. A second give is a weapon already owned and does nothing, so a
// call site never has to check first.
func (w *Weapon) give() {
	if w.owned {
		return
	}
	w.owned = true
	w.game.loadWeaponSounds(w)
}

func (w *Weapon) DisplayName() string { return w.Data.Tag }

// Equip puts the weapon in hand, running its raise animation when it has one.
func (w *Weapon) Equip() {
	w.load()
	w.endFlash()
	if !thrownWeapons[w.Data.ID] && w.cursor.Goto("Select") {
		w.cursor.StartRaise()
		return
	}
	w.cursor.Goto("Ready")
}

// thrownWeapons is what is thrown rather than held, by folder name, and so is
// never raised into view on a switch.
//
// A gun is brought up and kept up, and its Select is written for that: a raise
// loop and an animation of a hand lifting it into frame. A grenade is not held
// at all - it is in a pouch until the moment it is thrown, and the throw is in
// the firing states, which draw the whole of it. Running its Select anyway put
// a hand on screen for a third of a second on every switch and then took it
// away again, because the state the script idles in draws nothing.
//
// A list and not a rule, the same way squarePixelFrames below is. The rule
// that suggests itself - skip the raise for a weapon with nothing to fire -
// reads the wrong thing: it makes a gun fired dry pop into frame instead of
// coming up, and it puts the grenade's own hand back the moment one is picked
// up. Being thrown is a property of the weapon, not of what is in the pouch.
// Brutal Doom's frag grenade is the only one in the folder today; the two
// grenade launchers are guns and are held like guns.
var thrownWeapons = map[string]bool{
	"bd_frag_grenade": true,
}

// Unequip stops the overlay and the weapon's own hum, if it has one. Several
// of these guns hum while they are held, on a looping sound their script
// starts and never stops, so putting one away has to stop that - but only
// that: a grenade still in the air plays on its own origin, not the
// player's, and keeps going until its own sound ends on its own. Its sounds
// stay loaded - see give - so putting it away is not what silences it either.
func (w *Weapon) Unequip() {
	w.endFlash()
	if w.game != nil && w.game.audio != nil {
		w.game.audio.StopChannels(playerOrigin)
	}
}

// endFlash is dropFlash for the two places that drop the overlay outside
// Update, where there is nobody to hand an op back to and the game is right
// here to be told.
func (w *Weapon) endFlash() {
	for _, op := range w.dropFlash() {
		if w.game != nil {
			w.game.applyWeaponOp(w, op)
		}
	}
}

// unload gives back the weapon's pictures: out of the 2D pass, and out of the
// weapon, so nothing is left pointing at the texture and it can be collected.
// Half a gigabyte of atlas is what the whole folder comes to.
func (w *Weapon) unload() {
	if w.Atlas == nil {
		return
	}
	w.game.DropSpriteBatch(w.Atlas.GetSpriteBatch())
	w.Atlas = nil
	w.spriteIndex = nil
}

// AmmoLine is what the HUD says about how loaded the weapon is: what is in the
// magazine out of what the magazine holds, and the reserve behind it. Empty for
// a weapon that costs nothing to fire.
//
// A weapon with one pool and no reload - most of the folder - has no magazine
// to count separately, so its one pool is the whole line.
func (w *Weapon) AmmoLine() string {
	clip, reserve := w.Data.Ammo.Clip, w.Data.Ammo.Type
	if clip == "" {
		if reserve == "" {
			return ""
		}
		return w.pool(reserve)
	}
	if clip == reserve {
		reserve = w.Data.Ammo.Type2
	}
	if reserve == "" {
		return w.pool(clip)
	}
	return fmt.Sprintf("%s  +%d", w.pool(clip), w.runtime.Inventory[reserve])
}

// pool is how much of one item the weapon is carrying, out of how much it can.
func (w *Weapon) pool(item string) string {
	return fmt.Sprintf("%d/%d", w.runtime.Inventory[item], w.runtime.Caps[item])
}

// debugWeapon makes the weapon in hand say where its script is, once a tic.
// A weapon that goes invisible or stops firing is a weapon whose script has
// walked somewhere unexpected, and the only way to see which state that is, is
// to watch it move. DEBUG_WEAPON=1 in the environment.
var debugWeapon = os.Getenv("DEBUG_WEAPON") != ""

// Update steps the weapon and its overlay and returns what they asked the world
// to do, in the order it was asked.
func (w *Weapon) Update(input wep.Input, elapsed float64) []wep.Op {
	w.runtime.Input = input
	// The mod's own control script, which this engine does not run: the items
	// its weapons read in place of the player's buttons.
	w.runtime.SetButtons(input, elapsed)
	// The items that take themselves back: a script's own pulses.
	w.runtime.Tick(elapsed)
	if debugWeapon {
		log.Printf("[weapon] %-28s shots=%d %s", w.cursor.Describe(), w.shots, w.ammoSummary())
	}
	effects := w.cursor.Update(w.runtime, elapsed)
	if w.flash != nil {
		effects = append(effects, w.flash.Update(w.runtime, elapsed)...)
		if w.flash.Stopped() {
			effects = append(effects, w.dropFlash()...)
		}
	}
	return effects
}

// dropFlash ends the overlay and puts out the light that went with it.
//
// A_Light1 lights the room and A_Light0 puts it out, and every one of these
// mods writes the pair - but only along the overlay's own path. An overlay
// that ends any other way never reaches its A_Light0: switching weapons
// mid-flash left the light on, parented to the player, and the player walked
// the rest of the level carrying a muzzle flash. Firing again started a new
// overlay that did reach its A_Light0, which is why it looked like it fixed
// itself.
func (w *Weapon) dropFlash() []wep.Op {
	if w.flash == nil {
		return nil
	}
	w.flash = nil
	return []wep.Op{{Op: wep.OpFlashLight, Level: 0,
		Source: "the flash overlay ending"}}
}

// ammoSummary is what the weapon is carrying of everything it spends, in the
// order the folder wrote it, so a state that will not fire can be read next to
// the counter it is waiting on.
func (w *Weapon) ammoSummary() string {
	names := make([]string, 0, len(w.Data.Items))
	for name := range w.Data.Items {
		if w.runtime.Caps[name] > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		fmt.Fprintf(&out, "%s=%d/%d ", name, w.runtime.Inventory[name], w.runtime.Caps[name])
	}
	return out.String()
}

// StartFlash puts the overlay on the label A_GunFlash named.
func (w *Weapon) StartFlash(label string) {
	w.flash = wep.NewOverlayCursor(&w.Data.States, label)
}

// Draw paints the weapon and, over it, whatever the flash overlay is showing.
// The overlay's own position is a shift from the weapon's, so a muzzle flash
// rides the gun through a recoil without the script saying so.
func (w *Weapon) Draw(screenWidth, screenHeight int32, bobX, bobY float64) {
	w.drawCursor(w.cursor, screenWidth, screenHeight, bobX, bobY)
	if w.flash != nil {
		x, y := w.cursor.Offsets()
		w.drawCursor(w.flash, screenWidth, screenHeight, x+bobX, y+bobY)
	}
}

// weaponBob is how far the gun has swung from where it is held, in the same
// 320x200 space the rest of its position is in.
//
// Doom traces a flattened figure of eight: the sway across is a full circle and
// the dip is that same angle folded into half of one, so the gun goes down twice
// for every sweep across and never rises above where it is held. The amplitude
// is the player's speed squared and quartered, capped - which in practice means
// it is at the cap whenever they are moving at all and nothing the moment they
// stop, so a weapon reads as still or walking rather than as a dial.
func (g *Game) weaponBob() (x, y float64) {
	weapon := g.currentWeapon()
	if weapon == nil || g.player == nil || !weapon.cursor.Bobs() {
		return 0, 0
	}
	x, y = weaponBobAt(g.bobSpeed, g.bobTics)
	if debugWeapon {
		log.Printf("[bob] speed=%.1f units/tic x=%+.1f y=%+.1f", g.bobSpeed, x, y)
	}
	return x, y
}

// bobSpeedSmoothing is how fast bobSpeed eases toward the player's actual
// speed: small enough that one bad frame (see bobSpeed's own doc comment)
// barely moves it, large enough that a real stop or start still reads within
// a couple of frames rather than as a slow fade.
const bobSpeedSmoothing = 0.08

// updateBobSpeed sets bobSpeed's target for weaponBob to ease toward: the
// player's horizontal speed, in the same Doom-units-a-tic weaponBobAt has
// always taken. Reading physics velocity here, once a tick with elapsed on
// hand, rather than fresh at draw time, is what lets it be smoothed at all.
func (g *Game) updateBobSpeed(elapsed float64) {
	if g.player == nil {
		return
	}
	velocity := g.player.Physics().Velocity()
	rawSpeed := math.Hypot(float64(velocity.X()), float64(velocity.Z())) /
		worldUnitsPerDoomUnit / wep.TicRate
	alpha := elapsed / bobSpeedSmoothing
	if alpha > 1 {
		alpha = 1
	}
	g.bobSpeed += (rawSpeed - g.bobSpeed) * alpha
}

// weaponBobAt is the sway itself: how fast the player is going, in Doom units
// a tic, and how far into the level's clock they are, in tics.
func weaponBobAt(speed, tics float64) (x, y float64) {
	bob := math.Min(speed*speed/4, maxWeaponBob)
	angle := 2 * math.Pi * tics / weaponBobCycleTics
	return bob * math.Cos(angle), bob * math.Abs(math.Sin(angle))
}

// How the sway is shaped: sixty four tics to go round once, and sixteen of the
// 320x200 space's units at its widest, which are Doom's own numbers.
const (
	weaponBobCycleTics = 64.0
	maxWeaponBob       = 16.0
)

func (w *Weapon) drawCursor(cursor *wep.Cursor, screenWidth, screenHeight int32, shiftX, shiftY float64) {
	step := cursor.Step()
	sprite := cursor.Sprite()
	if step == nil || sprite == "" || w.Atlas == nil {
		return // a frame that shows nothing, which is most of a firing state
	}
	index, known := w.spriteIndex[sprite]
	if !known {
		return // loading rejects this, so only hand-edited data gets here
	}

	// GZDoom positions a weapon in a fixed 320x200 space and scales that to the
	// view, so a gun is the same size and in the same place on every screen. Only
	// the height matters: the horizontal scale is the same number divided by
	// Doom's pixel aspect, which is why a wider window shows more room either
	// side of the gun rather than a wider gun.
	// Where this layer is holding its sprite, plus the weapon's own vertical
	// adjustment - a mod's way of saying its art sits higher or lower than the
	// space assumes, and one GZDoom only applies with no status bar in the way,
	// which is every frame here.
	x, y := cursor.Offsets()
	x += shiftX
	y += shiftY + float64(w.Data.Render.YAdjust)

	// And this is the whole of it: the picture's own hotspot, the pair of
	// numbers a Doom patch carries in its header and a PNG in its grAb chunk,
	// is subtracted from the middle of that 320x200 space. It is not a pivot
	// fraction and it is not a delta against some other frame - it is where
	// the frame's top left corner goes, and every weapon sprite in every one
	// of these mods is authored against it. A vanilla weapon frame's hotspot
	// is (width/2 - 160, height - 168), which is what puts a gun in the
	// middle of the screen with its bottom edge on the bottom edge; a mod's
	// recoil frame moves it a few pixels off that, and that difference is the
	// animation.
	hotspotX, hotspotY := w.Atlas.Hotspot(index)
	if os.Getenv("HOTSPOT_ZERO") != "" {
		hotspotX, hotspotY = 0, 0
	}
	scale := psprScale(float32(screenHeight))
	if prefix, listed := squarePixelFrames[w.Data.ID]; listed && strings.HasPrefix(sprite, prefix) {
		scale = mgl32.Vec2{scale.Y(), scale.Y()}
	}
	position := psprCorner(float32(screenWidth), float32(screenHeight),
		float32(x), float32(y), hotspotX, hotspotY, scale)
	// The position is the top left of the frame's own full canvas, before the
	// packer trimmed the transparent edges off it, which is the corner the
	// hotspot is measured from.
	source, destination := w.Atlas.GetSourceAndDestRects(index, position, mgl32.Vec2{0, 0}, scale)
	if debugWeapon {
		log.Printf("[draw] %s hot=%d,%d xy=%.0f,%.0f pos=%v src=%v dst=%v", sprite, hotspotX, hotspotY, x, y, position, source, destination)
	}
	w.Atlas.GetSpriteBatch().DrawTextured(source, destination, mgl32.Vec4{1, 1, 1, 1})
}

// squarePixelFrames names art a mod drew in square pixels and never said so,
// by the weapon it belongs to and the sprite name it starts with.
//
// GZDoom has a property for this - `Weapon.WeaponScaleY`, which defaults to
// Doom's 1.2 - and the honest fix is for a mod to set it. The Blacktail 51
// does not: its scope overlay is a circle drawn in square pixels, so Doom's
// aspect makes an egg of it, and it makes the same egg in GZDoom. But its
// *gun* frames are drawn for Doom's pixels like everyone else's, so the
// property GZDoom offers is no use here - it is per weapon, and one number
// cannot be right for both halves of this one.
//
// So this is a list and not a rule. A rule was tried - anything wider than the
// 320-unit weapon space is an overlay - and it was wrong: the Sniper Rifle and
// the Hunting Rifle ship overlays wider than that which are drawn for Doom's
// pixels correctly, and the rule turned both of those into eggs to straighten
// this one. Name the art that is wrong; leave everything else alone.
var squarePixelFrames = map[string]string{
	"blacktail_51_connector": "SCOP",
}

// psprScale is how far one unit of that 320x200 space reaches on this screen.
// Only the height decides it: a wider window shows more room either side of the
// gun rather than a wider gun, which is why the horizontal is the same number
// divided by Doom's pixel aspect instead of anything to do with the width.
func psprScale(viewHeight float32) mgl32.Vec2 {
	scaleY := viewHeight / psprHeight
	return mgl32.Vec2{scaleY / psprPixelAspect, scaleY}
}

// psprCorner is where the top left of a weapon frame's own full canvas goes:
// the picture's hotspot subtracted from where the layer is holding it, in the
// middle of GZDoom's weapon space. This is the whole of GZDoom's own weapon
// placement, and everything else about drawing a weapon is either the scale
// above or which numbers to pass in here.
func psprCorner(viewWidth, viewHeight, x, y float32, hotspotX, hotspotY int, scale mgl32.Vec2) mgl32.Vec2 {
	return mgl32.Vec2{
		viewWidth/2 + (x-psprCenterX-float32(hotspotX))*scale.X(),
		viewHeight/2 + (y-psprCenterY-float32(hotspotY))*scale.Y(),
	}
}

// The 320x200 space GZDoom lays a weapon out in. A sprite's hotspot is
// measured from the top left of that space's middle column and horizon.
//
// Doom's pixels are 1.2 times taller than they are wide, and that number is
// not a guess - it is what falls out of GZDoom's own two lines in
// HUDSprite::GetWeaponRect:
//
//	scalex = baseScale.X * (320 / (240 * WidescreenRatio)) * (vw / 320)
//	scale  = baseScale.Y * (SCREENHEIGHT * vw) / (SCREENWIDTH * 240)
//
// WidescreenRatio is the view's own width over its height, so both collapse to
// viewHeight/240 times the sprite's base scale, and DPSprite's base scale is
// (1.0, 1.2). That is scaleX = viewHeight/240 and scaleY = viewHeight/200, and
// the ratio between them is 1.2 on every screen shape - a wider window shows
// more room either side of the gun rather than a wider gun.
const (
	psprWidth       = 320.0
	psprHeight      = 200.0
	psprCenterX     = 160.0
	psprCenterY     = 100.0
	psprPixelAspect = 1.2
)

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
		// With a batch of its own: the pickups on the floor are world sprites,
		// but the same pictures are the icons the HUD and the inventory draw,
		// and those go through the 2D pass.
		g.sharedAtlas = g.LoadPackedAtlasWithBatch(shared.Path)
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
	// What the player has always got: ZDoom's Player.StartItem, which in every
	// one of these mods is the fists and the pistol. Everything else is picked
	// up, and until something is, the player is empty handed.
	for _, weapon := range g.weapons {
		if weapon.Data.Starting {
			weapon.give()
		}
	}
	if starting := g.startingInHand(); starting >= 0 {
		g.selectWeapon(starting)
	}
	// The two capture flags are the exception, because what they are for is
	// looking at a weapon: -weapon hands over the one it names, and -cycle
	// walks the whole folder, which means holding it.
	if index, known := g.weaponByID[g.startWeapon]; known {
		g.weapons[index].give()
		g.selectWeapon(index)
	}
	if g.autoCycle > 0 {
		for _, weapon := range g.weapons {
			weapon.give()
		}
		if g.currentWeapon() == nil {
			g.selectWeapon(0)
		}
	}
	return nil
}

// startingInHand is which of the weapons the player starts with comes up
// first, or -1 when they start with none.
//
// The one that costs ammo, which is Doom's own answer: you begin with the
// fists and the pistol and it is the pistol you are holding. It matters more
// here than it does there, because several of these mods' bare hands draw
// nothing at all - Alien Vendetta's fist is an invisible ready frame - and
// coming up holding one reads exactly like coming up holding nothing.
func (g *Game) startingInHand() int {
	first := -1
	for index, weapon := range g.weapons {
		if !weapon.owned {
			continue
		}
		if weapon.Data.Ammo.Type != "" {
			return index
		}
		if first < 0 {
			first = index
		}
	}
	return first
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
	// same ZDoom sound at the same relative path get the same cue; two that
	// don't are kept apart by the weapon's own ID, which is the scope every
	// cue of its is loaded and played under - see cueKey in the audio package.
	//
	// A sound that will not decode is reported and the weapon keeps working;
	// what makes a weapon refuse to load is checked at load, and a broken cue
	// is not worth taking the game down for while it is being played.
	if err := g.audio.LoadCueTree(weapon.Data.ID, weapon.soundsPath); err != nil {
		log.Printf("[weapons] %s: %v", weapon.Data.ID, err)
	}
	g.audio.SoundsLoaded(weapon.Data.ID)
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
	// Round the folder once looking for the next one the player actually
	// carries. Empty handed there is nothing to cycle to and the loop ends
	// where it started.
	next := g.currentWeaponIndex
	for range g.weapons {
		next = (next + step + len(g.weapons)) % len(g.weapons)
		if g.weapons[next].owned {
			g.selectWeapon(next)
			return
		}
	}
}

// selectWeaponByKey puts the weapon bound to a number key in the player's
// hands. An unbound key does nothing.
func (g *Game) selectWeaponByKey(key int) {
	id, bound := g.weaponKeys[key]
	if !bound {
		return
	}
	if index, known := g.weaponByID[id]; known && g.weapons[index].owned {
		g.selectWeapon(index)
	}
}

// updateWeapon steps the weapon in hand and acts on what it asks for.
func (g *Game) updateWeapon(elapsed float64) {
	weapon := g.currentWeapon()
	if weapon == nil {
		return
	}
	// The sway's phase is the level's own clock, so it keeps turning while a
	// weapon is being put away and the next one comes up mid swing rather than
	// starting its own.
	g.bobTics += elapsed * wep.TicRate
	g.updateBobSpeed(elapsed)

	input := wep.Input{Trigger: g.triggerHeld, Alt: g.altHeld, Reload: g.reloadPressed}
	g.reloadPressed = false // a reload is a press, not a state

	for _, op := range weapon.Update(input, elapsed) {
		g.applyWeaponOp(weapon, op)
	}
	g.updateEffects(elapsed)
}

// applyWeaponOp is the whole of what a compiled weapon can ask the world for.
func (g *Game) applyWeaponOp(weapon *Weapon, op wep.Op) {
	switch op.Op {
	case wep.OpFireBullets:
		weapon.shots++
		g.fireBullets(weapon, op, weapon.runtime.Rand)
	case wep.OpRailAttack:
		// A beam rather than a spread: one ray straight down the line of fire,
		// and a bolt of light drawn along it as far as it got. See beam.go.
		origin, direction := g.aim()
		hit := g.fireHitscanWith(weapon, origin, direction, op)
		g.spawnBeam(g.beamMuzzle(direction), beamEnd(origin, direction, hit))
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
		g.setWeaponZoom(weapon, op)

	case wep.OpPlaySound:
		g.playWeaponSound(weapon, op)
	case wep.OpSpin:
		// The weapon's script walked a circle of frames that cost no time. It
		// keeps running - a gun that goes invisible is worse than one that
		// stutters - but somebody has to hear about it.
		g.warnSpinningWeapon(weapon, op)

	case wep.OpStopSound:
		if g.audio != nil {
			g.audio.StopChannel(playerOrigin, op.Channel)
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

// warnSpinningWeapon says so once per weapon, rather than once a frame for as
// long as the player holds a gun whose script cannot settle.
func (g *Game) warnSpinningWeapon(weapon *Weapon, op wep.Op) {
	if g.warnedSpinning == nil {
		g.warnedSpinning = make(map[string]bool)
	}
	if g.warnedSpinning[weapon.Data.ID] {
		return
	}
	g.warnedSpinning[weapon.Data.ID] = true
	log.Printf("[weapons] %s: %s, at %s", weapon.Data.ID, op.Source, weapon.cursor.Describe())
}

// selectWeaponByTag puts a weapon in hand by the name its script uses, which is
// the actor's name rather than the folder's. Nothing matching is a script
// asking for a weapon this folder does not have, which is not worth a fuss.
func (g *Game) selectWeaponByTag(name string) {
	if name == "" {
		return
	}
	for index, weapon := range g.weapons {
		if weapon.owned && (strings.EqualFold(weapon.Data.Provenance.Actor, name) ||
			strings.EqualFold(weapon.Data.Tag, name)) {
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
	if hit := g.rayCastIgnoringHolograms(origin, direction); hit.HitInfo.Hit {
		return hit.WorldPosition()
	}
	return origin.Add(direction.Mul(seekReach))
}

// seekReach is how far ahead a missile aims when the crosshair is on nothing.
const seekReach = 64.0

// spawnFrom puts an actor into the world where another one already is, which is
// how a rocket lays a smoke trail behind itself.
//
// The actor library it draws from is the one that spawned the parent, not
// whatever is in hand now - a grenade thrown before a weapon switch still
// spawns its own FragGrenadeExplosion when it lands, rather than looking for
// one in whatever the player is holding by then and finding nothing.
func (g *Game) spawnFrom(parent *WorldActor, op wep.Op) {
	weapon := g.weaponByScope(parent.soundScope)
	if weapon == nil {
		return
	}
	actor, known := weapon.Actors[lowerName(op.Actor)]
	if !known || actor.Stock || g.sharedAtlas == nil {
		return
	}
	// ponytail: the child appears where the parent is and drifts, rather than
	// inheriting its heading. A trail behind a rocket looks right either way,
	// and anything that needs the heading needs seeking first. The height the
	// call asked for is still honoured, because a script that lays its smoke
	// eight units under the rocket means it.
	position := parent.object.Transform().GetPosition().
		Add(mgl32.Vec3{0, float32(op.SpawnHeight * worldUnitsPerDoomUnit), 0})
	g.spawnActorAt(weapon, actor, op, position, "Spawn", parent.runtime).setMaster(parent)
}

// playSoundAt plays a cue from somewhere in the world, panned and quietened by
// where that is relative to the listener. scope is whose cue it is - see
// cueKey in the audio package.
func (g *Game) playSoundAt(op wep.Op, position mgl32.Vec3, origin int, scope string) {
	if g.audio == nil {
		return
	}
	pan, volume := g.listenerFor(position, op.Volume)
	g.audio.PlayCueAt(scope, op.Sound, audio.CueOptions{
		Volume: volume, Pitch: op.Pitch, Looping: op.Looping,
		Channel: op.Channel, Pan: pan, Origin: origin,
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

// explodeArgs reads A_Explode(damage, radius) back out of a compiled op.
// zdwep's lowerExplode fills Op.Args in call order - op.Damage is a
// different op's field, never this one's, so reading it instead of Args[0]
// silently priced every frag grenade's blast at zero. Radius comes back in
// world units, ready to compare against a position delta.
//
// DamageType "Avoid" is not a call argument; it is read off the exploding
// actor's own class (see lowerExplode) and zeroes the damage outright.
// Brutal Doom's GrenadeAlerter is the case this exists for: it fires a real
// A_Explode every fuse tick while a thrown grenade is still counting down,
// purely to give monsters a whiff of it, and every monster class in the mod
// answers "Avoid" with DamageFactor 0 - it is not meant to hurt or flinch
// anyone. Without this, a bystander took a real hit, flinch included, once a
// second for as long as the grenade was in the air, well before it exploded.
func explodeArgs(op wep.Op) (damage int, radius float32) {
	if strings.EqualFold(op.DamageType, "Avoid") {
		return 0, 0
	}
	if len(op.Args) > 0 {
		damage = int(op.Args[0])
	}
	if len(op.Args) > 1 {
		radius = float32(op.Args[1]) * worldUnitsPerDoomUnit
	}
	return damage, radius
}

// explodeAt is A_Explode on a world actor already mid its own Death sprite: the
// mod's own frames are the explosion, so this only does the blast's damage,
// rather than laying the engine's generic explosion particles over the top of
// a picture that is already drawing one.
func (g *Game) explodeAt(position mgl32.Vec3, op wep.Op) {
	damage, radius := explodeArgs(op)
	events.Publish(events.EventDamage, DamageEvent{Amount: damage, Where: position})
	g.radiusDamage(position, radius, damage)
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
		reach = op.Args[0] * worldUnitsPerDoomUnit
	}
	hit := g.rayCastIgnoringHolograms(origin, direction)
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
	hit := g.rayCastIgnoringHolograms(origin, direction)
	if !hit.HitInfo.Hit {
		return
	}
	position := hit.WorldPosition()
	g.createImpactEffects(nil, wep.Op{Puff: wep.PuffExplosion}, hit.HitObject, position,
		hit.HitInfo.Side.AsDirection().ToVec3())
	damage, radius := explodeArgs(op)
	events.Publish(events.EventDamage, DamageEvent{
		Target: hit.HitObject, Amount: damage, Where: position,
	})
	g.radiusDamage(position, radius, damage)
}

// setWeaponZoom narrows the view the way a scope does. A factor of one or less
// puts it back.
//
// A_ZoomFactor writes Weapon.FOVScale, and the view is worked out from the
// weapon in hand every frame - `desired *= abs(ReadyWeapon.FOVScale)` - so the
// zoom belongs to the weapon rather than to the camera. Kept on the camera, a
// scope stayed on after the player switched to something else and there was no
// way back to a normal view but to scope and unscope again.
func (g *Game) setWeaponZoom(weapon *Weapon, op wep.Op) {
	factor := float32(1)
	if len(op.Args) > 0 && op.Args[0] > 0 {
		factor = float32(op.Args[0])
	}
	weapon.zoom = factor
	g.applyWeaponZoom()
}

// changeBaseFOV widens or narrows the view the player plays at, keeping
// whatever the weapon in hand is doing to it.
func (g *Game) changeBaseFOV(degrees float32) {
	if g.baseFOV == 0 {
		g.baseFOV = g.Camera.GetFOV()
	}
	g.baseFOV += degrees
	g.applyWeaponZoom()
}

// applyWeaponZoom puts the view where the weapon in hand says it should be.
func (g *Game) applyWeaponZoom() {
	zoom := float32(1)
	if weapon := g.currentWeapon(); weapon != nil && weapon.zoom > 0 {
		zoom = weapon.zoom
	}
	if g.baseFOV == 0 {
		g.baseFOV = g.Camera.GetFOV()
	}
	// Divided from what the player plays at, never from what the camera is at:
	// read back from the camera, an unzoomed weapon coming into the hands of a
	// player who was scoped would adopt the scope's own field of view as the
	// one to go back to, and the view would never widen again.
	g.Camera.SetFOV(g.baseFOV / zoom)
}

// meleeReach is how far a swing reaches when the script does not say, in
// voxels. Doom's own default melee range is 64 units, which is one of these.
//
// How many of Doom's units make one voxel is worldUnitsPerDoomUnit, in
// world_actor.go, and there is only one answer: this file used to keep a
// second one saying 32, so a script asking for a reach of 80 units got two and
// a half voxels of it while everything a weapon spawned was placed by the
// other number.
const meleeReach = 1.0

// fireBullets casts one ray per pellet, spread the way the script asked. The
// weapon's own source of randomness is used, so a seeded run scatters the same
// way twice.
func (g *Game) fireBullets(weapon *Weapon, op wep.Op, random *rand.Rand) {
	origin, direction := g.aim()
	count := op.Count
	if count < 1 {
		count = 1
	}
	for i := 0; i < count; i++ {
		g.fireHitscanWith(weapon, origin, spreadDirection(random, direction, g.Camera.GetUp(), op.SpreadH, op.SpreadV), op)
	}
}

// spreadDirection scatters a pellet within the horizontal and vertical spread,
// which are half angles in degrees the way ZDoom writes them.
func spreadDirection(random *rand.Rand, direction, overhead mgl32.Vec3, spreadH, spreadV float64) mgl32.Vec3 {
	if spreadH == 0 && spreadV == 0 {
		return direction
	}
	right, up := spreadBasis(direction, overhead)
	yaw := (randomSpan(random)) * spreadH * math.Pi / 180
	pitch := (randomSpan(random)) * spreadV * math.Pi / 180
	scattered := direction.
		Add(right.Mul(float32(math.Tan(yaw)))).
		Add(up.Mul(float32(math.Tan(pitch))))
	return scattered.Normalize()
}

// spreadBasis gives two axes across the line of fire, the shooter's own way up
// deciding which is across and which is up: a shotgun's spread is wider one way
// than the other, so a gun held on its side scatters on its side too.
//
// Crossing with the reference collapses when the line of fire is along it -
// looking straight up, on a deck - which would scatter every pellet to NaN, so
// that case crosses with something else.
func spreadBasis(direction, overhead mgl32.Vec3) (right, up mgl32.Vec3) {
	reference := overhead
	if reference.Len() < 0.001 || math.Abs(float64(direction.Dot(reference.Normalize()))) > 0.999 {
		reference = mgl32.Vec3{0, 0, 1}
		if math.Abs(float64(direction.Z())) > 0.999 {
			reference = mgl32.Vec3{1, 0, 0}
		}
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

func (g *Game) playWeaponSound(weapon *Weapon, op wep.Op) {
	if g.audio == nil {
		return
	}
	// A weapon's own sounds come from where the listener is standing, so there
	// is nothing to pan. What a spawned actor plays goes through playSoundAt,
	// which does.
	g.audio.PlayCueAt(weapon.Data.ID, op.Sound, audio.CueOptions{
		Volume:  op.Volume,
		Pitch:   op.Pitch,
		Looping: op.Looping,
		Channel: op.Channel,
		Origin:  playerOrigin,
	})
}

// playerOrigin is whose sound channels the weapon in hand plays on. ZDoom
// channels belong to an origin, and everything heard from where the listener
// stands - the gun, its reload, its idle hum - is the player's.
const playerOrigin = 0

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
		// Only if the flash is what is lit. The tracked slot is shared with
		// projectiles, and an A_Light0 arriving while a plasma bolt is in the
		// air used to put the bolt out.
		if g.muzzleFlashLit {
			if debugWeapon {
				log.Printf("[light] muzzle flash off")
			}
			g.muzzleFlashLit = false
			g.DetachLight()
		}
		return
	}
	if debugWeapon && !g.muzzleFlashLit {
		log.Printf("[light] muzzle flash on")
	}
	g.muzzleFlashLit = true
	g.AttachLightTo(g.player.GameObject, muzzleFlashColor,
		muzzleFlashIntensity*float32(level), muzzleFlashReach)
}

// weaponsDir is where the engine looks for weapons, under the asset tree the
// game is started next to.
const weaponsDir = "assets/weapons"
