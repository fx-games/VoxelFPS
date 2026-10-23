package main

// The characters and the scenery: which frames make which animation, and which
// of the game's own voices belong to which of them.

import (
	"fmt"
	"image"
	"path"
	"sort"
	"strings"

	"github.com/memmaker/voxelfps/wep"
)

// character is one of the game's inhabitants: the prefix its frames are named
// with, what to call it, and the three cues it makes a noise with. The cues are
// names in VSWAP's digitized table (sounds.go); several takes of one cue is
// several entries, which is how three different death cries stay three.
//
// The pairings are bstone's own - the switch in FirstSighting for the alert and
// the one in DoActor's death for the rest - not a guess off the names.
type character struct {
	prefix string
	id     string
	name   string
	health int
	// damage is what one of its attacks costs the player, on the same
	// hundred-point scale the player's own health is. Blake Stone's own numbers
	// are per difficulty and per class and live in bstone's state tables; these
	// are them fitted to this engine's one scale, which is what the engine can
	// actually use.
	damage int
	// group is the folder this one is filed under in assets/sprites/actors. The
	// game has no table saying what any of its characters is - the sprites are
	// named after their own prefixes - so this is by hand, from the name and
	// from where the game introduces it. See categories.go.
	group  string
	alert  []string
	death  []string
	attack []string
}

