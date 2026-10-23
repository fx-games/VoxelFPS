package glhf

import (
	"github.com/memmaker/voxelfps/platform/gl"
	"runtime"
)

// Frame is a fixed resolution texture that you can DrawWithOffset on.
type Frame struct {
	fb, rf, df  binder // framebuffer, read framebuffer, DrawWithOffset framebuffer
	color       *Texture
	depth       *Texture // TODO: optimization would be a separate stencil and depth buffer
	depthArray  *TextureArray
	depthLayers int
	hdr         bool // colour attachment is RGBA16F rather than RGBA8
}

// NewFrameBufferWithColorDepthStencil creates a new fully transparent Frame with given dimensions in pixels.
func NewFrameBufferWithColorDepthStencil(width, height int, smooth bool) *Frame {
	f := NewFrameBuffer()
	f.initColorStencilDepth(width, height, smooth)
	return f
}

// NewFrameBufferHDR is the same, with a half-float colour attachment so the scene can
// be rendered in linear HDR and tone mapped once at the end.
func NewFrameBufferHDR(width, height int, smooth bool) *Frame {
	f := NewFrameBuffer()
	f.hdr = true
	f.initColorStencilDepth(width, height, smooth)
	return f
}

func NewFrameBufferWithDepth(width, height, layers int) *Frame {
	f := NewFrameBuffer()
	f.depthLayers = layers
	f.initDepthBuffer(width, height)
	return f
}

func NewFrameBuffer() *Frame {
	f := &Frame{
		fb: binder{
			restoreLoc: gl.FRAMEBUFFER_BINDING,
			bindFunc: func(obj uint32) {
				gl.BindFramebuffer(gl.FRAMEBUFFER, obj)
			},
		},
		rf: binder{
			restoreLoc: gl.READ_FRAMEBUFFER_BINDING,
			bindFunc: func(obj uint32) {
				gl.BindFramebuffer(gl.READ_FRAMEBUFFER, obj)
			},
		},
		df: binder{
			restoreLoc: gl.DRAW_FRAMEBUFFER_BINDING,
			bindFunc: func(obj uint32) {
				gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, obj)
			},
		},
	}
	gl.GenFramebuffers(1, &f.fb.obj)

	runtime.SetFinalizer(f, (*Frame).delete)

	return f
}

func (f *Frame) delete() {
	CallNonBlock(func() {
		gl.DeleteFramebuffers(1, &f.fb.obj)
	})
}

// ID returns the OpenGL framebuffer ID of this Frame.
func (f *Frame) ID() uint32 {
	return f.fb.obj
}

// Begin binds the Frame. All DrawWithOffset operations will target this Frame until End is called.
func (f *Frame) Begin() {
	f.fb.bind()
}

func (f *Frame) SetDepthLayer(layer int) {
	gl.FramebufferTextureLayer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, f.depthArray.tex.obj, 0, int32(layer))
	CheckGLError("Frame.SetDepthLayer")
}

// End unbinds the Frame. All DrawWithOffset operations will go to whatever was bound before this Frame.
func (f *Frame) End() {
	f.fb.restore()
}

// Blit copies rectangle (sx0, sy0, sx1, sy1) in this Frame onto rectangle (dx0, dy0, dx1, dy1) in
// dst Frame.
//
// If the dst Frame is nil, the destination will be the framebuffer 0, which is the screen.
//
// If the sizes of the rectangles don't match, the source will be stretched to fit the destination
// rectangle. The stretch will be either smooth or pixely according to the source Frame's
// smoothness.
func (f *Frame) Blit(dst *Frame, sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1 int) {
	f.rf.obj = f.fb.obj
	if dst != nil {
		f.df.obj = dst.fb.obj
	} else {
		f.df.obj = 0
	}
	f.rf.bind()
	f.df.bind()

	filter := gl.NEAREST
	if f.color.smooth {
		filter = gl.LINEAR
	}

	gl.BlitFramebuffer(
		int32(sx0), int32(sy0), int32(sx1), int32(sy1),
		int32(dx0), int32(dy0), int32(dx1), int32(dy1),
		gl.COLOR_BUFFER_BIT, uint32(filter),
	)

	f.rf.restore()
	f.df.restore()
}

// Texture returns the Frame's underlying Texture that the Frame draws on.
func (f *Frame) Texture() *Texture {
	return f.color
}

func (f *Frame) Resize(width int, height int) {
	if f.color == nil {
		f.initDepthBuffer(width, height)
	} else {
		f.initColorStencilDepth(width, height, f.color.smooth)
	}
}

func (f *Frame) initColorStencilDepth(width int, height int, smooth bool) {
	if f.hdr {
		f.color = NewTextureHDR(width, height, smooth, gl.TEXTURE_2D)
	} else {
		f.color = NewTextureRGBA(width, height, smooth, make([]uint8, width*height*4), gl.TEXTURE_2D)
	}
	f.depth = NewStencilDepthTexture(width, height)

	f.fb.bind()
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, f.color.tex.obj, 0)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACHMENT, gl.TEXTURE_2D, f.depth.tex.obj, 0)

	f.fb.restore()

}

func (f *Frame) DepthTexture() *Texture {
	return f.depth
}

func (f *Frame) initDepthBuffer(width int, height int) {
	f.depthArray = NewDepthTextureArray(width, height, f.depthLayers, false)
	f.Begin()
	// prevent OpenGL from drawing to the color buffer
	gl.DrawBuffer(gl.NONE)
	gl.ReadBuffer(gl.NONE)

	f.End()

}

func (f *Frame) DepthTextureArray() *TextureArray {
	return f.depthArray
}
