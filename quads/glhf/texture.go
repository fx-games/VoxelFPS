package glhf

import (
    "fmt"
    "image"
    "image/color"
    "image/draw"
    "image/png"
    "io"
    "math"
    "os"
    "runtime"

    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
)

// Texture is an OpenGL texture.
type Texture struct {
    tex                         binder
    textureWidth, textureHeight int // in pixels
    smooth                      bool

    itemSizeX    int // textureWidth of each item in the atlas in pixels
    itemSizeY    int // textureHeight of each item in the atlas in pixels
    itemPaddingX int
    itemPaddingY int
    itemMarginX  int
    itemMarginY  int
}

// NewTextureFromReader creates a new texture from an io.Reader.
func NewTextureFromReader(r io.Reader) (*Texture, error) {
    img, _, err := image.Decode(r)
    if err != nil {
        return nil, err
    }
    nrgba := AsRGBA(img)

    var texture *Texture
    texture = NewTexture(
        nrgba.Bounds().Dx(),
        nrgba.Bounds().Dy(),
        false,
        nrgba.Pix,
    )
    return texture, nil
}

func AsRGBA(img image.Image) *image.RGBA {
    switch typedImage := img.(type) {
    case *image.RGBA:
        return typedImage
    default:
        bounds := typedImage.Bounds()
        nrgba := image.NewRGBA(bounds)
        draw.Draw(nrgba, bounds, typedImage, bounds.Min, draw.Src)
        return nrgba
    }
}

func NewRectOutlineTexture(width, height, borderWidth int) *Texture {
    var borderColor = [4]uint8{255, 255, 255, 255}
    var pixels []uint8
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            if x < borderWidth || x >= width-borderWidth || y < borderWidth || y >= height-borderWidth {
                pixels = append(pixels, borderColor[:]...)
            } else {
                pixels = append(pixels, 0, 0, 0, 0)
            }
        }
    }
    return NewTexture(width, height, false, pixels)
}

// NewTexture creates a new texture with the specified width and height with some initial
// pixel values. The pixels must be a sequence of RGBA values (one byte per component).
func NewTexture(width, height int, smooth bool, pixels []uint8) *Texture {
    tex := &Texture{
        tex: binder{
            restoreLoc: gl.TEXTURE_BINDING_2D,
            bindFunc: func(obj uint32) {
                gl.BindTexture(gl.TEXTURE_2D, obj)
            },
        },
        textureWidth:  width,
        textureHeight: height,
    }

    gl.GenTextures(1, &tex.tex.obj)

    tex.Begin()
    defer tex.End()

    // initial data
    gl.TexImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA,
        int32(width),
        int32(height),
        0,
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )

    borderColor := mgl32.Vec4{0, 0, 0, 0}
    gl.TexParameterfv(gl.TEXTURE_2D, gl.TEXTURE_BORDER_COLOR, &borderColor[0])
    gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_BORDER)
    gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_BORDER)

    tex.SetSmooth(smooth)

    runtime.SetFinalizer(tex, (*Texture).delete)

    return tex
}

func (t *Texture) delete() {
    CallNonBlock(func() {
        gl.DeleteTextures(1, &t.tex.obj)
    })
}

// ID returns the OpenGL ID of this Texture.
func (t *Texture) ID() uint32 {
    return t.tex.obj
}

// Width returns the width of the Texture in pixels.
func (t *Texture) Width() int {
    return t.textureWidth
}

// Height returns the height of the Texture in pixels.
func (t *Texture) Height() int {
    return t.textureHeight
}

// SetPixels sets the content of a sub-region of the Texture. Pixels must be an RGBA byte sequence.
func (t *Texture) SetPixels(x, y, w, h int, pixels []uint8) {
    if len(pixels) != w*h*4 {
        panic("set pixels: wrong number of pixels")
    }
    gl.TexSubImage2D(
        gl.TEXTURE_2D,
        0,
        int32(x),
        int32(y),
        int32(w),
        int32(h),
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )
}

// Pixels returns the content of a sub-region of the Texture as an RGBA byte sequence.
func (t *Texture) Pixels(x, y, w, h int) []uint8 {
    pixels := make([]uint8, t.textureWidth*t.textureHeight*4)
    gl.GetTexImage(
        gl.TEXTURE_2D,
        0,
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        gl.Ptr(pixels),
    )
    subPixels := make([]uint8, w*h*4)
    for i := 0; i < h; i++ {
        row := pixels[(i+y)*t.textureWidth*4+x*4 : (i+y)*t.textureWidth*4+(x+w)*4]
        subRow := subPixels[i*w*4 : (i+1)*w*4]
        copy(subRow, row)
    }
    return subPixels
}

