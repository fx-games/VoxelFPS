# AGENTS.md

Notes for agents working in this repo.

## Searching

`git grep -n pattern` searches the tracked tree. It is faster than `grep -r`
and skips `weapons/`'s six thousand generated files, which is where a plain
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
| `-weapon ID` | start holding this weapon, by its `weapons/` folder name |
| `-autofire` | hold the trigger down for the whole run |
| `-altfire` | hold the second attack button down: a scope, a bayonet, a second mode |
| `-cycle N` | put the next weapon in hand every N seconds, to walk the whole folder |
| `-tap D,U` | tap the trigger D tics down, U up, for a weapon that acts on a press |
| `-width`/`-height` | window size, for a capture that needs more pixels than 640x480 |

**A capture is deterministic.** `-shot-fps` makes every frame advance the world
by the same amount whatever the machine took to draw it, so tick 120 of one run
holds the same animation frame as tick 120 of the next. That is what makes a
before-and-after comparison mean something; without it a faster machine lands
on a different frame and every diff looks like a change. The debug overlay and
the minimap are switched off for the run so they are not in the picture.

On a hidpi display the PNG is the framebuffer, not the window: a 640x480 window
writes a 1280x960 file.

F12 still saves `screenshot00.png` while playing, for the times a capture needs
a place you have to walk to.

## Seeing why a weapon stops working: DEBUG_WEAPON

`DEBUG_WEAPON=1` makes the weapon in hand say where its script is, once a tic,
next to everything it is carrying that has a capacity:

```
[weapon] fire[807] MP42B0 1t   DualMP40Ammo=64/64 MP40Ammo=32/32 Mauser9mm=200/200
```

A weapon that has gone invisible or will not fire is a weapon whose script has
walked somewhere unexpected, and this is how you see which state. A counter
sitting at zero next to a state that keeps jumping away is a weapon waiting on
something nothing gives it. `shots` next to a known number of simulated seconds
is a rate of fire.

## Seeing why an effect looks wrong: DEBUG_ACTORS

What a muzzle smoke or a shell casing looks like is decided by five numbers out
of five different places - the picture, the actor's scale, its alpha, its render
style and the light where it stands. A screenshot says it looks wrong; this says
which of the five it is.

```bash
DEBUG_ACTORS=1 go run . -weapon bd_chaingun -autofire -shot /tmp/s.png -shot-at 129 2>&1 |
    grep '\[actor\]' | sort -u
```

```
[actor] HitpuffSmoke SMOKA0 at=[4.5 1.4 8] size=1.13x1.15 scale=0.70 alpha=0.40 style="translucent" emissive=0.00 baked=8
```

A negative size means a script wrote a negative scale and something forgot that
ZDoom reads that as a mirror. `baked=0` on something standing in a lit room
means it is being lit as if it were in the dark. A size in the tens means the
pixels-per-unit conversion is wrong for that sprite. And `at=` next to the
player's own position says whether a spawn landed where the call asked or in
the camera's lap.

## Where the weapons come from

The `weapons/` folder is generated. `tools/genweapons` reads a mod and writes
it; it never runs while the game does, and `weapons.lock` records what was
built from what.

```bash
go run ./tools/genweapons -source ../Brutal-Doom-Community-Expansion,~/Downloads/doom2.wad -list
```

Sources, and where they are:

- **Brutal Doom Community Expansion** —
  <https://github.com/BLOODWOLF333/Brutal-Doom-Community-Expansion>. Clone it;
  `weapons.lock` names a clone that no longer exists.
- **Final Doomer** — `~/Downloads/FinalDoomer_v3.4.pk3`.
- **doom2.wad** — `~/Downloads/doom2.wad`. Needed underneath the mods: they
  take the base game's sprites and sounds for granted, so it goes last in
  `-source`.
- The single-weapon community mods are the zips in `~/Downloads`: `Axe.zip`,
  `Colt45.zip`, `CombatKnife.zip`, `DualBerettas.zip`, `HuntingRifle.zip`,
  `MagnumRevolver.zip`, `Reaper.zip`, `SilencedPistol.zip` and the rest.

**Re-migrating is reproducible from the folder itself.** Every weapon.json
records the source it came from, its revision and its actor name, so the runs
that built the folder can be reconstructed from `provenance` rather than
remembered. The two big mods were migrated with `-all`, which compiles
leniently; the single-weapon mods were named one at a time. `-prune` afterwards
deletes the shared actor files nothing points at any more, and deleting
`weapons/_sprites.png` makes the next run repack the shared atlas.

## The two references worth reading before guessing

- **GZDoom's own source** — <https://github.com/zdoom/gzdoom>. What a weapon
  does on screen is decided in `src/rendering/hwrenderer/scene/hw_weapon.cpp`
  (`HUDSprite::GetWeaponRect`) and `src/playsim/p_pspr.cpp` (`DPSprite`). When
  a weapon draws wrong, read those rather than inventing a correction factor.
- **The DECORATE reference** — <https://zdoom.org/wiki/DECORATE>. It is the
  language `zdwep` compiles, and the wiki page for an action function says what
  its arguments mean, which is what `zdwep/policy.go` has to agree with.

## How GZDoom draws a weapon, since everything here depends on it

A weapon is laid out in a fixed 320x200 space and that space is scaled to the
view, so a gun is the same size on every screen. Only the height matters:

    scaleY = viewHeight / 200
    scaleX = scaleY / 1.2          // Doom's pixels are not square

The frame's top left corner then goes at

    x = viewWidth/2  + (psx - 160 - hotspotX) * scaleX
    y = viewHeight/2 + (psy - 100 - hotspotY) * scaleY

`hotspotX/Y` is the picture's own offset: the two numbers in a Doom patch's
header, or in a PNG's `grAb` chunk. It is **not** a pivot fraction and **not** a
delta against another frame. A vanilla weapon frame's offset is
`(width/2 - 160, height - 168)`, which is exactly what puts a gun in the middle
of the screen with its bottom edge on the screen's bottom edge; a mod's recoil
frame sits a few pixels off that, and that difference *is* the animation.

