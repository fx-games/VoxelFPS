package quads

import (
	bytes2 "bytes"
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxelfps/quads/glhf"
	"github.com/qmuntal/gltf"
	"github.com/qmuntal/gltf/modeler"
	_ "image/png"
	"log"
)

func LoadGLTFWithTextures(filename string) *CompoundMesh {
	result := LoadGLTF(filename, nil, nil)
	doc, err := gltf.Open(filename)
	if err != nil {
		println(err.Error())
		return nil
	}
	result.textures = tryLoadTextures(doc)
	return result
}

func LoadGLTFWithAnimationAndTextures(filename string, animationMap map[string]string) *CompoundMesh {
	result := LoadGLTF(filename, animationMap, nil)
	doc, err := gltf.Open(filename)
	if err != nil {
		println(err.Error())
		return nil
	}
	result.textures = tryLoadTextures(doc)
	return result
}

func LoadGLTFWithTexturesAndExtrusion(filename string) *CompoundMesh {
	result := LoadGLTF(filename, nil, nil)
	doc, err := gltf.Open(filename)
	if err != nil {
		println(err.Error())
		return nil
	}
	result.textures = tryLoadTextures(doc)
	return result
}

func LoadGLTF(filename string, animationMap map[string]string, forcedVertexColor *mgl32.Vec3) *CompoundMesh {
	doc, err := gltf.Open(filename)
	if err != nil {
		println(err.Error())
		return nil
	}
	defaultSceneIndex := 0
	if doc.Scene != nil {
		defaultSceneIndex = int(*doc.Scene)
	}
	defaultScene := doc.Scenes[defaultSceneIndex]
	//fmt.Println(fmt.Sprintf("[loadGLTF] Loading scene '%s' (1/%d)", defaultScene.name, len(doc.Scenes)))
	//fmt.Println(fmt.Sprintf("[loadGLTF] Scene contains %d node(s)", len(defaultScene.Nodes)))
	result := &CompoundMesh{
		SamplerFrames:  make(map[string][][]float32),
		animationSpeed: 1.0,
	}

	flatMeshes := make([]*SimpleMesh, len(doc.Meshes))
	for meshIndex, _ := range doc.Meshes {
		flatMeshes[meshIndex] = loadMesh(doc, uint32(meshIndex), forcedVertexColor)
	}
	flatNodes := make([]*MeshNode, len(doc.Nodes))
	for nodeIndex, node := range doc.Nodes {
		flatNodes[nodeIndex] = &MeshNode{
			name:         node.Name,
			animations:   make(map[string]*SimpleAnimationData),
			quatRotation: mgl32.QuatIdent(),
			translation:  mgl32.Vec3{0, 0, 0},
			scale:        mgl32.Vec3{1, 1, 1},
		}
		if node.Mesh != nil {
			flatNodes[nodeIndex].mesh = flatMeshes[*node.Mesh]
		}
	}
	for _, anim := range doc.Animations {
		//println(fmt.Sprintf("\nFound Animation: %s", anim.GameIdentifier))
		samplerFrames := make([][]float32, len(anim.Samplers))
		//samplerOutput := make([][][4]float32, len(anim.Samplers))
		var duration float32
		for samplerIndex, sampler := range anim.Samplers {
			samplerFrames[samplerIndex] = inputKeyframesFromSampler(doc, sampler)
			lastFrameTime := samplerFrames[samplerIndex][len(samplerFrames[samplerIndex])-1]
			duration = max(duration, lastFrameTime)
		}

		animationName := anim.Name
		if animationMap != nil && len(animationMap) > 0 {
			if newName, exists := animationMap[animationName]; exists {
				animationName = newName
			}
		}
		result.SamplerFrames[animationName] = samplerFrames
		for _, channel := range anim.Channels {
			var translationFrames [][3]float32
			var rotationFrames [][4]float32
			var scaleFrames [][3]float32
			nodeIndex := *channel.Target.Node
			node := flatNodes[nodeIndex]
			//println(fmt.Sprintf("\nNode for animation: %s", node.GameIdentifier))
			//println(fmt.Sprintf("Property provided by sampler: %s", channel.Targets.Path))
			samplerIndex := channel.Sampler
			sampler := anim.Samplers[samplerIndex]
			outputValues := outputKeyframesFromSampler(doc, sampler)
			if _, exists := node.animations[animationName]; !exists {
				node.animations[animationName] = &SimpleAnimationData{
					Duration: duration,
				}
			}

			switch channel.Target.Path { // THIS IS NEEDED FOR ANIMATION
			case gltf.TRSTranslation:
				translationFrames, _ = outputValues.([][3]float32)
				//println(fmt.Sprintf("Found # of Translation Keyframes: %d", len(translationFrames)))
				node.animations[animationName].TranslationFrames = translationFrames
				node.animations[animationName].TranslationSamplerIndex = samplerIndex
			case gltf.TRSRotation:
				rotationFrames, _ = outputValues.([][4]float32)
				//println(fmt.Sprintf("Found # of Rotation Keyframes: %d", len(rotationFrames)))
				node.animations[animationName].RotationFrames = rotationFrames
				node.animations[animationName].RotationSamplerIndex = samplerIndex
			case gltf.TRSScale:
				scaleFrames, _ = outputValues.([][3]float32)
				//println(fmt.Sprintf("Found # of Scale Keyframes: %d", len(scaleFrames)))
				node.animations[animationName].ScaleFrames = scaleFrames
				node.animations[animationName].ScaleSamplerIndex = samplerIndex
			}
		}
	}
	result.RootNode = buildSceneRoot(doc, flatNodes, defaultScene, result.getSamplerFrames)
	return result
}

