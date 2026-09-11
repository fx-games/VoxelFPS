// Command genpanels writes the small props a level's intel is made of:
// assets/meshes/keypad/mesh.glb, an upright wall panel with a display and
// twelve keys on it, assets/meshes/note/mesh.glb, a sheet of paper lying face
// up, assets/meshes/audiolog/mesh.glb, a data cartridge lying on its side, and
// assets/meshes/sign/mesh.glb, a wall display the game writes on (sign.go).
// Each is one box with a picture on one face, which is why they are generated
// rather than modelled - see tools/genstairs for the same shape.
//
//	go run ./tools/genpanels
//
// Each box is textured out of a 64x64 atlas split down the middle: the left
// three quarters is the face that is meant to be read, the right quarter is a
// flat swatch every other face samples out of. One texture, no tiling - this
// engine's loader clamps rather than repeats, so a UV rect past the edge
// smears instead of wrapping (see AGENTS.md, "Modeling scenery in Blockbench").
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

// The keypad, in world units where a voxel is 1. A voxel is a shade over two
// metres, since the player is 0.875 of one and Doom's player is 56 units tall -
// so this is a panel about 40cm by 55cm, hung with its middle at 1.15m. That is
// bigger than a real keypad and smaller than a door: big enough to aim at from
// across a corridor, and at the height a hand reaches it rather than at the
// height a first-person camera happens to be.
//
// It is centred on its own origin in X and Z, so turning one in the editor
// turns it about its own middle, and its picture is on the face that looks
// along -Z - which is what "as modelled" means for a prop nobody has turned.
const (
	keypadWidth  = 0.20
	keypadHeight = 0.28
	keypadDepth  = 0.05
	keypadBottom = 0.42
)

// The note: a square of paper on a floor or a table top, oversized on purpose
// (about half a metre) so that a sheet lying on the ground is something you can
// see across a room rather than a speck. Thin enough to read as paper and thick
// enough that a ray along its plane still meets a face.
const (
	noteSize      = 0.32
	noteThickness = 0.012
)

// The audio log: a data cartridge lying on its side, read from above like the
// note. Its footprint is three units wide to four deep on purpose - that is
// exactly the shape of the atlas's readable face, so the label on its lid is
// drawn at the same pixels per unit on both axes and nothing stretches.
const (
	audioLogWidth  = 0.18
	audioLogDepth  = 0.24
	audioLogHeight = 0.05
)

// The sign: a display 1.8m by 0.6m, 3:1 like the picture the game paints on it
// (quads.SignPicture), hung like the keypad - origin on the floor, face along
// -Z - with its middle a little above eye height, which is also low enough to
// fit under the one-voxel ceiling of a cramped deck.
const (
	signWidth  = 0.9
	signHeight = 0.3
	signDepth  = 0.04
	signBottom = 0.58
)

// atlasSplit is where the readable face's picture ends and the flat swatch the
// other five faces sample begins, as a fraction of the texture's width.
const atlasSplit = 0.75

// face is one side of a box: which corners, which way it points, and which
// rectangle of the texture it reads.
type face struct {
	corner [4]int
	normal [3]float32
	uv     [4]float32 // u0, v0, u1, v1
}

// artFace is the whole left-hand picture; blankFace is a patch well inside the
// right-hand swatch, so no sampling ever lands on the seam between the two.
var (
	artFace   = [4]float32{0, 0, atlasSplit, 1}
	blankFace = [4]float32{0.82, 0.2, 0.95, 0.8}
)

// box appends the six faces of an axis aligned box, with art on the one face
// named by artNormal and the flat swatch on the other five.
func box(min, max [3]float32, artNormal [3]float32,
	positions, normals *[][3]float32, uvs *[][2]float32, indices *[]uint16) {
	corners := [8][3]float32{
		{min[0], min[1], min[2]}, {max[0], min[1], min[2]},
		{max[0], min[1], max[2]}, {min[0], min[1], max[2]},
		{min[0], max[1], min[2]}, {max[0], max[1], min[2]},
		{max[0], max[1], max[2]}, {min[0], max[1], max[2]},
	}
	// Counter-clockwise seen from outside, the same winding genstairs uses.
	faces := []face{
		{[4]int{7, 3, 2, 6}, [3]float32{0, 0, 1}, blankFace},
		{[4]int{5, 1, 0, 4}, [3]float32{0, 0, -1}, blankFace},
		{[4]int{6, 2, 1, 5}, [3]float32{1, 0, 0}, blankFace},
		{[4]int{4, 0, 3, 7}, [3]float32{-1, 0, 0}, blankFace},
		{[4]int{4, 7, 6, 5}, [3]float32{0, 1, 0}, blankFace},
		{[4]int{3, 0, 1, 2}, [3]float32{0, -1, 0}, blankFace},
	}
	for _, f := range faces {
		if f.normal == artNormal {
			f.uv = artFace
		}
		base := uint16(len(*positions))
		for _, c := range f.corner {
			*positions = append(*positions, corners[c])
			*normals = append(*normals, f.normal)
		}
		u0, v0, u1, v1 := f.uv[0], f.uv[1], f.uv[2], f.uv[3]
		*uvs = append(*uvs, [2]float32{u0, v0}, [2]float32{u0, v1}, [2]float32{u1, v1}, [2]float32{u1, v0})
		*indices = append(*indices, base, base+1, base+2, base, base+2, base+3)
	}
}

