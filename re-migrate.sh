#!/usr/bin/env bash
#
# Rebuild the generated asset folders from the games and mods in 3rd-party/.
#
#   ./re-migrate.sh all             everything below, then a prune and a check
#   ./re-migrate.sh blake-stone     assets/sprites/bs_*, assets/textures/blake
#   ./re-migrate.sh system-shock    assets/sprites/ss_*, assets/textures/shock,
#                                   assets/meshes/ss_*
#   ./re-migrate.sh brutal-doom     the 43 bd_ weapons
#   ./re-migrate.sh final-doomer    the 80 fd_ weapons
#   ./re-migrate.sh mods            the nine single-weapon community mods
#   ./re-migrate.sh small-gun       the one weapon built from a sheet in this repo
#
# There is nothing to point at: the sources were gathered into 3rd-party/ once
# (see 3rd-party/README.md) so that a re-migration is this script and nothing
# else. Every tool here overwrites what it wrote last time rather than merging,
# and the shared picture and actor stores under assets/weapons/ are keyed by the
# hash of their own contents, so running the same target twice over the same
# sources produces the same bytes.
#
# "all" does the two DOS games first, because they write weapons of their own
# into the same folder the Doom migration then adds to, and prunes the shared
# stores at the very end, once everything that points into them exists.
set -euo pipefail
cd "$(dirname "$0")"

third=3rd-party
doom2=$third/doom2.wad
mods=$third/mods

usage() {
    sed -n '3,12p' "$0" | sed 's/^# \{0,1\}//'
}

target=${1:-}
case "$target" in
    all | blake-stone | system-shock | brutal-doom | final-doomer | mods | small-gun) ;;
    *)
        [ -n "$target" ] && echo "unknown target: $target" >&2
        usage >&2
        exit 1
        ;;
esac

# want is whether this run covers a target; need refuses to start one whose
# source is not where 3rd-party/README.md says it is, rather than letting a tool
# fail halfway through writing a folder.
want() { [ "$target" = all ] || [ "$target" = "$1" ]; }
need() {
    for path in "$@"; do
        if [ ! -e "$path" ]; then
            echo "missing: $path" >&2
            echo "see 3rd-party/README.md for what it is and where it came from" >&2
            exit 1
        fi
    done
}
step() { echo; echo "=== $* ==="; }

if want blake-stone; then
    step "Blake Stone: Aliens of Gold"
    need "$third/blake-stone/cd/VSWAP.BS6"
    go run ./tools/importbs -vswap "$third/blake-stone/cd/VSWAP.BS6" -install
fi

if want system-shock; then
    step "System Shock (1994)"
    need "$third/system-shock/SSHOCK/DATA"
    go run ./tools/importss -data "$third/system-shock/SSHOCK/DATA" -install
fi

# A source list is searched in order and doom2.wad goes last in every one of
# them: the mods take the base game's sprites and sounds for granted.
if want brutal-doom; then
    step "Brutal Doom Community Expansion"
    need "$third/bdce" "$doom2"
    # -all compiles leniently and reports what did not make it, which is what a
    # mod this size wants: plenty of its actors are not weapons at all. The
    # revision recorded in every bd_ weapon.json is the branch rather than the
    # commit, because that is what the folder already says; which commit the
    # clone is at is in 3rd-party/README.md.
    go run ./tools/genweapons -source "$third/bdce,$doom2" -all -prefix bd_ -revision master
fi

if want final-doomer; then
    step "Final Doomer v3.4"
    need "$third/FinalDoomer_v3.4.pk3" "$doom2"
    go run ./tools/genweapons -source "$third/FinalDoomer_v3.4.pk3,$doom2" -all -prefix fd_ -revision v3.4
    # The eightieth. Final Doomer's JPCP fist is two weapons in one actor: it
    # becomes an energy katana once the player has the chainsaw, and which one
    # you are holding is an inventory flag only the mod's own scripts set, so a
    # plain sweep only ever reaches the first of them. Forcing the flag on and
    # then dropping everything that leaves unreachable pulls the second out as a
    # weapon of its own, under a name of its own.
    go run ./tools/genweapons -source "$third/FinalDoomer_v3.4.pk3,$doom2" -prefix fd_ -revision v3.4 \
        -force-item FDGotChainsaw -drop-unreachable \
        -rename-id katana_energy -rename-tag "Energy Katana" FDJPCPFist
fi

if want mods; then
    step "the single-weapon community mods"
    need "$mods" "$doom2"
    # Each of these is one Realm667 Armory release holding one or two weapons,
    # so each is named rather than swept with -all. MagnumRevolver.pk3 is in the
    # folder and is not here: it is written entirely in ZScript, which this
    # migration does not read, and it ships no DECORATE lump to read instead.
    migrate_one() {
        local file=$1
        shift
        go run ./tools/genweapons -source "$mods/$file,$doom2" "$@"
    }
    migrate_one "Axe.wad" Axe
    migrate_one "BTConnector.pk3" BTConnector
    migrate_one "Colt45.pk3" Colt45
    migrate_one "CombatKnife.pk3" CombatKnife
    migrate_one "DualBerettas.pk3" Dual92
    migrate_one "Hunting Rifle.wad" HuntingRifle HuntingRifleScope
    migrate_one "Reaper.wad" Reaper
    migrate_one "SilencedPistol.pk3" SilencedPistol
    migrate_one "UACSniperRifle.wad" UACSniperRifle
fi

if want small-gun; then
    step "small_gun"
    # Not a mod at all: a plain grid sprite sheet in this repo, put through the
    # same weapon.json writer so it comes out a weapon like any other.
    go run ./tools/gensheet -sheet assets/smallgun_sheet.png -id small_gun \
        -tag "Small Gun" -prefix SMGN -size 64x64 -frames 0,1,2,3,4 -scale 3
fi

step "pruning the shared stores"
# Safe after any target - it keeps whatever the weapons in the folder still
# point at - but only a full run can shrink the stores to nothing but that.
go run ./tools/genweapons -prune

step "checking every weapon still fires"
go run ./tools/firecheck || true

echo
echo "done. 'git status assets' says what moved."
