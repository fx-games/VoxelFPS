package quads

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
)

func staticLight() *LightInfo {
	return &LightInfo{
		PerspectiveTransform: core.NewPerspectiveTransformForSpotlight(
			mgl32.Vec3{5.5, 2, 13.5}, mgl32.QuatIdent(), 45, 15, 512),
		Color: mgl32.Vec4{0, 0.7, 0, 1},
	}
}

func TestIsOff(t *testing.T) {
	lit := staticLight()
	if lit.IsOff() {
		t.Error("a light with a colour must not report as off")
	}
	off := staticLight()
	off.Color = flashlightOffColor // {0,0,0,1}: alpha stays 1 while the light is off
	if !off.IsOff() {
		t.Error("a black light must report as off, it contributes nothing")
	}

	dark := staticLight()
	dark.Color = mgl32.Vec4{0, 0.7, 0, 0} // alpha is intensity, so zero is off too
	if !dark.IsOff() {
		t.Error("a light at zero intensity must report as off")
	}
}

// The shader reads a light's reach and cone out of the spare vec4 components, and
// they have to be the same frustum it renders its shadow map from - otherwise it
// lights ground it has no depth information for, or stops short of ground it does.
func TestPackedCarriesFrustumReachAndCone(t *testing.T) {
	l := staticLight() // 45 degree FOV, 15 units far
	pos, col, dir, _, _ := l.packed()

	if pos.W() != 15 {
		t.Errorf("reach: got %v want the light's far plane, 15", pos.W())
	}
	// cos(22.5 degrees)
	if want := float32(0.9238795); mgl32.Abs(dir.W()-want) > 0.0001 {
		t.Errorf("cone cosine: got %v want %v (half of a 45 degree FOV)", dir.W(), want)
	}
	if dir.Vec3().Len() < 0.99 {
		t.Errorf("cone axis must be a unit forward vector, got %v", dir.Vec3())
	}
	if col != l.Color {
		t.Errorf("colour must pass through untouched, got %v want %v", col, l.Color)
	}
}

func TestShadowCacheHoldsWhileNothingChanges(t *testing.T) {
	l := staticLight()
	const scene = uint64(1234)

	if !l.NeedsShadowUpdate(scene) {
		t.Fatal("a light that has never rendered must need an update")
	}
	l.MarkShadowRendered(scene)

	if l.NeedsShadowUpdate(scene) {
		t.Error("nothing changed, so the cached depth map must still be valid")
	}
	// repeated calls must stay stable, not drift
	for i := 0; i < 100; i++ {
		if l.NeedsShadowUpdate(scene) {
			t.Fatalf("cache went stale on call %d without anything changing", i)
		}
	}
}

func TestShadowCacheInvalidatesOnChange(t *testing.T) {
	l := staticLight()
	l.MarkShadowRendered(1234)

	if !l.NeedsShadowUpdate(5678) {
		t.Error("a changed scene must invalidate the cached depth map")
	}

	l.MarkShadowRendered(1234)
	l.SetPosition(mgl32.Vec3{9, 9, 9}) // the light moved
	if !l.NeedsShadowUpdate(1234) {
		t.Error("a moved light must invalidate its own cached depth map")
	}
}

// An omni light is recognised by the shader only through the sentinel in
// direction.w, and it must never take a shadow pass: it has one frustum and would
// render a depth map covering a fifth of the sphere it claims to light.
func TestOmniLightPacksTheSentinelAndSkipsShadows(t *testing.T) {
	omni := staticLight()
	omni.Omni = true

	_, _, dir, _, _ := omni.packed()
	if dir.W() != omniConeCosine {
		t.Errorf("omni light packed cone cosine %v, want %v", dir.W(), omniConeCosine)
	}
	if omni.NeedsShadowUpdate(1) {
		t.Error("an omni light asked for a shadow pass")
	}

	spot := staticLight()
	if spot.NeedsShadowUpdate(1) != true {
		t.Error("a spotlight that never rendered still needs its shadow pass")
	}
	if _, _, spotDir, _, _ := spot.packed(); spotDir.W() == omniConeCosine {
		t.Error("a spotlight packed the omni sentinel")
	}
}

// The whole point of the split: the number of lights a map may hold is no longer
// the number of depth textures the atlas has room for.
func TestShadowLayersAreHandedOutToCastersOnly(t *testing.T) {
	lights := make([]LightInfo, MaxShadowLayers+4)
	for i := range lights {
		lights[i] = *staticLight()
	}
	lights[1].Omni = true // a point light has no single frustum to render depth from

	assignShadowLayers(lights)

	if lights[0].ShadowLayer != 0 {
		t.Errorf("first caster got layer %d, want 0", lights[0].ShadowLayer)
	}
	if lights[1].ShadowLayer != noShadowLayer {
		t.Errorf("omni light got layer %d, want none", lights[1].ShadowLayer)
	}
	if lights[2].ShadowLayer != 1 {
		t.Errorf("the caster after an omni light got layer %d, want 1 - the omni light must not eat a layer",
			lights[2].ShadowLayer)
	}

	// The omni light left one layer over, so exactly MaxShadowLayers casters fit and
	// the rest light without shadowing rather than sampling someone else's layer.
	casters := 0
	for i := range lights {
		if lights[i].ShadowLayer >= 0 {
			casters++
		}
		if lights[i].ShadowLayer >= MaxShadowLayers {
			t.Fatalf("light %d got layer %d, past the %d the atlas holds", i, lights[i].ShadowLayer, MaxShadowLayers)
		}
	}
	if casters != MaxShadowLayers {
		t.Errorf("%d lights got a layer, want the atlas full at %d", casters, MaxShadowLayers)
	}
	if last := lights[len(lights)-1]; last.ShadowLayer != noShadowLayer {
		t.Errorf("light past the last layer got layer %d, want none", last.ShadowLayer)
	}
	if lights[len(lights)-1].NeedsShadowUpdate(1) {
		t.Error("a light with no layer asked for a shadow pass")
	}
}

