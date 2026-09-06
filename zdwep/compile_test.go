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

// GZDoom's own A_FireBullets (wadsrc/static/zscript/actors/inventory/
// stateprovider.zs) always runs `if (weapon != NULL) A_StartSound(weapon.
// AttackSound, CHAN_WEAPON)`, with no flag that turns it off - a weapon
// whose script never calls A_PlaySound itself, an older and simpler way to
// write one, still has a gunshot in real GZDoom, which a migration that
// skips this would leave silent.
func TestFireBulletsPlaysAttackSoundImplicitly(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestQuietWeapon : Weapon
{
  AttackSound "weapon/testfire"
  States
  {
  Ready:
    TNT1 A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 1 A_FireBullets(5.6, 0, 1, 5)
    Goto Ready
  }
}
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestQuietWeapon")
	require.NoError(t, err)

	ops := stepOpsWith(t, result.Weapon.States, wep.OpFireBullets)
	require.Len(t, ops, 2, "the implicit sound and the shot itself")
	assert.Equal(t, wep.OpPlaySound, ops[0].Op)
	assert.Equal(t, "weapon/testfire", ops[0].Sound)
	assert.Equal(t, 1, ops[0].Channel, "CHAN_WEAPON")
	assert.Equal(t, wep.OpFireBullets, ops[1].Op)
	assert.Contains(t, result.Sounds, "weapon/testfire")
}

// A weapon that already plays its own sound is untouched: doubling it would
// be wrong for every mod already migrated that scripts its own gunshot.
func TestFireBulletsLeavesAnExplicitSoundAlone(t *testing.T) {
	result := compileTestShotgun(t)
	// TestShotgun sets no AttackSound, so the step A_FireBullets landed on
	// carries nothing implicit - just the shot itself, the way it already
	// did before AttackSound was read at all.
	ops := stepOpsWith(t, result.Weapon.States, wep.OpFireBullets)
	assert.Equal(t, []wep.Op{ops[0]}, ops)
	assert.Equal(t, wep.OpFireBullets, ops[0].Op)
}

// stepOpsWith finds the one step carrying this op and returns everything on
// it, so a test can see what landed alongside it.
func stepOpsWith(t *testing.T, machine wep.StateMachine, op string) []wep.Op {
	t.Helper()
	for _, step := range machine.Steps {
		for _, candidate := range step.Ops {
			if candidate.Op == op {
				return step.Ops
			}
		}
	}
	require.Fail(t, "no step carries op %q", op)
	return nil
}

// GZDoom's ParseActions (thingdef_states.cpp) treats a frame's action as
// either one call or, in braces, a `;`-separated sequence that all run the
// instant the frame is entered - `SOCO A 1 { A_WeaponReady(...);
// A_FireBullets(...); }`, a real line from a published weapon mod, whose
// braces were previously read as part of the first call's own name and
// dropped both actions with no error at all: a weapon that would never
// signal it was ready for input.
func TestBraceWrappedActionsAllRunOnTheSameFrame(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestBraceWeapon : Weapon
{
  States
  {
  Ready:
    TNT1 A 1 { A_WeaponReady(WRF_ALLOWRELOAD); A_FireBullets(0,0,1,0,"RedDot"); }
    Loop
  }
}
ACTOR RedDot { States { Spawn: TNT1 A -1 Stop } }
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestBraceWeapon")
	require.NoError(t, err)

	ops := opsAt(t, result.Weapon.States, "Ready")
	require.Len(t, ops, 2, "both calls in the braces, on the one frame")
	assert.Equal(t, wep.OpWeaponReady, ops[0].Op)
	assert.True(t, ops[0].AllowFire)
	assert.Equal(t, wep.OpFireBullets, ops[1].Op)
}

