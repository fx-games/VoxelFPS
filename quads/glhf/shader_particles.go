package glhf

// 48 + 8 = 56
func getParticleVertexFormat() AttrFormat {
    return AttrFormat{
        {Name: "position", Type: Vec3},      // 3*4
        {Name: "lifetimeLeft", Type: Float}, // 4
        {Name: "velocity", Type: Vec3},      // 3*4
        {Name: "sizeBegin", Type: Float},    // 4
        {Name: "colorBegin", Type: Vec3},    // 3*4
        {Name: "origin", Type: Vec3},        // 3*4
    }
}
func loadTransformFeedbackShader(vertexFormat AttrFormat) *Shader {
    uniformFormat := AttrFormat{
        Attr{Name: "deltaTime", Type: Float},
        Attr{Name: "maxDistance", Type: Float},
        Attr{Name: "lifetime", Type: Float},
    }

    tfShader, shaderErr := NewShader(
        vertexFormat,
        uniformFormat,
        transformFeedbackVertexShaderSource,
        "",
        "",
        []string{"VS_OUT.position\x00", "VS_OUT.lifetimeLeft\x00", "VS_OUT.velocity\x00", "VS_OUT.sizeBegin\x00", "VS_OUT.colorBegin\x00", "VS_OUT.origin\x00"},
    )
    if shaderErr != nil {
        panic(shaderErr)
    }

    return tfShader
}

func loadParticleShader(vertexFormat AttrFormat) *Shader {
    uniformFormat := AttrFormat{
        Attr{Name: "projection", Type: Mat4},
        Attr{Name: "modelView", Type: Mat4},

        Attr{Name: "lifetime", Type: Float},
        //glhf.Attr{Name: "colorBegin", Type: glhf.Vec4},
        Attr{Name: "colorEnd", Type: Vec3},
        Attr{Name: "sizeEnd", Type: Float},
    }

    particleShader, shaderErr := NewShader(
        vertexFormat,
        uniformFormat,
        quadParticleVertexShaderSource,
        quadParticleGeometryShaderSource,
        quadParticleFragmentShaderSource,
        nil,
    )
    if shaderErr != nil {
        panic(shaderErr)
    }

    return particleShader
}

const transformFeedbackVertexShaderSource = `#version 400 core

uniform float deltaTime;
uniform float maxDistance;
uniform float lifetime;

in vec3 position;
in float lifetimeLeft;
in vec3 velocity;
in float sizeBegin;
in vec3 colorBegin;
in vec3 origin;

out VS_OUT {
    vec3 position;
    float lifetimeLeft;
    vec3 velocity;
    float sizeBegin;
    vec3 colorBegin;
    vec3 origin;
} vs_out;

void main() {
    vec3 newPos = position + (velocity * deltaTime);
    vec3 newVelocity = velocity;

    if (maxDistance > 0.0) {
        float distance = length(newPos - origin);
        if (distance > maxDistance) {
            newVelocity = -velocity;
            newPos = position + (newVelocity * deltaTime);
        }
    }

    float newLifetimeLeft = max(lifetimeLeft - deltaTime, 0.0);

    if (lifetime < -100.0) { // no reduction for infinite lifetime particles
        newLifetimeLeft = lifetimeLeft;
    } else if (lifetime < 0.0 && newLifetimeLeft <= 0) { // loop
        newLifetimeLeft = lifetime * -1.0;
        newPos = origin;
    }

    vs_out.position = newPos;
    vs_out.lifetimeLeft = newLifetimeLeft;

    vs_out.velocity = newVelocity;
    vs_out.sizeBegin = sizeBegin;

    vs_out.colorBegin = colorBegin;
    vs_out.origin = origin;
}`

const quadParticleGeometryShaderSource = `#version 400 core

layout (points) in;
layout (triangle_strip, max_vertices = 4) out;

in VS_OUT {
    float lifetimeLeft;
    float sizeBegin;
    vec3 colorBegin;
    vec3 origin;
} gs_in[];

out GS_OUT {
    float lifetimeLeft;
    vec3 color;
} gs_out;


uniform mat4 projection;
uniform mat4 modelView;
uniform float lifetime;
uniform float sizeEnd;
uniform vec3 colorEnd;

// take a point as input and output a quad as triangle strip
void main() {
    float lifeLeft = gs_in[0].lifetimeLeft;

    if (lifeLeft <= 0.0) {
        return;
    }
    float percentOfLifeLeft = lifeLeft / lifetime;
    if (lifetime < -100.0) { // infinite lifetime
        percentOfLifeLeft = 1.0;
        lifeLeft = 1.0;
    } else if (lifetime < 0.0) { // loop lifetime
        percentOfLifeLeft = lifeLeft / -lifetime;
    }
    float currentSize = mix(gs_in[0].sizeBegin, sizeEnd, 1-percentOfLifeLeft);
    vec3 currentColor = mix(gs_in[0].colorBegin, colorEnd, 1-percentOfLifeLeft);

    gs_out.color = currentColor;
    gs_out.lifetimeLeft = lifeLeft;

    vec4 inputPosition = gl_in[0].gl_Position;

    vec2 origin = inputPosition.xy;

    float quadSize = currentSize;

    vec2 up = vec2(0, 1);

    vec2 right = vec2(1, 0);

    vec2 va = origin + (up * quadSize) + (right * quadSize);
    gl_Position = projection * vec4(va, inputPosition.zw);
    EmitVertex();

    vec2 vb = origin + (up * quadSize) - (right * quadSize);
    gl_Position = projection * vec4(vb, inputPosition.zw);
    EmitVertex();

    vec2 vc = origin - (up * quadSize) + (right * quadSize);
    gl_Position = projection * vec4(vc, inputPosition.zw);
    EmitVertex();

    vec2 vd = origin - (up * quadSize) - (right * quadSize);
    gl_Position = projection * vec4(vd, inputPosition.zw);
    EmitVertex();

    EndPrimitive();
}`

const quadParticleVertexShaderSource = `#version 400 core

in vec3 position;
in float lifetimeLeft;
in vec3 velocity;
in float sizeBegin;
in vec3 colorBegin;
in vec3 origin;

uniform mat4 projection;
uniform mat4 modelView;

out VS_OUT {
    float lifetimeLeft;
    float sizeBegin;
    vec3 colorBegin;
    vec3 origin;
} vs_out;

void main() {
    gl_Position = modelView * vec4(position, 1.0); // we just pass the position of this particle to the geometry shader
    vs_out.lifetimeLeft = lifetimeLeft; // from here on the pipeline looks good. setting this directly has visible resulta
    vs_out.sizeBegin = sizeBegin;
    vs_out.origin = origin;
    vs_out.colorBegin = colorBegin;
}`

const quadParticleFragmentShaderSource = `#version 400 core

out vec4 color;

in GS_OUT {
    float lifetimeLeft;
    vec3 color;
} fs_in;

uniform float lifetime;

void main() {
    float percentOfLifeLeft = fs_in.lifetimeLeft / lifetime;
    //vec4 lerpedColor = mix(colorBegin, colorEnd, 1-percentOfLifeLeft);
    color = vec4(fs_in.color, 1.0);
}`
