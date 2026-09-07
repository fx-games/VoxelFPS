package audio

import (
	"fmt"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/effects"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
	"github.com/gopxl/beep/wav"
	"io/fs"
	"log"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Player struct {
	loadedCues map[cueKey][]*beep.Buffer
	// cueFiles is what a cue could be played from, filled by LoadCueTree
	// without reading any of it. Decoding a weapon's whole sound folder as it
	// comes into the player's hands cost a third of a second of silence on
	// every switch, and most of it is never played: a mod ships several takes
	// of every sound and a dozen sounds the weapon only makes underwater.
	cueFiles      map[cueKey][]string
	currentStream beep.StreamSeekCloser
	// loadedScopes is which scopes have been through LoadCueTree, so a cue
	// asked for before anything has loaded - or for a scope nobody gave the
	// player - answers with silence instead of a half-filled cueFiles map.
	loadedScopes map[string]bool
	disabled     bool
	// channels holds one mixer per sound channel, keyed by the thing making
	// the sound as well as the channel number. ZDoom's channels belong to an
	// origin - a new sound on the player's CHAN_WEAPON replaces the player's
	// weapon sound, not the ricochet a puff on the far wall is playing on its
	// own CHAN_WEAPON. Keyed by number alone they all shared one slot, and
	// every hitscan silenced its own gun: the pistol's fire sound was cleared
	// by the puff's ricochet in the same tic it started.
	channels map[channelKey]*channel
}

// channelKey names one sound channel: whose it is, and which of ZDoom's
// numbers. Origin zero is the player - the weapon in hand and anything else
// heard from where the listener stands.
type channelKey struct {
	origin int
	number int
}

// cueKey names one cue: which weapon it belongs to and the name its own ops
// play it by. Two weapons can and do ship a file at the same relative path -
// a dozen of BDCE's guns each have their own Shotgun/Reload.ogg - and every
// weapon the player is carrying now has its sounds loaded at once rather
// than just the one in hand, so the scope has to be part of the key or the
// second weapon loaded silently overwrites the first one's sound.
type cueKey struct {
	scope string
	name  string
}

// channel is one origin's channel: the mixer it plays through, and what is on
// it. The cue and the loop flag are kept because a looping sound that is
// already playing must not be restarted - see playOnChannel.
//
// It is a Streamer of its own rather than the mixer being handed straight to
// the speaker, and that is what lets a channel ever go away. A beep.Mixer
// never drains - empty, it streams silence for ever - so a mixer given to the
// speaker stays in the speaker's list until the game ends. Every spawned thing
// that makes a sound has an origin of its own and a channel of its own, so
// firing for a couple of minutes left hundreds of dead mixers being stepped
// through on every audio callback; enough of them and the callback stops
// keeping up with the device, and the sound stops for good. Retired, the
// channel reports itself drained and the speaker's own mixer drops it.
type channel struct {
	mixer   *beep.Mixer
	cue     string
	looping bool
	retired bool
}

// Stream mixes what is on the channel, until the channel is retired.
//
// Read and written under the speaker's lock: the speaker calls this from the
// audio callback with the lock held, and everything that retires a channel
// takes the same lock.
func (c *channel) Stream(samples [][2]float64) (int, bool) {
	if c.retired {
		return 0, false
	}
	return c.mixer.Stream(samples)
}

func (c *channel) Err() error { return nil }

func NewPlayer(sampleRate int32) *Player {
	p := &Player{channels: make(map[channelKey]*channel)}
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
	p.loadedCues = make(map[cueKey][]*beep.Buffer)
	p.cueFiles = make(map[cueKey][]string)
	p.loadedScopes = make(map[string]bool)
}

func (p *Player) stopAndCloseStream() {
	if p.currentStream != nil {
		p.currentStream.Close()
	}
}

// LoadCueTree loads every ogg under a directory, naming each cue by its path
// relative to that directory without the extension. That is exactly how ZDoom
// names sounds, so a cue loaded from sounds/hellboundshotgun/fire.ogg answers to
// "hellboundshotgun/fire", which is the name the script plays it by.
//
// scope keeps this tree's cues apart from every other weapon's: see cueKey.
func (p *Player) LoadCueTree(scope, root string) error {
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
		key := cueKey{scope, cue}
		p.cueFiles[key] = append(p.cueFiles[key], name)
		return nil
	})
}