// buildSceneRoot builds every node the scene lists at the top level, not just
// the first. A single exported mesh only ever has one - Blockbench's own
// generator-style output (genstairs, the old genshelves) merges everything
// into one primitive - but Blockbench's own exporter puts every ungrouped
// outliner element in as its own sibling scene node, and a model authored as
// several loose cubes silently rendered as just the first of them until this
// wrapped the rest in a synthetic identity root: nothing else in this file
// walked past defaultScene.Nodes[0], so the missing pieces produced no error
// at all, just an incomplete model with a collider bigger than what showed up.
func buildSceneRoot(doc *gltf.Document, flatNodes []*MeshNode, defaultScene *gltf.Scene, samplerFrameSource func(animationName string) [][]float32) *MeshNode {
	if len(defaultScene.Nodes) == 1 {
		return buildNodeHierarchy(doc, flatNodes, defaultScene.Nodes[0], samplerFrameSource)
	}
	println(fmt.Sprintf("[loadGLTF] scene has %d top-level nodes, not 1 - wrapping them under a synthetic root (group everything under one node on export to avoid this)", len(defaultScene.Nodes)))
	root := &MeshNode{
		name:         "scene_root",
		animations:   make(map[string]*SimpleAnimationData),
		quatRotation: mgl32.QuatIdent(),
		translation:  mgl32.Vec3{0, 0, 0},
		scale:        mgl32.Vec3{1, 1, 1},
	}
	root.SetSamplerSource(samplerFrameSource)
	for _, nodeIndex := range defaultScene.Nodes {
		child := buildNodeHierarchy(doc, flatNodes, nodeIndex, samplerFrameSource)
		child.parent = root
		root.children = append(root.children, child)
	}
	return root
}

func tryLoadTextures(doc *gltf.Document) []*glhf.Texture {
	glError := gl.GetError()
	if glError != gl.NO_ERROR {
		println("failure before textures", glError)
	}
	results := make([]*glhf.Texture, len(doc.Textures))
	for texIndex, texture := range doc.Textures {
		//print(fmt.Sprintf("[LoadGLTFWithTextures] Texture at index %d ('%s'): ", texIndex, texture.name))
		imageSource := doc.Images[*texture.Source]
		if imageSource.IsEmbeddedResource() {
			embeddedTexture, err := loadEmbeddedTexture(imageSource)
			if err == nil {
				results[texIndex] = embeddedTexture
			}
		} else if imageSource.BufferView != nil {
			results[texIndex] = loadBufferTexture(doc, imageSource)
		} else {
			// Neither inline data nor a bufferView: an external URI. Nothing
			// in the asset tree exports one - Blockbench and the generators
			// all embed - and resolving it means plumbing the model's own
			// directory down here, so it is reported rather than guessed at.
			//
			// ponytail: leaves the slot without a texture. Resolve
			// imageSource.URI against the glTF's directory if a model ever
			// ships its pictures beside it.
			log.Printf("[gltf] image %q is an external reference, which is not loaded", imageSource.Name)
		}
	}
	glError = gl.GetError()
	if glError != gl.NO_ERROR {
		println("failure after textures", glError)
	}
	return results
}

