package main

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
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
	inventory.SetOnSelect(func(index int) {
		carried := g.carried()
		if index < len(carried) {
			g.selectWeapon(carried[index])
		}
	})
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
		})
	}
	return items
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

// drawEditorHUD is drawWeaponHUD's editor-mode counterpart: no weapon is
// held, so the same two lines say what a click does instead - ADD or DEL, and
// which of the five shapes it applies to - plus whether the player is flying
// or walking to get there.
func (g *Game) drawEditorHUD() {
	if g.hudName == nil {
		return
	}
	_, guiHeight := g.GuiSize()
	bottom := float32(guiHeight - hudMargin)
	textLeft := float32(hudMargin)

	action := map[bool]string{true: "ADD", false: "DEL"}[g.editor.addMode]

	_, lineHeight := g.hudName.Size()
	g.hudName.SetText(action + " - " + g.editor.shapeMethod.String())
	g.hudName.SetPosition(mgl32.Vec2{textLeft, bottom - float32(2*lineHeight)})
	g.hudName.Draw()

	movement := map[bool]string{true: "Walking", false: "Flying"}[g.gravitationEnabled]
	g.hudAmmo.SetText(movement)
	g.hudAmmo.SetPosition(mgl32.Vec2{textLeft, bottom - float32(lineHeight)})
	g.hudAmmo.Draw()
}

// drawHudIcon paints the weapon's picture into the HUD's corner box.
func (g *Game) drawHudIcon(index int, at mgl32.Vec2) {
	quads.DrawIconInBox(g.sharedAtlas, index, at, hudIconBox)
}
