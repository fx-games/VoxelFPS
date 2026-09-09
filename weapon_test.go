package main

import (
	"math"
	"testing"

	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/wep"
)

// A vanilla weapon frame's own offset is (width/2 - 160, height - 168). Every
// weapon sprite in every one of the migrated mods is authored against that: it
// is what puts a gun in the middle of the screen with its bottom edge on the
// screen's bottom edge, and a mod's recoil frame is a few pixels off it. If
// this stops holding, every gun in the folder is drawn somewhere it should not
// be, which is exactly the state this replaced.
func TestAVanillaWeaponFrameFillsTheBottomOfTheScreen(t *testing.T) {
	const viewWidth, viewHeight = 640, 480

	for _, frame := range []struct{ width, height int }{
		{256, 64},  // Brutal Doom's purist chaingun
		{320, 168}, // its minigun, on the full canvas
		{128, 128},
		{59, 55}, // and its super shotgun, on a small one
	} {
		hotspotX, hotspotY := frame.width/2-160, frame.height-168
		scale := psprScale(viewHeight)
		corner := psprCorner(viewWidth, viewHeight, 0, wep.WeaponTop, hotspotX, hotspotY, scale)

		bottom := corner.Y() + float32(frame.height)*scale.Y()
		if math.Abs(float64(bottom-viewHeight)) > 0.5 {
			t.Errorf("%dx%d frame ends at y=%.1f, want the bottom of the screen at %d",
				frame.width, frame.height, bottom, viewHeight)
		}

		// Half a source pixel of slack: an odd-width frame's offset is
		// width/2 rounded down, so it cannot be exactly centred and Doom's
		// own sprites are not.
		middle := corner.X() + float32(frame.width)/2*scale.X()
		if math.Abs(float64(middle-viewWidth/2)) > float64(scale.X())/2 {
			t.Errorf("%dx%d frame is centred on x=%.1f, want the middle of the screen at %d",
				frame.width, frame.height, middle, viewWidth/2)
		}
	}
}

// The 320x200 space is scaled by the view's height alone, so the same gun is
// the same size on a wide screen and a narrow one - a wider window shows more
// room either side of it rather than a wider gun.
func TestTheWeaponIsTheSameSizeOnAWiderScreen(t *testing.T) {
	narrow := psprCorner(640, 480, 0, wep.WeaponTop, -32, -104, psprScale(480))
	wide := psprCorner(1920, 480, 0, wep.WeaponTop, -32, -104, psprScale(480))

	if narrow.Y() != wide.Y() {
		t.Errorf("height moved from %.1f to %.1f on a wider screen", narrow.Y(), wide.Y())
	}
	if got, want := wide.X()-960, narrow.X()-320; math.Abs(float64(got-want)) > 0.001 {
		t.Errorf("offset from the middle went from %.1f to %.1f", want, got)
	}
}

