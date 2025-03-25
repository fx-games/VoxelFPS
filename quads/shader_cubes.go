package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    CubeShaderModelTransform int = iota
    CubeShaderAtlasGridSize
    CubeShaderDebugSwitch
)

func LoadCubeShader(uboBlock string) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "compressedValue", Type: glhf.UInt},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
            glhf.Attr{Name: "atlasGridSize", Type: glhf.IVec2}, // 1
            //glhf.Attr{Name: "debugSwitch", Type: glhf.Float},   // 2
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
    //shader.SetUniformAttr(CubeShaderDebugSwitch, float32(0.0))
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

in uint compressedValue;

flat out uint VertNormalIndex;
out float VertLightLevel;
out vec3 VertPos;

// set uniform locations
uniform mat4 modelTransform;

void decompressVertex(uint compressedValue, out vec3 position, out uint normalDir, out float lightLevel)
{
    uint positionX = compressedValue & uint(0x3F);// 0x3F = 0b111111
    uint yPart = compressedValue >> uint(6);
    uint positionY = yPart & uint(0x1F); // 6 bits => 0x3F = 0b111111, 5 bits => 0x1F = 0b11111, 4 bits => 0xF = 0b1111
    uint zPart = compressedValue >> uint(11);
    uint positionZ = zPart & uint(0x3F);
    position = vec3(float(positionX), float(positionY), float(positionZ));

    normalDir = (compressedValue >> uint(17)) & uint(0x7);// 0x7 = 0b111
    //textureIndex = (compressedValue >> uint(20)) & uint(0xFF);// 0xFF = 0b11111111

    uint light = (compressedValue >> uint(28)) & uint(0xF);
    lightLevel = float(light);
}


void main() {
    // decompress the vertex
    uint normalDir = uint(1);

    decompressVertex(compressedValue, VertPos, normalDir, VertLightLevel);

    // pass-through for fragment shader
    VertNormalIndex = normalDir;

    gl_Position = camProjectionView * modelTransform * vec4(VertPos, 1.0);
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
in float VertLightLevel;

uniform ivec2 atlasGridSize;
uniform mat4 modelTransform;
uniform float debugSwitch;

uniform sampler2D tex;

uniform usamplerBuffer voxelLookup;

layout (location = 0) out vec4 fragmentColor;

void main() {
    vec3 VertNormal = normalLookup[VertNormalIndex];

    ivec2 textureSize2d = textureSize(tex, 0);

    uint tileWidth  = uint(textureSize2d.x) / uint(atlasGridSize.x);
    uint tileHeight = uint(textureSize2d.y) / uint(atlasGridSize.y);

    uint mapSizeX = uint(64);
    uint mapSizeY = uint(32);
    uint mapSizeZ = uint(64);

    uint normOffsetX = uint((VertNormal.x + 1.0) * 0.5);
    uint normOffsetY = uint((VertNormal.y + 1.0) * 0.5);
    uint normOffsetZ = uint((VertNormal.z + 1.0) * 0.5);

    uint gridX = uint(floor(fragWorldPos.x)) - normOffsetX;
    uint gridY = uint(floor(fragWorldPos.y)) - normOffsetY;
    uint gridZ = uint(floor(fragWorldPos.z)) - normOffsetZ;

    uint wallOffset = (gridX + gridY * mapSizeX + gridZ * mapSizeX * mapSizeY) * uint(6);
    uint textureIndex = texelFetch(voxelLookup, int(wallOffset + VertNormalIndex)).r;

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
    
    vec3 floodLight = vec3(2.0, 2.0, 2.0) * (VertLightLevel / 15.0) * camDirection.xyz;

    vec3 fragWorldPos = (modelTransform * vec4(VertPos, 1.0)).xyz;
    vec3 fragWorldNormal = (modelTransform * vec4(VertNormal, 0.0)).xyz;

    fragmentColor = finalColor(surfaceColor, fragWorldPos, fragWorldNormal);
}`
