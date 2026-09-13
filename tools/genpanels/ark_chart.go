package main

// The ark's wayfinding plate: the whole ship in one drawing, the way the people
// who built it would have hung it by a door - a general arrangement in section,
// with a directory of decks beside it and a legend under that.
//
// It is drawn here, with the other panels, because it is the same object they
// are: one box with a picture on one face. The difference is that the picture is
// the point rather than a placeholder the game paints over, so the whole of it
// is baked into the model's own texture and nothing in the game knows a chart
// from a table. Hanging one is a line of build.map and nothing else.
//
// What it says is as built, before the war: the plate was screwed to the wall
// when the ship was new, and it has no idea that deck seven is open to space or
// that the shaft is choked in two places. The signs in the level carry that
// news; a printed plate cannot. Every number on it - the deck heights, the
// hull line, where the bulkheads and airlocks are - is read off the same
// figures assets/maps/ark/build.map builds the level from, so the drawing and
// the ship stay the same shape.

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// The plate, in world units where a voxel is 1 and a voxel is a shade over two
// metres: 1.1m by 1.7m, hung with its bottom at 0.6m. That is a poster rather
// than a sign - it is read standing in front of it, not glanced at down a
// corridor - and it still clears the two-voxel ceiling of the deck it hangs on.
// Hung like every other panel here: origin on the floor, picture along -Z.
const (
	arkChartWidth  = 0.53
	arkChartHeight = 0.80
	arkChartDepth  = 0.04
	arkChartBottom = 0.29
)

// The texture. The readable face is the left atlasSplit of it (see the other
// panels), so the art is 768 by 1152 - the plate's own 2:3 - and the right
// quarter is the flat swatch the five other faces of the box sample.
const (
	chartTexWidth  = 1024
	chartTexHeight = 1152
	chartArtWidth  = int(atlasSplit * chartTexWidth)
)

// The plate's inks. An anodised aluminium plate, etched dark, with one oxide
// red for the three things a reader has to find in a hurry: where they are
// standing, where the airlocks are, and which way is down.
var (
	chartPlate    = color.NRGBA{0xD5, 0xD1, 0xC4, 0xFF}
	chartPlateDim = color.NRGBA{0xC4, 0xC0, 0xB2, 0xFF}
	chartEdge     = color.NRGBA{0xB0, 0xAB, 0x9C, 0xFF}
	chartInk      = color.NRGBA{0x21, 0x2A, 0x31, 0xFF}
	chartInkPale  = color.NRGBA{0x74, 0x7C, 0x7E, 0xFF}
	chartBody     = color.NRGBA{0xBD, 0xC3, 0xBE, 0xFF}
	chartAir      = color.NRGBA{0xA9, 0xBC, 0xB4, 0xFF}
	chartIce      = color.NRGBA{0xBE, 0xCF, 0xD6, 0xFF}
	chartRed      = color.NRGBA{0xA6, 0x3C, 0x26, 0xFF}
)

// The drawing's frame, in texture pixels.
const (
	chartMargin      = 22
	chartSectionLeft = chartMargin + 10
	chartAxis        = chartSectionLeft + 150 // the ship's own axis, x=80
	chartWidthScale  = 4.05
	chartBreakRow    = 860 // where the section changes scale, in the lower cone
	chartFootRow     = 996
	// chartDoorHeight is how tall a shaft door is drawn where a voxel of the
	// ship is smaller than that, which under the scale break it is.
	chartDoorHeight = 7
)

// The directory beside the section.
const (
	chartListLeft = 352
	chartListTop  = 182
)

// chartAnchors pin heights of the ship to rows of the drawing, and everything
// between two of them is drawn at the scale they imply. It is a schematic
// rather than a section to scale: the eleven decks people live on are
// thirty-four voxels of a ship a hundred and eighty-two tall, and drawn true
// they are eight pixels each - a drawing of nothing, on the one plate that has
// to answer "which deck am I on". So the decks get half the plate, the drum and
// the ice above them are squashed to a third of that, and the lower cone - the
// reaction mass and the six machine decks nobody was meant to visit - is
// squashed again below a break. Widths are on one scale throughout.
var chartAnchors = [][2]float64{{183, 176}, {170, 230}, {149, 420}, {110, 900}, {0, float64(chartFootRow)}}

