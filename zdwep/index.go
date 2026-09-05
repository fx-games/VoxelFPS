package zdwep

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The indexer is deliberately neutral: it parses every actor it finds into
// plain data and resolves nothing. Inheritance, properties, sounds and sprites
// are all somebody else's problem, which is what lets one pass over the mod
// serve weapons, the ammo they use, the counters they branch on and the actors
// they spawn.

// StateLine is one line inside a States block, either a frame or a jump.
type StateLine struct {
	Sprite   string
	Frames   string
	Duration int // in tics, -1 means "stay here"
	// DurationMax is set when the script wrote random(a, b); the engine picks
	// per entry. It equals Duration for a fixed duration.
	DurationMax int
	Bright      bool
	OffsetX     int
	OffsetY     int
	// Action is the call written after the frame, verbatim, eg.
	// `A_FireBullets (4, 0.5, 10, 5, "FDBulletPuff", 0)`.
	Action string
	// Flow is the lowercased control flow keyword on a line that carries one:
	// goto, loop, stop, wait or fail. Frames leave it empty.
	Flow string
	// FlowTarget and FlowOffset carry `Goto Label+2`.
	FlowTarget string
	FlowOffset int

	SourceFile string
	Line       int
}

// IsFrame reports whether the line shows something, as opposed to jumping.
func (sl StateLine) IsFrame() bool { return sl.Flow == "" }

// StateBlock is an actor's States block. Doom states fall through from one
// label into the next, so the lines are one flat list and labels are offsets
// into it, exactly as ZDoom stores them. Splitting them per label would lose
// the fallthrough that scripts rely on.
type StateBlock struct {
	Lines      []StateLine
	Labels     map[string]int
	LabelOrder []string
}

// Label returns the index the label starts at.
func (sb StateBlock) Label(name string) (int, bool) {
	index, ok := sb.Labels[strings.ToLower(name)]
	return index, ok
}

// Actor is one DECORATE actor, unresolved.
type Actor struct {
	Name     string
	Parent   string
	Replaces string
	// Properties keys are lowercased, eg. "weapon.ammotype". Values are the
	// raw text after the key.
	Properties map[string]string
	PropOrder  []string
	Flags      map[string]bool
	States     StateBlock

	SourceFile string
	Line       int
}

// Property reads a property by its case-insensitive name.
func (a Actor) Property(name string) (string, bool) {
	value, ok := a.Properties[strings.ToLower(name)]
	return value, ok
}

// Index is every actor in a mod, keyed by lowercased name.
type Index struct {
	Actors map[string]*Actor
	// Files lists the DECORATE files that were read, in include order.
	Files []string
}

// Actor looks an actor up by its case-insensitive name.
func (idx *Index) Actor(name string) (*Actor, bool) {
	a, ok := idx.Actors[strings.ToLower(name)]
	return a, ok
}

// Ancestry walks an actor's parent chain, nearest first, starting with the
// actor itself. It stops at an unknown parent, which is normal: the chain
// usually ends in an engine class such as Weapon or Ammo that no DECORATE file
// defines.
func (idx *Index) Ancestry(name string) []*Actor {
	var chain []*Actor
	seen := make(map[string]bool)
	for current := strings.ToLower(name); current != ""; {
		if seen[current] {
			break // a cycle in the mod, not our problem to fix, only to survive
		}
		seen[current] = true
		actor, ok := idx.Actors[current]
		if !ok {
			break
		}
		chain = append(chain, actor)
		current = strings.ToLower(actor.Parent)
	}
	return chain
}

