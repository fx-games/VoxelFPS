package main

import (
	"bufio"
	"fmt"
	"github.com/go-gl/glfw/v3.3/glfw"
	"log"
	"maps"
	"os"
	"strings"
)

// keyBindingsFile lives next to the executable, in the same spirit as
// assets/weapons/keys.txt: plain text, hand-edited, one binding per line. Delete it
// or leave a line out and that action falls back to its default below.
const keyBindingsFile = "keybindings.txt"

// Action names for every keyboard action in the game. The number-row keys
// (weapon select 1-9, editor shape 1-5) stay hardcoded in input.go: they're
// slot numbers, not independently bindable actions.
const (
	ActionMoveForward  = "MoveForward"
	ActionMoveBackward = "MoveBackward"
	ActionMoveLeft     = "MoveLeft"
	ActionMoveRight    = "MoveRight"
	ActionCrouch       = "Crouch"
	ActionJump         = "Jump"

	ActionReload         = "Reload"
	ActionPreviousWeapon = "PreviousWeapon"
	ActionNextWeapon     = "NextWeapon"
	ActionInteract       = "Interact"

	ActionOpenInventory      = "OpenInventory"
	ActionOpenEscapeMenu     = "OpenEscapeMenu"
	ActionOpenWizardMenu     = "OpenWizardMenu"
	ActionToggleMouseCapture = "ToggleMouseCapture"

	ActionToggleSpawnMode         = "ToggleSpawnMode"
	ActionChangeDecorationTexture = "ChangeDecorationTexture"
	ActionToggleEditorMode        = "ToggleEditorMode"
	ActionOpenCubeTextureMapper   = "OpenCubeTextureMapper"
	ActionWireframeOff            = "WireframeOff"
	ActionWireframeOn             = "WireframeOn"
	ActionIncreaseFOV             = "IncreaseFOV"
	ActionDecreaseFOV             = "DecreaseFOV"
	ActionTestAtlasPicker         = "TestAtlasPicker"
	ActionSaveScreenshot          = "SaveScreenshot"
	ActionToggleGravity           = "ToggleGravity"
	ActionCyclePlayerLight        = "CyclePlayerLight"
	ActionSpawnGameObject         = "SpawnGameObject"
	ActionOpenVoxelEditMenu       = "OpenVoxelEditMenu"
	ActionEditorTabAction         = "EditorTabAction"
	ActionEditorCycleMode         = "EditorCycleMode"
	ActionEditorRotate            = "EditorRotate"
	ActionToggleEmissiveTexture   = "ToggleEmissiveTexture"
	ActionEntityMenuNext          = "EntityMenuNext"
	ActionEntityMenuPrevious      = "EntityMenuPrevious"
	ActionEntityMenuScrollUp      = "EntityMenuScrollUp"
	ActionEntityMenuScrollDown    = "EntityMenuScrollDown"
)

var defaultKeyBindings = map[string]glfw.Key{
	ActionMoveForward:  glfw.KeyW,
	ActionMoveBackward: glfw.KeyS,
	ActionMoveLeft:     glfw.KeyA,
	ActionMoveRight:    glfw.KeyD,
	ActionCrouch:       glfw.KeyC,
	ActionJump:         glfw.KeySpace,

	ActionReload:         glfw.KeyR,
	ActionPreviousWeapon: glfw.KeyComma,
	ActionNextWeapon:     glfw.KeyPeriod,
	ActionInteract:       glfw.KeyE,

	ActionOpenInventory:      glfw.KeyI,
	ActionOpenEscapeMenu:     glfw.KeyEscape,
	ActionOpenWizardMenu:     glfw.KeyF1,
	ActionToggleMouseCapture: glfw.KeyF2,

	ActionToggleSpawnMode:         glfw.KeyF3,
	ActionChangeDecorationTexture: glfw.KeyF4,
	ActionToggleEditorMode:        glfw.KeyF5,
	ActionOpenCubeTextureMapper:   glfw.KeyF6,
	ActionWireframeOff:            glfw.KeyF7,
	ActionWireframeOn:             glfw.KeyF8,
	ActionIncreaseFOV:             glfw.KeyF9,
	ActionDecreaseFOV:             glfw.KeyF10,
	ActionTestAtlasPicker:         glfw.KeyF11,
	ActionSaveScreenshot:          glfw.KeyF12,
	ActionToggleGravity:           glfw.KeyF,
	ActionCyclePlayerLight:        glfw.KeyT,
	ActionSpawnGameObject:         glfw.KeyV,
	ActionOpenVoxelEditMenu:       glfw.KeyR,
	ActionEditorTabAction:         glfw.KeyTab,
	// The editor's two new keys sit next to Tab and the number row, because in
	// editor mode the left hand never leaves them: mode, toggle, slot.
	ActionEditorCycleMode:       glfw.KeyQ,
	ActionEditorRotate:          glfw.KeyZ,
	ActionToggleEmissiveTexture: glfw.KeyG,
	ActionEntityMenuNext:        glfw.KeyL,
	ActionEntityMenuPrevious:    glfw.KeyK,
	ActionEntityMenuScrollUp:    glfw.KeyO,
	ActionEntityMenuScrollDown:  glfw.KeyP,
}

