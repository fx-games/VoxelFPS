package quads

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	Shader2DProjection = 0 + iota
	Shader2DScreenRect
	Shader2DUVRect
	Shader2DAppliedTintColor
	Shader2DVerticalCutoff
	Shader2DSpecialEffect
)

func Load2DShader(windowSizeX, windowSizeY int) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "cornerIndex", Type: glhf.UInt},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "projection", Type: glhf.Mat4},       // 0
			glhf.Attr{Name: "screenRect", Type: glhf.IVec4},      // 1
			glhf.Attr{Name: "uvRect", Type: glhf.IVec4},          // 2
			glhf.Attr{Name: "appliedTintColor", Type: glhf.Vec4}, // 3
			glhf.Attr{Name: "specialEffect", Type: glhf.UInt},    // 5
			glhf.Attr{Name: "verticalCutoff", Type: glhf.IVec2},  // 4
		}
		shader *glhf.Shader
	)
	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, twoDVertexShaderSource, twoDFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}

	shader.Begin()
	// scale using the projection matrix
	projectionMatrix := Get2DPixelCoordOrthographicProjectionMatrix(windowSizeX, windowSizeY)
	shader.SetUniformAttr(Shader2DProjection, projectionMatrix)
	shader.SetUniformAttr(Shader2DAppliedTintColor, mgl32.Vec4{1, 1, 1, 1})
	shader.SetUniformAttr(Shader2DVerticalCutoff, glhf.IntVec2{0, 81920})
	shader.SetUniformAttr(Shader2DScreenRect, glhf.IntVec4{0, 0, 400, 400})
	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
	return shader
}

const twoDVertexShaderSource = `
#version 400 core

in uint cornerIndex;

uniform mat4 projection;
uniform ivec4 screenRect;  // minX, minY, maxX, maxY
uniform ivec4 uvRect;      // minX, minY, maxX, maxY
uniform uint specialEffect;

ivec2[4] cornerLookup = ivec2[4](
    ivec2(2, 1),   // top right
    ivec2(2, 3),   // bottom right
    ivec2(0, 3),   // bottom left
    ivec2(0, 1)    // top left
);

out vec2 VertUV;
flat out ivec4 ScreenRect;

void main() {
    ivec2 lookup = cornerLookup[cornerIndex];
    ivec2 screenPos = ivec2(screenRect[lookup.x], screenRect[lookup.y]);

    ScreenRect = screenRect;
    
    VertUV = vec2(uvRect[lookup.x], uvRect[lookup.y]);
    gl_Position = projection * vec4(vec2(screenPos), 0.0, 1.0);
}
`

const twoDFragmentShaderSource = `
#version 400 core

in vec2 VertUV;
flat in ivec4 ScreenRect;

uniform sampler2D tex;
uniform vec4 appliedTintColor;
uniform ivec2 verticalCutoff;
uniform uint specialEffect;

layout(origin_upper_left) in vec4 gl_FragCoord;

out vec4 color;

const float eps = 0.001;

void main() {
    vec4 fragScreen = gl_FragCoord;

    if (fragScreen.y > verticalCutoff.y) {
        discard;
        return;
    }

    if (fragScreen.y < verticalCutoff.x) {
        discard;
        return;
    }

    if (specialEffect > 0) {
        // colored border
        if (specialEffect == 2 && (
            abs((ScreenRect.x) - uint(fragScreen.x)) > 1 ||
            abs((ScreenRect.z) - uint(fragScreen.x)) > 1 ||
            abs((ScreenRect.y) - uint(fragScreen.y)) > 1 ||
            abs((ScreenRect.w) - uint(fragScreen.y)) > 1
        )) {
            discard;
            return;
        }
        color = appliedTintColor;
        return;
    }

 	vec4 texColor = texelFetch(tex, ivec2(VertUV), 0);

    texColor *= appliedTintColor;
    color = texColor;
}
`
