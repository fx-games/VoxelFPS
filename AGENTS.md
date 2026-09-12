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
| `-roll N` | start with the horizon rolled N degrees, for what the suit sees |
| `-walk` | hold the forward key, for anything that only happens while moving |
| `-weapon ID` | start holding this weapon, by its `assets/weapons/` folder name |
| `-autofire` | hold the trigger down for the whole run |
| `-altfire` | hold the second attack button down: a scope, a bayonet, a second mode |
| `-cycle N` | put the next weapon in hand every N seconds, to walk the whole folder |
| `-tap D,U` | tap the trigger D tics down, U up, for a weapon that acts on a press |
| `-width`/`-height` | window size, for a capture that needs more pixels than 640x480 |
| `-editor MODE` | boot into the map editor in `blocks`, `props3d`, `props2d`, `pickups`, `weapons` or `actors` - a capture takes no keystrokes, so this is the only way to photograph the editor |
| `-pick DIR` | open that mode's own asset browser on that folder (`.` for the top), for the same reason - needs `-editor` |
| `-map NAME` | which level to load, by its folder under `assets/maps` (default `raymap`) |

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

## Making and changing a level, in text

A level on disk is two files nothing can hand-edit - `map.bin` is gzipped
chunks, `map.json` is generated JSON - so until now the only way to build one
was the in-game editor, with a mouse in it. `tools/mapgen` is the other door: a
level can also be a `build.map` beside them, one command a line.

```bash
go run ./tools/mapgen -map station            # build.map -> map.bin + map.json
go run ./tools/mapgen -map station -show 12   # what is solid at height 12
go run ./tools/mapgen -map ark -side 56       # what is solid in the x-y plane at z=56
go run ./tools/mapgen -map ark -leaks         # build, then: is the start sealed from space?
go run ./tools/mapgen -help                   # every command and its arguments
```

`-show` is the half that matters most when nobody is looking at the screen: it
prints one character a voxel at one height, with a ruler, so a coordinate read
off it is a coordinate to write back into the script. It reads `map.bin`, so it
works on a level that was never scripted - `-map doors -show 1` prints the
doors map's floor plan. `-side` is the same standing up, for a level that is
tall rather than wide: it is the one view that shows every floor at once.

The commands are in the package comment at the top of `tools/mapgen/main.go`,
which is the only list of them. The shapes are `fill`, `clear`, `cyl`,
`tube`, `cone`, `ball` and `beam` - any of them carves when its block is `air` -
and `blast` is battle damage: a ragged, seeded hole with a scorched rim; the
rest of a level - where the player starts, what stands in it, its
lights, fog and music - is the same file. Positional arguments come first and
`key=value` options after, and a bare word among the options is a flag
(`baked`). `fill`, `clear` and `light` take `step=DX,DY,DZ times=N` to lay the
same thing down in a row - a corridor of cabins is one line per row.

`-leaks` is the check a station level wants and nothing in the game makes: it
floods the air from the spawn, with doors and panes counted shut, and names the
first voxel of vacuum it reaches and the one it came through. The pressure hull
is a shape (`core.Station.Pressurised`), not a flood, so a gap in a wall is only
a gap to the eye - this is the eye. The ark's six lower decks pass it; that is
the promise that the start is not open to space.

**A level is authored in the text or in the editor, not both.** Saving in the
editor writes `map.bin` back, and the next `mapgen` run overwrites it. A level
with a `build.map` is the script's; one without is the editor's.

Two things a script cannot say, because they are facts about the atlas and the
door rather than about the level:

- **Nine atlas slots glow** (19, 26, 27, 29, 30, 45, 46, 47, 49 - the atlas's
  own `emIS` chunk, see `AtlasIndex.EmissiveChunkKey`). Building a room out of
  one of those is building a room out of light, which reads as a blown-out
  white space rather than a lit one. Read the chunk before picking textures -
  or name the level's textures yourself with `tex`, which writes the level its
  own atlas and says which ones glow (`assets/maps/ark` does).
- **A slide door needs two voxels of passage and the second one filled back
  in.** The leaf is one voxel and opens by rising one (`doorSlideRise`), so a
  one-voxel passage slides it into the wall, and a passage left two voxels tall
  at the door itself leaves a slot of sky over a shut door. Bore the passage
  two tall for headroom and then `fill` the voxel directly above each door: the
  shut door is a shut doorway, and the open one has slid up inside the lintel.

A **floor hatch** is that same door lying down: `facing=0,-1,0` turns the leaf
out of the floor plane, and because a slide door travels along its own up
rather than the world's, a hatch slides sideways under the deck beside it
instead of rising. It wants a one-voxel hole in an otherwise solid plate - the
voxel it slides into has to be there to hide in - and it holds the player's
weight shut, because the solver collides it as an oriented box. The ark's two
spine bulkheads are built that way; `facing` takes `DX,DZ` or `DX,DY,DZ`.

## A ladder, and why it is one voxel of ladder

A **ladder** is the piece that makes a level's vertical circulation work: a
climb volume the player goes up and down inside of instead of falling (see
`Game.climbVelocity`), with the model to look at bolted to it. Walking is
gravity's, and a deck without a ladder is a deck you can only fall out of.

The piece is **one voxel tall and stacks**. A shaft's worth of it is the same
module laid one a voxel - `entity Ladder mesh=ladder at=X,Y,Z step=0,1,0
times=N` - which is why `tools/genladder`'s rung spacing has to divide its
height exactly: the first rung sits half a spacing up, so an exact division
puts the next module's first rung a full spacing above the last one and the
seam does not show. The rails run the full height and meet flush. It was four
voxels tall once, which is a piece that fits no deck in particular: the ark's
decks are two, three and four voxels apart.

The climb volume is the model's height plus a little (`climbable` in
`assets/meshes/ladder/scenery.json`), and that little is what lets go of the
top rung onto the deck above rather than leaving the player stuck under it.

## A locked door, and the note that tells you the number

A door knows nothing about codes. A **keypad** stands beside it with four digits
on it and holds shut whichever door is nearest to it - within `keypadReach`,
which is about a doorway's width - and that is the whole of the wiring. Nothing
in a level says which keypad belongs to which door; where they stand says it.
`Game.bindKeypads` works it out once the level is standing, and again on every
editor placement or deletion, because "the nearest door" is a fact a single
click can change.

A **note** is a sheet of paper lying in the level with a line of writing on it.
Reading one takes it - it leaves the floor and turns up in the inventory - and
learns every four-digit number in that writing. A keypad prints its own number
on its panel once that number has been read somewhere, which is Prey's rule and
the reason notes exist at all. A note carries writing, not a code field: the
number in the writing *is* the number, so a level cannot say one thing on the
paper and another in its data.

Three ways to put them in a level, all of them the same two `core.MapEntity`
fields (`Code` and `Text`):

