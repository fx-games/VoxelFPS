package quads

import (
	"fmt"
	"math"

	"github.com/memmaker/goray/quads/glhf"
)

func vertexShaderHeader(uboBlock string) string {
	return fmt.Sprintf("#version 400 core\n%s\n", uboBlock)
}

// depthVertexShaderHeader is the vertex header for the shadow pass. The #define
// switches off the varyings the depth pass has no fragment shader to read, so the
// depth variant is the same source as the main one rather than a filtered copy.
func depthVertexShaderHeader(uboBlock string) string {
	return fmt.Sprintf("#version 400 core\n#define DEPTH_PASS\n%s\n", uboBlock)
}

func fragmentShaderHeader(uboBlock string) string {
	return fmt.Sprintf("#version 400 core\n%s\n%s", uboBlock, fragmentShaderSharedFunctions)
}

const (
	UboFogRange int = iota
	UboCamPosition
	UboCamDirection
	UboCamLightColor
	UboGlobalLightColor
	UboFogColor
	UboCamProjectionView
	UboToneMapParams
	UboShadowParams
	UboSunLight
)

func mainUBO() *glhf.UniformBufferObject {
	return glhf.NewUBOWithAttributes([]glhf.Attr{
		{Name: "fogRange", Type: glhf.Vec2},
		{Name: "camPosition", Type: glhf.Vec4},
		{Name: "camDirection", Type: glhf.Vec4},
		{Name: "camLightColor", Type: glhf.Vec4},
		{Name: "globalLightColor", Type: glhf.Vec4},
		{Name: "fogColor", Type: glhf.Vec4},
		{Name: "camProjectionView", Type: glhf.Mat4},
		{Name: "toneMapParams", Type: glhf.Vec2}, // x: exposure, y: gamma
		{Name: "shadowParams", Type: glhf.Vec2},  // x: normal offset scale, y: depth bias
		{Name: "sunLight", Type: glhf.Vec4},      // xyz: unit vector towards the sun, w: strength, 0 is off
	})
}

// DefaultExposure and DefaultGamma are what the tone map shipped with while it lived
// at the end of finalColor. They are the starting values for the runtime knobs.
const (
	DefaultExposure = float32(1.0)
	DefaultGamma    = float32(1.2)
)

// DefaultAmbientStrength is the fill light a level gets when it names none: white,
// at this strength - the flat grey the engine always started with.
const DefaultAmbientStrength = float32(0.2)

// DefaultShadowNormalOffset and DefaultShadowDepthBias are the two knobs of the
// normal-offset shadow bias. The offset is counted in shadow texels as they measure
// at the shaded fragment, so it means the same thing at any distance and any cone
// angle; the depth bias is only there for depth-buffer quantisation and is small on
// purpose. Both are exposed in the config menu (F1) because the right values depend
// on the shadow resolution and the map's scale, which no constant can know.
const (
	DefaultShadowNormalOffset = float32(2.0)
	DefaultShadowDepthBias    = float32(0.0005)
)

// HDRFromDisplay is the Go twin of the GLSL function of the same name: it takes a
// colour authored in display range and returns the linear HDR value that tone maps
// back to it. Needed for the clear colour, which is written into the HDR buffer
// directly rather than going through a shader.
//
// ponytail: the curve exists twice, here and in toneMapGLSL. Kept in step by
// TestHDRFromDisplayRoundTrips rather than by a shared code path, because Go and GLSL
// cannot share one.
func HDRFromDisplay(c, exposure, gamma float32) float32 {
	linear := float32(math.Pow(float64(clamp01(c)), float64(gamma)))
	if linear >= 1 {
		linear = 1 - 1e-4
	}
	return float32(-math.Log(float64(1-linear))) / exposure
}

