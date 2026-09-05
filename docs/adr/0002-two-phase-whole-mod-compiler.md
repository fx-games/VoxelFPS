# 2. The migration compiler resolves against a whole-mod actor index

Date: 2026-09-05

## Status

Accepted

Extends [1. Compile ZDoom weapon scripts to a state graph at migration time](0001-compiled-weapon-state-graph.md).

## Context

`zdwep.ParseActorScript` parses one file's text and returns one weapon. It
records the parent class name only to guess whether the actor is a weapon
(`isWeaponType` tests the parent name for the substring "weapon") and then
discards it. Inheritance is never resolved, and actors that are not weapons are
never parsed at all.

That is not enough to compile behaviour that depends on ammo and inventory:

- `A_JumpIfInventory("FDHellboundShotgunShots", 0, ...)` means "jump if the item
  is at its maximum amount". The maximum lives on a different actor,
  `FDHellboundShotgunShots : FDCounter { inventory.maxamount 3 }`.
- `Weapon.AmmoType "FDHellboundShells"` names an ammo actor whose
  `Inventory.MaxAmount` is the reserve cap.
- `ACTOR FDHellboundShotgun : FDWeapon` may inherit `Ready`, `Select` or
  `Deselect` states that the child does not override.

## Decision

The migration tool runs in two phases over the whole mod.

**Phase 1 — index.** Walk every DECORATE-reachable file and parse *every* actor
into a neutral record: name, parent, properties, flags, state labels. No weapon
filtering, no interpretation, no resolution. Produces `map[string]Actor`.

**Phase 2 — resolve.** For each actor whose resolved parent chain reaches
`Weapon`: flatten the chain (child overrides parent, states merge by label),
resolve ammo and counter actors to their caps, sounds through SNDINFO to files,
sprites to atlas indices, ACS calls to constants, and lower the states to the
closed instruction set. Anything unresolved is an error naming the actor and the
source line.

Both phases run only in the migration tool. The engine never performs either.

## Consequences

- Weapon detection becomes correct rather than substring-based: a weapon is an
  actor whose resolved chain reaches `Weapon`.
- The tool must tolerate parsing actors it will never emit, so the state-line
  parser has to fail softly during indexing (today `parseStateLine` panics on
  any line its regex does not match).
- Compile time grows with mod size, which is irrelevant — it runs offline.
- We do not reproduce GZDoom's parser. `FScanner`, the expression compiler,
  `ParseOldDecoration` and `replaces` stay unimplemented until a weapon we
  actually want fails without them.
