package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    PostShaderPositionOffset = 0 + iota
    PostShaderScaleFactor
    PostShaderTintColor
)

func LoadPostShader() *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "position", Type: glhf.Vec2},
            {Name: "texCoord", Type: glhf.Vec2},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "positionOffset", Type: glhf.Vec2},   // 0
            glhf.Attr{Name: "scaleFactor", Type: glhf.Vec2},      // 1
            glhf.Attr{Name: "appliedTintColor", Type: glhf.Vec4}, // 2
        }
        shader *glhf.Shader
    )
    var err error
    shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, postVertexShaderSource, postFragmentShaderSource)

    if err != nil {
        panic(err)
    }

    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }

    shader.Begin()
    // scale using the projection matrix
    shader.SetUniformAttr(PostShaderPositionOffset, mgl32.Vec2{0, 0})
    shader.SetUniformAttr(PostShaderTintColor, mgl32.Vec4{1, 1, 1, 1})
    shader.SetUniformAttr(PostShaderScaleFactor, mgl32.Vec2{1, 1})

    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
    return shader
}

const postVertexShaderSource = `
#version 400 core

in vec2 position;
in vec2 texCoord;

uniform vec2 scaleFactor;
uniform vec2 positionOffset;

out vec2 VertUV;

void main() {
    // gl_InstanceID, will change for every instance (quad)
    // gl_VertexID, will change for every vertex index (6 indices per quad)

    vec2 pos = (position * scaleFactor) + positionOffset;
    gl_Position = vec4(pos, 0.0, 1.0);

    // pass-through for fragment shader
    VertUV = vec2(texCoord.x, 1.0-texCoord.y);
}
`

const postFragmentShaderSource = `
#version 400 core

in vec2 VertUV;

uniform sampler2D tex;
//uniform sampler2D depthStencil;

uniform vec4 appliedTintColor;

out vec4 fragmentColor;

void main() {
//    ivec2 textureSize2d = textureSize(tex, 0);
//    float offsetX = 1.0 / float(textureSize2d.x);
//    float offsetY = 1.0 / float(textureSize2d.y);

    vec4 texColor = texture(tex, VertUV) * appliedTintColor;

    //float depthValue = texture(depthStencil,VertUV).r;//access the 24 first bit, 

    fragmentColor = texColor;
}
`
