package glhf

import (
    "runtime"

    "github.com/go-gl/gl/v3.3-core/gl"
)

// TextureBuffer is an OpenGL 3D texture.
type TextureBuffer struct {
    tex         binder
    buf         binder
    textureSize int // in pixels
    smooth      bool
}

// NewTextureBuffer creates a new 3D texture with the specified width, height, and depth,
// along with initial pixel values. The pixels must be a sequence of Red values (one byte).
func NewTextureBuffer(pixels []uint16) *TextureBuffer {
    size := len(pixels)
    tex := &TextureBuffer{
        tex: binder{
            restoreLoc: gl.TEXTURE_BINDING_BUFFER,
            bindFunc: func(obj uint32) {
                gl.BindTexture(gl.TEXTURE_BUFFER, obj)
            },
        },
        buf: binder{
            restoreLoc: gl.TEXTURE_BUFFER,
            bindFunc: func(obj uint32) {
                gl.BindBuffer(gl.TEXTURE_BUFFER, obj)
            },
        },
        textureSize: size,
    }

    gl.GenBuffers(1, &tex.buf.obj)

    tex.buf.bindOnly()
    gl.BufferData(gl.TEXTURE_BUFFER, len(pixels)*2, gl.Ptr(pixels), gl.STATIC_DRAW)
    tex.buf.restore()

    glError := gl.GetError()
    if glError != gl.NO_ERROR {
        panic(glError)
    }

    gl.GenTextures(1, &tex.tex.obj)
    tex.tex.bind()
    gl.TexBuffer(gl.TEXTURE_BUFFER, gl.R16UI, tex.buf.obj)
    tex.tex.restore()

    glError = gl.GetError()
    if glError != gl.NO_ERROR {
        panic(glError)
    }

    runtime.SetFinalizer(tex, (*TextureBuffer).delete)

    return tex
}

func (t *TextureBuffer) delete() {
    CallNonBlock(func() {
        gl.DeleteTextures(1, &t.tex.obj)
        gl.DeleteBuffers(1, &t.buf.obj)
    })
}

// ID returns the OpenGL ID of this TextureBuffer.
func (t *TextureBuffer) ID() uint32 {
    return t.tex.obj
}

func (t *TextureBuffer) Size() int {
    if t == nil {
        return 0
    }
    return t.textureSize
}

// SetPixels sets the content of a sub-region of the TextureBuffer. Pixels must be an RGBA byte sequence.
func (t *TextureBuffer) SetPixels(offset int, pixels []uint16) {
    t.buf.bind()
    gl.BufferSubData(
        gl.TEXTURE_BUFFER,
        offset*2,
        len(pixels)*2,
        gl.Ptr(pixels),
    )
    t.buf.restore()
}

func (t *TextureBuffer) BeginOnUnit(unit uint32) {
    gl.ActiveTexture(unit)
    t.Begin()
}

// Begin binds the TextureBuffer. This is necessary before using the TextureBuffer.
func (t *TextureBuffer) Begin() {
    t.tex.bind()

}

// End unbinds the TextureBuffer and restores the previous one.
func (t *TextureBuffer) End() {

    t.tex.restore()
}
