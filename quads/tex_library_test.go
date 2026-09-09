package quads

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNGFile(t *testing.T, path string, fill color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range img.Pix {
		img.Pix[i] = 0
	}
	img.Set(0, 0, fill)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A texture dragged onto the window comes with an absolute path from wherever
// the player keeps their art, which is exactly what the library is not: this is
// the copy into it, without a GL context anywhere near it.
func TestAcceptDroppedCopiesIntoTheLibrary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	browser := &TexBrowser{root: root, tileW: 8, tileH: 8}

	dropped := filepath.Join(outside, "wall.png")
	writePNGFile(t, dropped, color.RGBA{R: 255, A: 255})
	notAnImage := filepath.Join(outside, "notes.txt")
	if err := os.WriteFile(notAnImage, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	lying := filepath.Join(outside, "lying.png")
	if err := os.WriteFile(lying, []byte("this is not a png"), 0o644); err != nil {
		t.Fatal(err)
	}

	saved, err := browser.AcceptDropped([]string{dropped, notAnImage, lying})
	if err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatalf("saved %d files, want 1 - the txt and the fake png should be left alone", saved)
	}
	if _, err := os.Stat(filepath.Join(root, DroppedDir, "wall.png")); err != nil {
		t.Fatalf("the dropped texture is not in the library: %v", err)
	}
}

// Dropping the same file name twice must not overwrite: a map records a picked
// texture by its library path, so a path already handed out has to keep meaning
// the pixels it meant.
func TestAcceptDroppedKeepsBothWhenNamesCollide(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	browser := &TexBrowser{root: root, tileW: 8, tileH: 8}

	first := filepath.Join(outside, "wall.png")
	writePNGFile(t, first, color.RGBA{R: 255, A: 255})
	if _, err := browser.AcceptDropped([]string{first}); err != nil {
		t.Fatal(err)
	}

	second := filepath.Join(outside, "again", "wall.png")
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNGFile(t, second, color.RGBA{B: 255, A: 255})
	if _, err := browser.AcceptDropped([]string{second}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"wall.png", "wall-2.png"} {
		if _, err := os.Stat(filepath.Join(root, DroppedDir, name)); err != nil {
			t.Errorf("%s is missing: %v", name, err)
		}
	}
}

// ImageFromOSPath is the whole reason a drop works at all: Disk is rooted at the
// working directory, and an fs.FS refuses an absolute path outright.
func TestImageFromOSPathReadsAnAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wall.png")
	writePNGFile(t, path, color.RGBA{G: 255, A: 255})

	got := ImageFromOSPath(path)
	if got.Bounds().Dx() != 4 || got.Bounds().Dy() != 4 {
		t.Fatalf("got a %v image, want the 4x4 that was written - a placeholder is 1x1", got.Bounds())
	}
	if r, g, b, _ := got.At(0, 0).RGBA(); r != 0 || g == 0 || b != 0 {
		t.Errorf("pixel is not the green that was written: %v", got.At(0, 0))
	}
	if placeholder := Disk.Image(path); placeholder.Bounds().Dx() != 1 {
		t.Errorf("Disk.Image took an absolute path after all - this test is now pointless")
	}
}
