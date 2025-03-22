package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    GuiShaderProjection = 0 + iota
    GuiShaderPosition
    GuiShaderUVOffset
    GuiShaderTintColor
    GuiShaderScale
    GuiShaderVerticalCutoff
)

func LoadGuiShader(windowSizeX, windowSizeY int) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "position", Type: glhf.Vec2},
            {Name: "texCoord", Type: glhf.Vec2},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "projection", Type: glhf.Mat4},       // 0
            glhf.Attr{Name: "screenPos", Type: glhf.Vec2},        // 1
            glhf.Attr{Name: "uvOffset", Type: glhf.Vec2},         // 2
            glhf.Attr{Name: "appliedTintColor", Type: glhf.Vec4}, // 3
            glhf.Attr{Name: "scaleFactor", Type: glhf.Vec2},      // 4
            glhf.Attr{Name: "verticalCutoff", Type: glhf.IVec2},  // 5
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
    shader.SetUniformAttr(GuiShaderPosition, mgl32.Vec2{0, 0})
    shader.SetUniformAttr(GuiShaderUVOffset, mgl32.Vec2{0, 0})
    shader.SetUniformAttr(GuiShaderScale, mgl32.Vec2{1, 1})
    shader.SetUniformAttr(GuiShaderVerticalCutoff, glhf.IntVec2{0, 8192})
    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
    return shader
}

const guiVertexShaderSource = `
#version 330 core

in vec2 position;
in vec2 texCoord;

uniform vec2 uvOffset;
uniform vec2 uiScale;
uniform mat4 projection;
uniform vec2 screenPos;
uniform vec2 scaleFactor;

out vec2 VertUV;

void main() {
    vec2 pos = (position * scaleFactor) + screenPos;
    gl_Position = projection * vec4(pos, 0.0, 1.0);

    // pass-through for fragment shader
    VertUV = texCoord + uvOffset;
}
`

const guiFragmentShaderSource = `
#version 330 core

in vec2 VertUV;

uniform sampler2D tex;
uniform vec4 appliedTintColor;
uniform ivec2 verticalCutoff;

layout(origin_upper_left) in vec4 gl_FragCoord;

out vec4 color;

void main() {

    vec4 fragScreen = gl_FragCoord;

    if (fragScreen.y > verticalCutoff.y) {
        discard;
    }

    if (fragScreen.y < verticalCutoff.x) {
        discard;
    }
    
    if (VertUV.x < 0.0 && VertUV.y < 0.0) {
        color = appliedTintColor;
        return;
    }

 	vec4 texColor = texture(tex, VertUV);

    texColor *= appliedTintColor;
    color = texColor;
}
`