`psx/psy` is the psprite position, which persists from frame to frame:

- it starts at `(0, 32)` - `WEAPONTOP`;
- a frame's `offset(x, y)` sets it absolutely, and only on the axes it writes,
  because ZDoom ignores a zero;
- `A_WeaponReady` puts it back to `(0, 32)`, which is what ends a recoil the
  script never mentions again;
- `A_Raise` and `A_Lower` walk it between `WEAPONTOP` (32) and `WEAPONBOTTOM`
  (128) at six a tic;
- a flash overlay's own position is *added* to the weapon's, so a muzzle flash
  follows the gun through a recoil for free.

The weapon actor's `Scale` property does **not** apply to a weapon sprite -
GZDoom only scales the pickup lying on the floor by it. Applying it moves every
Brutal Doom gun off the bottom of the screen. It still applies to spawned world
actors, which is where `Render.Scale` is read.

## How GZDoom places what a weapon spawns

A spawn is measured from the actor that made it, never from its eyes, and the
two families of spawning function measure differently. Reading a number off the
wrong argument or the wrong axis is not subtle: it is what put the chainsaw's
idle exhaust in front of the camera instead of behind the player, and filled
the screen with grey smoke.

| call | arguments | forward | sideways | up, measured from |
| --- | --- | --- | --- | --- |
| `A_SpawnItem` | `(type, distance, zheight, useammo, transfer)` | `distance` | - | the spawner's feet |
| `A_SpawnItemEx` | `(type, xofs, yofs, zofs, xvel, yvel, zvel, angle, …)` | `xofs` | `yofs` | the spawner's feet |
| `A_CustomMissile` | `(type, spawnheight, spawnofs_xy, angle, …)` | - | `spawnofs_xy` | its middle |
| `A_FireCustomMissile` | `(type, angle, useammo, spawnofs_xy, spawnheight, …)` | - | `spawnofs_xy` | its middle |

Three of those four order their numbers differently, and `A_SpawnItem` has no
angle at all - its third argument is `useammo`, a flag.

Every sideways offset is to the **right**, including `A_SpawnItemEx`'s `yofs`.
That one looks as though it should not be: it is written as a rotation of a
plain `(x, y)` pair by the yaw, and Doom's +y is 90 degrees anticlockwise from
its +x. GZDoom flips it on purpose, and says so in the source - *"in relative
mode negative y values mean 'left' and positive ones mean 'right'. This is the
inverse orientation of the absolute mode!"*. Reading it the other way puts each
gun of a dual wielded weapon's muzzle flash beside the other gun, which is what
the picture shows and no amount of re-deriving the rotation will. A missile has no forward
offset because it leaves from inside the shooter and gets anywhere under its own
Speed; giving it one holds it in the player's face for as long as it lives.
An angle turns the other way from that. ZDoom counts one anticlockwise, so a
positive angle throws a thing to the spawner's *left*, and it turns the spawn's
velocity as well as its position - dropping it left every ejected shell casing
flying down the line of fire to land in front of the player. Brutal Doom names
the direction itself: `50CaseSpawn2 //right side eject` throws its brass at
`random(-120,-140)`, and the left handed gun of the same pair at
`random(120,140)`.

Offsets here are rotated by the whole aim rather than by the yaw alone - see
"The muzzle flash actor rides the aim" below - so stepping something forward
while looking at the ceiling steps it up along the barrel.

Doom's units convert at `worldUnitsPerDoomUnit` (1/64) and there is exactly one
of those: a wall panel is 64 units and a voxel here is 1, the player is 56 units
and 0.9 voxels tall, and a sprite is one map unit per pixel before its own
`Scale`.

Two more things that decide what an effect looks like:

- **`BRIGHT` is a property of the frame, not of the actor.** A frame marked
  BRIGHT ignores the light where it stands; an additive render style does not
  make anything bright, and a translucent actor's bright frames are.
- **What lands stops.** A thing that comes to rest on the floor is sent to its
  Death state, which is the frame a shell casing lies still on. In ZDoom that
  is `+MISSILE`'s own doing, and the flag does not survive the migration when
  the mod writes it as DECORATE's `PROJECTILE` keyword, so the engine does not
  wait for it: a tumble is a Loop with no exit of its own and would otherwise
  spin on the ground for as long as the casing lives.
- **A_ChangeVelocity adds; it does not replace.** Only `CVF_REPLACE` replaces,
  and `CVF_RELATIVE` measures along the thing's own heading. Read as a
  replacement, Final Doomer's C4 charge - thrown forward as a missile and then
  given a small upward kick - went straight up out of the player's hands.
- **Something that lands with its script still running is waiting, not
  playing out.** It gets `plantedActorLifetime` rather than the eight seconds
  an effect gets: a C4 charge stuck to a wall is waiting to be told to go off.
- **A `Stop` destroys the actor.** The state is literally called NULL in
  ZDoom and nothing runs after it. Kept alive to keep falling - which reads as
  the kinder thing to do - a casing spawner that had already dropped its brass
  on the way through `Spawn` landed a moment later, was sent to `Death` by the
  engine's own hit handling, and dropped a second casing. Every gun in Final
  Doomer ejected twice. A casing itself never stops: its tumble is a `Loop`,
  and landing is what ends it.
- **A zero-tic chain reads what the step before it wrote.** `publishState` runs
  between ops, not once a frame. A smoke puff shrinks itself to a tenth and
  then mirrors itself with `scalex - scalex*2`; against a frame-old `scalex`
  that second step came out as the unshrunk size, and one puff in two filled
  the screen with grey.
- **`RenderStyle Add` has to actually be additive.** Drawn with the ordinary
  blend it darkens instead of glowing, and forty overlapping puffs of a haze
  authored at six percent alpha turn a lit room black. It also must not write
  depth, or the nearest puff hides every sprite drawn after it.

