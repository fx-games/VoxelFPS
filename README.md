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
- Spotlights via UBO, authored in the map file (`Light` entries in the JSON map), with
  normal-offset shadow mapping, hardware PCF and per-light reach, cone and intensity
- Fog, and a `-shadowtest` map built to judge shadow bias by
- Render-to-framebuffer + post-processing pass, screenshot capture (F12)

**World & entities** — a plain JSON map format or ASCII string maps, voxel blocks with
DDA raycasting, AABB/OBB/MPR collision (`quads/voxel/`), a simple physics system with
optional gravity, projectiles, particles, animated sprites, paths and pathfinding
(A*, JPS, Dijkstra, BFS, FOV in `geometry/`).

**Doom asset import** — ZDoom-style weapon definitions (`zdwep/`) so Doom sprite weapons
load and animate. 134 of them are in `assets/weapons/`; the scripts are interpreted far enough to
play a firing animation and spend ammo, the effect of a shot is ours (`handlers.go`).
You start with what the mod's player class starts you with - the fists and a pistol - and
pick the rest up: the test map has a range room with every weapon in the folder lying on
the floor, a box of ammo beside each.

**In-game editor** (`edit_map.go`, `edit_tools.go`, `quads/ui_*.go`) — three modes on
one key: voxels, props, actors and the routes they walk. Per-face texture mapping,
atlas pickers, minimap, toasts, lists and buttons drawn with a bitmap font. What it
builds is saved back into the map file it was loaded from. Assets hot-reload via
`fsnotify`.

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
quits. VSync and the debug overlay go off with it. Judge frame time (ms), not FPS —
FPS is a reciprocal and averaging it hides hitches.

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
| `1`..`9` / `,` `.` | pick a weapon you are carrying, by its `keys.txt` binding / walk the rest |
| `R` | reload (while the mouse is captured) |
| `I` | inventory: what you are carrying, and what it is |
| `F` | toggle gravity |
| `T` | toggle flashlight (off by default) |
| `E` | take the outlined item / interact with hovered object |
| `V` | spawn current object |
| `Esc` / `F1` | escape menu / wizard menu |
| `F2` | toggle mouse capture |
| `F3` | spawn mode |
| `F4` | cycle decoration texture |
| `F5` | in and out of the map editor - see below |
| `R` / `F6` / `Tab` | edit menu / cube texture mapper / copy block textures |
| `F7` / `F8` | fill / wireframe |
| `F9` / `F10` | FOV + sensitivity up / down |
| `F11` | atlas picker test |
| `F12` | screenshot |
| `K` `L` `O` `P` | list select prev/next, scroll up/down |

### Map editor

`F5` swaps the whole game between playing and building; the mouse stays captured
either way, because building aims with the crosshair the same as firing does.

The editor is always in one of three modes, `Q` cycles them, and every other key
means whatever that mode says it means. What that is, right now, is printed in the
corner - which is the point: there is one legend and no tool that lives in a menu.

| | `BLOCKS` | `PROPS` | `ACTORS` |
|---|---|---|---|
| `1`-`9` | `1`-`5` shape, `6`-`8` dig/paint/fill | palette slot | which NPC |
| wheel | shape | palette | NPC |
| `Tab` | ADD / DEL | snap to grid / free | deselect |
| `Z` | | quarter turn | |
| left click | build | place | place, select an actor, or add a waypoint to the selected one |
| right click | cancel a drag | delete what the crosshair is on | drop the last waypoint |

The palette is the `assets/meshes` folder plus the torch and the flat sprite
decorations, so a new piece of scenery is a folder and not a line of Go. Placement
is free by default - a prop lands exactly where the ray met the floor - and `Tab`
snaps it to the middle of the tile instead. An actor's route is laid out by clicking
the floor where it should walk; it starts walking the new leg as soon as it finishes
the one it is on, so a route is checked by watching it, and the selected actor's
whole beat is ghosted as unit cubes while it is being drawn.

`Ctrl+S` saves: the voxels to `assets/<map>.bin`, and everything standing in them -
props, torches, actors and their routes - back into the map's own JSON. Both halves
are loaded again at startup. `F` is the fly toggle, and `R` still opens the menu for
what a key cannot carry: a texture out of an atlas of hundreds, load, clear.

`-editor blocks|props|actors` boots straight into it, which is how a screenshot of
the editor is taken (see AGENTS.md - a capture takes no keystrokes).

