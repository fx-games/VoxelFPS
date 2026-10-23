package main

// The editor's asset browsers. Every placement mode has one, they all look the
// same, and one key opens whichever belongs to the mode you are in - see
// ActionEditorOpenPicker.
//
// They exist because the palettes stopped being lists a person can walk. Two DOS
// games' worth of art is ninety models, six hundred sprites and a hundred and
// fifty guns; a wheel that steps through them one at a time is a wheel nobody
// reaches the end of, and a menu of six hundred names is not better. So the
// folders the art is sorted into are the interface: a grid of pictures, a folder
// to a category, the same widget the textures are picked out of.

import (
	"encoding/json"
	"image"
	"log"
	"os"
	"path/filepath"

	"github.com/memmaker/voxelfps/eassets"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/wep"
)

// The three roots the sprite tools browse, one per role - see spriteRole.
var spriteRoleDir = map[string]string{
	propsRole:  filepath.Join(spriteDir, propsRole),
	itemsRole:  filepath.Join(spriteDir, itemsRole),
	actorsRole: filepath.Join(spriteDir, actorsRole),
}

// weaponDir is the folder of folders the weapons come out of, one to a gun.
const weaponDir = "assets/weapons"

// openModePicker opens the browser for whatever the editor is placing right now.
// It is one key in every mode rather than one key per tool, because "show me what
// I can put here" is the same question in all of them.
func (g *Game) openModePicker() {
	g.openModePickerAt("")
}

// openModePickerAt is the same with a folder to start in, which only -pick gives.
func (g *Game) openModePickerAt(startDir string) {
	switch g.editor.mode {
	case ModeBlocks:
		// Building picks a block off the list; the paint and fill tools pick a
		// texture, and the library already has a browser of its own.
		if g.editor.tool != EditorToolPaint && g.editor.tool != EditorToolFill {
			g.openBlockPicker()
			return
		}
		g.pickTexture(g.MapTextures, func(selectedIndex int) {
			g.editor.currentPaintGunTextureIndex = selectedIndex
		})
	case ModeProps3D:
		g.openAssetPicker(startDir, meshDir, hasScenery, g.meshTile, g.selectPropByID)
	case ModeProps2D:
		g.openAssetPicker(startDir, spriteRoleDir[propsRole], hasSpriteKind, spriteTile, func(id string) {
			g.selectPropByID(propsRole + "/" + id)
		})
	case ModePickups:
		g.openAssetPicker(startDir, spriteRoleDir[itemsRole], hasSpriteKind, spriteTile, func(id string) {
			g.selectPropByID(itemsRole + "/" + id)
		})
	case ModeWeapons:
		g.openAssetPicker(startDir, weaponDir, hasWeapon, weaponTile, func(id string) {
			g.selectWeaponPickup(id, false)
		})
	case ModeActors:
		g.openAssetPicker(startDir, spriteRoleDir[actorsRole], hasSpriteKind, spriteTile, func(id string) {
			g.selectActorByID(actorsRole + "/" + id)
		})
	}
}

// openAssetPicker is the browser itself: it puts the widget up, and hands the
// folder that was clicked to chosen. The path handed over is relative to root,
// with forward slashes, which is the tail of what every one of these things is
// named by - see the callers, which put the role back on the front.
//
// startDir is the folder to open on, empty for wherever this root was left last.
// Only -pick ever gives one: a capture takes no keystrokes, so a page of the
// browser deeper than its first is a page no screenshot could otherwise reach.
func (g *Game) openAssetPicker(startDir, root string, isAsset func(dir string) bool, tile func(dir string, w, h int) *image.RGBA, chosen func(id string)) {
	previousMouseState := g.setupModal()
	g.widgetsAreBlockingMovement = true
	picker, browser := g.OpenAssetBrowser(root, startDir, isAsset, tile)
	picker.SetOnIndexClicked(func(selection int) {
		entry, isCell := browser.Entry(selection)
		// Close, not just pop: the browser has a folder of pictures uploaded and
		// drawn, and popping the widget on its own leaves that behind.
		picker.Close()
		g.popFromWidgetStack()
		if isCell {
			chosen(entry.Path)
		}
	})
	picker.SetOnClose(func() {
		g.captureMouse(previousMouseState)
	})
	g.replaceWidgetStack(picker)
}

