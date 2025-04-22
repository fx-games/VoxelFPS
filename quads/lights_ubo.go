package quads

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads/glhf"
)

type LightInfo struct {
    *core.PerspectiveTransform
    Color mgl32.Vec4
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
       vec4 direction;
       mat4 projView;
   };

   // Uniform block named Lights, follows std140 alignment rules
   layout (std140) uniform Lights {
        vec4 lightCounts;
        LightInfo lights [64];
   };
`
const floatsPerLightStruct = 4 + 4 + 4 + 16 // vec4 + vec4 + vec4 + mat4

func NewLightsUBO() *LightsUBO {
    floatsInArray := (64 * floatsPerLightStruct) + 4 // 4 for the lightCount
    totalSizeInBytes := floatsInArray * 4
    ubo := &LightsUBO{
        UniformBufferObject: glhf.NewUBO(totalSizeInBytes),
        rawBuffer:           make([]glhf.GlFloat, floatsInArray),
    }

    return ubo
}

func (lu *LightsUBO) SetLights(lights []LightInfo) {
    lu.lightCount = len(lights)

    floatsInArray := (lu.lightCount * floatsPerLightStruct) + 4 // 4 for the lightCount
    totalSizeInBytes := floatsInArray * 4                       // bytes per float32, one element of the vec4
    lu.rawBuffer[0] = glhf.GlFloat(float32(lu.lightCount))
    offset := 4
    for i := 0; i < lu.lightCount; i++ {
        curLight := lights[i]
        setLightAt(lu.rawBuffer, offset, i, floatsPerLightStruct, curLight.GetPosition().Vec4(1.0), curLight.Color, curLight.GetForward().Vec4(0), curLight.GetProjectionViewMatrix())
    }
    lu.Begin()
    gl.BufferSubData(gl.UNIFORM_BUFFER, 0, totalSizeInBytes, gl.Ptr(lu.rawBuffer))
    lu.End()
}

func setLightAt(lights []glhf.GlFloat, offset int, index int, elementsPerStruct int, pos, col, dir mgl32.Vec4, projView mgl32.Mat4) {
    lights[index*elementsPerStruct+0+offset] = glhf.GlFloat(pos.X()) // position
    lights[index*elementsPerStruct+1+offset] = glhf.GlFloat(pos.Y())
    lights[index*elementsPerStruct+2+offset] = glhf.GlFloat(pos.Z())
    lights[index*elementsPerStruct+3+offset] = glhf.GlFloat(pos.W())
    lights[index*elementsPerStruct+4+offset] = glhf.GlFloat(col.X()) // color
    lights[index*elementsPerStruct+5+offset] = glhf.GlFloat(col.Y())
    lights[index*elementsPerStruct+6+offset] = glhf.GlFloat(col.Z())
    lights[index*elementsPerStruct+7+offset] = glhf.GlFloat(col.W())
    lights[index*elementsPerStruct+8+offset] = glhf.GlFloat(dir.X()) // direction
    lights[index*elementsPerStruct+9+offset] = glhf.GlFloat(dir.Y())
    lights[index*elementsPerStruct+10+offset] = glhf.GlFloat(dir.Z())
    lights[index*elementsPerStruct+11+offset] = glhf.GlFloat(dir.W())
    lights[index*elementsPerStruct+12+offset] = glhf.GlFloat(projView[0]) // projView
    lights[index*elementsPerStruct+13+offset] = glhf.GlFloat(projView[1])
    lights[index*elementsPerStruct+14+offset] = glhf.GlFloat(projView[2])
    lights[index*elementsPerStruct+15+offset] = glhf.GlFloat(projView[3])
    lights[index*elementsPerStruct+16+offset] = glhf.GlFloat(projView[4])
    lights[index*elementsPerStruct+17+offset] = glhf.GlFloat(projView[5])
    lights[index*elementsPerStruct+18+offset] = glhf.GlFloat(projView[6])
    lights[index*elementsPerStruct+19+offset] = glhf.GlFloat(projView[7])
    lights[index*elementsPerStruct+20+offset] = glhf.GlFloat(projView[8])
    lights[index*elementsPerStruct+21+offset] = glhf.GlFloat(projView[9])
    lights[index*elementsPerStruct+22+offset] = glhf.GlFloat(projView[10])
    lights[index*elementsPerStruct+23+offset] = glhf.GlFloat(projView[11])
    lights[index*elementsPerStruct+24+offset] = glhf.GlFloat(projView[12])
    lights[index*elementsPerStruct+25+offset] = glhf.GlFloat(projView[13])
    lights[index*elementsPerStruct+26+offset] = glhf.GlFloat(projView[14])
    lights[index*elementsPerStruct+27+offset] = glhf.GlFloat(projView[15])
}
