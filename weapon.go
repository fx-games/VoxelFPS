package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"strings"
)

// Weapon is what the player holds. The sprites come either from a Doom atlas
// with its ZDoom script (DoomWeapon) or from a plain sprite sheet
// (SheetWeapon); what a shot does to the world is the WeaponEffect.
type Weapon interface {
	DisplayName() string
	FireEffect() WeaponEffect
	SetFireEffect(effect WeaponEffect)
	// Fire starts the firing animation and reports whether the shot goes off,
	// ie. whether the weapon was off cooldown.
	Fire(now float64) bool
	Draw(elapsed float64, screenWidth, screenHeight int32)
	Equip()
	Unequip()
}

func (w *DoomWeapon) DisplayName() string {
	return strings.ReplaceAll(w.Name, "_", " ")
}

func (w *DoomWeapon) FireEffect() WeaponEffect {
	return w.Effect
}

func (w *DoomWeapon) SetFireEffect(effect WeaponEffect) {
	w.Effect = effect
}

func (w *DoomWeapon) Draw(elapsed float64, screenWidth, screenHeight int32) {
	src, dst := w.DrawRects(elapsed, screenWidth, screenHeight)
	w.Atlas.GetSpriteBatch().DrawTextured(src, dst, mgl32.Vec4{1, 1, 1, 1})
}

func (w *DoomWeapon) Equip() {
	w.firing = false
	w.SetState("Ready")
}

func (w *DoomWeapon) Unequip() {}

// SheetWeapon draws itself from a plain grid sprite sheet: one idle frame, and
// an animation that plays once per shot.
type SheetWeapon struct {
	name              string
	sprite            *quads.AnimatedSprite
	atlas             *glhf.TextureAtlasFixed
	animation         int
	effect            WeaponEffect
	delayBetweenShots float64
	lastShotTime      float64
	scale             mgl32.Vec2
}

// loadSheetWeapon builds a weapon from a sprite sheet of equally sized frames.
// The frames are shown in order on every shot, the first one is the idle frame.
func (g *Game) loadSheetWeapon(name string, fileName string, frameWidth, frameHeight int, frames []uint16, secondsPerFrame float32, effect WeaponEffect, delayBetweenShots float64) *SheetWeapon {
	atlas := g.LoadFixedGuiAtlasWithBatch(fileName, frameWidth, frameHeight, 2)

	showTimes := make([]float32, len(frames))
	for i := range showTimes {
		showTimes[i] = secondsPerFrame
	}
	animation := g.Gui.AddAnimation(quads.SpriteAnimation{Frames: [][]uint16{frames}, ShowTimeInSeconds: showTimes})

	weapon := &SheetWeapon{
		name:              name,
		sprite:            g.Gui.Add(atlas, frames[0], 0, 0),
		atlas:             atlas,
		animation:         animation,
		effect:            effect,
		delayBetweenShots: delayBetweenShots,
		scale:             g.UIScale,
	}
	weapon.sprite.Hide()
	return weapon
}

func (w *SheetWeapon) DisplayName() string {
	return w.name
}

func (w *SheetWeapon) FireEffect() WeaponEffect {
	return w.effect
}

func (w *SheetWeapon) SetFireEffect(effect WeaponEffect) {
	w.effect = effect
}

func (w *SheetWeapon) Fire(now float64) bool {
	if now-w.lastShotTime < w.delayBetweenShots {
		return false
	}
	w.lastShotTime = now
	w.sprite.PlayAnimation(w.animation)
	return true
}

// Draw only places the sprite, the GUI drawer paints it along with the rest of
// the HUD.
func (w *SheetWeapon) Draw(elapsed float64, screenWidth, screenHeight int32) {
	frameWidth, frameHeight := w.atlas.GetAtlasItemSize()
	width := float32(frameWidth) * w.scale.X()
	height := float32(frameHeight) * w.scale.Y()
	left := (float32(screenWidth) - width) / 2
	top := float32(screenHeight) - height
	w.sprite.SetDestRect(mgl32.Vec4{left, top, left + width, top + height})
}

func (w *SheetWeapon) Equip() {
	w.sprite.Show()
}

func (w *SheetWeapon) Unequip() {
	w.sprite.Hide()
}