// blakeCharacters is everything in Blake Stone that walks, in the order the
// game introduces them. Health is the game's own hit points scaled to this
// engine's hundred-point actor, which is what a default NPC has.
var blakeCharacters = []character{
	{prefix: "RENT", id: "sector_patrol", name: "Sector Patrol", health: 60, damage: 8, group: "humans",
		alert: []string{"HALT"}, death: []string{"RENTDEATH1", "RENTDEATH2"}, attack: []string{"SHOOT"}},
	{prefix: "PRO", id: "star_sentinel", name: "Star Sentinel", health: 100, damage: 12, group: "humans",
		alert: []string{"PROHALT"}, death: []string{"PROGUARDDEATH", "PRODEATH2", "PRODEATH3"}, attack: []string{"SHOOT"}},
	{prefix: "SWAT", id: "star_trooper", name: "Star Trooper", health: 120, damage: 14, group: "humans",
		alert: []string{"SWATHALT"}, death: []string{"SWATDIE", "SWATDEATH2", "SWATDEATH3"}, attack: []string{"SHOOT"}},
	{prefix: "OFC", id: "general_scientist", name: "General Scientist", health: 40, damage: 6, group: "humans",
		alert: []string{"SCIENTISTHALT"}, death: []string{"SCIENTISTDEATH", "SCIDEATH2", "SCIDEATH3"}, attack: []string{"SHOOT"}},
	{prefix: "GOLD", id: "goldstern", name: "Dr. Goldstern", health: 200, damage: 18, group: "bosses",
		alert: []string{"GOLDSTERNHALT"}, death: []string{"GOLDSTERNLAUGH"}, attack: []string{"SHOOT"}},

	{prefix: "GSCOUT", id: "gold_scout", name: "Goldstern Scout", health: 80, damage: 25, group: "robots",
		alert: []string{"SCOUT_ALERT"}, death: []string{"EXPLODE1"}},
	{prefix: "FSCOUT", id: "floating_bomb", name: "Floating Bomb", health: 80, damage: 25, group: "robots",
		alert: []string{"SCOUT_ALERT"}, death: []string{"EXPLODE2"}},

	{prefix: "GENETIC", id: "genetic_guard", name: "Genetic Guard", health: 120, damage: 14, group: "mutants",
		alert: []string{"GGUARDHALT"}, death: []string{"GGUARDDEATH"}, attack: []string{"PUNCHATTACK"}},
	{prefix: "MUTHUM1", id: "mutant_human_1", name: "Mutant Human", health: 100, damage: 12, group: "mutants",
		alert: []string{"BLUEBOYHALT"}, death: []string{"BLUEBOYDEATH"}, attack: []string{"CLAWATTACK"}},
	{prefix: "MUTHUM2", id: "mutant_human_2", name: "Mutant Human II", health: 100, damage: 12, group: "mutants",
		alert: []string{"DOGBOYHALT"}, death: []string{"DOGBOYDEATH"}, attack: []string{"CLAWATTACK"}},
	{prefix: "SCAN_ALIEN", id: "scan_alien", name: "Scan Alien", health: 140, damage: 16, group: "aliens",
		alert: []string{"SCANHALT"}, death: []string{"SCANDEATH"}, attack: []string{"SPITATTACK"}},
	{prefix: "LCAN_ALIEN", id: "lcan_alien", name: "Large Canister Alien", health: 160, damage: 18, group: "aliens",
		alert: []string{"LCANHALT"}, death: []string{"LCANDEATH"}, attack: []string{"SPITATTACK"}},
	{prefix: "GURNEY_MUT", id: "gurney_mutant", name: "Gurney Mutant", health: 120, damage: 14, group: "mutants",
		alert: []string{"GURNEY"}, death: []string{"GURNEYDEATH"}, attack: []string{"CLAWATTACK"}},
	{prefix: "POD", id: "pod_alien", name: "Pod Alien", health: 140, damage: 16, group: "aliens",
		alert: []string{"PODHALT"}, death: []string{"PODDEATH"}, attack: []string{"SPITATTACK"}},
	{prefix: "ELECTRO_SPHERE", id: "electro_sphere", name: "Electro-Sphere", health: 60, damage: 10, group: "aliens",
		alert: []string{"ELECAPPEAR"}, death: []string{"ELECDIE"}, attack: []string{"ELECTSHOT"}},
	{prefix: "ELEC", id: "electro_alien", name: "Electro-Alien", health: 100, damage: 14, group: "aliens",
		alert: []string{"ELECAPPEAR"}, death: []string{"ELECDIE"}, attack: []string{"ELECTSHOT"}},
	{prefix: "LIQUID", id: "liquid_alien", name: "Liquid Alien", health: 140, damage: 16, group: "aliens",
		death: []string{"LIQUIDDIE"}, attack: []string{"CLAWATTACK"}},
	{prefix: "TERROT", id: "turret", name: "Hanging Turret", health: 80, damage: 10, group: "robots",
		alert: []string{"TURRET"}, death: []string{"EXPLODE1"}, attack: []string{"SHOOT"}},
	{prefix: "VITAL", id: "vital", name: "Vital Sign", health: 60, damage: 0, group: "robots",
		death: []string{"EXPLODE1"}},

	{prefix: "BOSS1", id: "boss_1", name: "Tech Warrior", health: 250, damage: 22, group: "bosses",
		alert: []string{"BLUEBOYHALT"}, death: []string{"BLUEBOYDEATH"}, attack: []string{"PUNCHATTACK"}},
	{prefix: "BOSS2", id: "boss_2", name: "Alien Protector", health: 250, damage: 22, group: "bosses",
		alert: []string{"GGUARDHALT"}, death: []string{"PODDEATH"}, attack: []string{"CLAWATTACK"}},
	{prefix: "BOSS3", id: "boss_3", name: "Mech Sentinel", health: 300, damage: 25, group: "bosses",
		alert: []string{"BLUEBOYHALT"}, death: []string{"BLUEBOYDEATH"}, attack: []string{"PUNCHATTACK"}},
	{prefix: "BOSS4", id: "boss_4", name: "Reptilian Warrior", health: 300, damage: 25, group: "bosses",
		alert: []string{"GGUARDHALT"}, death: []string{"PODDEATH"}, attack: []string{"CLAWATTACK"}},
	{prefix: "BOSS5", id: "boss_5", name: "Acid Dragon", health: 350, damage: 28, group: "bosses",
		alert: []string{"PODHALT"}, death: []string{"PODDEATH"}, attack: []string{"SPITATTACK"}},
	{prefix: "BOSS6", id: "boss_6", name: "Bio-Mech Guardian", health: 400, damage: 30, group: "bosses",
		alert: []string{"GGUARDHALT"}, death: []string{"GGUARDDEATH"}, attack: []string{"PUNCHATTACK"}},
}

// walkFPS and deathFPS are how fast these animations run. Blake Stone's walk is
// four frames at roughly five a second; a death is a little quicker.
const (
	walkFPS  = 5
	deathFPS = 7
	fireFPS  = 5
)

