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
    lu.lights = lights
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

func (lu *LightsUBO) updateLightAt(index int) {
    light := lu.lights[index]
    setLightAt(lu.rawBuffer, 4, index, floatsPerLightStruct, light.GetPosition().Vec4(1.0), light.Color, light.GetForward().Vec4(0), light.GetProjectionViewMatrix())
    rawBufferOffset := (index * floatsPerLightStruct) + 4
    byteOffset := (4 + (index * floatsPerLightStruct)) * 4

    lu.Begin()
    gl.BufferSubData(gl.UNIFORM_BUFFER, byteOffset, floatsPerLightStruct*4, gl.Ptr(lu.rawBuffer[rawBufferOffset:]))
    lu.End()
}

func setLightAt(lightBuffer []glhf.GlFloat, offset int, index int, elementsPerStruct int, pos, col, dir mgl32.Vec4, projView mgl32.Mat4) {
    lightBuffer[index*elementsPerStruct+0+offset] = glhf.GlFloat(pos.X()) // position
    lightBuffer[index*elementsPerStruct+1+offset] = glhf.GlFloat(pos.Y())
    lightBuffer[index*elementsPerStruct+2+offset] = glhf.GlFloat(pos.Z())
    lightBuffer[index*elementsPerStruct+3+offset] = glhf.GlFloat(pos.W())
    lightBuffer[index*elementsPerStruct+4+offset] = glhf.GlFloat(col.X()) // color
    lightBuffer[index*elementsPerStruct+5+offset] = glhf.GlFloat(col.Y())
    lightBuffer[index*elementsPerStruct+6+offset] = glhf.GlFloat(col.Z())
    lightBuffer[index*elementsPerStruct+7+offset] = glhf.GlFloat(col.W())
    lightBuffer[index*elementsPerStruct+8+offset] = glhf.GlFloat(dir.X()) // direction
    lightBuffer[index*elementsPerStruct+9+offset] = glhf.GlFloat(dir.Y())
    lightBuffer[index*elementsPerStruct+10+offset] = glhf.GlFloat(dir.Z())
    lightBuffer[index*elementsPerStruct+11+offset] = glhf.GlFloat(dir.W())
    lightBuffer[index*elementsPerStruct+12+offset] = glhf.GlFloat(projView[0]) // projView
    lightBuffer[index*elementsPerStruct+13+offset] = glhf.GlFloat(projView[1])
    lightBuffer[index*elementsPerStruct+14+offset] = glhf.GlFloat(projView[2])
    lightBuffer[index*elementsPerStruct+15+offset] = glhf.GlFloat(projView[3])
    lightBuffer[index*elementsPerStruct+16+offset] = glhf.GlFloat(projView[4])
    lightBuffer[index*elementsPerStruct+17+offset] = glhf.GlFloat(projView[5])
    lightBuffer[index*elementsPerStruct+18+offset] = glhf.GlFloat(projView[6])
    lightBuffer[index*elementsPerStruct+19+offset] = glhf.GlFloat(projView[7])
    lightBuffer[index*elementsPerStruct+20+offset] = glhf.GlFloat(projView[8])
    lightBuffer[index*elementsPerStruct+21+offset] = glhf.GlFloat(projView[9])
    lightBuffer[index*elementsPerStruct+22+offset] = glhf.GlFloat(projView[10])
    lightBuffer[index*elementsPerStruct+23+offset] = glhf.GlFloat(projView[11])
    lightBuffer[index*elementsPerStruct+24+offset] = glhf.GlFloat(projView[12])
    lightBuffer[index*elementsPerStruct+25+offset] = glhf.GlFloat(projView[13])
    lightBuffer[index*elementsPerStruct+26+offset] = glhf.GlFloat(projView[14])
    lightBuffer[index*elementsPerStruct+27+offset] = glhf.GlFloat(projView[15])
}
