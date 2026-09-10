package main

// The cheat menu: the things worth having while a level is being built and
// nobody wants to earn them first. It is a page of the F1 menu rather than a
// key or a console, because there is no console and the keys are all spoken
// for - see EditorMode.

import (
	"fmt"
	"slices"

	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/wep"
)

// godMode makes Player.TakeDamage do nothing. It is checked there rather than
// at every place damage comes from, so a new source of damage is covered by
// being written at all.
type cheats struct {
	godMode bool
}

// cheatOptions is the menu page. Everything on it is one action, and every
// action closes the menu, because a cheat is a thing you use and then get back
// to what you were doing.
func (g *Game) cheatOptions() []quads.GuiOption {
	return []quads.GuiOption{
		quads.NewGuiOptionAction("Give All Weapons", func() {
			g.closeAllMenus()
			g.giveAllWeapons()
		}),
		quads.NewGuiOptionAction("Fill All Ammo", func() {
			g.closeAllMenus()
			g.fillAllAmmo()
		}),
		quads.NewGuiOptionAction("Full Health", func() {
			g.closeAllMenus()
			g.player.Heal(playerMaxHealth)
			g.Toast("Health restored")
		}),
		quads.NewGuiOptionBool("God Mode", &g.cheats.godMode),
		quads.NewGuiOptionAction("Kill Every Enemy", func() {
			g.closeAllMenus()
			g.killEveryEnemy()
		}),
	}
}

// giveAllWeapons hands over the whole folder, loads the sounds of each - which
// is what give() does - and puts the first one in hand if the player is empty
// handed, so the menu closing does not leave them holding nothing.
func (g *Game) giveAllWeapons() {
	given := 0
	for _, weapon := range g.weapons {
		if weapon.owned {
			continue
		}
		weapon.give()
		given++
	}
	if g.currentWeapon() == nil && len(g.weapons) > 0 {
		g.selectWeapon(0)
	}
	g.Toast(fmt.Sprintf("Gave %d weapon(s) - %d in all", given, len(g.weapons)))
}

// fillAllAmmo tops up every ammo pool every owned weapon knows about, to
// whatever that weapon says its own cap is. A weapon that spends nothing has no
// pools and is skipped by having none.
//
// Its ammo, and not its whole inventory. A migrated weapon's Items are mostly
// not ammo at all: they are the flags its own script signals itself with -
// Brutal Doom's assault rifle declares a hundred of them, FiredPrimary and
// Reloading and Kicking among them, and Final Doomer's Zaiger rifle has a
// FDLevelBeginAnimation its ready state checks for to play a reload once at
// spawn. Filling those to their cap left the rifle firing without a trigger and
// the Zaiger stuck in the reload it was told it had just started, for ever,
// because nothing ever takes a flag nobody set.
func (g *Game) fillAllAmmo() {
	filled := 0
	for _, weapon := range g.weapons {
		if !weapon.owned || weapon.runtime == nil {
			continue
		}
		for _, item := range ammoPools(weapon.Data) {
			cap, known := weapon.runtime.Caps[item]
			if !known || cap <= 0 {
				continue
			}
			weapon.runtime.Inventory.Give(weapon.runtime.Caps, item, cap)
			filled++
		}
	}
	g.Toast(fmt.Sprintf("Filled %d ammo pool(s)", filled))
}

// ammoPools is what a weapon actually spends: the pool it fires from, the
// second pool beside it, and the magazine a reload fills. Every one of them is
// named by the weapon itself - see wep.Ammo - which is what tells them apart
// from the rest of its inventory.
func ammoPools(weapon *wep.Weapon) []string {
	if weapon == nil {
		return nil
	}
	var pools []string
	for _, item := range []string{weapon.Ammo.Type, weapon.Ammo.Type2, weapon.Ammo.Clip, weapon.AmmoPickup} {
		if item == "" || slices.Contains(pools, item) {
			continue
		}
		pools = append(pools, item)
	}
	return pools
}

// killEveryEnemy drops everything hostile still standing, through the same
// death every shot goes through - so the bodies, the cries and the colliders
// all behave the way they do when it is earned.
func (g *Game) killEveryEnemy() {
	killed := 0
	for _, actor := range g.actors {
		if actor.dying || !actor.hostile || !actor.object.IsAlive() {
			continue
		}
		actor.die()
		killed++
	}
	g.Toast(fmt.Sprintf("Killed %d", killed))
}
