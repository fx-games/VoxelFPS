// Command genstairs writes scenery/stairs/mesh.glb: a four step staircase, each
// step a solid box from the floor up, with one 16x16 stone texture embedded in
// the .glb the way gentorch does it.
//
// It is generated rather than modelled because it is four boxes, and because the
// step rise has to stay under the engine's step height or the stairs stop being
// stairs. Run with `go run ./tools/genstairs` from the repository root.
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

// The staircase, in world units where a voxel is 1. Must match the boxes in
// scenery/stairs/scenery.json, which is where physics reads the same steps.
// The rise has to stay below the physics system's step height: at 0.25 against a
// step height of 0.289 there is room for it to be raised without the stairs
// turning into a wall.
const (
	stepRise  = 0.25
	stepRun   = 0.5
	stepCount = 4
	stepWidth = 1.0
	// The front face of the bottom step, so that a staircase spawned at a tile
	// centre spans exactly two tiles and its top lands on the tile boundary.
	frontZ = -0.5
)

// box appends the six faces of an axis aligned box. Every face gets the whole
// texture, which for a stone block is all the mapping it needs.
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

func buildStairs() (positions, normals [][3]float32, uvs [][2]float32, indices []uint16) {
	half := float32(stepWidth) / 2
	for i := 0; i < stepCount; i++ {
		// Each step runs from the floor to its own top, so the staircase is
		// solid instead of a flight of floating slabs.
		min := [3]float32{-half, 0, frontZ + float32(i)*stepRun}
		max := [3]float32{half, float32(i+1) * stepRise, frontZ + float32(i+1)*stepRun}
		box(min, max, &positions, &normals, &uvs, &indices)
	}
	return
}

func main() {
	positions, normals, uvs, indices := buildStairs()

	doc := gltf.NewDocument()

	imageIndex, err := modeler.WriteImage(doc, "stairs", "image/png", bytes.NewReader(stoneTexture()))
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
		Name:        "stairs",
		AlphaMode:   gltf.AlphaOpaque,
		DoubleSided: false,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})
	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: "stairs",
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
		Name:     "stairs",
		Mesh:     gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1},
		Scale:    [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	out := filepath.Join("scenery", "stairs", "mesh.glb")
	if err := gltf.SaveBinary(doc, out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d steps, %d vertices, %d triangles", out, stepCount, len(positions), len(indices)/3)
}

// stoneTexture is grey rubble. Grey on purpose: the mesh shader picks glowing
// texels out by brightness and colourfulness, and a staircase is not a light.
func stoneTexture() []byte {
	random := rand.New(rand.NewSource(1))
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			shade := uint8(0x6A + random.Intn(0x18))
			// A darker line along two edges reads as the seam between blocks and
			// makes the individual steps legible from a distance.
			if x == 0 || y == 0 {
				shade -= 0x20
			}
			img.SetNRGBA(x, y, color.NRGBA{R: shade, G: shade, B: uint8(float32(shade) * 0.96), A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
