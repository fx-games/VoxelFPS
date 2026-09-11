package main

// The 3D models: what obj3d.res holds, turned into the glTF this engine's mesh
// loader reads, and filed under assets/meshes so they are furniture like
// everything else there.
//
// Every model shares one picture. A face is either mapped to one of the
// sixty-four object textures in citmat.res or painted a single palette colour,
// and both are put in the same atlas - the textures in a grid at the top, the
// whole palette as a strip of swatches under them - so a model is one material
// and one draw rather than one per face colour.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/inkyblackness/hacked/ss1/content/object"
	"github.com/inkyblackness/hacked/ss1/resource"
	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
)

const (
	// modelsResource is the first of the models; the rest follow it.
	modelsResource = resource.ID(0x08FC)
	// objectTextures is the first of the sixty-four pictures a model's faces
	// are mapped to, in citmat.res.
	objectTextures     = resource.ID(0x01DB)
	objectTextureCount = 64
	// meshDir is where a model folder goes. It is the engine's own furniture
	// folder: a folder with a scenery.json in it is a piece you can place.
	meshDir = "assets/meshes"
)

// The shared atlas: an eight by eight grid of the object textures, and under it
// the palette as swatches, sixty-four to a row.
const (
	atlasCell     = 64
	atlasColumns  = 8
	atlasTexArea  = atlasCell * atlasColumns // 512, and square
	swatchSize    = 8
	swatchColumns = atlasTexArea / swatchSize // 64
	swatchRows    = 256 / swatchColumns       // 4
	atlasHeight   = atlasTexArea + swatchRows*swatchSize
)

// modelScale is how many voxels one unit of a model's own space is.
//
// A model is drawn by the same g3 pipeline as everything else in the station
// and in the same world units: its vertices are fix 16.16 (see readFixed), and
// one of those units is one map square - obj_coord_from_fix in map.h is a
// shift of eight off a coordinate whose top byte is the square, and
// gameobj.c's _o_rad divides physics_xr by the 96 radius units a square has.
// A map square is a voxel here, so the number is one.
//
// It was two, and that is what made every piece of the station's furniture
// twice the size it should be: a chair 2.3 metres tall, a couch as tall as the
// mutant standing next to it. The same trace fixes both imports at once - a
// sprite pixel is a ninety-sixth of a unit (pixelsPerVoxel in install.go) and
// a model unit is one - so 96 pixels of art, one model unit and one voxel are
// all the same length, and the sprites and the models cannot drift apart.
//
// ponytail: one number for every model. The real per-object size is PhysicsXR
// and PhysicsZ in the common properties, in units nothing here shares; read
// those and convert if a particular piece ever has to be a size its own
// geometry does not imply.
const modelScale = 1.0

// installModels writes a folder per model: the glTF, the shared picture, and a
// scenery.json that makes it a piece the editor can place.
func (in *installer) installModels() (int, error) {
	reader := in.art["OBJ3D.RES"]
	if reader == nil {
		return 0, nil
	}
	atlas, err := in.modelAtlas()
	if err != nil {
		return 0, err
	}
	atlasPNG := pngBytes(atlas)
	names, triples := in.modelNames()

	written := 0
	for _, id := range reader.IDs() {
		view, err := reader.View(id)
		if err != nil || view.BlockCount() == 0 {
			continue
		}
		block, err := view.Block(0)
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(block)
		if err != nil {
			continue
		}
		model, err := decodeModel3D(raw)
		if err != nil || len(model.faces) == 0 {
			continue
		}
		index := int(id.Value() - modelsResource.Value())
		name := names[index]
		folder := fmt.Sprintf("ss3d_%02d", index)
		if name == "" {
			// Seventeen of the eighty are drawn by no object in the property
			// table, so nothing names them and the resource number is the name.
			name = fmt.Sprintf("Model %02d", index)
		} else {
			folder = fmt.Sprintf("ss3d_%02d_%s", index, slug(name))
		}
		in.taken[folder] = true
		triple, named := triples[index]
		folder = path.Join(modelCategory(triple, named), folder)

		if err := writeModelFolder(folder, name, model, atlasPNG); err != nil {
			return written, fmt.Errorf("%s: %w", folder, err)
		}
		written++
	}
	return written, nil
}

