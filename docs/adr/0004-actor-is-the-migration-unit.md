# 4. The actor is the migration unit, and weapon folders are self-contained

Date: 2026-09-05

## Status

Accepted

Extends [1. Compile ZDoom weapon scripts to a state graph at migration time](0001-compiled-weapon-state-graph.md).

## Context

Weapon scripts spawn things. The Borstal shotgun spawns `FDGunSmokeSpawner` for
muzzle smoke and `FDHellboundShellCasingSpawner` for ejected shells. We want
those drawn the way ZDoom draws them, from the mod's own sprites, rather than
approximated with our particle system.

Those spawned things are DECORATE actors in their own right: a `Spawn` state
machine, sprites, `Speed`, `Scale`, `Alpha`, `RenderStyle` and flags such as
`+NOGRAVITY`. Rendering them faithfully means compiling and running them, not
just naming them.

Weapons also reference actors in other ways: the ammo actor, the counter actors,
and the puff type passed to `A_FireBullets`.

## Decision

The compiler's unit is the **actor**. A weapon is the actor the player holds.
Starting from each weapon, the compiler walks actor references transitively and
compiles everything it reaches.

The walk is bounded by the function policy file rather than by a depth limit: a
reference the policy file does not name is a migration error, so every edge in
the graph is a decision recorded in version control.

Each weapon's folder is self-contained. Actors reachable from two weapons are
compiled into both folders.

At runtime one stepper serves three roles — the weapon layer, the flash overlay
layer, and world actors. Only the inputs differ; world actors take no trigger or
reload input.

## Consequences

- Validating one weapon folder validates one weapon completely. Deleting a
  folder cannot break a sibling, and there is no load order.
- Duplicated actors cost a few kilobytes of atlas per weapon. Deterministic
  compilation means the duplicates cannot diverge.
- The parser must start keeping what it currently discards: flags, `Scale`,
  `Alpha`, `RenderStyle`, `Speed`.
- Spawned actors need world placement, velocity and gravity, which
  `quads/sys_physics.go` already provides, and billboard drawing, which
  `quads/sys_sprites.go` already provides.
- Rejected: a shared `weapons/_common/` folder. It deduplicates actors but
  introduces load order, global validation and refcounting on removal.