// chartHalfWidth is how wide the ship is at a height, in voxels either side of
// its axis - the same figures build.map builds it from: a ball of ice capping
// the drum, the drum itself, the cone of the hull between the drum and the tip,
// and the drive bell opening downwards under it.
func chartHalfWidth(y float64) float64 {
	switch {
	case y > 170: // the ice: a radius-60 ball whose rim meets the drum
		if d := 3600 - (y-122)*(y-122); d > 0 {
			return math.Sqrt(d)
		}
		return 0
	case y >= 149:
		return 36
	case y >= 10: // the hull line, r(y) = 4 + (y-10)*26/140
		return 4 + (y-10)*26/140
	case y >= 9:
		return 4
	default: // the bell
		return 10 - y*6/9
	}
}

// chartRow is where a height in voxels lands on the plate, read off the
// anchors either side of it.
func chartRow(y float64) int {
	for i := 0; i < len(chartAnchors)-1; i++ {
		top, bottom := chartAnchors[i], chartAnchors[i+1]
		if y <= top[0] && y >= bottom[0] {
			return int(top[1] + (top[0]-y)/(top[0]-bottom[0])*(bottom[1]-top[1]))
		}
	}
	return chartFootRow
}

// chartHeightAt is chartRow read backwards: which height of the ship a row of
// the drawing is.
func chartHeightAt(row int) float64 {
	for i := 0; i < len(chartAnchors)-1; i++ {
		top, bottom := chartAnchors[i], chartAnchors[i+1]
		if float64(row) >= top[1] && float64(row) <= bottom[1] {
			return top[0] - (float64(row)-top[1])/(bottom[1]-top[1])*(top[0]-bottom[0])
		}
	}
	return 0
}

// chartColumn is a distance either side of the ship's axis, in voxels, as a
// column on the plate.
func chartColumn(offset float64) int {
	return chartAxis + int(offset*chartWidthScale)
}

// deck is one line of the directory and one band of the section: where it
// stands, what it is called, and what is on it. Floor and ceiling are the
// voxels build.map lays its slab and its lid at.
type deck struct {
	tag     string
	name    string
	detail  string
	floor   float64
	ceiling float64
}

// The ship, top to bottom, exactly as build.map builds it. The drum and the ice
// are not decks and carry no tag; the engineering decks below the lower
// bulkhead are one voxel of grating each, which is why their ceilings are their
// own floors again.
var chartDecks = []deck{
	{"", "ICE SHIELD", "WATER ICE - DUST AND RADIATION", 170, 183},
	{"", "THE DRUM", "TANKS - CORE - EIGHT BOOMS", 149, 170},
	{"D11", "BRIDGE", "COMMAND - AUTHORIZED CREW", 146, 148},
	{"D10", "PROMENADE", "GARDENS UNDER GLASS", 141, 145},
	{"D9", "THE RESIDENCES", "FOUR SUITES - ATRIUM", 137, 140},
	{"D8", "STAFF RESIDENCES", "OFFICERS - SCIENTISTS", 134, 136},
	{"D7", "HYDROPONICS", "THE FARM", 130, 133},
	{"D6", "MEDICAL", "LAB - GENE VAULT - WARD - COLD STORE", 127, 129},
	{"D5", "CREW QUARTERS", "AIRLOCK EAST", 124, 126},
	{"D4", "SERVICE COMMONS", "MESS - LAUNDRY - CHAPEL - WASH", 122, 123},
	{"D3", "SERVICE CABINS", "3-001 TO 3-136", 120, 121},
	{"D2", "SERVICE CABINS", "2-001 TO 2-136", 118, 119},
	{"D1", "LIFE SUPPORT", "WATER RECLAMATION - AIRLOCK WEST", 115, 117},
	{"E6", "STORES", "", 104, 104},
	{"E5", "PUMP HALL", "", 90, 90},
	{"E4", "MASS DECK", "", 76, 76},
	{"E3", "MASS DECK", "", 62, 62},
	{"E2", "COOLANT", "", 48, 48},
	{"E1", "REACTOR GALLERY", "", 34, 34},
	{"", "DRIVE", "REACTOR - SHADOW SHIELD - BELL", 0, 27},
}