// A light that changes layer is pointing at a depth map that was rendered for
// someone else, so its cache has to go with it.
func TestChangingLayerDropsTheCachedShadowMap(t *testing.T) {
	lights := []LightInfo{*staticLight(), *staticLight()}
	assignShadowLayers(lights)
	lights[1].MarkShadowRendered(1)
	if lights[1].NeedsShadowUpdate(1) {
		t.Fatal("the light just rendered its depth map")
	}

	// The light in front of it went away, so it moves down a layer.
	lights = lights[1:]
	assignShadowLayers(lights)

	if lights[0].ShadowLayer != 0 {
		t.Fatalf("layer %d after the light in front was removed, want 0", lights[0].ShadowLayer)
	}
	if !lights[0].NeedsShadowUpdate(1) {
		t.Error("a light that changed layer kept a depth map rendered into another layer")
	}
}

// The player light is slot 0 and everything that rebuilds the list has to put it
// back there: the frame loop updates it by index, and lights[0] would panic on an
// empty list.
func TestPlayerLightKeepsSlotZero(t *testing.T) {
	lu := &LightsUBO{trackedIndex: noTrackedLight}
	player := lu.newPlayerLight(PlayerLightFlashlight)

	lu.lights = append([]LightInfo{player}, *staticLight(), *staticLight())
	if got := lu.playerLight(); got.Color != flashlightColor {
		t.Errorf("slot 0 held %v, want the player's light", got.Color)
	}

	lu.lights = nil
	if got := lu.playerLight(); got.PerspectiveTransform == nil {
		t.Error("an empty list must still yield a usable player light, not a nil transform")
	}
}

// Both of the player's lights cast: a torch is a wide cone, not an omni light, and
// an omni light is the one thing that cannot have a shadow map here.
func TestPlayerLightKindsBothCastAndAreExclusive(t *testing.T) {
	lu := &LightsUBO{trackedIndex: noTrackedLight}

	flashlight := lu.newPlayerLight(PlayerLightFlashlight)
	if flashlight.Omni || !flashlight.CastsShadow() {
		t.Error("a flashlight is a shadow-casting spotlight")
	}
	torch := lu.newPlayerLight(PlayerLightTorch)
	if torch.Omni || !torch.CastsShadow() {
		t.Error("a carried torch casts a shadow, so it cannot be an omni light")
	}

	// Both hold a layer, so cycling between them cannot leave one unshadowed.
	lights := []LightInfo{flashlight, torch}
	assignShadowLayers(lights)
	for i, light := range lights {
		if light.ShadowLayer < 0 {
			t.Errorf("player light kind %d got no shadow layer", i)
		}
	}

	if torch.Color == flashlight.Color {
		t.Error("a torch and a flashlight must not be the same light")
	}
	// The difference between them is the cone: a beam against a pool of light.
	if torch.ConeCosine() >= flashlight.ConeCosine() {
		t.Errorf("torch cone cosine %v is not wider than the flashlight's %v",
			torch.ConeCosine(), flashlight.ConeCosine())
	}
	if lu.newPlayerLight(PlayerLightOff).IsOff() != true {
		t.Error("the off kind must report as off")
	}
}

// A torch is baked into the level, not carried in the light list: that is what
// keeps a map full of them under the slot cap.
func TestBakedMapLightsSeedTheVoxelLightInsteadOfASlot(t *testing.T) {
	dynamic, baked := splitMapLights([]core.MapLight{
		{Position: mgl32.Vec3{1.5, 0.3, 2.5}, Color: mgl32.Vec3{1, 1, 1}, Baked: true},
		{Position: mgl32.Vec3{5.5, 2, 13.5}, Color: mgl32.Vec3{1, 1, 1}, Angle: 45, Reach: 15},
	}, 512)

	if len(dynamic) != 1 {
		t.Fatalf("%d lights went into the light list, want only the one that is not baked", len(dynamic))
	}
	if len(baked) != 1 {
		t.Fatalf("%d torches went into the bake, want 1", len(baked))
	}
	if got := baked[0]; got.X != 1 || got.Y != 0 || got.Z != 2 {
		t.Errorf("torch baked at %v, want the grid cell holding 1.5, 0.3, 2.5", got)
	}
}