func loadBufferTexture(doc *gltf.Document, imageSource *gltf.Image) *glhf.Texture {
	bufferView := doc.BufferViews[*imageSource.BufferView]
	buffer := doc.Buffers[bufferView.Buffer]
	data := buffer.Data[bufferView.ByteOffset : bufferView.ByteOffset+bufferView.ByteLength]
	loadedTexture, err := glhf.NewTextureFromReader(bytes2.NewReader(data))
	if err != nil {
		println(fmt.Sprintf("Error loading texture from buffer: %s", err.Error()))
	} else {
		//println(fmt.Sprintf("'%s' loaded from buffer", imageSource.name))
	}
	return loadedTexture
}

func loadEmbeddedTexture(image *gltf.Image) (*glhf.Texture, error) {
	data, err := image.MarshalData()
	if err != nil {
		println(fmt.Sprintf("Error loading embedded texture: %s", err.Error()))
		return nil, err
	}
	// save bytes as png
	loadedTexture, err := glhf.NewTextureFromReader(bytes2.NewReader(data))
	if err != nil {
		println(fmt.Sprintf("Error loading embedded texture: %s", err.Error()))
		return nil, err
	}
	if image.Name == "" {
		println("Loaded from embedded resource")
	} else {
		//println(fmt.Sprintf("'%s' loaded from embedded resource", image.name))
	}
	return loadedTexture, nil
}

// loadMesh loads all submeshes with their texture indices
func loadMesh(doc *gltf.Document, meshIndex uint32, forcedVertexColor *mgl32.Vec3) *SimpleMesh {
	result := make([]*SubMesh, 0)
	mesh := doc.Meshes[meshIndex]
	for _, subMesh := range mesh.Primitives {
		currentSubmesh := &SubMesh{}
		material := doc.Materials[*subMesh.Material]
		if material.PBRMetallicRoughness.BaseColorTexture != nil {
			textureIndex := material.PBRMetallicRoughness.BaseColorTexture.Index
			currentSubmesh.TextureIndex = textureIndex
		}

		if subMesh.Mode != gltf.PrimitiveTriangles {
			println("WARNING: Only triangles are supported for now")
		}
		indexOfPositions := subMesh.Attributes["POSITION"]
		indexOfColors, hasColors := subMesh.Attributes["COLOR_0"]
		indexOfNormals := subMesh.Attributes["NORMAL"]
		indexOfUVs := subMesh.Attributes["TEXCOORD_0"]

		positionAccessor := doc.Accessors[indexOfPositions]
		normalsAccessor := doc.Accessors[indexOfNormals]
		indicesAccessor := doc.Accessors[*subMesh.Indices]
		uvsAccessor := doc.Accessors[indexOfUVs]

		var posBuffer [][3]float32
		var colorBuffer [][4]uint8
		var indicesBuffer []uint32
		var normalsBuffer [][3]float32
		var uvBuffer [][2]float32
		var err error
		posBuffer, err = modeler.ReadPosition(doc, positionAccessor, posBuffer)
		if err != nil {
			println(err)
			return nil
		}

		if hasColors {
			colorAccessor := doc.Accessors[indexOfColors]
			colorBuffer, err = modeler.ReadColor(doc, colorAccessor, colorBuffer)
			if err != nil {
				println(err)
				return nil
			}
		}

		indicesBuffer, err = modeler.ReadIndices(doc, indicesAccessor, indicesBuffer)
		if err != nil {
			println(err)
			return nil
		}
		normalsBuffer, err = modeler.ReadNormal(doc, normalsAccessor, normalsBuffer)
		if err != nil {
			println(err)
			return nil
		}
		uvBuffer, err = modeler.ReadTextureCoord(doc, uvsAccessor, uvBuffer)
		if err != nil {
			println(err)
			return nil
		}
		// merge them..
		var meshVertices []glhf.GlFloat
		for i := 0; i < len(posBuffer); i++ {
			/*
				{Name: "position", Type: glhf.Vec3},
				{Name: "texCoord", Type: glhf.Vec2},
				{Name: "vertexColor", Type: glhf.Vec3},
				{Name: "normal", Type: glhf.Vec3},
			*/
			colorR := glhf.GlFloat(1.0)
			colorG := glhf.GlFloat(1.0)
			colorB := glhf.GlFloat(1.0)
			if hasColors {
				colorR = glhf.GlFloat(colorBuffer[i][0]) / 255.0
				colorG = glhf.GlFloat(colorBuffer[i][1]) / 255.0
				colorB = glhf.GlFloat(colorBuffer[i][2]) / 255.0
			} else if forcedVertexColor != nil {
				colorR = glhf.GlFloat(forcedVertexColor.X())
				colorG = glhf.GlFloat(forcedVertexColor.Y())
				colorB = glhf.GlFloat(forcedVertexColor.Z())
			}

			meshVertices = append(
				meshVertices,

				glhf.GlFloat(posBuffer[i][0]),
				glhf.GlFloat(posBuffer[i][1]),
				glhf.GlFloat(posBuffer[i][2]),

				glhf.GlFloat(uvBuffer[i][0]),
				glhf.GlFloat(uvBuffer[i][1]),

				colorR,
				colorG,
				colorB,

				glhf.GlFloat(normalsBuffer[i][0]),
				glhf.GlFloat(normalsBuffer[i][1]),
				glhf.GlFloat(normalsBuffer[i][2]),
			)
		}
		currentSubmesh.Indices = indicesBuffer
		currentSubmesh.VertexData = meshVertices
		currentSubmesh.VertexCount = int(indicesAccessor.Count)
		result = append(result, currentSubmesh)
	}
	currentMesh := &SimpleMesh{
		// Vertex Format (translation, Normal, UV)
		// X0,Y0,Z0, NX0,NY0,NZ0, U0,V0
		SubMeshes: result,
	}
	return currentMesh
}