func main() {
	half := float32(keypadWidth) / 2
	var kPos, kNorm [][3]float32
	var kUV [][2]float32
	var kIdx []uint16
	// The face to read points along -Z, which is where a level's yaw zero looks
	// and so what "as modelled" means for a prop nobody has turned.
	box([3]float32{-half, keypadBottom, -keypadDepth / 2},
		[3]float32{half, keypadBottom + keypadHeight, keypadDepth / 2},
		[3]float32{0, 0, -1}, &kPos, &kNorm, &kUV, &kIdx)
	write("keypad", keypadTexture(), kPos, kNorm, kUV, kIdx)

	noteHalf := float32(noteSize) / 2
	var nPos, nNorm [][3]float32
	var nUV [][2]float32
	var nIdx []uint16
	// A note lies flat and is read from above, so its picture is on its top
	// face - which is also why it needs no facing: a square seen from overhead
	// reads the same however it was turned.
	box([3]float32{-noteHalf, 0, -noteHalf},
		[3]float32{noteHalf, noteThickness, noteHalf},
		[3]float32{0, 1, 0}, &nPos, &nNorm, &nUV, &nIdx)
	write("note", noteTexture(), nPos, nNorm, nUV, nIdx)

	logHalfX, logHalfZ := float32(audioLogWidth)/2, float32(audioLogDepth)/2
	var lPos, lNorm [][3]float32
	var lUV [][2]float32
	var lIdx []uint16
	// The lid is what is read, so the picture goes on the top face - the same
	// as the note, and for the same reason: a thing lying on the floor is seen
	// from above.
	box([3]float32{-logHalfX, 0, -logHalfZ},
		[3]float32{logHalfX, audioLogHeight, logHalfZ},
		[3]float32{0, 1, 0}, &lPos, &lNorm, &lUV, &lIdx)
	write("audiolog", audioLogTexture(), lPos, lNorm, lUV, lIdx)

	signHalf := float32(signWidth) / 2
	var sPos, sNorm [][3]float32
	var sUV [][2]float32
	var sIdx []uint16
	box([3]float32{-signHalf, signBottom, -signDepth / 2},
		[3]float32{signHalf, signBottom + signHeight, signDepth / 2},
		[3]float32{0, 0, -1}, &sPos, &sNorm, &sUV, &sIdx)
	write("sign", signTexture(), sPos, sNorm, sUV, sIdx)
}

func write(name string, texture []byte, positions, normals [][3]float32, uvs [][2]float32, indices []uint16) {
	doc := gltf.NewDocument()
	imageIndex, err := modeler.WriteImage(doc, name, "image/png", bytes.NewReader(texture))
	if err != nil {
		log.Fatal(err)
	}
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest, MinFilter: gltf.MinNearest,
		WrapS: gltf.WrapClampToEdge, WrapT: gltf.WrapClampToEdge,
	})
	doc.Textures = append(doc.Textures, &gltf.Texture{Sampler: gltf.Index(0), Source: gltf.Index(imageIndex)})
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name: name, AlphaMode: gltf.AlphaOpaque, DoubleSided: false,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})
	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: name,
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
		Name: name, Mesh: gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1}, Scale: [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	dir := filepath.Join("assets", "meshes", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	out := filepath.Join(dir, "mesh.glb")
	if err := gltf.SaveBinary(doc, out); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s: %d vertices, %d triangles", out, len(positions), len(indices)/3)
}

// keypadTexture draws the panel: a dark casing, a lit display across the top
// and twelve keys under it. The display is the only bright, saturated thing on
// it, which is what the mesh shader's emissive pass picks out - so the panel
// reads as powered without the casing glowing with it.
func keypadTexture() []byte {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	casing := color.NRGBA{0x3A, 0x3E, 0x44, 0xFF}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, casing)
		}
	}
	// The bezel: a lighter border round the readable face only.
	rect(img, 1, 1, 46, 62, color.NRGBA{0x54, 0x5A, 0x62, 0xFF})
	rect(img, 3, 3, 44, 60, casing)
	// The display, amber and bright, with four dashes waiting for a code.
	rect(img, 6, 6, 41, 17, color.NRGBA{0x14, 0x10, 0x06, 0xFF})
	for i := 0; i < 4; i++ {
		rect(img, 10+i*8, 13, 15+i*8, 14, color.NRGBA{0xFF, 0xB4, 0x28, 0xFF})
	}
	// Twelve keys, three across and four down.
	for row := 0; row < 4; row++ {
		for column := 0; column < 3; column++ {
			x := 7 + column*12
			y := 22 + row*9
			rect(img, x, y, x+9, y+6, color.NRGBA{0x6B, 0x71, 0x79, 0xFF})
			rect(img, x+1, y+1, x+8, y+5, color.NRGBA{0x86, 0x8D, 0x96, 0xFF})
		}
	}
	// The flat swatch the other five faces read, with a little noise so the
	// casing does not read as a solid block of one colour.
	random := rand.New(rand.NewSource(3))
	for y := 0; y < size; y++ {
		for x := int(atlasSplit * size); x < size; x++ {
			shade := uint8(0x30 + random.Intn(0x10))
			img.SetNRGBA(x, y, color.NRGBA{shade, shade, uint8(int(shade) + 6), 0xFF})
		}
	}
	return encode(img)
}