// modelNames pairs a model with the object that is drawn as it. The link is the
// object's own MFD/Mesh ID: for an object in a panel that field says which
// readout to open, and for one in the world it says which mesh to render.
//
// The render type that means "drawn as a textured polygon" is TextPoly, which
// reads like it is about writing and is not: sixty-four of the sixty-five
// objects with that type name a mesh, and none of TPoly, FlatPoly or TLPoly
// name one at all.
//
// A mesh two objects share keeps the first name, which is why there is a DATA
// BOLT and not a DRILL BIT.
func (in *installer) modelNames() (map[int]string, map[int]object.Triple) {
	names := map[int]string{}
	triples := map[int]object.Triple{}
	in.properties.Iterate(func(triple object.Triple, prop *object.Properties) bool {
		if prop.Common.RenderType != object.RenderTypeTextPoly {
			return true
		}
		index := int(prop.Common.MfdOrMeshID)
		if _, taken := names[index]; !taken {
			names[index] = in.objectName(triple)
			triples[index] = triple
		}
		return true
	})
	return names, triples
}

// modelCategory is the folder a model is filed under, out of the class and
// subclass of the object drawn as it. The property table is the game's own
// taxonomy of its furniture and it is already the answer: subclass 0 of the
// decorations is the electronics, subclass 1 the furniture, and so on down to
// the cyberspace set, which is three whole classes rather than a subclass.
// Sorting eighty models by hand would be the same list, guessed at.
//
// A folder rather than a flat eighty because the editor browses this directory:
// nine chairs and desks under "furniture" is a page you can look at, and eighty
// models named after their resource number is not.
func modelCategory(triple object.Triple, named bool) string {
	if !named {
		// Seventeen models are drawn by no object in the table. Whatever they
		// were for, nothing in the game reaches them, and guessing a category
		// off the resource number next door files half of them wrong - the
		// three after the cyberspace panels are cyberspace geometry, not panels.
		return "unused"
	}
	switch {
	case triple.Class == decorationClass:
		switch triple.Subclass {
		case 0:
			return "electronics"
		case 1:
			return "furniture"
		case 4:
			return "medical"
		case 5:
			return "lab"
		}
		// Subclass 6 is the rocks and 7 the fixtures - boulders, walls, rails
		// and a pillar. Both are the shape of the room rather than something
		// standing in it.
		return "structure"
	case triple.Class == panelClass:
		return "terminals"
	}
	// What is left is classes 2, 8 and 14 - the projectiles, the cyberspace
	// items and the cyberspace critters - and every one of them is a model that
	// only ever appears inside the cyberspace levels.
	return "cyberspace"
}

// The two object classes the models are sorted by name. The rest are only
// reached by the default in modelCategory, which is why they have no constant.
const (
	// decorationClass is class seven, the furniture and fittings of the station.
	decorationClass = object.Class(7)
	// panelClass is class nine: the terminals and readouts mounted on walls.
	panelClass = object.Class(9)
)

// modelAtlas builds the one picture every model is drawn with.
func (in *installer) modelAtlas() (image.Image, error) {
	atlas := image.NewRGBA(image.Rect(0, 0, atlasTexArea, atlasHeight))
	for i := 0; i < objectTextureCount; i++ {
		source := objectTextures.Plus(i)
		if i == objectTextureDefault {
			source = defaultObjectTexture
		}
		img, _ := in.blockImage("CITMAT.RES", source, 0, false, nil)
		if img == nil {
			continue
		}
		at := image.Rect(
			(i%atlasColumns)*atlasCell, (i/atlasColumns)*atlasCell,
			(i%atlasColumns+1)*atlasCell, (i/atlasColumns+1)*atlasCell,
		)
		// Stretched to fill the cell, not dropped into a corner of it. Only a
		// handful of these pictures are 64 by 64; most are 32 by 32, and there
		// are 64x32, 32x64 and 64x16 among them. A face's texture coordinates
		// run 0 to 1 across whatever size its picture is, so laying a 32x32 one
		// in the top left of a 64 cell means three quarters of every face
		// sampling the empty rest of the cell - which is what made a wall of
		// oscilloscopes and control panels come out as blank white slabs.
		drawStretched(atlas, at, img)
	}
	for i := 0; i < 256; i++ {
		at := image.Rect(
			(i%swatchColumns)*swatchSize, atlasTexArea+(i/swatchColumns)*swatchSize,
			(i%swatchColumns+1)*swatchSize, atlasTexArea+(i/swatchColumns+1)*swatchSize,
		)
		draw.Draw(atlas, at, &image.Uniform{C: in.opaque[i]}, image.Point{}, draw.Src)
	}
	return atlas, nil
}

