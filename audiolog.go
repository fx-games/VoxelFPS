package main

import (
	"log"

	"github.com/memmaker/voxelfps/audio"
	"github.com/memmaker/voxelfps/quads"
)

// An audio log is a note that talks: a data cartridge lying in the level with a
// recording on it. It is the note's twin on purpose - picked up with the same
// key, kept the same way, and carrying the same kind of thing a level wants to
// tell you - and it differs in the one way that matters, which is that it is
// listened to rather than read. So it is playable from the inventory, on a tab
// of its own, and playable without opening anything at all: the quickplay key
// starts the last one found and leaves it running while the player walks away
// from where it was lying. That is the whole point of a recording over a piece
// of paper - a note stops you to read it, a log comes with you.

const (
	audioLogMeshName = "audiolog"

	// logCueDir is where the recordings are, one file per log, named by the
	// entity's own Sound. Nothing here lists them: a new log is a file and a
	// line in a level, not a line of Go - the same rule the soundtrack follows.
	logCueDir = "assets/audio/logs"
	// logScope keeps the recordings apart from every weapon's sounds, which are
	// scoped by the weapon's own ID. See cueKey in the audio package.
	logScope = "logs"
	// logChannel is what makes a second log stop the first rather than talk
	// over it. One voice at a time is what a channel is for.
	logChannel = 1
	// logGain is full scale. A voice under the guns is a voice nobody hears,
	// and PlayCueAt reads a volume of zero as a request for silence.
	logGain = 1.0

	// audioLogEmissiveStrength lights the cartridge's running light and nothing
	// else on it - the only bright, saturated texels on the lid, which is
	// exactly what the emissive pass picks out. Lower than the keypad's: a log
	// lying on a floor is a thing to spot, not a thing to read across a room.
	audioLogEmissiveStrength = float32(1.8)

	// audioLogTab is what the inventory calls the page they are on.
	audioLogTab = "Audio Logs"
	// inventoryItemsTab is everything else - the weapons and the notes, which
	// were the whole inventory before there was more than one page of it.
	inventoryItemsTab = "Items"
)

// AudioLog is one cartridge: the recording it carries, the label on its lid,
// and whether it has been picked up. Taken means it has left the floor and is
// in the inventory, which is the same bargain a note makes.
type AudioLog struct {
	object *quads.GameObject
	cue    string
	title  string
	taken  bool
}

// Title is what to call it in a toast and in the list. A log placed in the
// editor and never labelled still has to be called something.
func (a *AudioLog) Title() string {
	if a.title == "" {
		return "Unlabelled log"
	}
	return a.title
}

func (g *Game) spawnAudioLog(object *quads.GameObject, cue, title string) {
	g.audioLogs = append(g.audioLogs, &AudioLog{object: object, cue: cue, title: title})
}

// audioLogFor is noteFor's twin: the log behind a GameObject a raycast handed
// back, or nil.
func (g *Game) audioLogFor(object *quads.GameObject) *AudioLog {
	for _, entry := range g.audioLogs {
		if entry.object == object {
			return entry
		}
	}
	return nil
}

// takeAudioLog is the E key on one. It takes it and says what it is and how to
// hear it, and it deliberately does not start playing: a log picked up in the
// middle of a corridor is one you decide to listen to, and the toast is where
// that decision is offered.
func (g *Game) takeAudioLog(entry *AudioLog) {
	if !entry.taken {
		entry.taken = true
		entry.object.Kill()
	}
	// The last one found is what the quickplay key means, because the log you
	// have just picked up is the one the toast is talking about.
	g.lastAudioLog = entry
	// The key the toast names is whatever quickplay is bound to today, not what
	// it was bound to when this line was written - see Game.keyLabel.
	g.Toast("Audio log: " + entry.Title() + "   [" + g.keyLabel(ActionQuickplayLog) + "] to play")
}

// quickplayLog is the quickplay key: the log found most recently starts
// playing, without a menu and without stopping the player walking.
func (g *Game) quickplayLog() {
	if g.lastAudioLog == nil {
		g.Toast("No audio log to play")
		return
	}
	g.playAudioLog(g.lastAudioLog)
}

// playAudioLog starts one. The recordings are loaded the first time one is
// asked for rather than at startup, the same way a weapon's sounds are: a level
// with no logs in it should cost nothing, and the cues do not survive a map
// load anyway - see mapLoad, which unloads every cue there is.
func (g *Game) playAudioLog(entry *AudioLog) {
	if entry.cue == "" {
		g.Toast(entry.Title() + ": nothing recorded on it")
		return
	}
	if g.audio == nil {
		g.audio = audio.NewPlayer(audioSampleRate)
	}
	if !g.logCuesLoaded {
		// A missing folder is a game with no logs in it, not a game that
		// stops - the same call the soundtrack makes about a missing
		// SoundFont.
		if err := g.audio.LoadCueTree(logScope, logCueDir); err != nil {
			log.Printf("[logs] %s: %v", logCueDir, err)
			return
		}
		g.audio.SoundsLoaded(logScope)
		g.logCuesLoaded = true
	}
	// A sound origin of its own, taken from the same counter everything else in
	// the world takes one from so it can never be another thing's. It has to
	// be somebody else's than the player's: putting a weapon away stops every
	// channel of the player's, and a log has to keep talking through a weapon
	// switch or "in the background" means nothing.
	if g.logOrigin == 0 {
		g.lastActorOrigin++
		g.logOrigin = g.lastActorOrigin
	}
	g.audio.PlayCueAt(logScope, entry.cue, audio.CueOptions{
		Volume:  logGain,
		Channel: logChannel,
		Origin:  g.logOrigin,
	})
	g.Toast("Playing " + entry.Title())
}

// audioLogItems is the logs the player is carrying, as inventory rows on their
// own tab. They share the note's lack of a picture for the same reason - every
// cartridge is the same cartridge - and unlike a note, selecting one does
// something: it plays.
func (g *Game) audioLogItems() []quads.InventoryItem {
	var items []quads.InventoryItem
	for _, entry := range g.audioLogs {
		if !entry.taken {
			continue
		}
		items = append(items, quads.InventoryItem{
			Name:   entry.Title(),
			Detail: []string{"recording  " + orNone(entry.cue), "", "[E] play"},
			Icon:   -1,
			Tab:    audioLogTab,
			Use:    func() { g.playAudioLog(entry) },
		})
	}
	return items
}
