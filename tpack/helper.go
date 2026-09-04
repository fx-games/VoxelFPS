package tpack

import (
	"bytes"
	"github.com/memmaker/goray/png_chunks"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
)

func writeImageToPNG(image image.Image, dstPath string) error {
	outFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return png.Encode(outFile, image)
}

func writePNGWithEmbeddedAtlas(image image.Image, atlasEntries Byter, output io.Writer) error {
	atlasData := atlasEntries.Bytes()

	var byteBuffer *bytes.Buffer
	byteBuffer = new(bytes.Buffer)

	err := png.Encode(byteBuffer, image)
	if err != nil {
		return err
	}

	return png_chunks.InjectChunkIntoPNG(byteBuffer, output, "atls", atlasData)
}

func makeEven(value int) int {
	if value%2 == 0 {
		return value
	}
	return value + 1
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

func getTrimRect(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixels := AsRGBA(img).Pix
	minX, minY, maxX, maxY := detectOpaqueEdges(pixels, width, height)
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func detectOpaqueEdges(pixels []uint8, textureWidth int, textureHeight int) (int, int, int, int) {
	// find the first non-transparent pixel on each side
	minX := textureWidth
	minY := textureHeight

	maxX := 0
	maxY := 0

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
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	return minX, minY, maxX, maxY
}
