package audio

import (
	"os"

	"github.com/sinshu/go-meltysynth/meltysynth"
)

// Music is a MIDI file played through a SoundFont, rendered as the speaker asks
// for it rather than up front. A minute of the soundtrack is fifty megabytes of
// samples and a second or two of synthesis if it is rendered whole, and the
// game would pay both on every level load; synthesising the few thousand
// samples the audio callback is actually asking for costs neither.
//
// The loop is the sequencer's own: it runs back to the top of the file without
// the stream ever ending, so nothing above has to seek it.
type Music struct {
	sequencer *meltysynth.MidiFileSequencer
	// left and right are the sequencer's mono-per-side scratch, kept between
	// calls because the callback asks for the same length every time.
	left, right []float32
	// Gain is what the music plays at against the sound effects, which are
	// mixed at full scale.
	Gain    float64
	stopped bool
}

// StreamMIDI opens a MIDI file and the SoundFont to play it with. The stream
// loops for as long as it is played.
func StreamMIDI(soundFontPath, midiPath string, sampleRate int32, gain float64) (*Music, error) {
	mid, err := os.Open(midiPath)
	if err != nil {
		return nil, err
	}
	defer mid.Close()
	midiFile, err := meltysynth.NewMidiFile(mid)
	if err != nil {
		return nil, err
	}

	sf2, err := os.Open(soundFontPath)
	if err != nil {
		return nil, err
	}
	defer sf2.Close()
	soundFont, err := meltysynth.NewSoundFont(sf2)
	if err != nil {
		return nil, err
	}

	synthesizer, err := meltysynth.NewSynthesizer(soundFont, meltysynth.NewSynthesizerSettings(sampleRate))
	if err != nil {
		return nil, err
	}
	sequencer := meltysynth.NewMidiFileSequencer(synthesizer)
	sequencer.Play(midiFile, true)
	return &Music{sequencer: sequencer, Gain: gain}, nil
}

func (m *Music) Stream(samples [][2]float64) (int, bool) {
	if m.stopped {
		return 0, false
	}
	if len(m.left) < len(samples) {
		m.left = make([]float32, len(samples))
		m.right = make([]float32, len(samples))
	}
	left, right := m.left[:len(samples)], m.right[:len(samples)]
	m.sequencer.Render(left, right)
	for i := range samples {
		samples[i][0] = float64(left[i]) * m.Gain
		samples[i][1] = float64(right[i]) * m.Gain
	}
	return len(samples), true
}

// Close stops the music. It is what ends the stream: a looping sequencer never
// runs out on its own, so without this the speaker would keep the old track
// mixed in under the new one for the rest of the run.
func (m *Music) Close() error {
	m.stopped = true
	return nil
}

// The stream has no length and no position to seek to - it is synthesised on
// demand and loops for ever - but StartStream takes what the OGG streams are,
// so it answers as an endless one.
func (m *Music) Len() int       { return 0 }
func (m *Music) Position() int  { return 0 }
func (m *Music) Seek(int) error { return nil }
func (m *Music) Err() error     { return nil }