## Weapon bob

The gun sways while the player walks, in the same 320x200 space its position is
in: `weaponBobAt` in `weapon.go`. Doom's own shape - the sweep across is a full
circle and the dip is that same angle folded into half of one, so the gun goes
down twice for every sweep and never rises above where it is held. Sixty four
tics to the cycle, sixteen units at its widest, amplitude the player's speed
squared and quartered and capped, which means full sway whenever they are moving
and none the moment they stop. `A_WeaponReady(WRF_NOBOB)` turns it off until the
next `A_WeaponReady` that does not, which is how a reload holds still.

It is a function of speed, so it never shows in a capture that stands still.
`-walk` holds the forward key down.

## The player never sees the muzzle flash actor, and neither should we

Brutal Doom's `PlayerMuzzle1` is a world actor, spawned by `A_SpawnItemEx` at a
fixed offset from the *player* - 32 units ahead, 28 up from the feet - for one
tic. **In GZDoom the player who fires never sees it.** The mod ships five lumps
for its one frame - `PLMZA1`, `PLMZA2A8`, `PLMZA3A7`, `PLMZA4A6`, `PLMZA5` -
and the last two are authored with no opaque pixels at all. A spawn faces the
way the spawner faces, so the shooter is looking at its back, which is rotation
5, which is blank. The flash is there for everyone *else*; what the shooter
sees is the flash drawn into the weapon's own frames.

`AssetIndex.Sprite` used to take the first lump in sorted order, which is
rotation 1, the view from the *front* - so every gun in the mod fired a 21 unit
fireball into the player's face. It now takes rotation 5, then 0, then whatever
the mod has, and the folder was re-migrated on that rule. Thirty one frames
changed picture: `PLMZA0` to the blank one, and thirty others - `TRAC`, `MISL`,
`AXET`, `GBPJ`, `PLAY`, `SPOS` and the rest - to the back of a thing that is
flying away from the player, which is the side they were always looking at.
Fifty weapons name one of them, and `firecheck` reports the same 36 weapons
failing before and after, so nothing else moved.

Offsets themselves ride the aim rather than the yaw: `aimOffset` in
`world_actor.go` measures a spawn's forward, sideways and height along the line
of aim, so what a weapon throws out follows the barrel down. That is a
deliberate departure from GZDoom, and the only one in how a spawn is placed - a
level shot puts a spawn exactly where the yaw-only maths does.

What is attached to the barrel is the weapon's own frame. `MP42B0` and `MP42D0`
are the idle gun with fire drawn on the muzzle, which is why they are 116 pixels
tall where `MP42A0` is 72. Both are drawn, and the small bright flash at the
barrel is the frame, not the actor.

What genuinely is missing is **weapon bob**: GZDoom sways the psprite in a
flattened figure of eight while the player walks, `A_WeaponReady` setting
`PSPF_ADDBOB` and `P_BobWeapon` adding to `psx`/`psy` at draw time. Nothing here
does that, so the gun is unnaturally still. It is proportional to the player's
speed, so it never shows in a `-shot` capture, which stands still.

## The player is Doom's player, and so is the view

Everything a weapon spawns is placed relative to the player, so the player has
to be the one the mods were authored against: 56 units tall, looking out at 41,
which is `playerHeight` and `playerEyeHeight` in `world_actor.go` and the AABB
in `spawn_entities.go`. The view is `DoomVerticalFOV`, 73.74 degrees, which is
Doom's 90 across at 4:3.

Both were nearly-but-not-quite before, and it showed. The view was 45 degrees
vertical, a telephoto lens next to Doom's, which drew everything in the world
about 1.75 times larger than its authors saw it - Brutal Doom's muzzle flash is
a 21 unit sprite 32 units in front of the player, 36 degrees across, which is
half of Doom's view and most of a 45 degree one. And the eye was at 45 units
instead of 41, which is enough to put that flash below the bottom of the screen
until you looked down.

A weapon sprite is unaffected by the field of view either way: GZDoom lays those
out in a fixed 320x200 space rather than through the projection. That is exactly
why the two only look right together at Doom's number.

## A magazine starts loaded; a counter does not

Both are `: Ammo` with a maximum, and DECORATE says nothing about which is
which. The engine hands every weapon out at once with nothing to pick up, so a
magazine that started empty is a weapon born empty - Brutal Doom's dual MP40
keeps its right hand magazine in a third ammo pool the weapon never names, and
that gun did not fire a single shot.

What tells them apart is that **a reload fills a magazine**. `reloadedItems` in
`zdwep/compile_weapon.go` walks the states a Reload reaches and collects what
they give. It stops at the labels that are the weapon's other business - the
kick, the taunt, the sprint - matched anywhere in the name, because a mod writes
`DoKick` and `FinishExecution`, and because ZDoom's numeric jumps compile to
invented `@frame` labels part way inside a region and would otherwise let the
walk in through the side door.

## A sprite name is only unique inside its own mod

`weapons/_sprites/` is one store shared by every weapon, because a smoke puff
spawned by six guns should be one picture. It is keyed by content, not by the
frame's name: `SMOKA0.ac80a16d.png`, with the compiled actor naming that.

Keying it by the bare name looks obviously right and is not. Brutal Doom's
`SMOKA0` is 56 pixels square; Final Doomer's is 141 by 130. Both wrote to
`_sprites/SMOKA0.png`, the writer skipped a name it had already seen, and Final
Doomer happened to be migrated last - so every Brutal Doom weapon spat a puff of
smoke two and a half times too big, and a re-migration could never fix it,
because the file was already there. Seventeen frames collided, all of them
smoke.

Two mods that really do ship the same bytes still share one file. `-prune`
clears both shared stores of anything no installed weapon names - except the
engine's own decal pictures, which live there too and which no weapon.json will
ever name, so `wep.EngineDecalSprites` is what keeps the sweep off them.

