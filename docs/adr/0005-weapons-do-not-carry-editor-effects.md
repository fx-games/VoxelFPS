# 5. Weapons fire the way ZDoom fires; editor tools leave the weapon system

Date: 2026-09-05

## Status

Accepted

## Context

`WeaponEffect` (`handlers.go`) is an enum assigned to each weapon in `main.go`:
voxel destruction, paint gun, fill gun, plain bullet, killing projectile. It
decides what pulling the trigger does. Three of its five values — voxel
destruction, paint gun, fill gun — are map authoring tools. `setWeaponEffect`
exists so the F1 wizard menu can turn the gun in hand into a paint gun, and
`WeaponEffectHitscanVoxelPaintGunFill` has no weapon of its own at all.

Compiled weapons carry the real firing data instead: `A_FireBullets (4, 0.5,
10, 5, "FDBulletPuff", 0)` is four pellets, a spread, a damage figure and a puff
actor. The enum would throw all of that away.

## Decision

Weapons behave as they do in ZDoom. They are hitscan or they fire projectiles,
and which one, how many, how far spread and how hard is compiled from the
script.

`WeaponEffect` leaves the weapon system entirely. Voxel destruction, painting
and filling become map editor actions, invoked in editor mode, with no route
through the weapon in hand.

A hitscan shot casts one ray per pellet, spawns the compiled puff actor where it
lands, marks a decal, and publishes damage. What a puff does on impact is a
policy-file mapping from the ZDoom puff actor, not a hardcoded enum.

## Consequences

- The shotgun fires four pellets with spread instead of a single ray.
- Firing cadence comes from the script's frame tics.
  `delayBetweenShots`, `maxFireAnimationSeconds`, `FireLabel` guessing and
  `warnedFunctions` all delete.
- Editor tools stop depending on holding a weapon, so they work in editor mode
  regardless of loadout, and the fill gun stops being a mode with no weapon.
- Damage has nothing to apply to yet — there are no enemies and no health in the
  codebase. It is published as an event and ignored, the same treatment
  `make_noise` gets.
