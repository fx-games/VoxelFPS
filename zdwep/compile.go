package zdwep

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/memmaker/voxelfps/wep"
)

// Compiling is the second phase: the index says what the mod contains, and this
// turns one weapon of it into data the engine can step. Everything ZDoom
// specific stops here. Sprite names, sound names, inheritance, relative jumps
// and the mod's own configuration are all resolved, and anything that cannot be
// is an error naming the actor and the line, never a warning at runtime.

// Compiler holds the state of compiling one actor.
type Compiler struct {
	idx    *Index
	policy Policy

	actorName string
	ammoType  string
	ammoType2 string

	// frameSteps maps a frame's ordinal within the actor's states to the step
	// it became. ZDoom's numeric jump offsets count frames and skip the goto
	// and loop directives between them, so a jump of 2 needs this rather than
	// a step index.
	frameSteps []int
	// currentFrame is the ordinal of the frame being lowered, which is where a
	// relative jump counts from.
	currentFrame int

	// strictLabels is on for a weapon, whose jumps must all land, and off for
	// an actor reached through one. A base actor legitimately jumps to a label
	// only its subclasses define - Final Doomer's pickup base does exactly
	// that - and compiling the base on its own cannot resolve it.
	strictLabels bool

	neededItems  map[string]bool
	neededActors map[string]bool
	neededSounds map[string]bool
	neededLabels map[string]bool
	// sprites is every atlas frame the compiled actor asks for, in first use
	// order, which keeps the packed atlas stable between runs.
	sprites    []string
	spriteSeen map[string]bool
	// actorSprites are the frames of the things a weapon spawns, kept apart
	// because they are shared between weapons and packed once for all of them
	// rather than into each weapon's own atlas.
	actorSprites    []string
	actorSpriteSeen map[string]bool
	dropped         []string
	droppedSeen     map[string]bool
	// defaulted names the puffs and spawners the policy had no line for, so
	// the compiled weapon can carry what it guessed at.
	defaultedNames []string
	defaultedSeen  map[string]bool
}

func newCompiler(idx *Index, policy Policy) *Compiler {
	return &Compiler{
		idx:             idx,
		policy:          policy,
		neededItems:     make(map[string]bool),
		neededActors:    make(map[string]bool),
		neededSounds:    make(map[string]bool),
		neededLabels:    make(map[string]bool),
		spriteSeen:      make(map[string]bool),
		actorSpriteSeen: make(map[string]bool),
		droppedSeen:     make(map[string]bool),
		defaultedSeen:   make(map[string]bool),
	}
}

func (c *Compiler) needItem(name string)  { c.neededItems[name] = true }
func (c *Compiler) needActor(name string) { c.neededActors[name] = true }
func (c *Compiler) needSound(name string) { c.neededSounds[name] = true }
func (c *Compiler) needLabel(name string) { c.neededLabels[strings.ToLower(name)] = true }
func (c *Compiler) actorIsMissile(n string) bool {
	actor, ok := c.idx.Actor(n)
	if !ok {
		return false
	}
	for _, a := range c.idx.Ancestry(n) {
		if a.Flags["missile"] {
			return true
		}
	}
	return actor.Flags["missile"] || c.inheritsAStockProjectile(n)
}