// textureUV is where a point on one of the object textures lands on the atlas.
// Texture identifier zero is the format's "special, object specific" case,
// which has no picture of its own; it takes the first cell.
func textureUV(texture int, u, v float32) [2]float32 {
	// The identifier is the citmat resource offset itself: the game binds each
	// one with g3_set_vtext(id, resource 0x01DB + id) and a textured face names
	// the same id - see load_model_vtexts in the source release's rendtool.c,
	// and Geometry.md, which says the same thing. There is no adjustment to
	// make. Subtracting one here, which is what this did, shifted every model's
	// textures along by a slot: a desk came out wearing the wall panel next to
	// it in the sheet, and every model in the game was wrong the same way.
	if texture < 0 || texture >= objectTextureCount {
		texture = 0
	}
	cell := texture
	x := float32(cell%atlasColumns)*atlasCell + insetInCell(u)
	y := float32(cell/atlasColumns)*atlasCell + insetInCell(v)
	return [2]float32{x / atlasTexArea, y / atlasHeight}
}

// halfTexel is how far inside its own cell a face's edge is held, in atlas
// pixels. Every cell's neighbour is a different picture, and a coordinate
// sitting exactly on the boundary is sampled half from each - the thin wrong
// stripe down the side of a face.
const halfTexel = 0.5

// insetInCell turns one of a face's own coordinates into a position inside its
// cell, in atlas pixels.
//
// The model's coordinates are already 0 to 1 - none of the eighty models tiles
// a texture, and what runs outside is a few thousandths of overshoot on a face
// whose corners were meant to be the corners of the picture. So this holds them
// to the cell rather than wrapping them round it, which is what it used to do:
// fmod turned the far edge of a face, at exactly 1, into 0, and every face
// mapped to the whole of its texture was drawn as a smear of its first column
// instead. That is the last of "the UVs are slightly off".
func insetInCell(value float32) float32 {
	pixel := value * atlasCell
	if pixel < halfTexel {
		return halfTexel
	}
	if pixel > atlasCell-halfTexel {
		return atlasCell - halfTexel
	}
	return pixel
}

// swatchUV is the middle of one palette swatch, which is what a face painted a
// single colour is mapped to.
func swatchUV(index byte) [2]float32 {
	x := float32(int(index)%swatchColumns)*swatchSize + swatchSize/2
	y := float32(atlasTexArea) + float32(int(index)/swatchColumns)*swatchSize + swatchSize/2
	return [2]float32{x / atlasTexArea, y / atlasHeight}
}