// installCharacters writes a folder per character. A character is directional
// if the game drew it from eight sides, which is what the S_ or W1_ frames say;
// everything else is drawn once and looks the same from everywhere, and comes
// out with a single direction, which the engine already understands.
func (in *installer) installCharacters() (int, error) {
	written := 0
	for _, spec := range blakeCharacters {
		grid := &sheet{}
		animations := map[string]*spriteAnim{}

		var style attackStyle
		if in.frame(spec.prefix+"_S_1") != nil || in.frame(spec.prefix+"_W1_1") != nil || in.frame(spec.prefix+"_1") != nil {
			style = in.addDirectional(spec, grid, animations)
		} else {
			var built bool
			style, built = in.addFlat(spec, grid, animations)
			if !built {
				continue
			}
		}
		if animations["idle"] == nil {
			continue
		}

		attack := attackFor(spec, style)
		kind := &spriteKind{
			Name:       spec.name,
			Actor:      true,
			Hostile:    attack != nil,
			Attack:     attack,
			Health:     spec.health,
			Scale:      spriteScale,
			Animations: animations,
			Provenance: map[string]string{
				"source": "Blake Stone: Aliens of Gold",
				"file":   "VSWAP.BS6",
				"actor":  "SPR_" + spec.prefix + "_*",
			},
		}
		in.taken[idPrefix+spec.id] = true
		id := path.Join(roleActors, spec.group, idPrefix+spec.id)
		if err := in.writeKind(id, kind, grid, spec.cues()); err != nil {
			return written, fmt.Errorf("%s: %w", spec.id, err)
		}
		written++
	}
	return written, nil
}

// cues is the sound folder this character wants: one entry a role, several
// takes each where the game has them.
func (spec character) cues() map[string][]string {
	out := map[string][]string{}
	for role, takes := range map[string][]string{"alert": spec.alert, "death": spec.death, "attack": spec.attack} {
		if len(takes) > 0 {
			out[role] = takes
		}
	}
	return out
}

// addDirectional builds the eight-view set: stand, four walk frames, and the
// death, flinch and shot the game only ever drew from the front.
func (in *installer) addDirectional(spec character, grid *sheet, animations map[string]*spriteAnim) attackStyle {
	idle := make([][]uint16, 8)
	walk := make([][]uint16, 8)
	// The two rows are laid out facing-major so the finished sheet reads as one
	// row of stands and four rows of steps, which is how the art was drawn and
	// what makes the file worth opening.
	stand := make([]image.Image, 8)
	steps := [4][]image.Image{}
	for dir := 0; dir < 8; dir++ {
		stand[dir] = in.firstFrame(
			fmt.Sprintf("%s_S_%d", spec.prefix, dir+1),
			fmt.Sprintf("%s_W1_%d", spec.prefix, dir+1),
			// A thing that turns but does not walk - the hanging turret -
			// numbers its eight views and nothing else.
			fmt.Sprintf("%s_%d", spec.prefix, dir+1),
		)
		for step := 0; step < 4; step++ {
			steps[step] = append(steps[step], in.frame(fmt.Sprintf("%s_W%d_%d", spec.prefix, step+1, dir+1)))
		}
	}
	for dir := 0; dir < 8; dir++ {
		if stand[dir] == nil {
			return attackStyle{}
		}
		idle[dir] = []uint16{grid.add(stand[dir])}
	}
	for step := 0; step < 4; step++ {
		for dir := 0; dir < 8; dir++ {
			frame := steps[step][dir]
			if frame == nil {
				frame = stand[dir]
			}
			walk[dir] = append(walk[dir], grid.add(frame))
		}
	}
	animations["idle"] = &spriteAnim{Frames: idle, FPS: walkFPS}
	animations["walk"] = &spriteAnim{Frames: walk, FPS: walkFPS, Loop: true}
	return in.addCommon(spec, grid, animations, idle[0])
}

