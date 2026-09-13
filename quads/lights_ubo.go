package quads

import (
	"fmt"
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
)

type LightInfo struct {
	*core.PerspectiveTransform
	Color mgl32.Vec4

	// Omni makes this a point light: it lights in every direction and casts no
	// shadow, because a single perspective frustum has no depth information for
	// the sphere around it. The shader recognises it by a cone cosine of -1 (see
	// packed) and skips both the cone test and the shadow lookup, and the shadow
	// pass skips it entirely. For a small moving light - a projectile, a spark -
	// that is the whole feature; a cube shadow map would be six passes a frame.
	Omni bool

	// ShadowLayer is this light's layer in the shadow atlas, or noShadowLayer when
	// it casts none. It used to be the light's index in the list, which tied the
	// number of lights in the world to the number of depth textures the atlas can
	// hold. assignShadowLayers hands them out, packed lays this into params.x, and
	// the shader reads it as both "does this light shadow" and "which layer".
	ShadowLayer int32

	// What the depth map in the shadow buffer was rendered from. A light only has
	// to redraw it when its own frustum moved or the casting scene changed.
	shadowProjView mgl32.Mat4
	shadowScene    uint64
	shadowRendered bool
}

// noShadowLayer marks a light with no layer in the atlas: an omni light, which has
// no single frustum to render depth from, or a spotlight that arrived after the
// last layer was handed out. It still lights, it just does not shadow.
const noShadowLayer = int32(-1)

// CastsShadow reports whether this light wants a layer of the atlas. Deliberately
// not conditioned on IsOff: the player light starts switched off and would then
// never hold a layer, so switching it on would light without shadowing until the
// next time the list was rebuilt. An off light is skipped by the shadow pass
// itself, which costs nothing but the layer sitting idle.
func (l LightInfo) CastsShadow() bool {
	return !l.Omni
}