The sweep is exact, not approximate: after a migration both stores hold the 820
actors and 2163 pictures the installed weapons reach and nothing else. A name
that looks like it belongs to a weapon that was removed usually does not -
`X727B0.a24d62e5.png` reads as a throwing animation nobody has any more, and it
is Brutal Doom's fists throwing a zombie civilian's corpse.

## The mod's control script, which nothing here runs

Brutal Doom's weapons never read the player's buttons. `BDControls.txt` is an
ACS loop that reads them, gives an inventory item for each one that is down -
`FiredPrimary`, `Reloading`, `PressedUse`, `IsCrouching`, `IsRunning` - waits
four tics and takes them all back. The weapons branch on those items, and a
full automatic is written as a jump at the end of its firing states:

    SMGR A 1 A_JumpIfInventory("FiredPrimary", 1, "Fire")
    Goto Ready3

Nothing here runs ACS, so the engine keeps those items itself. Which item stands
for which button is a decision about a mod's vocabulary, so it lives in
`ControlItems` in `zdwep/policy.go` next to every other one; the compiler writes
the ones a weapon actually reads into its `buttons`, and `Runtime.SetButtons`
gives and takes them. Without it every automatic in the mod fires exactly once
per press.

That pairs with `+WEAPON.NOAUTOFIRE`, which is on twenty six of these weapons.
It stops the *ready frame* from re-firing while the trigger is held, so the
weapon's own script decides the cadence. Both halves are needed: without the
flag a semi-automatic empties its magazine in two seconds, and without the items
an automatic is a semi-automatic.

## An item the mod's ACS takes back is an item nothing here takes back

`ControlItems` covers the ones a control script keeps in step with the buttons.
The other half of the same problem is an item a *weapon* gives itself and hands
an ACS script the job of clearing.

Brutal Doom's railgun is the case that shows it. Its Fire state opens with
`A_JumpIfInventory("Pumping", 1, "Ready")`, and all three places that give
itself `Pumping` - the shot, the alt shot, and the reload's `PutMag` - follow it
with `ACS_NamedExecuteAlways("RailgunRecharge")`. Nothing here runs ACS, so the
item stayed for ever: the gun fired exactly once, and every trigger pull after
that walked out of `Fire` on its first line. It reads as a reload bug, because
reloading is the other way into it, and it is neither the reload nor the ammo -
it is a cooldown nobody was counting down.

`scriptHeldItems` in `wep/runtime.go` gives those items a lifetime through the
same clock `Powerup.Duration` already uses. It is a knob, and the number is not
invented: 44 tics is the length of `Railgun/Recharge.ogg`, which is what the
player hears while the gun is not ready.

Ten of these weapons give `Pumping` and two take it back themselves, so the
shape is normal in this mod. Only the railgun reads one it never clears.

## Firing is GZDoom's own three rules, and they are all in the source

`p_pspr.cpp` moved to ZScript; the three that decide a weapon's rate of fire
are in `wadsrc/static/zscript/actors/player/player.zs` and
`actors/inventory/stateprovider.zs`.

- **`CheckWeaponFire` only runs while the weapon is ready.** `TickPSprites`
  calls it behind a `WF_WEAPONREADY` check, so `attackdown` - our
  `Runtime.TriggerDown` - is set and cleared *there and nowhere else*. Letting
  the trigger go in the middle of a firing sequence does not arm the next shot;
  a semi-automatic waits for a release it can see from a ready frame.
- **`A_ReFire` goes to Hold, not Fire.** It calls `FireWeapon(null)`, which
  asks `GetAtkState(!!player.refire)`, which is `FindState('Hold')` with
  `Fire` as the fallback. Fire is where a wind-up lives: sending a re-fire
  back through it made the minigun spin up again between every pair of bullets
  and shoot three times a second instead of seventeen. A state the call names
  itself is used as written.
- **A held trigger repeats because the *mod* says so.** `+WEAPON.NOAUTOFIRE`
  only stops the ready frame from re-firing. Brutal Doom marks its automatics
  with it too, and every one of them - the pistol as much as the assault rifle
  - ends its firing states with `A_JumpIfInventory("FiredPrimary",1,"Fire")`,
  which loops for as long as the control script keeps handing that item out.
  So the dual pistols really do empty both magazines on one held trigger, at
  the six tics a pair their script asks for. There is nothing in the data that
  separates a semi-automatic from an automatic here.

## Alt fire is a second button, and half these weapons have one

`CheckWeaponFire` is one chain of else-ifs over two buttons: `BT_ATTACK` while
`WF_WEAPONREADY`, then `BT_ALTATTACK` while `WF_WEAPONREADYALT`, with one
`attackdown` shared between them - so the trigger wins while both are held, and
the flag is only cleared by a frame that sees neither. `A_WeaponReady` sets the
two separately, which is why `wep.Op` carries `AllowFire` *and* `AllowAlt`:
`WRF_NOPRIMARY` is a frame that takes only the second button, and the scoped
railgun rests on forty-two of them.

`FireWeaponAlt` returns without doing anything when the weapon has no `AltFire`
state, so it never falls back on the primary one - a weapon with nothing on the
button ignores it. A repeat goes to `AltHold`, because `A_ReFire` re-enters
whichever attack `Weapon.bAltFire` says is running (`Runtime.FiringAlt` here).

The button is the right mouse button, which used to toggle mouse capture; F2
does that now, and the right button only toggles capture when the mouse is
already free.

## A tag is a name, not a mechanism

Final Doomer's Plutonia shotgun is tagged "Scoped Shotgun" and has a scope
drawn on its sprite. It has no AltFire state, no `A_ZoomFactor`, and there is
no `A_ZoomFactor` anywhere in the mod - `grep` the whole pk3. Nothing triggers
that scope because in Final Doomer nothing ever did; the name is the joke, and
the two weapons in this folder that really do scope come from other mods. Read
the mod before hunting for the key binding.

