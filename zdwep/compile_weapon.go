package zdwep

import (
	"fmt"
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
	c.ammoType, _ = c.mergedProperty(actor.Name, "weapon.ammotype")
	c.ammoType = strings.Trim(c.ammoType, `"`)
	if c.ammoType != "" {
		c.needItem(c.ammoType)
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
		Ammo:       wep.Ammo{Type: c.ammoType, Use: c.ammoUse(actor.Name)},
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
	weapon.Items = c.items()
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
		Speed:     inner.floatProperty(actor.Name, "speed"),
		States:    states,
		NoGravity: inner.mergedFlag(actor.Name, "nogravity"),
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
	if use := c.intProperty(name, "weapon.ammouse"); use > 0 {
		return use
	}
	return 1
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
func (c *Compiler) items() map[string]wep.Item {
	if len(c.neededItems) == 0 {
		return nil
	}
	items := make(map[string]wep.Item, len(c.neededItems))
	for _, name := range sortedNames(c.neededItems) {
		item := wep.Item{Max: 1}
		if actor, ok := c.idx.Actor(name); ok {
			if max := c.intProperty(actor.Name, "inventory.maxamount"); max > 0 {
				item.Max = max
			}
			if amount := c.intProperty(actor.Name, "inventory.amount"); amount > 0 {
				item.Initial = 0
			}
		}
		items[name] = item
	}
	return items
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

// Weapons lists every actor in the mod that is a weapon, by name, sorted.
func Weapons(idx *Index) []string {
	var names []string
	for _, actor := range idx.Actors {
		if IsWeapon(idx, actor.Name) {
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
			if gone[machine.Steps[i].Sprite] {
				machine.Steps[i].Sprite = ""
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
