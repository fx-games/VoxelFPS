package main

import (
    "testing"

    "github.com/memmaker/goray/quads"
)

// The menu shows these labels and applies the step at the same index, so the two
// lists drifting apart would silently apply the wrong scale.
func TestGuiScaleLabelsMatchTheSteps(t *testing.T) {
    want := []string{"0.25x", "0.5x", "1.0x", "2.0x", "3.0x", "4.0x", "6.0x", "8.0x"}

    if len(guiScaleLabels) != len(guiScaleSteps) {
        t.Fatalf("%d labels for %d steps", len(guiScaleLabels), len(guiScaleSteps))
    }
    for i, label := range want {
        if guiScaleLabels[i] != label {
            t.Errorf("label %d = %q, want %q", i, guiScaleLabels[i], label)
        }
    }
}

// The menu opens on the scale the GUI actually starts at.
func TestDefaultGuiScaleIndexIsOne(t *testing.T) {
    if guiScaleSteps[defaultGuiScaleIndex] != 1.0 {
        t.Errorf("default index points at %vx, want 1.0x", guiScaleSteps[defaultGuiScaleIndex])
    }
}

// The menu shows WindowModeNames and hands back the selected index as a
// WindowMode, so the two have to line up.
func TestWindowModeNamesMatchTheModes(t *testing.T) {
    want := map[quads.WindowMode]string{
        quads.WindowModeWindowed:   "Windowed",
        quads.WindowModeBorderless: "Borderless",
        quads.WindowModeFullscreen: "Fullscreen",
    }
    if len(quads.WindowModeNames) != len(want) {
        t.Fatalf("%d names for %d modes", len(quads.WindowModeNames), len(want))
    }
    for mode, name := range want {
        if quads.WindowModeNames[mode] != name {
            t.Errorf("name for mode %d = %q, want %q", mode, quads.WindowModeNames[mode], name)
        }
    }
}