- **In the editor**, PROPS mode. `keypad`, `note`, `door_slide` and
  `door_swing_leaf` are folders under `assets/meshes/` with a `scenery.json`, so
  they are palette slots like a table. Place one, then press **F on it** - the
  key that uses a thing in the game gives it what it says while building one.
  The editor's own line in the corner prints a keypad's code and whether it has
  a door in reach. F on a door still opens it.
- **In `build.map`**: `entity Keypad mesh=keypad at=X,Y,Z code=7412` and
  `note X Y Z the writing`. `note` is its own command rather than a `text=`
  option because a `key=value` may not hold a space.
- **`assets/maps/keypads`** is the level that exercises all of it: a hall with a
  slide door into a vault and a swing door into an office, a keypad on each, and
  three notes that chain one code to the next.

The models are generated - `go run ./tools/genpanels` - because each is one box
with a picture on one face. The keypad's picture faces **-Z**, which is what "as
modelled" means for a prop nobody has turned, so a keypad on a wall whose open
side faces +Z wants `facing=0,1`. Mount it *proud of* the wall: a panel spawned
on the wall's own voxel is a panel inside the wall, and invisible.

## A sign, and why each one is its own model

A **sign** is a wall display with a few words on it (`sign.go`): a deck's
directory, a department's name over its door, a red warning at a sealed hatch.
What it says is its entity's `Text` - the field a note keeps its writing in -
painted onto the display by `quads.SignPicture` with the editor's bitmap font,
at the biggest whole-pixel size the words fit. Lines split at `|`, and a line
starting with `!` is written red.

Every sign gets **a copy of the model of its own** (`MeshDrawer.NewOwnMesh`).
A loaded model is shared by every placement of it, and so is its texture - that
is how a computer's screen animates on every computer at once - so a sign
sharing the model would be every sign saying the last words painted. A copy is
a load and a draw call; the editor's ghost, which has no collider, shares the
blank one.

- **In `build.map`**: `sign X Y Z FACING the words`. X Y Z is the floor under
  it, like the keypad - the model is hung at head height - and FACING is the way
  its face looks, `0,1` for a sign on a wall whose open side is +z. Put X Y Z a
  few hundredths proud of the wall, or the sign is inside it.
- **In the editor**, PROPS mode, the `sign` palette slot: F on it types the
  words, Enter starts a new line, and it repaints as you type.

The model is generated with the other panels, `go run ./tools/genpanels`: a
0.9 by 0.3 box, 3:1 like the 384x128 display painted on it.

## An audio log, and the key that plays it without stopping you

An **audio log** is a note that talks: a data cartridge lying in the level with
a recording on it. It is deliberately the note's twin - the same E key takes it,
it leaves the floor the same way, it turns up in the inventory the same way -
and it differs in the one way that matters. A note stops you to read it. A
recording comes with you.

That difference is the whole of what the code adds (`audiolog.go`):

- **Picking one up plays nothing.** It says what it is and which key plays it -
  `Audio log: Maintenance log, day 41   [L] to play` - and the key it names is
  whatever `QuickplayLog` is bound to today, through `Game.keyLabel`.
- **Quickplay (`L`) starts the log found most recently** and leaves the player
  walking. It is under `g.isMouseCursorCaptured`, the same condition Reload is
  under, because `L` is also the entity menu's cursor and that menu only exists
  while the cursor is free.
- **A log plays on a sound origin of its own**, taken from `lastActorOrigin`
  like everything else in the world that makes a noise. It must not be the
  player's: `Weapon.Unequip` stops every channel of the player's origin, and a
  log silenced by a weapon switch is not playing in the background. All logs
  share one channel, so a second one stops the first rather than talking over
  it.
- **The recordings load the first time one is played**, not at startup, because
  `mapLoad` unloads every cue there is - so a level with no logs pays nothing
  and a level with them reloads them when it is loaded again.

The recordings are `assets/audio/logs/<name>.ogg` (or `.wav`), one file per log,
and nothing in Go lists them: a log names its own file, the same way a level
names its own music track. `assets/audio/logs/vance-41.ogg` is the sample one,
and it was made on this machine out of macOS's own speech synthesiser rather
than recorded:

```bash
say -v Daniel -r 168 -o /tmp/log.wav --data-format=LEI16@44100 -f assets/audio/logs/vance-41.txt
ffmpeg -i /tmp/log.wav \
    -af "highpass=f=280,lowpass=f=3600,acompressor=threshold=-18dB:ratio=4,volume=3dB" \
    -ac 2 -ar 44100 -c:a vorbis -strict -2 -q:a 2 assets/audio/logs/vance-41.ogg
```

The words are in `vance-41.txt` beside it, which is what makes the recording
reproducible - the cue loader reads only `.ogg` and `.wav`, so a script sitting
in the same folder is not a cue.

The filter chain is what makes it sound like a recording rather than like a Mac:
a voice band and a compressor. **Stereo and 44100 are not optional** - ffmpeg's
own Vorbis encoder refuses mono, and a file at any other rate plays at the wrong
speed, since nothing here resamples a cue to the speaker's rate.

Two ways to put one in a level:

- **In `build.map`**: `log X Y Z CUE THE LABEL` - the cue is one word, naming
  the file without its extension, and the rest of the line is the label on the
  lid. Same shape as `note`, and for the same reason. `assets/maps/keypads` has
  one lying a couple of steps from where the player starts, saying out loud what
  its notes say in writing.
- **In the editor**, PROPS mode, as the `audiolog` palette slot. It has no `F`
  authoring page the way a keypad and a note do, so one placed there is a
  cartridge with no recording and no label until `build.map` gives it one. It
  says so when played rather than playing silence.

**The inventory has tabs now** (`quads.GuiInventory`), because a recording is
not a thing you read down a list with the guns. Each `InventoryItem` names its
own `Tab` and the tabs are whatever the items name, in that order - so a new
kind of carried thing brings its own page without a list of pages anywhere. Two
consequences worth knowing:

- **`Tab` closes the inventory no longer** - it is the tab key now, along with
  left/right and A/D. `Escape` and `I` close it.
- **A row's index stopped being a name for it**, since a tab shows only some of
  what is carried. `InventoryItem.Use` is a closure on the item instead, which
  is what `F` on a row runs: hold that weapon, play that recording, nothing at
  all for a note.

## Glass, and why it is not a block

A window is a hole in the wall with a `glass` ring hanging in it. The glass is
**quads, not blocks**, and that is not a shortcut - the chunk pass has no
blending, and the greedy mesher culls the faces of whatever a block stands
against, so a see-through block would show holes through the sill and the
lintel it touches. A quad has none of those problems, and the renderer already
had a translucent, world-oriented, one-draw-call pass for decals to live in.

