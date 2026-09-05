# 3. One inventory map for ammo, magazines and flags

Date: 2026-09-05

## Status

Accepted

## Context

ZDoom weapon scripts branch on three things that look different but are the
same underneath: reserve ammo (`Weapon.AmmoType`), rounds chambered
(`FDHellboundShotgunShots`, max 3), and boolean state
(`FDHellboundShotgunOverloaded`). All three are inventory items with an amount
and a maximum, manipulated by the same four functions.

We could model them as three engine concepts: ammo pools, per-weapon magazines,
and weapon flags.

## Decision

One `map[string]int` inventory per player, plus a cap table compiled from each
actor's `Inventory.MaxAmount`. Reserve ammo, chambered rounds and flags are all
entries in it.

`A_JumpIfInventory(item, n, target)` keeps ZDoom's semantics exactly: `n > 0`
jumps when the amount is at least `n`; `n == 0` jumps when the item is at its
maximum.

Whether a weapon can fire or reload at a given moment is **not** a weapon
property. It comes from the `A_WeaponReady` flags on the frame the weapon is
currently idling on (`WRF_NOFIRE`, `WRF_ALLOWRELOAD`), which the scripts choose
by branching on inventory. The stepper accepts trigger and reload input only
while resting on a `weapon_ready` op that permits it.

## Consequences

- Three mechanisms collapse to one map and one cap table.
- Cross-weapon ammo sharing, pickup caps and "gun is empty, trigger does
  nothing" all fall out without special cases.
- An empty gun idles on its own no-fire frame because its script says so, rather
  than because the engine special-cased it.
- Weapons with no `Reload` state label are not reloadable, and the reload key is
  a no-op on them. Some weapons, this shotgun included, pump inside `Fire`
  instead.
- Inventory is player-wide and persists across weapon switches, matching ZDoom.
- Ammo and weapon pickups in the world are out of scope until pickup entities
  exist.
