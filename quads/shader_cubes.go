package quads

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	CubeShaderModelTransform int = iota
	CubeShaderAtlasGridSize
	CubeShaderMapSize
	CubeShaderEmissiveStrength
	CubeShaderEmissiveCutoff
	CubeShaderEmissiveSaturation
)

// Marking a texture emissive does not light the whole tile: it lights the texels that
// look like a lit fitting, so a console panel's screens come on while the metal around
// them stays dark. That is what makes it safe to push the strength well past white -
// only a small part of the tile gets it, and it needs real headroom over the bloom
// threshold before any light bleeds out.
//
// Brightness alone cannot make that distinction on this art. Polished grey metal is as
// bright as a cyan tube, so a luminance cutoff lit whole walls - measured on the wall
// sheet, a luminance-only mask took 63% of tex 19 rather than its light strips. Lit
// fittings here are saturated (cyan tubes, blue screens, green signs) and the metal is
// not, so the mask gates on both: bright AND colourful. Grey panels then score zero
// however bright they are.
//
// Set the saturation cutoff to 0 to go back to a pure brightness mask, and the
// brightness cutoff to 0 as well to light the whole face.
const (
	// 4.0 sat only just over the old bloom threshold. 7.0 puts a masked texel well
	// past it, so an emissive fitting is a light source on screen and not a bright decal.
	DefaultEmissiveStrength   = float32(7.0)
	DefaultEmissiveCutoff     = float32(0.35)
	DefaultEmissiveSaturation = float32(0.35)
)

func LoadCubeShader(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "compressedValue", Type: glhf.UInt},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
			glhf.Attr{Name: "atlasGridSize", Type: glhf.IVec2}, // 1
			// voxel grid dimensions of the lookup buffer. ivec4 because glhf has
			// no ivec3; w is unused.
			glhf.Attr{Name: "mapSize", Type: glhf.IVec4}, // 2
			// How far past white an emissive face is pushed. A uniform rather than a
			// constant because it is the knob you turn against the bloom threshold.
			glhf.Attr{Name: "emissiveStrength", Type: glhf.Float}, // 3
			// Luminance above which a texel of an emissive texture may glow.
			glhf.Attr{Name: "emissiveCutoff", Type: glhf.Float}, // 4
			// ...and how colourful it also has to be, which is what separates a lit
			// fitting from bright bare metal.
			glhf.Attr{Name: "emissiveSaturation", Type: glhf.Float}, // 5
		}
		shader *glhf.Shader
	)

	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, vertexShaderHeader(uboBlock)+chunkVertexShaderSource, fragmentShaderHeader(uboBlock)+chunkFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}
	shader.Begin()
	shader.SetUniformAttr(CubeShaderModelTransform, mgl32.Ident4())
	shader.SetUniformAttr(CubeShaderEmissiveStrength, DefaultEmissiveStrength)
	shader.SetUniformAttr(CubeShaderEmissiveCutoff, DefaultEmissiveCutoff)
	shader.SetUniformAttr(CubeShaderEmissiveSaturation, DefaultEmissiveSaturation)
	shader.BindSamplerNameToTextureUnit("tex", gl.TEXTURE0)
	shader.BindSamplerNameToTextureUnit("voxelLookup", gl.TEXTURE1)
	shader.BindSamplerNameToTextureUnit("shadowMap", gl.TEXTURE2)
	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}

func LoadCubeShaderDepthPass(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "compressedValue", Type: glhf.UInt},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
		}
		shader *glhf.Shader
	)

	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, depthVertexShaderHeader(uboBlock)+chunkVertexShaderSource, emptyFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}
	shader.Begin()
	shader.SetUniformAttr(CubeShaderModelTransform, mgl32.Ident4())
	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}