// noteTexture draws a sheet of paper with writing on it. The writing is
// deliberately unreadable scribble: what a note says is its text, which the
// game prints when the note is read, and a picture that tried to say it too
// would be a picture that goes stale the first time the text is edited.
func noteTexture() []byte {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	paper := color.NRGBA{0xD8, 0xD2, 0xBE, 0xFF}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, paper)
		}
	}
	rect(img, 0, 0, 47, 63, color.NRGBA{0xB2, 0xAC, 0x98, 0xFF})
	rect(img, 2, 2, 45, 61, paper)
	random := rand.New(rand.NewSource(7))
	ink := color.NRGBA{0x3B, 0x35, 0x2C, 0xFF}
	for line := 0; line < 9; line++ {
		y := 8 + line*6
		end := 12 + random.Intn(28)
		rect(img, 7, y, 7+end, y+1, ink)
	}
	// The right-hand swatch is the paper's own edge colour, so the four sides
	// of the sheet read as the thickness of paper rather than as a border.
	for y := 0; y < size; y++ {
		for x := int(atlasSplit * size); x < size; x++ {
			img.SetNRGBA(x, y, color.NRGBA{0xC4, 0xBE, 0xAA, 0xFF})
		}
	}
	return encode(img)
}

// audioLogTexture draws the cartridge lid: a dark shell, a paper label with the
// same unreadable scribble the note has - what a log says is what it plays, and
// a picture that tried to say it too would go stale - and one green running
// light. The light is the only bright, saturated thing on it, which is what the
// mesh shader's emissive pass picks out, so the cartridge reads as powered
// while its shell stays dark. Same trick as the keypad's display.
func audioLogTexture() []byte {
	const size = 64
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	shell := color.NRGBA{0x2C, 0x30, 0x38, 0xFF}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, shell)
		}
	}
	// The lid's raised rim, and the two spool windows a tape cartridge has.
	rect(img, 1, 1, 46, 62, color.NRGBA{0x45, 0x4A, 0x54, 0xFF})
	rect(img, 3, 3, 44, 60, shell)
	for _, centre := range []int{18, 46} {
		rect(img, 14, centre-6, 33, centre+6, color.NRGBA{0x16, 0x18, 0x1D, 0xFF})
		rect(img, 20, centre-2, 27, centre+2, color.NRGBA{0x5A, 0x60, 0x6A, 0xFF})
	}
	// The label across the middle, with a line of writing on it.
	rect(img, 6, 27, 41, 37, color.NRGBA{0xCE, 0xC6, 0xAE, 0xFF})
	random := rand.New(rand.NewSource(11))
	ink := color.NRGBA{0x3B, 0x35, 0x2C, 0xFF}
	for line := 0; line < 2; line++ {
		y := 30 + line*4
		rect(img, 9, y, 9+14+random.Intn(14), y+1, ink)
	}
	// The running light.
	rect(img, 6, 6, 12, 10, color.NRGBA{0x28, 0xFF, 0x64, 0xFF})
	// The flat swatch the other five faces read: the shell, roughened a little
	// so the sides do not read as one block of colour.
	for y := 0; y < size; y++ {
		for x := int(atlasSplit * size); x < size; x++ {
			shade := uint8(0x26 + random.Intn(0x0C))
			img.SetNRGBA(x, y, color.NRGBA{shade, shade, uint8(int(shade) + 8), 0xFF})
		}
	}
	return encode(img)
}

// signTexture is a blank sign, which is what one looks like before the game has
// written on it and what the editor's browser shows: casing, bezel, a dark
// display with three amber dashes on it. The game paints over the whole of it
// with quads.SignPicture, which keeps this layout.
func signTexture() []byte {
	const width, height = 512, 128
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	display := int(atlasSplit*width) - 1
	rect(img, 0, 0, width-1, height-1, color.NRGBA{58, 64, 68, 0xFF})
	rect(img, 3, 3, display-3, height-4, color.NRGBA{28, 33, 36, 0xFF})
	rect(img, 7, 7, display-7, height-8, color.NRGBA{8, 14, 16, 0xFF})
	for i := 0; i < 3; i++ {
		x := display/2 - 70 + i*56
		rect(img, x, height/2-4, x+28, height/2+4, color.NRGBA{255, 184, 72, 0xFF})
	}
	return encode(img)
}

// rect fills an inclusive rectangle.
func rect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func encode(img *image.NRGBA) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}
