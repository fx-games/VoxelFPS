package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    QuadShaderModelTransform int = iota
    QuadShaderOriginOffset
    QuadShaderTextureIndex
    QuadShaderAtlasItemSize
    QuadShaderTintColor
)

func LoadQuadShader(uboBlock string) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "cornerIndex", Type: glhf.UInt}, // 0-3
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
            glhf.Attr{Name: "originOffset", Type: glhf.Vec2},   // 1
            glhf.Attr{Name: "textureIndex", Type: glhf.UInt},   // 2
            glhf.Attr{Name: "atlasItemSize", Type: glhf.IVec2}, // 3
            glhf.Attr{Name: "tintColor", Type: glhf.Vec4},      // 4
        }
        shader *glhf.Shader
    )
    var err error
    shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, vertexShaderHeader(uboBlock)+quadVertexShaderSource, fragmentShaderHeader(uboBlock)+quadFragmentShaderSource)

    if err != nil {
        panic(err)
    }

    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }

    shader.Begin()
    shader.SetUniformAttr(QuadShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    shader.SetUniformAttr(QuadShaderOriginOffset, mgl32.Vec2{0, 0})
    shader.SetUniformAttr(QuadShaderAtlasItemSize, [2]int32{int32(64), int32(64)})
    shader.BindSamplerNameToTextureUnit("shadowMap", gl.TEXTURE2)
    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
    return shader
}
func LoadQuadShaderDepthPass(uboBlock string) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "cornerIndex", Type: glhf.UInt}, // 0-3
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
            glhf.Attr{Name: "originOffset", Type: glhf.Vec2},   // 1
            glhf.Attr{Name: "textureIndex", Type: glhf.UInt},   // 2
            glhf.Attr{Name: "atlasItemSize", Type: glhf.IVec2}, // 3
        }
        shader *glhf.Shader
    )
    var err error
    shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, depthVertexShaderHeader(uboBlock)+quadVertexShaderSource, quadFragmentShaderSourceDepth)

    if err != nil {
        panic(err)
    }

    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }

    shader.Begin()
    shader.SetUniformAttr(QuadShaderOriginOffset, mgl32.Vec2{0, 0})
    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
    return shader
}

const quadVertexShaderSource = `
out vec2 VertUV;// tl: 0,0 tr: 1,0 bl: 0,1 br: 1,1
#ifndef DEPTH_PASS
out vec3 VertPos;
out vec3 VertNormal;
#endif

layout(location = 0) in uint cornerIndex;

uniform mat4 modelTransform;
uniform uint textureIndex;
uniform ivec2 atlasItemSize;
uniform vec2 originOffset;

uniform sampler2D tex;

float getLeftU(uint texIndex, uint xCount, uint tileWidthInPx, uint textureSizeX) {
    uint leftBorder = ((texIndex % xCount) * (tileWidthInPx));
    return float(leftBorder) / float(textureSizeX);
}

float getRightU(uint texIndex, uint xCount, uint tileWidthInPx, uint textureSizeX) {
    uint leftBorder = ((texIndex % xCount) * (tileWidthInPx));
    return float(leftBorder + tileWidthInPx) / float(textureSizeX);
}

float getTopV(uint texIndex, uint xCount, uint tileHeightInPx, uint textureSizeY) {
    uint topBorder = ((texIndex / xCount) * (tileHeightInPx));
    return float(topBorder) / float(textureSizeY);
}

float getBottomV(uint texIndex, uint xCount, uint tileHeightInPx, uint textureSizeY) {
    uint topBorder = ((texIndex / xCount) * (tileHeightInPx));
    return (float(topBorder + tileHeightInPx) / float(textureSizeY)) - 0.0001;
}

/*
relative vertex positions
0        vec3(0.5, 0.5, 0.0),  // top right
1        vec3(0.5, -0.5, 0.0),  // bottom right
2        vec3(-0.5, -0.5, 0.0), // bottom left
3        vec3(-0.5, 0.5, 0.0) // top left

uv mapping
0 -> rightU, topV
1 -> rightU, bottomV
2 -> leftU, bottomV
3 -> leftU, topV
*/

vec2 getUV(uint cornerIndex, uint textureIndex) {
    ivec2 textureSize2d = textureSize(tex,0);
    uint textureSizeX = uint(textureSize2d.x);
    uint textureSizeY = uint(textureSize2d.y);
    uint tileWidthInPx = uint(atlasItemSize.x);
    uint tileHeightInPx = uint(atlasItemSize.y);
    uint xCount = (textureSizeX / tileWidthInPx);

    float uValue = cornerIndex == uint(0) || cornerIndex == uint(1) ? getRightU(textureIndex, xCount, tileWidthInPx, textureSizeX) : getLeftU(textureIndex, xCount, tileWidthInPx, textureSizeX);
    float vValue = cornerIndex == uint(1) || cornerIndex == uint(2) ? getBottomV(textureIndex, xCount, tileHeightInPx, textureSizeY) : getTopV(textureIndex, xCount, tileHeightInPx, textureSizeY);

    return vec2(uValue, vValue);
}

void main() {
    // tr, br, bl, tl
    const vec3 vertPosByCorner[4] = vec3[4](
        vec3(0.5, 0.5, 0.0),  // top right
        vec3(0.5, -0.5, 0.0),  // bottom right
        vec3(-0.5, -0.5, 0.0), // bottom left
        vec3(-0.5, 0.5, 0.0) // top left
    );

    vec3 relativePos = vertPosByCorner[cornerIndex] + vec3(originOffset, 0.0);

    gl_Position = camProjectionView * modelTransform * vec4(relativePos, 1.0);

    VertUV = getUV(cornerIndex, textureIndex);
#ifndef DEPTH_PASS
    VertNormal = vec3(0.0, 0.0, 1.0); // THIS PROBABLY NEEDS TO BE TRANSFORMED?
    VertPos = relativePos;
#endif
}`

const quadFragmentShaderSource = `
in vec3 VertPos;
in vec2 VertUV;
in vec3 VertNormal;

uniform mat4 modelTransform;
uniform vec4 tintColor;

uniform sampler2D tex;

out vec4 fragmentColor;

void main() {
    vec4 surfaceColor = texture(tex, VertUV);

    if (surfaceColor.a == 0) {
        discard;
    } else {
        // apply tint
        float VertLightLevel = 12;
        vec3 floodLight = vec3(2.0, 2.0, 2.0) * (VertLightLevel/15.0);
    
        surfaceColor *= tintColor;
    
        vec3 fragWorldPos = (modelTransform * vec4(VertPos, 1.0)).xyz;
        vec3 fragWorldNormal = (modelTransform * vec4(VertNormal, 0.0)).xyz;
    
        //vec3 transformedFragPos = (camProjectionView * modelTransform * vec4(VertPos, 1.0)).xyz;
    
        fragmentColor = finalColor(surfaceColor, fragWorldPos, fragWorldNormal, vec3(0.0));
    }
}`

const quadFragmentShaderSourceDepth = `
#version 400 core

in vec2 VertUV;

uniform sampler2D tex;

void main() {
    vec4 surfaceColor = texture(tex, VertUV); 
    
    float[2] depthValues = float[2](
        1.0,
        gl_FragCoord.z
    );
    
    int isOpaque = int(step(0.5, surfaceColor.a));
    gl_FragDepth = depthValues[isOpaque];
}`