// A frame that writes offset() moves the gun and it stays moved: that is how a
// recoil the script never mentions again still ends, at A_WeaponReady.
func TestOffsetsPersistUntilAWeaponReadyPutsThemBack(t *testing.T) {
	machine := wep.StateMachine{
		Steps: []wep.Step{
			{Sprite: "GUNA0", Tics: 1, OffsetY: 40},
			{Sprite: "GUNA0", Tics: 1},
			{Sprite: "GUNA0", Tics: 1, Ops: []wep.Op{{Op: wep.OpWeaponReady}}},
		},
		Labels: map[string]int{"fire": 0},
	}
	cursor := wep.NewCursor(&machine, "Fire")
	runtime := &wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}, Vars: map[string]float64{}}

	if _, y := cursor.Offsets(); y != wep.WeaponTop {
		t.Fatalf("a weapon starts held at %v, not %v", wep.WeaponTop, y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto the offset frame
	if _, y := cursor.Offsets(); y != 40 {
		t.Errorf("offset(0, 40) put the gun at %v, want 40", y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto a frame that writes nothing
	if _, y := cursor.Offsets(); y != 40 {
		t.Errorf("the next frame moved the gun to %v; a zero offset leaves it alone", y)
	}

	cursor.Update(runtime, 1.0/wep.TicRate) // onto A_WeaponReady
	if _, y := cursor.Offsets(); y != wep.WeaponTop {
		t.Errorf("A_WeaponReady left the gun at %v, want it back at %v", y, wep.WeaponTop)
	}
}

// Doom's weapon sway: a full circle across for two dips down, sixteen units at
// its widest, never above where the gun is held, and nothing at all when the
// player stands still. Every one of those is something a player notices the
// moment it is wrong - a gun that rises above where it rests reads as
// floating, and one that sways while standing still reads as drunk.
func TestTheWeaponSwaysTheWayDoomsDoes(t *testing.T) {
	const running = 16.0 // Doom units a tic, which is a run

	if x, y := weaponBobAt(0, 17); x != 0 || y != 0 {
		t.Errorf("standing still swayed to (%v, %v)", x, y)
	}

	// A whole cycle, sampled finely enough to catch the turns.
	const steps = 640
	xs := make([]float64, steps+1)
	ys := make([]float64, steps+1)
	for i := range xs {
		tic := weaponBobCycleTics * float64(i) / steps
		xs[i], ys[i] = weaponBobAt(running, tic)
		if ys[i] < 0 {
			t.Fatalf("tic %.1f swung the gun %v above where it rests", tic, -ys[i])
		}
		if math.Abs(xs[i]) > maxWeaponBob+0.001 || ys[i] > maxWeaponBob+0.001 {
			t.Fatalf("tic %.1f swung to (%v, %v), past the %v cap", tic, xs[i], ys[i], maxWeaponBob)
		}
	}

	if got := peaks(ys); got != 2 {
		t.Errorf("the gun reached the bottom of a dip %d times in one cycle, want 2", got)
	}
	// One sweep across, the long way: right at the start of the cycle, left at
	// the middle of it.
	for _, at := range []struct {
		tic  float64
		want float64
	}{{0, maxWeaponBob}, {weaponBobCycleTics / 2, -maxWeaponBob}} {
		if x, _ := weaponBobAt(running, at.tic); math.Abs(x-at.want) > 0.001 {
			t.Errorf("tic %v is %v across, want %v", at.tic, x, at.want)
		}
	}

	// And the cycle really is one: it comes back to where it started.
	if math.Abs(xs[0]-xs[steps]) > 0.001 || math.Abs(ys[0]-ys[steps]) > 0.001 {
		t.Errorf("after one cycle the gun is at (%v, %v), not (%v, %v)",
			xs[steps], ys[steps], xs[0], ys[0])
	}
}

// Two guns that spend the same ammo type - a pistol and a chaingun both
// drawing on Doom's Clip - share one count rather than each carrying its own
// full magazine: picking up a box for one and firing the other should move
// the same number. See docs/adr/0003-single-inventory-map.md and
// Game.shareInventory.
func TestTwoWeaponsSpendingTheSameAmmoShareOnePool(t *testing.T) {
	game := &Game{}
	pistol := NewWeapon(game, &wep.Weapon{
		ID:   "pistol",
		Ammo: wep.Ammo{Type: "Clip", Use: 1},
		Items: map[string]wep.Item{
			"Clip": {Max: 200, Initial: 50},
		},
	}, nil, "", "")
	chaingun := NewWeapon(game, &wep.Weapon{
		ID:   "chaingun",
		Ammo: wep.Ammo{Type: "Clip", Use: 2},
		Items: map[string]wep.Item{
			"Clip": {Max: 200, Initial: 50},
		},
	}, nil, "", "")

	// A reserve ammo type is filled to its cap when a weapon that spends it is
	// built, not to Item.Initial - see wep.NewRuntime - so both start full.
	if got := chaingun.AmmoLine(); got != "200/200" {
		t.Fatalf("chaingun ammo before firing = %q, want 200/200", got)
	}

	pistol.runtime.Inventory.Take("Clip", 10)

	if got := chaingun.AmmoLine(); got != "190/200" {
		t.Errorf("chaingun ammo after the pistol spent Clip = %q, want 190/200 - the pools are not shared", got)
	}
}

// A_Explode(damage, radius) compiles to Op.Args in call order, not to
// Op.Damage - see explodeArgs. A frag grenade's own JSON carries {67, 160},
// so this pins the mapping down against a real compiled call rather than a
// made-up pair of numbers.
func TestExplodeArgsReadsDamageAndRadiusInCallOrder(t *testing.T) {
	damage, radius := explodeArgs(wep.Op{Op: wep.OpExplode, Args: []float64{67, 160}})
	if damage != 67 {
		t.Errorf("damage = %d, want 67", damage)
	}
	wantRadius := float32(160) * worldUnitsPerDoomUnit
	if math.Abs(float64(radius-wantRadius)) > 0.0001 {
		t.Errorf("radius = %v, want %v", radius, wantRadius)
	}

	// Args less than the JSON promised isn't a real weapon this compiler would
	// ever emit, but a caller that forgot to check op.Op first could hand
	// this an unrelated, argument-less op - it should not panic.
	if damage, radius := explodeArgs(wep.Op{Op: wep.OpDie}); damage != 0 || radius != 0 {
		t.Errorf("explodeArgs(no args) = (%d, %v), want (0, 0)", damage, radius)
	}

	// GrenadeAlerter (`GrenadeAlerter : FragGrenadeExplosion`) overrides its
	// parent's Spawn state with its own A_Explode(2, 240) and DamageType
	// "Avoid" - a Brutal Doom convention every monster class answers with
	// DamageFactor 0, because the whole point of the actor is to give away a
	// live grenade's position without hurting anyone. This engine has no
	// DamageFactor table, so explodeArgs is where that has to be honoured
	// instead: zero the blast rather than let a bystander flinch every fuse
	// tick, well before the grenade goes off for real.
	if damage, radius := explodeArgs(wep.Op{
		Op: wep.OpExplode, Args: []float64{2, 240}, DamageType: "Avoid",
	}); damage != 0 || radius != 0 {
		t.Errorf("explodeArgs(DamageType Avoid) = (%d, %v), want (0, 0)", damage, radius)
	}
	// ZDoom itself is case-insensitive about class properties.
	if damage, _ := explodeArgs(wep.Op{
		Op: wep.OpExplode, Args: []float64{2, 240}, DamageType: "avoid",
	}); damage != 0 {
		t.Errorf("explodeArgs(DamageType avoid) damage = %d, want 0", damage)
	}
}

func peaks(values []float64) int {
	count := 0
	for i := 1; i < len(values)-1; i++ {
		if values[i] > values[i-1] && values[i] >= values[i+1] {
			count++
		}
	}
	return count
}

// A thrown weapon is never raised into view: its Select lifts a hand into
// frame before dropping into an idle that draws nothing, which flashed a hand
// on screen on every switch. See Equip and thrownWeapons.
func TestEquippingAThrownWeaponSkipsTheRaise(t *testing.T) {
	weapon := raisingStub("bd_frag_grenade")

	weapon.Equip()

	if got := weapon.cursor.Sprite(); got != "" {
		t.Errorf("equipping a grenade drew %q, want nothing - Select is being played", got)
	}
	if _, y := weapon.cursor.Offsets(); y != wep.WeaponTop {
		t.Errorf("equipped a grenade at y=%.0f, want %.0f - it is still down in its raise",
			y, wep.WeaponTop)
	}
}

// Everything else raises on every switch, whatever is in its magazine.
func TestEquippingAHeldWeaponAlwaysRaises(t *testing.T) {
	for _, id := range []string{"bd_shotgun", "bd_grenade_launcher"} {
		weapon := raisingStub(id)
		weapon.runtime.Inventory["Shell"] = 0 // fired dry, and still comes up

		weapon.Equip()

		if got := weapon.cursor.Sprite(); got != "GUNA0" {
			t.Errorf("%s drew %q on equip, want GUNA0 - the raise is being skipped", id, got)
		}
		if _, y := weapon.cursor.Offsets(); y != wep.WeaponBottom {
			t.Errorf("%s equipped at y=%.0f, want %.0f - the raise did not start",
				id, y, wep.WeaponBottom)
		}
	}
}

// A Select that raises something into frame, and a Ready that draws nothing -
// the shape that made the flash visible in the first place.
func raisingStub(id string) *Weapon {
	weapon := NewWeapon(&Game{}, &wep.Weapon{
		ID:    id,
		Ammo:  wep.Ammo{Type: "Shell", Use: 1},
		Items: map[string]wep.Item{"Shell": {Max: 50}},
		States: wep.StateMachine{
			Labels: map[string]int{"select": 0, "ready": 2},
			Steps: []wep.Step{
				{Sprite: "GUNA0", Tics: 1, Ops: []wep.Op{{Op: wep.OpRaise}}},
				{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}},

				{Tics: 1, Ops: []wep.Op{{Op: wep.OpWeaponReady, AllowFire: true}}},
				{Tics: 0, Ops: []wep.Op{{Op: wep.OpLoop}}},
			},
		},
	}, nil, "", "")
	weapon.Atlas = &glhf.TextureAtlasPacked{} // stands in for load()
	return weapon
}
