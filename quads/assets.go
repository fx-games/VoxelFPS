package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/tpack"
    "image"
    _ "image/png"
    "io"
    "os"
    "path"
)

type BitmapFont struct {
    Texture *glhf.TextureAtlasFixed
    Index   BitmapFontIndex
    UIScale mgl32.Vec2
}

func (t BitmapFont) MeasureString(text []string, scale mgl32.Vec2, padChar, padLine float32) (int, int) {
    cW, cH := t.Texture.GetAtlasItemSize()
    scaleX := scale.X() // * t.UIScale.X()
    scaleY := scale.Y() // * t.UIScale.Y()
    charWidth := float32(cW) * scaleX
    charHeight := float32(cH) * scaleY
    charsX, charsY := getDimensions(text)
    tWidth := (float32(charsX) * (charWidth + float32(padChar))) - float32(padChar)
    tHeight := (float32(charsY) * (charHeight + float32(padLine))) - float32(padLine)
    return int(tWidth), int(tHeight)
}

// LoadBitmapFont loads a bitmap font from a texture and an index file.
// Expects the .ixd file to be in the same directory as the texture file.
func LoadBitmapFont(filename string, glyphWidth int, glyphHeight int, uiScale mgl32.Vec2) BitmapFont {
    extension := path.Ext(filename)
    withoutExtension := filename[:len(filename)-len(extension)]
    fontTextureAtlas := LoadTextureAtlasFixed(filename, glyphWidth, glyphHeight)
    atlasIndex := NewBitmapFontIndexFromFile(withoutExtension + ".idx")
    return BitmapFont{Texture: fontTextureAtlas, Index: atlasIndex, UIScale: uiScale}
}

func LoadBitmapFontWithoutIndex(filename string, glyphWidth int, glyphHeight int) *glhf.TextureAtlasFixed {
    fontTextureAtlas := LoadTextureAtlasFixed(filename, glyphWidth, glyphHeight)
    return fontTextureAtlas
}
func LoadTextureAtlasFixed(filename string, tileWidth int, tileHeight int) *glhf.TextureAtlasFixed {
    return glhf.NewTextureAtlasFixed(LoadTexture(filename), tileWidth, tileHeight, filename)
}

func NewOneWhitePixelTexture() *glhf.Texture {
    img := image.NewRGBA(image.Rect(0, 0, 1, 1))
    img.Set(0, 0, image.White)
    texture, err := glhf.NewTextureFromImage(img)
    if err != nil {
        panic(err)
    }
    return texture
}

func LoadTextureAtlasPacked(fileName string) *glhf.TextureAtlasPacked {
    file, err := os.Open(fileName)
    if err != nil {
        panic(err)
    }
    defer file.Close()
    return ReadTextureAtlasPacked(fileName, file)
}

func ReadTextureAtlasPacked(fileName string, file io.ReadSeeker) *glhf.TextureAtlasPacked {
    atlasImage, atlasMeta, err := tpack.DecodeTrimmedAtlas(file)
    if err != nil {
        panic(err)
    }
    atlas, _ := glhf.NewTextureFromImage(atlasImage)
    return glhf.NewTextureAtlasPacked(atlas, atlasMeta, fileName)
}

func LoadTexture(filePath string) *glhf.Texture {
    return loadTextureWithTarget(filePath, gl.TEXTURE_2D)
}
func loadTextureWithTarget(filePath string, textureTarget uint32) *glhf.Texture {
    file, err := os.Open(filePath)
    if err != nil {
        panic(err)
    }
    defer file.Close()
    texture, err := glhf.NewTextureFromReaderWithTarget(file, textureTarget)
    if err != nil {
        panic(err)
    }
    return texture
}

// LoadCubemap loads a cubemap from 6 files.
// The order of the files should be:
// 0: right
// 1: left
// 2: up
// 3: down
// 4: back
// 5: front
// If the skybox looks weird, try to change the order of the files.
// Left/Right and Front/Back might be swapped.
// If Top/Bottom look weird, try to rotate the source image 90 degrees.
func LoadCubemap(filePath [6]string) *glhf.Cubemap {
    var pixels [6][]uint8
    var width, height int

    for idx, filename := range filePath {
        file, err := os.Open(filename)
        if err != nil {
            panic(err)
        }
        img, _, err := image.Decode(file)
        if err != nil {
            return nil
        }
        nrgba := glhf.AsRGBA(img)
        if width == 0 {
            width = nrgba.Bounds().Dx()
            height = nrgba.Bounds().Dy()
        }

        pixels[idx] = nrgba.Pix
        file.Close()
    }

    return glhf.NewCubemap(width, height, pixels)
}

func LoadImage(filePath string) image.Image {
    file, err := os.Open(filePath)
    if err != nil {
        panic(err)
    }
    defer file.Close()
    img, _, err := image.Decode(file)
    if err != nil {
        return nil
    }
    return img
}
