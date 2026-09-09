# AGENTS.md

Notes for agents working in this repo.

## Searching

`git grep -n pattern` searches the tracked tree. It is faster than `grep -r`
and skips `assets/weapons/`'s six thousand generated files, which is where a plain
recursive grep spends all its time.

## Seeing what a change did: screenshots

The game builds a real GL window, so the only honest way to check a rendering
change is to look at a frame of it. `-shot` does that without playing:

```bash
go run . -weapon bd_chaingun -shot /tmp/shot.png -shot-at 120
```

It runs the game normally, saves the finished frame on tick 120 - the whole
frame, GUI included, off the back buffer - and quits. Then read the PNG.

An animation needs more than one frame, so `-shot-count` takes a strip:

```bash
go run . -weapon bd_chaingun -autofire -shot /tmp/fire.png \
    -shot-at 120 -shot-every 3 -shot-count 8
```

That writes `fire-00.png` through `fire-07.png`, three ticks apart, with the
trigger held down the whole time. Stitch them into one picture to look at them
together rather than reading eight images:

```bash
python3 -c "
from PIL import Image; import glob
ims=[Image.open(f).resize((426,320)) for f in sorted(glob.glob('/tmp/fire-*.png'))]
s=Image.new('RGB',(3*426,((len(ims)+2)//3)*320))
for i,im in enumerate(ims): s.paste(im,((i%3)*426,(i//3)*320))
s.save('/tmp/sheet.png')"
```

The flags:

| flag | what it does |
| --- | --- |
| `-shot PATH` | take screenshots and quit; without it nothing is captured |
| `-shot-at N` | first screenshot on tick N (default 90 - the map has to load) |
| `-shot-every N` | ticks between screenshots (default 6) |
| `-shot-count N` | how many to take; above 1 they are numbered `-00`, `-01`, ... |
| `-shot-fps N` | the frame time every frame is told it took (default 60) |
| `-pitch N` | start looking N degrees down, for what a weapon leaves on the floor |
| `-walk` | hold the forward key, for anything that only happens while moving |
| `-weapon ID` | start holding this weapon, by its `assets/weapons/` folder name |
| `-autofire` | hold the trigger down for the whole run |
| `-altfire` | hold the second attack button down: a scope, a bayonet, a second mode |
| `-cycle N` | put the next weapon in hand every N seconds, to walk the whole folder |
| `-tap D,U` | tap the trigger D tics down, U up, for a weapon that acts on a press |
| `-width`/`-height` | window size, for a capture that needs more pixels than 640x480 |
| `-editor MODE` | boot into the map editor in `blocks`, `props` or `actors` - a capture takes no keystrokes, so this is the only way to photograph the editor |

**A capture is deterministic.** `-shot-fps` makes every frame advance the world
by the same amount whatever the machine took to draw it, so tick 120 of one run
holds the same animation frame as tick 120 of the next - what makes a
before-and-after comparison mean something. The debug overlay and the minimap
are switched off for the run so they are not in the picture. On a hidpi
display the PNG is the framebuffer, not the window: a 640x480 window writes a
1280x960 file. F12 still saves `screenshot00.png` while playing.

## Seeing why a weapon stops working: DEBUG_WEAPON

`DEBUG_WEAPON=1` makes the weapon in hand say where its script is, once a tic,
next to everything it is carrying that has a capacity:

```
[weapon] fire[807] MP42B0 1t   DualMP40Ammo=64/64 MP40Ammo=32/32 Mauser9mm=200/200
```

A weapon that has gone invisible or will not fire is a weapon whose script has
walked somewhere unexpected. A counter sitting at zero next to a state that
keeps jumping away is a weapon waiting on something nothing gives it.

## Seeing why an effect looks wrong: DEBUG_ACTORS

What a muzzle smoke or a shell casing looks like is decided by five numbers -
the picture, the actor's scale, its alpha, its render style and the light
where it stands. A screenshot says it looks wrong; this says which of the
five it is.

```bash
DEBUG_ACTORS=1 go run . -weapon bd_chaingun -autofire -shot /tmp/s.png -shot-at 129 2>&1 |
    grep '\[actor\]' | sort -u
```

```
[actor] HitpuffSmoke SMOKA0 at=[4.5 1.4 8] size=1.13x1.15 scale=0.70 alpha=0.40 style="translucent" emissive=0.00 baked=8
```

A negative size means a script wrote a negative scale (ZDoom reads that as a
mirror). `baked=0` on something standing in a lit room means it is lit as if
in the dark. A size in the tens means the pixels-per-unit conversion is wrong
for that sprite. `at=` next to the player's own position says whether a spawn
landed where the call asked or in the camera's lap.

## Where the weapons come from

The `assets/weapons/` folder is generated. `tools/genweapons` reads a mod and writes
it; it never runs while the game does, and `weapons.lock` records what the last
run built from what.

Every source that produced a weapon in the folder, and where it is on this
machine. `doom2.wad` goes last in every `-source` list: the mods take the base
game's sprites and sounds for granted, and a source list is searched in order.