// stockProjectiles are ZDoom's own missile classes. A mod inherits from one and
// writes nothing about how it flies, because all of that - +MISSILE, +NOGRAVITY
// and the rest - is in the class, which the mod never ships and this index
// therefore never sees. Final Doomer's rocket is `FDTNTMissile : Rocket` and
// nothing else, and read as a plain actor it fell out of the launcher and
// landed at the player's feet.
// The value is the class's own Speed, which the mod inherits along with the
// flags and just as invisibly. `SuperBFGBall : BFGBall` writes a damage, a
// decal and a death, and not one word about how fast it travels, because
// BFGBall says 25; read as nothing, Brutal Doom's BFG ball hung in the air at
// the end of the barrel. A zero means the class has no speed of its own -
// FastProjectile is a behaviour, not a missile, and every mod that inherits it
// writes its own.
var stockProjectiles = map[string]float64{
	"rocket": 20, "fastprojectile": 0, "plasmaball": 25, "bfgball": 25,
	// BFGExtra is the spray's hit effect. It is not actually a Projectile in
	// ZDoom - it carries +NOBLOCKMAP +NOGRAVITY and nothing else - but those
	// are the two flags that matter here and Projectile implies both.
	"bfgextra": 0, "doomimpball": 10, "cacodemonball": 10, "baronball": 15,
	"hellknightball": 15, "fatshot": 20, "arachnotronplasma": 25,
	"revenanttracer": 10, "spidermastermindplasma": 25,
}

// stockFloaters are ZDoom's own classes that carry +NOGRAVITY and are not in
// any mod to be read, the same blind spot stockProjectiles covers for missiles.
// Every puff in these mods is `: BulletPuff`, which is +NOGRAVITY, and nothing
// the subclass writes says so; read as a plain actor the puff fell, so the
// glow a hitscan leaves sank away below the bullet chip it was drawn beside
// while the chip stayed on the wall.
//
// ponytail: the classes actually inherited from here, not ZDoom's whole
// hierarchy. A migration that meets a new one fails loudly at the same place
// this list is read from, which is better than a table nobody can check.
var stockFloaters = map[string]bool{
	"bulletpuff": true,
	// The melee equivalent, and Heretic's, which several of these mods reach
	// for when a blade hits a wall.
	"woodpuff": true, "metalpuff": true,
}

// inheritsAStockFloater reports whether the actor's line ends at one of them.
// The last actor a mod defines names its parent even when that parent is not
// in the index, which is exactly the case this has to answer.
func (c *Compiler) inheritsAStockFloater(name string) bool {
	chain := c.idx.Ancestry(name)
	if len(chain) == 0 {
		return false
	}
	return stockFloaters[strings.ToLower(chain[len(chain)-1].Parent)]
}

// stockProjectileParent is the ZDoom missile class the actor's line ends at,
// if any. The last actor the mod defines names its parent even when that
// parent is not in the index, which is exactly the case this has to answer.
func (c *Compiler) stockProjectileParent(name string) (string, bool) {
	chain := c.idx.Ancestry(name)
	if len(chain) == 0 {
		return "", false
	}
	parent := strings.ToLower(chain[len(chain)-1].Parent)
	_, known := stockProjectiles[parent]
	return parent, known
}

// inheritsAStockProjectile reports whether the actor's line ends at one of
// them.
func (c *Compiler) inheritsAStockProjectile(name string) bool {
	_, ok := c.stockProjectileParent(name)
	return ok
}

// speedOf is the actor's Speed, falling back on the stock class it inherits
// from when nothing in the mod's own chain writes one.
func (c *Compiler) speedOf(name string) float64 {
	if speed := c.floatProperty(name, "speed"); speed != 0 {
		return speed
	}
	if parent, ok := c.stockProjectileParent(name); ok {
		return stockProjectiles[parent]
	}
	return 0
}

// eval works out what a DECORATE expression comes to, against the mod's own
// settings and the state a running actor keeps.
func (c *Compiler) eval(text string) (Value, error) {
	return EvalExpr(text, c.resolveName)
}