So a pane (`core.Pane`, `SpriteDrawer.AddPane`) is a decal that belongs to the
level rather than to something that happened to it. Three differences from a
bullet hole, all of them in `quads/decals.go`:

- it is placed when the map loads and never overwritten, because a ring of
  windows is more quads than `MaxDecals` holds;
- it is drawn **after** the skybox, because a pane writes no depth and a sky
  drawn later would paint straight over it - this is the whole reason
  `DrawPanes` is not simply another line next to `DrawDecals`;
- it is drawn **farthest first**. Nothing here writes depth, so what is drawn
  last ends up on top whatever the distances say, and a ring of windows has its
  far side behind its near side through the glass all day long. Decals never
  need sorting because every decal on one wall is the same distance from it;
- it is solid. The panes are what a level stores; which voxel each one fills is
  worked back out of it at load time (`core.Pane.Cell`) and `Game.solidAt` adds
  those cells to the blocks the physics pushes against. Deriving rather than
  storing is what stops a level from saying the glass is drawn in one place and
  solid in another.

**A pane collides as a whole voxel, so every face of that voxel has to be
glazed** - the inside as well as the outside. Glazing only the face that looks
outwards leaves the other five solid and invisible, which is an invisible wall
a voxel short of the window, and makes the unglazed side faces of the
neighbouring cells read as gaps between the panes. `glazeRing` therefore takes
the one-voxel-thick skin of the ring and puts a pane on every face of it that
borders something that is not also glass. The two halves of that are what
`TestTheGlassClosesTheRingTheWallLeftOpen` checks: a flood fill from outside
never reaches the middle, and no glass face is left undrawn.

The picture is `WINDOWGLASS` in the shared store, drawn by `tools/gendecals`
alongside the bullet chips, and **it is a flat colour on purpose**. Anything
drawn on a pane - a frame round the edge, a diagonal sheen, a bevel - is a
feature of one pane, and a wall of glass is many panes with their edges
touching, so every such feature tiles into a grid of seams across what is meant
to read as one window. A frame is the worst of them: it turns a glazed ribbon
into a wall of little windows, and the frame on the face you are standing
against reads as a gap rather than as glass. What gives a window its shape is
the hole in the wall around it, which the level already built.

## The station, and what "spinning" means here

`assets/maps/station` is a space station: one deck, a hub, two airlocks, a
glazed window band the whole way round, and space outside to fly in. Two things
in the engine are its and nothing else's, both switched on by the level rather
than by a flag:

- **`core.Station`** is the level as a rotating body. Nothing rotates. The
  blocks never move and gravity inside stays -Y, which is the point: walking
  and jumping on the deck are the ordinary ones because the station is the
  frame everything is measured in. What turns is the sky, at `Spin` degrees a
  second about Y (`Game.updateStationSpin` -> `SetSkyboxSpin`), which is the
  same relative motion a turning hull under fixed stars would show - through a
  window or from outside the hull. A player floating free outside therefore
  turns with it rather than watching it turn; an inertial EVA needs the
  centrifugal and Coriolis terms on the free player, or a world that really
  rotates.
- **The same struct's cylinder is where the air is**, and `eva.go` is what
  happens outside it: no gravity, the movement keys become thrusters along the
  full look direction, and velocity is kept between frames instead of being
  walked off. Which side of the hull the player is on is a test on the position
  and nothing else, so there is no entering and leaving to get wrong - an
  airlock is only where the hull lets you cross, not a switch.
- **Out there the horizon is the suit's**, and so is the frame the mouse turns
  in. On the ground the camera is two angles about the world - yaw about world
  up, pitch off the world horizon, clamped short of vertical so a walking
  player is never on their head. In vacuum `FPSCamera.SetFreeLook` hands the
  same mouse deltas to the basis instead: yaw about the helmet's own up, pitch
  about its own right, no clamp anywhere, so the suit loops over the top and
  keeps going. `Q` and `E` roll it (`RollBy`, `evaRollSpeed`), and jump and
  crouch push along the visor's up rather than the world's. Everything that
  moves the camera goes through `ChangeAngles`, which is where the two ways
  part, so a weapon's recoil follows the visor out there without knowing that
  it does.
- **Gravity is a fact about a place, not a switch on the world.** Inside the
  hull things fall; outside it nothing does, and that is one rule rather than a
  special case per thing that falls. `PhysicsSystem.SetWeightlessAt` is handed
  `Game.inVacuum` when a level with a `Station` loads, which covers everything
  the solver moves - a dropped item, a body, an actor - and `WorldActor.move`
  asks the same question for the things weapons spawn: sparks, shell casings,
  a thrown grenade. The symptom when it was only the player who floated was a
  shotgun throwing sparks that fell out of a vacuum, and a grenade arcing in
  it.
- **What a gun spawns is measured against the visor, not the world.** The
  muzzle offsets a weapon script writes - forward, sideways, and a height - are
  turned by `aimOffset` and `explicitVelocity` about whichever way is up for
  the shooter, which is the camera's own up. Barrel-rolled in a suit, the gun
  on screen is on its side and its muzzle has to be where the barrel now is;
  measured against world up, the shot leaves from where the barrel would have
  been standing on a deck. A pellet spread is the same question asked of a
  shape rather than a point - `spreadBasis` fans wider across than up, and
  across is the shooter's across - so a shotgun held on its side scatters on
  its side. `SXF_ABSOLUTEVELOCITY` is left alone: it says world axes and means
  them.
- **Coming back in is a blend, not a cut.** Leaving free look reads the two
  angles back off wherever the eyes ended up, so gravity taking over moves
  nothing, and then `Game.levelHorizon` eases the roll to level over about a
  second. A player who crosses the threshold looking past vertical cannot be
  written as a yaw and a pitch at all and has their pitch clamped into range -
  which is where the levelling was taking them anyway. `-roll N` is the capture
  flag for photographing any of it, because a capture takes no keystrokes.

`MapLoadResult.SkyFullSphere` is the third piece: a level with no horizon, so
the cubemap covers the whole sphere instead of fading into fog below eye level.
Every level written before there was one keeps its horizon.

## Where the weapons come from

The `assets/weapons/` folder is generated. `tools/genweapons` reads a mod and writes
it; it never runs while the game does, and `weapons.lock` records what the last
run built from what.

**Every source lives in `3rd-party/`, and `./re-migrate.sh` is the only command
that reads them.** The folder is gitignored and `3rd-party/README.md` says what
each thing is and which archive it came out of; a checkout has the generated
`assets/` and not the games behind it. Nothing points into `~/Downloads` or a
scratchpad any more, and nothing is under `/private/tmp` waiting for a reboot to
take it.

```bash
./re-migrate.sh all              # every source below, then a prune and a check
./re-migrate.sh brutal-doom      # or one at a time: blake-stone, system-shock,
./re-migrate.sh mods             # final-doomer, mods, small-gun
```

