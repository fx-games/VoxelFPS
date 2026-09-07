package zdwep

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// variantTestMod is shaped like Final Doomer's own katana: one weapon whose
// Fire branches on an inventory flag ("GotEnergyMode") nothing here ever
// gives, between two otherwise-complete attack sequences that share nothing
// but their entry point.
const variantTestMod = `
ACTOR TestVariantWeapon : Weapon
{
  Weapon.AmmoType "TestVariantAmmo"
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
    TNT1 A 0 A_JumpIfInventory("GotEnergyMode", 1, "FireEnergy")
  FireNormal:
    TSHT B 5 A_FireBullets(0, 0, 1, 10, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestVariantAmmo", 1)
    Goto Ready
  FireEnergy:
    TSHT C 5 A_FireBullets(0, 0, 1, 30, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestVariantAmmo", 1)
    Goto Ready
  }
}

ACTOR TestVariantAmmo : Ammo
{
  Inventory.MaxAmount 50
}
`

func compileTestVariantWeapon(t *testing.T) *Result {
	t.Helper()
	idx := indexFromFiles(t, map[string]string{"DECORATE": variantTestMod})
	result, err := CompileWeapon(idx, DefaultPolicy(), "TestVariantWeapon")
	require.NoError(t, err)
	return result
}

// TestDropUnreachableStatesRemovesTheModeNothingGives is the default
// (normal) compile: GotEnergyMode is never given, so FireEnergy is dead and
// DropUnreachableStates removes it, the same way it does for the actual
// katana's own Energy branch today, with no ForceItemGiven involved at all.
func TestDropUnreachableStatesRemovesTheModeNothingGives(t *testing.T) {
	result := compileTestVariantWeapon(t)
	dropped := result.DropUnreachableStates()
	assert.Greater(t, dropped, 0)

	machine := result.Weapon.States
	_, hasEnergy := machine.Label("fireenergy")
	_, hasNormal := machine.Label("firenormal")
	assert.False(t, hasEnergy, "FireEnergy is never reached and should be gone")
	assert.True(t, hasNormal, "FireNormal is the only reachable branch and must survive")
}

// TestForceItemGivenSwapsWhichModeSurvives is the split: forcing the flag on
// before dropping makes the energy branch the one that survives, and the
// normal one the dead weight instead - proving the mechanism works both
// ways, which is what turns one compiled weapon into two.
func TestForceItemGivenSwapsWhichModeSurvives(t *testing.T) {
	result := compileTestVariantWeapon(t)
	result.ForceItemGiven("GotEnergyMode")
	dropped := result.DropUnreachableStates()
	assert.Greater(t, dropped, 0)

	machine := result.Weapon.States
	_, hasEnergy := machine.Label("fireenergy")
	_, hasNormal := machine.Label("firenormal")
	assert.True(t, hasEnergy, "forcing the flag should keep FireEnergy reachable")
	assert.False(t, hasNormal, "FireNormal is unreachable once the flag is always true")

	// The load-time check in wep/load.go rejects any jump left aimed at a
	// label that no longer exists - confirm nothing does.
	for i, step := range machine.Steps {
		for _, op := range step.Ops {
			if op.To == "" {
				continue
			}
			_, ok := machine.Label(op.To)
			assert.True(t, ok, "step %d: op %q jumps to %q, which no longer exists", i, op.Op, op.To)
		}
	}
}
