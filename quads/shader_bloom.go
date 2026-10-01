package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/platform/gl"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	BloomShaderMode int = iota
	BloomShaderTexelDirection
	BloomShaderThreshold
	BloomShaderBlurRadius
)

// Bloom modes. One shader does both jobs: the first pass reads the HDR scene and
// keeps only what is brighter than the threshold, every pass after that blurs.
const (
	BloomModeExtract int32 = 0
	BloomModeBlur    int32 = 1
)

// LoadBloomShader builds the extract/blur shader used by the bloom chain. It renders
// the same fullscreen quad as the post shader.
func LoadBloomShader() *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec2},
			{Name: "texCoord", Type: glhf.Vec2},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "mode", Type: glhf.Int},             // 0
			glhf.Attr{Name: "texelDirection", Type: glhf.Vec2},  // 1
			glhf.Attr{Name: "bloomThreshold", Type: glhf.Float}, // 2
			// Spacing between blur taps, in texels. Doubling it per pass pair widens
			// the glow geometrically without adding taps - a poor man's mip chain.
			glhf.Attr{Name: "blurRadius", Type: glhf.Float}, // 3
		}
	)
	shader, err := glhf.NewBasicShader(vertexFormat, uniformFormat, fullscreenVertexShaderSource, bloomFragmentShaderSource)
	if err != nil {
		panic(err)
	}
	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}

	shader.Begin()
	shader.SetUniformAttr(BloomShaderMode, BloomModeExtract)
	shader.SetUniformAttr(BloomShaderTexelDirection, mgl32.Vec2{1, 0})
	shader.SetUniformAttr(BloomShaderThreshold, DefaultBloomThreshold)
	shader.SetUniformAttr(BloomShaderBlurRadius, float32(1))
	shader.BindSamplerNameToTextureUnit("tex", gl.TEXTURE0)
	shader.End()

	if glErr := gl.GetError(); glErr != gl.NO_ERROR {
		panic(glErr)
	}
	return shader
}

// A 5-tap Gaussian, applied separably: horizontal then vertical is 10 taps for what a
// 2D kernel would spend 25 on, and repeating the pair widens the blur without widening
// the kernel. Weights are the normalised binomial row, which is close enough to a
// Gaussian that nothing in a glow will ever show the difference.
const bloomFragmentShaderSource = `#version 400 core

in vec2 VertUV;

uniform sampler2D tex;
uniform int mode;
uniform vec2 texelDirection;
uniform float bloomThreshold;
uniform float blurRadius;

out vec4 fragmentColor;

const float weights[5] = float[5](0.227027, 0.1945946, 0.1216216, 0.054054, 0.016216);

void main() {
    if (mode == 0) {
        // Extract: keep only the part of the colour that is over the threshold, so a
        // surface sitting just under it does not start glowing faintly. Working on the
        // HDR buffer is what makes this meaningful - a threshold above 1.0 picks out
        // genuinely blown-out highlights rather than merely bright texels.
        vec3 hdr = texture(tex, VertUV).rgb;
        fragmentColor = vec4(max(hdr - vec3(bloomThreshold), vec3(0.0)), 1.0);
        return;
    }

    vec2 texelStep = texelDirection * blurRadius / vec2(textureSize(tex, 0));
    vec3 result = texture(tex, VertUV).rgb * weights[0];
    for (int i = 1; i < 5; i++) {
        result += texture(tex, VertUV + texelStep * float(i)).rgb * weights[i];
        result += texture(tex, VertUV - texelStep * float(i)).rgb * weights[i];
    }
    fragmentColor = vec4(result, 1.0);
}
`