// AllPixels returns the content of the whole Texture as an RGBA byte sequence.
// NOTE: Will bind and unbind the texture. Needs OpenGL context.
func (t *Texture) AllPixels() []uint8 {
    t.tex.bind()
    pixels := t.Pixels(0, 0, t.textureWidth, t.textureHeight)
    t.tex.restore()
    return pixels
}

// SetSmooth sets whether the Texture should be drawn "smoothly" or "pixely".
//
// It affects how the Texture is drawn when zoomed. Smooth interpolates between the neighbour
// pixels, while pixely always chooses the nearest pixel.
func (t *Texture) SetSmooth(smooth bool) {
    t.smooth = smooth
    if smooth {
        gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
        gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    } else {
        gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
        gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
    }
}

// Smooth returns whether the Texture is set to be drawn "smooth" or "pixely".
func (t *Texture) Smooth() bool {
    return t.smooth
}

func (t *Texture) BeginOnUnit(unit uint32) {
    gl.ActiveTexture(unit)
    t.Begin()
}

// Begin binds the Texture. This is necessary before using the Texture.
func (t *Texture) Begin() {
    t.tex.bind()
}

// End unbinds the Texture and restores the previous one.
func (t *Texture) End() {
    t.tex.restore()
}

func (t *Texture) GetUV(index uint16) (GlFloat, GlFloat, GlFloat, GlFloat) {
    tileWidthInPx := t.itemSizeX
    tileHeightInPx := t.itemSizeY

    xCount := int(t.textureWidth / tileWidthInPx)

    leftBorder := t.itemMarginX + ((int(index) % xCount) * (tileWidthInPx + t.itemPaddingX))
    topBorder := t.itemMarginY + ((int(index) / xCount) * (tileHeightInPx + t.itemPaddingY))

    topLeftU := GlFloat(leftBorder) / GlFloat(t.textureWidth)
    topLeftV := GlFloat(topBorder) / GlFloat(t.textureHeight)

    bottomRightU := GlFloat(leftBorder+tileWidthInPx) / GlFloat(t.textureWidth)
    bottomRightV := (GlFloat(topBorder+tileHeightInPx) / GlFloat(t.textureHeight)) - 0.0001

    return topLeftU, topLeftV, bottomRightU, bottomRightV
}

func (t *Texture) SetAtlasItemSize(width int, height int) {
    t.itemSizeX = width
    t.itemSizeY = height
}

func (t *Texture) SetPaddingBetweenItems(x, y int) {
    t.itemPaddingX = x
    t.itemPaddingY = y
}

func (t *Texture) GetAtlasItemSize() (int, int) {
    return t.itemSizeX, t.itemSizeY
}

func (t *Texture) SetAtlasMargin(x int, y int) {
    t.itemMarginX = x
    t.itemMarginY = y
}

func (t *Texture) SaveAsPNG(filename string) {
    t.tex.bind()
    pixels := t.Pixels(0, 0, t.textureWidth, t.textureHeight)
    t.tex.restore()

    outImage := image.NewNRGBA(image.Rect(0, 0, t.textureWidth, t.textureHeight))
    for y := 0; y < t.textureHeight; y++ {
        for x := 0; x < t.textureWidth; x++ {
            outImage.Set(x, y, color.NRGBA{
                R: pixels[(y*t.textureWidth+x)*4],
                G: pixels[(y*t.textureWidth+x)*4+1],
                B: pixels[(y*t.textureWidth+x)*4+2],
                A: pixels[(y*t.textureWidth+x)*4+3],
            })
        }
    }

    file, err := os.Create(filename)
    if err != nil {
        println(fmt.Sprintf("could not create %s", filename))
    }
    err = png.Encode(file, outImage)
    if err != nil {
        println(fmt.Sprintf("could not encode %s", filename))
    }
    file.Close()
}

func (t *Texture) GridWidth() float32 {
    return float32(t.textureWidth) / float32(t.itemSizeX)
}

func (t *Texture) GridHeight() float32 {
    return float32(t.textureHeight) / float32(t.itemSizeY)
}