// youAreHere is the deck this plate hangs on. A printed plate cannot work out
// where it is, so it is told - the same way a real one is printed per deck and
// screwed up where it belongs.
const youAreHere = "D6"

// The two bulkheads that cut the ship into three: the hatch under D1 and the
// one over D6, which is what holds the air in the six decks between them.
var chartBulkheads = []float64{115, 130}

// arkChartTexture draws the plate.
func arkChartTexture() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, chartTexWidth, chartTexHeight))
	rect(img, 0, 0, chartTexWidth-1, chartTexHeight-1, chartPlate)
	// The swatch the box's other five faces read, so its edges are plate rather
	// than a slice of the drawing.
	rect(img, chartArtWidth, 0, chartTexWidth-1, chartTexHeight-1, chartEdge)

	chartFrame(img)
	chartTitle(img)
	chartSection(img)
	chartDirectory(img)
	chartLegend(img)
	return encode(img)
}

// chartFrame is the plate's border and the four screws that hold it up.
func chartFrame(img *image.NRGBA) {
	right, bottom := chartArtWidth-chartMargin, chartTexHeight-chartMargin
	outline(img, chartMargin, chartMargin, right, bottom, 3, chartInk)
	outline(img, chartMargin+7, chartMargin+7, right-7, bottom-7, 1, chartInkPale)
	for _, at := range [][2]int{
		{chartMargin + 18, chartMargin + 18}, {right - 18, chartMargin + 18},
		{chartMargin + 18, bottom - 18}, {right - 18, bottom - 18},
	} {
		disc(img, at[0], at[1], 6, chartEdge)
		disc(img, at[0], at[1], 2, chartInkPale)
	}
}

// chartTitle is the head of the plate: what the ship is called, what the
// drawing is, and - in the small print a plate gives the reader to check they
// are looking at the right copy - which deck this one was screwed to.
func chartTitle(img *image.NRGBA) {
	label(img, image.Pt(chartSectionLeft, 32), "THE ARK", 5, 3, chartInk)
	label(img, image.Pt(chartSectionLeft, 106), "GENERAL ARRANGEMENT - DECK DIRECTORY", 2, 1, chartInk)
	label(img, image.Pt(chartSectionLeft, 138), "AS BUILT - SCHEMATIC, NOT TO SCALE", 1, 1, chartInkPale)
	label(img, image.Pt(chartListLeft, 138), "THIS PLATE HANGS ON DECK 6 - MEDICAL", 1, 1, chartRed)
	rect(img, chartSectionLeft, 158, chartArtWidth-chartMargin-10, 160, chartInk)
}