// resolveName is what an expression's names mean here. Three kinds: the mod's
// settings, which are numbers by the time we read them; ZDoom's flag and
// pointer words, which are numbers too; and everything an actor carries while
// it runs, which is a name the engine reads rather than a number.
func (c *Compiler) resolveName(name string, args []Arg) (Resolution, bool) {
	lower := strings.ToLower(name)

	// CheckClass asks what kind of thing something is, which only the running
	// game knows.
	if strings.EqualFold(name, "CheckClass") && len(args) >= 1 && args[0].IsStr {
		return Resolution{Var: "class:" + strings.ToLower(args[0].Str)}, true
	}

	// CountInv reads how many of an item the actor is carrying, which the
	// engine's own inventory already has an answer for, unlike CallACS or
	// CheckClass - it ships as a runtime read rather than reading as zero.
	if strings.EqualFold(name, "CountInv") && len(args) == 1 && args[0].IsStr {
		// Kept in the case the script itself wrote it in, the same as every
		// other inventory-item reference here (A_GiveInventory, A_TakeInven
		// tory): Inventory is a plain map keyed by that spelling, and
		// lowercasing only this one path would read as always empty against
		// an AmmoType1 or a Give/Take spelled with any capital in it.
		item := args[0].Str
		c.needItem(item)
		return Resolution{Var: "inv:" + item}, true
	}

	// A mod script called for its answer, in an expression. We have no ACS, so
	// the engine answers it.
	if strings.HasPrefix(lower, "acs_named") && len(args) >= 1 && args[0].IsStr {
		c.defaulted("acs script " + args[0].Str)
		return Resolution{Var: "acs:" + strings.ToLower(args[0].Str)}, true
	}

	if strings.EqualFold(name, "CallACS") && len(args) == 1 && args[0].IsStr {
		script := strings.ToLower(args[0].Str)
		if value, known := c.policy.Settings.ACS[script]; known {
			return Resolution{Const: constant(value)}, true
		}
		// A script rather than a setting. We have no ACS, so it becomes
		// something the engine answers, and the compiled weapon records that it
		// was left open.
		c.defaulted("acs " + args[0].Str)
		return Resolution{Var: "acs:" + script}, true
	}

	if len(args) > 0 {
		return Resolution{}, false
	}

	if value, known := zdoomConstant(lower); known {
		return Resolution{Const: constant(value)}, true
	}
	// The mod's own `const int x = 200;`, which its scripts pass as arguments.
	if value, known := c.idx.Constants[lower]; known {
		return Resolution{Const: constant(value)}, true
	}
	if actorVariable(lower) {
		return Resolution{Var: lower}, true
	}
	return Resolution{}, false
}

// zdoomConstant covers the named numbers DECORATE writes into arguments: the
// flag words each function has its own family of, and the pointer selectors.
// Only the ones whose value changes what we compile are spelled out; the rest
// read as zero, because the lowering that cares reads the argument's text.
func zdoomConstant(name string) (float64, bool) {
	switch name {
	case "chan_auto":
		return 0, true
	case "chan_weapon":
		return 1, true
	case "chan_voice":
		return 2, true
	case "chan_item":
		return 3, true
	case "chan_body":
		return 4, true
	case "chan_5":
		return 5, true
	case "chan_6":
		return 6, true
	case "chan_7":
		return 7, true
	case "attn_none":
		return 0, true
	case "attn_norm":
		return 1, true
	case "attn_idle":
		return 1.001, true
	case "attn_static":
		return 3, true
	case "aaptr_default", "aaptr_null":
		return 0, true
	case "aaptr_target":
		return 1, true
	case "aaptr_master":
		return 2, true
	case "aaptr_tracer":
		return 4, true
	}
	for _, family := range flagFamilies {
		if strings.HasPrefix(name, family) {
			return 0, true
		}
	}
	return 0, false
}

// flagFamilies are the prefixes ZDoom's flag words come in. A flag contributes
// nothing to a number, and the lowerings that act on flags read the argument's
// text instead of its value.
var flagFamilies = []string{
	"wrf_", "fpf_", "tif_", "cbf_", "sxf_", "rgf_", "xf_", "jlosf_",
	"cpf_", "rtf_", "wof_", "smf_", "ptrof_", "qf_", "spf_", "fbf_",
	"raf_", "srf_", "warpf_", "kilf_",
}