// meshTile is a model's cell: a render of the model itself, off screen. It is a
// method because the render needs the GL context the application holds.
func (g *Game) meshTile(dir string, _, _ int) *image.RGBA {
	return g.MeshThumbnail(meshFileIn(dir))
}

// hasSpriteKind is whether a folder is one kind rather than a category of them.
func hasSpriteKind(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, spriteKindFile))
	return err == nil
}

// spriteTile is a sprite kind's cell: the first frame of its idle, cut out of its
// own sheet. Read off disk rather than out of the uploaded atlas, because a
// folder nobody has placed has no atlas yet and browsing must not be what uploads
// six hundred of them.
func spriteTile(dir string, w, h int) *image.RGBA {
	raw, err := os.ReadFile(filepath.Join(dir, spriteKindFile))
	if err != nil {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	kind := &SpriteKind{}
	if err := json.Unmarshal(raw, kind); err != nil {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	sheet := quads.Disk.Image(filepath.ToSlash(filepath.Join(dir, spriteSheetFile)))
	return quads.FitTile(frameFromSheet(sheet, kind.Tile.X, kind.Tile.Y, kind.idleFrames()), w, h)
}

// frameFromSheet is one cell of a grid sheet. A sheet whose tile size the JSON
// does not describe is drawn whole rather than cut at a guess.
func frameFromSheet(sheet image.Image, tileW, tileH int, frames []uint16) image.Image {
	if tileW <= 0 || tileH <= 0 || len(frames) == 0 {
		return sheet
	}
	columns := sheet.Bounds().Dx() / tileW
	if columns <= 0 {
		return sheet
	}
	index := int(frames[0])
	at := image.Rect(
		(index%columns)*tileW, (index/columns)*tileH,
		(index%columns+1)*tileW, (index/columns+1)*tileH,
	).Add(sheet.Bounds().Min)
	if !at.In(sheet.Bounds()) {
		return sheet
	}
	return sheet.(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(at)
}

// hasWeapon is whether a folder under assets/weapons is a gun. The two folders
// there that are not are the shared stores, and both start with an underscore.
func hasWeapon(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "weapon.json"))
	return err == nil
}

// weaponTile is a gun's cell: the picture it lies on the floor as, out of the
// shared store the migration put every weapon's pickup art in. A weapon whose
// sources had no pickup art gets an empty cell, which is the truth about it -
// placing one puts nothing in the world either.
func weaponTile(dir string, w, h int) *image.RGBA {
	raw, err := os.ReadFile(filepath.Join(dir, "weapon.json"))
	if err != nil {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	var data struct {
		Icon string `json:"icon"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.Icon == "" {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	picture := filepath.Join(filepath.Dir(dir), wep.SpritesDir, data.Icon+".png")
	return quads.FitTile(quads.Disk.Image(filepath.ToSlash(picture)), w, h)
}

// setupTileLabelFont hands the browsers the glyphs they caption their cells with.
// It is here rather than in quads because the font is embedded in eassets, and
// eassets imports quads: the pixels have to be pushed down rather than pulled up.
func (g *Game) setupTileLabelFont() {
	index, err := eassets.Embedded.FS.Open("quadratica.idx")
	if err != nil {
		log.Printf("[editor] no font index for the browser captions: %v", err)
		return
	}
	defer index.Close()
	quads.SetTileLabelFont(
		eassets.Embedded.Image("quadratica.png"), 8, 14,
		quads.NewBitmapFontIndexFromReader(index))
}