## Layout

```
main.go handlers.go input.go load_map.go edit_map.go spawn_entities.go doom_weapon.go
scenery.go  pickup.go   the furniture folder and what stands on it
quads/      renderer, systems, UI widgets, shaders
quads/glhf/ OpenGL abstraction layer
quads/voxel/ voxel block/chunk/mesh, raycast, collision shapes
core/       camera, transforms, map types
geometry/   pathfinding, FOV, points/rects
dungen/     dungeon generators
wep/        compiled weapon format, loader and stepper
zdwep/      Doom weapon migration (offline only)
tpack/ eassets/       texture packer, embedded assets
audio/ events/ png_chunks/
```

Everything the game reads at runtime is under `assets/`, resolved relative to the
directory it is started from:

```
assets/
  base/        the map and sprite atlases, by their tile size in the file name
  dialogue/    one conversation per speaker, named after the actor
  meshes/      one folder per model; a scenery.json in it makes it furniture
  sheets/      per-entity sprite sheets
  skybox/      six faces to a set
  textures/    the texture library the editor picks from, dropped/ included
  weapons/     one folder per weapon, plus the shared actor and sprite stores
  raymap.json  the map
```

The UI atlas and the bitmap font are the exception: they are baked into the
binary from `eassets/files/`, so a build with no `assets/` next to it still draws
its own error messages.

~26k lines of Go. Tests cover `quads/voxel` (OBB sweep, DDA raycast), `png_chunks`,
and the weapon pipeline end to end in `zdwep` (compile, write, load, fire).

## Weapons

Weapons are data, and they are picked up rather than handed out. Each carries a
picture of itself lying on the floor, which is also its icon in the inventory (`I`)
and on the HUD. `assets/weapons/` holds one self-contained folder per weapon, plus
`keys.txt` binding the number keys:

```
assets/weapons/
  keys.txt              1: small_gun          (and , and . walk the rest)
  _actors/              every actor any weapon spawns, named by its content
    3f9a....json
  _sprites/             the frames those actors are drawn with
  _sprites.png          them packed, rebuilt when the store changes
  small_gun/
    weapon.json         behaviour: a state graph over a closed instruction set
    sprites.png         its frames, packed, with the table in the atLS chunk
    sounds/             under the names the ops play them by
```

Everything in the folder is checked at startup; a folder that does not load stops
the game with every problem it found, listed.

136 weapons are in there: 125 migrated from Final Doomer and Brutal Doom Community
Expansion, plus ten single-weapon community mods (Axe, Reaper, a Colt45, a
silenced pistol, a hunting rifle with its own scoped alt-fire weapon, a UAC
sniper rifle, dual Berettas, a combat knife, and the Blacktail 51 'Connector')
- some shipped as pk3s and some as the older, pre-pk3 way of shipping a single
weapon: a WAD carrying its own DECORATE, SNDINFO and sprites alongside no map
at all.

Porting out of a ZDoom mod:

```
go run ./tools/genweapons -source mod.pk3 -list          # what is in it
go run ./tools/genweapons -source mod.pk3 -scan          # what it would need
go run ./tools/genweapons -source mod.pk3,doom2.wad -all -prefix bd_
```

Sources are searched in order, so a mod sits on top of the game data it takes for
granted: Final Doomer's weapons spawn stock bullet puffs and never ship one. A
pk3, a directory and an IWAD all work. `-scan` lowers every actor in the mod and
reports the whole vocabulary it needs, so a new mod is one pass rather than one
weapon at a time, and `-prune` sweeps shared actor files nothing points at.

It indexes every actor in the mod, resolves inheritance, ammo caps, sounds and
sprites, and compiles the states down to instructions the engine steps. An action
function with no entry in `zdwep/policy.go` fails the migration naming the actor and
the line, rather than going quiet at runtime. `weapons.lock` records hashes only, so
re-running the migration and diffing it says whether the mod moved, the compiler
moved, or nothing did.

Compiling is not the same as working: a weapon can pass every check here and still
never actually fire, if the two-tier ammo it scripts by hand starts the runtime
thinks is empty, or a jump lands somewhere the trigger never gets read again.
`tools/firecheck` catches that without a window or a GPU - it raises every weapon
in the folder, taps the trigger the way a player does (not an unbroken hold, which
reads as "still charging" to a weapon whose A_Refire means the opposite), and reports
whichever ones never spawn a projectile, take a hitscan shot, or swing:

