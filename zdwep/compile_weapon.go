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
	// Sprites are the atlas frames to pack, in first use order so the packed
	// atlas comes out the same on every run.
	Sprites []string
	// Sounds are the cue names the weapon plays, as the scripts write them.
	Sounds []string
	// SourceScript is the DECORATE the weapon was compiled from, kept beside the
	// compiled weapon so a re-migration diff says whether upstream or the
	// compiler moved.
	SourceScript string
}

// CompileWeapon turns one actor and everything it reaches into compiled data.
func CompileWeapon(idx *Index, policy Policy, actorName string) (*Result, error) {
	actor, ok := idx.Actor(actorName)
	if !ok {
		return nil, fmt.Errorf("no actor named %q in this mod", actorName)
	}
	if !idx.InheritsFrom(actorName, "Weapon") {
		return nil, fmt.Errorf("%s does not inherit from Weapon", actor.Name)
	}

	c := newCompiler(idx, policy)
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
		compiled, extra, err := compileActor(idx, policy, name, c)
		if err != nil {
			return nil, err
		}
		result.Actors[strings.ToLower(name)] = compiled
		pending = append(pending, extra...)
	}

	weapon.Items = c.items()
	result.Sprites = c.sprites
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
	inner.sprites = shared.sprites
	inner.spriteSeen = shared.spriteSeen

	states, err := inner.lowerStates(inner.mergedStates(actor.Name))
	if err != nil {
		return nil, nil, fmt.Errorf("%s (%s):\n%s", actor.Name, actor.SourceFile, err)
	}

	shared.sprites = inner.sprites
	for sound := range inner.neededSounds {
		shared.needSound(sound)
	}
	for item := range inner.neededItems {
		shared.needItem(item)
	}
	for _, dropped := range inner.dropped {
		shared.drop(dropped)
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
func WeaponID(actor *Actor, idx *Index) string {
	name := actor.Name
	for _, candidate := range idx.Ancestry(actor.Name) {
		if tag, ok := candidate.Property("tag"); ok {
			name = strings.Trim(tag, `"`)
			break
		}
	}
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

// Weapons lists every actor in the mod that is a weapon, by name, sorted.
func Weapons(idx *Index) []string {
	var names []string
	for _, actor := range idx.Actors {
		if idx.InheritsFrom(actor.Name, "Weapon") {
			names = append(names, actor.Name)
		}
	}
	sort.Strings(names)
	return names
}
