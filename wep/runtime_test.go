package wep

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const oneTic = 1.0 / TicRate

// selectFireWeapon is the shape every weapon has, migrated or hand written: a
// Select that loops on A_Raise until the weapon is up, a Ready that loops
// waiting for the trigger, a Fire that goes off once, and a Deselect that loops
// on A_Lower until the weapon is away.
func selectFireWeapon() *Weapon {
	states := StateMachine{
		Labels: map[string]int{"select": 0, "ready": 2, "fire": 4, "deselect": 6},
		Steps: []Step{
			{Sprite: "GUNA0", Tics: 1, Ops: []Op{{Op: OpRaise}}},
			{Tics: 0, Ops: []Op{{Op: OpLoop}}},

			{Sprite: "GUNA0", Tics: 1, Ops: []Op{{Op: OpWeaponReady, AllowFire: true}}},
			{Tics: 0, Ops: []Op{{Op: OpLoop}}},

			{Sprite: "GUNB0", Tics: 2, Ops: []Op{{Op: OpFireBullets, Count: 1, Damage: 5}}},
			{Tics: 0, Ops: []Op{{Op: OpGoto, To: "ready"}}},

			{Sprite: "GUNA0", Tics: 1, Ops: []Op{{Op: OpLower}}},
			{Tics: 0, Ops: []Op{{Op: OpLoop}}},
		},
	}
	return &Weapon{Format: Format, ID: "test_gun", Tag: "Test Gun", States: states}
}

func run(cursor *Cursor, runtime *Runtime, tics int) []Op {
	var effects []Op
	for i := 0; i < tics; i++ {
		effects = append(effects, cursor.Update(runtime, oneTic)...)
	}
	return effects
}

func countOps(effects []Op, name string) int {
	count := 0
	for _, op := range effects {
		if op.Op == name {
			count++
		}
	}
	return count
}

// A weapon is equipped by starting on Select with the weapon out of sight. It
// has to arrive at Ready under its own steam: A_Raise loops until it is up, and
// nothing else moves it. Getting this wrong leaves every weapon stuck in its
// raise loop, unable to ever fire.
func TestRaisingAWeaponArrivesAtReadyAndFires(t *testing.T) {
	weapon := selectFireWeapon()
	runtime := NewRuntime(weapon, nil)
	cursor := NewCursor(&weapon.States, "Select")
	cursor.StartRaise()

	require.InDelta(t, 1.0, cursor.Raised(), 0.001, "it starts fully out of sight")

	run(cursor, runtime, 3)
	assert.Less(t, cursor.Raised(), 1.0, "and comes up as it loops")

	run(cursor, runtime, 10)
	assert.InDelta(t, 0.0, cursor.Raised(), 0.001, "until it is all the way up")

	_, onReady := cursor.Ready()
	assert.True(t, onReady, "and then it is waiting for the trigger")

	// The shot goes off on the tic the trigger is read, and its frame lasts
	// two, so two tics is exactly one shot. The third would arrive back at
	// Ready and fire again, because the trigger is still down.
	runtime.Input.Trigger = true
	assert.Equal(t, 1, countOps(run(cursor, runtime, 2), OpFireBullets))
	assert.Equal(t, 1, countOps(run(cursor, runtime, 1), OpFireBullets),
		"a held trigger fires again as soon as the weapon is ready")
}

// Putting a weapon away is the same loop the other way, and it stops once the
// weapon is gone rather than looping forever.
func TestLoweringAWeaponPutsItAway(t *testing.T) {
	weapon := selectFireWeapon()
	runtime := NewRuntime(weapon, nil)
	cursor := NewCursor(&weapon.States, "Deselect")

	run(cursor, runtime, 20)
	assert.InDelta(t, 1.0, cursor.Raised(), 0.001)
	assert.True(t, cursor.Stopped())
}

func TestInventoryRespectsItsCaps(t *testing.T) {
	caps := Caps{"shells": 3}
	inventory := Inventory{}

	inventory.Give(caps, "shells", 2)
	assert.Equal(t, 2, inventory["shells"])
	assert.False(t, inventory.AtMax(caps, "shells"))

	inventory.Give(caps, "shells", 5)
	assert.Equal(t, 3, inventory["shells"], "a cap is a cap")
	assert.True(t, inventory.AtMax(caps, "shells"))

	inventory.Take("shells", 99)
	assert.Equal(t, 0, inventory["shells"], "and nothing goes negative")

	// An item with no cap is never at its maximum, however much of it there is.
	inventory.Give(caps, "uncapped", 100)
	assert.False(t, inventory.AtMax(caps, "uncapped"))
}
