package audio

import (
	"os"
	"testing"
)

// TestStreamMIDI plays half a second of the soundtrack through the SoundFont,
// which is the whole music path: the generated MIDI parses, the SoundFont
// loads, and the stream hands back sound rather than silence.
func TestStreamMIDI(t *testing.T) {
	const soundFont = "../assets/audio/soundfont.sf2"
	if _, err := os.Stat(soundFont); err != nil {
		t.Skip("no soundfont in this checkout")
	}
	music, err := StreamMIDI(soundFont, "../assets/audio/medical.mid", 44100, 1)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([][2]float64, 22050)
	if n, ok := music.Stream(samples); !ok || n != len(samples) {
		t.Fatalf("streamed %d, %v", n, ok)
	}
	var peak float64
	for _, s := range samples {
		if v := s[0]*s[0] + s[1]*s[1]; v > peak {
			peak = v
		}
	}
	if peak < 0.0001 {
		t.Fatalf("silence: peak %g", peak)
	}
	music.Close()
	if _, ok := music.Stream(samples); ok {
		t.Fatal("still streaming after Close")
	}
}