// addFlat builds the set for something the game drew once: a mutant, a boss, a
// turret. It reports whether there was anything to build.
func (in *installer) addFlat(spec character, grid *sheet, animations map[string]*spriteAnim) (attackStyle, bool) {
	walk := in.firstSeries(
		spec.prefix+"_W%d", spec.prefix+"_WALK%d", spec.prefix+"_S%d",
		spec.prefix+"_ROAM%d",
	)
	if len(walk) == 0 {
		// A thing that never moves off its spot still has a pose it holds -
		// the wall-mounted vital sign is one picture and then a death.
		if stand := in.firstFrame(spec.prefix + "_STAND"); stand != nil {
			walk = []image.Image{stand}
		}
	}
	if len(walk) == 0 {
		// A canister alien or a gurney mutant stands in its container until it
		// wakes up; READY is that pose, and it is the only one some of them have
		// before they start walking.
		if ready := in.firstFrame(spec.prefix + "_READY"); ready != nil {
			walk = []image.Image{ready}
		}
	}
	if len(walk) == 0 {
		return attackStyle{}, false
	}
	walkRow := grid.addAll(walk)
	animations["idle"] = &spriteAnim{Frames: [][]uint16{{walkRow[0]}}, FPS: walkFPS}
	animations["walk"] = &spriteAnim{Frames: [][]uint16{walkRow}, FPS: walkFPS, Loop: true}
	return in.addCommon(spec, grid, animations, []uint16{walkRow[0]}), true
}

// addCommon adds the three animations every character shares the spelling of,
// whether it was drawn from eight sides or one: the death, the flinch, and the
// wind-up and release of whatever it attacks with. Anything the game has not
// got falls back to the idle frame, so an animation always exists.
func (in *installer) addCommon(spec character, grid *sheet, animations map[string]*spriteAnim, fallback []uint16) (attackStyle attackStyle) {
	death := in.firstSeries(spec.prefix+"_DIE_%d", spec.prefix+"_DIE%d", spec.prefix+"_DEAD_%d")
	// The corpse frame is the pose the body rests in, so it is the last frame of
	// the death rather than a separate thing - the engine freezes there.
	if corpse := in.firstFrame(spec.prefix + "_DEAD"); corpse != nil {
		death = append(death, corpse)
	}
	if len(death) > 0 {
		animations["dying"] = &spriteAnim{Frames: [][]uint16{grid.addAll(death)}, FPS: deathFPS}
	} else {
		animations["dying"] = &spriteAnim{Frames: [][]uint16{fallback}, FPS: deathFPS}
	}

	if hurt := in.firstFrame(spec.prefix+"_PAIN_1", spec.prefix+"_OUCH", spec.prefix+"_PAIN1"); hurt != nil {
		animations["hit"] = &spriteAnim{Frames: [][]uint16{{grid.add(hurt)}}, FPS: deathFPS}
	} else {
		animations["hit"] = &spriteAnim{Frames: [][]uint16{fallback}, FPS: deathFPS}
	}

	// Which spelling the game used is also what kind of attack it is: a guard
	// SHOOTs, a mutant SWINGs, a canister alien SPITs. That is the only record
	// of it there is, and it is enough - see attackFor.
	var attack []image.Image
	for _, style := range attackStyles {
		if frames := in.series(spec.prefix + style.suffix); len(frames) > 0 {
			attack, attackStyle = frames, style
			break
		}
	}
	if len(attack) == 0 {
		animations["prepare_fire"] = &spriteAnim{Frames: [][]uint16{fallback}, FPS: fireFPS}
		animations["fire"] = &spriteAnim{Frames: [][]uint16{fallback}, FPS: fireFPS}
		return
	}
	row := grid.addAll(attack)
	// The wind-up is everything but the last frame and the release is the run
	// back down through them, which is how the engine's own first character is
	// animated - the shot lands on the frame the art puts the flash on.
	windUp := row[:len(row)-1]
	if len(windUp) == 0 {
		windUp = row
	}
	release := make([]uint16, 0, len(row))
	for i := len(row) - 1; i >= 0; i-- {
		release = append(release, row[i])
	}
	animations["prepare_fire"] = &spriteAnim{Frames: [][]uint16{windUp}, FPS: fireFPS}
	animations["fire"] = &spriteAnim{Frames: [][]uint16{release}, FPS: fireFPS}
	return attackStyle
}

// attackStyle is one way a Blake Stone character attacks: the frame-name
// suffix it is drawn under, how far it reaches, and whether what it throws
// travels.
type attackStyle struct {
	suffix     string
	reach      float32
	projectile bool
}

// attackStyles are the four the game has, in the order they are looked for. A
// character drawn with more than one is at its most dangerous first.
var attackStyles = []attackStyle{
	{suffix: "_SHOOT%d", reach: 24},
	{suffix: "_FIRE_%d", reach: 24},
	{suffix: "_SPIT%d", reach: 18, projectile: true},
	{suffix: "_SWING%d", reach: 2.5},
}

