package quads

import (
	"bytes"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/memmaker/voxelfps/tpack"
	"image"
	"image/color"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
)

// Assets loads game assets from any file system. Use Disk for files next to the
// executable, or wrap an embed.FS for baked-in assets.
type Assets struct {
	fs.FS
}

// Disk reads assets from the working directory.
var Disk = Assets{os.DirFS(".")}

func (a Assets) open(filePath string) fs.File {
	file, err := a.Open(filePath)
	if err != nil {
		panic("could not open asset " + filePath + ": " + err.Error())
	}
	return file
}

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

// BitmapFont loads a bitmap font from a texture and an index file.
// Expects the .idx file to be next to the texture file.
func (a Assets) BitmapFont(filename string, glyphWidth int, glyphHeight int, uiScale mgl32.Vec2) BitmapFont {
	withoutExtension := filename[:len(filename)-len(path.Ext(filename))]
	indexFile := a.open(withoutExtension + ".idx")
	defer indexFile.Close()
	return BitmapFont{
		Texture: a.TextureAtlasFixed(filename, glyphWidth, glyphHeight),
		Index:   NewBitmapFontIndexFromReader(indexFile),
		UIScale: uiScale,
	}
}

func (a Assets) TextureAtlasFixed(filename string, tileWidth int, tileHeight int) *glhf.TextureAtlasFixed {
	return glhf.NewTextureAtlasFixed(a.Texture(filename), tileWidth, tileHeight, filename)
}

// TextureAtlasPacked loads a trimmed/packed atlas plus its metadata.
func (a Assets) TextureAtlasPacked(fileName string) *glhf.TextureAtlasPacked {
	file := a.open(fileName)
	defer file.Close()
	seeker, isSeekable := file.(io.ReadSeeker)
	if !isSeekable {
		data, err := io.ReadAll(file)
		if err != nil {
			panic("could not read atlas " + fileName + ": " + err.Error())
		}
		seeker = bytes.NewReader(data)
	}
	return ReadTextureAtlasPacked(fileName, seeker)
}

func ReadTextureAtlasPacked(fileName string, file io.ReadSeeker) *glhf.TextureAtlasPacked {
	atlasImage, atlasMeta, err := tpack.DecodeTrimmedAtlas(file)
	if err != nil {
		panic(err)
	}
	atlas, err := glhf.NewTextureFromImage(atlasImage)
	if err != nil {
		panic(err)
	}
	return glhf.NewTextureAtlasPacked(atlas, atlasMeta, fileName)
}

// Texture loads a texture, or a magenta placeholder if the file is missing or
// will not decode. A texture is data a user can edit, move or break while the
// game is running - the editor reloads them - so one bad PNG has to be a wall
// that is visibly wrong, not a dead process. Structural assets that are only
// read at startup (font indices, atlas metadata, shaders) still panic.
func (a Assets) Texture(filePath string) *glhf.Texture {
	file, err := a.Open(filePath)
	if err != nil {
		log.Printf("[Assets] could not open texture %s: %v - using placeholder", filePath, err)
		return NewPlaceholderTexture()
	}
	defer file.Close()
	texture, err := glhf.NewTextureFromReader(file)
	if err != nil {
		log.Printf("[Assets] could not decode texture %s: %v - using placeholder", filePath, err)
		return NewPlaceholderTexture()
	}
	return texture
}

// Image follows Texture: a file that is missing or is not an image gives a magenta
// placeholder, not a dead process. One of its callers is the atlas picker, which
// decodes whatever the player dropped on the window - a text file dropped by
// accident used to end the session.
func (a Assets) Image(filePath string) image.Image {
	file, err := a.Open(filePath)
	if err != nil {
		log.Printf("[Assets] could not open image %s: %v - using placeholder", filePath, err)
		return placeholderImage()
	}
	defer file.Close()
	return decodeImage(file, filePath)
}

// ImageFromOSPath reads an image by an operating system path instead of a path
// inside the asset tree. Disk is rooted at the working directory and an fs.FS
// refuses an absolute path outright, so every file dragged onto the window -
// which is the one case where the path comes from outside the tree - used to
// decode as a magenta placeholder however good the PNG was.
func ImageFromOSPath(filePath string) image.Image {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[Assets] could not open image %s: %v - using placeholder", filePath, err)
		return placeholderImage()
	}
	defer file.Close()
	return decodeImage(file, filePath)
}

func decodeImage(file io.Reader, filePath string) image.Image {
	img, _, err := image.Decode(file)
	if err != nil {
		log.Printf("[Assets] could not decode image %s: %v - using placeholder", filePath, err)
		return placeholderImage()
	}
	return img
}

// placeholderImage is the magenta of NewPlaceholderTexture, before it is uploaded.
func placeholderImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	return img
}

// Cubemap loads a cubemap from 6 files.
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
func (a Assets) Cubemap(filePath [6]string) *glhf.Cubemap {
	var pixels [6][]uint8
	var width, height int

	for idx, filename := range filePath {
		nrgba := glhf.AsRGBA(a.Image(filename))
		if width == 0 {
			width = nrgba.Bounds().Dx()
			height = nrgba.Bounds().Dy()
		}
		pixels[idx] = nrgba.Pix
	}

	return glhf.NewCubemap(width, height, pixels)
}

func NewOneWhitePixelTexture() *glhf.Texture {
	return newSolidTexture(color.White)
}

// NewPlaceholderTexture is the stand-in for a texture that failed to load.
// Magenta because nothing in the art is magenta, so it reads as "missing" at a
// glance instead of looking like a dark or untextured surface.
func NewPlaceholderTexture() *glhf.Texture {
	return newSolidTexture(color.RGBA{R: 255, G: 0, B: 255, A: 255})
}

func newSolidTexture(fill color.Color) *glhf.Texture {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, fill)
	texture, err := glhf.NewTextureFromImage(img)
	if err != nil {
		panic(err) // a 1x1 image in memory: this is a broken GL context, not bad data
	}
	return texture
}
