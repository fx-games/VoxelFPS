package main

// Reading enough of a sprite kind to place one. mapgen writes map files without
// running the game, so it cannot ask the game what a folder holds - but a
// script that had to repeat a character's frame indices would be a script that
// goes stale the moment the folder is re-imported.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/grid"
)

// spriteDir is the folder of folders, one to a kind. It matches the game's own
// spriteDir; there is nothing to share between them but the string.
const spriteDir = "assets/sprites"

// spriteKind is the part of sprite.json a map file needs: what the thing is
// called, how big one frame is, and which frame it stands on facing each way.
type spriteKind struct {
	Name       string                 `json:"name"`
	Tile       grid.Point             `json:"tile"`
	Animations map[string]*spriteAnim `json:"animations"`
}

type spriteAnim struct {
	Frames [][]uint16 `json:"frames"`
}

func readSpriteKind(id string) (*spriteKind, error) {
	path := filepath.Join(spriteDir, id, "sprite.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sprite=%s: %w", id, err)
	}
	kind := &spriteKind{}
	if err := json.Unmarshal(raw, kind); err != nil {
		return nil, fmt.Errorf("sprite=%s: %w", id, err)
	}
	if kind.Animations["idle"] == nil || len(kind.Animations["idle"].Frames) == 0 {
		return nil, fmt.Errorf("sprite=%s: no idle animation", id)
	}
	return kind, nil
}

// idleFrames is the first frame of the idle, one per facing - what the thing
// stands on before any animation starts.
func (k *spriteKind) idleFrames() []uint16 {
	idle := k.Animations["idle"]
	out := make([]uint16, 0, len(idle.Frames))
	for _, direction := range idle.Frames {
		if len(direction) > 0 {
			out = append(out, direction[0])
		}
	}
	return out
}
