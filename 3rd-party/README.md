# 3rd-party

Every game and mod `re-migrate-all.sh` reads, in one place so that the script
needs no paths of its own. **None of it is ours and none of it is checked in** -
`/3rd-party/` is in `.gitignore`. Two of these are commercial games and the rest
are community releases under their own terms; a checkout of this repo has the
generated `assets/` and not the sources they came from.

| folder | what it is | where it came from |
| --- | --- | --- |
| `doom2.wad` | Doom II | goes last in every mod's source list - the mods take its sprites and sounds for granted |
| `bdce/` | Brutal Doom Community Expansion, 43 weapons, `bd_` | a clone of <https://github.com/BLOODWOLF333/Brutal-Doom-Community-Expansion> at `02623c7`, recorded as revision `master` |
| `FinalDoomer_v3.4.pk3` | Final Doomer v3.4, 80 weapons, `fd_` | the release pk3 |
| `mods/` | nine single-weapon community mods, 10 weapons, no prefix | Realm667 Armory releases, unpacked; the zips they came in are in `mods/archive/`, each carrying its own `CREDITS` or `readme.txt` naming its author |
| `blake-stone/cd/` | Blake Stone: Aliens of Gold, 5 weapons and all its art and sound | the CD's own files. `VSWAP.BS6` is the only one anything reads today; the rest of the `.BS6` set - maps, audio, the VGA graphics - is here so a later importer does not need the disc again |
| `system-shock/SSHOCK/` | System Shock (1994), 16 weapons and everything else the two importers file | the game's install directory. `SSHOCK/DATA` is what is read |

## The engines that drew this art originally

An importer is a guess about somebody else's file format until you can read the
code that wrote it. These are those engines, checked out here so a wrong colour,
a wrong offset or a sound at the wrong pitch can be traced to the line that
decided it rather than argued about.

| folder | what it is | what has already been read out of it |
| --- | --- | --- |
| `gzdoom/` | <https://github.com/ZDoom/gzdoom>, shallow clone | where a weapon lands on screen (`src/rendering/hwrenderer/scene/hw_weapon.cpp`, `HUDSprite::GetWeaponRect`), what the psprite layer does (`src/playsim/p_pspr.cpp`), and what an action function means - `A_ReFire` and `GetAtkState` are ZScript now, in `wadsrc/static/zscript/actors/inventory/`, which is what says a held trigger re-enters `Hold` and a released one falls through to the swing |
| `blake-stone/bstone/` | <https://github.com/bibendovsky/bstone>, shallow clone | the source port, and the readable one. `tools/importbs/names.go` and `statics.go` are its `initialize_sprites()` and `statinfo` tables; `palette.go` is its `vgapal.cpp`, because the palette was linked into the executable and is in none of the data files; `sounds.go` is its `digitized_map` |
| `blake-stone/blake-stone-source/` | <https://github.com/johndrinkwater/blake-stone>, shallow clone | the game's own 1993 source, which is where bstone's tables came from in the first place - `3D_ACT1.C` holds the real `statinfo`. Read this when bstone and the data disagree |
| `system-shock/shockmac/` | <https://github.com/NightDive-Studio/shockmac>, the 1994 source release, shallow clone | `Game Src/WEAPONS.C` for which noise a gun makes (`weapon_combat_fxs`), how fast it fires (`player_fire_weapon` and `AUTOFIRE_SPEED`) and what the energy weapons spend; `Game Src/RENDTOOL.C` for how a model finds its texture (`load_model_vtexts`); `Game Src/GAMEREND.C` for the three turquoise lines a beam weapon draws |

**The System Shock release has classic Mac line endings**, one carriage return
and no newline, so `grep` decides its files are binary and tells you nothing at
all rather than telling you it found nothing. That is a silent wrong answer, not
an error. Two ways round it:

```bash
grep -ra "weapon_combat_fxs" 3rd-party/system-shock/shockmac      # -a: it is text
tr '\r' '\n' < "3rd-party/system-shock/shockmac/ShockMac/Game Src/WEAPONS.C" |
    grep -n "weapon_combat_fxs"                                  # ... with line numbers
```

Two more references are not folders here. `inkyblackness/hacked` reads System
Shock's container format and its compression, and is a Go dependency of this
module rather than a checkout - `go doc` it. The DECORATE language `zdwep`
compiles is documented at <https://zdoom.org/wiki/DECORATE>, and the wiki page
for an action function is what `zdwep/policy.go` has to agree with.

## The archives the extracted forms came out of

Kept beside each game so the extraction is reproducible rather than remembered.

- `blake-stone/archive/` - the CD image as it was downloaded (`.bin`/`.cue`,
  MODE1/2352) and the zip holding it. The ISO is every sector's bytes 16 to
  2064; `hdiutil attach` mounts what comes out, and `cd/` is a copy of that.
- `system-shock/archive/System_Shock_1994.zip` - `SystemSh/SSHOCK` inside it is
  what was moved out. The zip also holds `SSP` and a `CD` directory, which
  nothing here reads.
- `mods/archive/` - the ten zips. `MagnumRevolver.pk3` is unpacked with the
  others and migrates to nothing: it is written entirely in ZScript, a far
  larger language than DECORATE, and ships no DECORATE lump to read instead.

## What is not here

`assets/smallgun_sheet.png` is in the repo, not in this folder - it was never a
mod, just a grid sprite sheet, and `tools/gensheet` turns it into the same
`weapon.json` the migration writes. The script runs it alongside the rest.

`go.mod` here is ours and is checked in. It builds nothing; it stops the parent
module's `./...` at this directory, because gzdoom vendors a Go file of its own
and without it `go list`, `go vet` and `go test` all report a package of
somebody else's as one of ours.
