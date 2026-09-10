package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/memmaker/goray/quads"
)

func testAudioLog(cue, title string) *AudioLog {
	return &AudioLog{object: quads.NewGameObject("AudioLog"), cue: cue, title: title}
}

// Picking one up takes it out of the world and puts it on the inventory's own
// page, and it is the one the quickplay key means from then on.
func TestPickingUpAnAudioLogPutsItOnItsOwnTab(t *testing.T) {
	g := &Game{keys: defaultKeyBindings}
	first := testAudioLog("vance-41", "Maintenance log, day 41")
	second := testAudioLog("vance-42", "Maintenance log, day 42")
	g.audioLogs = []*AudioLog{first, second}

	if items := g.audioLogItems(); len(items) != 0 {
		t.Fatalf("%d logs in the inventory before any was picked up", len(items))
	}

	g.takeAudioLog(first)
	if first.object.IsAlive() {
		t.Error("the cartridge is still lying in the level after it was taken")
	}
	if g.lastAudioLog != first {
		t.Error("quickplay does not mean the log that was just picked up")
	}
	g.takeAudioLog(second)
	if g.lastAudioLog != second {
		t.Error("quickplay still means the older log after a newer one was found")
	}

	items := g.audioLogItems()
	if len(items) != 2 {
		t.Fatalf("%d rows for two logs picked up", len(items))
	}
	for _, item := range items {
		if item.Tab != audioLogTab {
			t.Errorf("%q is on the %q tab, want %q", item.Name, item.Tab, audioLogTab)
		}
		if item.Use == nil {
			t.Errorf("%q cannot be played from the inventory", item.Name)
		}
	}
	// The notes stay where they always were, on the other page - two tabs, not
	// one list with the logs appended to it.
	g.notes = []*Note{{object: quads.NewGameObject("Note"), text: "the vault is on 7412", taken: true}}
	tabs := map[string]int{}
	for _, item := range g.carriedItems() {
		tabs[item.Tab]++
	}
	if tabs[inventoryItemsTab] != 1 || tabs[audioLogTab] != 2 {
		t.Errorf("the inventory came out as %v, want one note on %q and two logs on %q",
			tabs, inventoryItemsTab, audioLogTab)
	}
}

// A log with no recording on it - one dropped in the editor and never given one
// - says so rather than playing silence, and never reaches the audio player.
func TestAnAudioLogWithNoRecordingSaysSo(t *testing.T) {
	g := &Game{keys: defaultKeyBindings}
	blank := testAudioLog("", "")
	if blank.Title() != "Unlabelled log" {
		t.Errorf("Title() = %q for a log with no label", blank.Title())
	}
	// No audio player is set up, so a call that got as far as one would panic.
	g.playAudioLog(blank)
	if g.logCuesLoaded {
		t.Error("the recordings were loaded for a log that has none")
	}
}

// The toast names whatever key quickplay is actually bound to, so rebinding it
// rebinds what the game says about it.
func TestTheToastNamesTheKeyQuickplayIsBoundTo(t *testing.T) {
	g := &Game{keys: defaultKeyBindings}
	if label := g.keyLabel(ActionQuickplayLog); label != "L" {
		t.Errorf("quickplay is labelled %q, want L", label)
	}
	if defaultKeyBindings[ActionQuickplayLog] != glfw.KeyL {
		t.Error("quickplay is not on L by default")
	}
}

// The test level's log has to have something to play: the cue it names is a
// file that is really there, the same way its keypad codes are really written
// on its notes.
func TestTheTestLevelsAudioLogHasARecording(t *testing.T) {
	level, err := loadJSONMap(mapPathFor("keypads"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, spawn := range level.Entities {
		if spawn.Entity.MeshName != audioLogMeshName {
			continue
		}
		found++
		if spawn.Entity.Text == "" {
			t.Error("an audio log in the level has no label on it")
		}
		matches, err := filepath.Glob(filepath.Join(logCueDir, spawn.Entity.Sound+".*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			t.Errorf("the log named %q, and there is no such recording in %s",
				spawn.Entity.Sound, logCueDir)
		}
	}
	if found == 0 {
		t.Fatal("the test level has no audio log in it at all")
	}
	if _, err := os.Stat(filepath.Join(meshDir, audioLogMeshName, "mesh.glb")); err != nil {
		t.Errorf("the cartridge model is missing: %v", err)
	}
}