// attackFor is what this character's attack becomes on disk, or nil for one the
// game drew no attack for at all.
func attackFor(spec character, style attackStyle) *spriteAttack {
	if style.suffix == "" || spec.damage <= 0 {
		return nil
	}
	return &spriteAttack{
		Damage:     spec.damage,
		Range:      style.reach,
		Projectile: style.projectile,
	}
}

// prop is a piece of scenery: one folder, one animation, however many frames
// the game drew for it. The request that drove this was barrels - a thing with
// four frames has to keep all four and be one object, not four sprites.
type prop struct {
	// name is what the editor calls it. frames is the printf pattern of a
	// numbered run, or single is one picture's own name.
	name   string
	frames string
	single string
	fps    float32
}

// blakeProps are the scenery pieces whose frames the game numbers, which is
// every animated one. The rest - the eighty numbered statics - are found by
// walking staticNames, so this table only carries what a rule cannot.
var blakeProps = map[string]prop{
	"steam":         {name: "Steam Vent", frames: "STEAM_%d", fps: 6},
	"pipe_steam":    {name: "Steam Pipe Jet", frames: "PIPE_STEAM_%d", fps: 6},
	"blood_drip":    {name: "Blood Drip", frames: "BLOOD_DRIP%d", fps: 5},
	"water_drip":    {name: "Water Drip", frames: "WATER_DRIP%d", fps: 5},
	"deco_arc":      {name: "Arc Decoration", frames: "DECO_ARC_%d", fps: 8},
	"green_ooze":    {name: "Green Ooze", frames: "GREEN_OOZE%d", fps: 4},
	"black_ooze":    {name: "Black Ooze", frames: "BLACK_OOZE%d", fps: 4},
	"explosion":     {name: "Explosion", frames: "EXPLOSION_%d", fps: 12},
	"grenade_blast": {name: "Fission Blast", frames: "GRENADE_EXPLODE%d", fps: 12},
	"security_lamp": {name: "Security Lamp", frames: "SECURITY_%s", fps: 3},
	"air_vent":      {name: "Air Vent", single: "AIR_VENT"},
	"grate":         {name: "Floor Grate", single: "GRATE"},
	"steam_pipe":    {name: "Steam Pipe", single: "STEAM_PIPE"},
	"rubble":        {name: "Door Rubble", single: "RUBBLE"},
	"bonzi_tree":    {name: "Bonzi Tree", single: "BONZI_TREE"},
	"pot_plant":     {name: "Potted Plant", single: "POT_PLANT"},
	"tube_plant":    {name: "Tube Plant", single: "TUBE_PLANT"},
	"hitech_chair":  {name: "Hi-Tech Chair", single: "HITECH_CHAIR"},
	"doorbomb":      {name: "Plasma Detonator", single: "DOORBOMB"},
	"automapper":    {name: "Auto-Mapper Bonus", single: "AUTOMAPPER"},
	"candy_bar":     {name: "Candy Bar", single: "CANDY_BAR"},
	"candy_wrapper": {name: "Candy Wrapper", single: "CANDY_WRAPER"},
	"sandwich":      {name: "Sandwich", single: "SANDWICH"},
	"sandwich_wrap": {name: "Sandwich Wrapper", single: "SANDWICH_WRAPER"},
	"dead_rent":     {name: "Dead Sector Patrol", single: "DEAD_RENT"},
	"dead_pro":      {name: "Dead Star Sentinel", single: "DEAD_PRO"},
	"dead_swat":     {name: "Dead Star Trooper", single: "DEAD_SWAT"},
}

