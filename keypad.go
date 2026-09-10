package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
)

// A locked door, Prey's way round: the door itself knows nothing about codes.
// A keypad stands beside it with four digits on it, and the door is shut until
// somebody types them. Where the digits come from is the other half - a note
// lying somewhere in the level with the number written on it - and a code that
// has been read is shown on the panel while it is open, so a level is a place
// to look for numbers rather than a place to write them down.

const (
	keypadMeshName = "keypad"
	noteMeshName   = "note"

	// codeLength is four because Prey's is, and because four is short enough to
	// carry across a room in your head and long enough to be worth finding.
	codeLength = 4

	// keypadReach is how far from a keypad the door it secures may be. A keypad
	// is mounted beside its own door; anything further away than the width of
	// the doorway is a different door.
	keypadReach = 3.0

	// keypadEmissiveStrength lights the panel's display. Far lower than the
	// torch's, because a keypad is a lit readout in a room rather than the
	// thing lighting the room - and only the display is bright and saturated
	// enough for the mesh shader's emissive pass to pick out, so the casing
	// around it stays dark. See tools/genpanels.
	keypadEmissiveStrength = float32(2.5)
)

// Keypad is one panel: the four digits it wants, the door it holds shut, and
// whether it has already been satisfied. entered is what has been typed into it
// so far, which only ever exists while its panel is open.
type Keypad struct {
	object   *quads.GameObject
	code     string
	door     *Door
	unlocked bool
	entered  string
	denied   bool
}

// Note is a sheet of paper in the level with a line of writing on it. Reading
// one learns every four-digit number in that writing and puts it in the
// inventory, so what a level told you once is somewhere to look it up again.
type Note struct {
	object *quads.GameObject
	text   string
	taken  bool
}

func (g *Game) spawnKeypad(object *quads.GameObject, code string) {
	g.keypads = append(g.keypads, &Keypad{object: object, code: code})
}

func (g *Game) spawnNote(object *quads.GameObject, text string) {
	g.notes = append(g.notes, &Note{object: object, text: text})
}

// keypadFor and noteFor are doorFor's siblings: the thing behind a GameObject a
// raycast handed back, or nil. Linear scans over a level's handful of each,
// run on a keypress rather than on a frame.
func (g *Game) keypadFor(object *quads.GameObject) *Keypad {
	for _, pad := range g.keypads {
		if pad.object == object {
			return pad
		}
	}
	return nil
}

func (g *Game) noteFor(object *quads.GameObject) *Note {
	for _, note := range g.notes {
		if note.object == object {
			return note
		}
	}
	return nil
}

// bindKeypads works out which door each keypad secures - the nearest one within
// keypadReach - and locks the ones that are spoken for. It runs once the whole
// level is standing rather than as each keypad spawns, because a keypad may
// well load before its door does, and again whenever the editor drops another
// door or keypad, because "the nearest door" is a fact about the level that a
// single placement can change.
//
// Every door is unlocked first, so a keypad that has been moved away from the
// door it used to hold does not leave it shut for ever.
func (g *Game) bindKeypads() {
	// The editor deletes a door or a keypad by killing its object, so a rebind
	// is also where the lists let go of what is no longer standing. Notes are
	// not swept with them: a note the player has taken is dead in the world and
	// still in the inventory, which is the point of taking it.
	g.doors = slices.DeleteFunc(g.doors, func(door *Door) bool { return !door.object.IsAlive() })
	g.keypads = slices.DeleteFunc(g.keypads, func(pad *Keypad) bool { return !pad.object.IsAlive() })
	for _, door := range g.doors {
		door.locked = false
	}
	for _, pad := range g.keypads {
		pad.door = g.nearestDoor(pad.object.Transform().GetPosition())
		if pad.door != nil && pad.code != "" && !pad.unlocked {
			pad.door.locked = true
		}
	}
}

// nearestDoor is the door closest to pos, or nil when the closest one is
// further off than a keypad can be from its own door.
func (g *Game) nearestDoor(pos mgl32.Vec3) *Door {
	var nearest *Door
	closest := float32(keypadReach)
	for _, door := range g.doors {
		away := door.closedPos.Sub(pos).Len()
		if away <= closest {
			nearest, closest = door, away
		}
	}
	return nearest
}

