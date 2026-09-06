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

	_, y := cursor.Offsets()
	require.InDelta(t, WeaponBottom, y, 0.001, "it starts fully out of sight")

	run(cursor, runtime, 3)
	_, y = cursor.Offsets()
	assert.Less(t, y, float64(WeaponBottom), "and comes up as it loops")

	run(cursor, runtime, 20)
	_, y = cursor.Offsets()
	assert.InDelta(t, WeaponTop, y, 0.001, "until it is all the way up")

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
	_, y := cursor.Offsets()
	assert.InDelta(t, WeaponBottom, y, 0.001)
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

// A_ReFire is what a chaingun's firing loop ends on, and where it goes back to
// decides the weapon's rate of fire. ZDoom re-enters the Hold state, not Fire:
// Fire is where the wind-up lives, and a minigun that spun up again between
// every pair of bullets fired three times a second instead of seventeen.
func TestRefireGoesBackToHoldNotThroughTheWindUp(t *testing.T) {
	weapon := &Weapon{Format: Format, ID: "spinner", States: StateMachine{
		Labels: map[string]int{"ready": 0, "fire": 2, "hold": 4},
		Steps: []Step{
			{Sprite: "GUNA0", Tics: 1, Ops: []Op{{Op: OpWeaponReady, AllowFire: true}}},
			{Tics: 0, Ops: []Op{{Op: OpLoop}}},

			{Sprite: "SPINA0", Tics: 10}, // the wind-up, which costs ten tics
			{Tics: 0, Ops: []Op{{Op: OpGoto, To: "hold"}}},

			{Sprite: "GUNB0", Tics: 1, Ops: []Op{{Op: OpFireBullets, Count: 1, Damage: 5}}},
			{Tics: 0, Ops: []Op{{Op: OpRefire, To: "fire"}}},
			{Tics: 0, Ops: []Op{{Op: OpGoto, To: "ready"}}},
		},
	}}
	runtime := NewRuntime(weapon, nil)
	cursor := NewCursor(&weapon.States, "Ready")
	runtime.Input.Trigger = true

	// The wind-up costs ten tics and nothing is fired while it runs.
	assert.Equal(t, 0, countOps(run(cursor, runtime, 8), OpFireBullets),
		"the wind-up holds the first shot off")
	run(cursor, runtime, 4)

	// Past it, the Hold loop is one shot a tic, and it stays there.
	assert.Equal(t, 10, countOps(run(cursor, runtime, 10), OpFireBullets),
		"every tic after the wind-up is a shot")

	// Letting go drops out of the loop, and the next press winds up again.
	runtime.Input.Trigger = false
	run(cursor, runtime, 2)
	runtime.Input.Trigger = true
	assert.Equal(t, 0, countOps(run(cursor, runtime, 8), OpFireBullets),
		"and letting go winds it up again")
}

// The second attack button is its own chain in CheckWeaponFire: a ready frame
// says separately whether it takes the trigger and whether it takes the alt
// button, and FireWeaponAlt does nothing at all on a weapon that has no
// AltFire state - it does not fall back on the primary one.
func TestAltFireIsItsOwnButtonAndItsOwnState(t *testing.T) {
	scoped := func(allowFire, allowAlt bool, withAltFire bool) *Cursor {
		labels := map[string]int{"ready": 0, "fire": 2, "altfire": 4}
		if !withAltFire {
			delete(labels, "altfire")
		}
		machine := StateMachine{
			Labels: labels,
			Steps: []Step{
				{Sprite: "GUNA0", Tics: 1, Ops: []Op{{Op: OpWeaponReady, AllowFire: allowFire, AllowAlt: allowAlt}}},
				{Tics: 0, Ops: []Op{{Op: OpLoop}}},

				{Sprite: "GUNB0", Tics: 1, Ops: []Op{{Op: OpFireBullets, Count: 1, Damage: 5}}},
				{Tics: 0, Ops: []Op{{Op: OpGoto, To: "ready"}}},

				{Sprite: "GUNC0", Tics: 1, Ops: []Op{{Op: OpZoom, Args: []float64{4}}}},
				{Tics: 0, Ops: []Op{{Op: OpGoto, To: "ready"}}},
			},
		}
		return NewCursor(&machine, "Ready")
	}
	alt := Input{Alt: true}

	// A frame that takes both: the alt button reaches AltFire, and the trigger
	// still reaches Fire.
	runtime := &Runtime{Inventory: Inventory{}, Caps: Caps{}, Input: alt}
	assert.Positive(t, countOps(run(scoped(true, true, true), runtime, 4), OpZoom),
		"the alt button starts the weapon's own AltFire")

	runtime = &Runtime{Inventory: Inventory{}, Caps: Caps{}, Input: Input{Trigger: true}}
	assert.Positive(t, countOps(run(scoped(true, true, true), runtime, 4), OpFireBullets),
		"and the trigger still starts Fire")

	// WRF_NOSECONDARY: the scoped railgun rests on such a frame.
	runtime = &Runtime{Inventory: Inventory{}, Caps: Caps{}, Input: alt}
	assert.Equal(t, 0, countOps(run(scoped(true, false, true), runtime, 4), OpZoom),
		"a frame that refuses the second button stays put")

	// WRF_NOPRIMARY is the other way round, and is what a scoped weapon rests
	// on so that only unscoping works.
	runtime = &Runtime{Inventory: Inventory{}, Caps: Caps{}, Input: alt}
	assert.Positive(t, countOps(run(scoped(false, true, true), runtime, 4), OpZoom),
		"a frame that refuses the trigger still takes the second button")

	// And a weapon with nothing on the button ignores it rather than firing.
	runtime = &Runtime{Inventory: Inventory{}, Caps: Caps{}, Input: alt}
	assert.Equal(t, 0, countOps(run(scoped(true, true, false), runtime, 4), OpFireBullets),
		"no AltFire state means the second button does nothing at all")
}