// chartSection draws the ship itself: the hull cut open down its axis, every
// deck as a band across it, the spine up the middle with its ladder in it, and
// the handful of marks a reader actually navigates by.
func chartSection(img *image.NRGBA) {
	// The hull, row by row: its two edges and what is between them. Reading the
	// ship's own half width per row draws the ice cap, the drum, the cone and
	// the bell in one loop, so the drawing cannot drift from the level's shape.
	for row := chartRow(183); row <= chartFootRow; row++ {
		y := chartHeightAt(row)
		half := chartHalfWidth(y)
		left, right := chartColumn(-half), chartColumn(half)
		inside := chartBody
		switch {
		case y > 170:
			inside = chartIce
		case y >= 115 && y <= 130:
			// The six decks that hold air, which is the one thing on this plate
			// that has ever mattered to the person reading it.
			inside = chartAir
		}
		rect(img, left, row, right, row, inside)
		rect(img, left, row, left+2, row, chartInk)
		rect(img, right-2, row, right, row, chartInk)
	}

	// The spine: a 3x3 shaft the whole height of the ship with a door onto every
	// deck, and the ladder standing in it. It is the ship's one corridor, so it
	// is drawn as one - two rails and a rung every few pixels.
	shaftLeft, shaftRight := chartColumn(-1.5), chartColumn(1.5)
	braceX := chartColumn(17)
	top, bottom := chartRow(149), chartRow(28)

	// The doors, which are the whole point of the shaft: build.map cuts one out
	// of its east wall a voxel above every deck's floor, all seventeen of them,
	// so the right rail is drawn with a gap at each rather than as an unbroken
	// tube. Without them the plate says a reader gets onto E6 by cutting.
	doorway := map[int]bool{}
	for _, d := range chartDecks {
		if d.tag == "" {
			continue
		}
		sill := chartRow(d.floor)
		lintel := chartRow(d.floor + 1)
		if sill-lintel < chartDoorHeight {
			lintel = sill - chartDoorHeight
		}
		for row := lintel; row <= sill; row++ {
			doorway[row] = true
		}
	}
	for row := top; row <= bottom; row++ {
		rect(img, shaftLeft, row, shaftLeft, row, chartInk)
		if !doorway[row] {
			rect(img, shaftRight, row, shaftRight, row, chartInk)
		}
	}
	for rung := top + 7; rung < bottom; rung += 14 {
		rect(img, shaftLeft+2, rung, shaftRight-2, rung, chartInkPale)
	}

	// The decks: a line at every floor, with the number inside the hull at its
	// left. The drum and the ice are not decks and get the pale line only.
	for _, d := range chartDecks {
		row := chartRow(d.floor)
		if d.floor < 110 {
			// Below the break the machine decks are one brace rather than six
			// drawn decks, but each of them has its door like everything else,
			// so each gets the stub of floor that door opens onto.
			if d.tag != "" {
				rect(img, shaftRight, row, braceX, row, chartInkPale)
			}
			continue
		}
		half := chartHalfWidth(d.floor)
		left, right := chartColumn(-half)+3, chartColumn(half)-3
		ink := chartInk
		if d.tag == "" {
			ink = chartInkPale
		}
		rect(img, left, row, right, row, ink)
		if d.tag != "" {
			label(img, image.Pt(left+7, row-16), d.tag, 1, 0, chartInk)
		}
	}

	// Where the reader is standing, which is the whole reason a deck gets a
	// plate of its own: its band picked out, and said in words inside it.
	for _, d := range chartDecks {
		if d.tag != youAreHere {
			continue
		}
		top, bottom := chartRow(d.ceiling+1), chartRow(d.floor)
		half := chartHalfWidth(d.floor)
		for row := top; row <= bottom; row++ {
			for x := chartColumn(-half) + 3; x <= chartColumn(half)-3; x++ {
				if (x+row)%3 == 0 {
					img.SetNRGBA(x, row, chartRed)
				}
			}
		}
		outline(img, chartColumn(-half)+3, top, chartColumn(half)-3, bottom, 2, chartRed)
		words := "YOU ARE HERE"
		wide := labelWidth(words, 1, 1)
		at := image.Pt(chartAxis+16, (top+bottom)/2-7)
		rect(img, at.X-4, at.Y-3, at.X+wide+2, at.Y+15, chartPlate)
		label(img, at, words, 1, 1, chartRed)
	}

	// The two bulkheads that cut the ship into three. They are why the middle of
	// it is a place you can breathe in, so they are drawn in the ink a reader
	// looks for first.
	for _, y := range chartBulkheads {
		row := chartRow(y)
		half := chartHalfWidth(y)
		rect(img, chartColumn(-half), row-2, chartColumn(half), row+2, chartRed)
	}

	// The two airlocks, each on the side of the hull it is really on: D1 to the
	// west, D5 to the east. A suit goes out of these and nowhere else - the
	// legend says what the mark is, so nothing has to be written out here.
	airlock(img, 116, -1)
	airlock(img, 125, 1)

	// The break in the scale, and under it the lower cone: the machine decks in
	// one brace rather than six lines, since nothing down there is a place to
	// be going.
	for x := chartColumn(-42); x <= chartColumn(42); x++ {
		wobble := int(3 * math.Sin(float64(x)/9))
		rect(img, x, chartBreakRow+wobble, x, chartBreakRow+2+wobble, chartInk)
		rect(img, x, chartBreakRow+14+wobble, x, chartBreakRow+16+wobble, chartInk)
		rect(img, x, chartBreakRow+3+wobble, x, chartBreakRow+13+wobble, chartPlate)
	}
	braceTop, braceBottom := chartRow(104), chartRow(34)
	rect(img, braceX, braceTop, braceX+2, braceBottom, chartInkPale)
	rect(img, braceX, braceTop, braceX+8, braceTop+2, chartInkPale)
	rect(img, braceX, braceBottom, braceX+8, braceBottom+2, chartInkPale)
	label(img, image.Pt(braceX+14, (braceTop+braceBottom)/2-12), "E1-E6", 1, 1, chartInk)
	label(img, image.Pt(braceX+14, (braceTop+braceBottom)/2+4), "SUIT ONLY", 1, 1, chartRed)

	// Which way is down. The ship tumbles about the drum, so weight is towards
	// the drive on every deck and falls off the higher you live - the one fact
	// about this ship that catches out everybody who was not born on it.
	arrowX := chartColumn(-30)
	arrowTop, arrowBottom := chartRow(126), chartRow(118)
	rect(img, arrowX, arrowTop, arrowX, arrowBottom, chartRed)
	marker(img, arrowX, arrowBottom+11, 6, chartRed)
	labelDown(img, arrowX-6, arrowTop-68, "DOWN", chartRed)
}