// assignShadowLayers hands out the atlas layers, in list order, to the lights that
// cast. Anything past the last layer lights without shadowing rather than sampling
// a layer that belongs to someone else. A light that changes layer drops its cached
// depth map with it: the layer it now points at holds another light's scene.
func assignShadowLayers(lights []LightInfo) {
	next := int32(0)
	for i := range lights {
		layer := noShadowLayer
		if lights[i].CastsShadow() && next < MaxShadowLayers {
			layer = next
			next++
		}
		if lights[i].ShadowLayer != layer {
			lights[i].ShadowLayer = layer
			lights[i].shadowRendered = false
		}
	}
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
//	params.x    shadow atlas layer, or -1 for a light that casts no shadow
//
// Reach and cone come from the light's shadow frustum, so the region it lights and
// the region it has depth information for cannot drift apart.
func (l LightInfo) packed() (pos, col, dir, params mgl32.Vec4, projView mgl32.Mat4) {
	coneCosine := l.ConeCosine()
	if l.Omni {
		coneCosine = omniConeCosine
	}
	return l.GetPosition().Vec4(l.GetFarPlaneDist()),
		l.Color,
		l.GetForward().Vec4(coneCosine),
		mgl32.Vec4{float32(l.ShadowLayer), 0, 0, 0},
		l.GetProjectionViewMatrix()
}

// omniConeCosine is the sentinel an omni light packs into direction.w. A real cone
// cosine cannot reach -1 (that would be a 360 degree spotlight), so the shader can
// read it as "no cone, no shadow" without another vec4.
const omniConeCosine = float32(-1)

// NeedsShadowUpdate reports whether the cached depth map for this light is stale.
// An omni light never has one.
func (l *LightInfo) NeedsShadowUpdate(sceneSignature uint64) bool {
	return !l.Omni && l.ShadowLayer >= 0 && (!l.shadowRendered ||
		l.shadowScene != sceneSignature ||
		l.shadowProjView != l.GetProjectionViewMatrix())
}

func (l *LightInfo) MarkShadowRendered(sceneSignature uint64) {
	l.shadowRendered = true
	l.shadowScene = sceneSignature
	l.shadowProjView = l.GetProjectionViewMatrix()
}

func hashPos(pos mgl32.Vec3) uint64 {
	h := uint64(14695981039346656037)
	for _, f := range [3]float32{pos.X(), pos.Y(), pos.Z()} {
		h = (h ^ uint64(math.Float32bits(f))) * 1099511628211
	}
	return h
}

// shadowSceneSignatureFor hashes what one light's depth map depends on: the world
// mesh generation plus the position of every caster within reach of this light.
// A caster beyond reach - the far plane of the light's own shadow frustum - is
// clipped by that frustum and cannot appear in the depth map regardless, so
// filtering by distance is exact, not a heuristic.
//
// This is what keeps a mover on one side of the map from invalidating every
// light's cache: hashing every caster in the world into one signature shared by
// every light meant a single patrolling NPC left no light's shadow map ever
// cached, anywhere. Per-object hashes are summed rather than chained, because the
// order casters are visited in changes from frame to frame.
func shadowSceneSignatureFor(lightPos mgl32.Vec3, reach float32, meshGeneration uint64, casterPositions []mgl32.Vec3) uint64 {
	signature := meshGeneration * 0x9E3779B97F4A7C15
	for _, pos := range casterPositions {
		if pos.Sub(lightPos).Len() <= reach {
			signature += hashPos(pos)
		}
	}
	return signature
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

// SunLightFrom builds the level's star as a light: a spotlight standing off in
// space, aimed at the ship, bracketing it with its own near and far planes. It
// is placed where a sky that has not turned yet puts it - see SunStandpoint for
// where it goes as the sky turns.
//
// Power is compensated for the standoff here rather than in the map, because the
// falloff is inverse square referenced to LIGHT_REF_DIST: a light a hundred and
// fifty units away needs a four-figure intensity to land as brightly as a lamp
// on the wall, and a four-figure number in a map file reads as a typo.
func SunLightFrom(sun *core.Sun, shadowResolution int) LightInfo {
	towards := sun.Towards.Normalize()
	position := sun.At.Add(towards.Mul(sun.Distance))
	transform := core.NewPerspectiveTransformForSpotlight(
		position,
		mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, towards.Mul(-1)),
		sun.Angle, sun.Distance+sun.Depth/2, shadowResolution,
	)
	transform.SetNearPlaneDist(max(spotlightMinNearPlane, sun.Distance-sun.Depth/2))
	const lightRefDist = 2.5 // LIGHT_REF_DIST in the shader
	standoff := sun.Distance / lightRefDist
	return LightInfo{
		PerspectiveTransform: transform,
		Color:                sun.Color.Vec4(sun.Power * (1 + standoff*standoff)),
	}
}

// spotlightMinNearPlane is core.spotlightNearPlane, which a sun is only allowed
// to push further out.
const spotlightMinNearPlane = float32(0.5)

// SunStandpoint is where the sun stands and which way it points once the sky has
// turned by spin radians about axis. The sky is turned by rotating it under a
// fixed camera (GlApplication.SetSkyboxSpin), so a star drawn at Towards in the
// cubemap's own frame is seen out there at the same rotation applied to it -
// which is the only reason the light and the picture stay together.
func SunStandpoint(sun *core.Sun, spin float32, axis mgl32.Vec3) (position, forward mgl32.Vec3) {
	if axis.Len() == 0 {
		axis = mgl32.Vec3{0, 1, 0}
	}
	turned := mgl32.HomogRotate3D(spin, axis.Normalize()).Mul4x1(sun.Towards.Normalize().Vec4(0)).Vec3()
	return sun.At.Add(turned.Mul(sun.Distance)), turned.Mul(-1)
}

type LightsUBO struct {
	*glhf.UniformBufferObject
	lightCount int
	lights     []LightInfo
	rawBuffer  []glhf.GlFloat
	// dirty is set when rawBuffer holds a moved light the GPU has not been given
	// yet. See Flush.
	dirty bool

	// Slot 0 is the player's own light and nothing else. What kind it is, and what
	// it hangs off - the camera - lives here, because this is what rebuilds the
	// list and has to put it back at the front every time.
	playerParent     core.Transformer
	shadowResolution int
	playerKind       PlayerLightKind
	lastPlayerKind   PlayerLightKind

	// One light follows a game object: whatever the player last fired. See
	// AttachLightTo.
	trackedIndex  int
	trackedObject *GameObject
}

// noTrackedLight is trackedIndex when nothing is following an object.
const noTrackedLight = -1

// MaxLights caps the UBO and the shader's light loop. One light costs 128 bytes of
// the block and one iteration per lit fragment, which is why this is not simply
// enormous - but it is no longer tied to the shadow atlas, so a room can hold more
// lights than the atlas has layers.
const MaxLights = 32

// MaxShadowLayers caps the shadow atlas, which is the expensive half: every layer
// is a 512^2 depth texture whether it is used or not, and a layer that goes stale
// costs a full depth pass over the scene. Lights past the last layer still light.
const MaxShadowLayers = 16

var lightsGLSLBlock = fmt.Sprintf(`
   struct LightInfo
   {
       vec4 position;
       vec4 color;
       vec4 direction;
       vec4 params;
       mat4 projView;
   };

   // Uniform block named Lights, follows std140 alignment rules
   layout (std140) uniform Lights {
        vec4 lightCounts;
        LightInfo lights [%d];
   };
`, MaxLights)

const floatsPerLightStruct = 4 + 4 + 4 + 4 + 16 // position, color, direction, params, projView

func NewLightsUBO() *LightsUBO {
	floatsInArray := (MaxLights * floatsPerLightStruct) + 4 // 4 for the lightCount
	totalSizeInBytes := floatsInArray * 4
	ubo := &LightsUBO{
		UniformBufferObject: glhf.NewUBO(totalSizeInBytes),
		rawBuffer:           make([]glhf.GlFloat, floatsInArray),
		trackedIndex:        noTrackedLight,
	}

	return ubo
}

// setAll replaces the whole light list and uploads it. Unexported on purpose: it is
// the one place shadow layers are handed out, and the one place slot 0 could be
// lost, so every caller goes through SetStaticLights or AddLights - both of which
// keep the player light where the frame loop expects it. Anything past MaxLights is
// dropped; the count is returned so the caller can say so out loud.
func (lu *LightsUBO) setAll(lights []LightInfo) (dropped int) {
	if len(lights) > MaxLights {
		dropped = len(lights) - MaxLights
		lights = lights[:MaxLights]
	}
	assignShadowLayers(lights)
	lu.lightCount = len(lights)
	lu.lights = lights
	if lu.trackedIndex >= lu.lightCount { // the tracked light did not survive the cap
		lu.trackedIndex = noTrackedLight
		lu.trackedObject = nil
	}
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
	lu.dirty = false
	return dropped
}

// SetStaticLights replaces every light in the scene except the player's, which
// stays in slot 0. A map that brings its own lighting calls this on load, so
// switching maps does not accumulate the lights of every map loaded before it.
// Returns how many lights did not fit.
func (lu *LightsUBO) SetStaticLights(infos ...LightInfo) (dropped int) {
	// The list is rebuilt from scratch, so whatever slot the tracked light had is
	// gone with it.
	lu.trackedIndex = noTrackedLight
	lu.trackedObject = nil
	return lu.setAll(append([]LightInfo{lu.playerLight()}, infos...))
}

// AddLights appends to the scene's lights, keeping the ones already there.
func (lu *LightsUBO) AddLights(infos ...LightInfo) (dropped int) {
	return lu.setAll(append(lu.lights, infos...))
}

// AddMovableLight appends one light and hands back the slot it landed in, or -1
// if the list was full. The slot is what MoveLight takes: a light that is moved
// by something the map knows nothing about - the sun, which stands where the sky
// says it does - is added once here and moved from then on.
func (lu *LightsUBO) AddMovableLight(info LightInfo) int {
	slot := len(lu.lights)
	if lu.AddLights(info) > 0 {
		return -1
	}
	return slot
}

// MoveLight stands a light somewhere else, points it somewhere else, and marks
// its slot for the next Flush - the same one-slot update the player's light gets
// every frame, rather than a rebuild of the whole list, which would hand out the
// shadow layers again and throw away every cached depth map with them.
func (lu *LightsUBO) MoveLight(index int, position, forward mgl32.Vec3) {
	if index < 0 || index >= lu.lightCount {
		return
	}
	lu.lights[index].SetPosition(position)
	lu.lights[index].SetForward(forward)
	lu.updateLightAt(index)
}

// MuteLights writes a light count of zero into the block the shaders read and
// hands back the call that puts the real one back. It is for a pass that draws
// something which is not standing in the level - a model previewed on its own,
// which has to look the same whatever the map around it is lit by. The lights
// themselves are untouched, only the count the shader loops to, so restoring is
// a word rather than a re-upload of the whole block.
func (lu *LightsUBO) MuteLights() (restore func()) {
	lu.uploadLightCount(0)
	return func() { lu.uploadLightCount(lu.lightCount) }
}

func (lu *LightsUBO) uploadLightCount(count int) {
	value := glhf.GlFloat(float32(count))
	lu.Begin()
	gl.BufferSubData(gl.UNIFORM_BUFFER, 0, 4, gl.Ptr(&value))
	lu.End()
}

// Lights is the current list, for the shadow pass to walk. The pass writes to the
// cache fields on each light, so this hands out the slice itself.
func (lu *LightsUBO) Lights() []LightInfo {
	return lu.lights
}

// updateLightAt packs one light into rawBuffer and leaves the upload to Flush.
func (lu *LightsUBO) updateLightAt(index int) {
	setLightAt(lu.rawBuffer, 4, index, floatsPerLightStruct, lu.lights[index])
	lu.dirty = true
}

// Flush gives the GPU every light moved since the last one, once a frame, before
// anything is drawn with them.
//
// It used to be a BufferSubData per moved light, straight away. On the ark the sun
// moves every frame and so does the player's light, and the second of those writes
// landed on a buffer the GPU was still reading for the frame before: the driver
// held the render thread in that call until it was done, 2.1 ms a frame. The
// whole block is 4 KB, so it is handed over whole with BufferData, which gives the
// buffer new storage and lets the frame still in flight keep the old.
func (lu *LightsUBO) Flush() {
	if !lu.dirty {
		return
	}
	lu.Begin()
	gl.BufferData(gl.UNIFORM_BUFFER, len(lu.rawBuffer)*4, gl.Ptr(lu.rawBuffer), gl.STREAM_DRAW)
	lu.End()
	lu.dirty = false
}

func setLightAt(lightBuffer []glhf.GlFloat, offset int, index int, elementsPerStruct int, light LightInfo) {
	pos, col, dir, params, projView := light.packed()
	// The order here is the order of the fields in lightsGLSLBlock. std140 gives a
	// vec4 and a mat4 the same 16 byte alignment, so the struct is tightly packed
	// and one running index covers all of it.
	at := index*elementsPerStruct + offset
	for _, vec := range [4]mgl32.Vec4{pos, col, dir, params} {
		for i := 0; i < 4; i++ {
			lightBuffer[at] = glhf.GlFloat(vec[i])
			at++
		}
	}
	for i := 0; i < 16; i++ {
		lightBuffer[at] = glhf.GlFloat(projView[i])
		at++
	}
}

// PlayerLightKind is what the player is carrying. Slot 0 holds exactly one of these
// at a time - a player can never be lit by both at once. Both cast shadows: they are
// the same mechanism with different cones, a tight beam against a wide pool of warm
// light.
type PlayerLightKind int

const (
	PlayerLightOff PlayerLightKind = iota
	PlayerLightFlashlight
	PlayerLightTorch
)

func (k PlayerLightKind) String() string {
	switch k {
	case PlayerLightFlashlight:
		return "flashlight"
	case PlayerLightTorch:
		return "torch"
	default:
		return "off"
	}
}

// The alpha is intensity, not opacity, and it is large because the falloff is
// inverse square: it is the brightness at LIGHT_REF_DIST (2.5 units), not a
// multiplier on the final colour. Reach and cone angle come from the light's shadow
// frustum, which is why they are in newPlayerLight rather than here.
var (
	flashlightColor    = mgl32.Vec4{0.7, 0.7, 0.7, 8.0}
	flashlightOffColor = mgl32.Vec4{0, 0, 0, 1}
	// Warm, and weaker than the flashlight because it spreads over a far wider cone:
	// the same intensity would wash the room out.
	torchLightColor = mgl32.Vec4{1.0, 0.72, 0.42, 6.0}
)

const (
	flashlightFOV   = float32(45)
	flashlightReach = float32(15)
	// A held torch is a short, wide pool of warm light rather than a beam - but it is
	// still a spotlight, because that is what can cast a shadow here. A shadow map is
	// one perspective frustum, and a true point light would need six of them (a cube
	// map, six depth passes a frame) to shadow the sphere it lights.
	//
	// 120 degrees is the practical ceiling for one frustum: the projection stretches
	// towards the rim, so a shadow texel out there covers tan(60) = 1.7 world units
	// per unit of distance at 512^2, and pushing the cone wider spends quality on the
	// part of the image the player is least likely to be looking at. What it costs is
	// the light behind the player: a real torch lights the wall at your back and this
	// one does not.
	//
	// ponytail: one forward cone. Cube shadow maps if the light behind the player
	// ever matters more than five extra depth passes a frame do.
	torchLightFOV   = float32(120)
	torchLightReach = float32(9)
)

// AttachPlayerLightTo hangs the player's light off the camera and puts it in slot 0.
// Called once during setup, before any map is loaded: everything that rebuilds the
// light list puts this light back at the front, and the frame loop re-uploads slot 0
// by index every frame, which is what makes it follow the camera.
func (lu *LightsUBO) AttachPlayerLightTo(parent core.Transformer, shadowResolution int, kind PlayerLightKind) {
	lu.playerParent = parent
	lu.shadowResolution = shadowResolution
	lu.playerKind = kind
	lu.lastPlayerKind = PlayerLightFlashlight
	lu.setAll(append([]LightInfo{lu.newPlayerLight(kind)}, lu.lights...))
}

// playerLight is whatever is in slot 0, for a rebuild to put back. Before
// AttachPlayerLightTo has run there is nothing to put back, so it builds an unlit
// one - a list with no slot 0 would crash the frame loop, which updates it by index.
func (lu *LightsUBO) playerLight() LightInfo {
	if len(lu.lights) > 0 {
		return lu.lights[0]
	}
	return lu.newPlayerLight(PlayerLightOff)
}

func (lu *LightsUBO) newPlayerLight(kind PlayerLightKind) LightInfo {
	fov, reach := flashlightFOV, flashlightReach
	if kind == PlayerLightTorch {
		fov, reach = torchLightFOV, torchLightReach
	}
	var transform *core.PerspectiveTransform
	if lu.playerParent != nil {
		transform = core.NewPerspectiveTransformForSpotlightFromParent(lu.playerParent, fov, reach, lu.shadowResolution)
	} else {
		transform = core.NewPerspectiveTransformForSpotlight(mgl32.Vec3{}, mgl32.QuatIdent(), fov, reach, lu.shadowResolution)
	}
	light := LightInfo{PerspectiveTransform: transform, Color: flashlightOffColor}
	switch kind {
	case PlayerLightFlashlight:
		light.Color = flashlightColor
	case PlayerLightTorch:
		light.Color = torchLightColor
	}
	return light
}

// SetPlayerLight swaps what the player is carrying. It goes through the full rebuild
// rather than writing slot 0 in place: the light is rebuilt around a new frustum, and
// switching it off has to hand its atlas layer back to the pool.
func (lu *LightsUBO) SetPlayerLight(kind PlayerLightKind) PlayerLightKind {
	lu.playerKind = kind
	if kind != PlayerLightOff {
		lu.lastPlayerKind = kind
	}
	lights := append([]LightInfo{lu.newPlayerLight(kind)}, lu.lights[min(1, len(lu.lights)):]...)
	lu.setAll(lights)
	return kind
}

// TogglePlayerLight switches the player's light off, or back on as whatever it was
// last time. The kind itself is chosen with CyclePlayerLight.
func (lu *LightsUBO) TogglePlayerLight() PlayerLightKind {
	if lu.playerKind == PlayerLightOff {
		return lu.SetPlayerLight(lu.lastPlayerKind)
	}
	return lu.SetPlayerLight(PlayerLightOff)
}

// CyclePlayerLight steps off -> flashlight -> torch -> off. One slot, so picking one
// puts the other away.
func (lu *LightsUBO) CyclePlayerLight() PlayerLightKind {
	switch lu.playerKind {
	case PlayerLightOff:
		return lu.SetPlayerLight(PlayerLightFlashlight)
	case PlayerLightFlashlight:
		return lu.SetPlayerLight(PlayerLightTorch)
	default:
		return lu.SetPlayerLight(PlayerLightOff)
	}
}

func (lu *LightsUBO) PlayerLight() PlayerLightKind {
	return lu.playerKind
}

// UpdatePlayerLight re-uploads slot 0, which is what makes the light follow the
// camera it hangs off. Called every frame.
func (lu *LightsUBO) UpdatePlayerLight() {
	if lu.lightCount == 0 {
		return
	}
	lu.updateLightAt(0)
}

// AttachLightTo parents a light to a game object: it moves with the object and
// switches itself off the frame the object dies. Reach is how far it carries and
// intensity its brightness at LIGHT_REF_DIST, the same numbers a map light uses.
// Omni, so it costs no shadow atlas layer - a projectile lighting the room it flies
// through does not need to occlude it.
//
// ponytail: one tracked slot for the whole application, reused by every caller. The
// thing that wants it is whatever the player just fired, and two of those at once
// would need a slot pool.
func (lu *LightsUBO) AttachLightTo(obj *GameObject, color mgl32.Vec3, intensity, reach float32) {
	// The FOV is unused - an omni light has no cone - but the frustum still supplies
	// the position and the far plane, which is the light's reach.
	transform := core.NewPerspectiveTransformForSpotlightFromParent(obj.Transform(), 90, reach, lu.shadowResolution)
	transform.SetPosition(mgl32.Vec3{}) // sit on the object, not under it like the flashlight
	light := LightInfo{
		PerspectiveTransform: transform,
		Color:                color.Vec4(intensity),
		Omni:                 true,
	}
	if lu.trackedIndex < 0 {
		slot := len(lu.lights)
		if slot >= MaxLights {
			return // every slot is a map light; the projectile just does not light the room
		}
		lu.AddLights(light)
		lu.trackedIndex = slot
	} else {
		lu.lights[lu.trackedIndex] = light
		lu.updateLightAt(lu.trackedIndex)
	}
	lu.trackedObject = obj
}

// DetachLight switches the tracked light off but keeps its slot, so the next
// projectile reuses it instead of growing the list on every shot.
func (lu *LightsUBO) DetachLight() {
	if lu.trackedIndex < 0 {
		return
	}
	lu.lights[lu.trackedIndex].Color = mgl32.Vec4{}
	lu.updateLightAt(lu.trackedIndex)
	lu.trackedObject = nil
}

// UpdateTrackedLight re-uploads the tracked light's slot, which is what makes it
// follow its parent, and switches it off once that parent is dead.
func (lu *LightsUBO) UpdateTrackedLight() {
	if lu.trackedIndex < 0 || lu.trackedObject == nil {
		return
	}
	if !lu.trackedObject.IsAlive() {
		lu.DetachLight()
		return
	}
	lu.updateLightAt(lu.trackedIndex)
}

// LitCount is how many lights are switched on, for the debug overlay.
func (lu *LightsUBO) LitCount() (lit int) {
	for i := range lu.lights {
		if !lu.lights[i].IsOff() {
			lit++
		}
	}
	return lit
}

// splitMapLights sorts a map's lights into the two things they can be: a slot in
// the renderer's light list, or a seed for the level's baked voxel light. A baked
// one is a grid position and nothing else - the flood fill has one colour and no
// direction, which is the trade that makes it free per frame.
func splitMapLights(mapLights []core.MapLight, shadowResolution int) (dynamic []LightInfo, baked []voxel.Int3) {
	for _, light := range mapLights {
		if light.Baked {
			baked = append(baked, voxel.PositionToGridInt3(light.Position))
			continue
		}
		dynamic = append(dynamic, LightFromMap(light, shadowResolution))
	}
	return dynamic, baked
}
