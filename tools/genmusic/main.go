// genmusic writes the game's soundtrack as General MIDI files.
//
// The tracks are composed here rather than sequenced by hand, but the point of
// the generator is that a song is more than a key and a tempo: each one picks a
// bass style, an arpeggio shape, a drum kit pattern, a lead voice, a chord
// voicing and an arrangement, and those are what make one deck sound unlike the
// next. Two songs sharing an engine is fine; two songs sharing a groove is what
// makes a soundtrack sound like one long track.
//
// Run it from the repo root: go run ./tools/genmusic
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
)

const (
	ppq   = 480       // ticks per quarter note
	beat  = ppq       // one beat
	bar   = 4 * beat  // 4/4 throughout
	half  = beat / 2  // eighth
	six   = beat / 4  // sixteenth
	trip  = beat / 3  // eighth triplet
	swing = beat / 12 // how far a swung offbeat is pushed back
)

// GM drum keys
const (
	kick  = 36
	snare = 38
	rim   = 37
	hat   = 42
	ohat  = 46
	crash = 49
	ride  = 51
	tom   = 45
	hitom = 50
)

var (
	minor    = []int{0, 2, 3, 5, 7, 8, 10}
	phrygian = []int{0, 1, 3, 5, 7, 8, 10} // the flat second is most of the dread
	dorian   = []int{0, 2, 3, 5, 7, 9, 10} // minor with a lit corner in it
)

// A song's layers, each named by the shape it plays rather than by the sound it
// plays it with - the program numbers are separate, so a saw and a bell can run
// the same figure and a track can change voice without changing groove.
const (
	// bass
	drive  = "drive"   // eighths, no gaps, root-heavy
	funk   = "funk"    // syncopated sixteenths with ghost notes
	drone  = "drone"   // one note held across two bars
	pulse  = "pulse"   // dotted, leaves holes to hear the room in
	offbmc = "offbeat" // only ever on the and

	// arpeggio
	run16  = "run16" // sixteenth triad run
	wide   = "wide"  // eighths leaping octaves and ninths
	pedal  = "pedal" // a repeated pedal note with a line moving against it
	triplt = "triplet"
	sparse = "sparse" // four long notes, high up, with delay
	noArp  = ""

	// drums
	four   = "four"  // four on the floor, offbeat open hat
	brk    = "break" // breakbeat with ghost snares
	indus  = "indus" // sixteenth hats, snare on three, tom accents
	halft  = "halftime"
	tick   = "tick" // a pulse and not much else
	noDrum = ""

	// lead
	phrase = "phrase" // running eighth melody
	call   = "call"   // long notes, plenty of air
	bell   = "bell"   // high sparse motif, delayed
	noLead = ""

	// pad voicing
	triad   = "triad"
	seventh = "seventh"
	sus     = "sus"
	quartal = "quartal" // stacked fourths: no third, so no mood, just space
)

type song struct {
	file   string
	bpm    int
	root   int // MIDI note of the tonic, bass register
	scale  []int
	chords []int // scale degree per bar, looped

	bassProg, arpProg, padProg, leadProg int // GM programs, 0-based

	bassStyle, arpStyle, drumStyle, leadStyle, voicing string
	arrangement                                        string
	swung                                              bool // swing the offbeats
	echo                                               bool // delay repeats on the lead and the sparse arp

	bars int
	seed int64
}