// actorVariable reports whether a name is state a running actor carries. The
// user_ variables are a script's own scratch space; the rest is the handful of
// actor properties DECORATE lets an expression read.
func actorVariable(name string) bool {
	if strings.HasPrefix(name, "user_") {
		return true
	}
	switch name {
	case "alpha", "scalex", "scaley", "health", "mass", "waterlevel",
		"velx", "vely", "velz", "momx", "momy", "momz", "x", "y", "z",
		"args", "special1", "special2", "score", "reactiontime", "tid",
		"height", "radius", "speed", "floorz", "ceilingz", "damage",
		"stamina", "gravity", "frame", "scale", "ceilingwater", "accuracy":
		return true
	}
	return false
}

// evalInt reads an expression that has to be a plain number, such as how many
// pellets to fire. A value that varies collapses to the middle of its span.
func (c *Compiler) evalInt(text string) (int, error) {
	value, err := c.eval(text)
	if err != nil {
		return 0, err
	}
	return int(value.Range.Value()), nil
}

// evalArgs reads a run of arguments. When every one of them settles here they
// travel as numbers; when any depends on the running actor they all travel as
// expressions, because the engine has to work them out together anyway.
func (c *Compiler) evalArgs(call Call, defaults ...string) ([]float64, []wep.Expr, error) {
	numbers := make([]float64, 0, len(defaults))
	programs := make([]wep.Expr, 0, len(defaults))
	varies := false
	for i, fallback := range defaults {
		value, err := c.eval(call.Arg(i, fallback))
		if err != nil {
			return nil, nil, err
		}
		numbers = append(numbers, value.Range.Value())
		programs = append(programs, value.program())
		if !value.Folded() {
			varies = true
		}
	}
	if varies {
		return numbers, programs, nil
	}
	return numbers, nil, nil
}

// evalFloat is evalInt for a value with a fraction.
func (c *Compiler) evalFloat(text string) (float64, error) {
	value, err := c.eval(text)
	if err != nil {
		return 0, err
	}
	return value.Range.Value(), nil
}

// engineLabels are state labels GZDoom provides itself, which no DECORATE file
// defines and every mod jumps to. LightDone ends a weapon's flash overlay, and
// Null removes an actor; both mean the layer is finished, which is a Stop.
var engineLabels = map[string]bool{"lightdone": true, "null": true}

// jumpTarget turns a jump's destination into a label name. A number is an
// offset in frames from the frame being lowered, which becomes a label we
// invent for the frame it lands on.
func (c *Compiler) jumpTarget(text string) (string, error) {
	text = strings.TrimSpace(strings.Trim(strings.TrimSpace(text), `"`))
	if text == "" {
		return "", errors.New("a jump with no destination")
	}
	if offset, err := strconv.Atoi(text); err == nil {
		return fmt.Sprintf("@frame%d", c.currentFrame+offset), nil
	}
	// Super::Ready reads as Ready. Our states are already flattened over the
	// parent chain, so the parent's label is the one we have unless the child
	// overrode it, which is a distinction no weapon here relies on.
	if index := strings.LastIndex(text, "::"); index >= 0 {
		text = text[index+2:]
	}
	if engineLabels[strings.ToLower(text)] {
		return engineStopLabel, nil
	}
	if plus := strings.IndexByte(text, '+'); plus >= 0 {
		name := strings.TrimSpace(text[:plus])
		offset, err := strconv.Atoi(strings.TrimSpace(text[plus+1:]))
		if err != nil {
			return "", fmt.Errorf("cannot read jump destination %q", text)
		}
		c.needLabel(name)
		return fmt.Sprintf("%s+%d", name, offset), nil
	}
	c.needLabel(text)
	return text, nil
}

func (c *Compiler) defaulted(what string) {
	if c.defaultedSeen[what] {
		return
	}
	c.defaultedSeen[what] = true
	c.defaultedNames = append(c.defaultedNames, what)
}

func (c *Compiler) drop(source string) {
	if source == "" || c.droppedSeen[source] {
		return
	}
	c.droppedSeen[source] = true
	c.dropped = append(c.dropped, source)
}

