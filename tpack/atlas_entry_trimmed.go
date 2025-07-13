package tpack

import (
    "bytes"
    "encoding/binary"
    "fmt"
)

type TrimmedAtlasEntry struct {
    SpriteName                LPString8
    X, Y                      uint16
    SourceWidth, SourceHeight uint16

    TrimRectX, TrimRectY          uint16
    TrimRectWidth, TrimRectHeight uint16
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
        SpriteName:     LPString8(spriteName),
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

func (e TrimmedAtlasEntry) String() string {
    return fmt.Sprintf("%s %d %d %d %d %d %d %d %d", e.SpriteName, e.X, e.Y, e.SourceWidth, e.SourceHeight, e.TrimRectX, e.TrimRectY, e.TrimRectWidth, e.TrimRectHeight)
}
