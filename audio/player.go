package audio

import (
	"fmt"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
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

// LoadCueTree loads every ogg under a directory, naming each cue by its path
// relative to that directory without the extension. That is exactly how ZDoom
// names sounds, so a cue loaded from sounds/hellboundshotgun/fire.ogg answers to
// "hellboundshotgun/fire", which is the name the script plays it by.
//
// LoadCuesFromDir names cues after the directory it was pointed at, which puts
// an extra segment on the front; this does not.
func (p *Player) LoadCueTree(root string) error {
	if p.disabled {
		return nil
	}
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		extension := path.Ext(name)
		if !strings.EqualFold(extension, ".ogg") && !strings.EqualFold(extension, ".wav") {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		cue := filepath.ToSlash(strings.TrimSuffix(relative, extension))
		// A migration writes a sound the mod gave several takes of as
		// name.ogg, name~2.ogg and so on. They are one cue, and PlayCue picks
		// between the takes.
		if tilde := strings.LastIndexByte(cue, '~'); tilde >= 0 {
			cue = cue[:tilde]
		}
		return p.loadCue(name, cue)
	})
}

func (p *Player) loadCue(fileName string, cueName string) error {
	if p.disabled {
		return nil
	}
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}

	// A migrated sound is an ogg when the mod shipped one and a WAV when it
	// shipped a Doom sound lump, which is most of them.
	var streamer beep.StreamSeekCloser
	var format beep.Format
	if strings.EqualFold(path.Ext(fileName), ".wav") {
		streamer, format, err = wav.Decode(f)
	} else {
		streamer, format, err = vorbis.Decode(f)
	}
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

	// A sound that loops has to be stoppable, and a channel is the only handle
	// there is on one. A script that asks for a looping sound on no channel
	// gets one of its own rather than a sound that plays until the game ends.
	channel := options.Channel
	if options.Looping && channel <= 0 {
		channel = loopChannel
	}
	if channel > 0 {
		p.playOnChannel(channel, streamer)
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

// StopChannel silences whatever is playing on one of ZDoom's channels, which
// is what A_StopSound asks for: a weapon that stops its own idle hum before it
// fires.
func (p *Player) StopChannel(channel int) {
	if p.disabled {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	if mixer, playing := p.channels[channel]; playing {
		mixer.Clear()
	}
}

// StopChannels silences every channel, which is what putting a weapon away
// means: a gun that hums while it is held has to stop humming once it is not.
func (p *Player) StopChannels() {
	if p.disabled {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	for _, mixer := range p.channels {
		mixer.Clear()
	}
}

// loopChannel is where a looping sound goes when its script named no channel.
// ZDoom's own channels are small numbers, so this sits above them.
const loopChannel = 64

func clampPan(pan float64) float64 {
	if pan < -1 {
		return -1
	}
	if pan > 1 {
		return 1
	}
	return pan
}