// An item can be a pulse rather than a state: ZDoom's Powerup.Duration gives
// it a lifetime, and Final Doomer's C4 detonator is an item that exists for
// two tics - long enough for every charge in the air to see it, short enough
// that the next charge thrown is not blown up on the way out. Without the
// clock the item stays for ever and every later charge goes off instantly.
func TestAnItemWithADurationTakesItselfBack(t *testing.T) {
	weapon := selectFireWeapon()
	weapon.Items = map[string]Item{"Detonate": {Max: 1, Duration: 2}}
	runtime := NewRuntime(weapon, nil)

	runtime.Inventory.Give(runtime.Caps, "Detonate", 1)
	runtime.StartItemTimer("Detonate")
	assert.Equal(t, 1, runtime.Inventory["Detonate"])

	runtime.Tick(oneTic)
	assert.Equal(t, 1, runtime.Inventory["Detonate"], "still there a tic later")

	runtime.Tick(oneTic)
	assert.Equal(t, 0, runtime.Inventory["Detonate"], "and gone after its two")
}

// An item a mod hands to its own ACS script to take back is one nothing here
// ever takes: Brutal Doom's railgun gives itself "Pumping" and leaves the
// clearing to ACS_NamedExecuteAlways("RailgunRecharge"), and its Fire state
// opens by jumping to Ready while that item is held. Without a clock on it the
// gun fires exactly once and every pull after that walks straight back out.
func TestAnItemAScriptWouldTakeBackExpiresAnyway(t *testing.T) {
	weapon := selectFireWeapon()
	weapon.Items = map[string]Item{"Pumping": {Max: 1}} // no Powerup.Duration: ACS's job
	runtime := NewRuntime(weapon, nil)

	runtime.Inventory.Give(runtime.Caps, "Pumping", 1)
	runtime.StartItemTimer("Pumping")
	assert.Equal(t, 1, runtime.Inventory["Pumping"])

	for tic := 0; tic < scriptHeldItems["pumping"]; tic++ {
		runtime.Tick(oneTic)
	}
	assert.Equal(t, 0, runtime.Inventory["Pumping"], "the recharge is over, so is the lock")
}

// Loop goes back to the top of the state block, and a label the compiler
// invented for a `Goto Ready+6` is not the top of anything. Counted as one,
// Brutal Doom's MP40 looped the nine zero-tic jumps at the bottom of its Ready
// block and never reached the one frame in it that costs a tic.
func TestLoopIgnoresALabelTheCompilerInvented(t *testing.T) {
	machine := &StateMachine{
		Steps: []Step{
			{Tics: 1},                          // 0: the frame the block rests on
			{Tics: 0},                          // 1
			{Tics: 0, Ops: []Op{{Op: OpLoop}}}, // 2
		},
		Labels: map[string]int{"ready": 0, "@ready.1": 1},
	}
	starts := labelStarts(machine)
	assert.Equal(t, []int{0, 0, 0}, starts, "every step belongs to Ready, not to @ready.1")
}

