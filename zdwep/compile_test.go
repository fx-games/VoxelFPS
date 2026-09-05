package zdwep

import (
	"testing"

	"github.com/memmaker/goray/wep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMod is a pump action shotgun written the way Brutal Doom writes one, so
// the awkward parts are all present: a counter for the rounds in the chamber,
// an A_JumpIfInventory with an amount of zero, a numeric jump that skips over a
// Goto, a refire chain that falls through from one label into the next, and an
// A_JumpIf on a setting the mod reads through ACS.
//
// Written here rather than taken from the mod so the tests carry no borrowed
// content and can run anywhere.
const testMod = `
ACTOR TestShells : Ammo
{
  Inventory.MaxAmount 50
}

ACTOR TestChambered : Ammo
{
  Inventory.MaxAmount 3
}

ACTOR TestOverloaded : Ammo
{
  Inventory.MaxAmount 1
}

ACTOR TestBaseWeapon : Weapon
{
  States
  {
  Select:
    TSHT A 1 A_Raise
    Loop
  Deselect:
    TSHT A 1 A_Lower
    Loop
  }
}

ACTOR TestShotgun : TestBaseWeapon
{
  Weapon.AmmoType "TestShells"
  Weapon.AmmoUse 1
  Weapon.SlotNumber 3
  Weapon.YAdjust 4
  Scale 1.5
  Tag "Test Shotgun"
  States
  {
  Ready:
    TNT1 A 0 A_JumpIfInventory ("TestShells", 1, "ReadyAmmo")
    TSHT A 1 A_WeaponReady (WRF_NOFIRE)
    Loop
  ReadyAmmo:
    TSHT A 1 A_WeaponReady
    Goto Ready
  Dryfire:
    TSHT A 6 A_PlaySound("weapons/dryfire", 5, 1, 0, 1.25)
    Goto Ready
  Fire:
    TNT1 A 0 A_JumpIfInventory ("TestChambered", 0, "Pump")
    TNT1 A 0 A_TakeInventory("TestOverloaded", 1)
    TNT1 A 0 A_JumpIfInventory ("TestShells", 1, 2)
    TNT1 A 0 A_JumpIfInventory ("TestChambered", 1, "Pump")
    Goto Dryfire
    TNT1 A 0 A_GiveInventory("TestChambered", 1)
    TNT1 A 0 A_AlertMonsters
    TNT1 A 0 A_GunFlash
    TNT1 A 0 A_JumpIf(CallACS("FD_gunsmoke") == 0, 2)
    TNT1 A 0 A_FireCustomMissile ("TestSmoke", 0, 0, 0, 6)
    TNT1 A 0 A_PlaySound("testshotgun/fire", CHAN_WEAPON, 0.8, 0, 0.7)
    TNT1 A 0 A_FireBullets (4, 0.5, 10, 5, "TestPuff", 0)
    TNT1 A 0 A_TakeInventory("TestShells", 1)
    TSHT N 1 Bright A_SetPitch (pitch - (0.8 * CallACS("FD_recoil")/65536.0))
    TSHT OP 1
    TNT1 A 0 A_ReFire
    Goto Pump
  Pump:
    TSHT ABC 1
    Goto Ready
  Flash:
    TNT1 A 2 A_Light1
    TNT1 A 2 A_Light0
    Stop
  }
}

ACTOR TestSmoke
{
  +NOGRAVITY
  RenderStyle Translucent
  Alpha 0.4
  Scale 0.8
  States
  {
  Spawn:
    TSMK ABCDE 4
    Stop
  }
}
`

func compileTestShotgun(t *testing.T) *Result {
	t.Helper()
	idx := indexFromFiles(t, map[string]string{"DECORATE": testMod})
	policy := DefaultPolicy()
	policy.Puffs["testpuff"] = "bullet"
	policy.Spawns["testsmoke"] = "sprite"
	// What a real mod's CVARINFO would say: full recoil, gun smoke on.
	policy.Settings.ACS["fd_recoil"] = 65536
	policy.Settings.ACS["fd_gunsmoke"] = 1

	result, err := CompileWeapon(idx, policy, "TestShotgun")
	require.NoError(t, err)
	return result
}

// opsAt returns the ops of the step a label starts at.
func opsAt(t *testing.T, machine wep.StateMachine, label string) []wep.Op {
	t.Helper()
	index, ok := machine.Label(label)
	require.True(t, ok, "no label %q", label)
	return machine.Steps[index].Ops
}

func TestCompileReadsWeaponProperties(t *testing.T) {
	result := compileTestShotgun(t)
	weapon := result.Weapon

	assert.Equal(t, "test_shotgun", weapon.ID)
	assert.Equal(t, "Test Shotgun", weapon.Tag)
	assert.Equal(t, 3, weapon.Slot)
	assert.Equal(t, "TestShells", weapon.Ammo.Type)
	assert.Equal(t, 1, weapon.Ammo.Use)
	assert.Equal(t, [2]float64{1.5, 1.5}, weapon.Render.Scale)
	assert.Equal(t, 4, weapon.Render.YAdjust)

	// The weapon pumps inside its own firing states, so there is nothing for a
	// reload key to do.
	assert.False(t, weapon.Reloadable)

	// The caps come from actors the weapon never names outside a jump.
	require.Contains(t, weapon.Items, "TestChambered")
	assert.Equal(t, 3, weapon.Items["TestChambered"].Max)
	assert.Equal(t, 50, weapon.Items["TestShells"].Max)
}

func TestCompileInheritsStatesFromParent(t *testing.T) {
	result := compileTestShotgun(t)

	// Select and Deselect are only on the parent.
	raise := opsAt(t, result.Weapon.States, "Select")
	require.Len(t, raise, 1)
	assert.Equal(t, wep.OpRaise, raise[0].Op)

	lower := opsAt(t, result.Weapon.States, "Deselect")
	require.Len(t, lower, 1)
	assert.Equal(t, wep.OpLower, lower[0].Op)
}

// The bug the whole pipeline exists to fix: an amount of zero means "when the
// chamber is full", and the old interpreter took that jump every time, which
// sent the weapon into its pump animation the moment the trigger was pulled.
func TestCompileKeepsAmountZeroAsAnAtMaxTest(t *testing.T) {
	result := compileTestShotgun(t)

	fire := opsAt(t, result.Weapon.States, "Fire")
	require.Len(t, fire, 1)
	assert.Equal(t, wep.OpJumpIfInventory, fire[0].Op)
	assert.Equal(t, "TestChambered", fire[0].Item)
	assert.True(t, fire[0].AtMax, "an amount of zero is an at-maximum test")
	assert.Equal(t, 0, fire[0].N)
}

// A numeric jump counts frames and skips the Goto between them, so 2 lands on
// the frame that starts firing, not on the Goto Dryfire in the way.
func TestCompileResolvesNumericJumpsPastControlFlow(t *testing.T) {
	result := compileTestShotgun(t)
	machine := result.Weapon.States

	fireIndex, ok := machine.Label("Fire")
	require.True(t, ok)

	shellCheck := machine.Steps[fireIndex+2].Ops
	require.Len(t, shellCheck, 1)
	assert.Equal(t, wep.OpJumpIfInventory, shellCheck[0].Op)
	assert.Equal(t, "TestShells", shellCheck[0].Item)
	assert.Equal(t, 1, shellCheck[0].N)
	assert.False(t, shellCheck[0].AtMax)

	target, ok := machine.Label(shellCheck[0].To)
	require.True(t, ok, "the jump names a label that exists")

	// Two frames on from the check, past the Goto that sits between them in the
	// script, is the give that starts the shot going. The Goto is not a state,
	// so it does not count towards the offset.
	landed := machine.Steps[target].Ops
	require.Len(t, landed, 1)
	assert.Equal(t, wep.OpGive, landed[0].Op)
	assert.Equal(t, "TestChambered", landed[0].Item)
}

func TestCompileReadsFireBulletsInZDoomArgumentOrder(t *testing.T) {
	result := compileTestShotgun(t)
	machine := result.Weapon.States

	var shot *wep.Op
	for i := range machine.Steps {
		for j := range machine.Steps[i].Ops {
			if machine.Steps[i].Ops[j].Op == wep.OpFireBullets {
				shot = &machine.Steps[i].Ops[j]
			}
		}
	}
	require.NotNil(t, shot)

	// A_FireBullets(spread_xy, spread_z, numbullets, damage, ...): the count is
	// the third argument, not the first.
	assert.Equal(t, 10, shot.Count)
	assert.Equal(t, 4.0, shot.SpreadH)
	assert.Equal(t, 0.5, shot.SpreadV)
	assert.Equal(t, 5, shot.Damage)
	assert.Equal(t, "bullet", shot.Puff)
}

// A_JumpIf reads mod configuration through ACS, so it folds to a constant at
// migration time: with gun smoke turned on the branch never runs and the smoke
// spawns.
func TestCompileFoldsACSConditions(t *testing.T) {
	result := compileTestShotgun(t)
	machine := result.Weapon.States

	var sawNop, sawSpawn bool
	for _, step := range machine.Steps {
		for _, op := range step.Ops {
			if op.Op == wep.OpNop && op.Source != "" {
				sawNop = true
			}
			if op.Op == wep.OpSpawnActor && op.Actor == "TestSmoke" {
				sawSpawn = true
			}
		}
	}
	assert.True(t, sawNop, "the smoke test folds away")
	assert.True(t, sawSpawn, "and the smoke itself stays")

	smoke, ok := result.Actors["testsmoke"]
	require.True(t, ok, "the spawned actor is compiled too")
	assert.True(t, smoke.NoGravity)
	assert.Equal(t, "translucent", smoke.Render.Style)
	assert.InDelta(t, 0.4, smoke.Render.Alpha, 0.001)
	assert.Len(t, smoke.States.Steps, 6) // five frames and the Stop
}

func TestCompileRecordsSpritesAndSounds(t *testing.T) {
	result := compileTestShotgun(t)

	// The weapon's own frames go in its own atlas.
	assert.Contains(t, result.Sprites, "TSHTA0")
	assert.Contains(t, result.Sprites, "TSHTN0")
	assert.NotContains(t, result.Sprites, "TSMKE0")

	// What it spawns is shared with every other weapon that spawns the same.
	assert.Contains(t, result.ActorSprites, "TSMKE0")

	// TNT1 frames show nothing, so they never reach either.
	for _, sprite := range append(result.Sprites, result.ActorSprites...) {
		assert.NotContains(t, sprite, "TNT1")
	}
	assert.Equal(t, []string{"testshotgun/fire", "weapons/dryfire"}, result.Sounds)
}

func TestCompileResolvesRecoilAndSoundArguments(t *testing.T) {
	result := compileTestShotgun(t)
	machine := result.Weapon.States

	var recoil, sound *wep.Op
	for i := range machine.Steps {
		for j := range machine.Steps[i].Ops {
			switch machine.Steps[i].Ops[j].Op {
			case wep.OpRecoil:
				recoil = &machine.Steps[i].Ops[j]
			case wep.OpPlaySound:
				if machine.Steps[i].Ops[j].Sound == "testshotgun/fire" {
					sound = &machine.Steps[i].Ops[j]
				}
			}
		}
	}

	require.NotNil(t, recoil)
	// pitch - (0.8 * 65536 / 65536), with the current pitch reading as zero.
	assert.InDelta(t, -0.8, recoil.Delta, 0.0001)

	require.NotNil(t, sound)
	assert.Equal(t, 1, sound.Channel) // CHAN_WEAPON
	assert.InDelta(t, 0.8, sound.Volume, 0.0001)
	assert.InDelta(t, 0.7, sound.Attenuation, 0.0001)
	assert.False(t, sound.Looping)
}

func TestCompileFailsOnAnActionWithNoPolicy(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR Exotic : Weapon
{
  States
  {
  Fire:
    TSHT A 1 A_OverlayFlags(1, 2, 3)
    Stop
  }
}
`})
	_, err := CompileWeapon(idx, DefaultPolicy(), "Exotic")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "A_OverlayFlags has no entry in the policy")
	assert.Contains(t, err.Error(), "DECORATE:7")
}

func TestCompileFailsOnAJumpToNowhere(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR Lost : Weapon
{
  States
  {
  Fire:
    TSHT A 1
    Goto Nowhere
  }
}
`})
	_, err := CompileWeapon(idx, DefaultPolicy(), "Lost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Nowhere")
}

