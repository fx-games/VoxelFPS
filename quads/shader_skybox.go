package quads

import (
	"fmt"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/platform/gl"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	SkyboxShaderCamProjectionView int = iota
	SkyboxShaderHorizonFade
)

func LoadSkyboxShader(cam core.Camera, uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
		}
		uniformFormat = glhf.AttrFormat{
			// Not camProjectionView: the fragment stage includes the Main block, which has
			// a field of that name, and GLSL ES does not allow the two to share it.
			glhf.Attr{Name: "skyProjectionView", Type: glhf.Mat4},
			glhf.Attr{Name: "horizonFadeDegrees", Type: glhf.Float},
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
	shader.SetUniformAttr(SkyboxShaderHorizonFade, float32(7))
	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}

const skyboxVertexShaderSource = `#version 400 core

in vec3 position;

uniform mat4 skyProjectionView;

out vec3 TexCoords;

void main() {
    TexCoords = position;
    vec4 pos = skyProjectionView * vec4(position, 1.0);
    gl_Position = pos.xyww;
}`

func skyboxFragmentShaderSource(uboBlock string) string {
	return fmt.Sprintf(`#version 400 core
%s
%s
layout (location = 0) out vec4 color;

in vec3 TexCoords;

uniform samplerCube skybox;
uniform float horizonFadeDegrees;

void main() {
    // The sky meets the fog at the horizon: everything below it is solid fog colour, and the
    // sky fades in over a narrow band above it (Minecraft uses 7 degrees). The clear colour is
    // the fog colour, so the geometry, the fog and the sky all end up the same colour there.
    // A fade of zero or less is a level with no horizon at all - space - where
    // the cubemap covers the whole sphere and there is no fog below to meet.
    float pitch = asin(clamp(normalize(TexCoords).y, -1.0, 1.0));
    float skyAmount = 1.0;
    if (horizonFadeDegrees > 0.0) {
        float fadeWindow = radians(horizonFadeDegrees);
        skyAmount = smoothstep(0.0, 1.0, clamp(pitch / fadeWindow, 0.0, 1.0));
    }
    // Written into the linear HDR scene buffer, so both the fog colour and the cubemap
    // sample - authored in display range - are lifted to the HDR values that tone map
    // back to themselves in the post pass.
    vec3 skyDisplay = mix(fogColor.rgb, texture(skybox, TexCoords).rgb, skyAmount);
    color = vec4(hdrFromDisplay(skyDisplay, toneMapParams.y, toneMapParams.x), 1.0);
}`, uboBlock, toneMapGLSL)
}
