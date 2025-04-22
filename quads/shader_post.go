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
uniform sampler2D depthStencil;

uniform vec4 appliedTintColor;

//layout(origin_upper_left) in vec4 gl_FragCoord;

out vec4 fragmentColor;

void main() {
    ivec2 textureSize2d = textureSize(tex, 0);
    float offsetX = 1.0 / float(textureSize2d.x);
    float offsetY = 1.0 / float(textureSize2d.y);

    vec2 offsets[9] = vec2[](
        vec2(0.0f, 2*offsetY),   // up
        vec2( 0.0f,    offsetY), // top-center
        vec2(-2*offsetX, 0.0f),   // left
        vec2(-offsetX,  0.0f),   // center-left
        
        vec2( 0.0f,    0.0f),   // center-center
        vec2( offsetX,  0.0f),   // center-right
        vec2(2*offsetX, 0.0f),   // right
        vec2( 0.0f,   -offsetY), // bottom-center

        vec2(0.0f, -2*offsetY)   // down
    );

    float kernel[9] = float[](
                       2.0/9.0, 
                       1.0/9.0, 
     2.0/9.0, 1.0/9.0, 0.0/9.0, 1.0/9.0, 2.0/9.0,
                       1.0/9.0,
                       2.0/9.0
    );
    
    vec3 col = vec3(0.0); 
    for (int i = 0; i < 9; i++) {
        vec3 sampleTex = vec3(texture(tex, VertUV.st + offsets[i]));
        col += sampleTex * kernel[i];
    }

    vec4 texColor = texture(tex, VertUV) * appliedTintColor;
    

    float depthValue = texture(depthStencil,VertUV).r;//access the 24 first bit, 
    float depthStep = step(0.995, depthValue);

    fragmentColor = (1.0 - depthStep) * texColor + depthStep * vec4(col, 1.0);
    fragmentColor = vec4(col, 1.0);
}
/*
void main() {
    vec2 uv = VertUV;

    float depthValue = texture(depthStencil,uv).r;//access the 24 first bit, 
    //float worldDepth = nearPlane + ( farPlane - nearPlane ) * depthBufferValue;
    float depthStep = step(0.995, depthValue);
    vec4 texColor = texture(tex, VertUV) * appliedTintColor;
    fragmentColor = (1.0 - depthStep) * texColor + depthStep * vec4(0.0, 0.0, 0.0, 1.0);
}
*/
`
