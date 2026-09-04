package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    PostShaderBloomStrength = 0 + iota
)

// Bloom defaults. The threshold is above 1.0 on purpose: the scene is linear HDR, so
// only genuinely blown-out fragments - the flashlight hitting a wall head on, a lamp
// seen directly - should glow. Strength is how much of the blurred result is added
// back on top.
//
// Calibrated against what this scene actually produces, which is a lot hotter than the
// first guess assumed. The static lamps run at intensity 6, so anything near one is
// past 1.0 already, and the flashlight puts a facing wall around 2.0. A threshold at
// white therefore blooms ordinary lit geometry; 1.5 keeps it to genuinely blown-out
// things, which the emissive screens (strength 4 on a masked texel, so ~3.5) clear
// easily. The original 1.1/0.35 was arithmetic nonsense in the other direction: an
// emissive surface at 1.2 lost almost everything to the threshold and what survived was
// divided by three, giving a glow of about 0.03 - invisible, which is what prompted this.
const (
    DefaultBloomThreshold = float32(1.5)
    DefaultBloomStrength  = float32(0.6)
    // bloomBlurPasses is how many horizontal+vertical pairs the chain runs, each at
    // twice the tap spacing of the last. Three pairs starting at 2 texels reach ~32
    // texels of quarter-resolution buffer, which is a wide, soft glow for six passes
    // over a sixteenth of the screen.
    bloomBlurPasses    = 3
    bloomBaseBlurRadius = float32(2)
)

// LoadPostShader takes the main UBO block so the tone map reads the same exposure and
// gamma the scene shaders use - one source of truth, fed from mainUBO().
func LoadPostShader(uboBlock string) *glhf.Shader {
    var (
        vertexFormat = glhf.AttrFormat{
            {Name: "position", Type: glhf.Vec2},
            {Name: "texCoord", Type: glhf.Vec2},
        }
        uniformFormat = glhf.AttrFormat{
            glhf.Attr{Name: "bloomStrength", Type: glhf.Float}, // 0
        }
    )
    shader, err := glhf.NewBasicShader(vertexFormat, uniformFormat, fullscreenVertexShaderSource, postFragmentShaderSource(uboBlock))
    if err != nil {
        panic(err)
    }
    if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
        panic(errorMsg)
    }

    shader.Begin()
    shader.SetUniformAttr(PostShaderBloomStrength, DefaultBloomStrength)
    shader.BindSamplerNameToTextureUnit("tex", gl.TEXTURE0)
    shader.BindSamplerNameToTextureUnit("bloomTex", gl.TEXTURE1)
    shader.End()

    if glErr := gl.GetError(); glErr != gl.NO_ERROR {
        panic(glErr)
    }
    return shader
}

// fullscreenVertexShaderSource is shared by the post and bloom shaders - both draw the
// same screen quad. The offset and scale uniforms it used to carry were set to identity
// at load and never touched again.
const fullscreenVertexShaderSource = `#version 400 core

in vec2 position;
in vec2 texCoord;

out vec2 VertUV;

void main() {
    gl_Position = vec4(position, 0.0, 1.0);
    VertUV = vec2(texCoord.x, 1.0 - texCoord.y);
}
`

// The one place the scene leaves HDR. Bloom is added in linear space, before the tone
// map, so a glow around a blown-out highlight rolls off with everything else instead of
// being pasted on top of an already-compressed image.
func postFragmentShaderSource(uboBlock string) string {
    return `#version 400 core
` + uboBlock + toneMapGLSL + `

in vec2 VertUV;

uniform sampler2D tex;
uniform sampler2D bloomTex;
uniform float bloomStrength;

out vec4 fragmentColor;

void main() {
    vec3 hdr = texture(tex, VertUV).rgb;
    hdr += texture(bloomTex, VertUV).rgb * bloomStrength;
    fragmentColor = vec4(toneMapping(hdr, toneMapParams.y, toneMapParams.x), 1.0);
}
`
}
