package quads

import (
	"fmt"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
)

type LightInfo struct {
	*core.PerspectiveTransform
	Color mgl32.Vec4

	// What the depth map in the shadow buffer was rendered from. A light only has
	// to redraw it when its own frustum moved or the casting scene changed.
	shadowProjView mgl32.Mat4
	shadowScene    uint64
	shadowRendered bool
}

// IsOff reports a light that contributes nothing, so it needs neither a shadow
// map nor a slot in the shader's light loop.
func (l LightInfo) IsOff() bool {
	return (l.Color.X() <= 0 && l.Color.Y() <= 0 && l.Color.Z() <= 0) || l.Color.W() <= 0
}

// packed lays a light out the way the shader reads it. The three spare components
// carry what used to be hardcoded in the shader for every light alike:
//
//	position.w  reach, the light's own far plane
//	color.w     intensity
//	direction.w cosine of the half FOV, the outer edge of the cone
//
// Reach and cone come from the light's shadow frustum, so the region it lights and
// the region it has depth information for cannot drift apart.
func (l LightInfo) packed() (pos, col, dir mgl32.Vec4, projView mgl32.Mat4) {
	return l.GetPosition().Vec4(l.GetFarPlaneDist()),
		l.Color,
		l.GetForward().Vec4(l.ConeCosine()),
		l.GetProjectionViewMatrix()
}

// NeedsShadowUpdate reports whether the cached depth map for this light is stale.
func (l *LightInfo) NeedsShadowUpdate(sceneSignature uint64) bool {
	return !l.shadowRendered ||
		l.shadowScene != sceneSignature ||
		l.shadowProjView != l.GetProjectionViewMatrix()
}

func (l *LightInfo) MarkShadowRendered(sceneSignature uint64) {
	l.shadowRendered = true
	l.shadowScene = sceneSignature
	l.shadowProjView = l.GetProjectionViewMatrix()
}

// LightFromMap turns the map file's description of a light into one the renderer can
// use. The map speaks in angles, a colour and a reach; a LightInfo is a projection
// frustum plus a packed vec4, and the two are kept apart on purpose - core has no
// business importing GL, and a map format should not have to know how a shadow
// frustum is built.
func LightFromMap(light core.MapLight, shadowResolution int) LightInfo {
	angle := light.Angle
	if angle <= 0 {
		angle = 45
	}
	reach := light.Reach
	if reach <= 0 {
		reach = 15
	}
	intensity := light.Intensity
	if intensity <= 0 {
		intensity = 1
	}
	// The cone angle authored in the map is the full angle, the way a person thinks
	// about a lamp; the frustum wants the same number as its FOV, so it passes
	// straight through. Reach is the far plane, which is what keeps the lit region
	// and the region with depth information the same region.
	return LightInfo{
		PerspectiveTransform: core.NewPerspectiveTransformForSpotlight(
			light.Position,
			mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, light.Forward()),
			angle, reach, shadowResolution,
		),
		Color: light.Color.Vec4(intensity),
	}
}

type LightsUBO struct {
	*glhf.UniformBufferObject
	lightCount int
	lights     []LightInfo
	rawBuffer  []glhf.GlFloat
}

// MaxLights caps the shadow map array, the UBO and the shader's light loop. Each
// light costs a layer of the depth texture (512^2 x 4 bytes) whether it is used
// or not, so this is not free headroom.
const MaxLights = 16

var lightsGLSLBlock = fmt.Sprintf(`
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
        LightInfo lights [%d];
   };
`, MaxLights)

const floatsPerLightStruct = 4 + 4 + 4 + 16 // vec4 + vec4 + vec4 + mat4

func NewLightsUBO() *LightsUBO {
	floatsInArray := (MaxLights * floatsPerLightStruct) + 4 // 4 for the lightCount
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
		setLightAt(lu.rawBuffer, offset, i, floatsPerLightStruct, curLight)
	}
	lu.Begin()
	gl.BufferSubData(gl.UNIFORM_BUFFER, 0, totalSizeInBytes, gl.Ptr(lu.rawBuffer))
	lu.End()
}

func (lu *LightsUBO) updateLightAt(index int) {
	light := lu.lights[index]
	setLightAt(lu.rawBuffer, 4, index, floatsPerLightStruct, light)
	rawBufferOffset := (index * floatsPerLightStruct) + 4
	byteOffset := (4 + (index * floatsPerLightStruct)) * 4

	lu.Begin()
	gl.BufferSubData(gl.UNIFORM_BUFFER, byteOffset, floatsPerLightStruct*4, gl.Ptr(lu.rawBuffer[rawBufferOffset:]))
	lu.End()
}

func setLightAt(lightBuffer []glhf.GlFloat, offset int, index int, elementsPerStruct int, light LightInfo) {
	pos, col, dir, projView := light.packed()
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
