package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

// GuiScrollbar is a track and a thumb, drawn as two flat rects the way
// GuiBorder's own background is - no atlas frame of its own, since nothing
// here has one to spare for a bar a few pixels wide. It knows nothing about
// what it scrolls or how scrolling happens; whatever owns the list it stands
// beside tells it the shape of the problem (SetRange) and where to stand
// (SetBounds), and this just draws where that puts the thumb.
type GuiScrollbar struct {
	colorBatch *glhf.SpriteBatch

	trackColor mgl32.Vec4
	thumbColor mgl32.Vec4

	position mgl32.Vec2
	size     mgl32.Vec2

	total, visible, offset int
}

// minThumbHeight keeps the thumb grabbable even when a list is mostly empty
// screen and a handful of lines - a thumb thinner than this reads as a
// scratch on the border, not a handle.
const minThumbHeight = 16

func NewGuiScrollbar(uiContext UIContext) *GuiScrollbar {
	return &GuiScrollbar{
		colorBatch: uiContext.ColorBatch,
		trackColor: mgl32.Vec4{1, 1, 1, 0.12},
		thumbColor: mgl32.Vec4{1, 1, 1, 0.45},
	}
}

// SetColors overrides the default translucent white, for a caller whose list
// runs a different tint - GuiList's own green, say.
func (s *GuiScrollbar) SetColors(track, thumb mgl32.Vec4) {
	s.trackColor = track
	s.thumbColor = thumb
}

// SetBounds is the track's own rectangle, in the same screen space the list
// beside it draws in - usually a handful of pixels wide, run along its
// right edge.
func (s *GuiScrollbar) SetBounds(position, size mgl32.Vec2) {
	s.position = position
	s.size = size
}

// SetRange describes the list this bar is standing next to: how many lines
// there are in total, how many of them are ever on screen at once, and which
// one scrolling currently starts from.
func (s *GuiScrollbar) SetRange(total, visible, offset int) {
	s.total = total
	s.visible = visible
	s.offset = offset
}

// NeedsScrollbar is the same question the list beside it asks before
// scrolling at all: nothing is drawn when the list already shows everything.
func (s *GuiScrollbar) NeedsScrollbar() bool {
	return s.total > s.visible && s.visible > 0
}

// thumbRect is the thumb's own position and height within the track: a
// fraction of the track's height proportional to how much of the list is
// visible, floored at minThumbHeight, and slid down the remaining travel by
// how far scrolled the list already is.
func (s *GuiScrollbar) thumbRect() (top, height float32) {
	trackHeight := s.size.Y()
	height = trackHeight * float32(s.visible) / float32(s.total)
	if height < minThumbHeight {
		height = min(minThumbHeight, trackHeight)
	}
	maxScroll := s.total - s.visible
	if maxScroll <= 0 {
		return s.position.Y(), height
	}
	travel := trackHeight - height
	top = s.position.Y() + travel*float32(s.offset)/float32(maxScroll)
	return top, height
}

func (s *GuiScrollbar) Draw() {
	if !s.NeedsScrollbar() {
		return
	}
	s.colorBatch.DrawColoredRect(
		mgl32.Vec4{s.position.X(), s.position.Y(), s.position.X() + s.size.X(), s.position.Y() + s.size.Y()},
		s.trackColor)
	top, height := s.thumbRect()
	s.colorBatch.DrawColoredRect(
		mgl32.Vec4{s.position.X(), top, s.position.X() + s.size.X(), top + height},
		s.thumbColor)
}