Every source that produced a weapon in the folder. `doom2.wad` goes last in
every `-source` list: the mods take the base game's sprites and sounds for
granted, and a source list is searched in order.

| source | weapons | prefix | where it is |
| --- | --- | --- | --- |
| Brutal Doom Community Expansion | 43 | `bd_` | `3rd-party/bdce` — a clone of <https://github.com/BLOODWOLF333/Brutal-Doom-Community-Expansion> at `02623c7`, recorded as revision `master` |
| Final Doomer v3.4 | 80 | `fd_` | `3rd-party/FinalDoomer_v3.4.pk3` |
| nine single-weapon community mods | 10 | none | `3rd-party/mods/`, the zips they came in beside them in `mods/archive/`. Ten were downloaded; one does not migrate |
| `assets/smallgun_sheet.png` | 1 | none | in this repo; not a mod at all, see below |
| Blake Stone: Aliens of Gold | 5 | `bs_` | `3rd-party/blake-stone/cd/` — the CD's whole `.BS6` set; only `VSWAP.BS6` is read today |
| System Shock (1994) | 16 | `ss_` | `3rd-party/system-shock/SSHOCK/DATA` |
| Doom II | — | — | `3rd-party/doom2.wad` |

That is 155 weapons in the folder.

The ten community mods, each a Realm667 Armory-format release carrying its own
`CREDITS` or `readme.txt` naming its author, and the weapons each produced:

| mod | weapon |
| --- | --- |
| `Axe.wad` | `axe` |
| `BTConnector.pk3` | `blacktail_51_connector` |
| `Colt45.pk3` | `colt45` |
| `CombatKnife.pk3` | `combat_knife` |
| `DualBerettas.pk3` | `dual_92s` |
| `Hunting Rifle.wad` | `huntingrifle`, `huntingriflescope` |
| `Reaper.wad` | `reaper` |
| `SilencedPistol.pk3` | `silencedpistol` |
| `UACSniperRifle.wad` | `sniper_rifle` |
| `MagnumRevolver.pk3` | **none** — written entirely in ZScript, which is a different and far larger language than DECORATE. It ships no DECORATE lump and this migration cannot read it. |

`small_gun` was never in a mod. `tools/gensheet` builds it out of a plain grid
sprite sheet in this repo, and writes the same `weapon.json` the migration
writes. It is `./re-migrate.sh small-gun`.

**The eightieth Final Doomer weapon is not in the sweep.** `-all` finds 79;
`fd_katana_energy` is the same actor as `fd_katana` - Final Doomer's JPCP fist
becomes an energy katana once the player has the chainsaw, and which of the two
you are holding is an inventory flag only the mod's own scripts ever set, so a
plain compile only ever reaches the first. `-force-item` compiles as if the flag
were held, `-drop-unreachable` deletes everything that leaves stranded, and
`-rename-id`/`-rename-tag` give the result a name of its own. The script has the
line; this is what it is for, and the same three flags are how any other
script-gated second mode would come out.

**A migration is reproducible, and that is checked rather than hoped.** Running
`./re-migrate.sh all` twice writes the same bytes both times, which is what makes
diffing `weapons.lock` mean anything. Nothing in a compile may depend on Go's map
order: `ownedByStoppedLabel` ranged over the label map and kept one name per
step, and because Brutal Doom writes `Ready:` and `Ready3:` on the same frame all
over the place, two runs disagreed about whether the pump shotgun's chambered
shells were a magazine. `TestTheSameWeaponCompilesTheSameWayEveryTime` is the
guard.

`-list` names the weapons in a mod without compiling anything, which is how you
find the actor name the third form wants.

**Re-migrating is reproducible from the folder itself.** Every weapon.json
records the source it came from, its revision and its actor name, so a run that
`re-migrate.sh` has lost can be reconstructed from `provenance` rather than
remembered - which is how the energy katana's own line was recovered. The two
big mods are migrated with `-all`, which compiles leniently; the single-weapon
mods are named one at a time. `-prune` runs last and deletes the shared actor
files nothing points at any more, and deleting `assets/weapons/_sprites.png`
makes the next run repack the shared atlas rather than add to it.

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

**A flag only the mod's ACS ever takes back is dropped.** Nothing here runs a
mod's ACS, so a weapon that gives itself a flag and hands the clearing of it to a
script latches that flag on for good. Brutal Doom's railgun is the case:
firing gives `Pumping`, waits out a recharge in its own states, and calls
`ACS_NamedExecuteAlways("RailgunRecharge")`, whose entire body is a delay of
exactly that recharge and `TakeInventory("Pumping",1)`. The wait is in the state
machine either way and the frames it spends there are `A_WeaponReady(WRF_NOPRIMARY)`,
so the trigger is dead for all of it; the flag is only there to bounce a Fire
that arrives inside the window. With nothing to clear it the railgun fired once
per level. `zdwep.Result.DropFlagsOnlyACSTakesBack` removes the gives, and is
narrow on purpose - the weapon has to call a script somewhere, give the item
itself, never take it back in any of its own states, and read it with a
`jump_if_inventory`; ammo pools and anything `Weapon.Buttons` already keeps are
never touched. It catches `Pumping` on the railgun and `UsedStamina` on thirty
other BDCE weapons, whose sprint script would otherwise leave every one of them
permanently tired.

**A missile's DeathSound is read alongside its SeeSound.** ZDoom plays one from
`P_SpawnMissile` and the other from `P_ExplodeMissile`, so neither appears in any
state and the migration only copies the sounds it has seen. Reading only the
first left the classic plasma rifle firing with a noise and landing without one.
`actorSoundOf` reads both; `WorldActor.hitSomething` plays the second, which is
the same call that sends a missile to its Death state.

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
   `hoveredObj`/`hoveredPickup` reads to outline it, name it, and hand `F` to
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

## Where the characters and the flat scenery come from

`assets/sprites/` is the weapons folder's shape applied to everything in the
world that is a picture rather than a model: one folder a kind, holding a
`sheet.png` of equal-sized frames, a `sprite.json` saying which frames make
which animation, a `boxes.json` of what is drawn on each of those frames, and -
for anything that makes a noise - a `sounds/` folder.

```
assets/sprites/bs_star_trooper/
    sheet.png          a plain grid; sprite.json says how big one cell is
    sprite.json        name, tile, actor, health, scale, animations, sounds
    boxes.json         one box a cell, measured at import, never by the engine
    sounds/alert.wav   cue names, loaded under a scope of the folder's own id
    sounds/death.wav
    sounds/death~2.wav a second take of the same cue; the player picks between
```

**Two boxes, two questions.** `sprite.json` carries one box an animation, the
middle of its facings - what the thing *collides* by, because a solver must not
care where the player is looking or which frame a walk is on. `boxes.json`
carries one box a frame - what it is *picked* by, because a ray is testing the
picture on the screen. Both are measured by the importers, off the sheet they
have just written, and the engine only reads them.

