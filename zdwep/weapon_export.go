package zdwep

import (
	"bytes"
	"github.com/memmaker/goray/png_chunks"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/tpack"
	"image"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func dumpPNG(img image.Image, filename string) {
	if !strings.HasSuffix(strings.ToLower(filename), ".png") {
		filename += ".png"
	}
	outFile, err := os.Create(filename)
	if err != nil {
		println(err)
		return
	}
	err = png.Encode(outFile, img)
	if err != nil {
		println(err)
	}
	outFile.Close()
}

func dumpTXT(data string, filename string) {
	outFile, err := os.Create(filename)
	if err != nil {
		println(err)
		return
	}
	_, err = outFile.WriteString(data)
	if err != nil {
		println(err)
	}
	outFile.Close()
}

func dumpDoomWeapon(weapon DoomWeapon, pk3file string, outputDir string) {
	zf := NewCIZipWrapper(pk3file)
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		println(err)
		return
	}

	dumpTXT(weapon.OriginalScript, filepath.Join(outputDir, "script.txt"))
	for _, sfx := range weapon.FoundSounds {
		soundData, err := zf.Open(sfx)
		if err != nil {
			println(err)
			continue
		}
		outFile, err := os.Create(filepath.Join(outputDir, path.Base(sfx)))
		if err != nil {
			soundData.Close()
			println(err)
			continue
		}
		_, err = io.Copy(outFile, soundData)
		if err != nil {
			println(err)
		}
		outFile.Close()
		soundData.Close()
	}

	spriteOrder := make([]tpack.NamedImage, 0)

	for _, sprite := range weapon.FoundSprites {
		spriteData, err := zf.Open(sprite)
		if err != nil {
			println(err)
			continue
		}
		img, _, err := image.Decode(spriteData)

		spriteData.Close()
		if err != nil {
			println(err)
			continue
		}
		spriteOrder = append(spriteOrder, tpack.NamedImage{
			Image: img,
			Name:  sprite,
		})
		dumpPNG(img, filepath.Join(outputDir, path.Base(sprite)))
	}
	scriptString := weapon.OriginalScript
	baseFilename := safeFilename(weapon.GetName())

	atlas, metadata := tpack.PackImagesIntoAtlasTrimmed(spriteOrder)

	outFile, err := os.Create(filepath.Join(outputDir, baseFilename+".png"))
	writeDoomAtlas(atlas, metadata, scriptString, outFile)
	outFile.Close()
}

func ReadDoomAtlasFromFile(filename string) (atlas *glhf.TextureAtlasPacked, zscript string, err error) {
	reader, err := os.Open(filename)
	defer reader.Close()
	if err != nil {
		return nil, "", err
	}
	return readDoomAtlas(filename, reader)
}

func readDoomAtlas(filename string, atlasFile io.ReadSeeker) (atlas *glhf.TextureAtlasPacked, zscript string, err error) {
	atlas = quads.ReadTextureAtlasPacked(filename, atlasFile)
	atlasFile.Seek(0, io.SeekStart)
	zscript = png_chunks.ReadTextChunkFromPNG(atlasFile, "zscript")
	return atlas, zscript, nil
}
func writeDoomAtlas(image image.Image, atlasEntries tpack.Byter, zscript string, outFile io.Writer) error {
	atlasData := atlasEntries.Bytes()

	var byteBuffer *bytes.Buffer
	byteBuffer = new(bytes.Buffer)

	err := png.Encode(byteBuffer, image)
	if err != nil {
		return err
	}

	var outBuffer *bytes.Buffer
	outBuffer = new(bytes.Buffer)
	err = png_chunks.InjectChunkIntoPNG(byteBuffer, outBuffer, "atLS", atlasData)
	if err != nil {
		return err
	}
	return png_chunks.InjectTextChunkIntoPNG(outBuffer, outFile, "zscript", zscript)
}

func safeFilename(name string) string {
	baseName := filepath.Base(name)
	withoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	return strings.NewReplacer(" ", "_", "\"", "", "'", "", "´", "").Replace(withoutExt)
}
