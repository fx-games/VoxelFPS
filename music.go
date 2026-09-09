package main

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/memmaker/goray/audio"
	"github.com/memmaker/goray/quads"
)

// The soundtrack: one MIDI file per track under assets/audio, played through
// the SoundFont beside them. Which track a level plays is the level's own
// business - see core.MapLoadResult.Music - so a new deck picks its music by
// naming it in map.json, not by a case added here.
const (
	musicDir       = "assets/audio"
	soundFontPath  = musicDir + "/soundfont.sf2"
	musicGain      = 0.35 // under the guns, which mix at full scale
	musicExtension = ".mid"
)

// playMusic puts a track on, looping, and takes the last one off. Asking for
// the track that is already playing does nothing, which is what keeps a hot
// reload - the same level, saved again from the editor - from restarting the
// music every time the map file is written.
func (g *Game) playMusic(name string) {
	if name == g.musicName {
		return
	}
	g.musicName = name
	if g.audio == nil {
		g.audio = audio.NewPlayer(audioSampleRate)
	}
	if name == "" {
		g.audio.StopStream()
		return
	}
	music, err := audio.StreamMIDI(soundFontPath, filepath.Join(musicDir, name+musicExtension), audioSampleRate, musicGain)
	if err != nil {
		// A missing SoundFont is the normal way to have no music - it is a big
		// binary that need not be in a checkout - so this is a line in the log
		// and a silent game, not a level that refuses to load.
		if !os.IsNotExist(err) {
			log.Printf("[music] %s: %v", name, err)
		}
		return
	}
	g.audio.StartStream(music, false)
}

// musicNames is the soundtrack there is: one row per .mid under assets/audio,
// named after the file. Nothing here knows what tracks exist, the same way
// nothing knows what levels do - a new track is a file, not a line of Go.
func musicNames() []string {
	entries, err := os.ReadDir(musicDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if name := entry.Name(); !entry.IsDir() && filepath.Ext(name) == musicExtension {
			names = append(names, strings.TrimSuffix(name, musicExtension))
		}
	}
	slices.Sort(names)
	return names
}

// musicOptions is the F1 menu's music player: play any track, or stop. The menu
// stays open as one is picked - flipping between tracks is the point of it - so
// what is playing is said in a toast rather than marked in a row that would only
// be redrawn on the way back in.
func (g *Game) musicOptions() []quads.GuiOption {
	names := musicNames()
	if len(names) == 0 {
		return []quads.GuiOption{quads.NewGuiOptionAction("no tracks in "+musicDir, func() {})}
	}
	options := make([]quads.GuiOption, 0, len(names)+2)
	for _, name := range names {
		options = append(options, quads.NewGuiOptionAction(name, func() {
			g.playMusic(name)
			g.Toast("Playing " + name)
		}))
	}
	options = append(options,
		quads.NewGuiOptionAction("Stop", func() {
			g.playMusic("")
			g.Toast("Music off")
		}),
		// The level's own track again, for getting back to what the deck plays
		// after listening through the rest.
		quads.NewGuiOptionAction("Level track", func() {
			g.playMusic(g.mapSource.Music)
			g.Toast("Playing the level's track")
		}),
	)
	return options
}