func (t *Texture) AABBFromOpaque() (minCoords mgl32.Vec3, maxCoords mgl32.Vec3) {
    pixels := t.AllPixels()
    return aabbFromOpaquePixels(pixels, t.textureWidth, t.textureHeight)
}

func (t *Texture) AABBFromOpaqueAtIndex(index uint16) (mgl32.Vec3, mgl32.Vec3) {
    topLeftU, topLeftV, bottomRightU, bottomRightV := t.GetUV(index)

    minX := int(float32(topLeftU) * float32(t.textureWidth))
    minY := int(float32(topLeftV) * float32(t.textureHeight))
    maxX := int(float32(bottomRightU) * float32(t.textureWidth))
    maxY := int(float32(bottomRightV) * float32(t.textureHeight))

    w := maxX - minX
    h := maxY - minY
    t.tex.bind()
    pixels := t.Pixels(minX, minY, w, h)
    t.tex.restore()
    return aabbFromOpaquePixels(pixels, w, h)
}

func (t *Texture) HasIndex(index int) bool {
    return index < int(t.GridWidth()*t.GridHeight())
}

func (t *Texture) SetPixelsAtIndex(index uint16, pixels []uint8) {
    topLeftU, topLeftV, _, _ := t.GetUV(index)

    w := t.itemSizeX
    h := t.itemSizeY

    minX := int(float32(topLeftU) * float32(t.textureWidth))
    minY := int(float32(topLeftV) * float32(t.textureHeight))

    t.tex.bind()
    t.SetPixels(minX, minY, w, h, pixels)
    t.tex.restore()
    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
}

func ExpandToIndex(t *Texture, index int) *Texture {
    newWidth := t.textureWidth
    gridWidth := t.GridWidth()
    isVertExpansion := true
    if gridWidth == 1 {
        newWidth = t.textureWidth * 8
        gridWidth = 8
        isVertExpansion = false
    }

    newGridHeight := int(math.Ceil(float64(float32(index) / gridWidth)))
    newHeight := newGridHeight * t.itemSizeY
    newPixels := make([]uint8, newWidth*newHeight*4)

    sourcePixels := t.AllPixels()

    if isVertExpansion {
        copy(newPixels, sourcePixels)
    } else { // copy horizontally
        for y := 0; y < t.textureHeight; y++ {
            for x := 0; x < t.textureWidth; x++ {
                srcIndex := (y*t.textureWidth + x) * 4
                destIndex := (y*newWidth + x) * 4
                copy(newPixels[destIndex:destIndex+4], sourcePixels[srcIndex:srcIndex+4])
            }
        }
    }

    texture := NewTexture(newWidth, newHeight, t.smooth, newPixels)
    texture.SetAtlasItemSize(t.itemSizeX, t.itemSizeY)
    return texture
}

func aabbFromOpaquePixels(pixels []uint8, textureWidth, textureHeight int) (mgl32.Vec3, mgl32.Vec3) {
    // find the first non-transparent pixel on each side
    minX := textureWidth
    minY := textureHeight

    maxX := 0

    for y := 0; y < textureHeight; y++ {
        for x := 0; x < textureWidth; x++ {
            if pixels[(y*textureWidth+x)*4+3] > 0 {
                if x < minX {
                    minX = x
                }
                if x > maxX {
                    maxX = x
                }
                if y < minY {
                    minY = y
                }
            }
        }
    }

    normalizedMinX := (float32(minX) / float32(textureWidth)) - 0.5
    normalizedMaxX := (float32(maxX) / float32(textureWidth)) - 0.5
    normalizedMaxY := 1 - (float32(minY) / float32(textureHeight))

    return mgl32.Vec3{normalizedMinX, 0, -0.05}, mgl32.Vec3{normalizedMaxX, normalizedMaxY, 0.05}
}

func SetPixel(pixels []uint8, x, y, width int, color [4]uint8) {
    idx := 4 * (y*width + x)
    pixels[idx] = color[0]
    pixels[idx+1] = color[1]
    pixels[idx+2] = color[2]
    pixels[idx+3] = color[3]
}

func GetPixel(pixels []uint8, x, y, width int) [4]uint8 {
    idx := 4 * (y*width + x)
    return [4]uint8{pixels[idx], pixels[idx+1], pixels[idx+2], pixels[idx+3]}
}