// codePattern is how a note gives a code away: four digits in a row, anywhere
// in what it says. That is the whole mechanism - a note does not carry a code
// field the level author has to keep in step with its own writing, it carries
// writing, and the number in the writing is the number.
var codePattern = regexp.MustCompile(`\d{4}`)

// learnCodes remembers every code written in text.
func (g *Game) learnCodes(text string) {
	if g.knownCodes == nil {
		g.knownCodes = map[string]bool{}
	}
	for _, code := range codePattern.FindAllString(text, -1) {
		g.knownCodes[code] = true
	}
}

// readNote is the E key on a note: it learns what the note says, takes it, and
// puts it up to be read. Taking it is what makes it an item - it leaves the
// floor and turns up in the inventory, where it can be read again.
func (g *Game) readNote(note *Note) {
	g.learnCodes(note.text)
	if !note.taken {
		note.taken = true
		note.object.Kill()
	}
	g.openPanel(func() string {
		return noteTitle(note.text) + "\n\n" + note.text + "\n\n[ESC]"
	}, nil)
}

// noteTitle is what the inventory calls a note, which is the start of what it
// says. A note has no name of its own to give: it is a scrap of paper, and the
// only thing that tells two of them apart is their writing.
func noteTitle(text string) string {
	title := strings.TrimSpace(text)
	if title == "" {
		return "Blank note"
	}
	if len(title) > 28 {
		title = strings.TrimSpace(title[:28]) + "..."
	}
	return title
}

// openKeypad is the E key on a keypad: Prey's panel, which is where the digits
// go in and - this is the whole reason notes exist - where a code that has
// already been read is printed, so finding the number once is finding it for
// good.
func (g *Game) openKeypad(pad *Keypad) {
	pad.entered = ""
	pad.denied = false
	g.openPanel(func() string { return g.keypadDisplay(pad) }, func(key glfw.Key, mods glfw.ModifierKey) bool {
		digit, isDigit := typedDigit(key)
		switch {
		case isDigit && len(pad.entered) < codeLength:
			pad.denied = false
			pad.entered += digit
		case key == glfw.KeyBackspace && len(pad.entered) > 0:
			pad.denied = false
			pad.entered = pad.entered[:len(pad.entered)-1]
		default:
			return false
		}
		if len(pad.entered) < codeLength {
			return false
		}
		// Four digits is the whole answer, so it is checked without waiting for
		// an enter key - the same as the real thing, and there is nothing else
		// the fourth digit could have meant.
		if pad.entered != pad.code {
			pad.entered, pad.denied = "", true
			return false
		}
		g.unlock(pad)
		g.Toast("Accepted")
		return true
	})
}

// keypadDisplay is what the panel says: the digits so far, the code if it has
// been read somewhere, and what the keys do.
func (g *Game) keypadDisplay(pad *Keypad) string {
	shown := pad.entered + strings.Repeat("_", codeLength-len(pad.entered))
	lines := []string{"KEYPAD", "", strings.Join(strings.Split(shown, ""), " ")}
	switch {
	case pad.denied:
		lines = append(lines, "", "DENIED")
	case g.knownCodes[pad.code]:
		lines = append(lines, "", "code on file: "+pad.code)
	default:
		lines = append(lines, "", "code unknown")
	}
	return strings.Join(append(lines, "", "0-9 enter   BACKSPACE   ESC"), "\n")
}

// unlock opens the door a keypad holds and leaves it open: a code entered is a
// door dealt with, not a door that shuts again behind you. A keypad standing
// beside no door accepts its code and opens nothing, which is what the editor's
// own line about it says will happen - see keypadStatus.
func (g *Game) unlock(pad *Keypad) {
	pad.unlocked = true
	if pad.door == nil {
		return
	}
	pad.door.locked = false
	pad.door.Interact()
}

// setKeypadCode is the editor writing a code onto a keypad. It goes into both
// the live keypad and the entity description a save writes out, because those
// are the two places a code has to be for it to survive the run.
func (g *Game) setKeypadCode(pad *Keypad, code string) {
	pad.code = code
	pad.unlocked = false
	if record := g.placedRecord(pad.object); record != nil {
		record.entity.Code = code
	}
	g.bindKeypads()
}

