# Open issues

State after the September 2026 cleanup pass (removed `physics/` + Bullet binding, `wad/`,
`geometry/fov.go`, `geometry/camera.go`, `quads/voxel/mpr.go`; merged the disk and embedded
asset loaders into `quads.Assets`; added DDA raycast tests).

Fixed since: `SpriteBatch` now grows its index buffer along with its vertex buffer,
`DDARaycast` takes a `maxDistance` and reports real misses, the collision code sweeps oriented
colliders for real, gravity and jumping work, long moves are split into steps, and a collision
reports one event for the surface that was actually hit (see Physics below).

## Bugs

- ~~**`quads/voxel/map.go:230`** — `chunkIsVisible` returns after a sphere-vs-frustum test~~
  **Stale.** `chunkIsVisible` is a single `IsCubeInViewFrustum(chunk.AsAABB())` call
  (`quads/voxel/map.go:340`). The commented-out alternative below it is the only thing left.
- ~~`zdwep/weapon_test.go` does not compile~~ **Deleted.** It called the old
  `readDoomAtlas` signature and read from `../doom_likes/`, which is gitignored, so it could
  never pass in CI. The Doom atlas parser is uncovered again; a replacement would need a
  small PNG in `zdwep/testdata/`.

## Lighting

- **Lights and shadow layers are separate budgets.** `MaxLights = 32` sizes the UBO and the
  shader loop; `MaxShadowLayers = 16` sizes the atlas. `assignShadowLayers` hands layers to
  casters in list order and writes the layer into `params.x`; a light past the last layer
  lights without shadowing instead of sampling someone else's depth map. A light that
  changes layer drops its cached shadow with it.
- **Slot 0 is the player's light and holds exactly one kind**: off, flashlight (45 degree
  beam, 15 units) or torch (120 degree pool of warm light, 9 units). `T` cycles them, so the
  player can never be lit by two of their own lights at once. Both cast shadows and both
  hold an atlas layer; the torch is a wide spotlight rather than an omni light because a
  shadow map is one frustum, and a sphere would need six.
- **A torch is a baked light.** A `Torch` entity in the map, and the editor's torch drop,
  both go to `Map.AddTorchLight` and seed the voxel flood fill - no slot, no per-frame cost,
  and a level can hold hundreds. A `Light` entity is dynamic unless its `Baked` field is set.
  Baked light has one colour for the whole level (`BAKED_LIGHT_COLOR`), which is the trade.
- A map whose dynamic lights do not fit under `MaxLights` now toasts how many were dropped
  instead of silently losing them.
- Lights are map data (`core.MapLight`, `MapLoadResult.Lights`), not Go literals:
  `raymap.ldtk` holds the two static ones as `Light` entities, and loading a map
  replaces the scene's lights with the map's. Slot 0 stays the flashlight. `AGENTS.md`
  and finding #14 have the detail.
- Nothing writes a light back to the map file. F1 -> "Drop Light Here" places one in
  the running engine and toasts its yaw and pitch to copy into LDtk by hand, and it
  is gone on the next reload.
