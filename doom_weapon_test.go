package main

import (
	"github.com/memmaker/goray/png_chunks"
	"github.com/memmaker/goray/zdwep"
	"os"
	"path/filepath"
	"testing"
)

// weaponFromAssets builds the animation state machine straight from the
// shipped sprite atlas, without needing a GL context for the texture.
func weaponFromAssets(t *testing.T, name string) *DoomWeapon {
	t.Helper()
	file, err := os.Open(filepath.Join("assets", name+".png"))
	if err != nil {
		t.Fatalf("could not open the weapon atlas: %v", err)
	}
	defer file.Close()

	script, err := zdwep.ParseActorScript(png_chunks.ReadTextChunkFromPNG(file, "zscript"))
	if err != nil {
		t.Fatalf("could not parse the weapon script: %v", err)
	}
	return newDoomWeapon(name, script.ParsedScript)
}

// Every weapon the game hands out has to idle and fire without running off the
// end of its script or getting stuck on a frame that never ends.
func TestWeaponScriptsRun(t *testing.T) {
	for _, name := range []string{"Ol_Faithful", "Borstal_Shotgun", "Charge_Pistol"} {
		t.Run(name, func(t *testing.T) {
			weapon := weaponFromAssets(t, name)

			if !weapon.hasFrames(weapon.FireLabel) {
				t.Fatalf("fire label %q has no frames", weapon.FireLabel)
			}

			const frame = 1.0 / 60.0
			idleSprite := weapon.currentSprite()
			if idleSprite == "" {
				t.Fatal("weapon has no sprite to idle on")
			}
			for i := 0; i < 600; i++ { // ten seconds of idling
				weapon.update(frame)
			}
			if weapon.currentSprite() != idleSprite {
				t.Errorf("idle weapon animated itself from %q to %q", idleSprite, weapon.currentSprite())
			}

			if !weapon.Fire(1) {
				t.Fatal("expected the weapon to fire")
			}
			firedSprites := map[string]bool{}
			for i := 0; i < 30; i++ { // half a second of the firing animation
				weapon.update(frame)
				firedSprites[weapon.currentSprite()] = true
			}
			if len(firedSprites) < 2 {
				t.Errorf("firing showed no animation, only %v", firedSprites)
			}
			for i := 0; i < 600; i++ { // ten seconds of settling back down
				weapon.update(frame)
			}
			if weapon.currentSprite() == "" {
				t.Error("weapon ended up without a sprite")
			}
			// Scripts that count ammo we do not have would animate forever.
			if !weapon.isReady() {
				t.Errorf("weapon did not return to Ready, stuck in %q", weapon.WeaponScript[weapon.CurrentLabel].Label)
			}
			if weapon.currentSprite() != idleSprite {
				t.Errorf("weapon settled on %q instead of its idle frame %q", weapon.currentSprite(), idleSprite)
			}
		})
	}
}

// A weapon does not fire again until its delay has passed.
func TestWeaponFireDelay(t *testing.T) {
	weapon := weaponFromAssets(t, "Borstal_Shotgun")
	weapon.DelayBetweenShots = 0.5

	if !weapon.Fire(10) {
		t.Fatal("the first shot should go off")
	}
	if weapon.Fire(10.2) {
		t.Error("a shot went off during the delay")
	}
	if !weapon.Fire(10.6) {
		t.Error("no shot after the delay had passed")
	}
}

// Charge_Pistol carries an empty "Fire" label, the frames live under
// FireLevel0. Picking the empty one would leave the weapon without frames.
func TestWeaponWithoutFireLabelPicksAFireState(t *testing.T) {
	weapon := weaponFromAssets(t, "Charge_Pistol")

	if weapon.FireLabel == "Fire" {
		t.Errorf("expected a fallback fire label, got %q", weapon.FireLabel)
	}
	if !weapon.hasFrames(weapon.FireLabel) {
		t.Errorf("fire label %q has no frames", weapon.FireLabel)
	}
}
