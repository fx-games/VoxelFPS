# Open issues

State after the September 2026 cleanup pass (removed `physics/` + Bullet binding, `wad/`,
`geometry/fov.go`, `geometry/camera.go`, `quads/voxel/mpr.go`; merged the disk and embedded
asset loaders into `quads.Assets`; added DDA raycast tests).

Fixed since: `SpriteBatch` now grows its index buffer along with its vertex buffer,
`DDARaycast` takes a `maxDistance` and reports real misses, and the collision code sweeps
oriented colliders for real (see Physics below).

## Bugs

- **`quads/voxel/map.go:230`** — `chunkIsVisible` returns after a sphere-vs-frustum test; the
  AABB path below is unreachable and marked `// TODO: THIS DOES NOT WORK`. Culling is therefore
  sphere-only and over-inclusive. This is the "do better frustum culling" item in `optimize.txt`.
- **`zdwep/weapon_test.go` does not compile** (pre-existing): calls
  `readDoomAtlas(pngFile)` but the function is now `readDoomAtlas(filename string, atlasFile
  io.ReadSeeker) (*glhf.TextureAtlasPacked, string, error)`. It also reads from
  `../doom_likes/`, which is gitignored, so it can never pass in CI. `go vet ./...` fails
  because of it.
- **Tunneling** — `CheckForCollisions` gathers colliders from the grid cells the move touches
  and is documented as "CANNOT BE CALLED WITH HIGH-VELOCITY OBJECTS". A fast projectile still
  passes through thin geometry. Needs the swept bounds to drive the cell walk, or a raycast
  pass for projectiles.
- **Collision events fire per pair, per pass** — `CheckForCollisions` publishes
  `EventPhysicsCollision` for every collider with `TimeOfImpact < 1`, not only the one that was
  actually hit, and again for each slide pass. Consumers see duplicates.
- **Gravity is never applied** — `PhysicsComponent.ApplyGravity` and
  `PhysicsSystem.accelerationFromGravity` exist, but the code that would use them in `Update`
  is commented out. `F` toggles a flag in `Game` that does nothing to the simulation. This is
  the "Physics: Jumping, Gravity, Throwing" item in `optimize.txt`.

## Robustness

- 115 `panic(` calls in non-test code. Asset and shader load failures kill the process; a
  missing texture should be a visible magenta placeholder, not a crash.
- 36 ignored errors (`x, _ := ...`) outside tests. The worst were in the asset loaders and are
  fixed (`quads.Assets.open` panics with the file name); the rest are mostly GL and parsing.
- `dungen/accretion.go:176` — `panic("first room cannot be placed")` instead of retrying or
  reporting a failed generation.
- `go vet` warnings still open: unreachable code in `png_chunks/chunk_text.go:174`,
  `quads/glhf/shader.go:265`, `quads/gl_application.go:855`, `quads/sys_pathfinder.go:95`,
  plus the two listed under Bugs; six unkeyed `voxel.Int3` literals in `edit_map.go`
  (silently wrong if the field order ever changes). Worth adding `go vet ./...` to
  `.github/workflows/go.yml`.

## Dead or unused code

- `dungen/` (~2700 lines) — `loadDungeonMap` in `load_map.go` is never called; `main` always
  loads the LDtk map. `geometry/jps.go` exists only to serve it. Wire it up or drop both.
- `audio/` (~350 lines) — imported nowhere, but keeps `gopxl/beep` and `go-meltysynth` in
  `go.mod`.
- `geometry/cc.go` — connected components, no callers.
- `quads/glhf/texture3d.go`, `quads/glhf/texture_array.go` (~340 lines) — no constructor call
  outside the package. Kept for the planned texture-array voxel rendering in `optimize.txt`.
- `quads/voxel/map.go` and `geometry/` still carry commented-out alternative implementations.

## Physics notes

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

- `Game` in `main.go` is a god object: 30+ fields, 69 methods across `main.go`, `input.go`,
  `handlers.go`, `edit_map.go`, `spawn_entities.go`, `doom_weapon.go`. Player state, editor
  state, weapon state and UI state are mixed. Splitting the editor half into its own struct is
  the biggest single readability win.
- `GlApplication` (`quads/gl_application.go`) — 89-field struct, 1088 lines, same problem one
  layer down.
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

## Gameplay backlog

`optimize.txt` is the running list (jumping/gravity/throwing, decals, textured particles,
pickups, destructible props, hitscan vs projectile weapons, water, dialog and container UI,
enemy movement, SDF fonts) plus the rendering optimizations (single draw call for statics,
octree culling, instanced rendering, texture arrays instead of per-vertex texture indices).
`design.txt` holds the texture atlas decisions.