## Doom's pixel aspect is 1.2, and it is not a matter of taste

`psprPixelAspect` was `320/240`, which is 1.333, so every weapon sprite came
out eleven percent narrower than GZDoom draws it. On a gun that reads as
nothing; on a scope it is unmistakable, because a round hole becomes an egg.
All three scoped weapons here - the Sniper Rifle, the Hunting Rifle and the
Blacktail 51 - were oval.

The number is not a guess. `HUDSprite::GetWeaponRect` has both halves:

    scalex = baseScale.X * (320 / (240 * WidescreenRatio)) * (vw / 320)
    scale  = baseScale.Y * (SCREENHEIGHT * vw) / (SCREENWIDTH * 240)

WidescreenRatio is the view's own width over its height, so both collapse to
`viewHeight/240` times the sprite's base scale, and `DPSprite`'s base scale is
`(1.0, 1.2)`. That is `scaleX = viewHeight/240`, `scaleY = viewHeight/200`, and
1.2 between them whatever shape the window is.

There used to be a *rule* here - a frame wider than the 320-unit weapon space
was called an overlay and drawn in square pixels - and it was a wrong diagnosis
of this bug. Most scope art is authored for Doom's pixels like everything else:
the Sniper Rifle's hole is 209x171 in its own picture, which is 1.22 wide to
tall, which is round once it is squeezed by 1.2. Square pixels made it 1.22 the
other way, and the rule did that to every overlay in the folder to straighten
the one that needed it.

`squarePixelFrames` in `weapon.go` is what is left of it: a list, not a rule,
and the only deviation in the weapon drawing. The Blacktail 51's scope really
is a circle drawn in square pixels - it is an egg in GZDoom too, because the
mod ships it without setting `Weapon.WeaponScaleY`, whose default is 1.2. That
property would not have helped anyway: it is per weapon, and the Blacktail's
*gun* frames are drawn for Doom's pixels like everyone else's, so no single
number is right for both halves of it. Name the art that is wrong, by weapon
and by sprite prefix, and leave the rest alone.

## A scope is on the weapon, not on the camera

`A_ZoomFactor` writes `Weapon.FOVScale`, and the view is `desired *=
abs(ReadyWeapon.FOVScale)` worked out from the weapon in hand every frame. So
the zoom belongs to the weapon: put a scoped weapon away and the view widens on
its own, take it out again and it is still scoped. `Weapon.zoom` here, applied
by `applyWeaponZoom` on every weapon change.

`baseFOV` is the field of view the player plays at and a scope divides it. It
is never read back off the camera - do that and an unzoomed weapon coming into
the hands of a scoped player adopts the scope's own field of view as the one to
go back to, and the view never widens again.

## What decides whether a shot marks the wall

`P_LineAttack` spawns the puff and then, separately, a decal - and only if
something names one. `SpawnShootDecal` reads the shooter's *ready weapon*
first and falls back on the puff:

- the weapon's own `Decal` property wins (Doom's hitscan weapons carry
  `Decal "BulletChip"`, and so do Final Doomer's);
- else the puff's, inherited;
- `Decal "None"` or `+NODECAL` means nothing at all.

ZDoom's own `BulletPuff` carries no Decal, so "the puff decides" is wrong in
both directions. The Blacktail 51's laser pointer is the case that proves it:
it fires `A_FireBullets(0,0,1,0,"RedDot",FBF_NOFLASH|FBF_NORANDOMPUFFZ)` from
its *ready* frame, thirty five times a second, and its RedDot puff says
`Decal "None"` precisely so the wall is not chipped thirty five times a
second. A katana's puff names nothing either, which is why a sword leaves no
bullet holes.

`wep.Weapon.Decal` and `wep.Actor.Decal` carry the name; the engine draws its
one bullet-hole picture for the names that mean a chip in a wall and nothing
for the rest - a scorch, a crack, a sword slash and a blood smear have no
picture here yet, and drawing a bullet hole for them is worse than drawing
nothing.

## A firing function spends its own ammo

`A_FireBullets` and `A_FireCustomMissile` call `DepleteAmmo` themselves -
`flags & FBF_USEAMMO`, which is the default when no flags are written, and
`useammo`, which defaults to true. A weapon whose script never takes its own
ammo is not a weapon with infinite ammo; it is one whose ammo the engine
spends. Without that, Final Doomer's C4 launcher could throw charges for ever
and never reach the state that arms the detonator.

`A_TakeInventory(item, 0)` means take *all* of it, not one.

## An item can be a pulse, and A_Die is not a delete

Two things stood between Final Doomer's C4 charges and going off, and neither
of them was ACS.

- **`Powerup.Duration` is a clock on an item.** `FDHellboundC4Detonate` is a
  Powerup that lasts *two tics*: the launcher gives it to itself, every charge
  in the air sees it through `A_JumpIfInTargetInventory`, and it is gone before
  the next charge is thrown. Kept for ever, the next charge blows up on the way
  out of the barrel. `wep.Item.Duration` carries it and `Runtime.Tick` counts
  it down. A negative duration is written in seconds, which is how a mod says
  "until something takes it".
- **`A_Die` sends the actor to its Death state, and does nothing at all to
  something already dead.** The planted charge's Death state opens with
  `A_Die` and then runs on into the explosion, so the second pass through that
  frame has to fall through rather than jump back. Deleting the actor there -
  which is what "die" reads like - meant the charge vanished silently and
  nothing ever exploded.

## A thing a weapon spawns can talk to the player who fired it

`A_GiveInventory(item, amount, AAPTR_TARGET)` from a missile hands the item to
whoever fired it, and `A_JumpIfInTargetInventory` asks that same actor what it
is carrying. That is a whole mechanism, not a detail: Final Doomer's C4 charge
tells the launcher it exists the moment it spawns, and every charge in the air
watches the player's inventory for the item that means "go off". A
`WorldActor` keeps the runtime of the weapon that spawned it for exactly this.

