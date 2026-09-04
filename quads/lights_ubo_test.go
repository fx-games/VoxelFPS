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
    pos, col, dir, _ := l.packed()

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
