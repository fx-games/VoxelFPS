package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/memmaker/voxelfps/platform/glfw"
)

func TestLoadKeyBindingsDefaultFileMatchesDefaults(t *testing.T) {
	bindings := loadKeyBindings(keyBindingsFile)
	for action, want := range defaultKeyBindings {
		if got := bindings[action]; got != want {
			t.Errorf("%s: keybindings.txt gave %v, default is %v", action, got, want)
		}
	}
}

func TestLoadKeyBindingsOverridesAndIgnoresBadLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.txt")
	content := "# comment\n\nJ => MoveForward\nnot a valid line\nZZZ => MoveLeft\nA => NotAnAction\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	bindings := loadKeyBindings(path)
	if bindings[ActionMoveForward] != glfw.KeyJ {
		t.Errorf("MoveForward: got %v, want KeyJ", bindings[ActionMoveForward])
	}
	if bindings[ActionMoveLeft] != defaultKeyBindings[ActionMoveLeft] {
		t.Errorf("MoveLeft should have kept its default after an unknown key name")
	}
}

func TestLoadKeyBindingsMissingFileUsesDefaults(t *testing.T) {
	bindings := loadKeyBindings(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	for action, want := range defaultKeyBindings {
		if bindings[action] != want {
			t.Errorf("%s: got %v, want default %v", action, bindings[action], want)
		}
	}
}

// Every action has to be in keybindings.txt, not only the ones somebody wanted
// moved. The file is how a player finds out what can be rebound at all, and an
// action that only exists in the Go defaults is one nobody can discover.
func TestEveryActionIsListedInTheDefaultFile(t *testing.T) {
	listed := map[string]bool{}
	text, err := os.ReadFile(keyBindingsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, action, found := strings.Cut(line, "=>"); found {
			listed[strings.TrimSpace(action)] = true
		}
	}
	for action := range defaultKeyBindings {
		if !listed[action] {
			t.Errorf("%s is bound in the defaults but missing from %s", action, keyBindingsFile)
		}
	}
}
