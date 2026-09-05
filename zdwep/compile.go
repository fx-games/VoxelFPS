package zdwep

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/goray/wep"
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
	return actor.Flags["missile"]
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
	for _, actor := range c.idx.Ancestry(name) {
		if value, set := actor.Flags[strings.ToLower(flag)]; set {
			return value
		}
	}
	return false
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

	previousSprite, previousFrame := "", byte('A')
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

		sprite := line.Sprite
		if sprite == "####" || sprite == "----" {
			sprite = previousSprite
		}
		frames := expandFrames(line.Frames)
		for _, frame := range frames {
			if frame == '#' || frame == '-' {
				frame = rune(previousFrame)
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
				// A frame letter is a letter whichever case it is written in,
				// and mods write both. Uppercasing it here keeps one name for
				// one picture, in the atlas and in the shared store alike.
				step.Sprite = strings.ToUpper(fmt.Sprintf("%s%c0", sprite, frame))
				c.useSprite(step.Sprite)
			}
			ops, err := c.lowerAction(line)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s:%d: %s", line.SourceFile, line.Line, err))
			}
			step.Ops = ops
			c.frameSteps = append(c.frameSteps, len(machine.Steps))
			machine.Steps = append(machine.Steps, step)
			previousSprite, previousFrame = sprite, byte(frame)
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

func (c *Compiler) lowerFlowLine(line StateLine) (wep.Step, error) {
	step := wep.Step{Tics: 0}
	switch line.Flow {
	case "loop":
		step.Ops = []wep.Op{{Op: wep.OpLoop, Source: "Loop"}}
	case "stop":
		step.Ops = []wep.Op{{Op: wep.OpStop, Source: "Stop"}}
	case "wait":
		step.Ops = []wep.Op{{Op: wep.OpWait, Source: "Wait"}}
	case "fail":
		step.Ops = []wep.Op{{Op: wep.OpStop, Source: "Fail"}}
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

// lowerAction turns a frame's action call into ops, through the policy.
func (c *Compiler) lowerAction(line StateLine) ([]wep.Op, error) {
	if strings.TrimSpace(line.Action) == "" {
		return nil, nil
	}
	call, ok := ParseCall(line.Action)
	if !ok {
		return nil, fmt.Errorf("cannot read the action %q", line.Action)
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