| source | weapons | prefix | where it is |
| --- | --- | --- | --- |
| Brutal Doom Community Expansion | 43 | `bd_` | `$OLD/bdce` — a clone of <https://github.com/BLOODWOLF333/Brutal-Doom-Community-Expansion> at `02623c7`, recorded as revision `master` |
| Final Doomer v3.4 | 80 | `fd_` | `~/Downloads/FinalDoomer_v3.4.pk3` |
| nine single-weapon community mods | 10 | none | `~/Downloads/*.zip`, unpacked under `$OLD/mods/<Name>/`. Ten were downloaded; one does not migrate |
| `assets/smallgun_sheet.png` | 1 | none | in this repo; not a mod at all, see below |
| Doom II | — | — | `~/Downloads/doom2.wad` |

`$OLD` is `/private/tmp/claude-501/-Users-felix-Projects-go-fps/3b5b84ba-fb8f-4f32-b475-f14ef98a2c7d/scratchpad`,
an earlier session's scratchpad. **It is under `/private/tmp` and will not
survive a reboot.** The durable copies are the zips and the pk3 in
`~/Downloads`; the BDCE clone is not, and has to be cloned again when it goes.

The ten downloaded, each a Realm667 Armory-format release carrying its own
`CREDITS` or `readme.txt` naming its author, and the weapons each produced:

| zip | weapon |
| --- | --- |
| `Axe.zip` | `axe` |
| `BTConnector.zip` | `blacktail_51_connector` |
| `Colt45.zip` | `colt45` |
| `CombatKnife.zip` | `combat_knife` |
| `DualBerettas.zip` | `dual_92s` |
| `HuntingRifle.zip` | `huntingrifle`, `huntingriflescope` |
| `Reaper.zip` | `reaper` |
| `SilencedPistol.zip` | `silencedpistol` |
| `UACSniperRifle.zip` | `sniper_rifle` |
| `MagnumRevolver.zip` | **none** — written entirely in ZScript, which is a different and far larger language than DECORATE. It ships no DECORATE lump and this migration cannot read it. |

`small_gun` was never in a mod. `tools/gensheet` builds it out of a plain grid
sprite sheet in this repo, and writes the same `weapon.json` the migration
writes:

```bash
go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
    -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 3
```

The three runs that built the rest:

```bash
go run ./tools/genweapons -source "$OLD/bdce,$HOME/Downloads/doom2.wad" \
    -all -prefix bd_ -revision master
go run ./tools/genweapons -source "$HOME/Downloads/FinalDoomer_v3.4.pk3,$HOME/Downloads/doom2.wad" \
    -all -prefix fd_ -revision v3.4
go run ./tools/genweapons -source "$OLD/mods/DualBerettas/DualBerettas.pk3,$HOME/Downloads/doom2.wad" \
    Dual92
```

`-list` names the weapons in a mod without compiling anything, which is how you
find the actor name the third form wants.

**Re-migrating is reproducible from the folder itself.** Every weapon.json
records the source it came from, its revision and its actor name, so the runs
that built the folder can be reconstructed from `provenance` rather than
remembered. The two big mods were migrated with `-all`, which compiles
leniently; the single-weapon mods were named one at a time. `-prune` afterwards
deletes the shared actor files nothing points at any more, and deleting
`assets/weapons/_sprites.png` makes the next run repack the shared atlas.

**Every bd_ weapon compiles with five animations already gone.** BDCE's shared
`BrutalWeapon` base class gives every one of its weapons a kick, a taunt, a
grenade toss and an execution/curbstomp finisher - real states, real frames -
gated behind inventory flags (`Kicking`, `Taunting`, `TossGrenade`,
`ExecuteDownedEnemy` and its per-victim siblings) that only a key bind this
engine does not have would ever set. `zdwep.Result.PruneDeadStates`
(`zdwep/compile_weapon.go`) walks the compiled state graph from where the
engine's own input can actually send the cursor (`weaponEntryLabels`), and
separately from BDCE's own label names for those five animations
(`deadAnimationLabels`, straight out of `DEFAULTWEAPON.txt`), and deletes only
their intersection - a state that is dead for some other, unrelated reason (no
sprint key, no aim-down-sights hold, no manual unload) is left exactly as it
was, because nothing asked for that one. It runs once, in `genweapons`, right
after `DropSprites`/`DropSounds`; a re-migration reproduces it automatically.
Cut roughly half of every bd_ weapon's own states and 55-90% of its own
`sprites.png` (43 weapons, 296MB folder down to 240MB). `tools/firecheck`
still reports all 43 firing after.

**Some mods hide reload behind alt-fire.** Final Doomer's own pistols (five of
them - `fd_pistol` and friends) never got a real `Reload` label; the script
reloads from `AltFire` instead and disables the button once the magazine is
full. `zdwep.Result.UnifyReloadOnAltFire` detects this by behavior, not name -
`AltFire` reachable, gives back the weapon's own ammo, never fires or zooms
anywhere in its own closure - and aliases `Reload` onto the same steps, moving
`AllowAlt` to `AllowReload` wherever the mod had granted it. A weapon with a
real secondary fire (a scope, a charged shot) never matches and is untouched.
Runs once, in `genweapons`, right after `PruneDeadStates`.

## Adding something to the map for picking

Two different things get added: a **weapon** (already migrated, see above) or
a **scenery piece** (furniture - see the next section for its own folder
format). Getting either one pickable in the world is a third, separate step.

