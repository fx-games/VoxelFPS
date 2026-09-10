package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopxl/beep"
)

// neverEnds is what beep's wav decoder becomes when the data chunk in the
// header is longer than the file: it reads nothing and reports it is still
// going, for ever.
type neverEnds struct{}

func (neverEnds) Stream(samples [][2]float64) (int, bool) { return 0, true }
func (neverEnds) Err() error                              { return nil }

func TestATruncatedSoundDoesNotHangTheGame(t *testing.T) {
	done := make(chan struct{})
	go func() {
		beep.NewBuffer(beep.Format{SampleRate: 22050, NumChannels: 1, Precision: 2}).
			Append(endWhenSilent{neverEnds{}})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Append never returned: a short WAV still freezes the game")
	}
}

// An idle hum is started from a Ready frame, and a Ready frame runs every tic.
// Restarted each time it never gets past its first thirty-fifth of a second,
// which is what turned the Prism Cannon's hum into a buzz.
func TestALoopAlreadyPlayingIsNotStartedAgain(t *testing.T) {
	p := &Player{channels: map[channelKey]*channel{
		{origin: 0, number: 7}: {mixer: &beep.Mixer{}, cue: "prism/idle", looping: true},
	}}

	if !p.sameLoopAlreadyPlaying(channelKey{0, 7}, "prism/idle", true) {
		t.Error("the hum already running was restarted")
	}
	if p.sameLoopAlreadyPlaying(channelKey{0, 7}, "prism/spin", true) {
		t.Error("a different loop on that channel should replace the one on it")
	}
	if p.sameLoopAlreadyPlaying(channelKey{0, 7}, "prism/idle", false) {
		t.Error("a one-shot never stands in for the loop")
	}
	if p.sameLoopAlreadyPlaying(channelKey{0, 1}, "prism/idle", true) {
		t.Error("another channel of the same origin is a different channel")
	}
}

// ZDoom's channels belong to an origin. Keyed by number alone, a puff playing
// its ricochet on its own CHAN_WEAPON cleared the gun's shot from the player's
// CHAN_WEAPON in the same tic it started, and the pistol fired silently.
func TestAPuffsChannelIsNotThePlayers(t *testing.T) {
	p := &Player{channels: map[channelKey]*channel{
		{origin: 0, number: 1}: {mixer: &beep.Mixer{}, cue: "pistol/fire"},
	}}

	if _, taken := p.channels[channelKey{origin: 3, number: 1}]; taken {
		t.Fatal("a spawned thing's CHAN_WEAPON is the player's")
	}
}

// A beep.Mixer never drains - empty, it streams silence for ever - so a mixer
// handed to the speaker stays in the speaker's list until the game ends. Every
// spawned thing that makes a sound has an origin and a channel of its own, and
// two hundred of them were stacking up inside a minute of firing: enough dead
// mixers and the audio callback stops keeping up with the device, and the
// sound stops for good.
func TestARetiredChannelDrainsSoTheSpeakerDropsIt(t *testing.T) {
	live := &channel{mixer: &beep.Mixer{}}
	samples := make([][2]float64, 8)

	if n, ok := live.Stream(samples); !ok || n != len(samples) {
		t.Fatalf("a live channel should stream silence, got %d, %v", n, ok)
	}

	live.retired = true
	if n, ok := live.Stream(samples); ok || n != 0 {
		t.Fatalf("a retired channel has to report itself drained, got %d, %v", n, ok)
	}
}

// The player's own origin is the listener and never goes away. Everything else
// does, and takes its channels with it.
func TestDropOriginRetiresOnlyThatOriginsChannels(t *testing.T) {
	p := &Player{channels: map[channelKey]*channel{
		{origin: 0, number: 1}: {mixer: &beep.Mixer{}, cue: "pistol/fire"},
		{origin: 7, number: 1}: {mixer: &beep.Mixer{}, cue: "puff/ricochet"},
		{origin: 7, number: 2}: {mixer: &beep.Mixer{}, cue: "rocket/see"},
		{origin: 8, number: 1}: {mixer: &beep.Mixer{}, cue: "casing/land"},
	}}
	casing := p.channels[channelKey{8, 1}]

	p.DropOrigin(7)
	if len(p.channels) != 2 {
		t.Fatalf("dropped %d channels, want the two of origin 7", 4-len(p.channels))
	}
	if _, kept := p.channels[channelKey{0, 1}]; !kept {
		t.Error("the listener's own channel was dropped")
	}

	// Dropping the player would retire the channel every weapon plays on.
	p.DropOrigin(0)
	if _, kept := p.channels[channelKey{0, 1}]; !kept {
		t.Error("origin 0 is the listener and is never dropped")
	}
	if casing.retired {
		t.Error("another origin's channel was retired with it")
	}
}

// wav8 writes the smallest 8-bit mono WAV a decoder will take, at a rate of
// this test's choosing - which is the whole point: the two DOS games this
// engine imports from recorded at 7000 and 11111, and Doom's own lumps are
// 11025, none of which is what the speaker runs at.
func wav8(t *testing.T, rate int, samples int) string {
	t.Helper()
	body := make([]byte, samples)
	for i := range body {
		body[i] = byte(0x80 + i%16)
	}
	out := make([]byte, 44+len(body))
	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(body)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], 1)
	binary.LittleEndian.PutUint32(out[24:], uint32(rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(rate))
	binary.LittleEndian.PutUint16(out[32:], 1)
	binary.LittleEndian.PutUint16(out[34:], 8)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(body)))
	copy(out[44:], body)

	path := filepath.Join(t.TempDir(), "cue.wav")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("writing the test WAV: %v", err)
	}
	return path
}

// Nothing between a buffered cue and the speaker resamples - beep mixes
// buffers as raw samples - so a cue has to be buffered at the speaker's own
// rate or it plays at the ratio between the two. Blake Stone records at 7000
// against a 44100 speaker, which came out six times too fast and an octave and
// a half too high: a voice line as a chirp.
func TestACueIsBufferedAtTheSpeakersRateWhateverTheFileSays(t *testing.T) {
	const speakerRate = 44100
	for _, fileRate := range []int{7000, 11025, 11111, 22050, 44100} {
		p := &Player{sampleRate: speakerRate}
		p.UnloadAllCues()

		if err := p.loadCue(wav8(t, fileRate, 2000), "test", "cue"); err != nil {
			t.Fatalf("%d Hz: %v", fileRate, err)
		}
		buffers := p.loadedCues[cueKey{"test", "cue"}]
		if len(buffers) != 1 {
			t.Fatalf("%d Hz: %d buffers, want 1", fileRate, len(buffers))
		}
		if got := buffers[0].Format().SampleRate; got != speakerRate {
			t.Errorf("a %d Hz cue buffered at %d, want the speaker's %d - it will play %.1fx too fast",
				fileRate, got, speakerRate, float64(speakerRate)/float64(got))
		}
		// And the length has to move with the rate, or resampling has been
		// declared without being done.
		wantSamples := 2000 * speakerRate / fileRate
		if got := buffers[0].Len(); got < wantSamples*9/10 || got > wantSamples*11/10 {
			t.Errorf("a %d Hz cue of 2000 samples buffered as %d, want about %d",
				fileRate, got, wantSamples)
		}
	}
}
