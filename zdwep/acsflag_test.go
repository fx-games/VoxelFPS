package zdwep

import (
	"testing"

	"github.com/memmaker/goray/wep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acsFlagTestMod is shaped like Brutal Doom's railgun: firing gives a
// "Pumping" flag and hands the clearing of it to an ACS script, and Fire
// checks the flag and bounces to Ready while it is set. Nothing here runs
// ACS, so without help the flag latches on and the weapon fires once.
//
// "Charging" is the control case in the same weapon: given and gated the same
// way, but the weapon takes it back itself, so it is not waiting on a script
// and must be left alone.
const acsFlagTestMod = `
ACTOR TestACSFlagWeapon : Weapon
{
  Weapon.AmmoType "TestACSAmmo"
  Weapon.AmmoUse 1
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
    TNT1 A 0 A_JumpIfInventory("Pumping", 1, "Ready")
    TNT1 A 0 A_JumpIfInventory("Charging", 1, "Ready")
    TSHT A 5 A_FireBullets(0, 0, 1, 10, "TestACSPuff")
    TNT1 A 0 A_TakeInventory("TestACSAmmo", 1)
    TNT1 A 0 A_GiveInventory("Charging", 1)
    TNT1 A 0 A_GiveInventory("Pumping", 1)
    TNT1 A 0 ACS_NamedExecuteAlways("TestRecharge", 0, 0, 0, 0)
    TSHT A 20
    TNT1 A 0 A_TakeInventory("Charging", 1)
    Goto Ready
  }
}

ACTOR TestACSAmmo : Ammo
{
  Inventory.MaxAmount 50
}
`

func compileACSFlagWeapon(t *testing.T) *Result {
	t.Helper()
	idx := indexFromFiles(t, map[string]string{"DECORATE": acsFlagTestMod})
	result, err := CompileWeaponLenient(idx, DefaultPolicy(), "TestACSFlagWeapon")
	require.NoError(t, err)
	return result
}

func givesOf(result *Result) map[string]int {
	out := map[string]int{}
	for _, step := range result.Weapon.States.Steps {
		for _, op := range step.Ops {
			if op.Op == wep.OpGive {
				out[op.Item]++
			}
		}
	}
	return out
}

// TestAFlagOnlyAScriptClearsIsDropped is the railgun's case: the give goes,
// so the jump that reads it never fires and the weapon can be fired twice.
func TestAFlagOnlyAScriptClearsIsDropped(t *testing.T) {
	result := compileACSFlagWeapon(t)
	require.Equal(t, 1, givesOf(result)["Pumping"])

	dropped := result.DropFlagsOnlyACSTakesBack()

	assert.Equal(t, []string{"Pumping"}, dropped)
	assert.Zero(t, givesOf(result)["Pumping"])
}

// TestAFlagTheWeaponClearsItselfIsKept is the control: "Charging" is given,
// gated and taken back inside the weapon's own states, so no script was ever
// holding it and there is nothing to fix.
func TestAFlagTheWeaponClearsItselfIsKept(t *testing.T) {
	result := compileACSFlagWeapon(t)

	result.DropFlagsOnlyACSTakesBack()

	assert.Equal(t, 1, givesOf(result)["Charging"], "the weapon takes Charging back itself")
}

// TestAWeaponWithNoScriptIsLeftAlone: with no ACS call anywhere, a latched
// flag is the mod meaning it, not a script this engine failed to run.
func TestAWeaponWithNoScriptIsLeftAlone(t *testing.T) {
	result := compileACSFlagWeapon(t)
	for i := range result.Weapon.States.Steps {
		kept := result.Weapon.States.Steps[i].Ops[:0]
		for _, op := range result.Weapon.States.Steps[i].Ops {
			if op.Op != wep.OpRunScript {
				kept = append(kept, op)
			}
		}
		result.Weapon.States.Steps[i].Ops = kept
	}

	assert.Empty(t, result.DropFlagsOnlyACSTakesBack())
	assert.Equal(t, 1, givesOf(result)["Pumping"])
}

// twoLabelsOneStepMod has "Ready:" and "PostReload:" on the same step, which is
// what Brutal Doom does all over its weapons and what used to make the reload
// walk's answer depend on Go's map order.
const twoLabelsOneStepMod = `
ACTOR TestSharedLabelWeapon : Weapon
{
  Weapon.AmmoType "TestSharedReserve"
  Weapon.AmmoType2 "TestSharedClip"
  Weapon.AmmoUse 1
  States
  {
  Select:
    TSHT A 1 A_Raise
    Loop
  Deselect:
    TSHT A 1 A_Lower
    Loop
  Ready:
  Sprint:
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TSHT A 5 A_FireBullets(0, 0, 1, 10, "TestSharedPuff")
    TNT1 A 0 A_TakeInventory("TestSharedClip", 1)
    Goto Ready
  Reload:
    TNT1 A 0 A_TakeInventory("TestSharedReserve", 1)
    TNT1 A 0 A_GiveInventory("TestSharedClip", 1)
    TSHT A 4
    Goto Ready
  }
}

ACTOR TestSharedReserve : Ammo { Inventory.MaxAmount 100 }
ACTOR TestSharedClip : Ammo { Inventory.MaxAmount 8 }
`

// TestTheSameWeaponCompilesTheSameWayEveryTime is the reproducibility the lock
// file rests on: nothing in a compile may depend on Go's map order. Compiling
// one weapon many times over used to disagree with itself about which pool was
// the magazine, because two labels naming one step fought over that step.
func TestTheSameWeaponCompilesTheSameWayEveryTime(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": twoLabelsOneStepMod})
	first, err := CompileWeaponLenient(idx, DefaultPolicy(), "TestSharedLabelWeapon")
	require.NoError(t, err)

	for i := 0; i < 40; i++ {
		again, err := CompileWeaponLenient(idx, DefaultPolicy(), "TestSharedLabelWeapon")
		require.NoError(t, err)
		assert.Equal(t, first.Weapon.Ammo, again.Weapon.Ammo, "run %d disagreed about the magazine", i)
		assert.Equal(t, first.Weapon.Items, again.Weapon.Items, "run %d disagreed about what starts loaded", i)
	}
}
