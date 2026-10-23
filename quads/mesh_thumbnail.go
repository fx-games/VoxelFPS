package quads

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxelfps/quads/glhf"
)

// ThumbSize is how big one model's preview is, in pixels. It matches the texture
// library's tile so the mesh browser and the texture browser lay out the same
// grid, at the same size, and the picker widget can draw either without knowing
// which it has.
const ThumbSize = 64

// thumbViewDirection is where the preview is looked at from: down and across
// both, so a chair reads as a chair rather than as a square. It is normalised in
// MeshThumbnail, so these are only proportions - three quarters of the way round
// and a little above.
var thumbViewDirection = mgl32.Vec3{1, 0.8, 1}

// thumbMargin is how much bigger than the model the picture is, so nothing is
// clipped by its own silhouette touching the edge of the cell.
const thumbMargin = 1.08

// MeshThumbnail draws one model on its own, off screen, and hands back the
// picture - what the mesh browser puts in a cell so a piece of scenery is chosen
// by looking at it rather than by reading a folder name.
//
// It is the ordinary mesh pass with the scene taken away: the model at the
// origin, an orthographic camera framed on its own bounds, and flat lighting.
// Flat because the alternative is the map's lighting, which is wherever the
// level's lamps happen to be - the same chair would come out black in a corridor
// and blown out under a lamp, and a preview has to look the same every time.
//
// ponytail: rendered on every visit to a directory, and cached only by
// MeshDrawer already holding the model. Write the picture next to the mesh if
// opening a folder of eighty ever hitches.
func (a *GlApplication) MeshThumbnail(meshFile string) *image.RGBA {
	mesh, bounds := a.Meshes.Mesh(meshFile)
	centre := bounds.Center()
	direction := thumbViewDirection.Normalize()
	radius := thumbRadius(bounds.Extents(), direction)
	if radius <= 0 {
		radius = 1 // a model with no extent at all, rather than a divide by zero
	}

	if a.thumbBuffer == nil {
		a.thumbBuffer = glhf.NewFrameBufferWithColorDepthStencil(ThumbSize, ThumbSize, false)
	}

	// Twice the radius back is far enough for the near plane to clear the model
	// without the far plane cutting it; with an orthographic projection the
	// distance changes nothing else.
	eye := centre.Add(direction.Mul(radius * 2))
	view := mgl32.LookAtV(eye, centre, mgl32.Vec3{0, 1, 0})
	// Orthographic, not perspective: a preview is a picture of the shape, and a
	// wide-angle lens two model-widths away bends every straight edge in it.
	projection := mgl32.Ortho(-radius, radius, -radius, radius, 0.01, radius*4)

	// The three bits of scene state the mesh shader reads that have nothing to do
	// with this model, put back at the end: what the level is lit by, how far the
	// fog reaches, and where the camera is.
	unmute := a.lightsUbo.MuteLights()
	a.SetAmbientLight(mgl32.Vec3{1, 1, 1}, 1)
	sun := a.sun
	a.SetSunLight(sun.Vec3(), 0)
	defer func() {
		unmute()
		a.SetAmbientLight(a.ambient.Vec3(), a.ambient.W())
		a.SetSunLight(sun.Vec3(), sun.W())
		a.updateMainUBO()
	}()

	a.thumbBuffer.Begin()
	gl.Viewport(0, 0, ThumbSize, ThumbSize)
	// Cleared transparent so the cell shows the picker's own background around
	// the model instead of a black square per piece.
	gl.ClearColor(0, 0, 0, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	a.setMainUBOCam(eye, centre.Sub(eye).Normalize(), projection.Mul4(view))
	a.meshShader.Begin()
	mesh.RootNode.SetParent(nil)
	mesh.Draw()
	a.meshShader.End()

	pixels := make([]uint8, ThumbSize*ThumbSize*4)
	gl.ReadPixels(0, 0, ThumbSize, ThumbSize, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	a.thumbBuffer.End()
	gl.Viewport(0, 0, int32(a.FramebufferWidth), int32(a.FramebufferHeight))

	return flippedRGBA(pixels, ThumbSize, ThumbSize)
}

// thumbRadius is half the side of the square the model needs on screen, looked at
// from direction. It is the box's own extents measured along the two axes across
// the view - the widest the silhouette can be and the tallest it can be - rather
// than the box's diagonal, which is what a corner of it would measure and leaves
// a third of every cell empty around a piece of furniture that has no corner
// pointing at the camera.
func thumbRadius(extents, direction mgl32.Vec3) float32 {
	up := mgl32.Vec3{0, 1, 0}
	right := direction.Cross(up).Normalize()
	across := func(axis mgl32.Vec3) float32 {
		return (absF(axis.X())*extents.X() + absF(axis.Y())*extents.Y() + absF(axis.Z())*extents.Z()) * 0.5
	}
	return max(across(right), across(right.Cross(direction).Normalize())) * thumbMargin
}

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// flippedRGBA turns a block of pixels read off the GPU into an image. GL counts
// rows from the bottom and image.RGBA counts them from the top, so the rows are
// copied back to front - without it every preview is upside down.
func flippedRGBA(pixels []uint8, width, height int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	stride := width * 4
	for y := 0; y < height; y++ {
		source := (height - 1 - y) * stride
		copy(out.Pix[y*out.Stride:y*out.Stride+stride], pixels[source:source+stride])
	}
	return out
}
