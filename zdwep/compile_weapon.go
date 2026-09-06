package zdwep

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

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