**A weapon needs nothing map-specific.** Every weapon in `assets/weapons/` is
automatically racked by `spawnWeaponRange` the moment the `WeaponRange` map
entity loads (`assets/raymap.json`, the room east of spawn) - a rack of
shelves per tile, a weapon and its own ammo side by side on a board. Add a
weapon to the folder (migrate it, or hand-write a `weapon.json`) and it shows
up there on the next run; nothing else has to change. It only needs pickup art
to look right (`icon`/`ammo_icon`, or `AmmoLine`/`DisplayName` for the text) -
see "A weapon has a picture of itself" further down.

**Anything else that should be pickable - a new kind of pickup, not a weapon -
needs the same four pieces `spawnPickup` (`pickup.go`) wires up for a weapon:**

1. A sprite (`quads.NewSpriteFloor`), sized off the picture's own measured box
   the way `pickupSize` does, not guessed.
2. A `PhysicsComponent` with `PassThrough = true`, added to the physics system.
   `PassThrough` is what makes a thing selectable and walkable-through at once:
   on the grid, so the raycast that drives the selection beam and `Show
   AABBs`/`Show OBBs` finds it, but skipped by the collision solver, so the
   player never bumps into it. Never call `PhysicsComponent.OBB`/the plain
   AABB path for a *stack* of pickups sharing a spot - only one collider comes
   back per ray unless `rayCast`'s per-cell nearest-first logic (`main.go`)
   already covers the case.
3. An entry in `g.pickupByObject[object] = pickup`, which is what
   `hoveredObj`/`hoveredPickup` reads to outline it, name it, and hand `E` to
   `take()`.
4. A `take()` case that does whatever picking it up means - give an item,
   flip a flag, whatever - and clears the map entry and kills the object.

**Furniture is not itself pickable.** `Game.furniture` is every scenery piece
in the world, and `hoveredObj` drops anything in it before the selection beam
or the entity menu ever sees it - a shelf is somewhere to *put* pickups, not
one. If a new piece of scenery should hold things (like the shelves do), give
it colliders that leave an open front (see "Where the furniture comes from")
so the pickup beam can reach in.

**In intentional mode** (the F1-menu default), that is the whole mechanism -
the ordinary world raycast plus `g.pickupByObject`. Nothing about the beam
itself is pickup-specific.

## Where the furniture comes from

`assets/meshes/` is the weapons folder's shape applied to what a room is furnished
with: one folder per piece, holding a mesh (`mesh.gltf` or the older
`mesh.glb` - `gltf.Open` sniffs the actual format from the file's own bytes,
not the extension, so both load the same way) and a `scenery.json` (`name`
and `colliders`). Plain models with nothing to bump into - the torch, the
cube, the door leaf - are folders in there too, without the `scenery.json`;
that file is the only thing that makes a model furniture, and a folder
without one is skipped by `loadScenery` rather than being an error. There is no list of pieces in Go and there must not be one -
a map entity's `MeshName` and the editor's `Place Scenery` menu both go
through `Game.scenery`, whatever `loadScenery` found on disk. Only the JSON is
read at startup; the model loads the first time something spawns one.

**`colliders` replaces the model's box, it does not add to it.** A piece that
names none collides as one box around the whole model, which is what a table
wants. A piece that names its own - stairs, shelves - replaces that:

- **Stairs** need a box per step, or the flight is a solid wall to climb into
  rather than up. `assets/meshes/stairs/scenery.json` holds four boxes;
  `stairs_test.go` walks the real physics system over them.
- **Shelves** need an open front, or nothing standing on a board is reachable
  by the pickup beam or by walking into it. The colliders are the two
  uprights and the three boards, leaving the middle open.

`tools/genstairs` draws the same numbers its JSON collides with - generated
because it is boxes, and because the stair rise has to stay under the physics
system's step height. The shelves are modeled by hand instead, in
`assets/meshes/shelves/shelf.bbmodel` (kept beside the exported `mesh.gltf` for
later edits) - see "Modeling scenery in Blockbench" below for how its scale
and texture stay exact. `TestThingsStandOnTheShelfsOwnBoards` checks the
weapon range stands things at heights the shelf actually has a board at.

## Modeling scenery in Blockbench

`assets/meshes/shelves` was modeled and textured in Blockbench through the
`blockbench-mcp-plugin` MCP server rather than generated, to see what that
workflow costs. What it takes to get a correct, game-ready model out of it:

