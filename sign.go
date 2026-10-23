package main

import (
	"log"

	"github.com/memmaker/voxelfps/quads"
)

// A sign is a display on a wall with a few words on it: which deck this is,
// which way the lab is, that the door ahead is sealed. What it says is its
// entity's Text - the field a note's writing is kept in - drawn onto a copy of
// the model of its own (quads.SignPicture, MeshDrawer.NewOwnMesh), so no two
// signs share a picture and changing one's words repaints that one.
//
// Lines are split at "|", and a line starting with "!" is a warning, written
// red. In a level script it is `sign X Y Z FACING the words`; in the editor it
// is E on the sign, like a note.

const (
	signMeshName = "sign"
	// graffitiMeshName is a sign's rougher cousin: words painted straight on a
	// wall, a transparent panel with quads.GraffitiPicture on it.
	graffitiMeshName = "graffiti"
	// signEmissiveStrength lights the writing. It is the only bright, saturated
	// thing on the panel, which is what the emissive pass picks out - see the
	// keypad for the same trick.
	signEmissiveStrength = float32(2.0)
)

type Sign struct {
	object *quads.GameObject
	text   string
}

func (g *Game) spawnSign(object *quads.GameObject, text string) {
	sign := &Sign{object: object}
	g.signs = append(g.signs, sign)
	g.setSignText(sign, text)
}

func (g *Game) signFor(object *quads.GameObject) *Sign {
	for _, sign := range g.signs {
		if sign.object == object {
			return sign
		}
	}
	return nil
}

// setSignText repaints a sign and records its words where a save finds them.
func (g *Game) setSignText(sign *Sign, text string) {
	sign.text = text
	if err := g.Meshes.SetTexture(sign.object.Mesh().MeshIndex, 0, quads.SignPicture(text)); err != nil {
		log.Printf("[sign] could not paint %q: %v", text, err)
	}
	if record := g.placedRecord(sign.object); record != nil {
		record.entity.Text = text
	}
}
