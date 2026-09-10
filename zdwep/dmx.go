package zdwep

import (
	"encoding/binary"
	"fmt"

	"github.com/memmaker/goray/audio"
)

// Most of a Doom mod's sounds are not files in any format a decoder knows: they
// are DMX lumps, the shape id Software stored sound in. Final Doomer ships 516
// of them against 179 oggs.
//
// A DMX lump is a tiny header and then unsigned eight bit mono samples, so
// turning one into a WAV is arithmetic rather than transcoding. Doing that here
// keeps ffmpeg off the common path entirely: no external tool, no version to
// pin, and the same bytes out every run.

const (
	dmxFormat     = 3
	dmxHeaderSize = 8
	// A DMX lump repeats its first and last sample sixteen times as padding,
	// left over from how the original mixer read ahead.
	dmxPadding = 16
)

// IsDMX reports whether a lump is a Doom sound.
func IsDMX(data []byte) bool {
	return len(data) >= dmxHeaderSize &&
		binary.LittleEndian.Uint16(data[0:2]) == dmxFormat
}

// DMXToWAV turns a Doom sound lump into a WAV file.
func DMXToWAV(lump []byte) ([]byte, error) {
	if !IsDMX(lump) {
		return nil, fmt.Errorf("not a Doom sound")
	}
	rate := int(binary.LittleEndian.Uint16(lump[2:4]))
	count := int(binary.LittleEndian.Uint32(lump[4:8]))
	if rate <= 0 {
		rate = 11025 // what the original sounds were recorded at
	}

	samples := lump[dmxHeaderSize:]
	if count > 0 && count <= len(samples) {
		samples = samples[:count]
	}
	// Drop the padding at both ends, when there is enough to be padding.
	if len(samples) > 2*dmxPadding {
		samples = samples[dmxPadding : len(samples)-dmxPadding]
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples")
	}
	// Written at the speaker's rate rather than the lump's. Doom recorded these
	// at 11025 and a few at 22050, and nothing between a cue and the speaker
	// resamples - see audio.WAVFromPCM8.
	return audio.WAVFromPCM8(samples, rate), nil
}
