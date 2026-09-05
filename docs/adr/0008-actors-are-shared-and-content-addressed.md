# 8. Actors are shared between weapons, and named by their content

Date: 2026-09-05

## Status

Accepted

Amends [4. The actor is the migration unit, and weapon folders are self-contained](0004-actor-is-the-migration-unit.md).

## Context

ADR 4 gave every weapon a self-contained folder and rejected a shared store,
weighing the duplication at "a few kilobytes of atlas per weapon".

Migrating Final Doomer and Brutal Doom Community Expansion together measured it
properly. 146 weapons, and:

- 867 distinct actors stored 13710 times, because Brutal Doom's weapons all
  reach the same gore, casing and smoke effects.
- 356 megabytes, of which 338 was JSON and only 13 was sprites.
- Indentation accounted for 2.4 times, and the verbatim DECORATE line kept on
  every op for another 1.65 times on top.

The duplication was not a few kilobytes. It was 15.8 times over, and the
premise ADR 4 rested on was simply wrong.

## Decision

Actors move into `weapons/_actors/`, one file each, named by the SHA-256 of
what is in it. A weapon lists the files it needs by hash.

Content addressing is what makes this different from the shared folder ADR 4
turned down:

- There are no names to collide, so nothing has to be namespaced per mod.
- The name is the content, so two weapons pointing at one file cannot disagree
  about what it holds, and a file cannot drift.
- There is no order to load them in, and no refcounting: removing a weapon
  folder leaves its actor files sitting there, harmless, and `genweapons
  -prune` sweeps the ones nothing points at.

Two smaller changes come with it. The JSON is written compact, because it is
generated and what makes a re-migration reviewable is `weapons.lock` saying
which files moved. And the verbatim DECORATE line is kept on a weapon's own ops,
which is what somebody reads against the script, but comes off an actor's,
which are bulk.

Together: 356 megabytes to 43.

## Consequences

- ADR 4's guarantee becomes "validating one folder validates one weapon and the
  actor files it names". Loading reads each actor file once however many
  weapons point at it.
- A weapons folder now has one shared directory in it. Loading skips it when
  looking for weapons.
- Orphaned actor files accumulate as weapons are re-migrated. They cost disk
  and nothing else, and `-prune` removes them.
- Actors are read-only shared state. Nothing writes one after it is made,
  which is what the name being the content already guarantees.
