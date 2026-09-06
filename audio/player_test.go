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
