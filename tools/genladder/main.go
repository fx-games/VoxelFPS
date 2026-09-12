// Command genladder writes assets/meshes/ladder/mesh.glb: two rails and a run of
// rungs, with one 16x16 wood texture embedded the way gentorch/genstairs do
// it. It is generated rather than modelled for the same reason stairs are -
// it is a handful of boxes, and the height has to match the scenery.json boxes
// exactly or the model and the climb volume disagree about where the top is.
//
// The piece is one voxel tall and tiles: a shaft's worth of ladder is the same
// module stacked, one a voxel, which is what `entity ... step=0,1,0 times=N`
// lays down. That is why the rung spacing has to divide the height - the rung
// pattern has to carry across the seam without a double gap at every joint -
// and why the rails run the full height, so stacked rails meet flush.
//
// Run with `go run ./tools/genladder` from the repository root.
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

// The ladder, in world units where a voxel is 1. Height must match the
// Collider box in assets/meshes/ladder/scenery.json, and the Climbable box is
// that plus climbOut there.
const (
	// One voxel, so the piece stacks: see the package comment.
	ladderHeight  = 1.0
	railThickness = 0.08
	railInset     = 0.28 // rail centre distance from the ladder's own x=0
	// Four rungs to the voxel, which is a rung every half metre at two metres
	// to the voxel. It has to divide ladderHeight exactly: the first rung sits
	// half a spacing up, so an exact division puts the last one half a spacing
	// below the top and the next module's first one a full spacing above it.
	rungSpacing   = 0.25
	rungThickness = 0.06
	rungDepth     = 0.08
	// wallZ pushes the whole model to the far edge of the tile it is placed
	// on, rather than centering it: an entity spawns at its tile's centre, and
	// a ladder centred there stands half a tile out from the wall it is
	// supposed to be mounted on, in a case where you can walk clean around or
	// past it before ever touching it. 0.5 is the tile edge; leaving a hair of
	// clearance keeps the rails from z-fighting the wall texture.
	wallZ = 0.46
)

// box appends the six faces of an axis-aligned box, one texture stretched over
// each face. Same helper as genstairs; a ladder and a staircase are both just
// boxes and neither tool is the natural home for a shared one yet.
func box(min, max [3]float32, positions, normals *[][3]float32, uvs *[][2]float32, indices *[]uint16) {
	corners := [8][3]float32{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]},
		{max[0], min[1], max[2]}, {min[0], min[1], max[2]},
		{min[0], max[1], min[2]}, {max[0], max[1], min[2]},
		{max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	faces := []struct {
		corner [4]int
		normal [3]float32
	}{
		{[4]int{7, 3, 2, 6}, [3]float32{0, 0, 1}},
		{[4]int{5, 1, 0, 4}, [3]float32{0, 0, -1}},
		{[4]int{6, 2, 1, 5}, [3]float32{1, 0, 0}},
		{[4]int{4, 0, 3, 7}, [3]float32{-1, 0, 0}},
		{[4]int{4, 7, 6, 5}, [3]float32{0, 1, 0}},
		{[4]int{3, 0, 1, 2}, [3]float32{0, -1, 0}},
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

func buildLadder() (positions, normals [][3]float32, uvs [][2]float32, indices []uint16) {
	for _, x := range []float32{-railInset, railInset} {
		min := [3]float32{x - railThickness/2, 0, wallZ - railThickness/2}
		max := [3]float32{x + railThickness/2, ladderHeight, wallZ + railThickness/2}
		box(min, max, &positions, &normals, &uvs, &indices)
	}
	// A rung's own box stops at each rail's inner face rather than running
	// through it: sharing a face with the rail box - same plane, same
	// position - is exactly what z-fighting is, two triangles at an identical
	// depth with the renderer picking a winner every other pixel.
	for y := float32(rungSpacing / 2); y < ladderHeight; y += rungSpacing {
		min := [3]float32{-railInset + railThickness/2, y - rungThickness/2, wallZ - rungDepth/2}
		max := [3]float32{railInset - railThickness/2, y + rungThickness/2, wallZ + rungDepth/2}
		box(min, max, &positions, &normals, &uvs, &indices)
	}
	return
}

func main() {
	positions, normals, uvs, indices := buildLadder()

	doc := gltf.NewDocument()

	imageIndex, err := modeler.WriteImage(doc, "ladder", "image/png", bytes.NewReader(woodTexture()))
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
		Name:        "ladder",
		AlphaMode:   gltf.AlphaOpaque,
		DoubleSided: false,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})
	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: "ladder",
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
		Name:     "ladder",
		Mesh:     gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1},
		Scale:    [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	out := filepath.Join("assets", "meshes", "ladder", "mesh.glb")
	if err := gltf.SaveBinary(doc, out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: height %.1f, %d vertices, %d triangles", out, float32(ladderHeight), len(positions), len(indices)/3)
}

// woodTexture is a plain brown plank, the way stoneTexture in genstairs is
// plain grey rubble: dull on purpose, so nothing on a ladder gets mistaken for
// one of the map's lit fittings.
func woodTexture() []byte {
	random := rand.New(rand.NewSource(2))
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			shade := uint8(0x7A + random.Intn(0x14))
			if y%4 == 0 {
				shade -= 0x18
			}
			img.SetNRGBA(x, y, color.NRGBA{R: shade, G: uint8(float32(shade) * 0.68), B: uint8(float32(shade) * 0.42), A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
