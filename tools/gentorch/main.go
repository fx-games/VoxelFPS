// Command gentorch writes assets/meshes/torch/mesh.glb: a Minecraft-style torch as six
// quads and one 16x16 texture, embedded in the .glb so the mesh loader's buffer
// path picks it up without a side-car PNG.
//
// The model is generated rather than modelled because it is a box: everything that
// makes it a torch is in the twelve numbers below and in the texture. Run with
// `go run ./tools/gentorch` from the repository root after changing either.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"path/filepath"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// Minecraft proportions, in world units where a block is 1: a 2x2 pixel stick,
// 10 pixels tall, standing on the floor.
const (
	halfWidth = 1.0 / 16.0
	height    = 10.0 / 16.0
)

type face struct {
	normal, right, up [3]float32
	center            [3]float32
	halfW, halfH      float32
	// The texture rectangle this face samples, in UV space.
	u0, v0, u1, v1 float32
}

// UV rectangles into the 16x16 texture below. glTF has v pointing down, so v0 is
// the top of the image - which is where the flame is, and where the top of the
// torch has to land.
const px = 1.0 / 16.0

var (
	sideUV   = [4]float32{6 * px, 6 * px, 8 * px, 16 * px}
	topUV    = [4]float32{6 * px, 4 * px, 8 * px, 6 * px}
	bottomUV = [4]float32{6 * px, 2 * px, 8 * px, 4 * px}
)

// The six faces. `normal` must equal `right` x `up` - that is what makes the
// vertex order below counter-clockwise seen from outside, so front-face culling
// keeps the torch and not its inside.
var faces = []face{
	{normal: [3]float32{1, 0, 0}, right: [3]float32{0, 0, -1}, up: [3]float32{0, 1, 0},
		center: [3]float32{halfWidth, height / 2, 0}, halfW: halfWidth, halfH: height / 2,
		u0: sideUV[0], v0: sideUV[1], u1: sideUV[2], v1: sideUV[3]},
	{normal: [3]float32{-1, 0, 0}, right: [3]float32{0, 0, 1}, up: [3]float32{0, 1, 0},
		center: [3]float32{-halfWidth, height / 2, 0}, halfW: halfWidth, halfH: height / 2,
		u0: sideUV[0], v0: sideUV[1], u1: sideUV[2], v1: sideUV[3]},
	{normal: [3]float32{0, 0, 1}, right: [3]float32{1, 0, 0}, up: [3]float32{0, 1, 0},
		center: [3]float32{0, height / 2, halfWidth}, halfW: halfWidth, halfH: height / 2,
		u0: sideUV[0], v0: sideUV[1], u1: sideUV[2], v1: sideUV[3]},
	{normal: [3]float32{0, 0, -1}, right: [3]float32{-1, 0, 0}, up: [3]float32{0, 1, 0},
		center: [3]float32{0, height / 2, -halfWidth}, halfW: halfWidth, halfH: height / 2,
		u0: sideUV[0], v0: sideUV[1], u1: sideUV[2], v1: sideUV[3]},
	{normal: [3]float32{0, 1, 0}, right: [3]float32{1, 0, 0}, up: [3]float32{0, 0, -1},
		center: [3]float32{0, height, 0}, halfW: halfWidth, halfH: halfWidth,
		u0: topUV[0], v0: topUV[1], u1: topUV[2], v1: topUV[3]},
	{normal: [3]float32{0, -1, 0}, right: [3]float32{1, 0, 0}, up: [3]float32{0, 0, 1},
		center: [3]float32{0, 0, 0}, halfW: halfWidth, halfH: halfWidth,
		u0: bottomUV[0], v0: bottomUV[1], u1: bottomUV[2], v1: bottomUV[3]},
}

