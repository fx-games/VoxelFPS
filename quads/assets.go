package quads

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
    "image"
    _ "image/png"
    "os"
    "path"
)

type BitmapFont struct {
    Texture *glhf.Texture
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
    fontTextureAtlas := LoadTexture(filename)
    fontTextureAtlas.SetAtlasItemSize(glyphWidth, glyphHeight)
    atlasIndex := NewBitmapFontIndexFromFile(withoutExtension + ".idx")
    return BitmapFont{Texture: fontTextureAtlas, Index: atlasIndex, UIScale: uiScale}
}

func LoadBitmapFontWithoutIndex(filename string, glyphWidth int, glyphHeight int) *glhf.Texture {
    fontTextureAtlas := LoadTexture(filename)
    fontTextureAtlas.SetAtlasItemSize(glyphWidth, glyphHeight)
    return fontTextureAtlas
}
func LoadTextureAtlas(filename string, tileWidth int, tileHeight int) *glhf.Texture {
    fontTextureAtlas := LoadTexture(filename)
    fontTextureAtlas.SetAtlasItemSize(tileWidth, tileHeight)
    return fontTextureAtlas
}
func LoadTexture(filePath string) *glhf.Texture {
    file, err := os.Open(filePath)
    if err != nil {
        panic(err)
    }
    defer file.Close()
    texture, err := glhf.NewTextureFromReader(file)
    if err != nil {
        panic(err)
    }
    texture.SetAtlasItemSize(texture.Width(), texture.Height())
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
