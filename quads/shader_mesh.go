package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/memmaker/goray/quads/glhf"
)

const (
    MeshShaderModelTransform int = iota
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
            glhf.Attr{Name: "modelTransform", Type: glhf.Mat4}, // 0
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

out vec4 fragmentColor;

void main() {
    vec4 surfaceColor = texture(tex, VertUV);
    
    if (surfaceColor.a == 0) {
        discard;
    }
    vec3 colorPerVertex = VertColor;
    
    vec3 fragWorldPos = (modelTransform * vec4(VertPos, 1.0)).xyz;
    vec3 fragWorldNormal = (modelTransform * vec4(VertNormal, 0.0)).xyz;
    
    fragmentColor = finalColor(surfaceColor, fragWorldPos, fragWorldNormal, vec3(0.0));
}
`
