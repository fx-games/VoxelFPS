# Open issues

Consolidated from `ISSUES.md` + `findings.md` (renderer review), September 2026.
Resolved items are cut; only what still needs doing stays. Cross-referenced
against the current source, not the review's own claims.

## Rendering / performance

- **Voxel textures use `texelFetch`**, no mips or filtering (`quads/shader_cubes.go`)
  — distant walls alias and shimmer. Fix is the texture-array plan in
  `optimize.txt`: one `sampler2DArray` layer per tile.
- **`LightsUBO.UpdatePlayerLight`** (`quads/lights_ubo.go:439`) re-uploads slot 0
  every frame even when the camera hasn't moved.
- **Forward shading is a fixed light loop** over every active light per
  fragment. Fine at a few lights, painful at 20+. Fix ladder: per-light range
  culling on the CPU, then clustered forward if it's ever needed — the
  `texelFetch` atlas above should go first, deferred fights it.
- Shadow layers are a separate budget from lights (`MaxLights = 32`,
  `MaxShadowLayers = 16`, `quads/lights_ubo.go`) — a light past the last layer
  lights without casting a shadow. A light that changes layer drops its cached
  shadow map with it.
- Scenery (`scenery.go`, `MeshDrawer`) still draws one `glDrawElements` call
  per placed piece. Since it never moves once placed, it belongs baked into
  the chunk mesh it stands in. Worth doing once a map places enough scenery
  for `MeshDrawer.DrawnLastFrame`'s draw-call count to show it.

## Weapons / migration

- ZScript is not read at all — a mod with no `DECORATE` lump (e.g. one written
  purely in ZScript) fails to migrate rather than half-compiling.
- Not wired into the engine yet, but compiled and carried so wiring it up
  needs no re-migration: `monster_action`, the mod's own ACS scripts
  (`run_script`), screen shake (`quake`), crosshairs, player flags, view zoom
  beyond the player's own; `give_to_target`/`radius_give`/`radius_thrust`
  (nothing has health or mass).
- `A_PlaySound`'s attenuation is compiled but not acted on — a weapon's sounds
  always come from the listener's position.
- The muzzle flash and projectile lights share the single tracked light slot
  (`quads/lights_ubo.go:452`, marked `ponytail:`), so firing while a bolt is
  in the air takes the light off the bolt.
- 96 of 136 weapons fire when tapped like a player taps a trigger via
  `tools/firecheck`; the rest are each their own scripted mechanic (charge
  gauge, heat gauge, remote detonator) worth chasing one at a time with the
  tool.
- 45 sprites and a handful of sounds are named by Final Doomer but shipped by
  neither it nor doom2.wad — those frames show nothing; each weapon lists
  which under `defaulted`.

## Lighting

- Nothing writes a placed light back to the map file — F1 → "Drop Light Here"
  places one in the running engine and toasts its yaw/pitch to copy into the map file
  by hand; it's gone on the next reload.
- Baked voxel light is still flat per face — the resolution greedy meshing
  leaves, and the resolution the emissive flag has always had. A texture with one
  glowing texel seeds the same light as one that glows all over.
- Sprites do not run the shared `emissiveMask`: a BRIGHT frame glows whole. That is
  deliberate (see `lighting.md` §4) but it does mean the F1 threshold sliders move
  walls and meshes and leave sprites alone.

## Dead code kept on purpose

See `AGENTS.md` → "Keep the unused packages" for the rule. Not covered there:

- `dungen/` (~2700 lines) — `loadDungeonMap` (`load_map.go:66`) is never
  called; `main` always loads `assets/raymap.json`. `geometry/jps.go` exists only to
  serve it.
- `quads/glhf/texture_array.go` (~340 lines) — no constructor call outside the
  package. Kept for the planned texture-array voxel rendering above.
- `quads/voxel/map.go` and `geometry/` still carry commented-out alternative
  implementations.
- `dungen/accretion.go:176` — `panic("first room cannot be placed")` instead
  of retrying or reporting a failed generation. Unreachable today since
  nothing calls `dungen`.

## Structure

- `Game` in `main.go` is still a god object outside the voxel-editor state
  already split into `Editor` (`edit_map.go`). Player, weapon, UI and
  render-knob state are still mixed in.
- The nine `quads/shader_*.go` files (1320 lines) repeat near-identical
  uniform plumbing. Only worth merging when two of them need the same change.

## Tests

Covered: `png_chunks`, `quads/voxel` (swept SAT, AABB sweep, DDA raycast), the
collision grid and the standing-on-it correction (`quads/sys_physics_test.go`),
instancing attribute locations, shadow cache. `zdwep/weapon_test.go` was
deleted (called an old signature, read from a gitignored fixture dir) — the
Doom atlas parser is uncovered again.

Untested and worth it: `tpack` atlas pack/decode round-trip, `quads/assets.go`
loaders against a `fstest.MapFS`. `SpriteBatch.grow` needs a GL context so it's
hand-verified only.

## Known limits

- `PathfindingSystem.calculatePath` (`quads/sys_pathfinder.go:90`) hardcodes a
  64x64 path range — wrong on a map bigger than that.
- 36 ignored errors (`x, _ := ...`) outside tests, mostly GL and parsing.

## Gameplay backlog

`optimize.txt` is the running list (jumping/gravity/throwing, decals,
textured particles, pickups, destructible props, hitscan vs projectile
weapons, water, dialog/container UI, enemy movement, SDF fonts, single draw
call for statics, octree culling, instanced rendering, texture arrays).
`design.txt` holds the texture atlas decisions.
