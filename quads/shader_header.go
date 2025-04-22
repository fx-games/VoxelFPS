package quads

import (
    "fmt"
    "github.com/memmaker/goray/quads/glhf"
)

func vertexShaderHeader(uboBlock string) string {
    return fmt.Sprintf("#version 400 core\n%s\n", uboBlock)
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
    })
}

const fragmentShaderSharedFunctions = `

uniform sampler2DArray shadowMap;

vec3 toneMapping(vec3 hdrColor, float gamma, float exposure) {
    vec3 mapped = vec3(1.0) - exp(-hdrColor * exposure);
    // gamma correction
    mapped = pow(mapped, vec3(1.0 / gamma));
    return mapped;
}

vec4 applyFog(vec3 transformedFragPos, vec4 inColor) {
    // Fog parameters, could make them uniforms and pass them into the fragment shader
    float fog_maxdist = fogRange.y;
    float fog_mindist = fogRange.x;
    
    // Calculate fog
    float dist = length(camPosition.xyz - transformedFragPos.xyz);
    float fog_factor = (fog_maxdist - dist) /
                      (fog_maxdist - fog_mindist);
    fog_factor = clamp(fog_factor, 0.0, 1.0);
    
    vec4 outputColor = mix(fogColor, inColor, fog_factor);
    return outputColor;
}

float ShadowCalculation(uint lightIndex, vec4 fragPosLightSpace) {
    // perform perspective divide
    vec3 projCoords = fragPosLightSpace.xyz / fragPosLightSpace.w;
    // transform to [0,1] range
    projCoords = projCoords * 0.5 + 0.5;
    // get closest depth value from light's perspective (using [0,1] range fragPosLight as coords)
    float closestDepth = texture(shadowMap, vec3(projCoords.xy, lightIndex)).r; 
    // get depth of current fragment from light's perspective
    float currentDepth = projCoords.z;
    // check whether current frag pos is in shadow
    float shadow = currentDepth > closestDepth  ? 1.0 : 0.0;

    return shadow;
}

vec3 calculateAmbientLight() {
    // ambient light
    float ambientStrength = globalLightColor.a;
    vec3 ambientLightColor = globalLightColor.rgb;

    vec3 ambient = ambientStrength * ambientLightColor;

    return ambient;
}

vec3 calculatePointLight(uint lightIndex, vec3 normal, vec3 fragPos, vec4 fragPosLightSpace, vec3 lightPosition, vec3 lightColor) {
    float constant = 1.0;
    float linear = 0.09;
    float quadratic = 0.032;

    // diffuse lighting
    vec3 norm = normalize(normal);
    vec3 lightDir = normalize(lightPosition - fragPos);  
    float diff = max(dot(norm, lightDir), 0.0);
    vec3 diffuse = diff * lightColor;
    
    // specular lighting
    float specularStrength = 0.5;
    vec3 viewDir = normalize(camPosition.xyz - fragPos);
    vec3 reflectDir = reflect(-lightDir, norm);
    int shininess = 32;
    float spec = pow(max(dot(viewDir, reflectDir), 0.0), shininess);
    vec3 specular = specularStrength * spec * lightColor;  

    // attenuation
    float distance    = length(lightPosition - fragPos);
    float attenuation = 1.0 / (constant + linear * distance + 
    		    quadratic * (distance * distance)); 

    diffuse  *= attenuation;
    specular *= attenuation;   

    // apply light model
    //vec3 finalColor = (diffuse + specular) * surfaceColor;
    
    float shadow = ShadowCalculation(lightIndex, fragPosLightSpace);       

    return (1.0 - shadow) * (diffuse + specular);
}


vec3 calculateSpotLight(uint lightIndex, vec3 normal, vec3 fragPos, vec4 fragPosLightSpace, vec3 lightPosition, vec3 lightColor, vec3 lightDirection, float innerCutoff, float outerCutoff) {    
    // LightDir: the vector pointing from the fragment to the light source.    
    vec3 lightDir = normalize(lightPosition - fragPos);
    // SpotDir: the direction the spotlight is aiming at.
    vec3 spotDir = normalize(-lightDirection);

    float theta     = dot(lightDir, spotDir);
    float epsilon   = innerCutoff - outerCutoff;
    float intensity = clamp((theta - outerCutoff) / epsilon, 0.0, 1.0); 

    return calculatePointLight(lightIndex, normal, fragPos, fragPosLightSpace, lightPosition, lightColor) * intensity;
}

vec3 calcLight(uint lightIndex, vec3 viewPos, vec3 lightPos, vec3 lightDirection, vec3 lightColor, vec3 fragWorldPos, vec4 fragPosLightSpace, vec3 fragWorldNormal) {           
    vec3 normal = normalize(fragWorldNormal);
    // diffuse
    vec3 lightDir = normalize(lightPos - fragWorldPos);
    float theta = dot(lightDir, normalize(-lightDirection));
    if (theta > 0.96) {
        return vec3(0.0);
    }
    float diff = max(dot(lightDir, normal), 0.0);
    vec3 diffuse = diff * lightColor;
    // specular
    vec3 viewDir = normalize(viewPos - fragWorldPos);
    float spec = 0.1;
    vec3 halfwayDir = normalize(lightDir + viewDir);  
    spec = pow(max(dot(normal, halfwayDir), 0.0), 64.0);
    vec3 specular = spec * lightColor;    
    // calculate shadow
    float shadow = ShadowCalculation(lightIndex, fragPosLightSpace);       
    vec3 lighting = (1.0 - shadow) * (diffuse + specular);    
    return lighting;
}

vec4 finalColor(vec4 surfaceColor, vec3 fragWorldPos, vec3 fragWorldNormal) {

    //vec3 litColor = floodLight * surfaceColor.rgb;
    //vec3 toneMappedColor = toneMapping(litColor, 1.2, 1.0);
    vec3 ambientLightColor = calculateAmbientLight();

    vec3 spotLightColor;
    uint lightCount = uint(lightCounts.x);
    for (uint i = 0; i < lightCount; i++) {
        LightInfo curLight = lights[i];
        vec3 lightPosition = vec3(curLight.position);
        vec3 lightColor = vec3(curLight.color);
        vec3 lightDirection = vec3(curLight.direction);
        vec4 fragPosLightSpace = curLight.projView * vec4(fragWorldPos, 1.0);
        //spotLightColor += calculateSpotLight(i, fragWorldNormal, fragWorldPos, fragPosLightSpace, lightPosition, lightColor, lightDirection, 1.98,  0.96);
        spotLightColor += calcLight(i, camPosition.xyz, lightPosition, lightDirection, lightColor, fragWorldPos, fragPosLightSpace, fragWorldNormal);
    }
    
    // FLASHLIGHT HERE
    //vec3 spotLightColor = calculateSpotLight(fragWorldNormal, fragWorldPos, camPosition, light_color, camDirection, 0.98,  0.96);
    //vec3 litColor = (ambientLightColor + pointLightColor + spotLightColor) * surfaceColor.rgb;

    vec3 litColor = (ambientLightColor + spotLightColor) * surfaceColor.rgb;

    vec4 foggedColor = applyFog(fragWorldPos, vec4(litColor, surfaceColor.a));
    
    return foggedColor;
}
`