// buffers are the decoded takes of a cue, decoded the first time it is played
// and kept until the cues are unloaded.
func (p *Player) buffers(scope, name string) []*beep.Buffer {
	key := cueKey{scope, name}
	if loaded, done := p.loadedCues[key]; done {
		return loaded
	}
	for _, file := range p.cueFiles[key] {
		if err := p.loadCue(file, scope, name); err != nil {
			log.Printf("[audio] %s: %v", file, err)
		}
	}
	if p.loadedCues[key] == nil {
		// Remembered as nothing rather than looked for again on every shot.
		p.loadedCues[key] = []*beep.Buffer{}
	}
	return p.loadedCues[key]
}

// endWhenSilent stops a streamer that has run out of file but not out of
// header. A WAV whose data chunk claims more bytes than the file holds - two
// of Final Doomer's are 56 bytes short - never reaches the end beep's decoder
// is counting towards, so it answers every read with no samples and "still
// going", and Buffer.Append, which only stops when a streamer says it is done,
// spins there for ever. The Quantum Accelerator froze the game the first time
// it fired, on the main thread, inside its own fire sound.
type endWhenSilent struct{ beep.Streamer }

func (s endWhenSilent) Stream(samples [][2]float64) (int, bool) {
	n, ok := s.Streamer.Stream(samples)
	return n, ok && n > 0
}

func (p *Player) loadCue(fileName string, scope string, cueName string) error {
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
	buffer.Append(endWhenSilent{streamer})

	closeErr := streamer.Close()
	if closeErr != nil {
		return closeErr
	}
	// A cue that decoded to nothing is a cue that is not there, and it must
	// not reach the speaker: beep.Loop asked for an endless loop of no samples
	// never returns from Stream at all - it drains the streamer, seeks back to
	// the start, and tries again, for ever, on the audio callback's own
	// goroutine. That is not a quiet sound, it is the end of all sound.
	//
	// Three of these WAVs decode to nothing today because their data chunk
	// claims 0xFFFFFFFF bytes, which the decoder reads as a negative length -
	// see repairWAV in the migration, which is where that is really fixed.
	if buffer.Len() == 0 {
		return fmt.Errorf("%s decoded to no samples at all", fileName)
	}

	key := cueKey{scope, cueName}
	if _, exists := p.loadedCues[key]; !exists {
		p.loadedCues[key] = make([]*beep.Buffer, 0)
	}
	p.loadedCues[key] = append(p.loadedCues[key], buffer)
	return nil
}