// A "####" frame shows whatever the actor is already showing, and what that is
// can only be known while it runs. Final Doomer's shell casing is the case that
// proves it: FDTNTShellCasing overrides nothing but Spawn, where it swaps its
// parent's 9MMC frames for TNTS ones, and inherits a Death written entirely in
// "####" frames. GZDoom draws a landed shotgun shell with TNTSI; folding the
// marker in at migration time draws the parent's 9mm casing instead, at a third
// of the right size.
func TestKeptSpriteFollowsTheActorNotTheStateItWasWrittenIn(t *testing.T) {
	// What the migration writes for the child: its own Spawn, the parent's
	// Death, and the two pictures the marker can land on.
	casing := func(spawn string) *StateMachine {
		return &StateMachine{
			Labels: map[string]int{"spawn": 0, "death": 2},
			Steps: []Step{
				{Sprite: spawn + "A0", Tics: 2},
				// A Goto shows nothing of its own and blanks nothing either.
				{Sprite: KeepBoth, Tics: 0, Ops: []Op{{Op: OpGoto, To: "death"}}},
				// "####" A 0 - the sprite is kept, the letter is not.
				{Sprite: "####A0", SpriteAlt: []string{"9MMCA0", "TNTSA0"}, Tics: 1},
				// "####" I 1000 - the frame it rests on for good.
				{Sprite: "####I0", SpriteAlt: []string{"9MMCI0", "TNTSI0"}, Tics: -1},
			},
		}
	}

	shell := NewOverlayCursor(casing("TNTS"), "spawn")
	assert.Equal(t, "TNTSA0", shell.Sprite(), "the child's own Spawn frame")
	run(shell, &Runtime{}, 4)
	assert.Equal(t, "TNTSI0", shell.Sprite(),
		"the parent's Death frame, drawn with the child's picture")

	// The parent itself is the same states with its own Spawn, and it keeps
	// showing its own.
	nine := NewOverlayCursor(casing("9MMC"), "spawn")
	run(nine, &Runtime{}, 4)
	assert.Equal(t, "9MMCI0", nine.Sprite())
}

// The other two markers: "####" with a "#" letter keeps the whole name, and
// "----" keeps it whatever letter is written next to it. A marker with nothing
// showing yet shows nothing, which is what a fresh psprite's SPR_TNT1 does.
func TestTheOtherKeepMarkers(t *testing.T) {
	assert.Equal(t, "GUNSB0", ResolveSprite(KeepBoth, nil, "GUNSB0"))
	assert.Equal(t, "", ResolveSprite(KeepBoth, nil, ""))
	assert.Equal(t, "", ResolveSprite("####I0", []string{"GUNSI0"}, ""))
	// A kept letter with nothing before it is 'A', because an actor's frame
	// starts at 0 where its sprite starts at SPR_TNT1.
	assert.Equal(t, "GUNSA0", ResolveSprite("GUNS#0", []string{"GUNSA0", "GUNSB0"}, ""))
	assert.Equal(t, "GUNSB0", ResolveSprite("GUNS#0", []string{"GUNSA0", "GUNSB0"}, "OTHRB0"))
	// An ordinary name is nobody's business but its own.
	assert.Equal(t, "GUNSC0", ResolveSprite("GUNSC0", nil, "GUNSB0"))
	// A weapon's own frames are in its own atlas under the name they were
	// written with, so a kept sprite spells its answer out.
	assert.Equal(t, "GUNSI0", ResolveSprite("####I0", nil, "GUNSB0"))
	// An actor's are in the shared store, where the name carries a hash of the
	// picture and only the migration can have spelled it.
	assert.Equal(t, "TNTSI0.a1b2c3d4",
		ResolveSprite("####I0", []string{"9MMCI0.0f0f0f0f", "TNTSI0.a1b2c3d4"}, "TNTSB0.99887766"))
}

// A Wait is not a frame of its own: it links the frame above it to itself, so
// that frame runs again and again - its action with it. Final Doomer's shell
// casings fade out that way, a hundredth of their alpha per tic, written as
// `"####" I 1 A_FadeTo(0, 0.01, 1)` and a Wait underneath. Parked on the Wait
// instead, the fade ran once and the casing sat there at full alpha for good.
func TestWaitRepeatsTheFrameAboveItActionAndAll(t *testing.T) {
	machine := StateMachine{
		// What the migration writes: the Wait becomes a jump back to the frame
		// it was written under, which is where its label points.
		Labels: map[string]int{"fadeout": 0, "@frame0": 0},
		Steps: []Step{
			{Sprite: "CASGI0", Tics: 1, Ops: []Op{{Op: OpFadeTo, Args: []float64{0, 0.01, 1}}}},
			{Sprite: KeepBoth, Tics: 0, Ops: []Op{{Op: OpGoto, To: "@frame0"}}},
		},
	}

	cursor := NewOverlayCursor(&machine, "fadeout")
	effects := run(cursor, &Runtime{}, 10)
	assert.Equal(t, 10, countOps(effects, OpFadeTo), "one fade a tic, not one in total")
	assert.False(t, cursor.Stopped(), "and it is still going")
	assert.Equal(t, "CASGI0", cursor.Sprite(), "still on the frame above the Wait")
}