func buildNodeHierarchy(document *gltf.Document, meshNodes []*MeshNode, nodeIndex int, samplerFrameSource func(animationName string) [][]float32) *MeshNode {
	docNode := document.Nodes[nodeIndex]
	meshNode := meshNodes[nodeIndex]
	meshNode.SetSamplerSource(samplerFrameSource)

	meshNode.setInitialTranslate(vec3PrecisionCast(docNode.TranslationOrDefault()))
	meshNode.setInitialRotation(vec4PrecisionCast(docNode.RotationOrDefault()))
	meshNode.setInitialScale(vec3PrecisionCast(docNode.ScaleOrDefault()))

	for _, childNodeIndex := range docNode.Children {
		meshNode.children = append(meshNode.children, buildNodeHierarchy(document, meshNodes, childNodeIndex, samplerFrameSource))
		childMeshNode := meshNodes[childNodeIndex]
		childMeshNode.parent = meshNode
	}
	return meshNode
}

func vec3PrecisionCast(orDefault [3]float64) [3]float32 {
	return [3]float32{float32(orDefault[0]), float32(orDefault[1]), float32(orDefault[2])}
}

func vec4PrecisionCast(orDefault [4]float64) [4]float32 {
	return [4]float32{float32(orDefault[0]), float32(orDefault[1]), float32(orDefault[2]), float32(orDefault[3])}
}
func inputKeyframesFromSampler(doc *gltf.Document, sampler *gltf.AnimationSampler) []float32 {
	var result []float32
	inputAccessor := doc.Accessors[sampler.Input]
	/*
		inputAcr := &gltf.Accessor{
			BufferView: gltf.Index(*inputAccessor.BufferView), Count: inputAccessor.Count, Type: inputAccessor.Type, ComponentType: inputAccessor.ComponentType,
		}

	*/
	var inputBufferUntyped []byte
	var err error
	keyFrames, err := modeler.ReadAccessor(doc, inputAccessor, inputBufferUntyped)
	if err != nil {
		println(err)
		return nil
	}
	result = keyFrames.([]float32)
	return result
}

func outputKeyframesFromSampler(doc *gltf.Document, sampler *gltf.AnimationSampler) interface{} {
	outputAccessor := doc.Accessors[sampler.Output]
	/*
		outputAcr := &gltf.Accessor{
			BufferView: gltf.Index(*outputAccessor.BufferView), Count: outputAccessor.Count, Type: outputAccessor.Type, ComponentType: outputAccessor.ComponentType,
		}

	*/
	var inputBufferUntyped []byte
	var err error
	result, err := modeler.ReadAccessor(doc, outputAccessor, inputBufferUntyped)
	if err != nil {
		println(err)
		return nil
	}

	return result
}