func (p *Player) PlayCue(scope, name string) {
	if p.disabled {
		return
	}
	if !p.loadedScopes[scope] {
		return
	}
	buffers := p.buffers(scope, name)
	if len(buffers) == 0 {
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

// SoundsLoaded marks a scope's cues as ready to play, once LoadCueTree has
// filled in what they could come from.
func (p *Player) SoundsLoaded(scope string) {
	p.loadedScopes[scope] = true
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
	// Origin is whose channel that is. Channels belong to the thing making the
	// sound in ZDoom, so a puff's own CHAN_WEAPON is not the player's; zero is
	// the player, and anything else in the world passes its own id.
	Origin int
}

// debugSound makes every cue say whether it was found and where it went. A
// sound that does not play is otherwise indistinguishable from one that plays
// silently: the cue may be missing from the folder, the channel it landed on
// may have been overwritten a tic later, or the volume may have come out at
// nothing. DEBUG_SOUND=1 in the environment.
var debugSound = os.Getenv("DEBUG_SOUND") != ""

// PlayCueAt plays a cue with volume, pitch, panning and channel behaviour.
// scope is whose sounds it comes from - see cueKey.
func (p *Player) PlayCueAt(scope, name string, options CueOptions) {
	if p.disabled || !p.loadedScopes[scope] {
		return
	}
	buffers := p.buffers(scope, name)
	if debugSound {
		log.Printf("[sound] %-34s takes=%d origin=%d chan=%d vol=%.2f pitch=%.2f loop=%v",
			name, len(buffers), options.Origin, options.Channel, options.Volume,
			options.Pitch, options.Looping)
	}
	if len(buffers) == 0 {
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
	number := options.Channel
	if options.Looping && number <= 0 {
		number = loopChannel
	}
	if number > 0 {
		p.playOnChannel(channelKey{options.Origin, number}, name, options.Looping, streamer)
		return
	}
	speaker.Play(streamer)
}

// playOnChannel starts a sound on one of ZDoom's channels, stopping whatever
// was on it. Each channel is a mixer that stays in the speaker for good, so
// replacing a sound is emptying its mixer rather than reaching into the
// speaker's own list.
//
// A looping sound already playing on the channel is left alone, which is
// ZDoom's own rule and not a refinement. These weapons start their idle hum
// from a Ready frame, and a Ready frame runs every tic: restarted each time,
// the loop never got past its first thirty-fifth of a second and the Prism
// Cannon's hum came out as a buzz thirty five times a second.
func (p *Player) playOnChannel(key channelKey, cue string, looping bool, streamer beep.Streamer) {
	speaker.Lock()
	if p.sameLoopAlreadyPlaying(key, cue, looping) {
		speaker.Unlock()
		if debugSound {
			log.Printf("[sound] %-34s already looping on origin=%d chan=%d, left alone",
				cue, key.origin, key.number)
		}
		return
	}
	open, exists := p.channels[key]
	if !exists {
		open = &channel{mixer: &beep.Mixer{}}
		p.channels[key] = open
	}
	open.cue, open.looping = cue, looping
	open.mixer.Clear()
	open.mixer.Add(streamer)
	speaker.Unlock()
	if !exists {
		speaker.Play(open)
	}
}

// sameLoopAlreadyPlaying reports whether this exact looping cue is the thing
// already running on that origin's channel, in which case ZDoom leaves it be
// rather than starting it again. Held apart from playOnChannel because it is
// the whole of the rule and the only part worth testing without a speaker.
func (p *Player) sameLoopAlreadyPlaying(key channelKey, cue string, looping bool) bool {
	if !looping {
		return false
	}
	open, playing := p.channels[key]
	return playing && open.looping && open.cue == cue
}

// StopChannel silences whatever is playing on one of ZDoom's channels, which
// is what A_StopSound asks for: a weapon that stops its own idle hum before it
// fires.
func (p *Player) StopChannel(origin, number int) {
	if p.disabled {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	if open, playing := p.channels[channelKey{origin, number}]; playing {
		open.clear()
	}
}

// StopChannels silences every channel belonging to one origin, which is what
// putting a weapon away means: a gun that hums while it is held has to stop
// humming once it is not. It reaches only that origin's channels - a grenade
// still in the air, or anything else the world is still playing a sound for,
// is none of a weapon switch's business and keeps going until its own sound
// ends on its own.
func (p *Player) StopChannels(origin int) {
	if p.disabled {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	for key, open := range p.channels {
		if key.origin != origin {
			continue
		}
		open.clear()
	}
}

// DropOrigin retires every channel belonging to something that no longer
// exists: a shell casing that has been cleared away, a rocket that has
// exploded. The player's own origin is never dropped - the listener is always
// there - and neither is anything still in the world.
func (p *Player) DropOrigin(origin int) {
	if p.disabled || origin == 0 {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	for key, open := range p.channels {
		if key.origin != origin {
			continue
		}
		open.clear()
		open.retired = true
		delete(p.channels, key)
	}
}

// clear silences a channel and forgets what was on it, so the next sound of
// the same name starts rather than being taken for one already playing.
func (c *channel) clear() {
	c.mixer.Clear()
	c.cue, c.looping = "", false
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
