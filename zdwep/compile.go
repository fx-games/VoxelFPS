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

	// frameSteps maps a frame's ordinal within the actor's states to the step
	// it became. ZDoom's numeric jump offsets count frames and skip the goto
	// and loop directives between them, so a jump of 2 needs this rather than
	// a step index.
	frameSteps []int
	// currentFrame is the ordinal of the frame being lowered, which is where a
	// relative jump counts from.
	currentFrame int

	neededItems  map[string]bool
	neededActors map[string]bool
	neededSounds map[string]bool
	neededLabels map[string]bool
	// sprites is every atlas frame the compiled actor asks for, in first use
	// order, which keeps the packed atlas stable between runs.
	sprites     []string
	spriteSeen  map[string]bool
	dropped     []string
	droppedSeen map[string]bool
}

func newCompiler(idx *Index, policy Policy) *Compiler {
	return &Compiler{
		idx:          idx,
		policy:       policy,
		neededItems:  make(map[string]bool),
		neededActors: make(map[string]bool),
		neededSounds: make(map[string]bool),
		neededLabels: make(map[string]bool),
		spriteSeen:   make(map[string]bool),
		droppedSeen:  make(map[string]bool),
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

// eval resolves a DECORATE expression against the mod's configuration.
func (c *Compiler) eval(text string) (Range, error) {
	return EvalExpr(text, func(name string, args []Arg) (Range, bool) {
		if strings.EqualFold(name, "CallACS") && len(args) == 1 && args[0].IsStr {
			value, known := c.policy.Settings.ACS[strings.ToLower(args[0].Str)]
			if !known {
				return Range{}, false
			}
			return constant(value), true
		}
		// ZDoom's flag words read as zero here: the flags that matter are read
		// from the argument's text by the lowering that cares about them.
		if strings.HasPrefix(strings.ToUpper(name), "FPF_") ||
			strings.HasPrefix(strings.ToUpper(name), "WRF_") ||
			strings.HasPrefix(strings.ToUpper(name), "CHAN_") ||
			strings.HasPrefix(strings.ToUpper(name), "TIF_") ||
			strings.HasPrefix(strings.ToUpper(name), "CBF_") {
			return constant(channelConstant(name)), true
		}
		return Range{}, false
	})
}

// channelConstant gives ZDoom's sound channel names their numbers. Anything
// else in that family reads as zero, which is what a flag word contributes to
// an argument we take the text of instead.
func channelConstant(name string) float64 {
	switch strings.ToUpper(name) {
	case "CHAN_WEAPON":
		return 1
	case "CHAN_VOICE":
		return 2
	case "CHAN_ITEM":
		return 3
	case "CHAN_BODY":
		return 4
	}
	return 0
}

func (c *Compiler) evalInt(text string) (int, error) {
	value, err := c.eval(text)
	if err != nil {
		return 0, err
	}
	return int(value.Value()), nil
}

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
				step.Sprite = fmt.Sprintf("%s%c0", strings.ToUpper(sprite), frame)
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
		return fmt.Errorf("jumps to labels that do not exist: %s", strings.Join(missing, ", "))
	}
	return nil
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
