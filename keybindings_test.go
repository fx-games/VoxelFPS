package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
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
