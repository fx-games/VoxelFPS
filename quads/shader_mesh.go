package quads

import (
	"github.com/memmaker/goray/platform/gl"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	// The two thresholds deciding which of this mesh's texels count as glowing.
	// Shared with the cube and sprite shaders and driven from the F1 menu - see
	// emissiveMask in the fragment header.
	MeshShaderEmissiveCutoff int = iota
	MeshShaderEmissiveSaturation
)

// MeshInstanceFormat is what one drawn copy of a model part carries: where it
// stands, as a mat4 at locations 4 to 7, and how it is lit, at 8 - its emissive
// strength and the baked voxel light level where it stands (see MeshComponent).
// The locations are pinned in the shader so the lit and the depth program read
// the same vertex array.
var MeshInstanceFormat = []glhf.InstanceAttr{
	{Location: 4, Floats: 16},
	{Location: 8, Floats: 2},
}

const meshInstanceFloats = 18

func LoadMeshShader(uboBlock string) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
			{Name: "texCoord", Type: glhf.Vec2},
			{Name: "vertexColor", Type: glhf.Vec3},
			{Name: "normal", Type: glhf.Vec3},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "emissiveCutoff", Type: glhf.Float},     // 0
			glhf.Attr{Name: "emissiveSaturation", Type: glhf.Float}, // 1
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
	shader.SetUniformAttr(MeshShaderEmissiveCutoff, DefaultEmissiveCutoff)
	shader.SetUniformAttr(MeshShaderEmissiveSaturation, DefaultEmissiveSaturation)
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
		uniformFormat = glhf.AttrFormat{}
		shader        *glhf.Shader
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
// One drawn copy of the part - see MeshInstanceFormat.
layout(location = 4) in mat4 instanceTransform;
layout(location = 8) in vec2 instanceLight;// x: emissive strength, y: baked light 0..15

#ifndef DEPTH_PASS
out vec3 VertWorldPos;
out vec2 VertUV;
out vec3 VertWorldNormal;
flat out vec2 VertLight;
#endif

void main() {
    vec4 worldPos = instanceTransform * vec4(position, 1.0);
    gl_Position = camProjectionView * worldPos;

#ifndef DEPTH_PASS
    VertUV = texCoord;
    // Here rather than per fragment: the transform is affine, so interpolating
    // the transformed values is what the fragment used to work out itself.
    VertWorldPos = worldPos.xyz;
    VertWorldNormal = (instanceTransform * vec4(normal, 0.0)).xyz;
    VertLight = instanceLight;
#endif
}`

const meshFragmentShaderSource = `
in vec3 VertWorldPos;
in vec2 VertUV;// tl: 0,0 tr: 1,0 bl: 0,1 br: 1,1
in vec3 VertWorldNormal;
// glhf fails a program on any link log, and the linker warns about a varying the
// fragment stage never reads, so the vertex colour the model format carries is
// not passed on at all rather than passed on and ignored.
flat in vec2 VertLight;// x: emissive strength, y: baked light 0..15

uniform sampler2D tex;

out vec4 fragmentColor;

void main() {
    vec4 surfaceColor = texture(tex, VertUV);

    if (surfaceColor.a == 0.0) {
        discard;
    }

    // The same emissiveMask a wall face and a sprite are put through, so a torch's
    // flame glows and the dark stick holding it does not, by the same numbers.
    // Zero strength - every mesh but the torch - costs the multiply and nothing else.
    vec3 emissive = surfaceColor.rgb * emissiveMask(surfaceColor.rgb) * VertLight.x;

    fragmentColor = finalColor(surfaceColor, VertWorldPos, VertWorldNormal, emissive, bakedLight(VertLight.y));
}
`