## DECORATE is tokens, not lines

Two shapes broke a line-at-a-time reader, and both are common:

- `Projectile` is a keyword standing for `+MISSILE +NOGRAVITY +NOBLOCKMAP
  +DROPOFF +ACTIVATEIMPACT +ACTIVATEPCROSS +NOTELEPORT`, not a property. Read
  as one, every missile in the mods kept its gravity and fell at the player's
  feet, and none of them counted as a missile at all.
- `{ +NOEXTREMEDEATH DamageType "Bullet" Decal "BulletChip" }` is three
  statements on one line. Read as one, all three were lost.

And a missile class the mod never defines carries its own flags **and its own
speed**: `FDTNTMissile : Rocket` says nothing about gravity because `Rocket`
says it, and `SuperBFGBall : BFGBall` says nothing about how fast it travels
because BFGBall says 25. Read as nothing, Brutal Doom's BFG ball hung in the
air at the end of the barrel. `stockProjectiles` in `zdwep/compile.go` names
the ZDoom classes and what each one's Speed is.

## A puff and a decal are two different things

`P_LineAttack` spawns the puff *and* then spawns a decal, from
`puffDefaults->DecalGenerator`, and neither is conditional on the other. Drawn
as an either/or - the mod's puff, else the engine's bullet hole - half these
weapons marked nothing at all, because their puff is a spark that fades in
three tics.

`P_SpawnPuff` also picks the state: a puff that hit *no actor* starts in
**Crash** when it has one. Final Doomer's puff answers `Spawn` with a flesh-hit
sound and no picture, and keeps its sparks, its ricochet and its little cloud
of smoke in `Crash` - started in `Spawn` on a wall, every hitscan in that mod
landed invisibly. That is what the Tactical Shotgun was doing.

The decal is lit by the baked light of the block *in front of* the surface it
is stuck to. Sampled at the hit position it lands inside the wall as often as
not, where the flood fill never reached, and the hole is drawn black on black.

## The bullet hole is GZDoom's bullet hole

`assets/base/decals/chip1.png` .. `chip5.png` are ZDoom's own decal graphics,
copied out of `wadsrc/static/graphics`. They are seven pixels across and five
to nine tall, and the grey in them is **alpha, not colour**: `decaldef.txt`
gives every one of BulletChip1..5 `shade "00 00 00"` and `translucent 0.85`, so
a shaded decal is the picture's grey used as coverage and the shade painted
through it. `tools/gendecals` does that conversion, and a shot picks one of the
five at random, which is what `decalgroup BulletChip` means.

**A decal picture keeps its own resolution.** They go into the shared picture
store, `weapons/_sprites`, and are packed at startup into the same atlas as
everything else a weapon throws at a wall - `wep.BulletChipSprites` and
`wep.BloodSplatSprite` name them, and the engine finds them there by name.
`assets/base/sprites_64x64.png` is for 64x64 sprites; a seven pixel chip blown
up sixty-four wide to fit one of its tiles is not a 64x64 sprite, it is a
smudge, and the atlas it was stretched into is not the one it belongs in.

The size follows from the same place. A decal's size in map units is its
picture's own size and the group scales by a half on both axes, so a seven by
nine chip is three and a half by four and a half units of wall - `decalSize`,
which reads the picture's dimensions off the atlas rather than keeping a number
next to it, because five chips are five different shapes. Worth saying out loud
because it is a *sixth* of the blob that was here before. It is a chip. On a
dark wall you will not see it, and that is also what GZDoom does; the floor is
where you can tell.

A decal quad is scaled on both axes separately for the same reason. One number
is right for one of the five and squashes the other four.

## Fog tints a colour; it does not decide coverage

`applyFog` mixed the whole vec4, alpha included, so a fragment far enough away
came out with the *fog's* alpha - which is one. Any translucent sprite at
distance was therefore painted solid: Final Doomer's bullet puff, which fades
to nothing inside a loop that also grows it ten percent a tic, ended up as a
sixty-five voxel opaque grey quad hanging over the sky, and firing a shotgun
turned the screen white. Fog mixes `rgb` and keeps `inColor.a`.

The growth was the other half of it. `A_FadeTo`'s third argument is
`FTF_REMOVE` and every one of these scripts passes it; the migration does not
carry it, so the engine removes anything that fades to zero - see
`WorldActor.fade`. Six hundred invisible puffs were alive at once, holding the
actor budget shut against everything that came after them.

## An instance buffer is read a stride at a time

Decals go into the same instance buffer as sprites, one `QuadInstanceFloats`
run each. `DrawDecals` wrote 23 of the 24 - everything but the baked light -
and that is not a decal with a default: every decal after the first was drawn
from the tail of the one before it, and the draw count, being the float count
divided by the stride, came out one short. No bullet hole had been drawn at all
since sprites learned their baked light. `appendDecalInstance` is one function
now, with a test that counts its floats.

## A sound can be shorter than its own header says

Two of Final Doomer's WAVs - `plutbfg9000/fire1` and `fire2` - declare a data
chunk fifty six bytes longer than the file actually holds. beep's decoder
counts towards the length in the header, so past the end of the file it answers
every read with no samples and "still going", and `Buffer.Append`, which only
stops when a streamer says it is done, spins there for ever. On the main
thread, inside the fire loop: the Quantum Accelerator froze the game solid the
first time it fired, and it read as a crash.

`endWhenSilent` in `audio/player.go` treats a streamer that produced nothing as
finished. It is one wrapper on the way into `Append`, it is not specific to
WAV, and it is the difference between a truncated sound and a hung game. A mod
you migrate tomorrow may ship another one - the scan is four lines of python
over `weapons/**/*.wav` comparing the data chunk's size against the file's.

## Loop goes to a label the mod wrote, not one we invented

`Loop` returns to the top of the state block, which the runtime works out by
scanning back to the nearest label. A label beginning with `@` is not one of
the mod's - it is the landing spot of a `Goto Ready+6`, or the engine's own
stop - and counting it as a state start puts `Loop` in the middle of the block
instead of at the top.