func main() {
	positions, normals, uvs, indices := buildQuads()

	doc := gltf.NewDocument()

	imageIndex, err := modeler.WriteImage(doc, "torch", "image/png", bytes.NewReader(torchTexture()))
	if err != nil {
		log.Fatal(err)
	}
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest, MinFilter: gltf.MinNearest,
		WrapS: gltf.WrapClampToEdge, WrapT: gltf.WrapClampToEdge,
	})
	doc.Textures = append(doc.Textures, &gltf.Texture{
		Sampler: gltf.Index(0), Source: gltf.Index(imageIndex),
	})
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name:        "torch",
		AlphaMode:   gltf.AlphaMask,
		DoubleSided: false,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})

	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: "torch",
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
		Name:     "torch",
		Mesh:     gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1},
		Scale:    [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	out := filepath.Join("assets", "meshes", "torch", "mesh.glb")
	if err := gltf.SaveBinary(doc, out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d quads, %d vertices, %d triangles", out, len(faces), len(positions), len(indices)/3)
}

func buildQuads() (positions, normals [][3]float32, uvs [][2]float32, indices []uint16) {
	for _, f := range faces {
		corner := func(rightSign, upSign float32) [3]float32 {
			return [3]float32{
				f.center[0] + f.right[0]*f.halfW*rightSign + f.up[0]*f.halfH*upSign,
				f.center[1] + f.right[1]*f.halfW*rightSign + f.up[1]*f.halfH*upSign,
				f.center[2] + f.right[2]*f.halfW*rightSign + f.up[2]*f.halfH*upSign,
			}
		}
		base := uint16(len(positions))
		// top-left, bottom-left, bottom-right, top-right - counter-clockwise from
		// outside, with the UV rectangle laid on the same way round.
		positions = append(positions, corner(-1, 1), corner(-1, -1), corner(1, -1), corner(1, 1))
		uvs = append(uvs, [2]float32{f.u0, f.v0}, [2]float32{f.u0, f.v1}, [2]float32{f.u1, f.v1}, [2]float32{f.u1, f.v0})
		for i := 0; i < 4; i++ {
			normals = append(normals, f.normal)
		}
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)
	}
	return
}

// torchTexture paints the 16x16 sheet: a dark wooden stick with a lit head. The
// wood is kept dark on purpose - the mesh shader picks the glowing texels out by
// brightness and colourfulness, the same test the cube shader uses, so a stick as
// bright as the flame would glow along its whole length.
func torchTexture() []byte {
	var (
		transparent = color.NRGBA{}
		woodDark    = color.NRGBA{R: 0x3A, G: 0x28, B: 0x18, A: 0xFF}
		woodLight   = color.NRGBA{R: 0x4E, G: 0x36, B: 0x20, A: 0xFF}
		flameCore   = color.NRGBA{R: 0xFF, G: 0xF2, B: 0xB0, A: 0xFF}
		flameMid    = color.NRGBA{R: 0xFF, G: 0xC0, B: 0x40, A: 0xFF}
		flameEdge   = color.NRGBA{R: 0xFF, G: 0x86, B: 0x18, A: 0xFF}
	)

	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, transparent)
		}
	}
	set := func(x, y int, c color.NRGBA) { img.SetNRGBA(x, y, c) }

	// Bottom cap (rows 2-3): the charred end of the stick.
	for y := 2; y < 4; y++ {
		set(6, y, woodDark)
		set(7, y, woodDark)
	}
	// Top cap (rows 4-5): the flame seen from above.
	set(6, 4, flameMid)
	set(7, 4, flameCore)
	set(6, 5, flameCore)
	set(7, 5, flameMid)

	// The side strip (rows 6-15): three rows of flame over seven of wood.
	sideRows := []struct {
		left, right color.NRGBA
	}{
		{flameEdge, flameMid},
		{flameCore, flameCore},
		{flameMid, flameEdge},
		{woodLight, woodDark},
		{woodDark, woodLight},
		{woodLight, woodDark},
		{woodDark, woodDark},
		{woodLight, woodDark},
		{woodDark, woodLight},
		{woodDark, woodDark},
	}
	for i, row := range sideRows {
		set(6, 6+i, row.left)
		set(7, 6+i, row.right)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