// keyByName is the vocabulary a keybindings.txt line can name a key with -
// the glfw constant's own suffix, so there is nothing new to document.
var keyByName = func() map[string]glfw.Key {
	m := map[string]glfw.Key{
		"Space": glfw.KeySpace, "Tab": glfw.KeyTab, "Escape": glfw.KeyEscape,
		"Enter": glfw.KeyEnter, "Backspace": glfw.KeyBackspace, "Delete": glfw.KeyDelete,
		"Comma": glfw.KeyComma, "Period": glfw.KeyPeriod, "Semicolon": glfw.KeySemicolon,
		"Apostrophe": glfw.KeyApostrophe, "Slash": glfw.KeySlash, "Backslash": glfw.KeyBackslash,
		"Minus": glfw.KeyMinus, "Equal": glfw.KeyEqual,
		"LeftBracket": glfw.KeyLeftBracket, "RightBracket": glfw.KeyRightBracket,
		"GraveAccent": glfw.KeyGraveAccent,
		"Up":          glfw.KeyUp, "Down": glfw.KeyDown, "Left": glfw.KeyLeft, "Right": glfw.KeyRight,
		"LeftShift": glfw.KeyLeftShift, "RightShift": glfw.KeyRightShift,
		"LeftControl": glfw.KeyLeftControl, "RightControl": glfw.KeyRightControl,
		"LeftAlt": glfw.KeyLeftAlt, "RightAlt": glfw.KeyRightAlt,
		"LeftSuper": glfw.KeyLeftSuper, "RightSuper": glfw.KeyRightSuper,
		"CapsLock": glfw.KeyCapsLock, "Home": glfw.KeyHome, "End": glfw.KeyEnd,
		"PageUp": glfw.KeyPageUp, "PageDown": glfw.KeyPageDown, "Insert": glfw.KeyInsert,
	}
	for k := glfw.KeyA; k <= glfw.KeyZ; k++ {
		m[string(rune('A'+k-glfw.KeyA))] = k
	}
	for k := glfw.Key0; k <= glfw.Key9; k++ {
		m[string(rune('0'+k-glfw.Key0))] = k
	}
	for k := glfw.KeyF1; k <= glfw.KeyF12; k++ {
		m[fmt.Sprintf("F%d", k-glfw.KeyF1+1)] = k
	}
	return m
}()

// loadKeyBindings reads "<Key> => <Action>" lines from path, starting from
// defaultKeyBindings and overriding whatever the file mentions. A missing
// file, or a file with nothing valid in it, just plays with the defaults.
func loadKeyBindings(path string) map[string]glfw.Key {
	bindings := maps.Clone(defaultKeyBindings)

	file, err := os.Open(path)
	if err != nil {
		return bindings
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		left, right, found := strings.Cut(line, "=>")
		if !found {
			log.Printf("%s:%d: expected `<Key> => <Action>`, got %q", path, lineNumber, line)
			continue
		}
		keyName, actionName := strings.TrimSpace(left), strings.TrimSpace(right)
		key, ok := keyByName[keyName]
		if !ok {
			log.Printf("%s:%d: unknown key %q", path, lineNumber, keyName)
			continue
		}
		if _, ok := defaultKeyBindings[actionName]; !ok {
			log.Printf("%s:%d: unknown action %q", path, lineNumber, actionName)
			continue
		}
		bindings[actionName] = key
	}
	return bindings
}

// key looks up the key currently bound to an action.
func (g *Game) key(action string) glfw.Key {
	return g.keys[action]
}
