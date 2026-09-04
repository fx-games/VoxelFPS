package main

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/zdwep"
	"regexp"
	"strings"
)

type DoomWeapon struct {
	Name                          string
	Atlas                         glhf.TextureAtlas
	WeaponScript                  []zdwep.WeaponScriptLabel
	labelToIndex                  map[string]int
	spriteToIndex                 map[string]int
	Scale                         mgl32.Vec2
	CurrentLabel                  int
	CurrentState                  int
	CurrentFrame                  int
	CurrentFrameDurationInSeconds float64 // 1 tick = 1/35th of a second

	// Effect is what pulling the trigger does in our world, the ZDoom script
	// only supplies the sprites.
	Effect            WeaponEffect
	DelayBetweenShots float64
	LastShotTime      float64
	// FireLabel is the script label to play when firing. Some weapons keep
	// their frames under a label other than "Fire".
	FireLabel string

	firing           bool
	secondsSinceFire float64
	warnedFunctions  map[string]bool
}

// maxFireAnimationSeconds is how long a firing animation may run before the
// weapon is put back to Ready. The scripts count ammo and inventory items we do
// not have, so some of them, like the Borstal shotgun, would reload forever.
const maxFireAnimationSeconds = 1.5

// newDoomWeapon builds the animation state machine from a parsed ZDoom script.
// The atlas is set separately, so this works without a GL context.
func newDoomWeapon(name string, script []zdwep.WeaponScriptLabel) *DoomWeapon {
	labelMap := make(map[string]int, len(script))
	for i, label := range script {
		labelMap[label.Label] = i
	}
	w := &DoomWeapon{
		Name:            name,
		WeaponScript:    script,
		labelToIndex:    labelMap,
		FireLabel:       "Fire",
		warnedFunctions: make(map[string]bool),
	}
	if !w.hasFrames(w.FireLabel) {
		// Charge_Pistol keeps its "Fire" label empty and starts at FireLevel0.
		for _, label := range script {
			if strings.HasPrefix(label.Label, "Fire") && len(label.States) > 0 {
				w.FireLabel = label.Label
				break
			}
		}
	}
	w.SetState("Ready")
	return w
}

func (w *DoomWeapon) hasFrames(label string) bool {
	index, exists := w.labelToIndex[label]
	return exists && len(w.WeaponScript[index].States) > 0
}

// Fire plays the firing animation if the weapon is off cooldown.
func (w *DoomWeapon) Fire(now float64) bool {
	if now-w.LastShotTime < w.DelayBetweenShots {
		return false
	}
	w.LastShotTime = now
	w.secondsSinceFire = 0
	w.firing = true
	w.SetState(w.FireLabel)
	return true
}

func (w *DoomWeapon) isReady() bool {
	return strings.HasPrefix(w.WeaponScript[w.CurrentLabel].Label, "Ready")
}

func (w *DoomWeapon) currentDurationInTics() int {
	return int(w.CurrentFrameDurationInSeconds * 35)
}

func (w *DoomWeapon) DrawRects(elapsed float64, screenWidth, screenHeight int32) (src, dst mgl32.Vec4) {
	w.update(elapsed)

	frameIndex := w.spriteIndex(w.currentSprite())

	drawPosInPixels := mgl32.Vec2{float32(screenWidth / 2), float32(screenHeight)}
	normalizedOrigin := mgl32.Vec2{0.5, 1}
	scale := w.Scale

	return w.Atlas.GetSourceAndDestRects(frameIndex, drawPosInPixels, normalizedOrigin, scale)
}

func (w *DoomWeapon) SetState(stateName string) {
	index, exists := w.labelToIndex[stateName]
	if !exists || len(w.WeaponScript[index].States) == 0 {
		return // a label without frames has nothing to show
	}
	w.CurrentLabel = index
	w.CurrentState = 0
	w.CurrentFrame = 0
	w.CurrentFrameDurationInSeconds = 0
	if w.currentState().DurationInTics == 0 {
		// The label starts on a script only state, walk on to a real frame.
		w.advanceUntil()
	}
}

func (w *DoomWeapon) currentSprite() string {
	state := w.currentState()
	frame := []rune(state.Frame)[w.CurrentFrame]
	return fmt.Sprintf("%s%c0", state.Sprite, frame)
}

func (w *DoomWeapon) currentState() zdwep.WeaponState {
	return w.WeaponScript[w.CurrentLabel].States[w.CurrentState]
}

