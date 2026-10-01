package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/platform/gl"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	QuadShaderTintColor int = iota
)

// The per-sprite values used to be uniforms reset before every draw. They are
// instance attributes now, so the locations have to be pinned in the GLSL: the
// same vertex array is drawn by the lit shader and the depth shader, and a VAO
// stores attribute pointers by location, not by name.
const (
	QuadInstanceModelLocation    = 1 // through 4, a mat4 spans four slots
	QuadInstanceOriginLocation   = 5
	QuadInstanceUVLocation       = 6
	QuadInstanceEmissiveLocation = 7
	QuadInstanceBakedLocation    = 8
)

// QuadInstanceFormat is the per-sprite attribute layout: mat4, vec2, vec4, float, float.
var QuadInstanceFormat = []glhf.InstanceAttr{
	{Location: QuadInstanceModelLocation, Floats: 16},
	{Location: QuadInstanceOriginLocation, Floats: 2},
	{Location: QuadInstanceUVLocation, Floats: 4},
	{Location: QuadInstanceEmissiveLocation, Floats: 1},
	{Location: QuadInstanceBakedLocation, Floats: 1},
}

// QuadInstanceFloats is how many float32 one sprite occupies in that buffer.
const QuadInstanceFloats = 16 + 2 + 4 + 1 + 1

func LoadQuadShader(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "cornerIndex", Type: glhf.UInt}, // 0-3
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "tintColor", Type: glhf.Vec4}, // 0
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
		uniformFormat = glhf.AttrFormat{}
		shader        *glhf.Shader
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
out float VertEmissive;
out float VertBaked;
#endif

layout(location = 0) in uint cornerIndex;

// Per-instance: one sprite per instance, advanced by glVertexAttribDivisor.
layout(location = 1) in mat4 instModelTransform;
layout(location = 5) in vec2 instOriginOffset;
layout(location = 6) in vec4 instUV;// leftU, topV, rightU, bottomV, worked out on the CPU
layout(location = 7) in float instEmissive;// 0 for a normal sprite, otherwise how far past white it glows
layout(location = 8) in float instBaked;// the flood-filled voxel light level, 0..15, where this sprite stands

uniform sampler2D tex;

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

// The rect arrives ready made, so a frame of any size in an atlas of any layout
// looks the same from here. It used to be worked out from a texture index and a
// uniform tile size, which only a fixed grid of equally sized frames can have,
// and a weapon's frames are packed and all different.
vec2 getUV(uint cornerIndex, vec4 uv) {
    float uValue = cornerIndex == uint(0) || cornerIndex == uint(1) ? uv.z : uv.x;
    float vValue = cornerIndex == uint(1) || cornerIndex == uint(2) ? uv.w - 0.0001 : uv.y;
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

    vec3 relativePos = vertPosByCorner[cornerIndex] + vec3(instOriginOffset, 0.0);
    vec4 worldPos = instModelTransform * vec4(relativePos, 1.0);

    gl_Position = camProjectionView * worldPos;

    VertUV = getUV(cornerIndex, instUV);
#ifndef DEPTH_PASS
    // The model transform is per instance now, so the fragment shader cannot
    // reach it. World position and normal are handed over instead.
    VertNormal = normalize((instModelTransform * vec4(0.0, 0.0, 1.0, 0.0)).xyz);
    VertPos = worldPos.xyz;
    VertEmissive = instEmissive;
    VertBaked = instBaked;
#endif
}`

const quadFragmentShaderSource = `
in vec3 VertPos;// world space
in vec2 VertUV;
in vec3 VertNormal;// world space
in float VertEmissive;
in float VertBaked;

uniform vec4 tintColor;

uniform sampler2D tex;

out vec4 fragmentColor;

void main() {
    vec4 surfaceColor = texture(tex, VertUV);

    if (surfaceColor.a == 0.0) {
        discard;
    } else {
        // apply tint
        surfaceColor *= tintColor;

        // An emissive sprite is handed to finalColor as its own light: the emissive
        // term is added after the lighting multiply, so shadows and the light loop
        // cannot darken it, and past 1.0 it is what the bloom threshold picks up.
        //
        // The baked term is the same flood-filled voxel light the walls around
        // this sprite are lit by, sampled once on the CPU where it stands. Without
        // it a sprite in a room lit only by baked torches is black while every
        // surface around it is lit - which is not what Doom does with a sprite,
        // where the sector's own light level lights whatever stands in it.
        fragmentColor = finalColor(surfaceColor, VertPos, VertNormal,
            surfaceColor.rgb * VertEmissive, bakedLight(VertBaked));
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
