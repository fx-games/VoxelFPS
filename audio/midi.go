package audio

import (
    "encoding/binary"
    "errors"
    "github.com/sinshu/go-meltysynth/meltysynth"
    "os"
    "time"
)

// RenderSMF renders a MIDI file using the given SoundFont file at sampleRate Hz.
func RenderSMF(soundFontFilePath, midiFilePath string, sampleRate int32) (*RawSamples, error) {
    // Load the MIDI file.
    mid, err := os.Open(midiFilePath)
    if err != nil {
        return nil, err
    }
    midiFile, err := meltysynth.NewMidiFile(mid)
    if err != nil {
        return nil, err
    }
    mid.Close()

    // Load the SoundFont.
    sf2, err := os.Open(soundFontFilePath)
    if err != nil {
        return nil, err
    }
    soundFont, err := meltysynth.NewSoundFont(sf2)
    if err != nil {
        return nil, err
    }
    sf2.Close()
    // Create the synthesizer.
    settings := meltysynth.NewSynthesizerSettings(sampleRate)
    synthesizer, err := meltysynth.NewSynthesizer(soundFont, settings)
    if err != nil {
        return nil, err
    }
    // Create the MIDI sequencer.
    sequencer := meltysynth.NewMidiFileSequencer(synthesizer)

    sequencer.Play(midiFile, false)

    // The output buffer.
    length := int(float64(settings.SampleRate) * float64(midiFile.GetLength()) / float64(time.Second))
    left := make([]float32, length)
    right := make([]float32, length)

    // Render the waveform.
    sequencer.Render(left, right)
    midiStream := &RawSamples{
        left:  left,
        right: right,
    }
    return midiStream, nil
}

type RawSamples struct {
    left, right  []float32
    offset       int
    lastSampleAt time.Time
}

func (m *RawSamples) Len() int {
    return len(m.left)
}

func (m *RawSamples) Position() int {
    return m.offset
}

func (m *RawSamples) Seek(p int) error {
    if p < 0 || p >= len(m.left) {
        return errors.New("invalid position")
    }
    m.offset = p
    return nil
}

func (m *RawSamples) Close() error {
    return nil
}

func (m *RawSamples) IsPlaying() bool {
    return time.Since(m.lastSampleAt) < time.Second
}

func (m *RawSamples) Stream(samples [][2]float64) (n int, ok bool) {
    floatsAvailable := len(samples)
    floatsAvailable = min(floatsAvailable, len(m.left)-m.offset)
    for i := m.offset; i < m.offset+floatsAvailable; i++ {
        sampleIndex := i - m.offset
        samples[sampleIndex][0] = float64(m.left[i])
        samples[sampleIndex][1] = float64(m.right[i])
    }
    m.offset += floatsAvailable
    m.lastSampleAt = time.Now()
    return floatsAvailable, floatsAvailable > 0
}

func (m *RawSamples) Err() error {
    return nil
}

func (m *RawSamples) WriteTo(w *os.File) error {
    countLeft := len(m.left)
    countRight := len(m.right)
    {
        err := binary.Write(w, binary.LittleEndian, uint64(countLeft))
        if err != nil {
            return err
        }
    }
    {
        err := binary.Write(w, binary.LittleEndian, uint64(countRight))
        if err != nil {
            return err
        }
    }

    for i := 0; i < len(m.left); i++ {
        err := binary.Write(w, binary.LittleEndian, m.left[i])
        if err != nil {
            return err
        }
    }
    for i := 0; i < len(m.right); i++ {
        err := binary.Write(w, binary.LittleEndian, m.right[i])
        if err != nil {
            return err
        }
    }
    return w.Sync()
}

func LoadRawSamples(r *os.File) (*RawSamples, error) {
    var countLeft, countRight uint64
    {
        err := binary.Read(r, binary.LittleEndian, &countLeft)
        if err != nil {
            return nil, err
        }
    }
    {
        err := binary.Read(r, binary.LittleEndian, &countRight)
        if err != nil {
            return nil, err
        }
    }
    left := make([]float32, countLeft)
    right := make([]float32, countRight)
    for i := uint64(0); i < countLeft; i++ {
        err := binary.Read(r, binary.LittleEndian, &left[i])
        if err != nil {
            return nil, err
        }
    }
    for i := uint64(0); i < countRight; i++ {
        err := binary.Read(r, binary.LittleEndian, &right[i])
        if err != nil {
            return nil, err
        }
    }
    return &RawSamples{
        left:  left,
        right: right,
    }, nil

}
