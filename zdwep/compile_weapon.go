package zdwep

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/memmaker/goray/wep"
)

// Result is everything one weapon needs, compiled.
type Result struct {
	Weapon *wep.Weapon
	Actors map[string]*wep.Actor
	// Sprites are the weapon's own frames, packed into its own atlas in first
	// use order so it comes out the same on every run.
	Sprites []string
	// ActorSprites are the frames of what it spawns. They go to the shared
	// picture store, because the same smoke belongs to sixty weapons.
	ActorSprites []string
	// Sounds are the cue names the weapon plays, as the scripts write them.
	Sounds []string
	// SourceScript is the DECORATE the weapon was compiled from, kept beside the
	// compiled weapon so a re-migration diff says whether upstream or the
	// compiler moved.
	SourceScript string
}

// CompileWeapon turns one actor and everything it reaches into compiled data.
// Strict refuses a weapon whose own jumps do not all land; without it a jump
// with nowhere to go ends that layer and is recorded, which is what migrating a
// whole mod wants, since a mod has weapons that reference states only their
// siblings define.
func CompileWeapon(idx *Index, policy Policy, actorName string) (*Result, error) {
	return compileWeapon(idx, policy, actorName, true)
}

// CompileWeaponLenient is CompileWeapon without the strict jump check.
func CompileWeaponLenient(idx *Index, policy Policy, actorName string) (*Result, error) {
	return compileWeapon(idx, policy, actorName, false)
}

func compileWeapon(idx *Index, policy Policy, actorName string, strict bool) (*Result, error) {
	actor, ok := idx.Actor(actorName)
	if !ok {
		return nil, fmt.Errorf("no actor named %q in this mod", actorName)
	}
	if !IsWeapon(idx, actorName) {
		return nil, fmt.Errorf(
			"%s is not a weapon: its chain is %s, which reaches none of %s.\n"+
				"If %s is a class this mod defines, the DECORATE lump does not include the "+
				"file holding it, and the whole mod has to be indexed for the chain to resolve.",
			actor.Name, strings.Join(chainNames(idx, actorName), " -> "),
			strings.Join(WeaponBaseClasses, ", "), UnresolvedRoot(idx, actorName))
	}

	c := newCompiler(idx, policy)
	c.strictLabels = strict
	c.actorName = actor.Name
	// AmmoType and AmmoType1 are the same property in DECORATE - a weapon
	// only ever writes one of the two spellings for its primary fire.
	c.ammoType, _ = c.mergedProperty(actor.Name, "weapon.ammotype1")
	if c.ammoType == "" {
		c.ammoType, _ = c.mergedProperty(actor.Name, "weapon.ammotype")
	}
	c.ammoType = strings.Trim(c.ammoType, `"`)
	if c.ammoType != "" {
		c.needItem(c.ammoType)
	}
	c.ammoType2, _ = c.mergedProperty(actor.Name, "weapon.ammotype2")
	c.ammoType2 = strings.Trim(c.ammoType2, `"`)
	if c.ammoType2 != "" {
		c.needItem(c.ammoType2)
	}

	states, err := c.lowerStates(c.mergedStates(actor.Name))
	if err != nil {
		return nil, fmt.Errorf("%s (%s):\n%s", actor.Name, actor.SourceFile, err)
	}

	weapon := &wep.Weapon{
		Format:     wep.Format,
		ID:         WeaponID(actor, idx),
		Tag:        c.tag(actor),
		Slot:       c.intProperty(actor.Name, "weapon.slotnumber"),
		Ammo:       wep.Ammo{Type: c.ammoType, Use: c.ammoUse(actor.Name), Type2: c.ammoType2, Use2: c.ammoUse2(actor.Name)},
		Render:     c.render(actor.Name, true),
		States:     states,
		Reloadable: hasLabel(states, "Reload"),
		Dropped:    c.dropped,
		Provenance: wep.Provenance{
			Actor:      actor.Name,
			SourceFile: actor.SourceFile,
			Tool:       wep.Format,
		},
	}

	result := &Result{Weapon: weapon, Actors: make(map[string]*wep.Actor)}

	// The reachable actors are walked to a fixed point: a spawned actor can
	// spawn another. Every edge got here through a policy entry, so the walk is
	// bounded by decisions rather than by a depth limit.
	pending := sortedNames(c.neededActors)
	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		if _, done := result.Actors[strings.ToLower(name)]; done {
			continue
		}
		if _, defined := idx.Actor(name); !defined {
			// A mod names things GZDoom provides and never defines: a plain
			// bullet puff, an exploding barrel. The op keeps the name and the
			// weapon records that the engine has to supply it.
			c.defaulted("stock actor " + name)
			result.Actors[strings.ToLower(name)] = &wep.Actor{
				Name:   name,
				Stock:  true,
				Render: wep.Render{Scale: [2]float64{1, 1}, Alpha: 1},
				States: wep.StateMachine{Labels: map[string]int{}},
			}
			continue
		}
		compiled, extra, err := compileActor(idx, policy, name, c)
		if err != nil {
			return nil, err
		}
		result.Actors[strings.ToLower(name)] = compiled
		pending = append(pending, extra...)
	}

	weapon.Ammo.Clip = clipOf(weapon.Ammo, states)
	weapon.Icon = c.weaponIcon(actor.Name)
	weapon.Starting = StartsWith(idx, actor.Name)
	weapon.AmmoPickup, weapon.AmmoIcon = c.ammoPickup()

	sort.Strings(c.defaultedNames)
	weapon.Defaulted = c.defaultedNames
	// Both spellings: ZDoom declares the flag on the Weapon class, so a mod may
	// write it scoped or bare and mean the same thing. Read only as
	// weapon.noautofire, the Blacktail - which writes +NOAUTOFIRE - fired and
	// re-scoped for as long as a button was held.
	weapon.NoAutoFire = actorHasFlag(idx, actorName, "weapon.noautofire") ||
		actorHasFlag(idx, actorName, "noautofire")
	weapon.Decal = decalOf(idx, actorName)
	weapon.Buttons = c.buttons()
	weapon.Items = c.items(weapon.States)
	result.Sprites = c.sprites
	result.ActorSprites = c.actorSprites
	result.Sounds = sortedNames(c.neededSounds)
	result.SourceScript = actor.SourceFile
	return result, nil
}

