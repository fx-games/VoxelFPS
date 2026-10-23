// Command importbs pulls Blake Stone: Aliens of Gold's art and sound out of its
// own VSWAP container and files it the way this project files assets.
//
//	go run ./tools/importbs -vswap ~/blake/VSWAP.BS6 -dump /tmp/bs
//
// -dump writes every wall and every sprite as its own PNG under the given
// directory, named the way bstone names it. It is the raw form, for looking at
// what is in there; the grouped output the game actually loads is written by the
// other modes.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/memmaker/voxelfps/audio"
)

func main() {
	vswapPath := flag.String("vswap", "", "path to VSWAP.BS6")
	dumpDir := flag.String("dump", "", "write every wall, sprite and sound here, one file each")
	install := flag.Bool("install", false, "file everything into this repo's own asset folders")
	flag.Parse()

	if *vswapPath == "" {
		fail(fmt.Errorf("-vswap is required"))
	}
	v, err := openVSWAP(*vswapPath)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d walls, %d sprites, %d sound pages\n", v.wallCount(), v.spriteCount(), len(v.offsets)-v.soundStart)

	if *dumpDir != "" {
		if err := dump(v, *dumpDir); err != nil {
			fail(err)
		}
	}
	if *install {
		in := newInstaller(v)
		walls, err := in.installWalls()
		if err != nil {
			fail(err)
		}
		characters, err := in.installCharacters()
		if err != nil {
			fail(err)
		}
		props, err := in.installProps()
		if err != nil {
			fail(err)
		}
		weapons, err := in.installWeapons()
		if err != nil {
			fail(err)
		}
		fmt.Printf("installed %d walls, %d characters, %d scenery pieces, %d weapons\n",
			walls, characters, props, weapons)
	}
}

func dump(v *vswap, dir string) error {
	for _, sub := range []string{"walls", "sprites", "sounds"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	for i := 0; i < v.wallCount(); i++ {
		img, err := v.wall(i, blakePalette)
		if err != nil {
			return err
		}
		if err := writePNG(filepath.Join(dir, "walls", fmt.Sprintf("W%03d.png", i)), img); err != nil {
			return err
		}
	}
	var broken int
	for i := 0; i < v.spriteCount(); i++ {
		img, err := v.sprite(i, blakePalette)
		if err != nil {
			broken++
			if broken < 4 {
				fmt.Println("  ", err)
			}
			continue
		}
		name := spriteNames[i]
		if name == "" {
			name = "UNNAMED"
		}
		if err := writePNG(filepath.Join(dir, "sprites", fmt.Sprintf("S%03d_%s.png", i, name)), img); err != nil {
			return err
		}
	}
	if broken > 0 {
		fmt.Printf("%d sprites failed to decode\n", broken)
	}
	for i, d := range v.digitizedTable() {
		samples := v.samples(d)
		if len(samples) == 0 {
			continue
		}
		name := soundNames[i]
		if name == "" {
			name = fmt.Sprintf("SOUND_%d", i)
		}
		if err := os.WriteFile(filepath.Join(dir, "sounds", name+".wav"), audio.WAVFromPCM8(samples, pcmRate), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "importbs:", err)
	os.Exit(1)
}
