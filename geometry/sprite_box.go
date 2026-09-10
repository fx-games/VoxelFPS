package geometry

// The box a picture deserves: the part of a sprite's tile that is actually
// drawn, measured off the pixels.
//
// It lives here because two very different things need the same answer and must
// not disagree about it. The engine measures a frame off the card, from the
// texture it uploaded; the DOS-game importers measure the same frame off the
// PNG they are about to write, so the answer can be stored in sprite.json and a
// spawn does not have to read the card back at all. One scan, one convention.

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
)

// SpriteBoxDepth is how thick a billboard's box is, in tile units. A picture
// has no depth of its own; this is the sliver it is given so that a ray has
// something to hit and a solver something to stop against. It grows with the
// sprite, because it is scaled along with the rest of the box.
const SpriteBoxDepth = 0.05

// Box is a sprite's box in its tile's own units: x runs -0.5 to 0.5 across the
// tile, y runs 0 at the tile's bottom to 1 at its top, and z is the sliver.
// Multiplying by the size the quad is drawn at is what makes it world units.
type Box struct {
	Min mgl32.Vec3 `json:"min"`
	Max mgl32.Vec3 `json:"max"`
}

// Empty is whether this box measures nothing, which is what a frame with no
// opaque pixel in it gives. Callers use it to fall back rather than to hand
// physics a point.
func (b Box) Empty() bool {
	return b.Max.X() <= b.Min.X() || b.Max.Y() <= b.Min.Y()
}

// Union is the smallest box holding both, so that an animation can have one box
// rather than a box that pops from frame to frame. An empty box on either side
// contributes nothing.
func (b Box) Union(other Box) Box {
	if b.Empty() {
		return other
	}
	if other.Empty() {
		return b
	}
	return Box{
		Min: mgl32.Vec3{lower(b.Min.X(), other.Min.X()), lower(b.Min.Y(), other.Min.Y()), lower(b.Min.Z(), other.Min.Z())},
		Max: mgl32.Vec3{upper(b.Max.X(), other.Max.X()), upper(b.Max.Y(), other.Max.Y()), upper(b.Max.Z(), other.Max.Z())},
	}
}

