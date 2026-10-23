package wep

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/tpack"
)

// A weapon lying on the floor, and the box of ammo beside it, are one picture
// each out of the shared store. How big that thing is in the world is a
// property of the picture: not of the file, which is padded with however much
// transparency the mod's artist left round it, but of the pixels in it.
//
// It is measured here, once, by the migration, and written into weapon.json.
// The alternative is measuring it at spawn time, which means reading the packed
// atlas back off the GPU once per pickup - seventy five megabytes a go, for an
// answer that cannot change between runs.
//
// The numbers are the picture's own pixels. What a pixel is worth in the world
// is the game's business, and it is the same scale every other sprite is drawn
// at.

// PickupBox is the opaque part of a pickup's picture, as [width, height]. A
// slice rather than a struct so a weapon with nothing to pick up writes no
// field at all.
type PickupBox []int

// Size is the box, and whether there is one. A weapon migrated before these
// were measured has none, and the caller falls back on the atlas.
func (b PickupBox) Size() (width, height int, ok bool) {
	if len(b) != 2 || b[0] <= 0 || b[1] <= 0 {
		return 0, 0, false
	}
	return b[0], b[1], true
}

// MeasurePickups fills in the boxes of whichever of the two pickup pictures the
// weapon has. A picture the store does not hold is not an error here: the
// migration already reports a weapon whose assets are missing, and a weapon
// without a measured box still spawns.
func MeasurePickups(weaponsDir string, weapon *Weapon) error {
	store := filepath.Join(weaponsDir, SpritesDir)
	for _, picture := range []struct {
		name string
		box  *PickupBox
	}{
		{weapon.Icon, &weapon.IconBox},
		{weapon.AmmoIcon, &weapon.AmmoIconBox},
	} {
		if picture.name == "" {
			continue
		}
		box, err := measurePicture(store, picture.name)
		if err != nil {
			return err
		}
		*picture.box = box
	}
	return nil
}

// measurePicture is the measuring itself: the same opaque bounds the packer
// trims a picture to, off the file in the store.
func measurePicture(store, name string) (PickupBox, error) {
	data, err := os.ReadFile(filepath.Join(store, name+".png"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	trimmed := tpack.TrimRect(decoded)
	if trimmed.Dx() <= 0 || trimmed.Dy() <= 0 {
		return nil, nil // a blank picture: nothing to stand on a shelf
	}
	return PickupBox{trimmed.Dx(), trimmed.Dy()}, nil
}
