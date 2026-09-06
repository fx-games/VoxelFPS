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