// InheritsFrom reports whether an actor's parent chain reaches the named class,
// including through parents the mod never defines. This is how a weapon is
// recognised: the chain reaches "Weapon", rather than the parent's name merely
// containing the word.
func (idx *Index) InheritsFrom(name, ancestor string) bool {
	ancestor = strings.ToLower(ancestor)
	seen := make(map[string]bool)
	for current := strings.ToLower(name); current != ""; {
		if current == ancestor {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		actor, ok := idx.Actors[current]
		if !ok {
			return false
		}
		current = strings.ToLower(actor.Parent)
	}
	return false
}

// BuildIndex reads the mod's DECORATE lump, follows its includes and parses
// every actor it can reach.
func BuildIndex(src *Source) (*Index, error) {
	idx := &Index{Actors: make(map[string]*Actor)}
	root, ok := src.Resolve("DECORATE")
	if !ok {
		return nil, fmt.Errorf("no DECORATE lump in this source")
	}
	visited := make(map[string]bool)
	if err := indexFile(src, idx, root, visited); err != nil {
		return nil, err
	}
	return idx, nil
}

var includeRegexp = regexp.MustCompile(`(?i)^#include\s+"?([^"\s]+)"?`)

func indexFile(src *Source, idx *Index, name string, visited map[string]bool) error {
	real, ok := src.Resolve(name)
	if !ok {
		return fmt.Errorf("#include %q: not in this source", name)
	}
	if visited[real] {
		return nil
	}
	visited[real] = true
	idx.Files = append(idx.Files, real)

	data, err := src.ReadFile(real)
	if err != nil {
		return fmt.Errorf("%s: %w", real, err)
	}
	text := stripBlockComments(string(data))

	includes, actors, err := parseDecorateFile(real, text)
	if err != nil {
		return err
	}
	for _, actor := range actors {
		// A later definition of the same name wins, which is what a mod that
		// overrides one of its own actors expects.
		idx.Actors[strings.ToLower(actor.Name)] = actor
	}
	for _, include := range includes {
		if err := indexFile(src, idx, include, visited); err != nil {
			return err
		}
	}
	return nil
}

// stripBlockComments removes /* */ comments while keeping the line count, so
// error messages still point at the right line.
func stripBlockComments(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	inComment := false
	for i := 0; i < len(text); i++ {
		if !inComment && i+1 < len(text) && text[i] == '/' && text[i+1] == '*' {
			inComment = true
			i++
			continue
		}
		if inComment {
			if i+1 < len(text) && text[i] == '*' && text[i+1] == '/' {
				inComment = false
				i++
				continue
			}
			if text[i] == '\n' {
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteByte(text[i])
	}
	return out.String()
}

var actorHeadRegexp = regexp.MustCompile(`(?i)^actor\s+([\w.]+)\s*(?::\s*([\w.]+))?\s*(?:replaces\s+([\w.]+))?\s*(-?\d+)?\s*(\{\s*\}?)?\s*$`)

// parseDecorateFile pulls the includes and actors out of one file.
func parseDecorateFile(fileName, text string) (includes []string, actors []*Actor, err error) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	lineNumber := 0
	var current *Actor
	// depth counts braces inside the actor body: 1 is the body, 2 is the States
	// block, 3 and up are the anonymous action blocks a state line can carry.
	depth := 0
	inStates := false
	// pendingLabels are labels written before the frame they point at, which is
	// every label, and several in a row when a script aliases one to another.
	var pendingLabels []string

	for scanner.Scan() {
		lineNumber++
		line := stripLineComment(scanner.Text())
		if line == "" {
			continue
		}

		if current == nil {
			if match := includeRegexp.FindStringSubmatch(line); match != nil {
				includes = append(includes, match[1])
				continue
			}
			head := actorHeadRegexp.FindStringSubmatch(line)
			if head == nil {
				continue // constants, damagetype definitions, anything else
			}
			current = &Actor{
				Name:       head[1],
				Parent:     head[2],
				Replaces:   head[3],
				Properties: make(map[string]string),
				Flags:      make(map[string]bool),
				States:     StateBlock{Labels: make(map[string]int)},
				SourceFile: fileName,
				Line:       lineNumber,
			}
			// The body's braces may sit on this line, on the next one, or be
			// an empty pair for an actor that only inherits.
			switch strings.ReplaceAll(head[5], " ", "") {
			case "{}":
				actors = append(actors, current)
				current = nil
			case "{":
				depth = 1
			}
			continue
		}

		if depth == 0 {
			if strings.HasPrefix(line, "{") {
				depth = 1
				line = strings.TrimSpace(line[1:])
				if line == "" {
					continue
				}
			} else {
				continue
			}
		}

		// A closing brace at the body's own depth ends the actor; inside the
		// States block it ends the block.
		if line == "}" {
			depth--
			if depth == 1 {
				inStates = false
				pendingLabels = nil
			}
			if depth == 0 {
				actors = append(actors, current)
				current = nil
			}
			continue
		}

		if !inStates {
			lower := strings.ToLower(line)
			if lower == "states" || lower == "states{" || lower == "states {" {
				inStates = true
				if strings.HasSuffix(line, "{") {
					depth = 2
				}
				continue
			}
			if inStatesOpening(line, &depth, &inStates) {
				continue
			}
			parseActorBodyLine(current, line)
			continue
		}

		if line == "{" {
			depth++
			continue
		}

		if label, ok := stateLabel(line); ok {
			pendingLabels = append(pendingLabels, label)
			continue
		}

		state, ok := parseStateLine(fileName, lineNumber, line)
		if !ok {
			continue
		}
		index := len(current.States.Lines)
		for _, label := range pendingLabels {
			lower := strings.ToLower(label)
			if _, exists := current.States.Labels[lower]; !exists {
				current.States.LabelOrder = append(current.States.LabelOrder, label)
			}
			current.States.Labels[lower] = index
		}
		pendingLabels = nil
		current.States.Lines = append(current.States.Lines, state)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", fileName, err)
	}
	if current != nil {
		// An actor whose closing brace is missing still holds everything we
		// read, and a mod that malformed will fail later on something specific.
		actors = append(actors, current)
	}
	return includes, actors, nil
}

// inStatesOpening handles `States` and its brace arriving on separate lines.
func inStatesOpening(line string, depth *int, inStates *bool) bool {
	if *inStates && strings.HasPrefix(line, "{") {
		*depth = 2
		return true
	}
	return false
}

func stripLineComment(raw string) string {
	line := raw
	if index := strings.Index(line, "//"); index >= 0 {
		// A // inside a string is not a comment; the only strings in DECORATE
		// that can hold one are sound and class names, which never do.
		line = line[:index]
	}
	return strings.TrimSpace(line)
}

var labelRegexp = regexp.MustCompile(`^([A-Za-z_][\w.]*)\s*:\s*$`)

func stateLabel(line string) (string, bool) {
	match := labelRegexp.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func parseActorBodyLine(actor *Actor, line string) {
	switch line[0] {
	case '+':
		actor.Flags[strings.ToLower(strings.TrimSpace(line[1:]))] = true
		return
	case '-':
		actor.Flags[strings.ToLower(strings.TrimSpace(line[1:]))] = false
		return
	}
	fields := strings.SplitN(line, " ", 2)
	key := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(fields[0]), ";"))
	if key == "" {
		return
	}
	value := ""
	if len(fields) == 2 {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(fields[1]), ";"))
	}
	if _, exists := actor.Properties[key]; !exists {
		actor.PropOrder = append(actor.PropOrder, key)
	}
	actor.Properties[key] = value
}