// compileActor compiles one spawned actor, sharing the weapon's sprite and
// sound collection so everything lands in the same folder.
func compileActor(idx *Index, policy Policy, name string, shared *Compiler) (*wep.Actor, []string, error) {
	actor, ok := idx.Actor(name)
	if !ok {
		return nil, nil, fmt.Errorf("actor %q is spawned but not defined in this mod", name)
	}
	inner := newCompiler(idx, policy)
	inner.actorName = actor.Name
	inner.sprites = shared.actorSprites
	inner.spriteSeen = shared.actorSpriteSeen

	states, err := inner.lowerStates(inner.mergedStates(actor.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("%s (%s):\n%s", actor.Name, actor.SourceFile, err)
	}

	// Read before the sounds are handed over: seeSoundOf asks for the file as
	// well as naming it, and asked for afterwards the plasma rifle's whole
	// firing sound was left behind in the mod.
	seeSound := inner.seeSoundOf(actor.Name)

	shared.actorSprites = inner.sprites
	for sound := range inner.neededSounds {
		shared.needSound(sound)
	}
	for item := range inner.neededItems {
		shared.needItem(item)
	}
	for _, dropped := range inner.dropped {
		shared.drop(dropped)
	}
	for _, name := range inner.defaultedNames {
		shared.defaulted(name)
	}

	compiled := &wep.Actor{
		Name:      actor.Name,
		Render:    inner.render(actor.Name, false),
		Speed:     inner.speedOf(actor.Name),
		States:    states,
		NoGravity: inner.actorFloats(actor.Name),
		Missile:   inner.actorIsMissile(actor.Name),
		Decal:     decalOf(idx, actor.Name),
		NoDecal:   inner.mergedFlag(actor.Name, "nodecal"),
		SeeSound:  seeSound,
	}
	return compiled, sortedNames(inner.neededActors), nil
}

// WeaponID is the folder name a weapon gets. The tag is what a player sees, so
// it makes a better name than the actor's internal one, and it is stable
// between runs.
//
// Tags are not unique: a mod with seven weapon sets has seven weapons tagged
// "Shotgun". The caller settles that, because only it knows what else it has
// already written.
func WeaponID(actor *Actor, idx *Index) string {
	name := actor.Name
	for _, candidate := range idx.Ancestry(actor.Name) {
		if tag, ok := candidate.Property("tag"); ok {
			name = strings.Trim(tag, `"`)
			break
		}
	}
	return SnakeName(name)
}

// SnakeName turns any name into something that can be a folder.
func SnakeName(name string) string {
	var out strings.Builder
	previousUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			previousUnderscore = false
		default:
			if !previousUnderscore && out.Len() > 0 {
				out.WriteByte('_')
				previousUnderscore = true
			}
		}
	}
	return strings.Trim(out.String(), "_")
}

func (c *Compiler) tag(actor *Actor) string {
	if tag, ok := c.mergedProperty(actor.Name, "tag"); ok {
		return strings.Trim(tag, `"`)
	}
	return actor.Name
}

func (c *Compiler) ammoUse(name string) int {
	if c.ammoType == "" {
		return 0
	}
	// AmmoUse1 0 is a real, deliberate value - some weapons manage their ammo
	// entirely by hand, through their own inventory Give/Take calls, and tell
	// the engine's own ammo check to stay out of it. Only a property neither
	// spelling declares falls back to the usual one round a shot.
	if value, ok := c.mergedProperty(name, "weapon.ammouse1"); ok {
		return atoiOr(value, 1)
	}
	if value, ok := c.mergedProperty(name, "weapon.ammouse"); ok {
		return atoiOr(value, 1)
	}
	return 1
}

func (c *Compiler) ammoUse2(name string) int {
	if c.ammoType2 == "" {
		return 0
	}
	if value, ok := c.mergedProperty(name, "weapon.ammouse2"); ok {
		return atoiOr(value, 1)
	}
	return 1
}

func atoiOr(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return n
}

func (c *Compiler) intProperty(name, property string) int {
	value, ok := c.mergedProperty(name, property)
	if !ok {
		return 0
	}
	number, err := strconv.Atoi(strings.TrimSpace(strings.Trim(value, `"`)))
	if err != nil {
		return 0
	}
	return number
}

func (c *Compiler) floatProperty(name, property string) float64 {
	value, ok := c.mergedProperty(name, property)
	if !ok {
		return 0
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(strings.Trim(value, `"`)), 64)
	if err != nil {
		return 0
	}
	return number
}

// render reads the drawing properties. ZDoom's Scale sets both axes and
// XScale/YScale override one, so they are read in that order.
func (c *Compiler) render(name string, isWeapon bool) wep.Render {
	render := wep.Render{Scale: [2]float64{1, 1}, Alpha: 1}
	if scale := c.floatProperty(name, "scale"); scale > 0 {
		render.Scale = [2]float64{scale, scale}
	}
	if x := c.floatProperty(name, "xscale"); x > 0 {
		render.Scale[0] = x
	}
	if y := c.floatProperty(name, "yscale"); y > 0 {
		render.Scale[1] = y
	}
	if alpha, ok := c.mergedProperty(name, "alpha"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(alpha), 64); err == nil {
			render.Alpha = value
		}
	}
	if style, ok := c.mergedProperty(name, "renderstyle"); ok {
		render.Style = strings.ToLower(strings.Trim(strings.TrimSpace(style), `"`))
	}
	if isWeapon {
		render.YAdjust = c.intProperty(name, "weapon.yadjust")
	}
	return render
}

