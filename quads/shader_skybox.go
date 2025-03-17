package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    SkyboxShaderCamProjectionView int = iota
)

func LoadSkyboxShader(cam core.Camera) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "position", Type: glhf.Vec3},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "camProjectionView", Type: glhf.Mat4},
            //glhf.Attr{Name: "drawColor", Type: glhf.Vec3},
        }
        shader *glhf.Shader
    )
    var err error
    shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, skyboxVertexShaderSource, skyboxFragmentShaderSource)

    if err != nil {
        panic(err)
    }

    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }
    shader.Begin()
    shader.SetUniformAttr(0, cam.GetProjectionViewMatrix())
    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }

    return shader
}

const skyboxVertexShaderSource = `#version 330 core

in vec3 position;

uniform mat4 camProjectionView;

out vec3 TexCoords;

void main() {
    TexCoords = position;
    vec4 pos = camProjectionView * vec4(position, 1.0);
    gl_Position = pos.xyww;
}`

const skyboxFragmentShaderSource = `#version 330 core

layout (location = 0) out vec4 color;

in vec3 TexCoords;

uniform samplerCube skybox;

void main() {
    color = texture(skybox, TexCoords);
}`
