package tpack

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "github.com/memmaker/goray/png_chunks"
    "image"
    "image/draw"
    "image/png"
    "io"
    "os"
    "path"
    "strings"
)

type LPString string

func (s LPString) String() string {
    return string(s)
}
func (s LPString) Bytes() []byte {
    length := uint8(len(s))
    return append([]byte{length}, []byte(s)...)
}

type AtlasMetadata []AtlasEntry

func (a AtlasMetadata) Bytes() []byte {
    var buffer []byte
    for _, entry := range a {
        buffer = append(buffer, entry.Bytes()...)
    }
    return buffer
}

func (a AtlasMetadata) Size(index int) (int, int) {
    return int(a[index].Width), int(a[index].Height)
}

func (a AtlasMetadata) Position(index int) (int, int) {
    return int(a[index].X), int(a[index].Y)
}

func (a AtlasMetadata) UV(index int) (minU int, minV int, maxU int, maxV int) {
    width, height := a.Size(index)
    x, y := a.Position(index)
    return x, y, x + width, y + height
}

func (a AtlasMetadata) SizeForLayout(index int) (int, int) {
    return a.Size(index)
}
func (a AtlasMetadata) PositionOffsetForLayout(index int) (int, int) {
    return 0, 0
}
func NewAtlasMetadataFromBytes(data []byte) AtlasMetadata {
    a := make(AtlasMetadata, 0)
    reader := bytes.NewReader(data)
    for reader.Len() > 0 {
        entry := NewAtlasEntryFromReader(reader)
        a = append(a, entry)
    }
    return a
}

type TrimmedAtlasMetadata []TrimmedAtlasEntry

func (a TrimmedAtlasMetadata) HasIndex(index int) bool {
    return index >= 0 && index < len(a)
}

func (a TrimmedAtlasMetadata) LastValidIndex() int {
    return len(a) - 1
}

func (a TrimmedAtlasMetadata) Name(index int) string {
    return string(a[index].SpriteName)
}

func (a TrimmedAtlasMetadata) Bytes() []byte {
    var buffer []byte
    for _, entry := range a {
        buffer = append(buffer, entry.Bytes()...)
    }
    return buffer
}

func NewTrimmedAtlasMetadataFromBytes(data []byte) TrimmedAtlasMetadata {
    a := make(TrimmedAtlasMetadata, 0)
    reader := bytes.NewReader(data)
    for reader.Len() > 0 {
        entry := NewTrimmedAtlasEntryFromReader(reader)
        a = append(a, entry)
    }
    return a
}

func (a TrimmedAtlasMetadata) UVSize(index int) (int, int) {
    return int(a[index].TrimRectWidth), int(a[index].TrimRectHeight)
}

func (a TrimmedAtlasMetadata) UVPosition(index int) (int, int) {
    return int(a[index].X), int(a[index].Y)
}

func (a TrimmedAtlasMetadata) UV(index int) (minU int, minV int, maxU int, maxV int) {
    width, height := a.UVSize(index)
    x, y := a.UVPosition(index)
    return x, y, x + width, y + height
}

func (a TrimmedAtlasMetadata) SizeForLayout(index int) (int, int) {
    return int(a[index].SourceWidth), int(a[index].SourceHeight)
}
func (a TrimmedAtlasMetadata) PositionOffsetForLayout(index int) (int, int) {
    return int(a[index].TrimRectX), int(a[index].TrimRectY)
}

type AtlasEntry struct {
    SpriteName          LPString
    X, Y, Width, Height uint16
}

func (a AtlasEntry) String() string {
    return fmt.Sprintf("%s %d %d %d %d", a.SpriteName, a.X, a.Y, a.Width, a.Height)
}

func (a AtlasEntry) Length() int {
    return len(a.SpriteName) + 9
}

func (a AtlasEntry) Bytes() []byte {
    writer := new(bytes.Buffer)
    binary.Write(writer, binary.BigEndian, a.SpriteName.Bytes())
    binary.Write(writer, binary.BigEndian, a.X)
    binary.Write(writer, binary.BigEndian, a.Y)
    binary.Write(writer, binary.BigEndian, a.Width)
    binary.Write(writer, binary.BigEndian, a.Height)
    return writer.Bytes()
}

