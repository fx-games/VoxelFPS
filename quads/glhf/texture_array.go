package glhf

import (
    "runtime"

    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
)

// TextureArray is an OpenGL 3D texture.
type TextureArray struct {
    tex                                        binder
    textureWidth, textureHeight, textureLayers int // in pixels
    smooth                                     bool
}

// NewTextureArray creates a new array of 2D textures with the specified width, height, and layers,
// along with initial pixel values. The pixels must be a sequence of Red values (16bits).
func NewTextureArray(width, height, layers int, smooth bool, pixels []uint8) *TextureArray {
    tex := &TextureArray{
        tex: binder{
            restoreLoc: gl.TEXTURE_BINDING_2D_ARRAY,
            bindFunc: func(obj uint32) {
                gl.BindTexture(gl.TEXTURE_2D_ARRAY, obj)
            },
        },
        textureWidth:  width,
        textureHeight: height,
        textureLayers: layers,
    }

    gl.GenTextures(1, &tex.tex.obj)

    tex.Begin()
    defer tex.End()

    // Initialize texture data
    gl.TexImage3D(
        gl.TEXTURE_2D_ARRAY,
        0,
        gl.RGBA8UI,
        int32(width),
        int32(height),
        int32(layers),
        0,
        gl.RGBA_INTEGER,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )

    glError := gl.GetError()
    if glError != gl.NO_ERROR {
        panic(glError)
    }
    borderColor := mgl32.Vec4{0, 0, 0, 0}
    gl.TexParameterfv(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_BORDER_COLOR, &borderColor[0])
    gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_BORDER)
    gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_BORDER)
    gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_BORDER)

    tex.SetSmooth(smooth)

    runtime.SetFinalizer(tex, (*TextureArray).delete)

    return tex
}

func (t *TextureArray) delete() {
    CallNonBlock(func() {
        gl.DeleteTextures(1, &t.tex.obj)
    })
}

// ID returns the OpenGL ID of this Texture3D.
func (t *TextureArray) ID() uint32 {
    return t.tex.obj
}

// Width returns the width of the Texture3D in pixels.
func (t *TextureArray) Width() int {
    return t.textureWidth
}

// Height returns the height of the Texture3D in pixels.
func (t *TextureArray) Height() int {
    return t.textureHeight
}

// Depth returns the depth of the Texture3D in pixels.
func (t *TextureArray) Depth() int {
    return t.textureLayers
}

// SetPixels sets the content of a sub-region of the Texture3D. Pixels must be an RGBA byte sequence.
func (t *TextureArray) SetPixels(x, y, w, h, layer int, pixels []uint8) {
    d := 1
    if len(pixels) != w*h*d {
        panic("set pixels: wrong number of pixels")
    }
    gl.TexSubImage3D(
        gl.TEXTURE_2D_ARRAY,
        0,
        int32(x),
        int32(y),
        int32(layer),
        int32(w),
        int32(h),
        int32(d),
        gl.RGBA_INTEGER,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )
}

// SetSmooth sets whether the Texture3D should be drawn "smoothly" or "pixely".
func (t *TextureArray) SetSmooth(smooth bool) {
    t.smooth = smooth
    if smooth {
        gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
        gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    } else {
        gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
        gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
    }
}

// Smooth returns whether the Texture3D is set to be drawn "smooth" or "pixely".
func (t *TextureArray) Smooth() bool {
    return t.smooth
}

func (t *TextureArray) BeginOnUnit(unit uint32) {
    gl.ActiveTexture(unit)
    t.Begin()
}

// Begin binds the Texture3D. This is necessary before using the Texture3D.
func (t *TextureArray) Begin() {
    t.tex.bind()
}

// End unbinds the Texture3D and restores the previous one.
func (t *TextureArray) End() {
    t.tex.restore()
}
