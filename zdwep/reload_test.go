package zdwep

import (
	"testing"

	"github.com/memmaker/goray/wep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reloadOnAltMod is shaped like Final Doomer's own pistols: no Reload label
// at all, and AltFire is where the magazine actually gets refilled, gated
// off (WRF_NOSECONDARY) on the frame that runs once the mod's own script
// finds the ammo already full.
const reloadOnAltMod = `
ACTOR TestReloadOnAltWeapon : Weapon
{
  Weapon.AmmoType "TestPistolAmmo"
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
    TNT1 A 0 A_JumpIfInventory("TestPistolAmmo", 0, "ReadyFull")
    TSHT A 1 A_WeaponReady
    Loop
  ReadyFull:
    TSHT A 1 A_WeaponReady(WRF_NOSECONDARY)
    Loop
  Fire:
    TSHT A 5 A_FireBullets(0, 0, 1, 10, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestPistolAmmo", 1)
    Goto Ready
  AltFire:
    TSHT B 10
    TNT1 A 0 A_GiveInventory("TestPistolAmmo", 12)
    Goto Ready
  }
}

ACTOR TestPistolAmmo : Ammo
{
  Inventory.MaxAmount 12
}
`

// scopedAltMod is the case that must not match: AltFire is a real second
// attack (a hitscan of its own, standing in for a scope's zoomed shot), so
// unifying it into Reload would take the scope away.
const scopedAltMod = `
ACTOR TestScopedWeapon : Weapon
{
  Weapon.AmmoType "TestRifleAmmo"
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
    TSHT A 5 A_FireBullets(0, 0, 1, 10, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestRifleAmmo", 1)
    Goto Ready
  AltFire:
    TSHT B 5 A_FireBullets(0, 0, 1, 20, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestRifleAmmo", 1)
    TNT1 A 0 A_GiveInventory("TestRifleAmmo", 1)
    Goto Ready
  Reload:
    TSHT C 10
    TNT1 A 0 A_GiveInventory("TestRifleAmmo", 20)
    Goto Ready
  }
}

ACTOR TestRifleAmmo : Ammo
{
  Inventory.MaxAmount 20
}
`

func TestUnifyReloadOnAltFireMovesADisguisedReload(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": reloadOnAltMod})
	result, err := CompileWeapon(idx, DefaultPolicy(), "TestReloadOnAltWeapon")
	require.NoError(t, err)

	changed := result.UnifyReloadOnAltFire()
	assert.True(t, changed, "should have detected and moved the disguised reload")

	machine := result.Weapon.States
	reloadIndex, hasReload := machine.Label("reload")
	altIndex, hasAlt := machine.Label("altfire")
	require.True(t, hasReload)
	require.True(t, hasAlt)
	assert.Equal(t, altIndex, reloadIndex, "reload should alias the same steps altfire already had")

	readyIndex, ok := machine.Label("ready")
	require.True(t, ok)
	readyOp := firstWeaponReadyFrom(t, machine, readyIndex)
	assert.False(t, readyOp.AllowAlt, "alt-fire should be off on the frame that used to allow it")
	assert.True(t, readyOp.AllowReload, "reload should be on wherever alt-fire used to be")

	// ReadyFull never allowed alt-fire to begin with - WRF_NOSECONDARY, the
	// mod's own way of saying there is nothing to reload - and unification
	// must not hand it a button it never had either.
	fullIndex, ok := machine.Label("readyfull")
	require.True(t, ok)
	fullOp := firstWeaponReadyFrom(t, machine, fullIndex)
	assert.False(t, fullOp.AllowReload, "a frame that never allowed alt-fire should not gain reload either")
}

// firstWeaponReadyFrom walks forward a few steps from a label looking for its
// weapon_ready op, since a label starting on a jump_if_inventory guard (as
// Ready and ReadyFull both do here) does not put the op on the label's own
// step.
func firstWeaponReadyFrom(t *testing.T, machine wep.StateMachine, from int) wep.Op {
	t.Helper()
	for i := from; i < len(machine.Steps) && i < from+5; i++ {
		for _, op := range machine.Steps[i].Ops {
			if op.Op == wep.OpWeaponReady {
				return op
			}
		}
	}
	t.Fatalf("no weapon_ready op found within 5 steps of index %d", from)
	return wep.Op{}
}

func TestUnifyReloadOnAltFireLeavesARealSecondaryFireAlone(t *testing.T) {
	idx := indexFromFiles(t, map[string]string{"DECORATE": scopedAltMod})
	result, err := CompileWeapon(idx, DefaultPolicy(), "TestScopedWeapon")
	require.NoError(t, err)

	changed := result.UnifyReloadOnAltFire()
	assert.False(t, changed, "a weapon with its own Reload and a firing AltFire must not be touched")

	machine := result.Weapon.States
	for _, step := range machine.Steps {
		for _, o := range step.Ops {
			if o.Op == wep.OpWeaponReady {
				assert.True(t, o.AllowAlt, "alt-fire must stay on for a real secondary attack")
			}
		}
	}
}