Brutal Doom's MP40 is thirteen zero-tic `A_JumpIfInventory` lines under
`Ready:`, one 1-tic `A_WeaponReady` at the top of them, and a `Loop` at the
bottom; another state does `Goto Ready+6`. With the synthetic label counted,
the loop ran the last nine lines over and over and never reached the only frame
in the block that costs a tic - ten thousand steps an update, `OpSpin`, and the
cursor abandoned wherever it stopped, which has a picture about two thirds of
the time. The gun flickered. `labelStarts` skips `@` now.

## The last lump with a name wins

GZDoom's `FileSystem::CheckNumForName` searches the lump list backwards, so a
later file shadows an earlier one with the same name. That is how a mod
overrides a sound with a better take in an `hd/` folder, and it is how Brutal
Doom means `PLSTA0` to be `sprites/weapons/plasmagun/PLSTA0.png` and not
`sprites/vehicles/bike/PLSTA0.png`.

`AssetIndex` kept the *first*, so the plasma gun's firing frames were a 43x24
bike part carrying a world sprite's hotspot - drawn off the left edge of the
screen, which looked exactly like the gun vanishing when you pulled the
trigger. It is a narrow rule: 44 of Brutal Doom's 11971 sprite lumps are named
twice, and 3 of its 750 sounds.

## A_ReFire repeats the attack that is running

`A_ReFire` with no state of its own does not mean "go to Fire". It re-enters
whichever attack is running - `A_ReFire` calls `FireWeaponAlt` when
`Weapon.bAltFire` is set and `FireWeapon` when it is not, and each asks its own
`GetAtkState` for **Hold** or **AltHold**. Compiling the default to the literal
`"Fire"` named the primary's state on an alt attack too: Brutal Doom's hand
grenade, whose `AltHold` is two tics and an `A_ReFire`, walked out of the alt
chain into `Fire` and pulled the pin again, for ever. The op carries no target
at all now and the runtime picks.

Holding alt fire on that grenade rests on `AltHold`, which is `TNT1` - the
weapon layer shows nothing while the pin is out. That is the mod, not us.

## Only a missile is launched with its Speed

`P_SpawnMissile` works a velocity out from `Speed` along the line of fire.
`A_SpawnItem` and `A_SpawnItemEx` call none of that: they put an actor
somewhere and leave it, and any velocity it has is the one written in the call.
Handing every spawn its Speed set Brutal Doom's `BigBlackSmoke` drifting - it
is `Speed 1` with `+MISSILE` on it - so `launchVelocity` reads Speed only for
`OpFireProjectile`.

The smoke that *is* launched by `A_CustomMissile` genuinely moves, and that is
the mod: `A_CustomMissile("BigBlackSmoke", 40, 0, random(0,360), 2,
random(40,160))` is aimmode 2, which spawns at Speed with no vertical velocity
and then applies the pitch.

## A pitch aims down, and a random angle is a range

`Op.AimPitch` carries the pitch a missile-spawning call fires at, in ZDoom's
sign, where **positive is down**. The two callers mean different things by it
and ZDoom implements them differently, so `pitchedBy` takes a flag:

- `A_FireCustomMissile` does `self.Pitch += pitch` around the spawn, so its
  pitch is an *adjustment* to where the player is already looking.
- `A_CustomMissile` under `CMF_ABSOLUTEPITCH` - which includes aimmode 2,
  because the C++ sets that flag itself after spawning - throws the aim away:
  `missilespeed = |cos(pitch)| * Speed` across, `Vel.Z = -sin(pitch) * Speed`
  up. The absolute value is ZDoom's own and it matters: Brutal Doom's smoke is
  fired at `random(40, 160)`, and past ninety a plain cosine sends it out
  backwards. In every other aimmode ZDoom ignores the argument, so the
  migration drops it rather than aiming things ZDoom does not.

The angle beside it has to be a **range**, not a number. `Op.Angle` was
`Range.Value()`, the middle of the span, so `random(0, 360)` came out as 180
and every puff in a cloud left on the same bearing - straight back at the
player, in the flamethrower's case. `Angle`/`AngleTo` now, rolled once per
spawn by `rollRange`, the same way the offsets and velocities already were.

A hand-built `wep.Op` has to write both halves of a range: a constant compiles
with `To` equal to itself, and `Angle: -90` with `AngleTo` left at zero is the
range -90..0, not the number -90.

**Re-run `genweapons -prune` after a migration that changes what actors
compile to.** The shared actor files are named by the hash of their contents,
so an old one stays on disk until something deletes it - and reading
`weapons/_actors/*.json` to check what the compiler did will show you the
stale copy right next to the new one.

## A thing on the ground stops turning to face you

Every world sprite here is an XY billboard: it follows the camera's pitch as
well as its yaw. That is what `+FORCEXYBILLBOARD` asks for and it is what keeps
a puff of smoke from going edge-on when you look down at it.

A shell casing that has landed is the one case where it reads as a defect. The
quad ends up lying almost flat on the floor, so turning on the spot sweeps the
shell round the floor with you, and a thing on the ground visibly spinning is
the first thing anybody notices. `move` drops a resting actor to a plain Y
billboard - GZDoom's own default (`gl_billboard_mode 0`): upright, facing you,
never pitching.

Note what this is **not**. Its animation had already stopped: landing sends a
casing to its Death state, and `DEBUG_ACTORS=1` shows the frame and the
position both frozen there. The tumble ending and the sprite still turning are
two different things, and only the second one was left.

## A velocity can name the thing that threw it

