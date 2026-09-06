// Command genshelves writes scenery/shelves/mesh.glb: a storage rack to stand
// equipment on - two uprights and three boards, every part a box, with one
// 16x16 dirty-metal texture embedded in the .glb the way genstairs does it.
//
// It is generated rather than modelled because it is five boxes, and because
// the board heights are the same numbers the colliders and the weapon range
// read out of scenery/shelves/scenery.json. Run with `go run ./tools/genshelves`
// from the repository root.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"path/filepath"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// The rack, in world units where a voxel is 1 and the player is 0.875 tall. A
// tile wide so it fills the tile it is dropped on, shallow enough that a row of
// them still leaves a walkable aisle, and its top board just above eye height.
//
// The board tops - 0.05, 0.60 and 1.05 - are what things stand on, and they are
// repeated in scenery/shelves/scenery.json as colliders. Change them here and
// there together.
const (
	width        = 1.0
	depth        = 0.4
	thickness    = 0.05
	uprightWidth = 0.05
	// Where each board's top surface is.
	board0 = 0.05
	board1 = 0.60
	board2 = 1.05
)

// box appends the six faces of an axis aligned box. Every face gets the whole
// texture, which for a plank is all the mapping it needs.
func box(min, max [3]float32, positions, normals *[][3]float32, uvs *[][2]float32, indices *[]uint16) {
	corners := [8][3]float32{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]},
		{max[0], min[1], max[2]}, {min[0], min[1], max[2]},
		{min[0], max[1], min[2]}, {max[0], max[1], min[2]},
		{max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	// Each face counter-clockwise seen from outside, so front face culling keeps
	// the outside of the box.
	faces := []struct {
		corner [4]int
		normal [3]float32
	}{
		{[4]int{7, 3, 2, 6}, [3]float32{0, 0, 1}},  // +z
		{[4]int{5, 1, 0, 4}, [3]float32{0, 0, -1}}, // -z
		{[4]int{6, 2, 1, 5}, [3]float32{1, 0, 0}},  // +x
		{[4]int{4, 0, 3, 7}, [3]float32{-1, 0, 0}}, // -x
		{[4]int{4, 7, 6, 5}, [3]float32{0, 1, 0}},  // +y
		{[4]int{3, 0, 1, 2}, [3]float32{0, -1, 0}}, // -y
	}
	for _, f := range faces {
		base := uint16(len(*positions))
		for _, c := range f.corner {
			*positions = append(*positions, corners[c])
			*normals = append(*normals, f.normal)
		}
		*uvs = append(*uvs, [2]float32{0, 0}, [2]float32{0, 1}, [2]float32{1, 1}, [2]float32{1, 0})
		*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)
	}
}

func buildShelves() (positions, normals [][3]float32, uvs [][2]float32, indices []uint16) {
	halfW, halfD := float32(width)/2, float32(depth)/2
	// Two uprights, floor to the top board.
	box([3]float32{-halfW, 0, -halfD}, [3]float32{-halfW + uprightWidth, board2, halfD},
		&positions, &normals, &uvs, &indices)
	box([3]float32{halfW - uprightWidth, 0, -halfD}, [3]float32{halfW, board2, halfD},
		&positions, &normals, &uvs, &indices)
	// Three boards, each hanging under its own top surface, spanning the gap
	// between the uprights rather than the shelf's own outer width - the full
	// width used to run the boards straight through both uprights, the same
	// wood filling the same space twice from directly above and z-fighting
	// there every time the shelf moved a pixel on screen.
	for _, top := range []float32{board0, board1, board2} {
		box([3]float32{-halfW + uprightWidth, top - thickness, -halfD}, [3]float32{halfW - uprightWidth, top, halfD},
			&positions, &normals, &uvs, &indices)
	}
	return
}

func main() {
	positions, normals, uvs, indices := buildShelves()

	doc := gltf.NewDocument()

	imageIndex, err := modeler.WriteImage(doc, "shelves", "image/png", bytes.NewReader(metalTexture()))
	if err != nil {
		log.Fatal(err)
	}
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest, MinFilter: gltf.MinNearest,
		WrapS: gltf.WrapRepeat, WrapT: gltf.WrapRepeat,
	})
	doc.Textures = append(doc.Textures, &gltf.Texture{
		Sampler: gltf.Index(0), Source: gltf.Index(imageIndex),
	})
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name:        "shelves",
		AlphaMode:   gltf.AlphaOpaque,
		DoubleSided: false,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})
	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: "shelves",
		Primitives: []*gltf.Primitive{{
			Mode:     gltf.PrimitiveTriangles,
			Material: gltf.Index(0),
			Indices:  gltf.Index(modeler.WriteIndices(doc, indices)),
			Attributes: gltf.PrimitiveAttributes{
				gltf.POSITION:   modeler.WritePosition(doc, positions),
				gltf.NORMAL:     modeler.WriteNormal(doc, normals),
				gltf.TEXCOORD_0: modeler.WriteTextureCoord(doc, uvs),
			},
		}},
	})
	doc.Nodes = append(doc.Nodes, &gltf.Node{
		Name:     "shelves",
		Mesh:     gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1},
		Scale:    [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	out := filepath.Join("scenery", "shelves", "mesh.glb")
	if err := gltf.SaveBinary(doc, out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d vertices, %d triangles", out, len(positions), len(indices)/3)
}

// metalTexture is dull, grimy sheet metal: a cool grey base, panel seams every
// fourth row the way the planking this replaced had plank gaps, and a
// scatter of darker rust and grease specks so it reads as handled equipment
// rather than a fresh coat of paint. Dull on purpose, same as the planking
// was: the mesh shader picks glowing texels out by brightness and
// colourfulness, and a shelf is not a lamp.
func metalTexture() []byte {
	random := rand.New(rand.NewSource(3))
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			shade := uint8(0x60 + random.Intn(0x10))
			// A darker line every fourth row reads as the seam between panels.
			if y%4 == 0 {
				shade -= 0x18
			}
			r, g, b := shade, shade, uint8(float32(shade)*1.06)
			// A grease or rust speck here and there, dark and slightly warm so
			// it reads as grime rather than more panel.
			if random.Intn(6) == 0 {
				dirt := uint8(0x18 + random.Intn(0x14))
				r, g, b = dirt+0x08, dirt, dirt
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
