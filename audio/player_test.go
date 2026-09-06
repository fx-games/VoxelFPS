package audio

import (
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
