# 1. Compile ZDoom weapon scripts to a state graph at migration time

Date: 2026-09-05

## Status

Accepted

## Context

Weapons are ported from ZDoom/Brutal Doom PK3 archives. Their behaviour lives in
DECORATE `States` blocks: a labelled state machine of sprite frames, tic
durations, action functions (`A_FireBullets`, `A_PlaySound`, `A_GiveInventory`)
and conditional jumps (`A_JumpIfInventory`, `A_ReFire`, `A_JumpIf`).

The engine today parses that DECORATE text at startup (`zdwep.ParseActorScript`)
and walks it with a regex-driven interpreter (`doom_weapon.go`). The interpreter
implements one action function, `A_JumpIfInventory`, and takes the jump
unconditionally because there is no inventory. Everything else is logged once
and ignored, and a `maxFireAnimationSeconds` timeout exists purely to break out
of scripts that would otherwise loop forever.

That interpretation is why the Borstal shotgun plays its reload on fire: its
`Fire:` label opens with `A_JumpIfInventory("FDHellboundShotgunShots", 0,
"GonnaReloadNothingery")`, which the interpreter always takes.

We want ammo, inventory and explicit reloading as real mechanics, so the
conditional edges in these scripts are load-bearing and cannot be flattened
away.

## Decision

A migration tool compiles each PK3 weapon into a **compiled weapon** — a state
graph over a closed instruction set — and writes it into the external weapons
folder. The engine loads that data and steps it. The engine contains no
DECORATE parser and no ZDoom semantics.

Any DECORATE construct the compiler does not recognise is a **migration-time
error naming the source line**, never a runtime warning.

## Consequences

- Determinism: the same PK3 always produces the same compiled weapon. Output is
  diffable and reviewable in version control.
- The engine's weapon runtime shrinks to a stepper over the instruction set plus
  an inventory map. `maxFireAnimationSeconds` and the unknown-function logging
  both go away.
- Porting a weapon that uses an unsupported action function is a deliberate act:
  extend the instruction set, or hand-edit the compiled output.
- ZDoom semantics have to be reimplemented correctly once, in the compiler,
  rather than approximated in the engine (`A_JumpIfInventory` with amount 0
  means "jump if the item is at its maximum amount").
- ACS calls (`CallACS("FD_recoil")`) are mod configuration reads, not logic;
  they resolve to constants or to our own settings at compile time.
