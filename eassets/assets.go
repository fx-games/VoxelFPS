package eassets

import (
    "embed"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/glhf"
    "github.com/memmaker/goray/tpack"
    "io"
    "io/fs"
    "path"
)

//go:embed files/*
var assets embed.FS

func ReadFile(filePath string) ([]byte, error) {
    return assets.ReadFile(path.Join("files", filePath))
}

func OpenFile(filePath string) (fs.File, error) {
    return assets.Open(path.Join("files", filePath))
}

func LoadTextureAtlasFixed(fileName string, tileWidth int, tileHeight int) *glhf.TextureAtlasFixed {
    file, _ := OpenFile(fileName)
    atlas, _ := glhf.NewTextureFromReader(file)
    file.Close()
    return glhf.NewTextureAtlasFixed(atlas, tileWidth, tileHeight, fileName)
}

func LoadTextureAtlasTrimmed(fileName string) *glhf.TextureAtlasPacked {
    file, _ := OpenFile(fileName)
    defer file.Close()
    atlasImage, atlasMeta, err := tpack.DecodeTrimmedAtlas(file.(io.ReadSeeker))
    if err != nil {
        panic(err)
    }
    atlas, _ := glhf.NewTextureFromImage(atlasImage)
    return glhf.NewTextureAtlasPacked(atlas, atlasMeta, fileName)
}

func LoadBitmapFont(filename string, glyphWidth int, glyphHeight int, uiScale mgl32.Vec2) quads.BitmapFont {
    extension := path.Ext(filename)
    withoutExtension := filename[:len(filename)-len(extension)]
    fontTextureAtlas := LoadTextureAtlasFixed(filename, glyphWidth, glyphHeight)
    indexReader, _ := OpenFile(withoutExtension + ".idx")
    defer indexReader.Close()
    atlasIndex := quads.NewBitmapFontIndexFromReader(indexReader)
    return quads.BitmapFont{Texture: fontTextureAtlas, Index: atlasIndex, UIScale: uiScale}
}

func LoadEmbeddedFont(uiScale mgl32.Vec2) quads.BitmapFont {
    return LoadBitmapFont("quadratica.png", 8, 14, uiScale)
}

func LoadEmbeddedUIAtlas() *glhf.TextureAtlasFixed {
    return LoadTextureAtlasFixed("ui_sheet.png", 16, 16)
}

func LoadEmbeddedTerrainAtlas() *glhf.TextureAtlasFixed {
    return LoadTextureAtlasFixed("basetex.png", 64, 64)
}
