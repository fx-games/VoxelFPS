package quads

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	GuiShaderProjection = 0 + iota
	GuiShaderPositionOffset
	GuiShaderScaleFactor
	GuiShaderTintColor
)

const (
	GuiShaderEffectTextureMapping = uint32(0) + iota
	GuiShaderEffectColoredBackground
	GuiShaderEffectColoredBorder
)

func LoadGuiShader(windowSizeX, windowSizeY int, uiScale mgl32.Vec2) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
			{Name: "texCoord", Type: glhf.Vec2},
			{Name: "vertColor", Type: glhf.Vec4},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "projection", Type: glhf.Mat4},       // 0
			glhf.Attr{Name: "positionOffset", Type: glhf.Vec2},   // 1
			glhf.Attr{Name: "scaleFactor", Type: glhf.Vec2},      // 2
			glhf.Attr{Name: "appliedTintColor", Type: glhf.Vec4}, // 3
		}
		shader *glhf.Shader
	)
	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, guiVertexShaderSource, guiFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}

	shader.Begin()
	// scale using the projection matrix
	projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(windowSizeX, windowSizeY)
	shader.SetUniformAttr(GuiShaderProjection, projectionMatrix)
	shader.SetUniformAttr(GuiShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
	shader.SetUniformAttr(GuiShaderScaleFactor, uiScale)

	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
	return shader
}

/*
shader.SetTextureUnitArray("tex", 2)
->
uniform sampler2D tex[2]; // bound texture units 0, 1
// but cannot be indexed by values passed via the vertex array..
// -> changing textures would require a new draw call so we can set a uniform
*/

const guiVertexShaderSource = `
#version 400 core

in vec3 position;
in vec2 texCoord;
in vec4 vertColor;

uniform uint specialEffect;
uniform mat4 projection;

uniform vec2 scaleFactor;
uniform vec2 positionOffset;

out vec2 VertUV;
out vec4 VertColor;

void main() {
    // gl_InstanceID, will change for every instance (quad)
    // gl_VertexID, will change for every vertex index (6 indices per quad)

    vec2 pos = (position.xy * scaleFactor) + positionOffset;
    vec4 glPos = projection * vec4(pos, 0.0, 1.0);
    glPos.z = position.z;
    gl_Position = glPos;

    // pass-through for fragment shader
    VertUV = texCoord;
    VertColor = vertColor;
}
`

const guiFragmentShaderSource = `
#version 400 core

in vec2 VertUV;
in vec4 VertColor;

uniform sampler2D tex;
uniform vec4 appliedTintColor;

//layout(origin_upper_left) in vec4 gl_FragCoord;

out vec4 fragmentColor;

void main() {
  	vec4 texColor = texelFetch(tex, ivec2(VertUV), 0);
    if (texColor.a == 0.0) {
        discard;
    }
    fragmentColor = texColor * VertColor * appliedTintColor;
}
`
