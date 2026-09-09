package main

import (
	"bytes"
	"testing"

	"github.com/sinshu/go-meltysynth/meltysynth"
)

// TestSongsParse renders every song through the same MIDI parser the game
// plays them with, so a generator that writes malformed deltas or a hanging
// note fails here rather than as silence in the level.
func TestSongsParse(t *testing.T) {
	for _, s := range songs {
		var buf bytes.Buffer
		buf.WriteString("MThd")
		buf.Write([]byte{0, 0, 0, 6, 0, 0, 0, 1, ppq >> 8, ppq & 0xff})
		buf.Write(compose(s))
		f, err := meltysynth.NewMidiFile(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("%s: %v", s.file, err)
		}
		if got, want := f.GetLength().Seconds(), float64(s.bars*4*60)/float64(s.bpm); got < want-1 {
			t.Errorf("%s: %.1fs, want about %.1fs", s.file, got, want)
		}
	}
}