// mergedStates flattens an actor's states over its parent chain. The lines of
// each actor stay contiguous so the fallthrough inside one actor's block
// survives, and a nearer actor's labels win.
func (c *Compiler) mergedStates(name string) StateBlock {
	chain := c.idx.Ancestry(name)
	merged := StateBlock{Labels: make(map[string]int)}
	for i := len(chain) - 1; i >= 0; i-- {
		actor := chain[i]
		offset := len(merged.Lines)
		merged.Lines = append(merged.Lines, actor.States.Lines...)
		for _, label := range actor.States.LabelOrder {
			lower := strings.ToLower(label)
			if _, exists := merged.Labels[lower]; !exists {
				merged.LabelOrder = append(merged.LabelOrder, label)
			}
			merged.Labels[lower] = actor.States.Labels[lower] + offset
		}
	}
	return merged
}

// mergedProperty reads a property from the nearest actor in the chain that
// sets it.
func (c *Compiler) mergedProperty(name, property string) (string, bool) {
	for _, actor := range c.idx.Ancestry(name) {
		if value, ok := actor.Property(property); ok {
			return value, true
		}
	}
	return "", false
}

func (c *Compiler) mergedFlag(name, flag string) bool {
	value, _ := c.mergedFlagSet(name, flag)
	return value
}

// mergedFlagSet also says whether anything in the actor's line wrote the flag
// at all, which is a different answer from writing it off: a missile that
// inherits from a class this mod never defines is one whose flags are all in
// that class, and only a mod that writes `-NOGRAVITY` itself means it.
func (c *Compiler) mergedFlagSet(name, flag string) (value, written bool) {
	for _, actor := range c.idx.Ancestry(name) {
		if value, set := actor.Flags[strings.ToLower(flag)]; set {
			return value, true
		}
	}
	return false, false
}

// lowerStates turns a state block into a state machine.
func (c *Compiler) lowerStates(block StateBlock) (wep.StateMachine, error) {
	machine := wep.StateMachine{Labels: make(map[string]int)}
	c.frameSteps = nil
	c.currentFrame = 0

	// lineSteps maps a source line to the step it produced, so labels can be
	// placed once the steps exist.
	lineSteps := make([]int, len(block.Lines))
	// Two passes: the first counts frames so a forward relative jump can name a
	// frame that has not been lowered yet, the second lowers.
	frameOfLine := make([]int, len(block.Lines))
	frameCount := 0
	for i, line := range block.Lines {
		frameOfLine[i] = frameCount
		if line.IsFrame() {
			frameCount += len([]rune(expandFrames(line.Frames)))
		}
	}

	// What a keep marker can land on. A "####" frame takes its sprite from the
	// actor it is running on rather than from the line it is written on, and a
	// "#" frame letter takes the letter the same way, so the name of the
	// picture is only known while the thing is running. The atlas is packed
	// long before then, so every name it could be is worked out here: every
	// sprite these states name, and every frame letter they use.
	prefixes, letters := keepCandidates(block)

	var problems []string

	for i, line := range block.Lines {
		lineSteps[i] = len(machine.Steps)
		if !line.IsFrame() {
			step, err := c.lowerFlowLine(line)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			machine.Steps = append(machine.Steps, step)
			continue
		}

		// A sprite name and a frame letter are what they are whichever case
		// they are written in, and mods write both. Uppercasing them here
		// keeps one name for one picture, in the atlas and in the shared store
		// alike, and leaves the keep markers - which are punctuation - alone.
		sprite := strings.ToUpper(line.Sprite)
		for _, frame := range expandFrames(line.Frames) {
			frame = unicode.ToUpper(frame)
			if frame == '-' {
				frame = wep.KeepFrame
			}
			c.currentFrame = frameOfLine[i] + (len(machine.Steps) - lineSteps[i])
			step := wep.Step{
				Tics:    line.Duration,
				TicsMax: line.DurationMax,
				Bright:  line.Bright,
				OffsetX: line.OffsetX,
				OffsetY: line.OffsetY,
			}
			if visibleSprite(sprite) {
				step.Sprite = fmt.Sprintf("%s%c0", sprite, frame)
				// A marker goes through as written; the engine reads it
				// against whatever the actor is showing. What has to be in the
				// atlas is the other thing: the names it can resolve to.
				sameSprite := sprite == wep.KeepSprite || sprite == wep.KeepBoth
				sameFrame := frame == wep.KeepFrame || sprite == wep.KeepBoth
				switch {
				case sameSprite && sameFrame:
					// It keeps the whole name, so there is nothing to spell
					// out and no picture of its own to pack.
					step.Sprite = wep.KeepBoth
				case sameSprite:
					for _, prefix := range prefixes {
						step.SpriteAlt = append(step.SpriteAlt, fmt.Sprintf("%s%c0", prefix, frame))
					}
				case sameFrame:
					for _, letter := range letters {
						step.SpriteAlt = append(step.SpriteAlt, sprite+letter+"0")
					}
				default:
					c.useSprite(step.Sprite)
				}
				for _, name := range step.SpriteAlt {
					c.useSprite(name)
				}
			}
			ops, err := c.lowerAction(line)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s:%d: %s", line.SourceFile, line.Line, err))
			}
			step.Ops = ops
			c.frameSteps = append(c.frameSteps, len(machine.Steps))
			machine.Steps = append(machine.Steps, step)
		}
	}

	for _, label := range block.LabelOrder {
		lower := strings.ToLower(label)
		index := block.Labels[lower]
		if index >= len(lineSteps) {
			continue // a label at the very end, pointing past the last line
		}
		machine.Labels[lower] = lineSteps[index]
	}

	c.addEngineStop(&machine)
	if err := c.resolveJumps(&machine); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return machine, errors.New(strings.Join(problems, "\n"))
	}
	return machine, nil
}

