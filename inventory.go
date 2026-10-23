package main

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads"
)

// What the player is carrying, and what the HUD says about the one in hand.
// Both read the same two things off a weapon - its icon in the shared atlas
// and its ammo - so they live together.

// openInventory puts the two pane overlay up. It is built fresh each time
// rather than kept, because what is in it is what has been picked up since the
// last time it was open.
func (g *Game) openInventory() {
	if g.sharedAtlas == nil {
		return
	}
	previousMouseState := g.setupModal()
	inventory := quads.NewGuiInventory(g.UiContext(), g.sharedAtlas)
	inventory.SetItems(g.carriedItems())
	inventory.SetOnClose(func() {
		g.captureMouse(previousMouseState)
		g.popFromWidgetStack()
	})
	guiWidth, guiHeight := g.GuiSize()
	inventory.HandleResize(guiWidth, guiHeight, g.UIScale)
	g.pushToWidgetStack(inventory)
}

// carried is which weapons in the folder the player has picked up, by their
// index in it, in the order the folder lists them.
func (g *Game) carried() []int {
	var indices []int
	for index, weapon := range g.weapons {
		if weapon.owned {
			indices = append(indices, index)
		}
	}
	return indices
}

// carriedItems is those weapons as rows for the inventory: the name in the
// list, the page on the right, and the picture that goes with it.
func (g *Game) carriedItems() []quads.InventoryItem {
	var items []quads.InventoryItem
	for _, index := range g.carried() {
		weapon := g.weapons[index]
		items = append(items, quads.InventoryItem{
			Name:   weapon.DisplayName(),
			Detail: g.weaponDetail(weapon),
			Icon:   g.iconIndex(weapon),
			Tab:    inventoryItemsTab,
			Use:    func() { g.selectWeapon(index) },
		})
	}
	// The notes go after the weapons on the same page, which is where they have
	// always been - picking one only turns to its writing, which is the whole of
	// what reading a note again is. The recordings get a page of their own,
	// because what is done with one of those is listen to it.
	items = append(items, g.noteItems()...)
	return append(items, g.audioLogItems()...)
}

// weaponDetail is the right hand page: what this weapon is, what it spends and
// whether it reloads. Aligned by padding rather than by a table layout,
// because it is five lines.
func (g *Game) weaponDetail(weapon *Weapon) []string {
	ammo := weapon.AmmoLine()
	if ammo == "" {
		ammo = "none"
	}
	reload := "no"
	if weapon.Data.Reloadable {
		reload = "yes"
	}
	held := ""
	if weapon == g.currentWeapon() {
		held = " (in hand)"
	}
	return []string{
		"id      " + weapon.Data.ID + held,
		"ammo    " + ammo,
		"clip    " + orNone(weapon.Data.Ammo.Clip),
		"spends  " + orNone(weapon.Data.Ammo.Type),
		"reload  " + reload,
		"from    " + weapon.Data.Provenance.Actor,
	}
}

func orNone(text string) string {
	if text == "" {
		return "none"
	}
	return text
}

// iconIndex is where a weapon's picture is in the shared atlas, or -1 for a
// weapon whose mod shipped none.
func (g *Game) iconIndex(weapon *Weapon) int {
	if index, known := g.sharedSpriteIndex[weapon.Data.Icon]; known {
		return index
	}
	return -1
}

// How the HUD sits in the bottom left corner, in GUI coordinates.
const (
	hudMargin  = 10
	hudIconBox = 48
)

// drawWeaponHUD says what is in the player's hands: the icon the pickup was
// drawn with, its name, and how loaded it is. Nothing at all when they are
// empty handed, which is how a run starts.
func (g *Game) drawWeaponHUD() {
	weapon := g.currentWeapon()
	if weapon == nil || g.hudName == nil {
		return
	}
	_, guiHeight := g.GuiSize()
	bottom := float32(guiHeight - hudMargin)

	textLeft := float32(hudMargin)
	if index := g.iconIndex(weapon); index >= 0 {
		g.drawHudIcon(index, mgl32.Vec2{hudMargin, bottom - hudIconBox})
		textLeft += hudIconBox + hudMargin
	}

	_, lineHeight := g.hudName.Size()
	g.hudName.SetText(weapon.DisplayName())
	g.hudName.SetPosition(mgl32.Vec2{textLeft, bottom - float32(2*lineHeight)})
	g.hudName.Draw()

	ammo := weapon.AmmoLine()
	if ammo == "" {
		ammo = "--"
	}
	g.hudAmmo.SetText(ammo)
	g.hudAmmo.SetPosition(mgl32.Vec2{textLeft, bottom - float32(lineHeight)})
	g.hudAmmo.Draw()
}

// drawHudIcon paints the weapon's picture into the HUD's corner box.
func (g *Game) drawHudIcon(index int, at mgl32.Vec2) {
	quads.DrawIconInBox(g.sharedAtlas, index, at, hudIconBox)
}

// drawHealthHUD says what is left of the player, in the corner opposite the
// weapon. It is drawn whether or not anything is in hand: being empty handed is
// not being unhurt.
func (g *Game) drawHealthHUD() {
	if g.hudHealth == nil || g.player == nil {
		return
	}
	guiWidth, guiHeight := g.GuiSize()
	g.hudHealth.SetText(fmt.Sprintf("HEALTH %d", g.player.Health()))
	width, lineHeight := g.hudHealth.Size()
	g.hudHealth.SetPosition(mgl32.Vec2{
		float32(guiWidth - hudMargin - width),
		float32(guiHeight-hudMargin) - float32(lineHeight),
	})
	g.hudHealth.Draw()
}