func (w *DoomWeapon) spriteIndex(spriteName string) int {
	return w.spriteToIndex[spriteName]
}

func (w *DoomWeapon) update(elapsed float64) {
	// Idle weapons hold their ready frame. The scripts idle by waiting for
	// player input in an engine we do not have, so left running they walk into
	// their firing and reloading frames on their own.
	if !w.firing {
		return
	}

	w.secondsSinceFire += elapsed
	if w.isReady() || w.secondsSinceFire > maxFireAnimationSeconds {
		w.firing = false
		w.secondsSinceFire = 0
		w.SetState("Ready")
		return
	}

	w.CurrentFrameDurationInSeconds += elapsed
	state := w.currentState()
	if w.currentDurationInTics() >= state.DurationInTics {
		w.advanceUntil()
	}
}

// maxFrameAdvances bounds how many zero length frames the script may walk
// through before we give up, so a script that jumps in a circle cannot hang the
// game.
const maxFrameAdvances = 1000

func (w *DoomWeapon) advanceUntil() {
	for advances := 0; advances < maxFrameAdvances; advances++ {
		w.advanceFrame()
		didChangeState := true
		for didChangeState {
			didChangeState = w.applyFrame()
			if w.currentState().DurationInTics > 0 {
				return
			}
		}
	}
	println("Weapon script for", w.Name, "did not settle on a frame, going back to Ready")
	w.SetState("Ready")
}

func (w *DoomWeapon) applyFrame() (didChangeState bool) {
	if w.currentState().ControlFlow != "" {
		return w.applyControlFlow(w.currentState().ControlFlow)
	}
	if w.currentState().KeywordsAndFunction != "" {
		return w.applyKeywordsAndFunction(w.currentState().KeywordsAndFunction)
	}
	return false
}

func (w *DoomWeapon) advanceFrame() {
	w.skipEmptyLabels()
	state := w.currentState()
	length := len(state.Frame)
	w.CurrentFrameDurationInSeconds = 0
	w.CurrentFrame += 1
	if w.CurrentFrame >= length {
		w.CurrentState += 1
		w.CurrentFrame = 0
		if w.CurrentState >= len(w.WeaponScript[w.CurrentLabel].States) {
			w.CurrentLabel += 1
			w.CurrentState = 0
			w.CurrentFrame = 0
			w.skipEmptyLabels()
		}
	}
}

// skipEmptyLabels moves off labels that carry no frames, and wraps around at
// the end of the script instead of running off it.
func (w *DoomWeapon) skipEmptyLabels() {
	for skipped := 0; skipped <= len(w.WeaponScript); skipped++ {
		if w.CurrentLabel >= len(w.WeaponScript) {
			w.CurrentLabel = 0
			w.CurrentState = 0
			w.CurrentFrame = 0
		}
		if len(w.WeaponScript[w.CurrentLabel].States) > 0 {
			return
		}
		w.CurrentLabel += 1
	}
}

func (w *DoomWeapon) applyControlFlow(flow string) bool {
	//Goto FireNormalEnd
	gotoRegexp := regexp.MustCompile(`Goto\s+(\w+)`)
	switch {
	case strings.ToLower(flow) == "loop":
		w.CurrentFrame = 0
		w.CurrentState = 0
		w.CurrentFrameDurationInSeconds = 0
	case gotoRegexp.MatchString(flow):
		label := gotoRegexp.FindStringSubmatch(flow)[1]
		w.SetState(label)
	default:
		println("Unknown control flow: ", flow)
	}
	return false
}

func (w *DoomWeapon) applyKeywordsAndFunction(function string) bool {
	//A_JumpIfInventory ("FDBTSXOverdrivePowerAlt", 1, "FireLevel1X")
	jumpInventoryRegexp := regexp.MustCompile(`A_JumpIfInventory\s*\(\s*"([^"]+)"\s*,\s*(\d+)\s*,\s*"([^"]+)"\s*\)`)
	switch {
	case jumpInventoryRegexp.MatchString(function):
		parts := jumpInventoryRegexp.FindStringSubmatch(function)
		label := parts[3]
		w.SetState(label)
		return true
	default:
		if !w.warnedFunctions[function] {
			w.warnedFunctions[function] = true
			println("Weapon", w.Name, "uses an unsupported function: ", function)
		}
	}
	return false
}