var (
	flowRegexp = regexp.MustCompile(`(?i)^(goto|loop|stop|wait|fail)\b\s*(.*)$`)
	// gotoTargetRegexp splits `Super::Ready+2` into its parts.
	gotoTargetRegexp = regexp.MustCompile(`^((?:\w+::)?[\w.]+)\s*(?:\+\s*(\d+))?`)
	stateHeadRegexp  = regexp.MustCompile(`^("[^"]*"|[A-Za-z0-9_#\-\[\]\\]{1,8})\s+("[^"]*"|[A-Za-z0-9_#\[\]\\]+)\s+(-?\d+|(?i:random)\s*\(\s*-?\d+\s*,\s*-?\d+\s*\))\s*(.*)$`)
	randomRegexp     = regexp.MustCompile(`(?i)^random\s*\(\s*(-?\d+)\s*,\s*(-?\d+)\s*\)$`)
	offsetRegexp     = regexp.MustCompile(`(?i)^offset\s*\(\s*(-?\d+)\s*,\s*(-?\d+)\s*\)`)
	lightRegexp      = regexp.MustCompile(`(?i)^light\s*\([^)]*\)`)
)

// parseStateLine reads one line of a States block. A line it cannot read is
// skipped rather than fatal: the indexer runs over every actor in the mod,
// including ones we will never emit, and one unreadable line in an actor nobody
// asked for must not stop the migration. Lines in actors we do emit are checked
// again during lowering, where an unknown one is an error.
func parseStateLine(fileName string, lineNumber int, line string) (StateLine, bool) {
	if match := flowRegexp.FindStringSubmatch(line); match != nil {
		state := StateLine{
			Flow:       strings.ToLower(match[1]),
			SourceFile: fileName,
			Line:       lineNumber,
		}
		if state.Flow == "goto" {
			target := gotoTargetRegexp.FindStringSubmatch(strings.TrimSpace(match[2]))
			if target == nil {
				return StateLine{}, false
			}
			state.FlowTarget = target[1]
			if target[2] != "" {
				state.FlowOffset, _ = strconv.Atoi(target[2])
			}
		}
		return state, true
	}

	head := stateHeadRegexp.FindStringSubmatch(line)
	if head == nil {
		return StateLine{}, false
	}
	state := StateLine{
		Sprite:     strings.Trim(head[1], `"`),
		Frames:     strings.Trim(head[2], `"`),
		SourceFile: fileName,
		Line:       lineNumber,
	}
	if random := randomRegexp.FindStringSubmatch(head[3]); random != nil {
		state.Duration, _ = strconv.Atoi(random[1])
		state.DurationMax, _ = strconv.Atoi(random[2])
	} else {
		state.Duration, _ = strconv.Atoi(head[3])
		state.DurationMax = state.Duration
	}

	rest := strings.TrimSpace(head[4])
	for rest != "" {
		lower := strings.ToLower(rest)
		switch {
		case strings.HasPrefix(lower, "bright"):
			state.Bright = true
			rest = strings.TrimSpace(rest[len("bright"):])
		case strings.HasPrefix(lower, "fast"):
			rest = strings.TrimSpace(rest[len("fast"):])
		case strings.HasPrefix(lower, "slow"):
			rest = strings.TrimSpace(rest[len("slow"):])
		case strings.HasPrefix(lower, "nodelay"):
			rest = strings.TrimSpace(rest[len("nodelay"):])
		case strings.HasPrefix(lower, "canraise"):
			rest = strings.TrimSpace(rest[len("canraise"):])
		case offsetRegexp.MatchString(rest):
			match := offsetRegexp.FindStringSubmatch(rest)
			state.OffsetX, _ = strconv.Atoi(match[1])
			state.OffsetY, _ = strconv.Atoi(match[2])
			rest = strings.TrimSpace(rest[len(match[0]):])
		case lightRegexp.MatchString(rest):
			rest = strings.TrimSpace(rest[len(lightRegexp.FindString(rest)):])
		default:
			state.Action = strings.TrimSuffix(strings.TrimSpace(rest), ";")
			rest = ""
		}
	}
	return state, true
}
