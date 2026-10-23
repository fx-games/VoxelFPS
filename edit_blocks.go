package main

// The block list: the cubes that have been dressed in the block texture mapper,
// kept so the next wall of the same stuff is one pick away rather than six
// drags. Closing the mapper on a new combination adds it; the block mode's
// browser lists them, a left click puts one in the brush and a right click
// takes it off the list.
//
// The list is slot numbers, the same thing a block stores, so it belongs to one
// level's atlas and lives beside it in the level's folder. A save bakes that
// atlas down and renumbers it, so the list's slots are kept through the bake
// and moved with it - see saveMap.

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/memmaker/voxelfps/quads/voxel"
)

// blockDesign is a block's six atlas slots, indexed by voxel.CubeSide.
type blockDesign [6]uint16

func designOf(block *voxel.Block) blockDesign {
	var design blockDesign
	for side := range design {
		design[side] = block.GetTexture(voxel.CubeSide(side))
	}
	return design
}

func (d blockDesign) block() *voxel.Block {
	textures := make(map[voxel.CubeSide]uint16, len(d))
	for side, slot := range d {
		textures[voxel.CubeSide(side)] = slot
	}
	return voxel.NewBlockWithSides(textures)
}

// blocksFile is where the list is kept: next to the atlas, named after it.
func (g *Game) blocksFile() string {
	atlas := g.MapTextures.Atlas.SourceFile()
	return strings.TrimSuffix(atlas, ".png") + ".blocks.json"
}

// loadBlocks reads the level's list. No file yet is an empty list, not a failure.
func (g *Game) loadBlocks() {
	g.editor.blocks = nil
	raw, err := os.ReadFile(g.blocksFile())
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err == nil {
		err = json.Unmarshal(raw, &g.editor.blocks)
	}
	if err != nil {
		log.Printf("[editor] block list %s unreadable: %v", g.blocksFile(), err)
	}
}

func (g *Game) saveBlocks() error {
	raw, err := json.MarshalIndent(g.editor.blocks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.blocksFile(), raw, 0o644)
}

// rememberBlock puts a design on the list unless it is already there.
func (g *Game) rememberBlock(design blockDesign) {
	if slices.Contains(g.editor.blocks, design) {
		return
	}
	g.editor.blocks = append(g.editor.blocks, design)
	if err := g.saveBlocks(); err != nil {
		g.Toast("Block list not saved: " + err.Error())
		return
	}
	g.Toast(fmt.Sprintf("Block added to the list (%d)", len(g.editor.blocks)))
}

// openBlockPicker is the block mode's browser: every design on the list as a
// cube. A left click loads it into the brush, a right click forgets it.
func (g *Game) openBlockPicker() {
	if len(g.editor.blocks) == 0 {
		g.Toast("No blocks yet - dress one in Block Textures and it lands here")
		return
	}
	names := make([]string, len(g.editor.blocks))
	for index := range names {
		names[index] = fmt.Sprintf("block %d", index+1)
	}
	previousMouseState := g.setupModal()
	picker := g.OpenListBrowser("blocks", names, func(index, w, h int) *image.RGBA {
		return g.blockTile(g.editor.blocks[index], w, h)
	})
	closePicker := func() {
		// Close, not just pop: the cubes are an uploaded texture in the 2D pass.
		picker.Close()
		g.popFromWidgetStack()
	}
	picker.SetOnIndexClicked(func(index int) {
		closePicker()
		g.editor.voxelBlock = g.editor.blocks[index].block()
		g.editor.tool = EditorToolNone // a block picked is a block to build with
	})
	picker.SetOnIndexRightClicked(func(index int) {
		if index >= len(g.editor.blocks) {
			return
		}
		g.editor.blocks = slices.Delete(g.editor.blocks, index, index+1)
		if err := g.saveBlocks(); err != nil {
			g.Toast("Block list not saved: " + err.Error())
		}
		closePicker()
		g.openBlockPicker() // the page again, one cube shorter
	})
	picker.SetOnClose(func() {
		g.captureMouse(previousMouseState)
	})
	g.replaceWidgetStack(picker)
	g.Toast("LMB use block   RMB remove it")
}

// blockTile is a design drawn as an isometric cube - its top and two of its
// sides, the sides shaded darker so the three read as a corner.
func (g *Game) blockTile(design blockDesign, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	cx, qy := float64(w)/2, float64(h)/4
	// Each face is a parallelogram: a corner and the two edges leaving it,
	// along the texture's width and down its height.
	faces := []struct {
		side                 voxel.CubeSide
		origin, across, down [2]float64
		shade                float64
	}{
		{voxel.Top, [2]float64{cx, 0}, [2]float64{cx, qy}, [2]float64{-cx, qy}, 1},
		{voxel.South, [2]float64{0, qy}, [2]float64{cx, qy}, [2]float64{0, 2 * qy}, 0.8},
		{voxel.East, [2]float64{cx, 2 * qy}, [2]float64{cx, -qy}, [2]float64{0, 2 * qy}, 0.6},
	}
	for _, face := range faces {
		texture := g.MapTextures.Tile(int(design[face.side]))
		bounds := texture.Bounds()
		a, b := face.across, face.down
		det := a[0]*b[1] - a[1]*b[0]
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				px, py := float64(x)+0.5-face.origin[0], float64(y)+0.5-face.origin[1]
				u, v := (px*b[1]-py*b[0])/det, (a[0]*py-a[1]*px)/det
				if u < 0 || u >= 1 || v < 0 || v >= 1 {
					continue
				}
				r, gr, bl, al := texture.At(bounds.Min.X+int(u*float64(bounds.Dx())), bounds.Min.Y+int(v*float64(bounds.Dy()))).RGBA()
				out.Set(x, y, color.RGBA64{
					R: uint16(float64(r) * face.shade), G: uint16(float64(gr) * face.shade),
					B: uint16(float64(bl) * face.shade), A: uint16(al),
				})
			}
		}
	}
	return out
}

// keptThroughBake is the slots a bake has to keep: the map's own and every one
// a design points at, sorted and each once, the way Bake takes them.
func keptThroughBake(used []uint16, designs []blockDesign) []uint16 {
	kept := slices.Clone(used)
	for _, design := range designs {
		kept = append(kept, design[:]...)
	}
	slices.Sort(kept)
	return slices.Compact(kept)
}

// remapDesigns moves designs onto the slots a bake gave their textures.
func remapDesigns(designs []blockDesign, remap map[uint16]uint16) {
	for i := range designs {
		for side, slot := range designs[i] {
			designs[i][side] = remap[slot]
		}
	}
}