func NewAtlasEntryFromReader(reader *bytes.Reader) AtlasEntry {
    var nameLength uint8
    binary.Read(reader, binary.BigEndian, &nameLength)
    var spriteName = make([]byte, nameLength)
    binary.Read(reader, binary.BigEndian, &spriteName)
    var x, y, width, height uint16
    binary.Read(reader, binary.BigEndian, &x)
    binary.Read(reader, binary.BigEndian, &y)
    binary.Read(reader, binary.BigEndian, &width)
    binary.Read(reader, binary.BigEndian, &height)
    return AtlasEntry{
        SpriteName: LPString(spriteName),
        X:          x,
        Y:          y,
        Width:      width,
        Height:     height,
    }
}

type TrimmedAtlasEntry struct {
    SpriteName                LPString
    X, Y                      uint16
    SourceWidth, SourceHeight uint16

    TrimRectX, TrimRectY          uint16
    TrimRectWidth, TrimRectHeight uint16
}

func (e TrimmedAtlasEntry) Bytes() []byte {
    writer := new(bytes.Buffer)
    binary.Write(writer, binary.BigEndian, e.SpriteName.Bytes())
    binary.Write(writer, binary.BigEndian, e.X)
    binary.Write(writer, binary.BigEndian, e.Y)
    binary.Write(writer, binary.BigEndian, e.SourceWidth)
    binary.Write(writer, binary.BigEndian, e.SourceHeight)
    binary.Write(writer, binary.BigEndian, e.TrimRectX)
    binary.Write(writer, binary.BigEndian, e.TrimRectY)
    binary.Write(writer, binary.BigEndian, e.TrimRectWidth)
    binary.Write(writer, binary.BigEndian, e.TrimRectHeight)
    return writer.Bytes()
}

func NewTrimmedAtlasEntryFromReader(reader *bytes.Reader) TrimmedAtlasEntry {
    var nameLength uint8
    binary.Read(reader, binary.BigEndian, &nameLength)
    var spriteName = make([]byte, nameLength)
    binary.Read(reader, binary.BigEndian, &spriteName)
    var x, y, sourceWidth, sourceHeight uint16
    binary.Read(reader, binary.BigEndian, &x)
    binary.Read(reader, binary.BigEndian, &y)
    binary.Read(reader, binary.BigEndian, &sourceWidth)
    binary.Read(reader, binary.BigEndian, &sourceHeight)
    var trimRectX, trimRectY, trimRectWidth, trimRectHeight uint16
    binary.Read(reader, binary.BigEndian, &trimRectX)
    binary.Read(reader, binary.BigEndian, &trimRectY)
    binary.Read(reader, binary.BigEndian, &trimRectWidth)
    binary.Read(reader, binary.BigEndian, &trimRectHeight)
    return TrimmedAtlasEntry{
        SpriteName:     LPString(spriteName),
        X:              x,
        Y:              y,
        SourceWidth:    sourceWidth,
        SourceHeight:   sourceHeight,
        TrimRectX:      trimRectX,
        TrimRectY:      trimRectY,
        TrimRectWidth:  trimRectWidth,
        TrimRectHeight: trimRectHeight,
    }
}

func (e TrimmedAtlasEntry) String() string {
    return fmt.Sprintf("%s %d %d %d %d %d %d %d %d", e.SpriteName, e.X, e.Y, e.SourceWidth, e.SourceHeight, e.TrimRectX, e.TrimRectY, e.TrimRectWidth, e.TrimRectHeight)
}

type NamedImage struct {
    image.Image
    Name string
}

func (i NamedImage) Pixels() []uint8 {
    return AsRGBA(i).Pix
}

func LoadTrimmedAtlas(filename string) (image.Image, TrimmedAtlasMetadata, error) {
    file, err := os.Open(filename)
    if err != nil {
        return nil, nil, err
    }
    defer file.Close()
    return DecodeTrimmedAtlas(file)
}

func DecodeTrimmedAtlas(inReader io.ReadSeeker) (image.Image, TrimmedAtlasMetadata, error) {
    img, err := png.Decode(inReader)
    if err != nil {
        return nil, nil, err
    }
    inReader.Seek(0, io.SeekStart)
    metadata := png_chunks.ReadChunkFromPNG(inReader, "atLS")
    decodedMeta := NewTrimmedAtlasMetadataFromBytes(metadata)
    return img, decodedMeta, nil
}

func writePackedAtlas(images []NamedImage, outWriter io.Writer) {
    dst, atlasEntries := packImagesIntoAtlas(images)

    err := writePNGAtlas(dst, atlasEntries, outWriter)
    if err != nil {
        println(err)
        return
    }
}