```
go run ./tools/firecheck -weapons weapons        # just the failures
go run ./tools/firecheck -weapons weapons -v      # every weapon, ok or not
```

Making one by hand from a sprite sheet:

```
go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
    -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 3
```

Looking at one:

```
go run . -weapon fd_drum_shotgun -autofire
```

The decisions behind all this are in [docs/adr/](docs/adr/), the vocabulary in
[CONTEXT.md](CONTEXT.md).

## Scenery

Furniture is data too, the same way weapons are: one folder per piece under
`assets/meshes/`, holding the model and a line or two saying what it is called and what
you bump into.

```
assets/meshes/
  table/
    mesh.glb            the model
    scenery.json        {"name": "Table"}
  stairs/
    mesh.glb
    scenery.json        one collider box per step - see below
  shelves/
    mesh.gltf           the model (plus shelf.bbmodel, the Blockbench source)
    scenery.json        two uprights and three boards
  torch/
    mesh.glb            no scenery.json: a plain model, spawned by name
```

Scenery and the plain models share one folder because they were the same thing
with two layouts. What makes a folder furniture is the `scenery.json` in it.

Only the JSON is read at startup; a model is loaded the first time something
spawns one, so a folder of scenery nobody has placed costs nothing. A map entity
naming a piece in its `MeshName` field gets it, and everything in the folder is in
the editor's placement palette (`F5`, then `Q` to `PROPS`) without a line of Go per
piece.

`colliders` is why the format has anything in it at all. A model is one box to the
drawer and would be one box to physics too, and one box is wrong twice: stairs are
a wall unless every step is its own collider, and nothing on a shelf can be
reached unless its front is open. A piece that names no colliders gets the box
around its model, which is what a table wants.

Stairs are generated, because they are boxes:

```
go run ./tools/genstairs      # four steps, rise under the engine's step height
```

Shelves are modeled by hand in Blockbench instead - see AGENTS.md for how
the model keeps its scale exact.

## Dialogue

An actor with a file named after it in `assets/dialogue/` can be talked to;
every other actor is a monster and stays quiet. The lines are a flat map rather
than a nested tree, so three replies can end on the same line without it being
written out three times, and a conversation may loop back on itself:

```json
{
  "speaker": "Scientist",
  "start": "greeting",
  "lines": {
    "greeting": {
      "text": "You shouldn't be down here.",
      "choices": [
        { "text": "Come with me.", "next": "hired", "do": "follow" },
        { "text": "Just passing through.", "next": "thanks" }
      ]
    },
    "hired":  { "text": "Right behind you." },
    "thanks": { "text": "Just be careful." }
  }
}
```

`do` is what picking a reply does in the world as opposed to what it says, and
it can only name something in `dialogueActions` - today `follow` and `stay`.
That list is the whole reach a dialogue file has into the game, and it is bound
to the actor being spoken to, so a file can never act on anything else. A file
that will not load leaves that actor mute and logs why rather than taking the
map down: this is content edited while the game is running.

## Picking things up

Two modes, switchable in the `F1` menu:

- **Intentional**, the default - aim at it. The selection beam outlines the item
  and names it, and `E` takes it. Nothing is picked up by accident.
- **Collision Based** - Doom's own. Walk within an item's radius and it is yours.

How big an item is, and so what the beam has to be on, is measured off the
picture's own pixels by the migration and kept in `weapon.json` as `icon_box`
and `ammo_icon_box`. A weapons folder migrated before that gets them from:

```bash
go run ./tools/genweapons -measure
```

The weapon range east of the spawn room racks the whole `assets/weapons/` folder on
shelves, a weapon and its own ammo side by side on a board, two boards to a rack
and a rack on every other row so there are aisles to walk down.

## Status & backlog

Last worked on July 2025; state is "renderer and editor are fun, gameplay is stubs".
Known bugs and dead code are tracked in [ISSUES.md](ISSUES.md).
`optimize.txt` holds the running todo list — highlights: jumping/throwing physics,
destructible props, hitscan vs projectile weapons, enemy movement,
dialog/container UI, SDF font rendering, and the rendering optimizations
(single-draw-call statics, octree culling, instancing, better frustum culling).
`design.txt` holds the atlas layout decisions.
