package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/memmaker/voxellib/tpack"
)

func main() {
	f, _ := os.Open("assets/weapons/bd_axe/sprites.png")
	img, _, err := tpack.DecodeTrimmedAtlas(f)
	if err != nil {
		panic(err)
	}
	sub := img.(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(image.Rect(648, 412, 956, 562))
	out, _ := os.Create(os.Args[1])
	png.Encode(out, sub)
	out.Close()
	fmt.Println("ok", img.Bounds())
}