**There is no list of kinds in Go and there must not be one.** A map entity's
`SpriteSheetName` is the folder name, the editor's two palettes are built from
whatever `loadSpriteKinds` found on disk, and `actor: true` in the JSON is the
only thing that decides whether a folder is placed with the actor tool and gets
an `Actor` (health, patrol, dying) or is a decoration in the props palette.
Adding a character is adding a folder.

**Six animations, by name**: `idle`, `walk`, `dying`, `hit`, `prepare_fire`,
`fire` - the six `quads.AnimIndex` values, which is what the engine plays. Only
`idle` is required; a kind that has not got one of the others is registered with
its idle in that slot, so nothing downstream has to check whether an animation
exists. `frames` is indexed **by facing first**: eight rows for something you
can walk round, one row for something that looks the same from everywhere,
which is what a death or an explosion is. Every facing of one animation must
run the same number of frames - `SpriteDrawer` times an animation off facing
zero.

**Every kind gets its own block of six animation slots**, handed out at startup
by `registerSpriteKinds`, and a sprite carries where its block starts in
`SpriteComponent.AnimBase`. `PlayAnimation(AnimWalk)` therefore means *this
kind's* walk. Comparing `CurrentAnimation()` to `AnimWalk` directly is only
right for a sprite whose base happens to be zero - use `IsPlaying(AnimWalk)`.

**`scale` is how tall the art is in voxels**, because the two games do not agree
about it: Blake Stone draws into a fixed 64-pixel tile with the figure filling
most of it, so one number does for everything; System Shock crops every picture
to what is in it, so a kind's scale comes off its own tile height and a med
patch is a quarter of a voxel while a cyborg is two.

**The width follows the frame's shape, and so does the box.** `SpriteKind.Size`
is the scale for the height and the tile's own aspect for the width, because a
non-square frame drawn on a square quad is a picture squashed - and the box it
is picked and bumped into by is squashed with it. `quads.SpriteAABB` measures
the opaque part of the frame the thing is standing on and scales it by that same
size, which is the path a barrel has always been picked by; a guard is a barrel
that walks, and a corpse is measured the same way off the frame it comes to rest
on.

**A box kept per animation is keyed by the sprite's own animation number**, not
the slot it plays out of - `SpriteComponent.LocalAnimation`. Every kind
registers its own six and gets its own block, so the slot for "dying" is a
different number for a guard than for a mutant, and a corpse keyed by the slot
finds nothing and keeps the standing body's box.

**Sounds are the weapon mechanism reused.** A kind with a `sounds/` folder gets
its own cue scope named for the folder, loaded by `loadSpriteKindSounds` with
the same `LoadCueTree` a weapon makes. `playActorCue` works the volume and the
pan out from where the player is standing, because a cue carries a volume and a
pan and nothing else - there is no 3D mixer. Roles: `alert` on spawn, `attack`
when it fires, `death` when it dies.

Two tests walk the whole installed set: `TestEveryInstalledSpriteKindIndexesFramesItsSheetHas`
checks every frame index against the picture's real size, and
`TestEveryInstalledActorHasAllSixAnimations` says out loud which actors fell
back to their idle instead of importing an animation.

## Importing a DOS game

Two of the games this engine borrows from ship their art in their own
containers, and there is a tool a game that reads it and writes the folders
above. Both are re-runnable: they overwrite, they never merge, and every folder
records in its own `provenance` which game and which of its own names it came
out of.

| tool | game | source | what it reads |
| --- | --- | --- | --- |
| `tools/importbs` | Blake Stone: Aliens of Gold | `VSWAP.BS6` off the CD | walls, sprites and digitised sound, all in the one file |
| `tools/importss` | System Shock (1994) | the `DATA` directory | `TEXTURE.RES`, `OBJART*.RES`, `HANDART.RES`, `DIGIFX.RES`, `CYBSTRNG.RES`, `CITMAT.RES`, `OBJ3D.RES`, `OBJPROP.DAT` |

Neither has to be run by hand: `./re-migrate.sh blake-stone` and
`./re-migrate.sh system-shock` are these two calls with the paths already in
them. By hand, they are:

```bash
go run ./tools/importbs -vswap 3rd-party/blake-stone/cd/VSWAP.BS6 -install
go run ./tools/importss -data 3rd-party/system-shock/SSHOCK/DATA -install
```

`-dump DIR` on either writes every picture and sound as its own file instead,
named the way the game names it. That is the form to look at when something
comes out wrong; `-install` is the form the game reads.

**Where the data is.** Both games are unpacked under `3rd-party/`, with the
archives they came out of beside them - see `3rd-party/README.md`. Blake Stone
arrived as a CD image, a `.bin`/`.cue` pair in MODE1/2352, so the ISO is every
sector's bytes 16..2064 and `hdiutil attach` mounts what comes out;
`blake-stone/cd/` is a copy of that, the whole `.BS6` set and not only the one
file anything reads today. System Shock arrived as a zip of a DOS install, and
`SSHOCK/DATA` inside it is the whole of what is read.

**Neither game's file says what anything is.** The pictures are numbered and the
numbers mean nothing on their own, so both importers carry a name table taken
from the source. Both sources are checked out under `3rd-party/`, so a number
that turns out to name the wrong thing is looked up rather than guessed at:

- `tools/importbs/names.go` and `statics.go` are bstone's own `initialize_sprites()`
  and `statinfo` tables (`3rd-party/blake-stone/bstone`), which is where the
  source release recorded that sprite 41 is a barrel. `palette.go` is its
  `vgapal.cpp`: the palette was linked into the executable and is in none of the
  data files. `sounds.go` is its `digitized_map`, because the digitised set is a
  subset of the hundred AdLib effects and is not in the same order. The game's
  own 1993 source is beside it in `3rd-party/blake-stone/blake-stone-source`,
  and `3D_ACT1.C` there is where `statinfo` originally came from.
- `tools/importss` reads the game's own tables instead - `OBJPROP.DAT` for what
  every object is and `CYBSTRNG.RES` for what it is called - through
  `inkyblackness/hacked`, which is the reference implementation of the container
  format and its compression. It is a dependency of this module for that one
  package; nothing in the game imports it.

**The two games do not start their eight views in the same place.** This
engine's facing zero is the front (`core.GetViewDirection`), and so is Blake
Stone's; System Shock's run starts on the thing's own left and reaches the front
six steps later. `viewRotation` in `tools/importss/critters.go` is that offset,
read off the art - the humanoid mutant is a left profile in view 0, its back in
view 2, a right profile in view 4 and face-on in view 6, in the walk block and
the stand block alike. Without it everything imported out of System Shock showed
you its side while it walked at you.
`TestADirectionalIdleNamesADifferentFrameForEveryFacing` is what catches the
next mistake of that shape: a wrong modulus or a clamp lands two facings on one
picture.

