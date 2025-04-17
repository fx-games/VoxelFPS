package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/quads/glhf"
)

type LightInfo struct {
    Position mgl32.Vec4
    Color    mgl32.Vec4
}

type LightsUBO struct {
    *glhf.UniformBufferObject
    lightCount int
    lights     []LightInfo
    rawBuffer  []glhf.GlFloat
}

const lightsGLSLBlock = `
   struct LightInfo
   {
       vec4 position;
       vec4 color;
   };

   // Uniform block named Lights, follows std140 alignment rules
   layout (std140) uniform Lights {
        vec4 lightCounts;
        LightInfo lights [64];
   };
`

func NewLightsUBO() *LightsUBO {
    floatsPerStruct := 2 * 4                    // vec4 + vec4
    floatsInArray := (64 * floatsPerStruct) + 4 // 4 for the lightCount
    totalSizeInBytes := floatsInArray * 4
    ubo := &LightsUBO{
        UniformBufferObject: glhf.NewUBO(totalSizeInBytes),
        rawBuffer:           make([]glhf.GlFloat, floatsInArray),
    }

    return ubo
}

func (lu *LightsUBO) SetLights(lights []LightInfo) {
    lu.lightCount = len(lights)
    floatsPerStruct := 2 * 4                               // vec4 + vec4
    floatsInArray := (lu.lightCount * floatsPerStruct) + 4 // 4 for the lightCount
    totalSizeInBytes := floatsInArray * 4                  // bytes per float32, one element of the vec4
    lu.rawBuffer[0] = glhf.GlFloat(float32(lu.lightCount))
    offset := 4
    for i := 0; i < lu.lightCount; i++ {
        curLight := lights[i]
        setLightAt(lu.rawBuffer, offset, i, floatsPerStruct, curLight.Position, curLight.Color)
    }
    lu.Begin()
    lu.SetData(0, totalSizeInBytes, gl.Ptr(lu.rawBuffer))
    lu.End()
}

func setLightAt(lights []glhf.GlFloat, offset int, index int, elementsPerStruct int, pos, col mgl32.Vec4) {
    lights[index*elementsPerStruct+0+offset] = glhf.GlFloat(pos.X()) // position
    lights[index*elementsPerStruct+1+offset] = glhf.GlFloat(pos.Y())
    lights[index*elementsPerStruct+2+offset] = glhf.GlFloat(pos.Z())
    lights[index*elementsPerStruct+3+offset] = glhf.GlFloat(pos.W())
    lights[index*elementsPerStruct+4+offset] = glhf.GlFloat(col.X()) // color
    lights[index*elementsPerStruct+5+offset] = glhf.GlFloat(col.Y())
    lights[index*elementsPerStruct+6+offset] = glhf.GlFloat(col.Z())
    lights[index*elementsPerStruct+7+offset] = glhf.GlFloat(col.W())
}