var songs = []song{
	// Medical: where you wake up. Dotted bass with holes in it, a crystal
	// arpeggio four notes to the bar, half-time drums a long way back.
	{"medical", 108, 50, minor, []int{0, 0, 5, 3},
		38, 98, 89, 80, pulse, sparse, halft, call, seventh, "drift", false, true, 32, 1},
	// Reactor: the deck that is always running. Nothing swings, nothing rests.
	{"reactor", 132, 52, phrygian, []int{0, 0, 1, 0, 3, 3, 1, 0},
		39, 81, 95, 87, drive, run16, indus, phrase, quartal, "hit", false, false, 32, 2},
	// Maintenance: a held bass note, wide bell intervals, a kick every other
	// bar. Mostly the room.
	{"maintenance", 92, 45, dorian, []int{0, 5, 3, 4},
		38, 98, 88, 80, drone, wide, tick, bell, sus, "drift", false, true, 32, 3},
	// Executive: the deck that still thinks it is an office. Swung breakbeat,
	// funk bass, a pedal arpeggio under a running lead.
	{"executive", 116, 43, dorian, []int{0, 3, 5, 4, 0, 3, 4, 4},
		39, 80, 51, 81, funk, pedal, brk, phrase, seventh, "build", true, false, 32, 4},
	// SHODAN: no drums at all, no arpeggio, a drone and two voices circling a
	// half step apart. She is not in a hurry.
	{"shodan", 76, 48, phrygian, []int{0, 1, 0, 1},
		38, 0, 99, 82, drone, noArp, noDrum, call, quartal, "void", false, true, 24, 5},
	// Combat: triplet arpeggio against four on the floor, so it never sits
	// still even while the chord does.
	{"combat", 150, 45, phrygian, []int{0, 0, 1, 0},
		39, 81, 92, 87, offbmc, triplt, four, phrase, triad, "hit", false, false, 24, 6},
}

// layer is which parts play in one four-bar section, and an arrangement is a
// song's run of them. They differ per song because where the drums come in is
// half of what a track sounds like: "hit" is everything at once, "drift" barely
// arrives at all.
type layer struct{ pad, arp, bass, drum, lead bool }

