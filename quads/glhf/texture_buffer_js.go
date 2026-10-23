//go:build js

package glhf

import (
	"runtime"

	"github.com/memmaker/voxelfps/platform/gl"
)

// TextureBuffer on js: WebGL2 has no buffer textures, so the uint16 table is a
// R16UI 2D texture texBufferWidth texels wide, and gl.ToES rewrites the shader's
// texelFetch on a usamplerBuffer to the matching 2D lookup.
type TextureBuffer struct {
	tex         binder
	textureSize int
}

const texBufferWidth = 4096

func NewTextureBuffer(pixels []uint16) *TextureBuffer {
	t := &TextureBuffer{
		tex: binder{
			restoreLoc: gl.TEXTURE_BINDING_2D,
			bindFunc:   func(obj uint32) { gl.BindTexture(gl.TEXTURE_2D, obj) },
		},
		textureSize: len(pixels),
	}
	rows := (len(pixels) + texBufferWidth - 1) / texBufferWidth
	gl.GenTextures(1, &t.tex.obj)
	t.tex.bind()
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.R16UI, texBufferWidth, int32(max(rows, 1)), 0, gl.RED_INTEGER, gl.UNSIGNED_SHORT, nil)
	t.tex.restore()
	t.SetPixels(0, pixels)
	runtime.SetFinalizer(t, (*TextureBuffer).delete)
	return t
}

func (t *TextureBuffer) delete() {
	CallNonBlock(func() { gl.DeleteTextures(1, &t.tex.obj) })
}

func (t *TextureBuffer) ID() uint32 { return t.tex.obj }

func (t *TextureBuffer) Size() int {
	if t == nil {
		return 0
	}
	return t.textureSize
}

// SetPixels writes pixels from offset on, one texture row at a time.
func (t *TextureBuffer) SetPixels(offset int, pixels []uint16) {
	t.tex.bind()
	for len(pixels) > 0 {
		x, y := offset%texBufferWidth, offset/texBufferWidth
		n := min(len(pixels), texBufferWidth-x)
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(n), 1, gl.RED_INTEGER, gl.UNSIGNED_SHORT, gl.Ptr(pixels[:n]))
		pixels, offset = pixels[n:], offset+n
	}
	t.tex.restore()
}

func (t *TextureBuffer) BeginOnUnit(unit uint32) {
	ActiveTexture(unit)
	t.Begin()
}

func (t *TextureBuffer) Begin() { t.tex.bind() }
func (t *TextureBuffer) End()   { t.tex.restore() }