// keepCandidates lists what the keep markers in a block of states can be read
// as: the sprites it names and the frame letters it uses, both uppercased and
// in first use order so the packed atlas stays stable between runs.
func keepCandidates(block StateBlock) (prefixes, letters []string) {
	seenPrefix := make(map[string]bool)
	seenLetter := make(map[rune]bool)
	for _, line := range block.Lines {
		if !line.IsFrame() {
			continue
		}
		sprite := strings.ToUpper(line.Sprite)
		if visibleSprite(sprite) && sprite != wep.KeepSprite && sprite != wep.KeepBoth && !seenPrefix[sprite] {
			seenPrefix[sprite] = true
			prefixes = append(prefixes, sprite)
		}
		for _, frame := range expandFrames(line.Frames) {
			frame = unicode.ToUpper(frame)
			if frame == wep.KeepFrame || frame == '-' || seenLetter[frame] {
				continue
			}
			seenLetter[frame] = true
			letters = append(letters, string(frame))
		}
	}
	return prefixes, letters
}

// expandFrames turns a frame sequence into its characters. ZDoom writes several
// frames on one line and runs the same action on each of them.
func expandFrames(frames string) []rune {
	if frames == "" {
		return []rune{'A'}
	}
	return []rune(frames)
}

func visibleSprite(sprite string) bool {
	upper := strings.ToUpper(sprite)
	return upper != "" && upper != "TNT1" && upper != "NULL"
}

func (c *Compiler) useSprite(name string) {
	if c.spriteSeen[name] {
		return
	}
	c.spriteSeen[name] = true
	c.sprites = append(c.sprites, name)
}

// previousFrameLabel names the frame a Wait sends the cursor back to: the last
// one lowered, which is the one written above it. It is the label a `Goto 0`
// would resolve to, and resolveJumps places it like any other.
func (c *Compiler) previousFrameLabel() string {
	return fmt.Sprintf("@frame%d", c.currentFrame)
}

