package zdwep

import (
	"math/rand"
	"testing"

	"github.com/memmaker/goray/wep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneTic is how long a single Doom tic lasts.
const oneTic = 1.0 / wep.TicRate

type harness struct {
	weapon  *wep.Weapon
	cursor  *wep.Cursor
	runtime *wep.Runtime
	effects []wep.Op
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	result := compileTestShotgun(t)
	runtime := wep.NewRuntime(result.Weapon, rand.New(rand.NewSource(1)))
	return &harness{
		weapon:  result.Weapon,
		cursor:  wep.NewCursor(&result.Weapon.States, "Ready"),
		runtime: runtime,
	}
}

// run advances the weapon by a number of tics, collecting everything it asked
// the world to do.
func (h *harness) run(tics int) {
	for i := 0; i < tics; i++ {
		h.effects = append(h.effects, h.cursor.Update(h.runtime, oneTic)...)
	}
}

func (h *harness) firstOp(name string) *wep.Op {
	for i := range h.effects {
		if h.effects[i].Op == name {
			return &h.effects[i]
		}
	}
	return nil
}

func (h *harness) countOp(name string) int {
	count := 0
	for _, op := range h.effects {
		if op.Op == name {
			count++
		}
	}
	return count
}

// The whole point of the exercise. With shells in reserve and an empty chamber,
// pulling the trigger fires: it does not walk into the pump animation, which is
// what the old runtime did because it took every A_JumpIfInventory.
func TestPullingTheTriggerFiresRatherThanPumping(t *testing.T) {
	h := newHarness(t)

	// Idling with ammo, the script parks on its firing-capable ready frame.
	h.run(2)
	ready, onReady := h.cursor.Ready()
	require.True(t, onReady, "the weapon rests on a ready frame")
	assert.True(t, ready.AllowFire, "with shells in reserve the trigger works")

	h.runtime.Input.Trigger = true
	h.run(1)

	shot := h.firstOp(wep.OpFireBullets)
	require.NotNil(t, shot, "the trigger fired the weapon")
	assert.Equal(t, 10, shot.Count)

	assert.NotNil(t, h.firstOp(wep.OpGunFlash), "and started the flash overlay")
	assert.NotNil(t, h.firstOp(wep.OpMakeNoise), "and made a noise")
	assert.NotNil(t, h.firstOp(wep.OpSpawnActor), "and put smoke in the air")

	// One round chambered, one shell gone from the reserve.
	assert.Equal(t, 1, h.runtime.Inventory["TestChambered"])
	assert.Equal(t, 49, h.runtime.Inventory["TestShells"])
}

// An empty gun rests on the frame its script marks WRF_NOFIRE, so the trigger
// does nothing at all rather than being blocked by a special case in the
// engine.
func TestAnEmptyWeaponRestsOnItsNoFireFrame(t *testing.T) {
	h := newHarness(t)
	h.runtime.Inventory["TestShells"] = 0

	h.run(2)
	ready, onReady := h.cursor.Ready()
	require.True(t, onReady)
	assert.False(t, ready.AllowFire)

	h.runtime.Input.Trigger = true
	h.run(10)
	assert.Nil(t, h.firstOp(wep.OpFireBullets), "an empty gun does not go off")
}

// Once the chamber is full the same jump that used to misfire does its job: the
// weapon pumps instead of firing again.
func TestAFullChamberSendsTheWeaponToThePump(t *testing.T) {
	h := newHarness(t)
	h.runtime.Inventory["TestChambered"] = 3 // at its maximum

	h.run(2)
	h.runtime.Input.Trigger = true
	h.run(4)

	assert.Nil(t, h.firstOp(wep.OpFireBullets), "a full chamber has to be worked first")
	index, ok := h.weapon.States.Label("Pump")
	require.True(t, ok)
	assert.Equal(t, "TSHTA0", h.weapon.States.Steps[index].Sprite)
}

// With no shells left and nothing chambered the script falls through to its
// dryfire click, which is the branch the numeric jump exists to skip.
func TestNoShellsAtAllDryfires(t *testing.T) {
	h := newHarness(t)
	h.runtime.Inventory["TestShells"] = 0

	// Force the weapon into its firing states without going through the ready
	// frame, which would refuse the trigger.
	require.True(t, h.cursor.Goto("Fire"))
	h.run(3)

	sound := h.firstOp(wep.OpPlaySound)
	require.NotNil(t, sound)
	assert.Equal(t, "weapons/dryfire", sound.Sound)
	assert.Nil(t, h.firstOp(wep.OpFireBullets))
}

// Holding the trigger runs the weapon round again through A_ReFire; letting go
// stops it.
func TestHoldingTheTriggerKeepsFiring(t *testing.T) {
	h := newHarness(t)
	h.runtime.Input.Trigger = true
	h.run(40)
	held := h.countOp(wep.OpFireBullets)
	assert.Greater(t, held, 1, "a held trigger fires more than once")

	released := newHarness(t)
	released.runtime.Input.Trigger = true
	released.run(2)
	released.runtime.Input.Trigger = false
	released.run(40)
	assert.Equal(t, 1, released.countOp(wep.OpFireBullets), "letting go stops after one shot")
}

// The flash overlay runs on its own cursor, at the same time as the weapon's
// own animation, which is how ZDoom draws a muzzle flash over the gun.
func TestTheFlashOverlayRunsAlongsideTheWeapon(t *testing.T) {
	h := newHarness(t)
	h.runtime.Input.Trigger = true
	h.run(1)

	flash := h.firstOp(wep.OpGunFlash)
	require.NotNil(t, flash)

	overlay := wep.NewCursor(&h.weapon.States, flash.To)
	var lights []wep.Op
	for i := 0; i < 6; i++ {
		lights = append(lights, overlay.Update(h.runtime, oneTic)...)
		h.cursor.Update(h.runtime, oneTic)
	}

	require.Len(t, lights, 2)
	assert.Equal(t, wep.OpFlashLight, lights[0].Op)
	assert.Equal(t, 1, lights[0].Level)
	assert.Equal(t, 0, lights[1].Level)
	assert.True(t, overlay.Stopped(), "the overlay ends on its own Stop")
}

// A spawned actor runs the same stepper with no input, and finishes.
func TestASpawnedActorRunsAndFinishes(t *testing.T) {
	result := compileTestShotgun(t)
	smoke := result.Actors["testsmoke"]
	require.NotNil(t, smoke)

	runtime := &wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}}
	cursor := wep.NewCursor(&smoke.States, "Spawn")

	seen := map[string]bool{}
	for i := 0; i < 40 && !cursor.Stopped(); i++ {
		if step := cursor.Step(); step != nil && step.Sprite != "" {
			seen[step.Sprite] = true
		}
		cursor.Update(runtime, oneTic)
	}
	assert.True(t, cursor.Stopped())
	assert.Len(t, seen, 5, "all five smoke frames were shown")
}

// A weapons folder somebody edited into a circle of instant steps has to
// complain rather than lock the frame up.
func TestACircleOfInstantStepsGivesUp(t *testing.T) {
	machine := wep.StateMachine{
		Steps: []wep.Step{
			{Tics: 0, Ops: []wep.Op{{Op: wep.OpGoto, To: "spin"}}},
		},
		Labels: map[string]int{"spin": 0},
	}
	cursor := wep.NewCursor(&machine, "spin")
	effects := cursor.Update(&wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}}, oneTic)

	require.NotEmpty(t, effects)
	assert.Equal(t, wep.OpStop, effects[len(effects)-1].Op)
	assert.True(t, cursor.Stopped())
}