// setNoteText is the same for a note's writing.
func (g *Game) setNoteText(note *Note, text string) {
	note.text = text
	if record := g.placedRecord(note.object); record != nil {
		record.entity.Text = text
	}
}

// editEntityUnderCrosshair is the E key while the editor is on: the same key
// that uses a thing in the game gives it what it says while building one. A
// keypad is asked for four digits and a note for a line of writing, in a panel
// standing where the thing does - which is the only place either of them is
// worth authoring, since what a keypad is for is which door it is beside.
//
// It reports whether it took the key. Anything else under the crosshair is left
// alone, so a door still opens when it is pressed in the editor - which is how
// the author checks that the doorway just bored is one the leaf fits.
func (g *Game) editEntityUnderCrosshair(object *quads.GameObject) bool {
	if pad := g.keypadFor(object); pad != nil {
		typed := pad.code
		g.openPanel(func() string {
			shown := typed + strings.Repeat("_", max(0, codeLength-len(typed)))
			return "SET KEYPAD CODE\n\n" + strings.Join(strings.Split(shown, ""), " ") +
				"\n\n0-9 type   BACKSPACE   ESC saves"
		}, func(key glfw.Key, mods glfw.ModifierKey) bool {
			if digit, isDigit := typedDigit(key); isDigit && len(typed) < codeLength {
				typed += digit
			} else if key == glfw.KeyBackspace && len(typed) > 0 {
				typed = typed[:len(typed)-1]
			}
			g.setKeypadCode(pad, typed)
			return false
		})
		return true
	}
	if note := g.noteFor(object); note != nil {
		typed := note.text
		g.openPanel(func() string {
			return "SET NOTE TEXT\n\n" + typed + "_" + "\n\nTYPE   BACKSPACE   ESC saves"
		}, func(key glfw.Key, mods glfw.ModifierKey) bool {
			if character, typedOne := typedRune(key, mods); typedOne {
				typed += string(character)
			} else if key == glfw.KeyBackspace && len(typed) > 0 {
				typed = typed[:len(typed)-1]
			}
			g.setNoteText(note, typed)
			return false
		})
		return true
	}
	return false
}

// typedDigit is the number row and the numeric keypad as one digit.
func typedDigit(key glfw.Key) (string, bool) {
	switch {
	case key >= glfw.Key0 && key <= glfw.Key9:
		return string(rune('0' + key - glfw.Key0)), true
	case key >= glfw.KeyKP0 && key <= glfw.KeyKP9:
		return string(rune('0' + key - glfw.KeyKP0)), true
	}
	return "", false
}

// typedRune is what a key means as a character. GLFW's key codes for the
// letters, the digits and the printable punctuation are their own ASCII codes,
// so this is a range test rather than a table - the shift key is the only thing
// that has to be decided, and it decides case and nothing else.
//
// ponytail: no character callback, so no dead keys, no AltGr and no layout
// beyond US. A note is a scrap of paper with a number on it; when one needs an
// umlaut, wire GLFW's SetCharCallback through InputReceiver.
func typedRune(key glfw.Key, mods glfw.ModifierKey) (rune, bool) {
	shift := mods&glfw.ModShift != 0
	switch {
	case key >= glfw.KeyA && key <= glfw.KeyZ:
		if shift {
			return rune(key), true
		}
		return rune(key) + 'a' - 'A', true
	case key >= glfw.Key0 && key <= glfw.Key9 && !shift:
		return rune(key), true
	case key == glfw.KeySpace, key == glfw.KeyApostrophe, key == glfw.KeyComma,
		key == glfw.KeyMinus, key == glfw.KeyPeriod, key == glfw.KeySlash,
		key == glfw.KeySemicolon, key == glfw.KeyEqual:
		return rune(key), true
	}
	return 0, false
}

