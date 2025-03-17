package glhf

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "runtime"
)

type Cubemap struct {
    tex           binder
    textureWidth  int
    textureHeight int
}

// NewCubemap creates a new texture with the specified width and height with some initial
// pixel values. The pixels must be a sequence of RGB values (one byte per component).
func NewCubemap(width, height int, pixels [6][]uint8) *Cubemap {
    tex := &Cubemap{
        tex: binder{
            restoreLoc: gl.TEXTURE_BINDING_CUBE_MAP,
            bindFunc: func(obj uint32) {
                gl.BindTexture(gl.TEXTURE_CUBE_MAP, obj)
            },
        },
        textureWidth:  width,
        textureHeight: height,
    }

    gl.GenTextures(1, &tex.tex.obj)

    tex.Begin()
    defer tex.End()

    for i := 0; i < 6; i++ {
        gl.TexImage2D(
            gl.TEXTURE_CUBE_MAP_POSITIVE_X+uint32(i),
            0,
            gl.RGBA,
            int32(width),
            int32(height),
            0,
            gl.RGBA,
            gl.UNSIGNED_BYTE,
            gl.Ptr(pixels[i]),
        )
    }

    gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_EDGE)

    runtime.SetFinalizer(tex, (*Cubemap).delete)

    return tex
}

// Begin binds the Texture. This is necessary before using the Texture.
func (t *Cubemap) Begin() {
    t.tex.bind()
}

// End unbinds the Texture and restores the previous one.
func (t *Cubemap) End() {
    t.tex.restore()
}

func (t *Cubemap) delete() {
    CallNonBlock(func() {
        gl.DeleteTextures(1, &t.tex.obj)
    })
}

// ID returns the OpenGL ID of this Texture.
func (t *Cubemap) ID() uint32 {
    return t.tex.obj
}