- **The worst one, because it doesn't fail to export or look wrong in
  Blockbench - it fails only in the running game, invisibly: Blockbench
  normalizes a per-face `uv: [x0,y0,x1,y1]` rect by the *project's* resolution
  setting (`create_project`'s texture size, still 16x16 by default), not by
  the actual texture's own width/height.** Painting a 128x128 texture and
  then giving a face a pixel-space UV rect like `[114,2,118,86]` looks
  completely correct in Blockbench's own viewport and every screenshot this
  session took of the model there - Blockbench displays UV against the
  texture's real size, it just *exports* against the project's declared size.
  The glTF that comes out has TEXCOORD values normalized by 16 (`114/16 =
  7.125`) instead of by 128 (`114/128 = 0.891`), i.e. wildly outside `[0,1]`.
  Sampling that in this engine hits the texture's `CLAMP_TO_BORDER` wrap mode
  (see below), which returns transparent black past the edge, and the mesh
  shader (`quads/shader_mesh.go`) discards any fragment with `alpha == 0` -
  so the whole model renders as nothing. It still passes every other check: it
  loads without error, its AABB and collider are exactly right, it's inside
  the view frustum, `MeshNode.Draw` issues its draw calls - and it still
  casts a correct shadow, because the depth-only shadow shader has no texture
  lookup or alpha discard at all. That combination (present in the collider
  and the shadow, absent in the color pass) is the signature of this bug
  specifically - a real occlusion or culling bug would drop the shadow too.
  **The fix is to set the project's resolution to match the real texture
  before exporting** (or immediately after creating a texture at a
  non-default size) - there is no dedicated MCP tool for it, so do it with
  `risky_eval`: `Project.texture_width = 128; Project.texture_height = 128`
  (and update the `.bbmodel`'s own `"resolution"` and the texture's
  `"uv_width"`/`"uv_height"` fields to match, if hand-patching a file that
  was exported before this was caught - the UV numbers stored in the file are
  still correct pixel coordinates, only the denominator used at export time
  was wrong). Sanity check before trusting any export with custom per-face
  UVs: read the compiled glTF's TEXCOORD accessor `min`/`max` and confirm
  every value is within `[0,1]` - anything outside that range will render as
  invisible-but-shadow-casting in this engine, not as a visibly stretched or
  offset texture the way it would in a renderer with a saner default wrap
  mode.
- **Project format `free` exports 16 Blockbench units to 1 world unit (1
  voxel).** Proven by placing a 16x16x16 cube and reading back the exported
  glTF: its POSITION accessor bounds are `[0,1]`. Nothing in the UI states
  this ratio - it has to be measured once per format. So a part that must be
  exactly one voxel wide is built 16 units wide, not 1.
- **The `gltf` export codec's MCP tool (`export_model`) silently returns an
  empty document (`"{}"`) for this plugin version.** The compiler itself
  works - calling `Codecs.gltf.compile()` directly via `risky_eval` returns
  the real JSON (with embedded base64 buffers and textures, self-contained,
  no external `.bin`/`.png`). Use that path and write the result to a file
  with `Write`/`patch` rather than trusting `export_model`'s `content` field
  for this codec.
- **A cube's per-face auto UV stretches the whole texture over that face**
  (confirmed: a 16x16x16 cube's OBJ export puts `vt 0 0 / 1 0 / 1 1 / 0 1` on
  every face regardless of that face's real proportions). Fine for a cube,
  wrong for a plank - the old generated shelf used this and squashed its
  texture on every non-square face. Fix: give `place_cube`'s `faces` a
  per-face `uv: [x0, y0, x1, y1]` in texture pixels, sized to match that
  face's real-world proportions at one shared pixel-per-unit density, so nothing
  stretches and nothing needs to tile.
- **Tiling a small texture across many faces does not work in this engine.**
  glTF samplers Blockbench exports are hardcoded `CLAMP_TO_EDGE`, and the
  engine's own texture loader (`quads/glhf/texture2d.go`) hardcodes
  `CLAMP_TO_BORDER` regardless of what the sampler says - a UV rect larger
  than the texture just smears the edge pixel instead of repeating. So reuse
  is done by pointing several faces' UV rects at the same region of one
  texture atlas, not by tiling a swatch.
- **`create_texture`'s `width`/`height` are silently ignored when a
  `fill_color` is also given** (tested against this plugin version - the
  texture comes back 16x16 regardless of the requested size). Work around it
  with `risky_eval`: build a blank canvas at the real size with the DOM
  Canvas API, then `new Texture({name, width, height}).fromDataURL(canvas.toDataURL()).add()`.
  Painting tools (`paint_with_brush`, `paint_fill_tool`) work fine once the
  texture is actually the right size - a symptom worth checking for is a
  texture that looks unaffected by painting at high pixel coordinates.
- Do the atlas layout in real units up front (list every distinct face
  rectangle by its world size times the chosen pixel density) - reusing one
  rectangle for several faces (both uprights, all three boards) keeps the
  atlas small and the model visually consistent.
- **The single biggest trap: Blockbench's exporter puts every top-level
  element or group in the scene as a separate *sibling* node.** Five cubes
  placed loose in the outliner export as five sibling `scene.nodes` entries,
  not one node with five children. Group everything under one Blockbench
  group before exporting (`add_group`, then `cube.addTo(group)` via
  `risky_eval` if the cubes already exist) so the export has exactly one root
  node with the cubes as its `children` - check this by reading the compiled
  JSON's `scene.nodes`, which should be `[N]` with a single index. This used
  to be silent and fatal: `quads/model_gltf.go`'s `LoadGLTF` only walked
  `scene.nodes[0]` and never looked at the rest, so an ungrouped export
  quietly rendered as just its first element - and worse, the piece's own
  collider (from `scenery.json`) still spawned and still cast a shadow
  regardless, so it read as "mesh missing" rather than "mesh broken", the more
  misleading failure to debug. `LoadGLTF` now wraps every top-level node under
  a synthetic root (`buildSceneRoot`) and prints a `[loadGLTF] scene has N
  top-level nodes` warning when it has to, so a stray ungrouped export fails
  loudly instead of silently now - but grouping on export is still the right
  habit, both because it's a warning worth not seeing and because a single
  group gives Blockbench's own outliner one thing to select and move.

**A piece of scenery is one thing**, however many boxes it collides as: one
model, one GameObject, one name. `Scenery.colliders` binds its extra boxes to
that one object with `Init` rather than `SetComponent` (a GameObject holds one
component of a type; a shelf is five boxes), so the ray that hits a board
reports the shelf, the outline is the shelf's own box, and killing a shelf
takes its boxes with it.

## Walking over a thing, and asking for it

`pickupModeIndex` is Doom's rule or a deliberate one, and it is in the F1
menu:

- **Collision Based**, what this has always done: a flat distance test in
  `updatePickups`, Doom's own radius, no collider.
- **Intentional**, the default: the same ray `updateGUI` already casts finds
  the nearest pickup it passes through, `DrawGui` outlines and names it, `E`
  takes it.

The world's own hit caps the beam's reach, so nothing behind a wall is
selectable, and a shelf's open front is what lets the ray in as far as a gun
on it. `Pickup.hit` is `voxel.IntersectRayOBB` against the pickup's own
collider under the transform that draws it - the same box `Show AABBs`/`Show
OBBs` and the selection outline use, so what can be picked out is exactly what
is outlined. An axis-aligned box round a billboard - what this did at first -
reads half again as wide as the picture on a diagonal, and on a shelf what
stands in that extra width is the next item along.

A pickup carries its collider but is **never given to the physics system's
solid list** - `PassThrough` keeps it on the grid (hittable) without making it
solid (bumpable). `SpriteDrawer.DrawAABBs` scales a sprite's debug box by the
sprite's own size, since a sprite's box lives in the space of its own quad;
without that a pickup a fifth of a voxel wide showed a box a whole voxel wide.

**A pickup is the size its own picture says**, measured once at migration
time: `wep.MeasurePickups` writes `icon_box`/`ammo_icon_box` into weapon.json
off the opaque pixels of the art, because asking the packed atlas at runtime
means reading a texture back off the GPU. The size is then capped on both
axes - `pickupMaxHeight` a little under the shelf's shortest gap,
`pickupMaxWidth` half the space between its uprights - scaling the whole
picture together, since a gun squeezed on one axis reads as a different gun.

## The two references worth reading before guessing

- **GZDoom's own source** — <https://github.com/zdoom/gzdoom>. What a weapon
  does on screen is decided in `src/rendering/hwrenderer/scene/hw_weapon.cpp`
  (`HUDSprite::GetWeaponRect`) and `src/playsim/p_pspr.cpp` (`DPSprite`). When
  a weapon draws wrong, read those rather than inventing a correction factor.
- **The DECORATE reference** — <https://zdoom.org/wiki/DECORATE>. It is the
  language `zdwep` compiles, and the wiki page for an action function says what
  its arguments mean, which is what `zdwep/policy.go` has to agree with.

## How GZDoom draws a weapon

A weapon is laid out in a fixed 320x200 space and that space is scaled to the
view, so a gun is the same size on every screen. Only the height matters:

    scaleY = viewHeight / 200
    scaleX = scaleY / 1.2          // Doom's pixels are not square

The frame's top left corner then goes at

    x = viewWidth/2  + (psx - 160 - hotspotX) * scaleX
    y = viewHeight/2 + (psy - 100 - hotspotY) * scaleY

`hotspotX/Y` is the picture's own offset: the two numbers in a Doom patch's
header, or in a PNG's `grAb` chunk - **not** a pivot fraction, **not** a delta
against another frame. A vanilla weapon frame's offset is `(width/2 - 160,
height - 168)`, which centers a gun with its bottom edge on the screen's; a
mod's recoil frame sits a few pixels off that, and that difference *is* the
animation. The fallback for a missing (or implausible - further out than the
320x200 screen, as Brutal Doom's one bad `TRACA5` chunk is) hotspot is zero,
not a guessed pivot, for the same reason: guessing `(width/2, height)` throws
a full-canvas frame most of a screen away.

`psx/psy` is the psprite position, persisting frame to frame: starts at `(0,
32)`; a frame's `offset(x, y)` sets it absolutely, only on the axes it writes;
`A_WeaponReady` resets it, ending an unmentioned recoil; `A_Raise`/`A_Lower`
walk it between 32 and 128 at six a tic; a flash overlay's position *adds* to
the weapon's, so a muzzle flash follows a recoil for free.

The weapon actor's `Scale` does **not** apply to the weapon sprite - GZDoom
only scales the pickup on the floor by it. It still applies to spawned world
actors (`Render.Scale`).

## How GZDoom places what a weapon spawns

A spawn is measured from the actor that made it, never from its eyes, and the
two families of spawning function measure differently:

| call | arguments | forward | sideways | up, measured from |
| --- | --- | --- | --- | --- |
| `A_SpawnItem` | `(type, distance, zheight, useammo, transfer)` | `distance` | - | the spawner's feet |
| `A_SpawnItemEx` | `(type, xofs, yofs, zofs, xvel, yvel, zvel, angle, …)` | `xofs` | `yofs` | the spawner's feet |
| `A_CustomMissile` | `(type, spawnheight, spawnofs_xy, angle, …)` | - | `spawnofs_xy` | its middle |
| `A_FireCustomMissile` | `(type, angle, useammo, spawnofs_xy, spawnheight, …)` | - | `spawnofs_xy` | its middle |

Three of those four order their numbers differently, and `A_SpawnItem` has no
angle at all - its third argument is `useammo`, a flag.

Every sideways offset is to the **right**, including `A_SpawnItemEx`'s `yofs`
- GZDoom flips the sign on purpose ("in relative mode negative y values mean
'left'..."). A missile has no forward offset: it leaves from inside the
shooter and gets anywhere under its own Speed. An angle turns the other way -
ZDoom counts anticlockwise, so a positive angle throws left, and it turns
velocity as well as position (an ejected shell casing's own direction).

Offsets here are rotated by the whole aim rather than by yaw alone
(`aimOffset` in `world_actor.go`), a deliberate departure from GZDoom, so
something a weapon throws follows the barrel down when aiming at the floor.

Doom's units convert at `worldUnitsPerDoomUnit` (1/64): a wall panel is 64
units and a voxel here is 1, the player is 56 units and 0.9 voxels tall, a
sprite is one map unit per pixel before its own `Scale`.

A few more facts that decide what a spawned effect looks like:

- **`BRIGHT` is a property of the frame, not the actor.** A BRIGHT frame
  ignores the light where it stands; additive alone does not make anything
  bright.
- **What lands stops.** `+MISSILE` sends a thing to its Death state on
  landing; the flag does not survive DECORATE's `PROJECTILE` keyword through
  migration, so the engine watches for rest explicitly rather than a tumble
  spinning forever with no exit.
- **`A_ChangeVelocity` adds; only `CVF_REPLACE` replaces**, and
  `CVF_RELATIVE` measures along the thing's own heading.
- **A `Stop` destroys the actor outright** (it is DECORATE's `NULL` state) -
  a spawner that already dropped its payload and is only kept "alive to keep
  falling" will instead be sent to Death by the engine's own hit handling and
  drop a second one.
- **A zero-tic chain reads what the previous op just wrote**, not a frame-old
  value - state publishes between ops, not once a frame.
- **`RenderStyle Add` has to actually be additive**, and must not write
  depth, or the nearest translucent sprite hides everything drawn after it.

## Weapon bob

The gun sways while walking, in the same 320x200 space as its position:
`weaponBobAt` in `weapon.go`. Doom's own shape - the sweep across is a full
circle, the dip is that angle folded into half of one, so the gun dips twice
per sweep and never rises above where it is held. Sixty-four tics to the
cycle, sixteen units wide, amplitude the player's speed squared and quartered
and capped. `A_WeaponReady(WRF_NOBOB)` turns it off until the next
`A_WeaponReady` that does not, which is how a reload holds still. It is a
function of speed, so `-walk` is needed to see it in a capture.

## The player-fired muzzle flash is invisible to the player who fired it

Brutal Doom's `PlayerMuzzle1` is a world actor spawned in front of the player
for one tic. **In GZDoom the player who fires never sees it** - a spawn faces
the way its spawner faces, so the shooter looks at its back (rotation 5),
which the mod ships with no opaque pixels at all. The flash is for everyone
*else*; what the shooter sees is the flash drawn into the weapon's own HUD
frames. All eight rotations are migrated (`AssetIndex.SpriteViews`); the
shooter still gets rotation 5, still blank, correctly.

**A world actor keeps all eight views, not just the one a step names.** A mod
ships five lumps for eight rotations (some mirrored pairs); `wep.Actor.Views`
maps the name a step names to the full eight, and each goes into the shared
store under its own name (`MISLE1`..`MISLE8`, no bare `MISLE0`). View
direction 0 is the front, 4 the back, running round the thing's own left
(pinned by `core.TestGetViewDirectionRunsRoundTheThingsOwnLeft`); Doom's
rotation number is that plus one. A mirrored half is mirrored once at
migration time, hotspot included (`width - x`). The view is chosen once a
frame, from the actor's own facing, in `showCurrentFrame` - not per draw call,
so the shadow pass draws the same angle the player sees, which is fine at the
pixel scale these are drawn at.

## The player is Doom's player, and so is the view

Everything a weapon spawns is placed relative to the player, so the player has
to be the one the mods were authored against: 56 units tall, looking out at
41 (`playerHeight`/`playerEyeHeight` in `world_actor.go`). The view is
`DoomVerticalFOV`, 73.74 degrees - Doom's 90 across at 4:3. A weapon sprite
itself is unaffected by field of view either way, since GZDoom lays those out
in the fixed 320x200 space rather than through the projection - which is
exactly why the two numbers only look right together.

## A magazine starts loaded; a counter does not

Both are `: Ammo` with a maximum, and DECORATE says nothing about which is
which - the engine hands every weapon out at once with nothing to pick up, so
a magazine that started empty is a weapon born empty. **A reload fills a
magazine**: `reloadedItems` in `zdwep/compile_weapon.go` walks the states a
Reload reaches and collects what they give, stopping at the weapon's other
business (kick, taunt, sprint) matched anywhere in the name.

## Sprite and actor lookup gotchas

- **A sprite name is only unique inside its own mod.** `assets/weapons/_sprites/` is
  one store shared by every weapon, keyed by content hash
  (`SMOKA0.ac80a16d.png`), not by bare name - two mods' same-named, different
  pictures would otherwise collide. `-prune` clears anything no installed
  weapon names, except the engine's own decal pictures
  (`wep.EngineDecalSprites`), which no weapon.json ever names but which live
  in the same store.
- **The mod's own control script is read, never run.** Brutal Doom's weapons
  branch on inventory items (`FiredPrimary`, `Reloading`, ...) that an ACS
  loop gives and takes every four tics; nothing here runs ACS, so
  `ControlItems` in `zdwep/policy.go` says which item stands for which button
  and the engine keeps them itself. Paired with `+WEAPON.NOAUTOFIRE`, which
  stops the ready frame (not the script) from re-firing on its own.
- **An item a script gives itself, expecting its *own* ACS to clear it, never
  gets cleared here.** Brutal Doom's railgun gives itself `Pumping` on every
  shot and relies on `ACS_NamedExecuteAlways` to take it back; unhandled, the
  gun fires once and locks up forever after. `scriptHeldItems` in
  `wep/runtime.go` gives such items a lifetime instead, sized off the sound
  that plays while the gun is not ready.
- **`A_ReFire` re-enters whichever attack is running** (Hold or AltHold via
  `Weapon.bAltFire`/`Runtime.FiringAlt`), not literally "Fire" - compiling it
  to a fixed target sent an alt-fire's refire into the primary's own state.
- **Alt-fire is a second, independent button** (`AllowFire/AllowAlt` on
  `weapon_ready`, from `WRF_NOPRIMARY`/`WRF_NOSECONDARY`), and
  `FireWeaponAlt` does nothing at all on a weapon with no `AltFire` state -
  it never falls back to the primary.
- **A weapon's tag can lie.** Final Doomer's "Scoped Shotgun" has a scope
  drawn on its sprite and no `A_ZoomFactor` anywhere in the mod. Read the mod,
  not the name, before hunting for a mechanism that was never wired up.
- **Doom's pixel aspect is 1.2, not `320/240`.** `HUDSprite::GetWeaponRect`
  reduces to `scaleX = viewHeight/240`, `scaleY = viewHeight/200`. Getting
  this wrong turns a round scope into an egg; `squarePixelFrames` in
  `weapon.go` is the (short) list of art that really is authored in square
  pixels, kept as named exceptions rather than a general rule.
- **A scope belongs to the weapon, not the camera.** `A_ZoomFactor` writes
  `Weapon.FOVScale`; the view multiplies the player's own `baseFOV` by
  whatever weapon is currently in hand, read fresh every frame - never cached
  off the camera, or an unzoomed weapon picks up the last zoom level.

## Decals, puffs and sound-file gotchas

- **A puff and a decal are separate, unconditional spawns** from the same
  hit - not an either/or. `SpawnShootDecal` reads the shooter's ready
  weapon's own `Decal` property first, then the puff's, inherited.
  `P_SpawnPuff` also picks the puff's *state*: a puff that hit nothing
  starts in `Crash` if the actor has one, `Spawn` otherwise - reversed, a
  hitscan on a wall draws nothing at all.
- **A firing function spends its own ammo** (`A_FireBullets`,
  `A_FireCustomMissile` call `DepleteAmmo` unless told not to) - a weapon
  whose script never takes its own ammo is not a weapon with infinite ammo,
  it is one whose ammo this engine spends for it regardless.
- **`Powerup.Duration` is a clock on an inventory item**, not ACS - it is how
  a spawned charge tells itself (and whoever is watching via
  `A_JumpIfInTargetInventory`) that its window has passed.
- **DECORATE is tokens, not lines.** `Projectile` is shorthand for seven
  flags at once, not a property; several statements can share one line in
  braces; an undefined subclass (`FDTNTMissile : Rocket`) still inherits its
  parent's flags and Speed. `stockProjectiles` in `zdwep/compile.go` is the
  fallback table for classes a mod never redefines.
- **`Loop` returns to the nearest real label, not a synthetic `@frame...`
  one** invented for a numeric jump target - counting a synthetic label as
  the block's start put `Loop` in the wrong place and starved the one frame
  in the block that cost any time.
- **The last lump with a name wins**, GZDoom's own rule
  (`FileSystem::CheckNumForName` searches backward) - and it only applies
  *within* one source layer. A `-source` list stacks several layers and
  later layers must not silently shadow the base game's own lumps just
  because they happen to share a name; `Source.LayerOf` keeps each name's
  match to its own layer.
- **A sound can be shorter than its header claims.** Two Final Doomer WAVs
  declare more data than they contain; naively decoded, the stream never
  reports itself finished and hangs the audio thread forever.
  `endWhenSilent` in `audio/player.go` treats "read, got nothing" as done.
- **ffmpeg on a pipe can't seek back to fix a WAV header it just wrote**,
  leaving both sizes at the streaming placeholder `0xFFFFFFFF` - read as a
  signed 32-bit size, that's -1, and decoding stops before sample zero.
  `repairWAV` in `zdwep/write.go` patches both sizes from the file's actual
  length, for ffmpeg's own output and for a mod's WAV that lies the same way.
- **An empty decoded buffer is one step from silencing the whole game** -
  `beep.Loop` on zero samples spins forever on the audio callback's own
  goroutine. `loadCue` refuses a cue that decodes to nothing.
- **A per-channel mixer must be retired when its origin leaves the world**,
  or dead mixers accumulate and every audio callback pays for stepping
  through all of them - `DropOrigin`, called from `WorldActor.remove`.

## A thing on the ground stops turning to face you

World sprites are XY billboards (follow the camera's pitch too), which keeps
a puff of smoke from going edge-on when you look down at it - but a landed
shell casing lying flat on the floor then visibly spins in place as the
camera turns. `move` drops a resting actor to a plain Y billboard once it
lands (upright, facing you, never pitching), which is GZDoom's own default.

## A velocity can name the thing that threw it

`SXF_ORIGINATOR` hands a spawn a velocity read off its spawner's own published
velocity rather than a fixed number - `Op.VelExprs` keeps the three
expressions rather than folding them to zero at migration time. The two
halves have to travel together: `SXF_ABSOLUTEVELOCITY` means those three
numbers are already world axes, not forward/sideways/up from the spawner's
facing, and reading a spawner's own `velx`/`vely`/`velz` is already in world
axes too.

## A weapon has a picture of itself, in the shared store

`weapon.json`'s `icon` (on the floor) and `ammo_icon` (the box beside it) name
pictures in the **shared** store, not the weapon's own atlas, because a
pickup on the ground and the inventory list draw weapons nobody is holding.
Two places to look, nearest first:

- **The `Spawn` state** - what most mods (and Doom itself) use.
- **`Inventory.Icon`**, the status-bar picture. Final Doomer's weapons have no
  `Spawn` state at all (nothing is ever dropped - a separate `CustomInventory`
  actor is the pickup, driven by its own ACS and tokens), so this is the only
  picture there is to use.

**`Inventory.Icon` can be a `TEXTURES` lump name, not a sprite lump** -
`zdwep/textures.go` reads every lump named `TEXTURES*` and resolves through
it first. **Which of a weapon's two ammo pools you pick up is decided by
which one has pickup art**, not by which slot it is in - `Weapon.AmmoType1`
is the magazine on some weapons and the reserve on others. **Doom's own four
ammo classes** (`Clip`, `Shell`, `RocketAmmo`, `Cell`) are in no mod's own
DECORATE, so `stockAmmoSprites`/`stockAmmoMax` answer for them directly.

## A weapon you always have is not one you pick up

`Player.StartItem` (fists and pistol, in every one of these mods) is handed
out before the first frame, so it is never lying around to find -
`weapon.json`'s `starting` marks it, and `Actor.StartItems` collects the
union across every player class a mod defines (Final Doomer has nine). The
one that ends up in your hands on spawn is the first starting weapon that
costs ammo - Doom's own rule.

## The test range

`assets/raymap.json` has a room east of spawn with a `WeaponRange` entity in
its corner; `spawnWeaponRange` racks every weapon the folder holds from
there - a rack of shelves per tile, a weapon and its own ammo side by side on
a board, two boards to a rack, skipping tiles the room's own pillars stand
on and leaving every other row empty so the aisles stay walkable. All 134
weapons fit today; a folder that outgrows the room simply racks fewer, and
the count is logged.

## The HUD counts the magazine, not the pool

`AmmoLine` (`9/31  +300`) is the clip out of its capacity, then the reserve.
Which pool is the clip (`Ammo.Clip`) is not something DECORATE states
directly - it is found the same way a reload's starting fill is, by walking
which pool the Reload states actually give back into. Only a pool the weapon
itself spends counts; a dual-wielded gun's off-hand magazine is not what the
HUD counts down.

## The inventory and the HUD

`I` opens `quads.GuiInventory`: carried items down the left (a `GuiList`, with
its own scrollbar once there are more than fit), the highlighted one's detail
page down the right (`GuiBorder` + `BitmapFontMesh`). **An icon is not a
weapon sprite and must not be placed like one** - `GetSourceAndDestRects`
places a frame by its own canvas and hotspot, correct for GZDoom's 320x200
weapon space and wrong in a panel; `DrawIconInBox` centers and scales the
picture to fit instead.

## Keep the unused packages

`dungen/`, `audio/`, `geometry/` (including `jps.go` and `cc.go`) and
`quads/glhf/texture3d.go` currently have no callers in the main loop. **Do not
delete them.** They are kept deliberately - procedural generation, sound and
the 3D texture path are all expected to be wired up later. An audit that
flags them as dead code is correct about the fact and wrong about the
conclusion.

## Run gofmt

The tree is gofmt'd and CI fails if it is not (`.github/workflows/test.yml`).
`gofmt -w .` before committing. `.git-blame-ignore-revs` holds the reformat
commit that made every future diff readable again.