// panel is a box of text on the widget stack with one function behind every
// key. It is what the keypad, a note being read and the editor's own way of
// typing into either of them all are: a GuiMessageBox that is asked what it
// says again after every keystroke. Nothing here is a new widget, only a new
// arrangement of one - see quads.GuiDialogue, which is the same idea.
type panel struct {
	game    *Game
	box     *quads.GuiMessageBox
	say     func() string
	onKey   func(glfw.Key, glfw.ModifierKey) bool // true closes the panel
	closed  bool
	restore bool
}

// openPanel puts one up. onKey may be nil, for a panel that only says something
// and waits to be dismissed.
func (g *Game) openPanel(say func() string, onKey func(glfw.Key, glfw.ModifierKey) bool) *panel {
	p := &panel{
		game:    g,
		box:     quads.NewGuiMessageBox(g.UiContext()),
		say:     say,
		onKey:   onKey,
		restore: g.setupModal(),
	}
	p.box.SetAlign(quads.AlignCenter)
	guiWidth, guiHeight := g.GuiSize()
	p.HandleResize(guiWidth, guiHeight, g.UIScale)
	g.pushToWidgetStack(p)
	return p
}

func (p *panel) close() {
	if p.closed {
		return
	}
	p.closed = true
	p.game.captureMouse(p.restore)
}

func (p *panel) ShouldClose() bool { return p.closed }

func (p *panel) HandleResize(screenWidth, screenHeight int, screenCoordScale mgl32.Vec2) {
	p.box.Show(p.say(), screenWidth, screenHeight)
}

func (p *panel) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
	if action != glfw.Press && action != glfw.Repeat {
		return true
	}
	switch {
	case key == glfw.KeyEscape:
		p.close()
	case p.onKey == nil && (key == glfw.KeyEnter || key == glfw.KeyKPEnter || key == glfw.KeyE):
		p.close()
	case p.onKey != nil && p.onKey(key, mods):
		p.close()
	}
	if !p.closed {
		guiWidth, guiHeight := p.game.GuiSize()
		p.box.Show(p.say(), guiWidth, guiHeight)
	}
	// Every key, taken: a panel is modal, and a movement key that leaked past
	// it would walk the player away from the keypad they are typing into.
	return true
}

// The mouse does nothing to a panel, but it must not reach the world either.
func (p *panel) HandleMousePosEvents(xpos, ypos float64) bool { return true }
func (p *panel) HandleMouseButtonEvents(xpos, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
	return true
}
func (p *panel) HandleScrollEvents(xoff, yoff float64) bool   { return true }
func (p *panel) HandleDrop(names []string, x, y float64) bool { return false }
func (p *panel) Draw()                                        { p.box.Draw() }

// noteItems is the notes the player is carrying, as inventory rows. They have
// no picture - a note is a piece of paper, and the sheet the world draws is the
// same sheet for all of them - so what tells them apart in the list is their
// own writing, which is also what the detail pane holds.
func (g *Game) noteItems() []quads.InventoryItem {
	var items []quads.InventoryItem
	for _, note := range g.notes {
		if !note.taken {
			continue
		}
		items = append(items, quads.InventoryItem{
			Name:   noteTitle(note.text),
			Detail: wrapLines(note.text, noteDetailWidth),
			Icon:   -1,
			Tab:    inventoryItemsTab,
		})
	}
	return items
}

// noteDetailWidth is how wide the inventory's detail pane is in characters.
// Narrower than the pane really is, so a long note wraps rather than being cut
// off by it.
const noteDetailWidth = 40

// wrapLines breaks text into lines of at most width characters, on spaces.
func wrapLines(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return []string{"(blank)"}
	}
	return lines
}

// keypadStatus is the editor HUD's line about whatever the crosshair is on, so
// a keypad's code and a note's writing are visible while building rather than
// only once a panel has been opened on them.
func (g *Game) keypadStatus(object *quads.GameObject) string {
	if pad := g.keypadFor(object); pad != nil {
		code := pad.code
		if code == "" {
			code = "unset"
		}
		door := "no door in reach"
		if pad.door != nil {
			door = "holds the door beside it"
		}
		return fmt.Sprintf("keypad code %s, %s   E to set", code, door)
	}
	if note := g.noteFor(object); note != nil {
		return "note: " + noteTitle(note.text) + "   E to write"
	}
	return ""
}
