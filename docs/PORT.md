# Port to VoxelLib

Branch `voxellib-port`. Each step ends with `gofmt -l .`, `go vet ./...` and
`go test ./...` clean, then a commit. Engine pieces land in VoxelLib first (with
tests, following its AGENTS.md), then the game switches over.

## Decisions
- Desktop only; the WASM/WebGL build (`build_web.sh`, `*_js.go`) is dropped.
- Rendering is VoxelLib `voxel/render` (wgpu); no GL code survives.
- Never name the reference block game in VoxelFPS or VoxelLib.

## Into VoxelLib
- Ladders/climbing: climbable blocks in `voxel/collide` and the player, ladder links in `voxel/nav`.
- Spotlight shadow maps (light list, atlas layers, spot frustum) in `voxel/render`.
- Billboard sprites (8-direction, animated, shadowed) in `voxel/render`.
- Decals (shared with VoxelTactics) in `voxel/render`.
- OBB in `voxel/collide` (the mesh collider is replaced by `model.RaycastPose`).
- Texture packer (`tpack`, MaxRects) as a GPU-free package.
- 2D grid toolkit (`geometry/` JPS, A*, Dijkstra, BFS, components, ranges).
- Dungeon generators (`dungen/`) as `worldgen/dungeon`.
- Quake-style movement (`q3move`) as an optional controller beside the default player.
- Hitscan helper (ray vs. targets and voxels).

## Stays in the game
Weapons (`wep`, `zdwep`, beams, effects), actors, doors (entity doors, via the nav
`Door` hook), crouch/EVA/carry, inventory glue, sprite kinds, editor (rewritten on
VoxelLib ui/world), asset tools and importers.

## Replaced by VoxelLib
Voxel map/chunks/mesher, DDA raycast, AABB physics, voxel A*, camera core, UI/fonts,
glTF models, particles, audio base, platform/GL.

## Steps
- [x] 1. GPU-free engine pieces into VoxelLib: `grid`, `worldgen/dungeon`, `tpack`,
      `collide.OBB`, `projectile.Hitscan`, `character` (`MoveQuake`, `Climber`), `nav.Graph.Links`.
      The game switches to OBB, q3move, ladders and hitscan in steps 2-3, once its
      world and physics run on VoxelLib (the APIs are float64 and world-based).
- [ ] 2. World: VoxelLib `voxel.World` replaces `quads/voxel`; maps converted.
- [ ] 3. Physics, movement, pathing on `collide`/`nav`.
- [ ] 4. Render features into VoxelLib: decals, sprites, spotlight shadows.
- [ ] 5. Client on `voxel/engine` + `voxel/render`, ui, audio.
- [ ] 6. Remove GL, web build, dead code; screenshot check.