const chunkVertexShaderSource = `
layout(column_major) uniform;

// Plan for the bit-packing
// 32 bits per vertex
// Bits 0..4: X position (0..31)
// xxxx xxxx xxxx xxxx xxxx xxxx xxxx xxxx
//                                  ^ ^^^^
//                                  X Position

// Bits 5..9: Y position (0..31)
// xxxx xxxx xxxx xxxx xxxx xxxx xxxx xxxx
//                            ^^ ^^^
//                            Y Position

// Bits 10..14: Z position (0..31)
// xxxx xxxx xxxx xxxx xxxx xxxx xxxx xxxx
//                      ^^^ ^^
//                      Z Position

// Bits 15..17: Normal (0..5)
// xxxx xxxx xxxx xxxx xxxx xxxx xxxx xxxx
//                  ^^ ^
//                  Normal Direction (1,0,0) (-1,0,0) (0,1,0) (0,-1,0) (0,0,1) (0,0,-1)

// Bits 18..31: Texture Index (0..4095)
// xxxx xxxx xxxx xxxx xxxx xxxx xxxx xxxx
// ^^^^ ^^^^ ^^^^ ^^
// Texture Index

//in vec3 position; // should be between 0..31
// 32 states needs 5 bits, so 5 bits per axis => 15 bits
//in vec3 normal;   // one of 6 directions, 0..5
// 6 states needs 3 bits, so 3 bits per normal
// what texture to apply? 1 byte?
// uv coords for texture? derive from texture index and position of vertex in quad (0..3)
// 4 states needs 2 bits, so 2 bits per uv coord

// that's 20 bits per vertex without the texture index
// if we assume a 32 bit int, that leaves 12 bits for the texture index

/* Compression 
xAxis  // 6 bits (0..63) 
yAxis  // 5 bits (0..31)
zAxis  // 6 bits (0..63)
Normal // 3 bits (0..5)
Total  // 20 bits used of 32
12-14 bits left for animation/lighting information
*/

#ifndef DEPTH_PASS
flat out uint VertNormalIndex;
out vec3 VertPos;
#endif

layout(location = 0) in uint compressedValue;

uniform mat4 modelTransform;

void decompressVertex(uint compressedValue, out vec3 position, out uint normalDir)
{
    uint positionX = compressedValue & uint(0x3F);// 0x3F = 0b111111
    uint yPart = compressedValue >> uint(6);
    uint positionY = yPart & uint(0x1F); // 6 bits => 0x3F = 0b111111, 5 bits => 0x1F = 0b11111, 4 bits => 0xF = 0b1111
    uint zPart = compressedValue >> uint(11);
    uint positionZ = zPart & uint(0x3F);
    position = vec3(float(positionX), float(positionY), float(positionZ));

    normalDir = (compressedValue >> uint(17)) & uint(0x7);// 0x7 = 0b111
    // Bits 20 and up are unused. The texture index, the emissive flag and the baked
    // light are per face, not per vertex, and live in the voxel lookup buffer.
}


void main() {
    // decompress the vertex
    uint normalDir = uint(1);
    vec3 position = vec3(0.0);

    decompressVertex(compressedValue, position, normalDir);
    gl_Position = camProjectionView * modelTransform * vec4(position, 1.0);

    // pass-through for fragment shader
#ifndef DEPTH_PASS
    VertNormalIndex = normalDir;
    VertPos = position;
#endif
}`