func (c *Compiler) lowerFlowLine(line StateLine) (wep.Step, error) {
	// A Loop, Goto, Stop or Wait is not a state in ZDoom - it is how one state
	// links to the next - so it shows nothing of its own and changes nothing
	// about what is being shown. An empty sprite here would say the opposite,
	// and blank every "####" frame reached through a loop.
	step := wep.Step{Tics: 0, Sprite: wep.KeepBoth}
	switch line.Flow {
	case "loop":
		step.Ops = []wep.Op{{Op: wep.OpLoop, Source: "Loop"}}
	case "stop":
		step.Ops = []wep.Op{{Op: wep.OpStop, Source: "Stop"}}
	case "wait", "fail":
		// A Wait is a link back to the frame above it, pointed at itself:
		// FStateDefinitions::SetWait marks that frame SDF_WAIT and FinishStates
		// resolves it to `NextState = &itself`. So the frame repeats - its
		// duration runs out and it is entered again, action and all - which is
		// how `"####" I 1 A_FadeTo(0, 0.01, 1)` followed by Wait fades a shell
		// casing out a hundredth at a time rather than fading it once. Held on
		// the Wait itself instead, the casing stayed where it was at full
		// alpha until something else took it away.
		//
		// Fail is the same statement: GZDoom's state parser sends WAIT and
		// FAIL to the same SetWait.
		source := "Wait"
		if line.Flow == "fail" {
			source = "Fail"
		}
		step.Ops = []wep.Op{{Op: wep.OpGoto, To: c.previousFrameLabel(), Source: source}}
	case "goto":
		target := line.FlowTarget
		if line.FlowOffset > 0 {
			target = fmt.Sprintf("%s+%d", target, line.FlowOffset)
		}
		resolved, err := c.jumpTarget(target)
		if err != nil {
			return step, fmt.Errorf("%s:%d: %w", line.SourceFile, line.Line, err)
		}
		step.Ops = []wep.Op{{Op: wep.OpGoto, To: resolved, Source: "Goto " + target}}
	default:
		return step, fmt.Errorf("%s:%d: unknown control flow %q", line.SourceFile, line.Line, line.Flow)
	}
	return step, nil
}

// lowerAction turns a frame's action call, or its brace-wrapped sequence of
// several, into ops, through the policy. All of them land on the one step
// this frame becomes, the same as GZDoom running them the instant it enters.
func (c *Compiler) lowerAction(line StateLine) ([]wep.Op, error) {
	actions := line.Actions
	if len(actions) == 0 {
		if strings.TrimSpace(line.Action) == "" {
			return nil, nil
		}
		actions = []string{line.Action}
	}
	var ops []wep.Op
	for _, action := range actions {
		lowered, err := c.lowerOneAction(action)
		if err != nil {
			return nil, err
		}
		ops = append(ops, lowered...)
	}
	return ops, nil
}

func (c *Compiler) lowerOneAction(action string) ([]wep.Op, error) {
	call, ok := ParseCall(action)
	if !ok {
		return nil, fmt.Errorf("cannot read the action %q", action)
	}
	lowering, known := c.policy.Functions[strings.ToLower(call.Name)]
	if !known {
		return nil, fmt.Errorf("%s has no entry in the policy", call.Name)
	}
	if lowering == nil {
		c.drop(call.Raw)
		return []wep.Op{{Op: wep.OpNop, Source: call.Raw}}, nil
	}
	ops, err := lowering(c, call)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", call.Name, err)
	}
	for _, op := range ops {
		if op.Op == wep.OpNop {
			c.drop(op.Source)
		}
	}
	return ops, nil
}

// engineStopLabel is where a jump to one of GZDoom's own end-of-layer labels
// goes: a step of our own that stops the layer.
const engineStopLabel = "@enginestop"

