package quads

import (
    "fmt"
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    SkyboxShaderCamProjectionView int = iota
    SkyboxShaderFogWidth
)

func LoadSkyboxShader(cam core.Camera, uboBlock string) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "position", Type: glhf.Vec3},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "camProjectionView", Type: glhf.Mat4},
            glhf.Attr{Name: "fogWidth", Type: glhf.Float},
            //glhf.Attr{Name: "drawColor", Type: glhf.Vec3},
        }
        shader *glhf.Shader
    )
    var err error
    shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, skyboxVertexShaderSource, skyboxFragmentShaderSource(uboBlock))

    if err != nil {
        panic(err)
    }

    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }
    shader.Begin()
    shader.SetUniformAttr(SkyboxShaderCamProjectionView, cam.GetProjectionViewMatrix())
    shader.SetUniformAttr(SkyboxShaderFogWidth, float32(0.4))
    shader.End()

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }

    return shader
}

const skyboxVertexShaderSource = `#version 400 core

in vec3 position;

uniform mat4 camProjectionView;

out vec3 TexCoords;

void main() {
    TexCoords = position;
    vec4 pos = camProjectionView * vec4(position, 1.0);
    gl_Position = pos.xyww;
}`

func skyboxFragmentShaderSource(uboBlock string) string {
    return fmt.Sprintf(`#version 400 core
%s
layout (location = 0) out vec4 color;

in vec3 TexCoords;

uniform samplerCube skybox;
uniform float fogWidth;

void main() {
    // the sky is further away than any fog distance, so it is solid fog towards the horizon
    // and thins out overhead, which hides the seam where the geometry ends
    float openness = abs(normalize(TexCoords).y);
    float fogAmount = 1.0 - smoothstep(0.0, fogWidth, openness);
    color = mix(texture(skybox, TexCoords), fogColor, fogAmount);
}`, uboBlock)
}
