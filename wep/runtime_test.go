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

// A script keeps its own timers in user variables and branches on them. That
// branch cannot be settled at migration time without changing what the weapon
// does, so it arrives as an expression and the stepper runs it.
func TestUserVariablesDriveAJump(t *testing.T) {
	// Count down from three, and go to Done when the counter runs out.
	countdown := Expr{
		{Kind: TokenVar, Name: "user_life"},
		{Kind: TokenValue, Value: 1},
		{Kind: TokenOp, Name: "-"},
	}
	spent := Expr{
		{Kind: TokenVar, Name: "user_life"},
		{Kind: TokenValue, Value: 0},
		{Kind: TokenOp, Name: "<="},
	}
	machine := StateMachine{
		Labels: map[string]int{"spawn": 0, "done": 3},
		Steps: []Step{
			{Sprite: "SMOKA0", Tics: 1, Ops: []Op{{Op: OpSetVar, Item: "user_life", Set: countdown}}},
			{Tics: 0, Ops: []Op{{Op: OpJumpIf, Cond: spent, To: "done"}}},
			{Tics: 0, Ops: []Op{{Op: OpGoto, To: "spawn"}}},
			{Tics: 0, Ops: []Op{{Op: OpStop}}},
		},
	}

	runtime := &Runtime{Inventory: Inventory{}, Caps: Caps{}, Vars: map[string]float64{}}
	runtime.SetVar("user_life", 3)
	cursor := NewCursor(&machine, "spawn")

	run(cursor, runtime, 2)
	assert.Equal(t, 1.0, runtime.Vars["user_life"])
	assert.False(t, cursor.Stopped(), "still going round")

	run(cursor, runtime, 2)
	assert.True(t, cursor.Stopped(), "the counter ran out and it ended itself")
}

// The jumps that ask about the surroundings get an answer now, not later. A
// game with nothing to see answers no, and the jump is not taken - which is a
// different thing from the op having been dropped.
func TestWorldJumpsAreAnsweredNotDropped(t *testing.T) {
	machine := StateMachine{
		Labels: map[string]int{"spawn": 0, "landed": 2},
		Steps: []Step{
			{Sprite: "CASGA0", Tics: 1, Ops: []Op{{Op: OpCheckFloor, To: "landed"}}},
			{Tics: 0, Ops: []Op{{Op: OpGoto, To: "spawn"}}},
			{Sprite: "CASGB0", Tics: 1, Ops: []Op{{Op: OpStop}}},
		},
	}

	falling := &Runtime{Inventory: Inventory{}, Caps: Caps{}}
	cursor := NewCursor(&machine, "spawn")
	run(cursor, falling, 6)
	assert.False(t, cursor.Stopped(), "nothing underneath, so it keeps falling")

	landed := &Runtime{Inventory: Inventory{}, Caps: Caps{}, World: groundUnderfoot{}}
	cursor = NewCursor(&machine, "spawn")
	run(cursor, landed, 3)
	assert.True(t, cursor.Stopped(), "something underneath, so it settles")
}

// groundUnderfoot is a world with a floor and nothing else.
type groundUnderfoot struct{ NoWorld }

func (groundUnderfoot) Below() bool { return true }

// A script that resizes something relative to how big it already is cannot have
// that worked out at migration time: `scalex * frandom(0.08, 0.14)` folds to
// zero, because scalex is not known yet, and a smoke puff that should be a
// thirtieth of its size comes out at none of it. So the arguments ship.
func TestArgumentsThatDependOnTheActorAreWorkedOutWhileItRuns(t *testing.T) {
	shrink := Expr{
		{Kind: TokenVar, Name: "scalex"},
		{Kind: TokenValue, Value: 0.1},
		{Kind: TokenOp, Name: "*"},
	}
	op := Op{Op: OpSetScale, Args: []float64{0, 0}, ArgExprs: []Expr{shrink, shrink}}

	runtime := &Runtime{Vars: map[string]float64{"scalex": 0.3}}
	values := op.ArgValues(runtime, nil)
	require.Len(t, values, 2)
	assert.InDelta(t, 0.03, values[0], 0.0001)

	// The same op read as plain numbers is the bug it exists to prevent.
	assert.Equal(t, 0.0, op.Args[0])

	// An op whose arguments all settled at migration time keeps carrying them.
	plain := Op{Op: OpSetScale, Args: []float64{2, 2}}
	assert.Equal(t, []float64{2, 2}, plain.ArgValues(runtime, nil))
}