`A_SpawnItemEx("FDGunSmoke", 0,0,0, velx, vely+frandom(-0.2,0.2), velz, 0,
SXF_NOCHECKPOSITION | SXF_ORIGINATOR | SXF_ABSOLUTEVELOCITY)` is how Final
Doomer hands a puff of gun smoke the throw of the spawner it came out of.
Folded to a number at migration time, `velx` is zero, and so is every other
one: the whole compiled op said "no velocity", so every puff hung in the muzzle
instead of leaving it. `Op.VelExprs` keeps all three as programs, the way
`ArgExprs` already does for `A_SetScale`, and they are read off the spawner's
own published `velx`/`vely`/`velz` when the spawn happens.

The two halves have to land together. `SXF_ABSOLUTEVELOCITY` says the three
numbers are Doom's own world axes, not forward/sideways/up from the spawner's
facing - and `velx`/`vely` read off another actor are already in world axes. Fix
the expressions without the flag and the smoke leaves sideways for every
heading but the one the test map happens to face; `Op.AbsoluteVelocity` is what
stops that. `publishState` writes Doom x to world X, Doom y to world Z and Doom
z to world Y, so reading them back is the same mapping inverted, and a spawn
inherits exactly the throw it was given.

## A hotspot can be a lie

`grAb` says where a picture is anchored, and GZDoom believes it. So does this,
with one bound: an offset further out than Doom's whole 320x200 screen is not
an anchor anybody authored. Brutal Doom ships exactly one - `TRACA5` says
`(27, -1232)` on a 32x32 tracer - and that drew every `DecorativeTracer` about
three voxels away from the shot that made it, eight of them on every shot of
the assault shotgun.

Nobody ever saw it in GZDoom, because the same mod replaces that actor's sprite
with an MD3 model and the lump is never rendered. That is the shape of this
whole class of bug: a mod's broken data stays invisible until an engine reads
the part of it GZDoom skips.

The fallback is zero, not a guess at the middle, for the reason
`OffsetsFromPNG` already gives: for a weapon frame the hotspot is subtracted
from the middle of the 320x200 space rather than used as a pivot, and guessing
`(width/2, height)` throws a full-canvas frame most of a screen away.

Two atlases carry these and both are cached against what they were packed from.
`weapons/<name>/sprites.png` is rebuilt by `genweapons`; **`weapons/_sprites.png`
is not** - it is rebuilt by the game at startup, and only when the store's names
or sizes change. A fix to how an offset is *read* changes neither, so delete
`weapons/_sprites.png` and run the game once, or the old numbers stay.

## Two weapons with one name is the mod's doing

Brutal Doom's KEYCONF puts `AssaultShotgun2` and `AssaultShotgun` on slot 3,
`ClassicMP40` and `MP40` on slot 2, `ClassicPlasmaRifle` and `Plasma_Gun` on
slot 6 - separate actors that share a `Tag`. The `2` ones are the mod's own
variants (`AssaultShotgun2` is the slug-loaded shotgun, reached through the
ChangeAmmo state, which gives it and selects it); the `Classic` ones belong to
the mod's classic mode. A folder per actor is right; they read as duplicates
only because the list shows the tag.

## Only the weapon in hand is loaded

A migrated mod is 134 weapons: half a gigabyte of atlas and a quarter of a
gigabyte of sound, which decodes to several times that. Held onto, a player who
cycles through the folder ends up carrying every one of them - the heap grew to
1.4 GB and every weapon's sprite batch stayed in the 2D pass being begun, ended
and drawn every frame.

- Pictures: `keepLoaded` keeps the last few weapons and gives the rest back
  (`Weapon.unload`, `GlApplication.DropSpriteBatch`). Loading one costs about
  fifty milliseconds, so a handful are kept for swapping between two guns.
- Sounds: `Unequip` unloads the cues, and `LoadCueTree` now only *indexes* the
  folder - a cue is decoded the first time it is played. Decoding a weapon's
  whole folder on equip cost a third of a second of silence per switch, and
  most of it is never played. It also means two weapons whose folders hold a
  sound of the same name no longer have to be the same sound.

`-cycle 0.2 -autofire` walks the whole folder while firing; watch RSS.

## A tic is a slice of a second, not a frame

`Cursor.Update` is given the elapsed seconds and spends them against the current
frame's duration. Two things make that come out right:

- **The overshoot is carried.** A frame ends part way through an update; if the
  remainder is dropped, every frame lasts up to one extra frame of wall time and
  a weapon's rate of fire becomes a function of the machine's frame rate. The
  same pistol fired 13.5 times a second at 60fps and 16 at 240.
- **Time is only charged to a frame the cursor is sitting on.** Charging before
  the first one has been entered banks a debt against a frame that does not
  exist, and the machine starts a step further along than it should.

`-shot-fps` is how to check it: run the same weapon at 30 and at 240 for the
same number of *simulated* seconds and count with `DEBUG_WEAPON`.

## Zero-tic loops are how a reload is written

`maxStepsPerUpdate` is ZDoom's ten thousand and has to be. The dual MP40 refills
its magazine one round at a time through a ten step zero-tic loop it runs sixty
four times, all inside one tic. At five hundred the cursor gave up a fifth of
the way through.

And giving up no longer stops the weapon. A cursor that stops has no frame to
draw, so the gun goes invisible and can never fire again - which reads exactly
like a missing frame and is nothing of the sort. It now abandons the update,
keeps its place, says so once, and tries again next frame: a weapon that
stutters is one somebody can still see and still put down.

## Keep the unused packages

`dungen/`, `audio/`, `geometry/` (including `jps.go` and `cc.go`) and
`quads/glhf/texture3d.go` currently have no callers in the main loop. **Do not
delete them.** They are kept deliberately - procedural generation, sound and the
3D texture path are all expected to be wired up later.

An audit that flags them as dead code is correct about the fact and wrong about
the conclusion. Leave them as they are.

## Run gofmt

The tree is gofmt'd and CI fails if it is not (`.github/workflows/test.yml`).
It used to be space-indented, so every edit came back tab-indented and buried
the real change in a whole-file diff. `gofmt -w .` before committing.
`.git-blame-ignore-revs` holds the reformat commit.
