package audio

// Writing a sound out at the rate the speaker wants it.
//
// Nothing between a buffered cue and the speaker resamples, so a sound on disk
// at any other rate is resampled on its way into the buffer - on the game
// thread, the first time it is played. For a Doom lump at 22050 that is a tenth
// of a millisecond and not worth avoiding; for a Blake Stone effect at 7000 it
// is two and a half, because a six-fold upsample is six times the output
// samples to filter. A room of guards is a first shout each, and every one of
// them is a hitch.
//
// So anything this repo writes itself is written at the speaker's rate. The
// resample at load stays for what was migrated before this existed - see
// loadCue, which does nothing when the rates already match.

import (
	"encoding/binary"

	"github.com/gopxl/beep"
)

// SampleRate is what the speaker runs at, and therefore the rate every sound
// this repo writes is in.
const SampleRate = 44100

// WAVFromPCM8 wraps unsigned 8-bit mono samples in the smallest WAV a decoder
// will take, resampled to SampleRate on the way if they were recorded at
// anything else. Both DOS games store their sounds as raw bytes with the rate
// left to the engine, which is why the rate is an argument rather than
// something read out of the data.
//
// The output stays 8-bit: the source is, so widening it would only make the
// file bigger.
func WAVFromPCM8(samples []byte, rate int) []byte {
	if rate != SampleRate && rate > 0 && len(samples) > 0 {
		samples = resamplePCM8(samples, rate)
	}
	out := make([]byte, 44+len(samples))
	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(samples)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], 1) // mono
	binary.LittleEndian.PutUint32(out[24:], SampleRate)
	binary.LittleEndian.PutUint32(out[28:], SampleRate)
	binary.LittleEndian.PutUint16(out[32:], 1)
	binary.LittleEndian.PutUint16(out[34:], 8)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(samples)))
	copy(out[44:], samples)
	return out
}

// resamplePCM8 runs the samples through the same resampler loadCue would have,
// so a sound written here and a sound resampled on the way in come out the
// same.
func resamplePCM8(samples []byte, rate int) []byte {
	source := beep.Resample(resampleQuality, beep.SampleRate(rate), SampleRate, &pcm8{samples: samples})
	out := make([]byte, 0, len(samples)*SampleRate/rate+1)
	var block [512][2]float64
	for {
		n, ok := source.Stream(block[:])
		for _, sample := range block[:n] {
			out = append(out, encodePCM8(sample[0]))
		}
		if !ok {
			return out
		}
	}
}

// pcm8 reads unsigned 8-bit mono samples the way beep wants them: two channels
// of signed floats between -1 and 1.
type pcm8 struct {
	samples []byte
	at      int
}

func (s *pcm8) Stream(out [][2]float64) (int, bool) {
	if s.at >= len(s.samples) {
		return 0, false
	}
	n := 0
	for n < len(out) && s.at < len(s.samples) {
		value := (float64(s.samples[s.at]) - 128) / 128
		out[n] = [2]float64{value, value}
		n, s.at = n+1, s.at+1
	}
	return n, true
}

func (s *pcm8) Err() error { return nil }

// encodePCM8 puts one sample back, clipped rather than wrapped: a resampler
// overshoots a little around a sharp edge, and a wrap turns that overshoot into
// a click at the opposite extreme.
func encodePCM8(value float64) byte {
	scaled := value*128 + 128
	switch {
	case scaled <= 0:
		return 0
	case scaled >= 255:
		return 255
	}
	return byte(scaled)
}