func packImagesIntoAtlas(images []NamedImage) (*image.RGBA, AtlasMetadata) {
    var rectList []RectangleWithIndex
    for index, sprite := range images {
        bounds := sprite.Bounds()
        rect := RectangleWithIndex{
            Rectangle: Rectangle{
                Width:  bounds.Dx(),
                Height: bounds.Dy(),
            },
            Index: index,
        }

        rectList = append(rectList, rect)
    }
    targetAtlasWidth := 2048
    targetAtlasHeight := 8192 * 2
    binPack := NewPacker(targetAtlasWidth, targetAtlasHeight)
    binPack.InsertAll(rectList, RULE_BEST_AREA_FIT)
    binPack.ShrinkBin(false)
    placedRects := binPack.GetUsedRectangles()
    neededWidth := makeEven(binPack.GetWidth())
    neededHeight := makeEven(binPack.GetHeight())
    dst := image.NewRGBA(image.Rect(0, 0, neededWidth, neededHeight))

    atlasEntries := make(AtlasMetadata, 0)
    for _, rect := range placedRects {
        sprite := images[rect.Index]
        bounds := sprite.Bounds()
        nrgba := AsRGBA(sprite)
        spriteName := path.Base(sprite.Name)
        if strings.HasSuffix(strings.ToLower(spriteName), ".png") {
            spriteName = spriteName[:len(spriteName)-4]
        }
        atlasEntries = append(atlasEntries, AtlasEntry{
            SpriteName: LPString(spriteName),
            X:          uint16(rect.X),
            Y:          uint16(rect.Y),
            Width:      uint16(rect.Width),
            Height:     uint16(rect.Height),
        })
        draw.Draw(dst, rect.ToImageRect(), nrgba, bounds.Min, draw.Src)
    }
    return dst, atlasEntries
}
func PackImagesIntoAtlasTrimmed(images []NamedImage) (*image.RGBA, TrimmedAtlasMetadata) {
    var rectList []RectangleWithIndex
    var trimRects []image.Rectangle
    for index, sprite := range images {
        trimRect := getTrimRect(sprite)
        rect := RectangleWithIndex{
            Rectangle: Rectangle{
                Width:  trimRect.Bounds().Dx(),
                Height: trimRect.Bounds().Dy(),
            },
            Index: index,
        }
        trimRects = append(trimRects, trimRect)
        rectList = append(rectList, rect)
    }
    targetAtlasWidth := 2048
    targetAtlasHeight := 8192 * 2
    binPack := NewPacker(targetAtlasWidth, targetAtlasHeight)
    binPack.InsertAll(rectList, RULE_BEST_AREA_FIT)
    binPack.ShrinkBin(false)
    placedRects := binPack.GetUsedRectangles()
    neededWidth := makeEven(binPack.GetWidth())
    neededHeight := makeEven(binPack.GetHeight())
    dst := image.NewRGBA(image.Rect(0, 0, neededWidth, neededHeight))

    atlasEntries := make(TrimmedAtlasMetadata, 0)
    for _, rect := range placedRects {
        sprite := images[rect.Index]
        bounds := sprite.Bounds()
        nrgba := AsRGBA(sprite)
        spriteName := path.Base(sprite.Name)
        if strings.HasSuffix(strings.ToLower(spriteName), ".png") {
            spriteName = spriteName[:len(spriteName)-4]
        }

        trimRect := trimRects[rect.Index]
        atlasEntries = append(atlasEntries, TrimmedAtlasEntry{
            SpriteName:     LPString(spriteName),
            X:              uint16(rect.X),
            Y:              uint16(rect.Y),
            SourceWidth:    uint16(bounds.Dx()),
            SourceHeight:   uint16(bounds.Dy()),
            TrimRectWidth:  uint16(rect.Width),
            TrimRectHeight: uint16(rect.Height),
            TrimRectX:      uint16(trimRect.Min.X),
            TrimRectY:      uint16(trimRect.Min.Y),
        })
        draw.Draw(dst, rect.ToImageRect(), nrgba, trimRect.Min, draw.Src)
    }
    return dst, atlasEntries
}

type Byter interface {
    Bytes() []byte
}

func writePNGAtlas(image image.Image, atlasEntries Byter, output io.Writer) error {
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
