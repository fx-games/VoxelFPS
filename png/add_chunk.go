package png

import (
    "encoding/binary"
    "fmt"
    "io"
    "os"
    "strings"
    "unsafe"
)

var littleEndian bool
var pngSig = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func endianSwap(x *uint32) {
    if littleEndian {
        *x = (*x>>24)&0xFF | (*x>>8)&0xFF00 | (*x<<8)&0xFF0000 | (*x<<24)&0xFF000000
    }
}

func injectTextChunk(key, content string, stream io.Writer) {
    length := uint32(len(key) + 1 + len(content))
    buffer := append([]byte("tEXt"+key+"\x00"+content), make([]byte, 4)...) // Extra 4 bytes for CRC

    lengthOut := length
    endianSwap(&lengthOut)
    binary.Write(stream, binary.BigEndian, lengthOut)
    stream.Write(buffer[:len(buffer)-4])

    crcOut := crcCalculate(buffer[:len(buffer)-4])
    endianSwap(&crcOut)
    binary.Write(stream, binary.BigEndian, crcOut)
}

func crcCalculate(data []byte) uint32 {
    // Placeholder for actual CRC calculation function
    return 0
}

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintf(os.Stderr, "Usage: %s <key> <content>\n\n", os.Args[0])
        fmt.Fprintln(os.Stderr, "Reads a PNG image from standard input;")
        fmt.Fprintln(os.Stderr, "Injects a text chunk (key=content);")
        fmt.Fprintln(os.Stderr, "Copies all other chunks unchanged to standard output.")
        os.Exit(1)
    }

    key, content := os.Args[1], os.Args[2]
    infile, outfile := os.Stdin, os.Stdout

    endianTest := uint32(1)
    littleEndian = (*(*byte)(unsafe.Pointer(&endianTest)) != 0)

    sig := make([]byte, len(pngSig))
    if _, err := io.ReadFull(infile, sig); err != nil || !strings.HasPrefix(string(sig), string(pngSig)) {
        fmt.Fprintln(os.Stderr, "Invalid PNG signature")
        os.Exit(1)
    }

    outfile.Write(pngSig)

    didTextChunk := false

    for {
        var length uint32
        name := make([]byte, 4)
        var crc uint32

        if err := binary.Read(infile, binary.BigEndian, &length); err != nil {
            break
        }
        endianSwap(&length)

        if _, err := io.ReadFull(infile, name); err != nil {
            break
        }

        buffer := make([]byte, length)
        if _, err := io.ReadFull(infile, buffer); err != nil {
            break
        }

        if err := binary.Read(infile, binary.BigEndian, &crc); err != nil {
            break
        }
        endianSwap(&crc)

        if string(name) == "IDAT" || string(name) == "IEND" {
            if !didTextChunk {
                injectTextChunk(key, content, outfile)
                didTextChunk = true
            }
        }

        binary.Write(outfile, binary.BigEndian, length)
        outfile.Write(name)
        outfile.Write(buffer)
        binary.Write(outfile, binary.BigEndian, crc)
    }
}