// items reads the cap and starting amount of everything the weapon's script
// touches. The caps are what make an amount of zero in A_JumpIfInventory mean
// something, and they live on actors the weapon never names outside the call.
// powerupDuration is how long an item lasts once it is given, in tics.
// ZDoom's Powerup.Duration counts tics, or seconds when it is written
// negative, and an item that is not a Powerup at all lasts until something
// takes it.
func (c *Compiler) powerupDuration(name string) int {
	value, written := c.mergedProperty(name, "powerup.duration")
	if !written {
		return 0
	}
	tics := atoiOr(strings.TrimSpace(value), 0)
	if tics < 0 {
		seconds := -tics
		if seconds > math.MaxInt32/int(wep.TicRate) {
			return math.MaxInt32 // "for the rest of the level", written as a huge number
		}
		return seconds * int(wep.TicRate)
	}
	return tics
}

func (c *Compiler) items(states wep.StateMachine) map[string]wep.Item {
	if len(c.neededItems) == 0 {
		return nil
	}
	magazines := reloadedItems(states)
	items := make(map[string]wep.Item, len(c.neededItems))
	for _, name := range sortedNames(c.neededItems) {
		item := wep.Item{Max: stockAmmoMax[strings.ToLower(name)]}
		if item.Max == 0 {
			item.Max = 1
		}
		if actor, ok := c.idx.Actor(name); ok {
			if max := c.intProperty(actor.Name, "inventory.maxamount"); max > 0 {
				item.Max = max
			}
			// A magazine starts loaded. ZDoom has the player pick a weapon up
			// with ammo already in it, and this engine hands out every weapon
			// at once with nothing to pick up, so a magazine that started
			// empty is a weapon born empty. Brutal Doom's dual MP40 is the
			// clearest case: its right hand magazine is a third ammo pool the
			// weapon itself never names, so it stayed at zero and the right
			// gun never fired a shot.
			// A magazine has a capacity; a flag is one or nothing. The walk
			// below can wander from a reload into the kick and the taunt a
			// Brutal Doom weapon shares with it, and those hand out flags,
			// so a weapon that started with Kicking already set would kick
			// the moment it was drawn.
			if item.Max > 1 && magazines[strings.ToLower(name)] {
				item.Initial = item.Max
			}
			item.Duration = c.powerupDuration(actor.Name)
		}
		items[name] = item
	}
	return items
}

// stockAmmoMax is how much of Doom's own ammo a player may carry. These four
// classes are in doom2.wad's own code rather than in any mod, so a weapon that
// names one as its reserve - `Weapon.AmmoType2 "Clip"` - finds nothing in the
// index to read a maximum from and used to be given the fallback of one.
//
// One is not a small reserve, it is a broken weapon. The Dual 92s reload a
// round at a time out of Clip, so with a reserve of one they came back from an
// empty magazine with a single round, fired it, and went back to a reload that
// had nothing left to load: the guns stopped for good on the first magazine.
var stockAmmoMax = map[string]int{
	"clip": 200, "shell": 50, "rocketammo": 50, "cell": 300,
}

// mainLoopLabels are where a reload comes back to when it is done. The walk
// below stops at them: past one it is no longer reading the reload, it is
// reading the whole weapon, and every counter a script keeps would look like
// a magazine.
// Matched anywhere in the name, not only at the front: a mod writes DoKick and
// FinishExecution and BerserkerKick, and a prefix test reaches none of them.
var mainLoopLabels = []string{"ready", "fire", "althold", "select", "spawn",
	"noammo", "sprint", "kick", "punch", "taunt", "salute", "execut",
	"grenade", "special", "slide", "stomp"}

// reloadedItems is what a weapon's reload puts in, which is what a magazine
// is. Nothing in DECORATE says so directly: a magazine and a counter are both
// `: Ammo` with a maximum, and the shells chambered in a pump shotgun look
// exactly like the rounds in an MP40's clip until you notice that only one of
// them is what the reload fills.
//
// So this walks the states a reload reaches - falling through, and following
// the jumps it takes - and collects what they give. Brutal Doom's dual MP40
// hands its two magazines a round at a time in a loop; Final Doomer's pump
// shotgun chambers its shell in the firing states, and never appears here.
// ownedByStoppedLabel marks every step that sits under one of the labels a
// reload walk must not wander into, whichever way it is entered.
func ownedByStoppedLabel(states wep.StateMachine) []bool {
	owner := make([]string, len(states.Steps))
	starts := make(map[int]string, len(states.Labels))
	for label, index := range states.Labels {
		// An invented label names no region of its own; the region is whatever
		// the mod's own label called it.
		if !strings.HasPrefix(label, "@") {
			starts[index] = label
		}
	}
	current := ""
	for i := range owner {
		if name, ok := starts[i]; ok {
			current = name
		}
		owner[i] = current
	}

	stopped := make([]bool, len(states.Steps))
	for i, name := range owner {
		for _, word := range mainLoopLabels {
			if strings.Contains(name, word) {
				stopped[i] = true
				break
			}
		}
	}
	return stopped
}

