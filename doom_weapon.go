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
    if !exists {
        return
    }
    w.CurrentLabel = index
    w.CurrentState = 0
    w.CurrentFrame = 0
    w.CurrentFrameDurationInSeconds = 0
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
    w.CurrentFrameDurationInSeconds += elapsed
    state := w.currentState()
    if w.currentDurationInTics() >= state.DurationInTics {
        w.advanceUntil()
    }
}

func (w *DoomWeapon) advanceUntil() {

    for {
        w.advanceFrame()
        didChangeState := true
        for didChangeState {
            didChangeState = w.applyFrame()
            if w.currentState().DurationInTics > 0 {
                return
            }
        }
    }
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
    for len(w.WeaponScript[w.CurrentLabel].States) == 0 {
        w.CurrentLabel += 1
    }
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
        }
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
        println("Unknown function: ", function)

    }
    return false
}
