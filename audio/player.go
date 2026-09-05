package audio

import (
	"fmt"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
	"math"
	"math/rand"
	"os"
	"path"
	"strings"
	"time"
)

type Player struct {
	loadedCues    map[string][]*beep.Buffer
	currentStream beep.StreamSeekCloser
	cuesLoaded    bool
	disabled      bool
	// channels holds one mixer per ZDoom sound channel, so a new sound on a
	// channel can replace what was playing there.
	channels map[int]*beep.Mixer
}

func NewPlayer(sampleRate int32) *Player {
	p := &Player{channels: make(map[int]*beep.Mixer)}
	p.UnloadAllCues()
	p.init(sampleRate)
	return p
}

func (p *Player) Disable() {
	p.disabled = true
}

func (p *Player) StopAll() {
	p.stopAndCloseStream()
	speaker.Clear()
}
func (p *Player) StreamOGGOnce(filename string) error {
	return p.streamOGGFile(filename, false)
}
func (p *Player) StreamOGGLooped(filename string) error {
	return p.streamOGGFile(filename, true)
}
func (p *Player) streamOGGFile(filename string, shouldLoop bool) error {
	if p.disabled {
		return nil
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	streamer, _, err := vorbis.Decode(f)
	if err != nil {
		return err
	}
	p.StartStream(streamer, shouldLoop)
	return nil
}

func (p *Player) StartStream(streamer beep.StreamSeekCloser, shouldLoop bool) {
	if p.disabled {
		return
	}
	p.stopAndCloseStream()

	speaker.Play(beep.Seq(streamer, beep.Callback(func() {
		if shouldLoop {
			streamer.Seek(0)
		} else {
			streamer.Close()
		}
	})))
	p.currentStream = streamer
}
func (p *Player) UnloadAllCues() {
	p.stopAndCloseStream()
	p.loadedCues = make(map[string][]*beep.Buffer)
}

func (p *Player) stopAndCloseStream() {
	if p.currentStream != nil {
		p.currentStream.Close()
	}
}
func (p *Player) LoadCuesFromDir(dirName string, cuePrefix string) {
	if p.disabled {
		return
	}
	cueBase := path.Base(dirName)
	if cuePrefix != "" {
		cueBase = fmt.Sprintf("%s/%s", cuePrefix, cueBase)
	}
	entries, _ := os.ReadDir(dirName)
	for _, entry := range entries {
		if entry.IsDir() {
			cueName := fmt.Sprintf("%s/%s", cueBase, entry.Name())
			p.loadAllCuesInDir(path.Join(dirName, entry.Name()), cueName)
		} else {
			extension := path.Ext(entry.Name())
			if strings.ToLower(extension) != ".ogg" {
				continue
			}
			withoutExt := entry.Name()[:len(entry.Name())-len(extension)]
			cueName := fmt.Sprintf("%s/%s", cueBase, withoutExt)
			p.loadCue(path.Join(dirName, entry.Name()), cueName)
		}
	}
}
func (p *Player) loadCue(fileName string, cueName string) error {
	if p.disabled {
		return nil
	}
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}

	streamer, format, err := vorbis.Decode(f)
	if err != nil {
		return err
	}

	buffer := beep.NewBuffer(format)
	buffer.Append(streamer)

	closeErr := streamer.Close()
	if closeErr != nil {
		return closeErr
	}

	if _, exists := p.loadedCues[cueName]; !exists {
		p.loadedCues[cueName] = make([]*beep.Buffer, 0)
	}
	p.loadedCues[cueName] = append(p.loadedCues[cueName], buffer)
	return nil
}

func (p *Player) PlayCue(name string) {
	if p.disabled {
		return
	}
	if !p.cuesLoaded {
		return
	}
	buffers, bufExists := p.loadedCues[name]
	if !bufExists || len(buffers) == 0 {
		return
	}

	randomIndex := 0
	if len(buffers) > 1 {
		randomIndex = rand.Intn(len(buffers))
	}

	buffer := buffers[randomIndex]
	//format := buffer.Format()

	sfx := buffer.Streamer(0, buffer.Len())
	speaker.Play(sfx)
}

func (p *Player) init(sampleRateInt int32) {
	sampleRate := beep.SampleRate(sampleRateInt)
	initErr := speaker.Init(sampleRate, sampleRate.N(time.Second/10))
	if initErr != nil {
		fmt.Fprintf(os.Stderr, "Error initializing speaker: %v", initErr)
		return
	}
}

func (p *Player) loadAllCuesInDir(dirName string, cueName string) {
	if p.disabled {
		return
	}
	entries, _ := os.ReadDir(dirName)
	for _, entry := range entries {
		if strings.ToLower(path.Ext(entry.Name())) != ".ogg" {
			continue
		}
		p.loadCue(path.Join(dirName, entry.Name()), cueName)
	}
}

func (p *Player) SoundsLoaded() {
	p.cuesLoaded = true
}

// CueOptions is how a cue should sound: as loud, as high and as far to one side
// as whatever asked for it says.
//
// Volume is a plain multiplier, where 1 is the sound as recorded; it is
// converted to the exponential scale hearing actually works on. Pitch is a
// playback rate, where 1 is unchanged. Pan runs from -1 for hard left to +1 for
// hard right.
type CueOptions struct {
	Volume  float64
	Pitch   float64
	Looping bool
	Pan     float64
	// Channel is ZDoom's notion: a new sound on a channel replaces whatever was
	// playing there. Channel 0 mixes freely.
	Channel int
}

// PlayCueAt plays a cue with volume, pitch, panning and channel behaviour.
func (p *Player) PlayCueAt(name string, options CueOptions) {
	if p.disabled || !p.cuesLoaded {
		return
	}
	buffers, exists := p.loadedCues[name]
	if !exists || len(buffers) == 0 {
		return
	}
	buffer := buffers[0]
	if len(buffers) > 1 {
		buffer = buffers[rand.Intn(len(buffers))]
	}

	var streamer beep.Streamer = buffer.Streamer(0, buffer.Len())
	if options.Looping {
		streamer = beep.Loop(-1, buffer.Streamer(0, buffer.Len()))
	}
	if options.Pitch > 0 && options.Pitch != 1 {
		streamer = beep.ResampleRatio(3, options.Pitch, streamer)
	}
	if options.Pan != 0 {
		streamer = &effects.Pan{Streamer: streamer, Pan: clampPan(options.Pan)}
	}
	if options.Volume > 0 && options.Volume != 1 {
		streamer = &effects.Volume{
			Streamer: streamer,
			Base:     2,
			Volume:   math.Log2(options.Volume),
		}
	} else if options.Volume == 0 {
		return // asked for silence, so make none
	}

	if options.Channel > 0 {
		p.playOnChannel(options.Channel, streamer)
		return
	}
	speaker.Play(streamer)
}

// playOnChannel starts a sound on one of ZDoom's channels, stopping whatever
// was on it. Each channel is a mixer that stays in the speaker for good, so
// replacing a sound is emptying its mixer rather than reaching into the
// speaker's own list.
func (p *Player) playOnChannel(channel int, streamer beep.Streamer) {
	speaker.Lock()
	mixer, exists := p.channels[channel]
	if !exists {
		mixer = &beep.Mixer{}
		p.channels[channel] = mixer
	}
	mixer.Clear()
	mixer.Add(streamer)
	speaker.Unlock()
	if !exists {
		speaker.Play(mixer)
	}
}

func clampPan(pan float64) float64 {
	if pan < -1 {
		return -1
	}
	if pan > 1 {
		return 1
	}
	return pan
}
