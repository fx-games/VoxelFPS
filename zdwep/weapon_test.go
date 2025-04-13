package zdwep

import (
    "cmp"
    "fmt"
    "github.com/stretchr/testify/assert"
    "os"
    "path/filepath"
    "slices"
    "testing"
)

func TestLoadMetadataFromAtlas(t *testing.T) {
    pngFile, _ := os.Open("../doom_likes/Brutal/BrutalPistol/BrutalPistol.png")
    image, metadata, script, err := readDoomAtlas(pngFile)
    pngFile.Close()
    if err != nil {
        t.Logf(fmt.Sprintf("ERROR: %s", err))
        return
    }
    assert.NotNil(t, image)
    for _, meta := range metadata {
        t.Logf(meta.String())
    }
    assert.Greater(t, len(metadata), 0)
    assert.Greater(t, len(script), 0)
}

func TestLoadWeaponFromPk3(t *testing.T) {

    // *** 80 weapons found (48 complete, 32 missing)
    pk3file := "../assets/FinalDoomer_v3.4.pk3"

    // *** 17 weapons found (10 complete, 7 missing)
    //pk3file := "../assets/Brutal.pk3"

    // *** 55 weapons found (24 complete, 31 missing)
    //pk3file := "../assets/LiTDOOM0.5.pk3"

    weapons := loadWeaponsFromPk3(pk3file)

    if len(weapons) == 0 {
        t.Logf("No weapons found")
        return
    }
    slices.SortStableFunc(weapons, func(i, j DoomWeapon) int {
        if i.IsComplete() == j.IsComplete() {
            return cmp.Compare(i.Name, j.Name)
        }
        if i.IsComplete() {
            return -1
        } else {
            return 1
        }
    })
    for _, doomWeapon := range weapons {
        dumpDoomWeapon(doomWeapon, pk3file, filepath.Join("..", "doom_likes", safeFilename(pk3file), safeFilename(doomWeapon.Name)))
    }

    longest := 0
    for _, doomWeapon := range weapons {
        if len(doomWeapon.String()) > longest {
            longest = len(doomWeapon.String())
        }
    }

    for _, doomWeapon := range weapons {
        t.Logf(fmt.Sprintf("> %s -> %s", padRight(doomWeapon.String(), longest+1), doomWeapon.SourceFile))
        for _, needed := range doomWeapon.MissingSprites {
            t.Logf(fmt.Sprintf(" |>SPR: %s", needed))
        }
        for _, needed := range doomWeapon.MissingSounds {
            t.Logf(fmt.Sprintf(" |>SND: %s", needed))
        }

    }

    //weapon := loadWeaponsFromPk3("../assets/BD_Derp.pk3")

    assert.Greater(t, len(weapons), 0)
}