func reloadedItems(states wep.StateMachine) map[string]bool {
	given := map[string]bool{}
	start, ok := states.Label("Reload")
	if !ok {
		return given
	}
	// Which named label each step belongs to. ZDoom's numeric jumps compile to
	// invented "@frame" labels part way inside a region, so checking only the
	// step a name starts at lets the walk in through the side door - which is
	// how it used to reach the kick, the taunt and the sprint, and come back
	// thinking a stamina meter was a magazine.
	stop := ownedByStoppedLabel(states)

	seen := make(map[int]bool, len(states.Steps))
	queue := []int{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		if at < 0 || at >= len(states.Steps) || seen[at] || (at != start && stop[at]) {
			continue
		}
		seen[at] = true

		step := states.Steps[at]
		leaves := false
		for _, op := range step.Ops {
			switch op.Op {
			case wep.OpGive:
				given[strings.ToLower(op.Item)] = true
			case wep.OpStop:
				leaves = true
			case wep.OpGoto:
				leaves = true
			}
			if op.To != "" {
				if index, found := states.Label(op.To); found {
					queue = append(queue, index)
				}
			}
		}
		if !leaves {
			queue = append(queue, at+1)
		}
	}
	return given
}

// actorHasFlag reads one of a DECORATE actor's flags, honouring the chain it
// inherits from and a subclass that turns an inherited flag back off.
// actorFloats reports whether this thing hangs where it is put instead of
// falling: +NOGRAVITY, or a missile class that carries the flag itself and is
// not in this mod to be read. A mod that means the exception writes it -
// Final Doomer's C4 charge is `Projectile` and then `-NOGRAVITY`, and it
// arcs.
func (c *Compiler) actorFloats(name string) bool {
	if value, written := c.mergedFlagSet(name, "nogravity"); written {
		return value
	}
	return c.inheritsAStockProjectile(name) || c.inheritsAStockFloater(name)
}

// seeSoundOf is the sound a missile makes when it is fired, which ZDoom plays
// from P_SpawnMissile and not from any state the actor writes. Inherited, like
// every other property, and asked for as an asset here because nothing else
// names it: the migration only copies the sounds it has seen, and the plasma
// rifle's fire sound was left behind in the mod because no A_PlaySound in the
// weapon or the ball ever mentioned it.
func (c *Compiler) seeSoundOf(name string) string {
	sound, ok := c.mergedProperty(name, "seesound")
	if !ok {
		return ""
	}
	sound = strings.Trim(strings.TrimSuffix(strings.TrimSpace(sound), ";"), `"`)
	if sound == "" || strings.EqualFold(sound, "none") {
		return ""
	}
	c.needSound(sound)
	return sound
}

// decalOf is the mark this actor leaves on a wall: ZDoom's Decal property,
// which is what P_LineAttack spawns a shot's decal from. It is inherited, and
// "None" or an empty name means the actor marks nothing - which is how a laser
// pointer fires a hitscan every tic without chipping the wall every tic.
//
// Nothing anywhere in the line means no generator either: ZDoom's own
// BulletPuff carries no Decal, and Doom's bullet chips come from the weapon,
// which declares `Decal "BulletChip"` itself.
func decalOf(idx *Index, name string) string {
	for _, actor := range idx.Ancestry(name) {
		value, written := actor.Property("decal")
		if !written {
			continue
		}
		value = strings.Trim(strings.TrimSuffix(strings.TrimSpace(value), ";"), `"`)
		if value == "" || strings.EqualFold(value, "none") {
			return ""
		}
		return value
	}
	return ""
}

func actorHasFlag(idx *Index, name, flag string) bool {
	for _, actor := range idx.Ancestry(name) {
		if set, written := actor.Flags[flag]; written {
			return set
		}
	}
	return false
}

// buttons is which of the items this weapon reads are really the player's
// buttons under another name. Only the ones it actually names: the map is what
// the engine has to keep for this weapon, and a weapon that never asks about
// the trigger does not need one kept for it.
func (c *Compiler) buttons() map[string]string {
	var buttons map[string]string
	for _, name := range sortedNames(c.neededItems) {
		input, ok := ControlItems[strings.ToLower(name)]
		if !ok {
			continue
		}
		if buttons == nil {
			buttons = map[string]string{}
		}
		buttons[name] = input
	}
	return buttons
}