const chunkFragmentShaderSource = `
layout(column_major) uniform;


vec3[6] normalLookup = vec3[6](
    vec3(1.0, 0.0, 0.0),    // xp
    vec3(-1.0, 0.0, 0.0),   // xn
    vec3(0.0, 1.0, 0.0),    // yp
    vec3(0.0, -1.0, 0.0),   // yn
    vec3(0.0, 0.0, 1.0),    // zp
    vec3(0.0, 0.0, -1.0)    // zn
);


flat in uint VertNormalIndex;
in vec3 VertPos;

uniform ivec2 atlasGridSize;
uniform mat4 modelTransform;
uniform ivec4 mapSize;

uniform sampler2D tex;
uniform usamplerBuffer voxelLookup;
uniform float emissiveStrength;
uniform float emissiveCutoff;
uniform float emissiveSaturation;

// One lookup entry describes one face: the top bit marks it emissive, the next four
// carry its baked torch light, the rest is the texture index. See Map.packFaceLookup.
// All three ride the one texel this shader already fetches - no second buffer, and
// nothing per face in the vertices, which is what lets greedy meshing merge freely.
#define EMISSIVE_TEXTURE_BIT uint(0x8000)
#define TORCH_LIGHT_MASK uint(0x7800)
#define TORCH_LIGHT_SHIFT uint(11)
#define EMISSIVE_TEXTURE_MASK uint(0x07FF)

layout (location = 0) out vec4 fragmentColor;

void main() {
    vec3 VertNormal = normalLookup[VertNormalIndex];
    vec3 fragWorldPos = (modelTransform * vec4(VertPos, 1.0)).xyz;

    ivec2 textureSize2d = textureSize(tex, 0);

    uint tileWidth  = uint(textureSize2d.x) / uint(atlasGridSize.x);
    uint tileHeight = uint(textureSize2d.y) / uint(atlasGridSize.y);

    uint mapSizeX = uint(mapSize.x);
    uint mapSizeY = uint(mapSize.y);

    // A face with a positive normal belongs to the cell one step back from the
    // fragment. Done in signed arithmetic and clamped: at grid coordinate 0 the
    // subtraction would otherwise wrap to ~4 billion and texelFetch would read
    // whatever sits at that offset.
    ivec3 normOffset = ivec3((VertNormal + 1.0) * 0.5);

    ivec3 grid = max(ivec3(floor(fragWorldPos)) - normOffset, ivec3(0));

    uint gridX = uint(grid.x);
    uint gridY = uint(grid.y);
    uint gridZ = uint(grid.z);

    uint wallOffset = (gridX + gridY * mapSizeX + gridZ * mapSizeX * mapSizeY) * uint(6);
    uint lookupValue = texelFetch(voxelLookup, int(wallOffset + VertNormalIndex)).r;
    uint textureIndex = lookupValue & EMISSIVE_TEXTURE_MASK;
    float isEmissive = float((lookupValue & EMISSIVE_TEXTURE_BIT) != uint(0));
    float torchLight = float((lookupValue & TORCH_LIGHT_MASK) >> TORCH_LIGHT_SHIFT);

    // Compute tile position in texel space
    uint tileX = uint(textureIndex % uint(atlasGridSize.x)) * tileWidth;
    uint tileY = uint(textureIndex / uint(atlasGridSize.x)) * tileHeight;

    // Compute texel coordinate within the tile
    uint localU = uint(0);
    uint localV = uint(0);

    if (VertNormal.z > 0.0) {
        localU = uint(fract(VertPos.x) * tileWidth);
        localV = uint((1.0 - fract(VertPos.y)) * tileHeight);
    } else if (VertNormal.z < 0.0) {
        localU = uint((1.0 - fract(VertPos.x)) * tileWidth);
        localV = uint((1.0 - fract(VertPos.y)) * tileHeight);
    } else if (VertNormal.x > 0.0) {
        localU = uint((1.0 - fract(VertPos.z)) * tileWidth);
        localV = uint((1.0 - fract(VertPos.y)) * tileHeight);
    } else if (VertNormal.x < 0.0) {
        localU = uint(fract(VertPos.z) * tileWidth);
        localV = uint((1.0 - fract(VertPos.y)) * tileHeight);
    } else if (VertNormal.y > 0.0) {
        localU = uint(fract(VertPos.x) * tileWidth);
        localV = uint(fract(VertPos.z) * tileHeight);
    } else if (VertNormal.y < 0.0) {
        localU = uint((1.0 - fract(VertPos.x)) * tileWidth);
        localV = uint(fract(VertPos.z) * tileHeight);
    }
    ivec2 texelCoord = ivec2(tileX + localU, tileY + localV);

    vec4 surfaceColor = texelFetch(tex, texelCoord, 0);
    
    vec3 fragWorldNormal = (modelTransform * vec4(VertNormal, 0.0)).xyz;

    // A texel of an emissive texture glows if it is both bright and colourful: that is
    // what tells a cyan tube or a blue screen readout apart from the bare metal panel it
    // is mounted on, which is just as bright but grey. Soft edges on both terms so the
    // boundary does not crawl as the texture minifies.
    float luminance = dot(surfaceColor.rgb, vec3(0.2126, 0.7152, 0.0722));
    float maxChannel = max(max(surfaceColor.r, surfaceColor.g), surfaceColor.b);
    float minChannel = min(min(surfaceColor.r, surfaceColor.g), surfaceColor.b);
    float saturation = maxChannel > 0.0001 ? (maxChannel - minChannel) / maxChannel : 0.0;

    float emissiveMask = smoothstep(emissiveCutoff, emissiveCutoff + 0.1, luminance)
                       * smoothstep(emissiveSaturation, emissiveSaturation + 0.1, saturation);
    vec3 emissive = surfaceColor.rgb * isEmissive * emissiveMask * emissiveStrength;

    fragmentColor = finalColor(surfaceColor, fragWorldPos, fragWorldNormal, emissive, bakedLight(torchLight));
}`