// writeModelFolder turns one decoded model into a folder the engine can spawn.
func writeModelFolder(folder, name string, model *model3D, atlasPNG []byte) error {
	dir := filepath.Join(meshDir, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var positions, normals [][3]float32
	var uvs [][2]float32
	var indices []uint16
	for _, face := range model.faces {
		base := uint16(len(positions))
		for corner, index := range face.indices {
			positions = append(positions, engineSpace(model.vertices[index]))
			normals = append(normals, engineSpace(face.normal))
			if face.texture >= 0 && corner < len(face.uvs) {
				uvs = append(uvs, textureUV(face.texture, face.uvs[corner][0], face.uvs[corner][1]))
			} else {
				uvs = append(uvs, swatchUV(face.colour))
			}
		}
		// A polygon of any number of corners, as a fan from its first.
		for corner := 1; corner+1 < len(face.indices); corner++ {
			indices = append(indices, base, base+uint16(corner), base+uint16(corner+1))
		}
	}
	if len(indices) == 0 {
		return nil
	}
	// Sit the model on the floor. The game placed these from its own level
	// data and their origins are wherever that data wanted them - some are
	// centred, some hang from the top - so a row of them dropped at one height
	// is a row where half float. Lowering each by its own lowest point means a
	// piece placed at floor level stands on the floor.
	lowest := positions[0][1]
	for _, p := range positions {
		if p[1] < lowest {
			lowest = p[1]
		}
	}
	for i := range positions {
		positions[i][1] -= lowest
	}

	doc := gltf.NewDocument()
	imageIndex, err := modeler.WriteImage(doc, folder, "image/png", bytes.NewReader(atlasPNG))
	if err != nil {
		return err
	}
	doc.Samplers = append(doc.Samplers, &gltf.Sampler{
		MagFilter: gltf.MagNearest, MinFilter: gltf.MinNearest,
		WrapS: gltf.WrapClampToEdge, WrapT: gltf.WrapClampToEdge,
	})
	doc.Textures = append(doc.Textures, &gltf.Texture{
		Sampler: gltf.Index(0), Source: gltf.Index(imageIndex),
	})
	doc.Materials = append(doc.Materials, &gltf.Material{
		Name:      folder,
		AlphaMode: gltf.AlphaOpaque,
		// Both sides: several of these models are open shells the game only
		// ever showed from outside, and a shelf drawn one-sided is a shelf with
		// a hole in it from the wrong angle.
		DoubleSided: true,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorTexture: &gltf.TextureInfo{Index: 0},
			MetallicFactor:   gltf.Float(0),
			RoughnessFactor:  gltf.Float(1),
		},
	})
	doc.Meshes = append(doc.Meshes, &gltf.Mesh{
		Name: folder,
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
		// The leaf, not the category folder in front of it: this names the node
		// inside the file, and it is the model that is named, not where it is filed.
		Name: path.Base(folder), Mesh: gltf.Index(0),
		Rotation: [4]float64{0, 0, 0, 1}, Scale: [3]float64{1, 1, 1},
	})
	doc.Scenes[0].Nodes = []int{0}

	if err := gltf.SaveBinary(doc, filepath.Join(dir, "mesh.glb")); err != nil {
		return err
	}
	// A scenery.json is what makes a folder of meshes a piece the editor can
	// place. No colliders: one box round the whole model is what a table wants,
	// and every one of these is roughly a box.
	scenery := map[string]any{"name": name}
	data, err := json.MarshalIndent(scenery, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "scenery.json"), append(data, '\n'), 0o644)
}

// engineSpace turns System Shock's axes into this engine's. Its vertical is the
// second coordinate and it counts downwards - every model in the file runs from
// zero at the top to a negative number at the bottom - so the sign flips and the
// object stands up out of the floor rather than hanging under it.
func engineSpace(v [3]float32) [3]float32 {
	return [3]float32{v[0] * modelScale, -v[1] * modelScale, v[2] * modelScale}
}

// objectTextureDefault is texture 0, and it is not a texture the way the other
// sixty-three are. A face that names it does not get citmat material 0 - which
// is a purple tile with a red C on it, a placeholder the game never draws.
// It gets a picture chosen from the placed object itself: the camera feed on a
// monitor, the readout on a panel, the covering on a chair, worked out by
// bitmap_from_tpoly_data from data that lives on the object in the level.
//
// There is no placed object here, only the model, so the game's own default is
// what there is to use. A piece of furniture with nothing said about it takes
// SECRET_FURNITURE_DEFAULT_O3DREP, which is 0x80 - custom material zero - and
// that is defaultObjectTexture below. See get_o3drep and bitmap_from_tpoly_data
// in the source release's gameobj.c and objsim.c.
//
// It matters more than one texture in sixty-four sounds like: texture 0 is on
// more faces than any other, and with the placeholder in that slot every couch,
// chair, monitor and control panel in the import came out magenta.
const objectTextureDefault = 0

// defaultObjectTexture is custom material zero, in the same file as the object
// materials: a teal panel, and what the game puts on a piece of furniture that
// says nothing about what it is covered in.
const defaultObjectTexture = resource.ID(0x0884)

// drawStretched copies an image into a rectangle at whatever size that
// rectangle is, nearest neighbour. Nearest because these are DOS textures and a
// smooth filter on a 32 pixel tile of pixel art is a blur of it; and by hand
// because that is the whole of what golang.org/x/image/draw would be pulled in
// for.
func drawStretched(dst *image.RGBA, at image.Rectangle, src image.Image) {
	bounds := src.Bounds()
	if bounds.Empty() || at.Empty() {
		return
	}
	for y := 0; y < at.Dy(); y++ {
		sourceY := bounds.Min.Y + y*bounds.Dy()/at.Dy()
		for x := 0; x < at.Dx(); x++ {
			sourceX := bounds.Min.X + x*bounds.Dx()/at.Dx()
			dst.Set(at.Min.X+x, at.Min.Y+y, src.At(sourceX, sourceY))
		}
	}
}
