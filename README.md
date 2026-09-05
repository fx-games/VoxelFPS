# go-fps

A from-scratch first-person engine in Go: OpenGL 3.3 core renderer, voxel/quad world,
in-game level editor. Not a game — a sandbox that boots into a small test map you
can walk around, shoot in, and edit while it runs.

![Go](https://img.shields.io/badge/go-1.23-blue)

## What works today

**Rendering** (`quads/`, `quads/glhf/`) — a thin OpenGL wrapper (`glhf`) with its own
shader, buffer, UBO, texture-array/atlas and framebuffer types, plus dedicated shaders for
voxel cubes, meshes, sprites, lines, skybox, GUI and post-processing.

- Greedy-meshed voxel chunks, texture indices kept out of the vertex buffer
- Sprite batches with fixed and packed texture atlases
- glTF/GLB static mesh loading (`qmuntal/gltf`)
- Spotlights via UBO, authored in the map file (`Light` entities in LDtk), with
  normal-offset shadow mapping, hardware PCF and per-light reach, cone and intensity
- Fog, and a `-shadowtest` map built to judge shadow bias by
- Render-to-framebuffer + post-processing pass, screenshot capture (F12)

**World & entities** — LDtk maps (`ldtk/`) or a plain ASCII string map, voxel blocks with
DDA raycasting, AABB/OBB/MPR collision (`quads/voxel/`), a simple physics system with
optional gravity, projectiles, particles, animated sprites, paths and pathfinding
(A*, JPS, Dijkstra, BFS, FOV in `geometry/`).

**Doom asset import** — ZDoom-style weapon definitions (`zdwep/`) so Doom sprite weapons
load and animate (`doom_weapon.go`). Three of them are in the player's hands, on `1`, `2` and
`3`; the scripts are interpreted far enough to play a firing animation, the effect of a shot
is ours (`handlers.go`). `4` is a plain sprite sheet weapon (`weapon.go`), no script involved.

**In-game editor** (`edit_map.go`, `quads/ui_*.go`) — voxel painting and selection,
per-face texture mapping, entity spawning, atlas pickers, minimap, toasts, lists and
buttons drawn with a bitmap font. Assets hot-reload via `fsnotify`.

**Audio** (`audio/`) — beep-based playback and a MIDI/SoundFont synth (`go-meltysynth`).
Present, not wired into the game loop.

**Procedural generation** (`dungen/`) — accretion, rogue-style, vault and megadungeon
generators. Present and standalone; not wired into the main loop yet.

## Build & run

Needs cgo, a C toolchain and GLFW system libs (`brew install glfw` on macOS).

```bash
go run .
```

Two generated maps stand in for the shipped one:

```bash
go run . -shadowtest                # one wall, one light, one walking caster
go run . -stress -stress-lights 8   # a hall of pillars and barrels, for frame times
```

Frame times, for before/after on the render path:

```bash
go run . -stress -stress-size 96 -stress-sprites 2 -bench 10
```

`-bench <seconds>` discards a warm up, records every frame, prints percentiles and
quits. VSync and the debug overlay go off with it. The current baseline is in the
Measurement section of `findings.md`.

`-shadowtest` is a rig, not a level: it exists so shadow bias can be judged. The two
knobs it is judged with are `-shadow-offset` (in shadow texels) and `-shadow-bias`,
also live on sliders in the F1 menu.

Windows cross-compile: `./build_win.sh` (mingw-w64). Tagged pushes (`v*.*.*`) build
macOS/Linux/Windows binaries via `.github/workflows/go.yml`.

## Controls

| Key | |
|---|---|
| `WASD` / `Space` / `C` | move / jump or up / down |
| `Left mouse` | fire |
| `1` `2` `3` `4` | Ol' Faithful (projectiles) / Borstal Shotgun (digs voxels) / Charge Pistol (paints voxels) / Small Gun (digs voxels) |
| `F` | toggle gravity |
| `T` | toggle flashlight (off by default) |
| `E` | interact with hovered object |
| `V` | spawn current object |
| `Esc` / `F1` | escape menu / wizard menu |
| `F2` | toggle mouse capture |
| `F3` | spawn mode |
| `F4` | cycle decoration texture |
| `F5` / `R` / `F6` / `Tab` | voxel edit / edit menu / cube texture mapper / copy block textures |
| `F7` / `F8` | fill / wireframe |
| `F9` / `F10` | FOV + sensitivity up / down |
| `F11` | atlas picker test |
| `F12` | screenshot |
| `K` `L` `O` `P` | list select prev/next, scroll up/down |

## Layout

```
main.go handlers.go input.go load_map.go edit_map.go spawn_entities.go doom_weapon.go
quads/      renderer, systems, UI widgets, shaders
quads/glhf/ OpenGL abstraction layer
quads/voxel/ voxel block/chunk/mesh, raycast, collision shapes
core/       camera, transforms, map types
geometry/   pathfinding, FOV, points/rects
dungen/     dungeon generators
wep/        compiled weapon format, loader and stepper
zdwep/      Doom weapon migration (offline only)
ldtk/ tpack/ eassets/  level format, texture packer, embedded assets
audio/ events/ png_chunks/
```

~26k lines of Go. Tests cover `quads/voxel` (OBB sweep, DDA raycast), `png_chunks`,
and the weapon pipeline end to end in `zdwep` (compile, write, load, fire).

## Weapons

Weapons are data. `weapons/` holds one self-contained folder per weapon, plus
`keys.txt` binding the number keys:

```
weapons/
  keys.txt              1: small_gun
  small_gun/
    weapon.json         behaviour: a state graph over a closed instruction set
    actors.json         anything the weapon puts into the world
    sprites.png         its frames, packed, with the table in the atLS chunk
    sounds/             under the names the ops play them by
```

Everything in the folder is checked at startup; a folder that does not load stops
the game with every problem it found, listed.

Porting a weapon out of a ZDoom mod:

```
go run ./tools/genweapons -source ../Brutal-Doom-Community-Expansion -list
go run ./tools/genweapons -source ../Brutal-Doom-Community-Expansion FDHellboundShotgun
```

It indexes every actor in the mod, resolves inheritance, ammo caps, sounds and
sprites, and compiles the states down to instructions the engine steps. An action
function with no entry in `zdwep/policy.go` fails the migration naming the actor and
the line, rather than going quiet at runtime. `weapons.lock` records hashes only, so
re-running the migration and diffing it says whether the mod moved, the compiler
moved, or nothing did.

Making one by hand from a sprite sheet:

```
go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
    -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 2
```

The decisions behind all this are in [docs/adr/](docs/adr/), the vocabulary in
[CONTEXT.md](CONTEXT.md).

## Status & backlog

Last worked on July 2025; state is "renderer and editor are fun, gameplay is stubs".
Known bugs and dead code are tracked in [ISSUES.md](ISSUES.md).
`optimize.txt` holds the running todo list — highlights: jumping/throwing physics,
decals, pickups, destructible props, hitscan vs projectile weapons, enemy movement,
dialog/container UI, SDF font rendering, and the rendering optimizations
(single-draw-call statics, octree culling, instancing, better frustum culling).
`design.txt` holds the atlas layout decisions.
