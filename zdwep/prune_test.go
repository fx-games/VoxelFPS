package zdwep

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneTestMod is a small weapon shaped like BDCE's own: Ready checks an
// inventory flag ("Kicking") this test never gives to decide whether to
// branch into a kick, and both the kick and an entirely separate, genuinely
// reachable path (Reload) end up at a shared "GoingToReady" utility label.
// Nothing here ever gives Kicking or Taunting, so DoKick and Taunt are dead
// by the same reasoning BDCE's own weapons are; GoingToReady is not, because
// Reload also uses it.
const pruneTestMod = `
ACTOR TestPruneWeapon : Weapon
{
  Weapon.AmmoType "TestPruneAmmo"
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
    TNT1 A 0 A_JumpIfInventory("Kicking", 1, "DoKick")
    TNT1 A 0 A_JumpIfInventory("Taunting", 1, "Taunt")
    TSHT A 1 A_WeaponReady
    Loop
  Fire:
    TSHT A 5 A_FireBullets(0, 0, 1, 10, "TestPuff")
    TNT1 A 0 A_TakeInventory("TestPruneAmmo", 1)
    Goto Ready
  Reload:
    TSHT A 10
    Goto GoingToReady
  GoingToReady:
    TSHT A 1
    Goto Ready
  DoKick:
    TSHT B 10
    Goto GoingToReady
  Taunt:
    TSHT C 20
    Goto Ready
  }
}

ACTOR TestPruneAmmo : Ammo
{
  Inventory.MaxAmount 50
}
`

func compileTestPruneWeapon(t *testing.T) *Result {
	t.Helper()
	idx := indexFromFiles(t, map[string]string{"DECORATE": pruneTestMod})
	result, err := CompileWeapon(idx, DefaultPolicy(), "TestPruneWeapon")
	require.NoError(t, err)
	return result
}

// TestPruneDeadStatesRemovesUnreachableKickAndTaunt is the direct case: an
// inventory flag nothing gives gates a state nothing outside it names, so
// reachableSteps and namedAnimationClosure agree and it goes.
func TestPruneDeadStatesRemovesUnreachableKickAndTaunt(t *testing.T) {
	result := compileTestPruneWeapon(t)
	before := len(result.Weapon.States.Steps)

	dropped := result.PruneDeadStates()

	assert.Greater(t, dropped, 0)
	assert.Equal(t, before-dropped, len(result.Weapon.States.Steps))
	_, hasKick := result.Weapon.States.Label("DoKick")
	_, hasTaunt := result.Weapon.States.Label("Taunt")
	assert.False(t, hasKick, "DoKick should have been pruned")
	assert.False(t, hasTaunt, "Taunt should have been pruned")
}

// TestPruneDeadStatesKeepsALabelASurvivorStillNames is the rescue case: even
// though DoKick reaches GoingToReady too, Reload's own live jump there means
// GoingToReady must survive - and every op reachable code still carries has
// to keep landing on a label that exists (wep/load.go rejects one that does
// not), so both the label and every remaining jump into it are checked.
func TestPruneDeadStatesKeepsALabelASurvivorStillNames(t *testing.T) {
	result := compileTestPruneWeapon(t)
	result.PruneDeadStates()

	machine := result.Weapon.States
	_, ok := machine.Label("GoingToReady")
	require.True(t, ok, "GoingToReady should survive - Reload still reaches it")

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