// airlock marks one on the hull: a chamber drawn on the outside of the plating,
// on the side of the ship it is really on.
func airlock(img *image.NRGBA, y float64, side int) {
	row := chartRow(y)
	edge := chartColumn(float64(side) * chartHalfWidth(y))
	from, to := edge, edge+side*20
	if side < 0 {
		from, to = to, from
	}
	rect(img, from, row-8, to, row+8, chartRed)
	rect(img, from+2, row-6, to-2, row+6, chartPlate)
}

// chartDirectory is the list beside the section: every deck by its number, what
// it is called and what is on it, in the order they stand. A row with something
// to say about it is given the room to say it; the machine decks are a name and
// a number each.
func chartDirectory(img *image.NRGBA) {
	right := chartArtWidth - chartMargin - 10
	top := chartListTop
	for _, d := range chartDecks {
		height := 28
		if d.detail != "" {
			height = 44
		}
		// A row's block is the whole of its height less a hair, so the line of
		// small print at the foot of one row is never painted over by the block
		// of the next.
		blockTop, blockBottom := top-4, top+height-5
		if d.tag == "" {
			rect(img, chartListLeft-6, blockTop, right, blockBottom, chartPlateDim)
		}
		ink := chartInk
		if d.tag == "" {
			ink = chartInkPale
		}
		label(img, image.Pt(chartListLeft, top), d.tag, 2, 1, chartRed)
		label(img, image.Pt(chartListLeft+64, top), d.name, 2, 1, ink)
		if d.detail != "" {
			label(img, image.Pt(chartListLeft+64, top+26), d.detail, 1, 1, chartInkPale)
		}
		if d.tag == youAreHere {
			outline(img, chartListLeft-8, blockTop, right, blockBottom, 2, chartRed)
		}
		top += height
	}
}

// chartLegend is the small print along the foot: what the marks on the section
// mean, and the two things about this ship that no plan of it can show.
func chartLegend(img *image.NRGBA) {
	top := chartFootRow + 18
	left := chartSectionLeft
	right := chartArtWidth - chartMargin - 10
	rect(img, left, top, right, top+1, chartInk)

	rect(img, left, top+16, left+24, top+20, chartRed)
	label(img, image.Pt(left+34, top+12), "PRESSURE BULKHEAD", 1, 1, chartInk)
	rect(img, left, top+36, left+24, top+40, chartInk)
	label(img, image.Pt(left+34, top+32), "DECK PLATE", 1, 1, chartInk)

	rect(img, left+230, top+12, left+250, top+28, chartRed)
	rect(img, left+232, top+14, left+248, top+26, chartPlate)
	label(img, image.Pt(left+260, top+16), "AIRLOCK", 1, 1, chartInk)
	rect(img, left+230, top+36, left+230, top+64, chartInk)
	rect(img, left+242, top+36, left+242, top+44, chartInk)
	rect(img, left+242, top+54, left+242, top+64, chartInk)
	for rung := top + 40; rung < top+64; rung += 10 {
		rect(img, left+231, rung, left+241, rung, chartInkPale)
	}
	rect(img, left+230, top+36, left+242, top+36, chartInk)
	label(img, image.Pt(left+260, top+38), "SPINE SHAFT AND LADDER,", 1, 1, chartInk)
	label(img, image.Pt(left+260, top+52), "A DOOR ON EVERY DECK", 1, 1, chartInkPale)

	label(img, image.Pt(left, top+74), "THE SHIP TUMBLES ABOUT THE DRUM. DOWN IS TOWARD THE DRIVE ON EVERY", 1, 1, chartInk)
	label(img, image.Pt(left, top+88), "DECK, AND YOU WEIGH LESS THE HIGHER YOU LIVE.", 1, 1, chartInk)
}

