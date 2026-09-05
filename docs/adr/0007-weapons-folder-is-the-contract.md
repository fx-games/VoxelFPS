# 7. The weapons folder is the engine's contract, validated on load

Date: 2026-09-05

## Status

Accepted

## Context

The engine currently hardcodes its weapons: a slice built in `main.go`, each
entry naming a PNG in `assets/` and a Go enum value. Adding a weapon means
editing and rebuilding.

Weapons are moving to an external folder the engine reads at startup. That
folder is hand-editable, so it is a trust boundary, even though its contents are
normally produced by a tool that already guarantees their correctness.

## Decision

`./weapons` is the engine's weapon contract. At startup it reads every
subdirectory as a weapon, plus `keys.txt` binding number keys to weapon ids.

Loading validates: format version, that every jump target names an existing
label, that every sprite resolves in the folder's atlas, that every sound
resolves to a file, that every op is known to this build, that every referenced
actor is present, and that `keys.txt` uses keys 1-9 without duplicates and names
only weapons that loaded.

**Every error is collected, then the engine refuses to start.** Not
first-error-abort, not skip-the-broken-weapon.

Nothing in the folder is Doom-specific. A hand-written weapon and a migrated one
are the same thing.

## Consequences

- Adding, removing or retuning a weapon is a data change.
- Skipping a broken weapon would silently change the loadout and leave the
  player to discover it mid-game; aborting on the first error would mean one run
  per typo. Collecting costs a slice and reports everything at once.
- The format version field turns a stale `weapons/` folder into one clear
  message instead of a confusing failure several checks later.
- `SheetWeapon` and the `Weapon` interface both delete: with weapons as data
  there is one weapon type, so the interface has one implementation and stops
  earning its keep.
- The folder must be present. A build with no `weapons/` has no weapons, and
  says so.
