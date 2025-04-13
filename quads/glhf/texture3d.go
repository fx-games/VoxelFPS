package glhf

import (
    "runtime"

    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
)

// Texture3D is an OpenGL 3D texture.
type Texture3D struct {
    tex                                       binder
    textureWidth, textureHeight, textureDepth int // in pixels
    smooth                                    bool
}

// NewTexture3D creates a new 3D texture with the specified width, height, and depth,
// along with initial pixel values. The pixels must be a sequence of Red values (one byte).
func NewTexture3D(width, height, depth int, smooth bool, pixels []uint8) *Texture3D {
    tex := &Texture3D{
        tex: binder{
            restoreLoc: gl.TEXTURE_BINDING_3D,
            bindFunc: func(obj uint32) {
                gl.BindTexture(gl.TEXTURE_3D, obj)
            },
        },
        textureWidth:  width,
        textureHeight: height,
        textureDepth:  depth,
    }

    gl.GenTextures(1, &tex.tex.obj)

    tex.Begin()
    defer tex.End()

    // Initialize texture data
    gl.TexImage3D(
        gl.TEXTURE_3D,
        0,
        gl.R8UI,
        int32(width),
        int32(height),
        int32(depth),
        0,
        gl.RED_INTEGER,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )

    glError := gl.GetError()
    if glError != gl.NO_ERROR {
        panic(glError)
    }
    borderColor := mgl32.Vec4{0, 0, 0, 0}
    gl.TexParameterfv(gl.TEXTURE_3D, gl.TEXTURE_BORDER_COLOR, &borderColor[0])
    gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_BORDER)
    gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_BORDER)
    gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_BORDER)

    tex.SetSmooth(smooth)

    runtime.SetFinalizer(tex, (*Texture3D).delete)

    return tex
}

func (t *Texture3D) delete() {
    CallNonBlock(func() {
        gl.DeleteTextures(1, &t.tex.obj)
    })
}

// ID returns the OpenGL ID of this Texture3D.
func (t *Texture3D) ID() uint32 {
    return t.tex.obj
}

// Width returns the width of the Texture3D in pixels.
func (t *Texture3D) Width() int {
    return t.textureWidth
}

// Height returns the height of the Texture3D in pixels.
func (t *Texture3D) Height() int {
    return t.textureHeight
}

// Depth returns the depth of the Texture3D in pixels.
func (t *Texture3D) Depth() int {
    return t.textureDepth
}

// SetPixels sets the content of a sub-region of the Texture3D. Pixels must be an RGBA byte sequence.
func (t *Texture3D) SetPixels(x, y, z, w, h, d int, pixels []uint8) {
    if len(pixels) != w*h*d*4 {
        panic("set pixels: wrong number of pixels")
    }
    gl.TexSubImage3D(
        gl.TEXTURE_3D,
        0,
        int32(x),
        int32(y),
        int32(z),
        int32(w),
        int32(h),
        int32(d),
        gl.RED_INTEGER,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )
}

// SetSmooth sets whether the Texture3D should be drawn "smoothly" or "pixely".
func (t *Texture3D) SetSmooth(smooth bool) {
    t.smooth = smooth
    if smooth {
        gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
        gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    } else {
        gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
        gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
    }
}

// Smooth returns whether the Texture3D is set to be drawn "smooth" or "pixely".
func (t *Texture3D) Smooth() bool {
    return t.smooth
}

func (t *Texture3D) BeginOnUnit(unit uint32) {
    gl.ActiveTexture(unit)
    t.Begin()
}

// Begin binds the Texture3D. This is necessary before using the Texture3D.
func (t *Texture3D) Begin() {
    t.tex.bind()
}

// End unbinds the Texture3D and restores the previous one.
func (t *Texture3D) End() {
    t.tex.restore()
}