// installProps writes a folder for every piece of scenery: the animated ones
// from the table above, then every numbered static bstone gave a name to.
func (in *installer) installProps() (int, error) {
	written := 0
	for id, spec := range blakeProps {
		var frames []image.Image
		switch {
		case spec.single != "":
			if img := in.frame(spec.single); img != nil {
				frames = []image.Image{img}
			}
		case strings.Contains(spec.frames, "%s"):
			// The security lamp is the one pair the game named rather than
			// numbered.
			for _, state := range []string{"NORMAL", "ALERT"} {
				if img := in.frame(fmt.Sprintf(spec.frames, state)); img != nil {
					frames = append(frames, img)
				}
			}
		default:
			frames = in.series(spec.frames)
		}
		if len(frames) == 0 {
			continue
		}
		if in.taken[idPrefix+id] {
			continue
		}
		in.taken[idPrefix+id] = true
		if err := in.writeProp(path.Join(blakePropCategory(id), idPrefix+id), spec.name, frames, spec.fps, "SPR_"+spec.single+spec.frames); err != nil {
			return written, err
		}
		written++
	}

	// The numbered statics, in sprite order so two runs write the same folders.
	// Several of bstone's own comments name the same thing twice - two coins,
	// two gurney mutants - and one of them names a character that already has a
	// folder, so a name already taken keeps its sprite's own name, which is the
	// only thing that tells those apart.
	for _, name := range sortedStatics() {
		img := in.frame(name)
		if img == nil {
			continue
		}
		label := staticNames[name][0]
		id := idPrefix + slug(label)
		if in.taken[id] {
			id = idPrefix + slug(name) + "_" + slug(label)
		}
		in.taken[id] = true
		filed := path.Join(staticCategory(name, staticNames[name][1]), id)
		if err := in.writeProp(filed, label, []image.Image{img}, 0, "SPR_"+name); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// sortedStatics is the numbered statics in the order the game numbers them, so
// two runs of the importer produce the same folders - a map's order does not.
func sortedStatics() []string {
	names := make([]string, 0, len(staticNames))
	for name := range staticNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// writeProp is one scenery folder: every frame it has, as one looping idle.
func (in *installer) writeProp(id, name string, frames []image.Image, fps float32, source string) error {
	grid := &sheet{}
	row := grid.addAll(frames)
	kind := &spriteKind{
		Name:  name,
		Scale: spriteScale,
		Animations: map[string]*spriteAnim{
			"idle": {Frames: [][]uint16{row}, FPS: fps, Loop: len(row) > 1},
		},
		Provenance: map[string]string{
			"source": "Blake Stone: Aliens of Gold",
			"file":   "VSWAP.BS6",
			"actor":  source,
		},
	}
	return in.writeKind(id, kind, grid, nil)
}

// slug turns a name out of the game into a folder name. A few of bstone's own
// comments start with the sprite's own number ("63 loot"); the number is not
// part of what the thing is called, and it is dropped.
func slug(name string) string {
	name = strings.TrimLeft(name, "0123456789 -")
	var out strings.Builder
	previousDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			previousDash = false
		case !previousDash:
			out.WriteByte('_')
			previousDash = true
		}
	}
	return strings.Trim(out.String(), "_")
}

// grenadeBlastCue is what the launcher files the explosion under in its own
// sounds folder. The thrown actor plays its weapon's cues, not its own.
const grenadeBlastCue = "explode"

// fissionGrenade is what the Fission Detonator throws: the game's own four
// tumbling frames, then its five-frame blast, which is where the damage is.
func (in *installer) fissionGrenade() (*wep.Effect, map[string]image.Image) {
	sprites := map[string]image.Image{}
	states := wep.StateMachine{Labels: map[string]int{}}

	fly := in.series("GRENADE_FLY%d")
	states.Labels["spawn"] = len(states.Steps)
	for i, img := range fly {
		name := fmt.Sprintf("BSGF%c0", 'A'+rune(i))
		sprites[name] = wep.Magnified(img, 1)
		states.Steps = append(states.Steps, wep.Step{Sprite: name, Tics: 3})
	}
	states.Steps = append(states.Steps, wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}})

	blast := in.series("GRENADE_EXPLODE%d")
	states.Labels["death"] = len(states.Steps)
	for i, img := range blast {
		name := fmt.Sprintf("BSGX%c0", 'A'+rune(i))
		sprites[name] = wep.Magnified(img, 1)
		step := wep.Step{Sprite: name, Tics: 3}
		if i == 0 {
			step.Ops = []wep.Op{
				{Op: wep.OpPlaySound, Sound: grenadeBlastCue, Volume: 1},
				{Op: wep.OpExplode, Damage: 60, N: 160},
			}
		}
		states.Steps = append(states.Steps, step)
	}
	states.Steps = append(states.Steps, wep.Step{Tics: 0, Ops: []wep.Op{{Op: wep.OpRemove}}})

	return &wep.Effect{
		Format:    wep.Format,
		Name:      "BlakeGrenade",
		Render:    wep.Render{Scale: [2]float64{1, 1}, Alpha: 1},
		Speed:     22,
		States:    states,
		Missile:   true,
		NoGravity: false,
	}, sprites
}
