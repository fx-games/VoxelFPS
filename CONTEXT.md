# Context

Glossary for the go-fps domain. Terms only — no implementation detail.

## Weapons

**Weapon** — what the player holds and fires. Ported from a ZDoom mod, or
authored directly.

**Actor** — a ZDoom entity definition. Weapons, ammo, counters and pickups are
all actors. Actors inherit from a parent actor.

**Counter** — an actor used by a ZDoom weapon purely to hold a number, such as
how many shells are chambered. It has a maximum amount, and the weapon's script
branches on whether it is at that maximum.

**Ammo** — the reserve a weapon draws from when fired. Distinct from a counter:
ammo is shared and picked up, a counter is internal to one weapon.

**Migration** — the offline conversion of a ZDoom mod into weapons this engine
can load. It is a pure function of its source: the same source always produces
the same result. It never runs while the game runs.

**Compiled weapon** — a migrated weapon as it exists on disk: behaviour,
sprites, sounds. The engine loads it; it does not interpret ZDoom.

**Op** — one instruction in a compiled weapon's behaviour, drawn from a closed
set the engine understands. Anything a ZDoom script does that is not an op is a
migration failure, not a runtime surprise.

**State label** — a named entry point into a weapon's behaviour: `Ready`,
`Fire`, `Reload`, `Select`, `Deselect`.

**Weapons folder** — the external data folder the engine reads compiled weapons
from at startup. Contents are validated on load; an invalid weapon is a startup
failure, not a silently broken gun.

**Key map** — the file in the weapons folder binding number keys to weapons.

**Tag** — a weapon's display name, shown to the player. Distinct from its
**id**, which names it in the weapons folder and the key map.

**Policy file** — the checked-in record of what every ZDoom action function
becomes. Every function has an explicit disposition; a function with none is a
migration failure. "Ignored" is therefore always a decision someone wrote down.

**Provenance** — what a compiled weapon records about where it came from: the
source, its revision, the tool version, and the credit line to attribute it by.

**Flash overlay** — the second layer a weapon draws over itself while firing:
muzzle flash sprites and the brief world-brightness bump that goes with them. It
runs concurrently with the weapon's own animation, not in sequence.

**Puff** — what a hitscan shot spawns where it lands. In ZDoom it is an actor;
it decides the impact effect.

**Spawner** — an actor a weapon throws into the world as a side effect of
firing: muzzle smoke, an ejected shell casing. Cosmetic, but drawn from the
mod's own sprites rather than approximated.