**The one thing System Shock does not write down is where a critter's pictures
are.** `OBJPROP.DAT` has a per-critter array of frame counts and in the shipped
file it is all zeroes - the engine fills it in from the art. The layout is
derived in `tools/importss/critters.go`, and the derivation is written out
there: five single-view postures at `1400 + 37*posture + critter`, the
eight-facing stand from 1585, the eight-facing walk from 1880, with a critter
that was only drawn from one side taking one resource rather than eight.

**Nothing between a cue and the speaker resamples.** beep mixes buffers as raw
samples, so a sound recorded at any other rate than the speaker's plays at the
ratio between the two: Doom's own lumps are 11025 and 22050 against a 44100
speaker and came out two and four times too fast, and a Blake Stone voice line
is 7000 and came out as a chirp.

**Anything this repo writes is written at `audio.SampleRate`** - both importers
and the migration's own DMX and ffmpeg paths all go through
`audio.WAVFromPCM8`. Doing it on the way in rather than on the way out is worth
it where the ratio is large: resampling a 7000 Hz effect at load costs 2.5ms on
the game thread the first time it is played, thirty times what decoding it
costs, and a room of guards is a first shout each. At 22050 it is a tenth of a
millisecond.

`loadCue` resamples anything that still is not, which today is the 1513 oggs
that came out of the Doom mods and are copied through the migration untouched -
see the note on that in the migration's `soundBytes`.
`TestACueIsBufferedAtTheSpeakersRateWhateverTheFileSays` covers both.

**The oggs stay as they shipped, and are resampled at load.** They are not worth
converting: an ffmpeg re-encode loses a generation of a lossy format for a tenth
of a millisecond a cue, and decoding them to WAV instead would put four hundred
megabytes in the repo. This is a decision, not an oversight - leave them alone.

