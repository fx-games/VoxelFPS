package eassets

import (
    "embed"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/glhf"
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

func LoadTextureAtlas(fileName string, tileWidth int, tileHeight int) *glhf.Texture {
    file, _ := OpenFile(fileName)
    atlas, _ := glhf.NewTextureFromReader(file)
    file.Close()
    atlas.SetAtlasItemSize(tileWidth, tileHeight)
    return atlas
}

func LoadBitmapFont(filename string, glyphWidth int, glyphHeight int, uiScale mgl32.Vec2) quads.BitmapFont {
    extension := path.Ext(filename)
    withoutExtension := filename[:len(filename)-len(extension)]
    fontTextureAtlas := LoadTextureAtlas(filename, glyphWidth, glyphHeight)
    indexReader, _ := OpenFile(withoutExtension + ".idx")
    defer indexReader.Close()
    atlasIndex := quads.NewBitmapFontIndexFromReader(indexReader)
    return quads.BitmapFont{Texture: fontTextureAtlas, Index: atlasIndex, UIScale: uiScale}
}

func LoadEmbeddedFont(uiScale mgl32.Vec2) quads.BitmapFont {
    return LoadBitmapFont("quadratica.png", 8, 14, uiScale)
}

func LoadEmbeddedUIAtlas() *glhf.Texture {
    return LoadTextureAtlas("ui_sheet.png", 16, 16)
}

func LoadEmbeddedTerrainAtlas() *glhf.Texture {
    return LoadTextureAtlas("basetex.png", 64, 64)
}