func hasLabel(machine wep.StateMachine, name string) bool {
	_, ok := machine.Label(name)
	return ok
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// chainNames spells out an actor's parent chain, ending in the class the mod
// never defines.
func chainNames(idx *Index, name string) []string {
	names := []string{}
	for _, actor := range idx.Ancestry(name) {
		names = append(names, actor.Name)
	}
	if root := UnresolvedRoot(idx, name); root != "" {
		names = append(names, root)
	}
	return names
}

// UnresolvedRoot is the first class in an actor's chain that the mod does not
// define. For a weapon in a fully indexed mod that is an engine class such as
// Weapon; anywhere else it is the name of a file the DECORATE lump did not
// include, which is the usual reason a weapon is not recognised.
func UnresolvedRoot(idx *Index, name string) string {
	chain := idx.Ancestry(name)
	if len(chain) == 0 {
		return name
	}
	return chain[len(chain)-1].Parent
}

// WeaponBaseClasses are the classes a weapon's chain has to reach. They are all
// engine classes, defined by GZDoom rather than by any mod, so the chain ends at
// a name no DECORATE file provides.
//
// DoomWeapon is on the list because it is where Doom mods actually stop: Brutal
// Doom's weapons are `: BrutalWeapon`, and BrutalWeapon is `: DoomWeapon`, which
// GZDoom derives from Weapon. Checking for Weapon alone finds nothing in a mod
// like that.
var WeaponBaseClasses = []string{"Weapon", "DoomWeapon", "HereticWeapon", "StrifeWeapon"}

// IsWeapon reports whether an actor's chain reaches one of the engine's weapon
// classes.
func IsWeapon(idx *Index, name string) bool {
	for _, base := range WeaponBaseClasses {
		if idx.InheritsFrom(name, base) {
			return true
		}
	}
	return false
}

// notPlayerWeapons are actors that satisfy IsWeapon only because a mod's own
// menu-driven machinery is built the same way a weapon is - something the
// player "holds" while it runs its own states - rather than because they are
// something to fight with. Brutal Doom drives its vehicles, its meat-shield
// grab-and-throw, its classic-mode weapon swap and its play-as-monster mode
// this way, and none of the three belong in a list a player picks a gun from.
//
// Named rather than inferred: nothing about a DECORATE chain says "this is a
// HUD", so -all and -list skip these by name. A name passed explicitly on the
// command line still compiles, in case one is ever wanted on its own.
var notPlayerWeapons = map[string]bool{
	// Vehicle "guns": what the player holds while riding a bike, tank, mech
	// or helicopter around the map, not something carried and fired.
	"mechgun": true, "bikehud": true, "attackhelicoptergun": true,
	"artillerytankgun": true, "heavymachinegunhud": true, "tankgun": true,
	// Meat shields: grab an enemy corpse to block shots, then throw it. A
	// weapon slot in the DECORATE sense, a grab-and-throw prompt to a player.
	"evilmarineshield": true, "impshield": true, "nazishield": true,
	"sguyshield": true, "zmanshield": true, "zombiecivilianshield": true,
	"cguyshield": true,
	// Play-as-monster mode's own HUD.
	"d4baronhud": true, "d4revenanthud": true,
	// The options menu's classic-mode weapon swap and its utility actors:
	// a rebind of the same three weapons already migrated under their own
	// names, an ammo cheat, a level-repair tool.
	"classicweapon": true, "classicsmg": true, "classicfist": true,
	"classicpistol": true, "classicsaw": true,
	"ammodroper": true, "repairtool": true,
	// BrutalWeapon is what every real weapon inherits from, not a weapon of
	// its own; it has states because Weapon requires them, not because
	// anything gives it to a player.
	"brutalweapon": true,
}

// Weapons lists every actor in the mod that is a weapon, by name, sorted.
func Weapons(idx *Index) []string {
	var names []string
	for _, actor := range idx.Actors {
		if IsWeapon(idx, actor.Name) && !notPlayerWeapons[strings.ToLower(actor.Name)] {
			names = append(names, actor.Name)
		}
	}
	sort.Strings(names)
	return names
}

// weaponEntryLabels are where the engine's own input can send the cursor -
// grep wep/runtime.go and weapon.go for every hardcoded Goto/has target and
// this is the complete list. Nothing outside this set is ever entered except
// by a jump written into the compiled ops themselves.
var weaponEntryLabels = []string{"select", "ready", "fire", "hold", "altfire", "althold", "reload"}

// deadAnimationLabels are BDCE's own labels (DEFAULTWEAPON.txt, the
// BrutalWeapon every bd_ weapon inherits from) for the five animations this
// engine has no button for: a kick, a taunt, tossing a grenade, and executing
// a body on the ground (curbstomping one is the same feature, branched by
// victim type inside DoExecution). Meat-shield grab-and-throw is a monster
// mechanic (CguyShield.txt and siblings), not a weapon state, so there is
// nothing of it here to begin with.
//
// This is the actual DECORATE source's own label list, not a name guess, so
// it stays exact across every weapon that inherits it. Pruning is scoped to
// exactly these five, deliberately - other things this engine also never
// reaches (a sprint key, a dual-wield toggle, an aim-down-sights hold, a
// manual unload) are left alone, because nothing here asked for them and a
// button for one of those is not implausible later; a kick with no leg is.
var deadAnimationLabels = []string{
	"dokick", "berserkerkick", "airkick", "superairkick", "slideattack",
	"berserkslide", "finishslide", "finishslidehitwall", "executeairstomp", "doairstomp",
	"taunt", "tauntfemale", "tauntclassic", "tauntfemaleclassic", "salute",
	"tossgrenade",
	"doexecution", "bashwall", "execution1", "execution2", "finishexecution",
}

// reachableSteps is which of a weapon's own steps the engine can ever actually
// reach, starting from weaponEntryLabels and following every op's own jump.
//
// jump_if_inventory is the one edge not taken for free: an inventory flag
// nothing reachable ever gives cannot be true, so a branch on it cannot be
// taken. Settable is found by the same walk it gates, so the two are computed
// to a fixed point: a step already known reachable can make an item settable,
// which can make a jump_if_inventory reachable, which can reach a step that
// gives another item.
//
// Every other jump - ammo running out, a random roll, a world check, a
// countdown - depends on something this analysis cannot rule out (real
// gameplay state, real randomness), so its target is always taken. Being
// wrong there only keeps a step that turns out unreachable after all, which
// costs nothing; being wrong the other way deletes a frame the gun still
// needs.
func reachableSteps(weapon *wep.Weapon) []bool {
	machine := &weapon.States
	n := len(machine.Steps)
	reachable := make([]bool, n)
	settable := make(map[string]bool)
	for name, item := range weapon.Items {
		if item.Initial > 0 {
			settable[strings.ToLower(name)] = true
		}
	}

	for {
		visited := make([]bool, n)
		stack := make([]int, 0, n)
		for _, name := range weaponEntryLabels {
			if index, ok := machine.Labels[name]; ok && index >= 0 && index < n {
				stack = append(stack, index)
			}
		}
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if i < 0 || i >= n || visited[i] {
				continue
			}
			visited[i] = true
			terminates := false
			for _, op := range machine.Steps[i].Ops {
				if op.Op == wep.OpGive {
					settable[strings.ToLower(op.Item)] = true
				}
				if op.Op == wep.OpGoto {
					terminates = true
				}
				if op.Op == wep.OpJumpIfInventory {
					if !settable[strings.ToLower(op.Item)] {
						continue
					}
				}
				if op.To == "" {
					continue
				}
				if target, ok := machine.Labels[strings.ToLower(op.To)]; ok {
					stack = append(stack, target)
				}
			}
			if !terminates && i+1 < n {
				stack = append(stack, i+1)
			}
		}
		grew := false
		for i := 0; i < n; i++ {
			if visited[i] && !reachable[i] {
				reachable[i] = true
				grew = true
			}
		}
		if !grew {
			return reachable
		}
	}
}

