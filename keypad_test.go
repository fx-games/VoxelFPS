package main

import (
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/platform/glfw"
	"github.com/memmaker/goray/quads"
)

func testKeypad(code string, at mgl32.Vec3) *Keypad {
	object := quads.NewGameObject("Keypad")
	object.Transform().SetPosition(at)
	return &Keypad{object: object, code: code}
}

// A keypad holds the door it is mounted beside and nothing else: the door
// within reach is locked, one across the room is not, and the right four digits
// open the one it holds.
func TestAKeypadHoldsTheDoorBesideIt(t *testing.T) {
	g := &Game{}
	beside := testDoor(doorSlide, mgl32.Vec3{4, 1, 4})
	elsewhere := testDoor(doorSwing, mgl32.Vec3{30, 1, 30})
	g.doors = []*Door{beside, elsewhere}
	g.keypads = []*Keypad{testKeypad("7412", mgl32.Vec3{4.8, 1, 4})}

	g.bindKeypads()
	if g.keypads[0].door != beside {
		t.Fatalf("keypad bound to %v, want the door beside it", g.keypads[0].door)
	}
	if !beside.locked {
		t.Error("the door the keypad is beside was left unlocked")
	}
	if elsewhere.locked {
		t.Error("a door across the room was locked by a keypad nowhere near it")
	}

	// Locked means locked: pressing the door itself does nothing at all.
	beside.Interact()
	g.updateDoors(1.0)
	if beside.openT != 0 {
		t.Fatalf("openT = %v, want a locked door to stay shut when pressed", beside.openT)
	}

	g.unlock(g.keypads[0])
	if beside.locked {
		t.Fatal("the door is still locked after its own code was accepted")
	}
	g.updateDoors(1.0)
	if beside.openT != 1 {
		t.Errorf("openT = %v, want the door to open as it unlocks", beside.openT)
	}
}

// A keypad with no code on it yet - one just dropped in the editor - holds
// nothing, and a rebind after the code is set does.
func TestAKeypadWithNoCodeHoldsNothing(t *testing.T) {
	g := &Game{}
	door := testDoor(doorSlide, mgl32.Vec3{4, 1, 4})
	g.doors = []*Door{door}
	pad := testKeypad("", mgl32.Vec3{4.8, 1, 4})
	g.keypads = []*Keypad{pad}

	g.bindKeypads()
	if door.locked {
		t.Error("a keypad with no code on it locked the door beside it")
	}
	g.setKeypadCode(pad, "1234")
	if !door.locked {
		t.Error("the door is still open after its keypad was given a code")
	}
}

// Reading a note learns every four-digit number written in it, which is the
// whole of how a code becomes known - a note carries writing, not a code field.
func TestReadingANoteLearnsTheCodesInIt(t *testing.T) {
	g := &Game{}
	g.learnCodes("the vault panel is still on its factory code, 7412. ops moved the archive to 2580.")
	for _, code := range []string{"7412", "2580"} {
		if !g.knownCodes[code] {
			t.Errorf("code %s was written on the note and not learned", code)
		}
	}
	if g.knownCodes["1111"] {
		t.Error("learned a code that was never written anywhere")
	}
}

// The keys: digits for a code, characters for a note's writing, and the shift
// key deciding case and nothing else.
func TestWhatAKeyTypes(t *testing.T) {
	if digit, ok := typedDigit(glfw.Key7); !ok || digit != "7" {
		t.Errorf("Key7 typed %q (%v), want 7", digit, ok)
	}
	if digit, ok := typedDigit(glfw.KeyKP3); !ok || digit != "3" {
		t.Errorf("KeyKP3 typed %q (%v), want 3", digit, ok)
	}
	if _, ok := typedDigit(glfw.KeyA); ok {
		t.Error("a letter typed a digit")
	}
	for _, want := range []struct {
		key       glfw.Key
		mods      glfw.ModifierKey
		character rune
	}{
		{glfw.KeyA, 0, 'a'},
		{glfw.KeyA, glfw.ModShift, 'A'},
		{glfw.Key4, 0, '4'},
		{glfw.KeySpace, 0, ' '},
		{glfw.KeyPeriod, 0, '.'},
	} {
		got, ok := typedRune(want.key, want.mods)
		if !ok || got != want.character {
			t.Errorf("key %v mods %v typed %q (%v), want %q", want.key, want.mods, got, ok, want.character)
		}
	}
	if _, ok := typedRune(glfw.KeyF5, 0); ok {
		t.Error("a function key typed a character")
	}
}

// The test level has to be solvable without being told the numbers: every
// keypad in it is opened by a code somebody wrote on a note lying in the level.
func TestTheTestLevelWritesItsOwnCodesDown(t *testing.T) {
	level, err := loadJSONMap(mapPathFor("keypads"))
	if err != nil {
		t.Fatal(err)
	}
	var written []string
	codes := map[string]bool{}
	for _, spawn := range level.Entities {
		if spawn.Entity.Text != "" {
			written = append(written, spawn.Entity.Text)
		}
		if spawn.Entity.Code != "" {
			codes[spawn.Entity.Code] = true
		}
	}
	if len(codes) < 2 {
		t.Fatalf("the level has %d keypad codes in it, want at least two", len(codes))
	}
	intel := strings.Join(written, "\n")
	for code := range codes {
		if !strings.Contains(intel, code) {
			t.Errorf("keypad code %s is written on no note in the level", code)
		}
	}
}
