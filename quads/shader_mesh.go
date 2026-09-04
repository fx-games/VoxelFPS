package quads

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	MeshShaderModelTransform int = iota
	// MeshShaderEmissiveStrength is how far past white the glowing texels of this
	// mesh are pushed - 0 for everything that does not glow, which is every mesh
	// but the torch today. The depth-pass shader does not have it, and both are
	// handed the same draw code, hence the HasUniform check in MeshDrawer.Draw.
	MeshShaderEmissiveStrength
)

func LoadMeshShader(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
			{Name: "texCoord", Type: glhf.Vec2},
			{Name: "vertexColor", Type: glhf.Vec3},
			{Name: "normal", Type: glhf.Vec3},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "modelTransform", Type: glhf.Mat4},    // 0
			glhf.Attr{Name: "emissiveStrength", Type: glhf.Float}, // 1
		}
		shader *glhf.Shader
	)
	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, vertexShaderHeader(uboBlock)+meshVertexShaderSource, fragmentShaderHeader(uboBlock)+meshFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}

	shader.Begin()
	shader.BindSamplerNameToTextureUnit("shadowMap", gl.TEXTURE2)
	shader.End()

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}
func LoadMeshShaderDepthPass(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
			{Name: "texCoord", Type: glhf.Vec2},
			{Name: "vertexColor", Type: glhf.Vec3},
			{Name: "normal", Type: glhf.Vec3},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
		}
		shader *glhf.Shader
	)
	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, depthVertexShaderHeader(uboBlock)+meshVertexShaderSource, emptyFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}

const meshVertexShaderSource = `
layout(location = 0) in vec3 position;
layout(location = 1) in vec2 texCoord;
layout(location = 2) in vec3 vertexColor;
layout(location = 3) in vec3 normal;

uniform mat4 modelTransform;

#ifndef DEPTH_PASS
out vec3 VertPos;
out vec2 VertUV;
out vec3 VertNormal;
out vec3 VertColor;
#endif

void main() {
    gl_Position = camProjectionView * modelTransform * vec4(position, 1.0);

    // pass-through for fragment shader
#ifndef DEPTH_PASS
    VertUV = texCoord;
    VertNormal = normal;
    VertPos = position;
    VertColor = vertexColor;
#endif
}`

const meshFragmentShaderSource = `
in vec3 VertPos;
in vec2 VertUV;// tl: 0,0 tr: 1,0 bl: 0,1 br: 1,1
in vec3 VertNormal;
in vec3 VertColor;

uniform mat4 modelTransform;

uniform sampler2D tex;
uniform float emissiveStrength;

out vec4 fragmentColor;

void main() {
    vec4 surfaceColor = texture(tex, VertUV);
    
    if (surfaceColor.a == 0) {
        discard;
    }
    vec3 colorPerVertex = VertColor;
    
    vec3 fragWorldPos = (modelTransform * vec4(VertPos, 1.0)).xyz;
    vec3 fragWorldNormal = (modelTransform * vec4(VertNormal, 0.0)).xyz;

    // Same test the cube shader uses on an emissive face: a texel glows only if it
    // is both bright and colourful, so a torch's flame glows and the dark stick
    // holding it does not. Zero strength - every mesh but the torch - costs the
    // multiply and nothing else.
    //
    // ponytail: the two cutoffs are constants here, mirroring the cube shader's
    // defaults. They join emissiveStrength as uniforms if they ever need a slider.
    float luminance = dot(surfaceColor.rgb, vec3(0.2126, 0.7152, 0.0722));
    float maxChannel = max(max(surfaceColor.r, surfaceColor.g), surfaceColor.b);
    float minChannel = min(min(surfaceColor.r, surfaceColor.g), surfaceColor.b);
    float saturation = maxChannel > 0.0001 ? (maxChannel - minChannel) / maxChannel : 0.0;
    float emissiveMask = smoothstep(0.35, 0.45, luminance) * smoothstep(0.35, 0.45, saturation);
    vec3 emissive = surfaceColor.rgb * emissiveMask * emissiveStrength;

    fragmentColor = finalColor(surfaceColor, fragWorldPos, fragWorldNormal, emissive);
}
`