// namedAnimationClosure is everything forward-reachable from
// deadAnimationLabels, taking every jump unconditionally (this is scoping the
// prune, not proving safety - reachableSteps already did that). It stops at
// weaponEntryLabels rather than walking through them: every one of these
// animations hands back to Ready or a sibling when it ends, and following
// that would claim the entire rest of the weapon's states belong to it.
func namedAnimationClosure(machine *wep.StateMachine) []bool {
	n := len(machine.Steps)
	isEntry := make(map[int]bool, len(weaponEntryLabels))
	for _, name := range weaponEntryLabels {
		if index, ok := machine.Labels[name]; ok {
			isEntry[index] = true
		}
	}

	visited := make([]bool, n)
	inClosure := make([]bool, n)
	stack := make([]int, 0, n)
	for _, name := range deadAnimationLabels {
		if index, ok := machine.Labels[name]; ok && index >= 0 && index < n {
			stack = append(stack, index)
		}
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if i < 0 || i >= n || visited[i] {
			continue
		}
		visited[i] = true
		if isEntry[i] {
			continue
		}
		inClosure[i] = true
		terminates := false
		for _, op := range machine.Steps[i].Ops {
			if op.Op == wep.OpGoto {
				terminates = true
			}
			if op.To == "" {
				continue
			}
			if target, ok := machine.Labels[strings.ToLower(op.To)]; ok {
				stack = append(stack, target)
			}
		}
		if !terminates && i+1 < n {
			stack = append(stack, i+1)
		}
	}
	return inClosure
}