// addEngineStop appends the step a jump to LightDone or Null lands on, if
// anything asked for it.
func (c *Compiler) addEngineStop(machine *wep.StateMachine) {
	if _, wanted := machine.Labels[engineStopLabel]; wanted {
		return
	}
	for _, step := range machine.Steps {
		for _, op := range step.Ops {
			if op.To == engineStopLabel {
				machine.Labels[engineStopLabel] = len(machine.Steps)
				machine.Steps = append(machine.Steps, wep.Step{
					Tics: 0,
					Ops:  []wep.Op{{Op: wep.OpStop, Source: "Goto LightDone"}},
				})
				return
			}
		}
	}
}

// resolveJumps turns the labels invented for relative jumps into real entries,
// and checks that every destination exists.
func (c *Compiler) resolveJumps(machine *wep.StateMachine) error {
	var missing []string
	seen := make(map[string]bool)

	note := func(target string) {
		if target == "" || seen[target] {
			return
		}
		seen[target] = true
		missing = append(missing, target)
	}

	for stepIndex := range machine.Steps {
		for opIndex, op := range machine.Steps[stepIndex].Ops {
			if op.To == "" {
				continue
			}
			resolved, ok := c.placeLabel(machine, op.To)
			if !ok {
				note(op.To)
				continue
			}
			machine.Steps[stepIndex].Ops[opIndex].To = resolved
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if c.strictLabels {
			return fmt.Errorf("jumps to labels that do not exist: %s", strings.Join(missing, ", "))
		}
		// The jump has nowhere to go in this actor, so it ends the layer, and
		// the compiled actor records that it was left dangling.
		c.defaulted("dangling jump: " + strings.Join(missing, ", "))
		c.retargetMissing(machine, missing)
	}
	return nil
}

// retargetMissing points jumps with no destination at a step that stops, and
// makes sure that step exists.
func (c *Compiler) retargetMissing(machine *wep.StateMachine, missing []string) {
	gone := make(map[string]bool, len(missing))
	for _, name := range missing {
		gone[name] = true
	}
	machine.Labels[engineStopLabel] = -1
	for stepIndex := range machine.Steps {
		for opIndex, op := range machine.Steps[stepIndex].Ops {
			if gone[op.To] {
				machine.Steps[stepIndex].Ops[opIndex].To = engineStopLabel
			}
		}
	}
	delete(machine.Labels, engineStopLabel)
	c.addEngineStop(machine)
}

// placeLabel resolves one destination, inventing a label for a frame offset or
// for a label plus an offset, and returns the name to store.
func (c *Compiler) placeLabel(machine *wep.StateMachine, target string) (string, bool) {
	if frameOrdinal, ok := strings.CutPrefix(target, "@frame"); ok {
		ordinal, err := strconv.Atoi(frameOrdinal)
		if err != nil || ordinal < 0 || ordinal >= len(c.frameSteps) {
			return "", false
		}
		name := strings.ToLower(target)
		machine.Labels[name] = c.frameSteps[ordinal]
		return name, true
	}
	if plus := strings.IndexByte(target, '+'); plus >= 0 {
		base := strings.ToLower(strings.TrimSpace(target[:plus]))
		offset, err := strconv.Atoi(strings.TrimSpace(target[plus+1:]))
		if err != nil {
			return "", false
		}
		start, ok := machine.Labels[base]
		if !ok {
			return "", false
		}
		ordinal := frameOrdinalOf(c.frameSteps, start) + offset
		if ordinal < 0 || ordinal >= len(c.frameSteps) {
			return "", false
		}
		name := fmt.Sprintf("@%s.%d", base, offset)
		machine.Labels[name] = c.frameSteps[ordinal]
		return name, true
	}
	lower := strings.ToLower(target)
	if _, ok := machine.Labels[lower]; !ok {
		return "", false
	}
	return lower, true
}

// frameOrdinalOf finds which frame a step index is, or the next frame after it
// when the step is a jump rather than a frame.
func frameOrdinalOf(frameSteps []int, stepIndex int) int {
	for ordinal, step := range frameSteps {
		if step >= stepIndex {
			return ordinal
		}
	}
	return len(frameSteps)
}