**A model's texture identifier is its own citmat slot.** `load_model_vtexts` in
the source release's `rendtool.c` binds each one as `g3_set_vtext(id, resource
0x01DB + id)`, and a textured face names the same id - there is no adjustment to
make. Texture 0 is the exception and is not a texture: it is a placeholder the
game always replaces with a picture off the placed object, and with nothing
placed the import uses the game's own default for a piece of furniture that says
nothing about itself, custom material zero at 0x0884. The pictures are not all
64x64 either - most are 32x32 - so they are stretched into the atlas cell rather
than dropped into a corner of it, because a face's texture coordinates run 0 to
1 across whatever size its own picture is.

**Which noise a System Shock gun makes is in the game's code, not its data.** A
critter's properties carry a sound index; a gun's do not. `weapon_combat_fxs` in
`GameSrc/weapons.c`, indexed by the gun's place among the sixteen and resolved
through `Headers/sfxlist.h`, is the pairing - `tools/importss/objects.go` copies
it out. A digitised effect is resource `201 + id`.

**The camera's far plane is set per map, from the map.** `core` starts every
camera at a hundred units and the gallery is 288 tiles long, so its end wall was
clipped away and the sky showed through it. `mapLoad` sets it to the loaded
map's diagonal. It is nearly free: what a depth buffer can tell apart at a given
distance goes as that distance squared over the near plane, and the far plane
only enters as 1/far.

**System Shock's hand art carries no hotspot** - every frame's anchor box is
zeroed - and its frames are not all the same size, so nothing in the data says
how they line up. The game did not read it from the data either: it has a table
per weapon and draws each bitmap's top left corner at the offset the table
gives, hanging from a line at y=168 in a 200-tall view and clipping whatever
falls past the bottom. `tools/importss/handart.go` is that table, out of the
source release's `GameSrc/hand.c`. It also says which bitmap each step of an
animation shows, which is not file order: a swing is five steps out of four
pictures, and the rapier starts on its second.

**A weapon out of either game is a grid of frames**, which is what
`wep.WriteGridWeapon` takes - the same call `tools/gensheet` makes for a weapon
that was never in a mod at all. Blake Stone's five and System Shock's sixteen go
in that way, with the game's own firing sound in the folder's `sounds/` and, for
the Fission Detonator, its own thrown grenade as a shared actor.
`go run ./tools/firecheck` holds the trigger down on every weapon in the folder
and says which never fired.

**The check that an import worked is a level** - see "The gallery" below.

## Enemies that shoot back

`hostile.go` is the whole of it: a player with health, and actors that come
after them.

**Hostility is a fact about the kind, not the map.** `"hostile": true` in a
folder's `sprite.json` is what makes an actor chase and attack; both importers
write it for every character whose own game gave it an attack, and the scientist
- the engine's own first character - does not have it. `"attack"` beside it is
what that attack does: `damage`, `range`, `interval`, and whether it is a
`projectile`. Neither importer invents those numbers - System Shock's are its
critters' own primary-attack block, Blake Stone's reach and kind come from which
frames the character was drawn with (`SHOOT` is a gun, `SPIT` is a thing that
flies, `SWING` is a claw).

**The mind is four states and no more**: patrol until the player comes within
`hostileSightRange` *and* there is a clear line; chase, re-pathing on a clock
because a path is a whole A* search; attack when inside its own reach; give up
past `hostileGiveUpRange`. `hasLineOfSight` tests walls only - not props, not
the guard standing next to it - which is the rule both source games use and the
only one that does not turn a crowd into a queue.

**The player has a hundred points and `hurtPlayer` is the only thing that takes
them.** God mode is checked there rather than at every source of damage, so a
new source is covered by being written at all. Death puts the player back at the
level's own spawn with a full bar and drops every hunt, because a room of
enemies still mid-chase would shoot the moment the screen came back.

**An enemy's projectile settles its damage as it is thrown.** The bolt that
flies is the engine's own (`fireProjectile`), which is drawn and lit, but the
collision handler it would arrive in kills an actor outright - and a bolt an
enemy threw must not do that to the enemy beside it. See the `ponytail:` note on
`fireEnemyProjectile` for what giving `ProjectileComponent` an owner would buy.

## Beams

`beam.go` draws the bolt of light an energy weapon leaves between its muzzle and
whatever it hit. It is not in any of the art: System Shock draws three straight
turquoise lines over the view in `gamerend.c`, and the migrated railgun never
had one at all. `OpRailAttack` spawns one, so both get it.

Three things about it are not decoration and will break it if they are undone:

- **The muzzle is well off the line of fire.** A beam that started on it would
  run straight away from the eye, and a line pointing at you projects to a
  point. System Shock has the same problem and solves it the same way, by
  drawing from the weapon at the bottom of the view up to the impact in the
  middle of it.
- **Both ends are drawn at the same distance from the eye, and it is a short
  one.** They are a barrel about a unit away and an impact twenty units away,
  and one quad of one width stretched between those is a wedge: the 0.05 units
  that was nineteen pixels at the muzzle was under one at the far end, so the
  beam read as a bright blob over the gun and nothing along its length, and
  pitched down it was all blob. `beamOnScreen` slides each end along its own
  line of sight to `beamDrawDistance`, which leaves both on exactly the pixel
  they were already on - every point on a ray through a pixel projects to that
  pixel - and makes the quad a rectangle at one depth. That is a line of even
  width at any range and any pitch, and it is also what System Shock does:
  `gamerend.c` draws in screen space, not in the world. The beam is then in
  front of everything, which is the same bargain those flat lines made.
- **The three axes have to come out right handed.** `SpriteModeBeam` builds its
  own basis - along the beam, across it, facing the camera - and the first
  version had the last two the wrong way round. The quad wound backwards, the
  face cull threw it away, and the beam was spawned and updated and drawn every
  frame and never appeared once.

**A beam lasts four frames.** `beamLife` is ninety milliseconds, so a screenshot
taken on an arbitrary tick usually catches the gap between two shots rather than
a beam. Judging a change to this by eye means a strip - `-shot-every 1
-shot-count 12` - not a single frame.

## The F1 menu's cheats

`cheats.go`: Give All Weapons, Fill All Ammo, Full Health, God Mode, Kill Every
Enemy. Giving a weapon is the same `Weapon.give()` a pickup makes, so the sounds
load the same way; killing an enemy is the same `Actor.die()` a shot causes, so
the bodies, the cries and the colliders all behave the way they do when it is
earned.

## The F1 menu's actors page

Every NPC is inactive when a level loads: it stands where it was put and does
nothing. `actor_debug.go` is the switch, and one button per animation beside it.

That is the default because a level of imported characters is a level of things
that immediately run at you, and most of the work of checking an import is
looking - at a walk cycle, at which frame a death rests on, at whether a facing
is the right way round - none of which can be done while they are closing in.
Everything that moves an actor is below one `if` in `updateActors`, so inactive
is not a state they are in; it is a thing that stops happening to them.

## Two levels for the two things that break

`assets/maps/facingcheck` is a small room with four enemies in it, two out of
each game, spawned inside `hostileSightRange` so the fight starts on its own. It
is the level to load when the thing being changed is the fighting: the gallery
below has everything in it but is two hundred voxels long, and walking to the
enemies takes most of a minute.

## The gallery, and where to look when an import goes wrong

`assets/maps/imported` is a hall with one of everything installed standing in
it. It is written from the folders rather than by hand, because there are six
hundred of them:

```bash
go run ./tools/mapgen -gallery       # write its build.map from what is installed
go run ./tools/mapgen -map imported  # compile it like any other level
go run . -map imported               # walk it
```

Four bays in the order you meet them: the scenery, which is walked through
rather than round; the characters that do not fight; every hostile both games
have; and System Shock's own 3D models. Take the cheat menu's weapons before
going past the second bay.

## System Shock's own 3D models

`tools/importss` also reads `obj3d.res`, which is not a mesh file: a model is a
little program of two-byte commands with no framing, so an interpreter has to
know every command or it loses the stream. `tools/importss/obj3d.go` is that
interpreter and `models.go` turns what it builds into `assets/meshes/ss3d_NN/`,
which makes each one furniture like everything else in that folder - placeable
from the editor's scenery menu and by `entity ... mesh=ss3d_NN`.

- **Every model shares one picture.** A face is either mapped to one of the
  sixty-four object textures in `citmat.res` or painted a single palette colour;
  both go in one atlas, the textures in a grid and the whole palette as swatches
  under them, so a model is one material and one draw.
- **Sixty-three of the eighty are named after the object drawn as them.** The
  link is the object's own MFD/Mesh ID: for an object in a panel that field says
  which readout to open, and for one in the world it says which mesh to render.
  The render type to look at is `TextPoly`, which reads like it is about writing
  and is not - it is the textured-polygon one, and sixty-four of the sixty-five
  objects with it name a mesh where `TPoly`, `FlatPoly` and `TLPoly` name none
  at all. The rest keep their resource number, and every folder carries the
  number anyway (`ss3d_16_couch`) so the order on disk is the order in the file.
- **A model unit is a voxel, so nothing is resized.** The models are drawn by
  the same `g3` pipeline as the sprites and in the same world units: a vertex is
  fix 16.16, and one of those units is one map square (`obj_coord_from_fix` in
  `map.h` is a shift of eight off a coordinate whose top byte is the square).
  That is the same trace as the sprites' ninety-six pixels a square, so 96
  pixels of art, one model unit and one voxel are all the same length and the
  two halves of the import cannot drift apart. `modelScale` was two for a while
  and every piece of furniture was twice the size it should be - a chair 2.3
  metres tall standing beside a 1.7-metre mutant.
- **The tree is thrown away.** The nodes are a BSP for the game's own renderer,
  which had no depth buffer; every face in every branch is collected and the
  depth buffer does what the tree was for.
- **Each one is lowered onto the floor** by its own lowest point, because the
  game placed these from level data and their origins are wherever that data
  wanted them. A piece the game hung on a wall therefore stands on the floor
  instead, which is the one thing about them that is not faithful.

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
  the nearest pickup it passes through, `DrawGui` outlines and names it, `F`
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

## The references worth reading before guessing

**All three engines are checked out in `3rd-party/`.** An importer is a guess
about somebody else's file format until you read the code that wrote it, so a
wrong colour, a wrong offset or a shot at the wrong pitch is traceable to the
line that decided it rather than something to argue about. See
`3rd-party/README.md`.

- **GZDoom** — `3rd-party/gzdoom`, <https://github.com/zdoom/gzdoom>. Where a
  weapon lands on screen is `src/rendering/hwrenderer/scene/hw_weapon.cpp`
  (`HUDSprite::GetWeaponRect`) and `src/playsim/p_pspr.cpp` (`DPSprite`). What
  an action function actually does is ZScript now, under
  `wadsrc/static/zscript/actors/inventory/` - `A_ReFire` is in
  `stateprovider.zs` and `GetAtkState` in `weapons.zs`, and between them they
  are why a held trigger re-enters `Hold` and a released one falls through to
  whatever comes next. When a weapon draws or behaves wrong, read those rather
  than inventing a correction factor.
- **bstone** — `3rd-party/blake-stone/bstone`,
  <https://github.com/bibendovsky/bstone>. The Blake Stone source port, and
  where `tools/importbs`'s name, static and sound tables and its palette came
  from. Blake Stone's own 1993 source is beside it in
  `3rd-party/blake-stone/blake-stone-source`
  (<https://github.com/johndrinkwater/blake-stone>) - `3D_ACT1.C` holds the real
  `statinfo`, and it is the tiebreaker when bstone and the data disagree.
- **The System Shock source release** — `3rd-party/system-shock/shockmac`,
  <https://github.com/NightDive-Studio/shockmac>. `Game Src/WEAPONS.C`,
  `RENDTOOL.C` and `GAMEREND.C` are the three `tools/importss` was written
  against. **Its files have classic Mac line endings**, so `grep` calls them
  binary and prints nothing at all rather than saying it found nothing - a
  silent wrong answer. Use `grep -ra`, or `tr '\r' '\n' < FILE | grep -n` when
  you want line numbers.
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

## An imported sprite is the size its own game drew it

Both importers work a kind's height out from the source game rather than from
taste, and both traces are worth keeping:

- **Blake Stone**: the Wolfenstein engine draws a sprite exactly as tall as a
  wall, and both are its 64-unit tile. A wall here is one voxel, so
  `spriteScale` is **1**. A guard fills about 56 of those 64 - 0.875 - which is
  this engine's player exactly, because the player is Doom's 56 units of
  `worldUnitsPerDoomUnit`.
- **System Shock**: `g3_set_bitmap_scale` is left at `fix_make(0, 2048/3)` =
  1/96 for every object in the world, and 96 is `PHYSICS_RADIUS_UNIT`, "radius
  units per square". One pixel of art is a ninety-sixth of a map square, and a
  map square is a voxel: `pixelsPerVoxel` is **96**.

`-ss-scale` and `-bs-scale` multiply every sprite from one import at once (see
`sourceScale` in `sprite_kind.go`). They are taste knobs on top of those two
numbers, not a replacement for them; both default to one.

A voxel is about two metres of room. The station runs its corridors two voxels
floor to ceiling; a hall built fourteen voxels tall is a cathedral.

**And the box it is bumped into by is the middle of its eight facings, not the
widest.** Each animation's box is measured off the sheet at import time
(`measureBoxes`, and `geometry.BoxAcrossDirections` under it), and the frames of
one facing are unioned into that facing's box before the facings are reduced to
the one box a billboard can have. Unioning the facings together instead - which
is what it used to do - gives the widest silhouette the thing ever has from any
angle, and for art cropped to its own content that is the whole tile: System
Shock's avian mutant is 140 pixels across with its wings out sideways and about
70 face on, so its box was twice as wide as the bird in front of you and you
walked into it a body's width early. A kind drawn from one angle only - an
explosion, a medkit on the floor - has a single facing, so its box is still the
union of its own frames and a growing blast still gets a growing box. `-obb` on
the command line draws every collider, and `assets/maps/boxcheck` is six things
in a row to draw them around.

## Extra frames are not always an animation

System Shock's `Bitmap3D` carries an animation bit, and only eight of the
eighty-three multi-frame objects have it. The rest use the frame count for
something that is not time: a lever up or down, a panel intact or smashed, and
- for every corpse in the station - **eight views of the same body**, which is
  exactly this engine's eight facings. `idleFrom` in `tools/importss/objects.go`
sorts them: animated stays an animation, exactly eight stills become the
facings, anything else stands on frame zero.

The exception is class 11, `CLASS_ANIMATING` - the explosions and fires. None
of them carries the bit, because the game animates them out of `effect.c`
rather than off the animation list.

## Rate of fire comes out of the game, not out of feel

- **Blake Stone**: `attackinfo` in bstone's `3d_agent.cpp` is tics per attack
  frame, and `T_Attack` decides how much of it a held trigger walks - the
  pistol runs all four frames and starts again (24 tics), while the rifle, the
  cannon and the launcher jump back two from the third, so their held cycle is
  the middle pair (11, 9, 12). Blake Stone counts at 70 tics a second and this
  engine at Doom's 35, so halve it.
- **System Shock**: `GunProps[..].fire_rate`, the first byte of the gun class's
  generic properties. Everything that is not automatic waits `fire_rate * 10`
  ticks; an automatic weapon accumulates `elapsed * fire_rate` and spends
  `AUTOFIRE_SPEED` (1500) a shot, so its rate goes *up* with `fire_rate` where
  every other weapon's goes down. `CIT_CYCLE` is 280 ticks a second.

## Four System Shock guns throw something

`player_fire_weapon`'s switch is the whole of it: `GUN_SUBCLASS_SPECIAL` (the
magpulse and the rail gun) and `GUN_SUBCLASS_BEAMPROJ` (the stungun and the
plasma rifle) call `player_fire_slow_projectile`. Each names what it throws as
a `MAKETRIP` in its own subclass-specific block, after the eight packed bytes
of `COMBAT_DATA_FIELDS` - see `projectileTriple`. The other twelve are hitscans,
beams or swings.

**The rail gun is one of the four, and that is not a bug.** It looks like a
projectile thrower because it is one: its record is `GUN_SUBCLASS_SPECIAL` and it
names a thing that flies. All four fly at the same speed, too - the byte before
`proj_triple` reads 6 in every one of the four records. What tells the rail gun
apart from the other three is only that its slug is metal, so it is the one
whose flying frames are not drawn bright (`slowProjectile`'s `bright`
argument).

## An object's inventory picture is the first block of its run

`objload.c` walks each object as: the 2D bitmap (the inventory and floor
picture), then its in-world frames, then the editor's icon. The importer took
the world frames and skipped the first block, which is why the guns had no
pickup art and the HUD drew a first-person barrel at icon size.

## A model's texture coordinates are already 0 to 1

None of the eighty models tiles a texture. What runs outside the unit square is
a few thousandths of overshoot on a face whose corners were meant to be the
corners of the picture, so `insetInCell` **clamps**, half a texel clear of the
cell edge. It used to wrap with `fmod`, which turned the far edge of a face -
at exactly 1 - into 0, and every face mapped to the whole of its texture was
drawn as a smear of its first column.

## Fill-all-ammo fills ammo

A migrated weapon's `Items` are mostly not ammo: they are the flags its own
script signals itself with. Brutal Doom's assault rifle declares a hundred of
them (`FiredPrimary`, `Reloading`, `Kicking`) and Final Doomer's Zaiger rifle
has an `FDLevelBeginAnimation` its ready state checks to play one reload at
spawn. The cheat used to fill every one of them to its cap: the rifle fired
without a trigger and the Zaiger stood in a reload it was told it had just
started, for ever. `ammoPools` is what a weapon actually spends - `Ammo.Type`,
`Ammo.Type2`, `Ammo.Clip`, `AmmoPickup` - and nothing else.

## Walk through it and still be able to select it

`core.MapEntity.PassThrough` puts an entity on the physics grid without making
it solid: the crosshair finds it, its OBB outlines it, the collision solver
skips it. `collider=false` in a map script leaves it off the grid altogether,
which means it cannot be selected at all - that is what made the gallery's five
hundred exhibits invisible to the crosshair. A pass-through piece of scenery is
also not `furniture`, because there is nothing standing on it to look through
it at.

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