func TestCompileIsDeterministic(t *testing.T) {
	first := compileTestShotgun(t)
	second := compileTestShotgun(t)
	assert.Equal(t, first.Sprites, second.Sprites)
	assert.Equal(t, first.Sounds, second.Sounds)
	assert.Equal(t, first.Weapon, second.Weapon)
}

// Brutal Doom's weapons are `: BrutalWeapon`, and BrutalWeapon is
// `: DoomWeapon`, which GZDoom derives from Weapon without any DECORATE file
// saying so. Looking for Weapon alone finds nothing in a mod written that way.
func TestWeaponsFoundThroughEngineBaseClasses(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR ModWeaponBase : DoomWeapon {}
ACTOR BrutalShotgun : ModWeaponBase
{
  Tag "Brutal Shotgun"
  States
  {
  Ready:
    BSHT A 1 A_WeaponReady
    Loop
  }
}
ACTOR SomeAmmo : Ammo {}
`})

	assert.Equal(t, []string{"BrutalShotgun", "ModWeaponBase"}, Weapons(idx))
	assert.False(t, IsWeapon(idx, "SomeAmmo"))

	result, err := CompileWeapon(idx, DefaultPolicy(), "BrutalShotgun")
	require.NoError(t, err)
	assert.Equal(t, "brutal_shotgun", result.Weapon.ID)
}

// A_SpawnItemEx orders its numbers nothing like A_SpawnItem or
// A_CustomMissile: three separate offsets and a separate velocity triple,
// not one "sideways offset, height, angle" triple with a different member
// first each time. Mirrors a real call Brutal Doom's chainsaw execution
// makes, spawning a health bonus that pops up and scatters rather than
// landing exactly where the kill did.
func TestSpawnItemExReadsItsOwnArgumentOrder(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestSpawnWeapon : Weapon
{
  States
  {
  Ready:
    TNT1 A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 0 A_SpawnItemEx("TestBonus", 0, 0, 24, frandom(-2.5, 2.5), frandom(-2.5, 2.5), 5)
    Goto Ready
  }
}

ACTOR TestBonus
{
  States
  {
  Spawn:
    BON1 A 4
    Loop
  }
}
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestSpawnWeapon")
	require.NoError(t, err)

	ops := opsAt(t, result.Weapon.States, "Fire")
	require.Len(t, ops, 1)
	op := ops[0]

	assert.Equal(t, wep.OpSpawnItem, op.Op)
	assert.Equal(t, "TestBonus", op.Actor)
	assert.Equal(t, 0.0, op.Angle, "no angle argument was given")

	// yofs is a plain 0, not a range: sideways offset and its "to" land equal.
	assert.Equal(t, 0.0, op.SpawnOffset)
	assert.Equal(t, 0.0, op.SpawnOffsetTo)
	// zofs 24 is the height, not folded into the angle A_CustomMissile would
	// have put in this same argument slot.
	assert.Equal(t, 24.0, op.SpawnHeight)

	// xvel and yvel are frandom(-2.5, 2.5): a range, rolled at runtime, not a
	// number fixed at migration time.
	assert.Equal(t, -2.5, op.VelX)
	assert.Equal(t, 2.5, op.VelXTo)
	assert.Equal(t, -2.5, op.VelY)
	assert.Equal(t, 2.5, op.VelYTo)
	// zvel 5 is a plain number: both ends of its range are 5.
	assert.Equal(t, 5.0, op.VelZ)
	assert.Equal(t, 5.0, op.VelZTo)

	_, needed := result.Actors["testbonus"]
	assert.True(t, needed, "the spawned actor is compiled and carried")
}