// rescueStillNamedSteps clears remove on any step a surviving step's own
// (non jump_if_inventory) op still names, in place, to a fixed point.
// jump_if_inventory is excluded on purpose: that op is the one this whole
// prune runs on, and a kept Ready frame's own dead check on "Kicking" must
// never be the thing that saves DoKick.
func rescueStillNamedSteps(machine *wep.StateMachine, remove []bool) {
	for {
		changed := false
		for i, step := range machine.Steps {
			if remove[i] {
				continue
			}
			for _, op := range step.Ops {
				if op.Op == wep.OpJumpIfInventory || op.To == "" {
					continue
				}
				if target, ok := machine.Labels[strings.ToLower(op.To)]; ok && remove[target] {
					remove[target] = false
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

// PruneDeadStates drops the steps that are both unreachable (reachableSteps)
// and part of one of the five named animations (namedAnimationClosure) - the
// intersection, not either alone: a state nothing but a kick ever reaches is
// only worth deleting once it is also confirmed dead, and a state that
// happens to be dead for its own unrelated reason (no sprint key, say) is left
// alone because nobody asked for that one.
//
// A jump_if_inventory still aimed at a label this deletes is stripped from its
// step - it already never fires, since its item is exactly what proved the
// target unreachable, but the label it names is gone and the load-time check
// in wep/load.go rejects a jump with nowhere to go, which is right for a
// stale reference and would otherwise be right about these too.
//
// Everything else that still names a step marked for removal rescues it
// instead: BDCE's own "GoingToReady" utility label, for one, is where a kick
// or a taunt ends up on its way back to Ready, but it is also where several
// other, unrelated (and here also dead, but out of scope) frames end up on
// theirs. One animation being in scope for this prune does not make a label
// three others still use fair game. The rescue runs to a fixed point, since
// saving a step can mean saving whatever its own ops name in turn.
func (r *Result) PruneDeadStates() (droppedSteps int) {
	machine := &r.Weapon.States
	reachable := reachableSteps(r.Weapon)
	prunable := namedAnimationClosure(machine)
	remove := make([]bool, len(machine.Steps))
	for i := range machine.Steps {
		remove[i] = prunable[i] && !reachable[i]
	}
	rescueStillNamedSteps(machine, remove)

	newIndex := make([]int, len(machine.Steps))
	newSteps := make([]wep.Step, 0, len(machine.Steps))
	for i, step := range machine.Steps {
		if remove[i] {
			newIndex[i] = -1
			continue
		}
		step.Ops = withoutDeadJumps(step.Ops, machine, remove)
		newIndex[i] = len(newSteps)
		newSteps = append(newSteps, step)
	}
	droppedSteps = len(machine.Steps) - len(newSteps)
	if droppedSteps == 0 {
		return 0
	}

	newLabels := make(map[string]int, len(machine.Labels))
	for name, index := range machine.Labels {
		if index >= 0 && index < len(newIndex) && newIndex[index] >= 0 {
			newLabels[name] = newIndex[index]
		}
		// A negative or out-of-range index (the engine's own stop marker) and
		// a label whose step got dropped both simply do not carry over.
	}
	machine.Steps = newSteps
	machine.Labels = newLabels
	r.Sprites = spritesUsedBy(newSteps)
	return droppedSteps
}

// withoutDeadJumps drops an op whose only reason to exist was reaching a step
// PruneDeadStates just removed - a jump_if_inventory on the item that proved
// it unreachable. Everything else survives untouched.
func withoutDeadJumps(ops []wep.Op, machine *wep.StateMachine, remove []bool) []wep.Op {
	kept := ops[:0]
	for _, op := range ops {
		if op.Op == wep.OpJumpIfInventory && op.To != "" {
			if target, ok := machine.Labels[strings.ToLower(op.To)]; ok && target >= 0 && target < len(remove) && remove[target] {
				continue
			}
		}
		kept = append(kept, op)
	}
	return kept
}

// spritesUsedBy is DropSprites and useSprite's own rule for what counts as a
// picture rather than a keep marker, applied after the fact: first use order,
// each name once, so a re-migration with nothing pruned writes back the same
// list it already had.
func spritesUsedBy(steps []wep.Step) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name == "" || seen[name] || wep.IsKeepMarker(name) {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, step := range steps {
		add(step.Sprite)
		for _, alt := range step.SpriteAlt {
			add(alt)
		}
	}
	return names
}

// DropSprites blanks the frames whose pictures the mod does not have, so a
// weapon that is otherwise complete can still be migrated. The frames stay in
// the animation with their timing intact and show nothing, which is what a
// missing sprite looks like, and every one is recorded on the weapon.
func (r *Result) DropSprites(missing []string) {
	if len(missing) == 0 {
		return
	}
	gone := make(map[string]bool, len(missing))
	for _, name := range missing {
		gone[name] = true
		r.Weapon.Defaulted = append(r.Weapon.Defaulted, "missing sprite "+name)
	}
	sort.Strings(r.Weapon.Defaulted)

	blank := func(machine *wep.StateMachine) {
		for i := range machine.Steps {
			step := &machine.Steps[i]
			if gone[step.Sprite] {
				step.Sprite = ""
			}
			// A keep marker with none of its alternatives left shows nothing,
			// which is what a missing picture looks like everywhere else here.
			if len(step.SpriteAlt) > 0 {
				step.SpriteAlt = without(step.SpriteAlt, gone)
				if len(step.SpriteAlt) == 0 {
					step.Sprite = ""
				}
			}
		}
	}
	blank(&r.Weapon.States)
	for _, actor := range r.Actors {
		blank(&actor.States)
	}
	// A pickup picture the sources do not have is no pickup picture. Left
	// named, it would fail the load check on a store that never got the file.
	if gone[r.Weapon.Icon] {
		r.Weapon.Icon = ""
	}
	if gone[r.Weapon.AmmoIcon] {
		r.Weapon.AmmoIcon = ""
	}
	r.Sprites = without(r.Sprites, gone)
	r.ActorSprites = without(r.ActorSprites, gone)
}

func without(names []string, gone map[string]bool) []string {
	kept := names[:0]
	for _, name := range names {
		if !gone[name] {
			kept = append(kept, name)
		}
	}
	return kept
}

// DropSounds does for sounds what DropSprites does for pictures.
func (r *Result) DropSounds(missing []string) {
	if len(missing) == 0 {
		return
	}
	gone := make(map[string]bool, len(missing))
	for _, name := range missing {
		gone[name] = true
		r.Weapon.Defaulted = append(r.Weapon.Defaulted, "missing sound "+name)
	}
	sort.Strings(r.Weapon.Defaulted)

	silence := func(machine *wep.StateMachine) {
		for i := range machine.Steps {
			for j := range machine.Steps[i].Ops {
				if gone[machine.Steps[i].Ops[j].Sound] {
					machine.Steps[i].Ops[j].Sound = ""
				}
			}
		}
	}
	silence(&r.Weapon.States)
	for _, actor := range r.Actors {
		if gone[actor.SeeSound] {
			actor.SeeSound = ""
		}
		silence(&actor.States)
	}

	kept := r.Sounds[:0]
	for _, name := range r.Sounds {
		if !gone[name] {
			kept = append(kept, name)
		}
	}
	r.Sounds = kept
}

// AllActorNames lists every actor in the mod, sorted.
func AllActorNames(idx *Index) []string {
	names := make([]string, 0, len(idx.Actors))
	for _, actor := range idx.Actors {
		names = append(names, actor.Name)
	}
	sort.Strings(names)
	return names
}

// ScanActor lowers one actor's states purely to find out what it needs, without
// building anything. Running it over a whole mod hands back the complete
// vocabulary in one pass, instead of discovering it one weapon at a time.
func ScanActor(idx *Index, policy Policy, name string) error {
	actor, ok := idx.Actor(name)
	if !ok {
		return fmt.Errorf("no actor named %q", name)
	}
	c := newCompiler(idx, policy)
	c.actorName = actor.Name
	_, err := c.lowerStates(c.mergedStates(actor.Name))
	return err
}

// weaponIcon is the picture of this weapon lying on the floor. Where it goes is
// the shared picture store, not the weapon's own atlas: a pickup, an inventory
// list and the HUD all draw weapons that are not in anybody's hands, and only
// the weapon in hand has its atlas loaded.
//
// GZDoom has two answers and a mod uses whichever suits it, so both are read,
// nearest first:
//
//   - **The Spawn state**, which is what the engine draws where a weapon has
//     been dropped. Brutal Doom, Doom itself and every single-weapon mod here
//     answer this way.
//   - **Inventory.Icon**, the name of a picture for the status bar. Final
//     Doomer answers this way and nothing else: its weapons have no Spawn
//     state at all, because they are never dropped - a separate
//     `replaces BFG9000` CustomInventory is what lies on the floor and it hands
//     out whichever of the nine weapon sets you are playing. What is left that
//     belongs to the weapon itself is its icon, which the mod builds in
//     TEXTURES out of that weapon's own pickup sprite at half size.
//
// A weapon with neither is one the mod ships no picture of but the frame it is
// held in. That is the last fallback and it is recorded as one rather than
// passed off as pickup art.
func (c *Compiler) weaponIcon(actorName string) string {
	if name := c.needShared(c.stateSprite(actorName, "spawn")); name != "" {
		return name
	}
	if name := c.needShared(c.iconSprite(actorName)); name != "" {
		return name
	}
	name := c.needShared(c.stateSprite(actorName, "ready"))
	if name != "" {
		c.defaulted("no pickup sprite, using the first-person frame " + name)
	}
	return name
}

// iconSprite is the picture Inventory.Icon names, as a sprite this migration
// can find. The property holds a *texture* name rather than a lump name, so it
// goes through the mod's TEXTURES definitions first - `PLCHAING` is a texture
// whose one patch is the sprite `PCHGZ0` - and a name that is already a lump
// is used as it stands, which is how a mod that points its icon straight at a
// sprite is read.
//
// An empty icon and TNT1A0, the invisible sprite, both mean the mod is saying
// this thing has no icon. Final Doomer's own weapon base class writes the
// empty one, and reading it as a name would give every weapon in the mod the
// same blank picture.
func (c *Compiler) iconSprite(actorName string) string {
	value, written := c.mergedProperty(actorName, "inventory.icon")
	if !written {
		return ""
	}
	name := strings.ToUpper(strings.Trim(strings.TrimSuffix(strings.TrimSpace(value), ";"), `"`))
	if patch, defined := c.idx.Textures[name]; defined {
		name = patch
	}
	if name == "" || name == "TNT1A0" {
		return ""
	}
	return name
}

// clipOf is which of a weapon's two ammo pools is the magazine: the one a
// reload fills. Only a pool the weapon actually spends counts - a dual wielded
// gun reloads two magazines and one of them is a counter for the off hand,
// which is not what the HUD is counting down.
func clipOf(ammo wep.Ammo, states wep.StateMachine) string {
	filled := reloadedItems(states)
	for _, pool := range []string{ammo.Type, ammo.Type2} {
		if pool != "" && filled[strings.ToLower(pool)] {
			return pool
		}
	}
	return ""
}

// StartsWith reports whether a player class hands this weapon out before the
// first frame - `Player.StartItem "FDPlutFist"`. Such a weapon is one the
// player has always got, so it is never anywhere to be picked up: Doom's own
// fists and pistol are the shape of it, and every one of these mods keeps it.
//
// The union over every player class in the mod, because a mod with nine of
// them - one per weapon set, which is what Final Doomer is - has nine fists
// and nine pistols and all of them are somebody's starting weapon.
func StartsWith(idx *Index, actorName string) bool {
	for _, actor := range idx.Actors {
		for _, item := range actor.StartItems {
			if strings.EqualFold(item, actorName) {
				return true
			}
		}
	}
	return false
}

// ammoPickup is the ammo this weapon's own box of ammo hands over, and what
// that box looks like. A weapon names up to two ammo pools and DECORATE says
// nothing about which is the reserve and which the loaded magazine - the Dual
// 92s have them the other way round from the shape the comments assume - so
// the one that is picked up is the one that has a picture of itself on the
// floor. Nobody drops a magazine, so no mod draws one.
func (c *Compiler) ammoPickup() (item, icon string) {
	for _, ammo := range []string{c.ammoType, c.ammoType2} {
		if sprite := c.needShared(c.ammoPickupSprite(ammo)); sprite != "" {
			return ammo, sprite
		}
	}
	return "", ""
}

// ammoPickupSprite is the picture of the item that refills a weapon's reserve.
// The mod's own art first, because a mod that redefines Clip means its own
// clip; Doom's own four ammo classes are not in any mod's DECORATE at all, so
// a weapon that names one and leaves it alone is answered from
// stockAmmoSprites, the same way stockAmmoMax already answers how much of it a
// player may carry.
func (c *Compiler) ammoPickupSprite(ammoType string) string {
	if ammoType == "" {
		return ""
	}
	if sprite := c.stateSprite(ammoType, "spawn"); sprite != "" {
		return sprite
	}
	return stockAmmoSprites[strings.ToLower(ammoType)]
}

// stateSprite is the first picture an actor shows under a label: the frame the
// state starts on, skipping the TNT1 frames a script opens with. It reads the
// merged states rather than the compiled machine because an ammo item is never
// compiled - nothing about it runs - and the answer is the same either way.
func (c *Compiler) stateSprite(actorName, label string) string {
	if actorName == "" {
		return ""
	}
	if _, defined := c.idx.Actor(actorName); !defined {
		return ""
	}
	block := c.mergedStates(actorName)
	start, ok := block.Labels[label]
	if !ok {
		return ""
	}
	for i := start; i < len(block.Lines); i++ {
		line := block.Lines[i]
		if !line.IsFrame() {
			continue
		}
		sprite := strings.ToUpper(line.Sprite)
		if !visibleSprite(sprite) || sprite == wep.KeepSprite || sprite == wep.KeepBoth {
			continue
		}
		frame := unicode.ToUpper(expandFrames(line.Frames)[0])
		if frame == wep.KeepFrame || frame == '-' {
			continue
		}
		return fmt.Sprintf("%s%c0", sprite, frame)
	}
	return ""
}

// needShared asks for a picture in the shared store, which is where everything
// drawn outside the weapon in hand comes from. It hands back the name so the
// caller can record it, and nothing for an empty name.
func (c *Compiler) needShared(name string) string {
	if name == "" {
		return ""
	}
	if !c.actorSpriteSeen[name] {
		c.actorSpriteSeen[name] = true
		c.actorSprites = append(c.actorSprites, name)
	}
	return name
}

// stockAmmoSprites is what Doom's own four ammo classes look like on the
// floor. They are in doom2.wad, which every one of these migrations has last
// in its source list precisely because the mods take the base game for
// granted.
var stockAmmoSprites = map[string]string{
	"clip": "CLIPA0", "shell": "SHELA0", "rocketammo": "ROCKA0", "cell": "CELLA0",
}