var arrangements = map[string][]layer{
	"build": {
		{pad: true, arp: true},
		{pad: true, arp: true, bass: true},
		{pad: true, arp: true, bass: true, drum: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, bass: true, drum: true, lead: true},
		{pad: true, arp: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
	},
	"hit": {
		{pad: true, arp: true, bass: true, drum: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{arp: true, bass: true, drum: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, bass: true, drum: true, lead: true},
		{pad: true, arp: true, drum: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
	},
	"drift": {
		{pad: true},
		{pad: true, arp: true},
		{pad: true, arp: true, lead: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, bass: true, drum: true},
		{pad: true, arp: true, lead: true},
		{pad: true, arp: true, bass: true, drum: true, lead: true},
		{pad: true, lead: true},
	},
	"void": {
		{pad: true},
		{pad: true, bass: true},
		{pad: true, bass: true, lead: true},
		{pad: true, lead: true},
		{pad: true, bass: true},
		{pad: true, bass: true, lead: true},
	},
}

// note maps a scale degree (any integer, negative or past the octave) to a MIDI
// note in the song's key.
func (s song) note(deg int) int {
	oct := deg / 7
	i := deg % 7
	if i < 0 {
		i, oct = i+7, oct-1
	}
	return s.root + 12*oct + s.scale[i]
}

func (s song) chord(b int) int { return s.chords[b%len(s.chords)] }

// voice is the pad's chord, in scale degrees above its root.
func (s song) voice() []int {
	switch s.voicing {
	case seventh:
		return []int{0, 2, 4, 6}
	case sus:
		return []int{0, 3, 4, 7}
	case quartal:
		return []int{0, 3, 6, 9}
	default:
		return []int{0, 2, 4, 7}
	}
}

type ev struct {
	tick int
	ord  int // ties: meta/program first, then note-offs, then note-ons
	data []byte
}

type track struct{ evs []ev }

func (t *track) add(tick, ord int, data ...byte) {
	t.evs = append(t.evs, ev{tick, ord, data})
}

// play writes one note. Notes that would land outside the key range or with no
// length are dropped rather than left hanging.
func (t *track) play(ch, key, vel, start, dur int) {
	if dur <= 0 || key < 0 || key > 127 || vel < 1 {
		return
	}
	if vel > 127 {
		vel = 127
	}
	t.add(start, 2, byte(0x90|ch), byte(key), byte(vel))
	t.add(start+dur, 1, byte(0x80|ch), byte(key), 0)
}

// delay plays a note and, on a song that asks for it, two quieter repeats a
// dotted eighth apart. It is the one effect a MIDI file can carry on its own:
// the SoundFont has no delay line, so the echo has to be played.
func (s song) delay(t *track, ch, key, vel, start, dur int) {
	t.play(ch, key, vel, start, dur)
	if !s.echo {
		return
	}
	t.play(ch, key, vel*45/100, start+beat*3/4, dur)
	t.play(ch, key, vel*22/100, start+beat*3/2, dur)
}

func varint(n int) []byte {
	b := []byte{byte(n & 0x7f)}
	for n >>= 7; n > 0; n >>= 7 {
		b = append([]byte{byte(n&0x7f | 0x80)}, b...)
	}
	return b
}

func (t *track) bytes() []byte {
	sort.SliceStable(t.evs, func(i, j int) bool {
		if t.evs[i].tick != t.evs[j].tick {
			return t.evs[i].tick < t.evs[j].tick
		}
		return t.evs[i].ord < t.evs[j].ord
	})
	var body bytes.Buffer
	last := 0
	for _, e := range t.evs {
		body.Write(varint(e.tick - last))
		body.Write(e.data)
		last = e.tick
	}
	body.Write(varint(0))
	body.Write([]byte{0xff, 0x2f, 0x00}) // end of track

	var out bytes.Buffer
	out.WriteString("MTrk")
	binary.Write(&out, binary.BigEndian, uint32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// figure is one bar of a part: when a note hits, how long it holds, how far
// from the bar's chord root it sits in scale degrees, and how hard. Generated
// once and transposed to each bar's chord, so a track repeats itself the way a
// written one does.
type figure struct{ at, dur, deg, vel int }

func bassFigure(s song, r *rand.Rand) []figure {
	var f []figure
	switch s.bassStyle {
	case drive:
		for i := 0; i < 8; i++ {
			deg := 0
			if i == 6 {
				deg = 7 // the octave, once a bar, to keep it from flatlining
			}
			f = append(f, figure{i * half, half - 20, deg, 100})
		}
	case funk:
		// Sixteenths with most of them missing: the holes are the groove, and
		// the ghost notes are what a finger does between them.
		keep := []bool{true, false, true, true, false, true, false, false, true, false, true, false, true, true, false, true}
		for i, on := range keep {
			if !on {
				continue
			}
			deg, vel := 0, 104
			switch {
			case i%4 == 3:
				deg, vel = []int{7, 4, -3}[r.Intn(3)], 72
			case i > 8 && r.Float64() < 0.4:
				deg = 2
			}
			f = append(f, figure{i * six, six - 10, deg, vel})
		}
	case pulse:
		for _, at := range []int{0, beat + half, beat * 2, beat*3 + half} {
			f = append(f, figure{at, beat - 40, 0, 98})
		}
		f = append(f, figure{beat * 2, beat / 2, 7, 70})
	case offbmc:
		for i := 0; i < 4; i++ {
			f = append(f, figure{i*beat + half, half - 20, 0, 100})
			if i == 3 {
				f[len(f)-1].deg = 7
			}
		}
	case drone:
		f = append(f, figure{0, bar*2 - 40, 0, 88})
	}
	return f
}

func arpBar(t *track, s song, r *rand.Rand, at, root, sect int) {
	tones := s.voice()
	switch s.arpStyle {
	case run16:
		up := []int{0, 2, 4, 7, 4, 2}
		for i := 0; i < 16; i++ {
			d := up[i%len(up)]
			if sect%2 == 1 {
				d = up[len(up)-1-i%len(up)] // every other section runs it backwards
			}
			t.play(1, s.note(root+d+14), 62+r.Intn(10), at+i*six, six-15)
		}
	case wide:
		// Ninths and octaves rather than steps: the same chord, spread so far
		// apart it stops reading as a chord.
		leaps := []int{0, 7, 4, 11, 2, 9, 7, 14}
		for i, d := range leaps {
			t.play(1, s.note(root+d+7), 54+r.Intn(14), at+i*half, half*2)
		}
	case pedal:
		// One note repeating while a line walks against it - the trick that
		// makes a two-chord loop sound like it is going somewhere.
		for i := 0; i < 16; i++ {
			if i%2 == 0 {
				t.play(1, s.note(root+14), 58, at+i*six, six-15)
				continue
			}
			t.play(1, s.note(root+tones[(i/2)%len(tones)]+14), 70+r.Intn(8), at+i*six, six-15)
		}
	case triplt:
		for i := 0; i < 12; i++ {
			d := tones[i%len(tones)]
			t.play(1, s.note(root+d+14), 60+r.Intn(12), at+i*trip, trip-12)
		}
	case sparse:
		// Four notes a bar, held long and delayed: the arpeggio as something to
		// stand in rather than dance to.
		for i := 0; i < 4; i++ {
			d := tones[(i+sect)%len(tones)]
			s.delay(t, 1, s.note(root+d+21), 66+r.Intn(10), at+i*beat, beat*2)
		}
	}
}

// leadPhrase generates four bars of melody in the style the song asks for.
// Two are generated per song and alternated, so the tune comes back.
func leadPhrase(s song, r *rand.Rand) []figure {
	var (
		f       []figure
		density float64
		low     int
		durs    []int
	)
	switch s.leadStyle {
	case phrase:
		density, low, durs = 0.38, 7, []int{1, 1, 2, 2, 3, 4}
	case call:
		density, low, durs = 0.16, 7, []int{4, 6, 8, 8, 12}
	case bell:
		density, low, durs = 0.22, 14, []int{1, 2, 2, 4}
	default:
		return nil
	}
	deg := low + 2*r.Intn(3)
	for slot := 0; slot < 32; slot++ { // eighths across four bars
		if r.Float64() > density {
			continue
		}
		deg += []int{-2, -1, -1, 1, 1, 2, 3, -3}[r.Intn(8)]
		if deg < low-2 {
			deg += 7
		}
		if deg > low+9 {
			deg -= 7
		}
		dur := durs[r.Intn(len(durs))]
		f = append(f, figure{slot * half, dur*half - 20, deg, 84 + r.Intn(12)})
		slot += dur - 1
	}
	return f
}

func drumBar(t *track, s song, r *rand.Rand, at, b int) {
	switch s.drumStyle {
	case four:
		for i := 0; i < 4; i++ {
			t.play(9, kick, 112, at+i*beat, 60)
			t.play(9, ohat, 74, at+i*beat+half, 100) // the offbeat that makes it four on the floor
		}
		t.play(9, snare, 100, at+beat, 60)
		t.play(9, snare, 100, at+beat*3, 60)
	case brk:
		// Kick, ghost snare, kick: the pattern is in the gaps, not the hits.
		t.play(9, kick, 112, at, 60)
		t.play(9, kick, 96, at+beat*2+six*2, 60)
		t.play(9, snare, 104, at+beat, 60)
		t.play(9, snare, 104, at+beat*3, 60)
		t.play(9, snare, 48, at+beat*2+six, 40) // ghost
		t.play(9, snare, 44, at+beat*3+six*3, 40)
		for i := 0; i < 8; i++ {
			t.play(9, ride, 56+(i%2)*14, at+i*half, 60)
		}
	case indus:
		t.play(9, kick, 114, at, 60)
		t.play(9, kick, 100, at+beat*3/2, 60)
		t.play(9, kick, 100, at+beat*2+six*2, 60)
		t.play(9, snare, 108, at+beat*2, 60)
		for i := 0; i < 16; i++ {
			vel := 52
			if i%4 == 0 {
				vel = 82
			}
			t.play(9, hat, vel, at+i*six, 40)
		}
		if b%4 == 3 {
			for i := 0; i < 4; i++ {
				t.play(9, []int{tom, tom, hitom, hitom}[i], 100, at+beat*3+i*six, 60)
			}
		}
	case halft:
		// One snare a bar, on three, and space around it.
		t.play(9, kick, 108, at, 60)
		t.play(9, snare, 102, at+beat*2, 60)
		t.play(9, kick, 92, at+beat*3+six*2, 60)
		for i := 0; i < 4; i++ {
			t.play(9, hat, 50+r.Intn(10), at+i*beat+half, 40)
		}
	case tick:
		// Barely a kit: a kick every other bar and a rim to mark the middle.
		if b%2 == 0 {
			t.play(9, kick, 96, at, 60)
		}
		t.play(9, rim, 62, at+beat*2, 40)
		if b%4 == 3 {
			t.play(9, ohat, 60, at+beat*3+half, 120)
		}
	}
}

func compose(s song) []byte {
	r := rand.New(rand.NewSource(s.seed))
	t := &track{}

	usec := 60000000 / s.bpm
	t.add(0, 0, 0xff, 0x51, 0x03, byte(usec>>16), byte(usec>>8), byte(usec))
	name := append([]byte{0xff, 0x03, byte(len(s.file))}, []byte(s.file)...)
	t.add(0, 0, name...)
	for ch, prog := range map[int]int{0: s.bassProg, 1: s.arpProg, 2: s.padProg, 3: s.leadProg} {
		t.add(0, 0, byte(0xc0|ch), byte(prog))
	}

	bassFig := bassFigure(s, r)
	phrases := [][]figure{leadPhrase(s, r), leadPhrase(s, r)}
	plan := arrangements[s.arrangement]

	for b := 0; b < s.bars; b++ {
		sect := b / 4
		on := plan[sect%len(plan)]
		at := b * bar
		root := s.chord(b)

		if on.pad {
			// A drone song's chord is held across the whole two bars it lasts,
			// rather than restruck every bar under a bass note that is not
			// moving either.
			length, skip := bar-40, false
			if s.bassStyle == drone {
				length, skip = bar*2-40, b%2 == 1
			}
			if !skip {
				for _, d := range s.voice() {
					t.play(2, s.note(root+d+7), 50, at, length)
				}
			}
		}
		if on.arp {
			arpBar(t, s, r, at, root, sect)
		}
		if on.bass {
			for _, f := range bassFig {
				if s.bassStyle == drone && b%2 == 1 {
					continue
				}
				at := at + f.at
				if s.swung && (f.at/half)%2 == 1 {
					at += swing
				}
				t.play(0, s.note(root+f.deg-7), f.vel+r.Intn(8), at, f.dur)
			}
		}
		if on.lead {
			ph := phrases[sect%2]
			off := (b % 4) * bar
			for _, f := range ph {
				if f.at < off || f.at >= off+bar {
					continue
				}
				s.delay(t, 3, s.note(root+f.deg), f.vel, at+f.at-off, f.dur)
			}
		}
		if on.drum && s.drumStyle != noDrum {
			drumBar(t, s, r, at, b)
			if b%8 == 0 {
				t.play(9, crash, 100, at, beat)
			}
		}
	}
	return t.bytes()
}

func write(dir string, s song) error {
	body := compose(s)
	var out bytes.Buffer
	out.WriteString("MThd")
	binary.Write(&out, binary.BigEndian, uint32(6))
	binary.Write(&out, binary.BigEndian, []uint16{0, 1, ppq}) // format 0, one track
	out.Write(body)
	return os.WriteFile(filepath.Join(dir, s.file+".mid"), out.Bytes(), 0644)
}

func main() {
	dir := "assets/audio"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	for _, s := range songs {
		if err := write(dir, s); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%-12s %3d bpm  %-7s bass, %-7s arp, %-8s drums, %s lead\n",
			s.file, s.bpm, s.bassStyle, s.arpStyle, s.drumStyle, s.leadStyle)
	}
}