// CountInv reads the very item NewRuntime pre-fills to capacity -
// Weapon.AmmoType1 - so it has to read it back under the same spelling that
// filled it. A mismatched case (one lowercased, one not) means the map
// lookup misses and Inventory reads as permanently empty: a real published
// weapon whose ammo is spelled "BTLoaded" jumped to its own Reload state on
// the very first tic, forever, because CountInv looked for "btloaded".
func TestCountInvMatchesTheItemsOwnCase(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestCasedAmmo : Ammo { Inventory.MaxAmount 20 }
ACTOR TestCaseSensitiveWeapon : Weapon
{
  Weapon.AmmoType1 "TestCasedAmmo"
  States
  {
  Ready:
    TNT1 A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 0 A_JumpIf(CountInv("TestCasedAmmo") == 0, "Empty")
    TNT1 A 1 A_FireBullets(0, 0, 1, 5)
    Goto Ready
  Empty:
    TNT1 A 1
    Goto Ready
  }
}
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestCaseSensitiveWeapon")
	require.NoError(t, err)

	runtime := wep.NewRuntime(result.Weapon, nil)
	cursor := wep.NewCursor(&result.Weapon.States, "Fire")
	fired := false
	for i := 0; i < 5 && !fired; i++ {
		for _, op := range cursor.Update(runtime, 1.0/wep.TicRate) {
			if op.Op == wep.OpFireBullets {
				fired = true
			}
		}
	}
	assert.True(t, fired, "a freshly loaded weapon should not read its own full magazine as empty")
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

// A magazine and a counter are the same thing in DECORATE: both are `: Ammo`
// with a maximum, and nothing says which is which. What tells them apart is
// that a reload fills a magazine. The engine hands every weapon out at once
// with nothing to pick up, so a magazine has to start loaded - and a counter
// has to not, or a pump shotgun starts with a shell already chambered and
// walks into its pump animation instead of firing.
func TestOnlyWhatAReloadFillsStartsFull(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestMagazine : Ammo { Inventory.MaxAmount 30 }
ACTOR TestChamber : Ammo { Inventory.MaxAmount 2 }
ACTOR TestReserve : Ammo { Inventory.MaxAmount 200 }

ACTOR TestReloader : Weapon
{
  Weapon.AmmoType "TestReserve"
  Tag "Test Reloader"
  States
  {
  Ready:
    TSHT A 1 A_WeaponReady(WRF_ALLOWRELOAD)
    Loop
  Fire:
    TSHT A 0 A_GiveInventory("TestChamber", 1)
    TSHT A 1 A_TakeInventory("TestMagazine", 1)
    Goto Ready
  Reload:
    TSHT A 0 A_JumpIfInventory("TestMagazine", 30, "Ready")
    TSHT A 0 A_GiveInventory("TestMagazine", 1)
    TSHT A 0 A_TakeInventory("TestReserve", 1)
    Goto Reload
  }
}
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestReloader")
	require.NoError(t, err)
	items := result.Weapon.Items

	assert.Equal(t, 30, items["TestMagazine"].Initial,
		"the reload fills the magazine, so the weapon is drawn loaded")
	assert.Equal(t, 0, items["TestChamber"].Initial,
		"the firing states chamber a round; that is a counter, not a magazine")

	// The reserve is the weapon's own declared ammo and the runtime fills that
	// itself, whatever the folder says about it.
	runtime := wep.NewRuntime(result.Weapon, nil)
	assert.Equal(t, 200, runtime.Inventory["TestReserve"])
	assert.Equal(t, 30, runtime.Inventory["TestMagazine"])
	assert.Equal(t, 0, runtime.Inventory["TestChamber"])
}

// The four spawning functions disagree about where every number lives, and
// getting one wrong puts a thing somewhere the script never asked for: a
// muzzle flash beside the player's shoulder instead of in front of the barrel,
// or a puff of smoke in the camera's lap. A_SpawnItem is the odd one - its
// distance is straight ahead where every other function's spare number is
// sideways, and it has no angle at all, the argument in that position being
// useammo.
func TestEachSpawningFunctionReadsItsOwnArgumentOrder(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": `
ACTOR TestSpawnWeapon2 : Weapon
{
  States
  {
  Ready:
    TNT1 A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 0 A_SpawnItem("TestFlash", 32, 28)
    TNT1 A 0 A_SpawnItemEx("TestFlash", 40, 8, 28)
    TNT1 A 0 A_FireCustomMissile("TestFlash", 0, 0, 12, -16)
    Goto Ready
  }
}

ACTOR TestFlash
{
  States
  {
  Spawn:
    BON1 A 4
    Stop
  }
}
`})

	result, err := CompileWeapon(idx, DefaultPolicy(), "TestSpawnWeapon2")
	require.NoError(t, err)

	start, ok := result.Weapon.States.Label("Fire")
	require.True(t, ok)
	var ops []wep.Op
	for _, step := range result.Weapon.States.Steps[start : start+3] {
		require.Len(t, step.Ops, 1)
		ops = append(ops, step.Ops[0])
	}

	item, itemEx, missile := ops[0], ops[1], ops[2]

	// A_SpawnItem(type, distance, zheight, useammo, transfer_translation).
	assert.Equal(t, 32.0, item.SpawnForward, "distance is straight ahead")
	assert.Equal(t, 0.0, item.SpawnOffset, "and there is nothing sideways")
	assert.Equal(t, 28.0, item.SpawnHeight)
	assert.Equal(t, 0.0, item.Angle, "A_SpawnItem has no angle argument at all")

	// A_SpawnItemEx(type, xofs, yofs, zofs, ...). Its yofs is to the right,
	// the same way the missile functions' spawnofs_xy is, however much the
	// rotation it is written as suggests otherwise.
	assert.Equal(t, 40.0, itemEx.SpawnForward)
	assert.Equal(t, 8.0, itemEx.SpawnOffset)
	assert.Equal(t, 28.0, itemEx.SpawnHeight)

	// A_FireCustomMissile(type, angle, useammo, spawnofs_xy, spawnheight, ...).
	// A missile leaves from inside the shooter and gets anywhere under its own
	// speed, so it has no forward offset to read.
	assert.Equal(t, 0.0, missile.SpawnForward)
	assert.Equal(t, 12.0, missile.SpawnOffset, "spawnofs_xy is sideways")
	assert.Equal(t, -16.0, missile.SpawnHeight)
}

// A "####" sprite is not a picture, it is an instruction: show whatever the
// actor is already showing. Which picture that is depends on the actor, so it
// cannot be settled here - Final Doomer's shell casing inherits a Death written
// entirely in "####" frames from a parent whose Spawn shows 9MMC frames, and
// overrides Spawn with TNTS ones. Folded in at migration time the landed shell
// draws the parent's 9mm casing; kept as a marker, the engine draws what the
// casing is on. Both pictures still have to reach the atlas, so the marker
// carries the names it can land on.
func TestKeptSpritesStayMarkersAndCarryWhatTheyCanShow(t *testing.T) {
	const mod = `
ACTOR TestCasing
{
  PROJECTILE
  States
  {
  Spawn:
    9MMC ABCDEFGH 2
    Loop
  Death:
    "####" A 0 A_ChangeFlag ("MOVEWITHSECTOR", 1)
    "####" I 1000 A_FadeTo(0, 0.01, 1)
    Stop
  }
}

ACTOR TestShellCasing : TestCasing
{
  States
  {
  Spawn:
    TNTS ABCDEFGH 2
    Loop
  }
}

ACTOR TestCasingGun : Weapon
{
  States
  {
  Select:
    TSHT A 1 A_Raise
    Loop
  Deselect:
    TSHT A 1 A_Lower
    Loop
  Ready:
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TSHT B 2 A_SpawnItemEx("TestShellCasing", 0, 0, 0, 3, 2, 4)
    Goto Ready
  }
}
`
	idx := indexFromFiles(t, map[string]string{"DECORATE": mod})
	policy := DefaultPolicy()
	policy.Spawns["testshellcasing"] = "sprite"
	result, err := CompileWeapon(idx, policy, "TestCasingGun")
	require.NoError(t, err)

	casing, ok := result.Actors["testshellcasing"]
	require.True(t, ok)

	death, ok := casing.States.Label("Death")
	require.True(t, ok)
	kept := casing.States.Steps[death+1] // "####" I 1000
	assert.Equal(t, "####I0", kept.Sprite, "the marker goes through as written")
	assert.Equal(t, []string{"9MMCI0", "TNTSI0"}, kept.SpriteAlt,
		"and names both pictures it can land on: the parent's and its own")

	// Which means both are packed, though neither is written as a frame
	// anywhere in the mod.
	assert.Contains(t, result.ActorSprites, "9MMCI0")
	assert.Contains(t, result.ActorSprites, "TNTSI0")

	// And running the thing draws the child's picture, not the parent's.
	cursor := wep.NewOverlayCursor(&casing.States, "Spawn")
	assert.Equal(t, "TNTSA0", cursor.Sprite())
	// It tumbles for a while, which is where it picks up the picture it keeps,
	// and lands - which for a +MISSILE casing is the engine sending it to Death.
	for i := 0; i < 4; i++ {
		cursor.Update(&wep.Runtime{}, 1.0/wep.TicRate)
	}
	require.True(t, cursor.Goto("Death"))
	for i := 0; i < 3; i++ {
		cursor.Update(&wep.Runtime{}, 1.0/wep.TicRate)
	}
	assert.Equal(t, "TNTSI0", cursor.Sprite())
}

// GZDoom's Wait is not a state. FStateDefinitions::SetWait marks the frame
// above it SDF_WAIT and FinishStates resolves that to `NextState = &itself`, so
// the frame repeats until something else moves the actor - running its action
// every time round. Fail is the same statement: the state parser sends WAIT and
// FAIL to the same SetWait.
func TestWaitAndFailSendTheCursorBackToTheFrameAboveThem(t *testing.T) {
	const mod = `
ACTOR TestFader
{
  PROJECTILE
  States
  {
  Spawn:
    TFAD AB 2
    Death:
    TFAD CD 1
    TFAD E 1 A_FadeTo(0, 0.01, 1)
    Wait
  Crash:
    TFAD F 1 A_FadeTo(0, 0.05, 1)
    Fail
  }
}

ACTOR TestFaderGun : Weapon
{
  States
  {
  Select:
    TSHT A 1 A_Raise
    Loop
  Deselect:
    TSHT A 1 A_Lower
    Loop
  Ready:
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TSHT B 2 A_SpawnItemEx("TestFader", 0, 0, 0)
    Goto Ready
  }
}
`
	idx := indexFromFiles(t, map[string]string{"DECORATE": mod})
	policy := DefaultPolicy()
	policy.Spawns["testfader"] = "sprite"
	result, err := CompileWeapon(idx, policy, "TestFaderGun")
	require.NoError(t, err)

	fader, ok := result.Actors["testfader"]
	require.True(t, ok)
	steps := fader.States.Steps

	// The Wait is the step after the frame it belongs to, and it jumps back to
	// it rather than stopping or holding.
	fade, ok := fader.States.Label("Death")
	require.True(t, ok)
	fade += 2 // TFAD C, TFAD D, then the frame with the fade on it
	require.Len(t, steps[fade+1].Ops, 1)
	assert.Equal(t, wep.OpGoto, steps[fade+1].Ops[0].Op)
	assert.Equal(t, fade, mustLabel(t, fader.States, steps[fade+1].Ops[0].To),
		"back to the frame above it")

	// Fail is lowered the same way, and used to stop the actor dead instead.
	crash, ok := fader.States.Label("Crash")
	require.True(t, ok)
	require.Len(t, steps[crash+1].Ops, 1)
	assert.Equal(t, wep.OpGoto, steps[crash+1].Ops[0].Op)
	assert.Equal(t, crash, mustLabel(t, fader.States, steps[crash+1].Ops[0].To))

	// And running it fades once a tic instead of once ever.
	cursor := wep.NewOverlayCursor(&fader.States, "Death")
	var fades int
	for i := 0; i < 12; i++ {
		for _, op := range cursor.Update(&wep.Runtime{}, 1.0/wep.TicRate) {
			if op.Op == wep.OpFadeTo {
				fades++
			}
		}
	}
	assert.Greater(t, fades, 5, "the fade runs again every tic")
	assert.False(t, cursor.Stopped())
}

func mustLabel(t *testing.T, machine wep.StateMachine, name string) int {
	t.Helper()
	index, ok := machine.Label(name)
	require.True(t, ok, "the jump names a label that exists")
	return index
}

// A mod's puff is `: BulletPuff` and writes nothing about gravity, because
// BulletPuff carries +NOGRAVITY and is in doom2.wad rather than in the mod.
// Read as a plain actor the puff fell: the glow a hitscan leaves sank away
// below the bullet chip it was drawn beside, while the chip stayed on the wall.
func TestAPuffInheritsNoGravityFromTheStockClassItNames(t *testing.T) {
	const mod = `
ACTOR TestPuffGun : Weapon
{
  States
  {
  Ready:
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 0 A_FireBullets (4, 0.5, 1, 5, "TestSparkPuff", 0)
    TNT1 A 0 A_FireCustomMissile ("TestFallingBit")
    TSHT A 1
    Goto Ready
  }
}

ACTOR TestSparkPuff : BulletPuff
{
  RenderStyle Add
  States { Spawn: TSMK A 4 Stop }
}

ACTOR TestFallingBit
{
  Gravity 0.7
  States { Spawn: TSMK A 4 Stop }
}
`
	idx := indexFromFiles(t, map[string]string{"DECORATE": mod})
	policy := DefaultPolicy()
	policy.Spawns["testsparkpuff"] = "sprite"
	policy.Spawns["testfallingbit"] = "sprite"

	result, err := CompileWeapon(idx, policy, "TestPuffGun")
	require.NoError(t, err)

	puff, ok := result.Actors["testsparkpuff"]
	require.True(t, ok, "the puff is compiled")
	assert.True(t, puff.NoGravity, "a BulletPuff subclass hangs where it is put")

	bit, ok := result.Actors["testfallingbit"]
	require.True(t, ok)
	assert.False(t, bit.NoGravity, "something that inherits nothing still falls")
}

// SeeSound is played from P_SpawnMissile and appears in no state, so a weapon
// whose only firing sound is one - Final Doomer's plasma rifle - compiled
// silent, and the sound file was never even copied out of the mod.
func TestAMissilesSeeSoundIsCompiledAndAskedFor(t *testing.T) {
	const mod = `
ACTOR TestSeeGun : Weapon
{
  States
  {
  Ready:
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TNT1 A 0 A_FireCustomMissile ("TestLoudBall")
    TSHT A 1
    Goto Ready
  }
}

ACTOR TestLoudBall
{
  Speed 25
  Projectile
  SeeSound "testplasma/fire"
  States { Spawn: TSMK A 4 Stop }
}
`
	idx := indexFromFiles(t, map[string]string{"DECORATE": mod})
	policy := DefaultPolicy()
	policy.Spawns["testloudball"] = "sprite"

	result, err := CompileWeapon(idx, policy, "TestSeeGun")
	require.NoError(t, err)

	ball, ok := result.Actors["testloudball"]
	require.True(t, ok)
	assert.Equal(t, "testplasma/fire", ball.SeeSound)
	assert.Contains(t, result.Sounds, "testplasma/fire",
		"the migration has to copy the file too, or the sound is named and missing")
}

// Doom's own ammo classes are in the game rather than in the mod, so a weapon
// that reloads out of "Clip" found no maximum to read and was given one. One
// is not a small reserve, it is a weapon that stops after a single round.
func TestStockAmmoKeepsItsRealCapacity(t *testing.T) {
	const mod = `
ACTOR TestClipGun : Weapon
{
  Weapon.AmmoType "TestMag"
  Weapon.AmmoType2 "Clip"
  States
  {
  Ready:
    TSHT A 1 A_WeaponReady(WRF_ALLOWRELOAD)
    Loop
  Fire:
    TNT1 A 0 A_FireBullets (4, 0.5, 1, 5, "BulletPuff", 0)
    TSHT A 1
    Goto Ready
  Reload:
    TNT1 A 0 A_TakeInventory("Clip", 1)
    TNT1 A 0 A_GiveInventory("TestMag", 1)
    TSHT A 1
    Goto Ready
  }
}

ACTOR TestMag : Ammo { Inventory.MaxAmount 30 }
`
	idx := indexFromFiles(t, map[string]string{"DECORATE": mod})
	result, err := CompileWeapon(idx, DefaultPolicy(), "TestClipGun")
	require.NoError(t, err)

	assert.Equal(t, 200, result.Weapon.Items["Clip"].Max)
	assert.Equal(t, 30, result.Weapon.Items["TestMag"].Max)
}