// lower and upper are min and max for float32s. The package's own min and max
// are for the integer grid everything else in here works on.
func lower(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func upper(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// OpaqueBoxOfPixels measures one tile out of an RGBA byte sequence, rows first
// and top down - which is how a texture comes back off the card.
func OpaqueBoxOfPixels(pixels []uint8, width, height int) Box {
	return opaqueBox(width, height, func(x, y int) bool {
		at := (y*width + x) * 4
		return at+3 < len(pixels) && pixels[at+3] > 0
	})
}

// OpaqueBoxOfImage measures one rectangle of a decoded picture, which is how a
// sheet's cell is measured before the sheet is written.
func OpaqueBoxOfImage(img image.Image, cell image.Rectangle) Box {
	cell = cell.Intersect(img.Bounds())
	if cell.Empty() {
		return Box{}
	}
	return opaqueBox(cell.Dx(), cell.Dy(), func(x, y int) bool {
		_, _, _, alpha := img.At(cell.Min.X+x, cell.Min.Y+y).RGBA()
		return alpha > 0
	})
}

// opaqueBox is the scan both of those are: the first and last row and column
// with anything drawn on them, turned into the tile's own units.
//
// Note the y flip. The scan counts rows down from the top, the way a picture is
// stored; the box counts up from the floor, the way a thing standing in a room
// is measured. And note the +1 on the far edges: the last opaque column is one
// column wide, so the box's right-hand edge is past it, not on it.
func opaqueBox(width, height int, opaque func(x, y int) bool) Box {
	if width <= 0 || height <= 0 {
		return Box{}
	}
	minX, minY := width, height
	maxX, maxY := -1, -1
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !opaque(x, y) {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < 0 {
		return Box{}
	}
	left := float32(minX)/float32(width) - 0.5
	right := float32(maxX+1)/float32(width) - 0.5
	top := 1 - float32(minY)/float32(height)
	bottom := 1 - float32(maxY+1)/float32(height)
	return Box{
		Min: mgl32.Vec3{left, bottom, -SpriteBoxDepth},
		Max: mgl32.Vec3{right, top, SpriteBoxDepth},
	}
}

// FrameBox is one frame's own box, kept as the four numbers that vary: left,
// bottom, right, top, in the tile's own units. The depth is the same sliver for
// every frame there has ever been - see SpriteBoxDepth - so it is not written
// out once per frame.
type FrameBox [4]float32

// Box is this frame's box in the shape everything else takes.
func (f FrameBox) Box() Box {
	return Box{
		Min: mgl32.Vec3{f[0], f[1], -SpriteBoxDepth},
		Max: mgl32.Vec3{f[2], f[3], SpriteBoxDepth},
	}
}

// AsFrameBox is the other direction, for storing a measured box.
func AsFrameBox(b Box) FrameBox {
	return FrameBox{b.Min.X(), b.Min.Y(), b.Max.X(), b.Max.Y()}
}

// SpriteBoxesFile is what a kind's per-frame boxes are stored in, beside the
// sheet they were measured off: one box per cell of the grid, in the order a
// frame index runs.
//
// Beside the sheet rather than inside sprite.json because sprite.json is a file
// somebody reads and edits - which frames make which animation - and this is a
// thousand numbers nobody will ever look at. Measured once, at import, because
// a picture that has been written to disk is not going to change its mind about
// which of its pixels are opaque.
const SpriteBoxesFile = "boxes.json"

// MeasureCells measures every cell of a grid sheet on its own, in the order a
// frame index runs across it. A cell with nothing drawn on it comes back empty,
// which is what the unused corner of a grid is.
func MeasureCells(sheet image.Image, tileWidth, tileHeight int) []FrameBox {
	if tileWidth <= 0 || tileHeight <= 0 {
		return nil
	}
	bounds := sheet.Bounds()
	columns, rows := bounds.Dx()/tileWidth, bounds.Dy()/tileHeight
	boxes := make([]FrameBox, 0, columns*rows)
	for index := 0; index < columns*rows; index++ {
		boxes = append(boxes, AsFrameBox(OpaqueBoxOfImage(sheet, GridCell(tileWidth, tileHeight, columns, index))))
	}
	return boxes
}

// WriteFrameBoxes puts them beside the sheet, one line of numbers rather than
// the fifty thousand lines an indented one of these would be.
func WriteFrameBoxes(dir string, boxes []FrameBox) error {
	data, err := json.Marshal(boxes)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, SpriteBoxesFile), append(data, '\n'), 0o644)
}

// ReadFrameBoxes reads them back, or nothing at all for a folder written before
// they were measured - which is not an error, it is a kind that falls back to
// the one box its animation has.
func ReadFrameBoxes(dir string) ([]FrameBox, error) {
	data, err := os.ReadFile(filepath.Join(dir, SpriteBoxesFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var boxes []FrameBox
	return boxes, json.Unmarshal(data, &boxes)
}

// GridCell is where one frame of a plain grid sheet is, which is the only
// layout the fixed atlas reads: index runs row-major across tiles of one size.
func GridCell(tileWidth, tileHeight, columns, index int) image.Rectangle {
	if columns <= 0 {
		columns = 1
	}
	x, y := (index%columns)*tileWidth, (index/columns)*tileHeight
	return image.Rect(x, y, x+tileWidth, y+tileHeight)
}

// BoxAcrossDirections is the one box an animation drawn from several angles
// deserves: each facing's own frames unioned into that facing's box, and then
// the middle of those boxes, component by component.
//
// Unioning the facings together instead - which is what this was - gives the
// widest silhouette the thing has from any angle, and for art cropped to its
// own content that is the whole tile. System Shock's avian mutant is a hundred
// and forty pixels across with its wings out sideways and about seventy face
// on, so one box over all eight facings was twice as wide as the bird actually
// in front of you, and you bumped into it a body's width early.
//
// A billboard has one box whichever way it is turned, so a single number has to
// stand in for eight views, and the middle one is the honest choice: the widest
// over-covers from seven angles out of eight and the narrowest under-covers
// from seven. The median is taken per component rather than by picking one
// facing whole, because a facing that happens to be drawn off-centre should not
// carry both edges with it.
//
// A kind drawn from one angle only - an explosion, a medkit on the floor - has
// a single facing, so this is exactly the union of its own frames and nothing
// about a growing blast changes.
func BoxAcrossDirections(sheet image.Image, tileWidth, tileHeight, columns int, directions [][]int) Box {
	var boxes []Box
	for _, indices := range directions {
		box := UnionOfCells(sheet, tileWidth, tileHeight, columns, indices)
		if !box.Empty() {
			boxes = append(boxes, box)
		}
	}
	if len(boxes) == 0 {
		return Box{}
	}
	return Box{
		Min: mgl32.Vec3{
			medianOf(boxes, func(b Box) float32 { return b.Min.X() }),
			medianOf(boxes, func(b Box) float32 { return b.Min.Y() }),
			-SpriteBoxDepth,
		},
		Max: mgl32.Vec3{
			medianOf(boxes, func(b Box) float32 { return b.Max.X() }),
			medianOf(boxes, func(b Box) float32 { return b.Max.Y() }),
			SpriteBoxDepth,
		},
	}
}

// medianOf is the middle of one component across the facings, averaging the two
// middle values when there is an even number of them - eight facings is the
// usual case and has no single middle.
func medianOf(boxes []Box, of func(Box) float32) float32 {
	values := make([]float32, 0, len(boxes))
	for _, box := range boxes {
		values = append(values, of(box))
	}
	slices.Sort(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

// UnionOfCells is the one box holding every named frame of a grid sheet, which
// is what an animation gets: a box that changes from frame to frame is a box
// that pops while the thing is walking, and a ray that hits a guard on one
// frame and misses him on the next.
func UnionOfCells(sheet image.Image, tileWidth, tileHeight, columns int, indices []int) Box {
	var box Box
	for _, index := range indices {
		box = box.Union(OpaqueBoxOfImage(sheet, GridCell(tileWidth, tileHeight, columns, index)))
	}
	return box
}