- Floor shadows work (finding #22): normal-offset bias, a `w <= 0` guard, and a
  spotlight near plane of 0.5 instead of 0.15. Judge them with `-shadowtest`, tune
  them with `-shadow-offset` / `-shadow-bias` or the two F1 sliders.
- The baked voxel flood-fill light is wired up (finding #17). The emissive texture set
  is what seeds it: every face using one of those textures lights the air in front of
  it, `Map.bakeEmissiveTorchlight` floods from there, and the level rides the vertex
  bits into `bakedLight()` in the shader. Both places that change the emissive set
  re-bake, so glow and light cannot drift apart. On `raymap.ldtk` that is 680 emitting
  faces baked in ~8ms at load.
  - The level rides the voxel lookup buffer, in the spare bits of the uint16 the cube
    shader already fetches per fragment for its texture index: emissive in bit 15,
    light in 14-11, texture index in 10-0 (`Map.packFaceLookup`). Nothing per face is
    in the vertices any more, so greedy meshing merges across a light gradient and
    raymap stays at its original 1324 triangles. It went through the vertex bits
    first, which cost 3448.
  - `DrawWithShader` re-floods the light whenever the lookup buffer is dirty, so a
    wall placed next to a panel changes the light it blocks. ~8ms on raymap, in a
    frame that was rebuilding and re-uploading the buffer anyway.
  - The colour and strength are constants in `quads/shader_header.go`
    (`BAKED_LIGHT_COLOR`, `BAKED_LIGHT_STRENGTH`), not a runtime slider.
  - Still flat per face - that is the resolution greedy meshing leaves, and the same
    resolution the emissive flag has always had.

## Weapons

- Weapons are data. `weapons/` holds one folder per weapon plus `_actors/`, the shared
  store, and `keys.txt` binding the number keys; `,` and `.` walk the rest. Everything
  is read and checked at startup and a folder that does not load stops the game with
  every problem it found listed. A weapon's pictures and sounds are read the first time
  it is held, not at startup.
- 146 weapons are migrated from Final Doomer and Brutal Doom Community Expansion. 144
  of the 147 in the folder do something when the trigger is held; the three that do not
  are two abstract base classes and a coat.
- Spawned actors are drawn: the mod's own smoke, casings and projectiles, from their
  own sprites. Their frames live in `weapons/_sprites/` and are packed into one atlas
  when the game starts, kept in `weapons/_sprites.png` until the store changes, so
  every casing and puff on screen is one texture and one batch whoever fired it.
- What the engine does not act on yet, all of it compiled and carried so that wiring it
  up needs no re-migration:
  - Monster behaviour (`monster_action`), the mod's own ACS scripts (`run_script`),
    screen shake (`quake`), crosshairs, player flags, and everything about how a
    player's own view beyond zoom. Velocity, fading, scaling, seeking and warping all
    work.
  - `give_to_target`, `radius_give` and `radius_thrust`: nothing has health or mass.
- A spawned thing carries no physics body. It moves by arithmetic and stops at the
  first solid block: a shell falls and settles on the floor, a missile that reaches a
  wall goes to its Death state. A body would sweep the voxel world every frame for
  every puff of smoke, and worse, the player would bump into ejected shell casings.
- Homing works: A_SeekerMissile turns a missile towards what the player is looking at,
  by no more than its own turn rate. There are no monsters to lock onto, so the
  crosshair is the target. A_Warp puts a thing on whatever spawned it.
- Spawned sprites do not cast shadows, the way Doom sprites never did. A hundred
  drifting puffs that cast would have every light redraw its shadow map every frame.
- No more than 192 spawned things are kept at once, oldest dropped first. That is
  right for smoke and casings and would be wrong for a rocket in flight.
- Damage is published as an event and plays the hit reaction. Nothing has health.
- `A_PlaySound`'s attenuation is compiled but not acted on. The player can pan and
  attenuate, but a weapon's own sounds come from where the listener is standing.
- The muzzle flash rides the single tracked light slot, which projectile lights also
  use, so firing while a bolt is in the air takes the light off the bolt.
- 45 sprites and a handful of sounds are named by Final Doomer and shipped by neither
  it nor doom2.wad. Those frames show nothing, and each weapon lists which under
  `defaulted`.

## Robustness

- 97 `panic(` calls in non-test code, all of them at load or init. The three that were
  reachable after startup are gone: `SetUniformAttr` on a uniform the program does not
  have now logs once and returns false (it has 65 call sites and runs from the draw
  path), `Assets.Image` returns the magenta placeholder like `Assets.Texture` already
  did (the atlas picker decodes whatever the player drops on the window), and
  `calculatePath` returns no path instead of panicking on a destination at another
  height. What is left is structural - a missing shader or font index is not something
  to carry on from.
- 36 ignored errors (`x, _ := ...`) outside tests. The worst were in the asset loaders and are
  fixed (`quads.Assets.open` panics with the file name); the rest are mostly GL and parsing.
- `dungen/accretion.go:176` — `panic("first room cannot be placed")` instead of retrying or
  reporting a failed generation. Unreachable today: nothing calls `dungen`.
- ~~`go vet` warnings~~ **Clear.** The four unreachable `return`s after `panic` are gone and
  the six `voxel.Int3` literals in `edit_map.go` are keyed. `go build`, `go vet` and
  `go test` all run on every push through `.github/workflows/test.yml` - `go.yml` only fires
  on release tags, which is too late to learn the tree is broken.

## Dead or unused code

**These stay. See `AGENTS.md`** - they have no callers today and are kept on purpose,
because procedural generation, sound and the 3D texture path are all expected to be
wired up later. Listing them here is a record of the fact, not a proposal.

- `dungen/` (~2700 lines) — `loadDungeonMap` in `load_map.go` is never called; `main` always
  loads the LDtk map. `geometry/jps.go` exists only to serve it. Wire it up or drop both.
- `audio/` (~350 lines) — imported nowhere, but keeps `gopxl/beep` and `go-meltysynth` in
  `go.mod`.
- `geometry/cc.go` — connected components, no callers.
- `quads/glhf/texture3d.go`, `quads/glhf/texture_array.go` (~340 lines) — no constructor call
  outside the package. Kept for the planned texture-array voxel rendering in `optimize.txt`.
- `quads/voxel/map.go` and `geometry/` still carry commented-out alternative implementations.

## Physics notes

- Objects never tunneled through voxels: the cell walk already covered the swept bounding box.
  What the "CANNOT BE CALLED WITH HIGH-VELOCITY OBJECTS" comment was really about is cost - that
  bounding box grows with the cube of the distance, so a 173 unit diagonal move cost 2.8ms in an
  empty world. `Update` now splits a move into steps of at most one unit, which makes it linear
  (the same move costs 0.5ms, a 1039 unit one 3ms).
- Gravity is off by default, as it was before; `F` toggles it and now also tells the physics
  system. With gravity on, the player keeps its vertical velocity between frames, jumps only
  when `IsGrounded`, and falls no faster than `terminalVelocity`. With gravity off, space and C
  fly straight up and down.
- Velocity into a surface is removed on impact, so an object resting on the floor does not
  build up speed, and sliding along a wall keeps the part of the movement that runs along it.

- Sprites are billboards: their transform turns towards the camera, so physics deliberately
  treats them as axis aligned boxes (`PhysicsComponent.HasOrientedCollider`). Only objects with
  a transform of their own are swept as oriented boxes. `PhysicsComponent.OBB(camera)` stays
  the camera-facing box, and is for picking only.
- `CheckAABBVsOBB` inflates the oriented box by the reach of the moving box along each of its
  axes and ray casts. The exact Minkowski sum of two boxes has bevelled edges, so a glancing
  hit near an edge of a rotated box reports slightly early. Marked with a `ponytail:` comment.
- `CheckOBBVsOBB` was removed: it composed the two transforms in the wrong order, had no
  callers, and its test asserted a time of impact that only holds when the two centers meet.
  Nothing in the engine sweeps a rotated box against another; the moving side is always an AABB.

## Structure

- `Game` in `main.go` is still a god object, but the voxel editor's state is out of it:
  `Editor` (`edit_map.go`) holds the brush, the selection, the pending selection action and
  the spawn ghost, reached as `g.editor.*`. What is left mixed in is player, weapon, UI and
  render-knob state. `lastHitResult` and `entityMenu` stayed on `Game` on purpose - gameplay
  reads both.
- `GlApplication` (`quads/gl_application.go`) — same problem one layer down, minus the light
  list: `LightsUBO` owns the lights, the player's light and the tracked light now, and
  `GlApplication` forwards to it.
- `ldtk/ldtk.go` — 1174 lines of quicktype-generated schema, of which `load_map.go` uses five
  types. Hand-written structs would be ~40 lines.
- The nine `quads/shader_*.go` files (1320 lines) repeat near-identical uniform plumbing.
  Only worth merging when two of them need the same change.

## Tests

Covered: `png_chunks`, `quads/voxel` (OBB sweep, DDA raycast incl. the range/miss cases),
`zdwep` (broken, see above). `SpriteBatch.grow` has no automated test — it needs a GL context;
it was verified by hand by shrinking a batch to one quad and watching it double.
Untested and worth it next: `voxel.SweepAABB` / `CheckAABBVsOBB` (the physics core),
`tpack` atlas pack/decode round-trip, `quads/assets.go` loaders against a `fstest.MapFS`.

## Measuring performance

`-stress` generates a map built to load the renderer, because the shipped test map draws 816
triangles in 2-3 calls and nothing can be measured against it (see `findings.md`,
Measurement). `go run . -stress` gives 128x128 tiles, a pillar every 6, about a thousand
barrel sprites and 8 downward spotlights; `-stress-size`, `-stress-pillars`,
`-stress-sprites` and `-stress-lights` move each knob. It is fully deterministic, so two
builds can be compared on the same frame. The FPS average is in the window title.

The pillars are load bearing: a flat floor with four outer walls collapses into a handful of
quads under greedy meshing and stresses nothing.

Known limit: `PathfindingSystem.calculatePath` hardcodes a 64x64 path range, so pathfinding
is wrong on a stress map bigger than that. Nothing in a stress run calls it.

## Gameplay backlog

`optimize.txt` is the running list (jumping/gravity/throwing, decals, textured particles,
pickups, destructible props, hitscan vs projectile weapons, water, dialog and container UI,
enemy movement, SDF fonts) plus the rendering optimizations (single draw call for statics,
octree culling, instanced rendering, texture arrays instead of per-vertex texture indices).
`design.txt` holds the texture atlas decisions.