// ToneMap is the Go twin of the GLSL toneMapping, used only to check the two agree.
func ToneMap(hdr, exposure, gamma float32) float32 {
	mapped := 1 - float32(math.Exp(float64(-hdr*exposure)))
	return float32(math.Pow(float64(mapped), 1/float64(gamma)))
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// toneMapGLSL is shared by the scene shaders, the skybox and the post pass. The scene
// now renders in linear HDR and is tone mapped once, in the post shader, so bloom can
// see how far past white a fragment went. Anything that mixes an authored display-range
// colour into the HDR buffer - the fog, the sky at the horizon - has to lift it with
// hdrFromDisplay first, or tone mapping would darken it on the way out.
const toneMapGLSL = `
vec3 toneMapping(vec3 hdrColor, float gamma, float exposure) {
    vec3 mapped = vec3(1.0) - exp(-hdrColor * exposure);
    // gamma correction
    mapped = pow(mapped, vec3(1.0 / gamma));
    return mapped;
}

// Inverse of toneMapping: the HDR value that tone maps back to this display colour.
vec3 hdrFromDisplay(vec3 displayColor, float gamma, float exposure) {
    vec3 linear = min(pow(clamp(displayColor, 0.0, 1.0), vec3(gamma)), vec3(1.0 - 1e-4));
    return -log(1.0 - linear) / exposure;
}
`

const fragmentShaderSharedFunctions = toneMapGLSL + `

uniform sampler2DArrayShadow shadowMap;

// Distance at which a light's intensity means what it says. Half brightness lands
// at 1.7x this, quarter at 2.6x.
#define LIGHT_REF_DIST 2.5

vec4 applyFog(vec3 transformedFragPos, vec4 inColor) {
    // Fog parameters, could make them uniforms and pass them into the fragment shader
    float fog_maxdist = fogRange.y;
    float fog_mindist = fogRange.x;
    
    // Calculate fog. The distance is measured on the horizontal plane only, the way Minecraft
    // does it, so tilting the head does not shift the fog across the world.
    float dist = length(camPosition.xz - transformedFragPos.xz);
    float fog_factor = 1.0 - smoothstep(fog_mindist, fog_maxdist, dist);
    
    // The scene buffer is linear HDR but fogColor is authored in display range, so it
    // is lifted to the HDR value that tone maps back to exactly the colour picked.
    vec3 fogHDR = hdrFromDisplay(fogColor.rgb, toneMapParams.y, toneMapParams.x);
    // Only the colour is mixed. Mixing the alpha too made fog decide coverage:
    // a translucent sprite far enough away came out with the fog's own alpha,
    // which is one, so a faded-to-nothing puff of smoke painted a solid
    // fog-coloured quad over everything behind it.
    return vec4(mix(fogHDR, inColor.rgb, fog_factor), inColor.a);
}

// Normal-offset shadow bias. A depth bias cannot work here: the slope-scaled term
// is near its maximum on a floor lit at a grazing angle and near its minimum on a
// wall lit head on, so the value that keeps walls free of acne erases floor shadows
// entirely. Nudging the sample position
// along the surface normal in *world* space instead takes the angle out of it - the
// offset is expressed in shadow texels as they measure at this fragment, so it stays
// the right size at any distance, any cone angle and any shadow resolution.
float ShadowCalculation(float shadowLayer, LightInfo light, vec3 fragWorldPos, vec3 normal, vec3 lightDir) {
    // How wide one shadow texel is in world units out here. direction.w carries the
    // cosine of the light's half FOV, which is also the half angle of its shadow
    // frustum, so the texel footprint follows from the distance alone.
    float dist = length(light.position.xyz - fragWorldPos);
    float cosHalfFov = clamp(light.direction.w, 0.001, 0.9999);
    float tanHalfFov = sqrt(1.0 - cosHalfFov * cosHalfFov) / cosHalfFov;
    float texelWorldSize = 2.0 * dist * tanHalfFov / float(textureSize(shadowMap, 0).x);

    // A grazing surface spans more depth across one texel, so it needs a bigger push.
    float grazing = clamp(1.0 - dot(normal, lightDir), 0.0, 1.0);
    vec3 samplePos = fragWorldPos + normal * (texelWorldSize * (1.0 + 2.0 * grazing) * shadowParams.x);

    vec4 fragPosLightSpace = light.projView * vec4(samplePos, 1.0);
    // Behind the light's near plane w goes negative and the divide below lands on
    // coordinates that look perfectly valid inside the map. There is no depth
    // information back there, so nothing behind the light is shadowed by it.
    if (fragPosLightSpace.w <= 0.0) {
        return 0.0;
    }
    // perspective divide, then into the [0,1] range the depth map is addressed in
    vec3 projCoords = fragPosLightSpace.xyz / fragPosLightSpace.w;
    projCoords = projCoords * 0.5 + 0.5;
    // outside the light's frustum there is no depth information, so nothing is shadowed
    if (projCoords.x < 0.0 || projCoords.x > 1.0 || projCoords.y < 0.0 || projCoords.y > 1.0 || projCoords.z > 1.0) {
        return 0.0;
    }
    // What is left of the depth bias is a small constant against depth-buffer
    // quantisation. The slope-scaled term is gone: the normal offset replaces it.
    float reference = projCoords.z - shadowParams.y;

    // The sampler compares in hardware, so each fetch is already a 2x2 PCF tap.
    // 3x3 taps on top of that soften the 512x512 edges without a big cost.
    // GLSL has no textureOffset overload for a shadow array sampler, so the taps
    // are offset by hand, one texel apart.
    vec2 texelSize = 1.0 / vec2(textureSize(shadowMap, 0).xy);
    float lit = 0.0;
    for (int x = -1; x <= 1; x++) {
        for (int y = -1; y <= 1; y++) {
            vec2 tap = projCoords.xy + vec2(x, y) * texelSize;
            lit += texture(shadowMap, vec4(tap, shadowLayer, reference));
        }
    }
    return 1.0 - (lit / 9.0);
}

vec3 calculateAmbientLight() {
    // ambient light
    float ambientStrength = globalLightColor.a;
    vec3 ambientLightColor = globalLightColor.rgb;

    vec3 ambient = ambientStrength * ambientLightColor;

    return ambient;
}

// sunLightAt is one directional light from a fixed side of the sky, so the faces of
// a block come out six different shades and the shape reads - flat ambient alone
// paints every face the same and the level looks like a drawing of itself.
//
// ponytail: wrapped (half) Lambert and unshadowed. Plain Lambert leaves the three
// faces turned away from the sun at the same ambient-only shade, which is exactly
// the flatness this is here to remove. Shadowing it needs its own orthographic
// shadow map - add that if the sun is ever meant to be seen in play, not only
// to build by.
vec3 sunLightAt(vec3 fragWorldNormal) {
    if (sunLight.w <= 0.0) {
        return vec3(0.0);
    }
    float wrapped = 0.5 + 0.5 * dot(normalize(fragWorldNormal), sunLight.xyz);
    return vec3(sunLight.w * wrapped * wrapped);
}

// Reach, cone and intensity come from the light itself, packed into the spare
// components of its vec4s (see LightInfo.packed). They used to be the same three
// constants for every light in the world, with reach borrowed from fogRange - so
// moving the fog slider relit the level, and no light could be brighter, longer
// or wider than any other.
vec3 calcLight(vec3 viewPos, LightInfo light, vec3 fragWorldPos, vec3 fragWorldNormal) {
    vec3 normal = normalize(fragWorldNormal);
    vec3 lightPos = light.position.xyz;
    float range = light.position.w;
    float coneOuter = light.direction.w;
    vec3 lightColor = light.color.rgb * light.color.a;

    vec3 toLight = lightPos - fragWorldPos;
    float dist = max(length(toLight), 0.0001);
    vec3 lightDir = toLight / dist;

    // A cone cosine of -1 marks an omni light (LightInfo.Omni): it has no cone, so
    // the cone test below is skipped for it.
    bool isOmni = coneOuter <= -1.0;
    // params.x is this light's layer in the shadow atlas, or -1 when it has none:
    // an omni light, or a spotlight that arrived after the last layer was handed
    // out. It used to be the light's index in the list, which is what tied the
    // number of lights in a map to the depth of the atlas. See assignShadowLayers.
    float shadowLayer = light.params.x;
    float cone = 1.0;
    if (!isOmni) {
        float theta = dot(lightDir, normalize(-light.direction.xyz));
        // one authored angle: the cone's outer edge is the light's own half FOV, and
        // the rim softens over half the remaining angle to the axis.
        cone = smoothstep(coneOuter, mix(coneOuter, 1.0, 0.5), theta);
        if (cone <= 0.0) {
            return vec3(0.0);
        }
    }
    // Inverse square, the way light actually falls off. Without it a wall one unit
    // away and a wall eight units away were both at full brightness, so the only
    // thing separating them was the diffuse angle - which is why a corridor's near
    // side walls went black while the far wall it points at stayed bright. Real
    // light buys the near wall back: it is 30x closer in energy terms, which more
    // than pays for its grazing angle.
    //
    // ponytail: referenced to LIGHT_REF_DIST so intensity stays a readable number
    // rather than the physical value at zero distance. Faded out over the last of
    // the light's reach, because past its far plane there is no shadow map for it.
    float distFactor = dist / LIGHT_REF_DIST;
    float falloff = (1.0 / (1.0 + distFactor * distFactor))
                  * (1.0 - smoothstep(range * 0.7, range, dist));
    if (falloff <= 0.0) {
        return vec3(0.0);
    }
    float diff = max(dot(lightDir, normal), 0.0);
    vec3 diffuse = diff * lightColor;
    // specular
    // A flashlight sits on the eye, so halfwayDir is almost the face normal on any
    // surface facing the camera: this term was firing at full light intensity on
    // head-on walls and at nothing on grazing ones, doubling the very gap that made
    // grazing walls look unlit. Voxel faces have one constant normal, so it cannot
    // be a highlight anyway - it is a flat per-face bonus. Kept small as a knob.
    vec3 viewDir = normalize(viewPos - fragWorldPos);
    vec3 halfwayDir = normalize(lightDir + viewDir);
    float spec = pow(max(dot(normal, halfwayDir), 0.0), 64.0);
    vec3 specular = spec * 0.15 * lightColor;
    // calculate shadow
    float shadow = shadowLayer < 0.0 ? 0.0 : ShadowCalculation(shadowLayer, light, fragWorldPos, normal, lightDir);
    
    vec3 lighting = (1.0 - shadow) * (diffuse + specular) * cone * falloff;    
    //vec3 lighting = (diffuse + specular);
    return lighting;
}

// BAKED_LIGHT_COLOR and BAKED_LIGHT_STRENGTH turn a block's flood-filled light level
// into a colour. Cool white because what emits it in the shipped map is cyan tubes and
// console screens, and below 1.0 because this is fill light: it lifts a room off pure
// ambient without competing with a shadow-mapped spotlight.
//
// ponytail: constants, not uniforms. The UBO is shared by every shader and this is one
// value that has never needed to move at runtime. If it wants a slider, it joins
// shadowParams in mainUBO.
const vec3 BAKED_LIGHT_COLOR = vec3(0.85, 0.92, 1.0);
const float BAKED_LIGHT_STRENGTH = 0.55;

// bakedLight converts a 0..15 voxel light level into light. Unshadowed and
// directionless on purpose - the level already encodes how far the light got around
// corners, which is the part a shadow map would otherwise have to pay for.
vec3 bakedLight(float lightLevel) {
    return BAKED_LIGHT_COLOR * BAKED_LIGHT_STRENGTH * (lightLevel / 15.0);
}

// The two halves of "which texels of a surface glow". They are uniforms in the shared
// header, and therefore the same numbers in every shader that draws a surface, because
// they are one look decision for the whole scene: a wall panel, a torch head and a
// plasma bolt disagreeing about what counts as glowing is not art direction, it is a
// bug you only see in one of the three. Both are on the F1 menu.
uniform float emissiveCutoff;
uniform float emissiveSaturation;

// emissiveMask picks the texels that glow out of a surface that may glow: a texel has
// to be both bright and colourful. That is what tells a cyan tube or a blue screen
// readout apart from the grey metal panel it is mounted on, which is just as bright.
// Soft edges on both terms so the boundary does not crawl as the texture minifies.
//
// How hard the glow is pushed is not decided here - see the emissiveStrength of each
// shader, which is global for walls, per mesh for models and per instance for sprites.
float emissiveMask(vec3 color) {
    float luminance = dot(color, vec3(0.2126, 0.7152, 0.0722));
    float maxChannel = max(max(color.r, color.g), color.b);
    float minChannel = min(min(color.r, color.g), color.b);
    float saturation = maxChannel > 0.0001 ? (maxChannel - minChannel) / maxChannel : 0.0;
    return smoothstep(emissiveCutoff, emissiveCutoff + 0.1, luminance)
         * smoothstep(emissiveSaturation, emissiveSaturation + 0.1, saturation);
}

// emissive is added after the lighting multiply and before the fog: a glowing surface
// does not care how lit it is, but it does still fade into the distance like everything
// else. Values above 1.0 are the point - that is what the bloom threshold picks up.
//
// baked is the flood-filled voxel light at this fragment, and it multiplies the surface
// like any other light does. occlusion is how much of the light that comes from
// everywhere - ambient, sun, baked - reaches this fragment past the blocks around it;
// a spotlight's shadow map already answers that for its own light, and emissive is
// not light arriving at all. Overloaded rather than given the extra arguments
// everywhere: meshes have neither, sprites have a light level but no neighbours.
vec4 finalColor(vec4 surfaceColor, vec3 fragWorldPos, vec3 fragWorldNormal, vec3 emissive, vec3 baked, float occlusion);

vec4 finalColor(vec4 surfaceColor, vec3 fragWorldPos, vec3 fragWorldNormal, vec3 emissive) {
    return finalColor(surfaceColor, fragWorldPos, fragWorldNormal, emissive, vec3(0.0), 1.0);
}

vec4 finalColor(vec4 surfaceColor, vec3 fragWorldPos, vec3 fragWorldNormal, vec3 emissive, vec3 baked) {
    return finalColor(surfaceColor, fragWorldPos, fragWorldNormal, emissive, baked, 1.0);
}

vec4 finalColor(vec4 surfaceColor, vec3 fragWorldPos, vec3 fragWorldNormal, vec3 emissive, vec3 baked, float occlusion) {

    vec3 ambientLightColor = calculateAmbientLight();

    // GLSL does not zero locals: this is accumulated into below, so it has to start at 0.
    vec3 spotLightColor = vec3(0.0);
    uint lightCount = uint(lightCounts.x);
    for (uint i = 0; i < lightCount; i++) {
        LightInfo curLight = lights[i];
        // switched off, and its shadow map is stale
        if (curLight.color.rgb == vec3(0.0) || curLight.color.a <= 0.0) {
            continue;
        }
        spotLightColor += calcLight(camPosition.xyz, curLight, fragWorldPos, fragWorldNormal);
    }
    
    // FLASHLIGHT HERE
    //vec3 spotLightColor = calculateSpotLight(fragWorldNormal, fragWorldPos, camPosition, light_color, camDirection, 0.98,  0.96);
    //vec3 litColor = (ambientLightColor + pointLightColor + spotLightColor) * surfaceColor.rgb;

    vec3 litColor = ((ambientLightColor + sunLightAt(fragWorldNormal) + baked) * occlusion + spotLightColor) * surfaceColor.rgb + emissive;

    // No tone mapping here any more. Anything the flashlight hits head-on still runs
    // past 1.0, but it now stays past 1.0 all the way into the post pass, where it is
    // tone mapped once - and where the bloom threshold can pick it out. Tone mapping
    // per fragment clipped that information away before anything could use it.
    vec4 foggedColor = applyFog(fragWorldPos, vec4(litColor, surfaceColor.a));
    
    return foggedColor;
}
`