// ---------------------------------------------------------------- drawing ---

// outline is an unfilled rectangle of a given thickness.
func outline(img *image.NRGBA, x0, y0, x1, y1, thickness int, c color.NRGBA) {
	rect(img, x0, y0, x1, y0+thickness-1, c)
	rect(img, x0, y1-thickness+1, x1, y1, c)
	rect(img, x0, y0, x0+thickness-1, y1, c)
	rect(img, x1-thickness+1, y0, x1, y1, c)
}

// disc is a filled circle, for the screws in the plate's corners.
func disc(img *image.NRGBA, cx, cy, radius int, c color.NRGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				img.SetNRGBA(cx+x, cy+y, c)
			}
		}
	}
}

// marker is a triangle pointing down (positive size) at a point: the arrowhead
// under the "down" arrow and the wedge beside the deck the reader is on.
func marker(img *image.NRGBA, cx, cy, size int, c color.NRGBA) {
	for row := 0; row <= size; row++ {
		half := size - row
		rect(img, cx-half, cy-size+row, cx+half, cy-size+row, c)
	}
}

// The bitmap face every word on the plate is set in: x/image's 7x13, blown up
// whole pixels at a time so it stays a crisp etched line rather than a blur.
// It is the same look the level's own signs have (quads.SignPicture draws the
// editor's bitmap font the same way), which is the point - one ship, one
// signwriter.
const (
	chartGlyphWidth  = 7
	chartGlyphHeight = 13
)

// label draws one line and returns how wide it came out. tracking is the extra
// pixels between letters, before scaling: the difference between a title and a
// caption on a plate like this is mostly the spacing.
func label(img *image.NRGBA, at image.Point, text string, scale, tracking int, ink color.NRGBA) int {
	face := basicfont.Face7x13
	cell := image.NewNRGBA(image.Rect(0, 0, chartGlyphWidth, chartGlyphHeight))
	pen := at.X
	for _, letter := range text {
		for i := range cell.Pix {
			cell.Pix[i] = 0
		}
		drawer := font.Drawer{
			Dst: cell, Src: image.NewUniform(ink), Face: face,
			Dot: fixed.P(0, face.Ascent),
		}
		drawer.DrawString(string(letter))
		for y := 0; y < chartGlyphHeight; y++ {
			for x := 0; x < chartGlyphWidth; x++ {
				if cell.NRGBAAt(x, y).A == 0 {
					continue
				}
				rect(img, pen+x*scale, at.Y+y*scale, pen+(x+1)*scale-1, at.Y+(y+1)*scale-1, ink)
			}
		}
		pen += (chartGlyphWidth + tracking) * scale
	}
	return pen - at.X
}

// labelWidth is how wide label will draw a line, for setting one flush right.
func labelWidth(text string, scale, tracking int) int {
	return len([]rune(text)) * (chartGlyphWidth + tracking) * scale
}

// labelDown writes a word down the plate rather than across it, for the one
// label that runs beside an arrow.
func labelDown(img *image.NRGBA, x, y int, text string, ink color.NRGBA) {
	for i, letter := range text {
		label(img, image.Pt(x, y+i*16), string(letter), 1, 0, ink)
	}
}
