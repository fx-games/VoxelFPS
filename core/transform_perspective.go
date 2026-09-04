package core

import (
	"github.com/go-gl/mathgl/mgl32"
	"math"
)

type PerspectiveTransform struct {
	*Transform
	fov           float32
	defaultFOV    float32
	windowWidth   int
	windowHeight  int
	nearPlaneDist float32
	farPlaneDist  float32

	// Caches keyed on their own inputs, so nothing has to remember to invalidate
	// them: the projection is rebuilt when a lens parameter changes, the frustum
	// planes when the projection-view matrix does.
	cachedProjection    mgl32.Mat4
	cachedProjectionKey lensParams
	cachedPlanes        [6]mgl32.Vec4
	cachedPlanesKey     mgl32.Mat4
}

// lensParams is every input to mgl32.Perspective, comparable so it can key a cache.
type lensParams struct {
	fov    float32
	width  int
	height int
	near   float32
	far    float32
}

func NewDefaultPerspectiveTransform(width, height int) *PerspectiveTransform {
	return &PerspectiveTransform{
		Transform: &Transform{
			translation: mgl32.Vec3{0, 0, 0},
			rotation:    mgl32.QuatIdent(),
			scale:       mgl32.Vec3{1, 1, 1},
		},
		fov:           float32(45.0),
		defaultFOV:    float32(45.0),
		nearPlaneDist: 0.15,
		farPlaneDist:  100.0,
		windowWidth:   width,
		windowHeight:  height,
	}
}

// spotlightNearPlane is deliberately far out compared to the camera's 0.15. A
// shadow frustum's depth precision is spent on its near-to-far ratio, and 0.15 to 15
// is 100:1, which leaves almost no resolution at the 8-15 units where a floor lit
// from head height actually is. Nothing casts a shadow within half a unit of a light
// bulb, so the near half unit is free to give away.
const spotlightNearPlane = float32(0.5)

func NewPerspectiveTransformForSpotlight(position mgl32.Vec3, rot mgl32.Quat, fov float32, farPlaneDist float32, shadowResolution int) *PerspectiveTransform {
	p := &PerspectiveTransform{
		Transform: &Transform{
			translation: position,
			rotation:    rot,
			scale:       mgl32.Vec3{1, 1, 1},
		},
		fov:           fov,
		defaultFOV:    fov,
		nearPlaneDist: spotlightNearPlane,
		farPlaneDist:  farPlaneDist,
		windowWidth:   shadowResolution,
		windowHeight:  shadowResolution,
	}
	return p
}
func NewPerspectiveTransformForSpotlightFromParent(parent Transformer, fov float32, farPlaneDist float32, shadowResolution int) *PerspectiveTransform {
	t := NewDefaultTransform()
	t.SetPosition(mgl32.Vec3{0, -0.1, 0})
	t.SetParent(parent)
	p := &PerspectiveTransform{
		Transform:     t,
		fov:           fov,
		defaultFOV:    fov,
		nearPlaneDist: spotlightNearPlane,
		farPlaneDist:  farPlaneDist,
		windowWidth:   shadowResolution,
		windowHeight:  shadowResolution,
	}
	return p
}

// GetProjectionMatrix returns the projection matrix for the camera.
// A projection matrix will transform a point from local camera space to screen space. (3D -> 2D)
func (c *PerspectiveTransform) GetProjectionMatrix() mgl32.Mat4 {
	key := lensParams{c.fov, c.windowWidth, c.windowHeight, c.nearPlaneDist, c.farPlaneDist}
	if key != c.cachedProjectionKey {
		c.cachedProjection = mgl32.Perspective(mgl32.DegToRad(c.fov), float32(c.windowWidth)/float32(c.windowHeight), c.nearPlaneDist, c.farPlaneDist)
		c.cachedProjectionKey = key
	}
	return c.cachedProjection
}
func (c *PerspectiveTransform) GetProjectionViewMatrix() mgl32.Mat4 {
	return c.GetProjectionMatrix().Mul4(c.GetViewMatrix())
}

/*
func (c *PerspectiveTransform) GetFrustumPlanes() []mgl32.Vec4 {
    mat := c.GetProjectionMatrix().Mul4(c.GetViewMatrix())
    c1, c2, c3, c4 := mat.Rows()
    return []mgl32.Vec4{ // stored as plane equations (ax + by + cz + d = 0)
        c4.Add(c1),                      // left
        c4.Sub(c1),                      // right
        c4.Sub(c2),                      // top
        c4.Add(c2),                      // bottom
        c4.Mul(c.nearPlaneDist).Add(c3), // front
        c4.Mul(c.farPlaneDist).Sub(c3),  // back
    }
}
*/

/*
public Hit IsPointInFrustum(Vector3 point)
    {
        // Compute vector from camera position to p
        Vector3 v = point - _cameraPosition;

        // compute and test the Z coordinate
        var pcz = Vector3.Dot(v, _z);
        if (pcz > _farD || pcz < _nearD)
            return Hit.Outside;

        // Compute and test the Y coordinate
        var pcy = Vector3.Dot(v, _y);
        var vAux = pcz * _tang;
        if (pcy > vAux || pcy < -vAux)
            return Hit.Outside;

        var pcx = Vector3.Dot(v, _x);
        var hAux = vAux * _ratio;
        if (pcx > hAux || pcx < -hAux)
            return Hit.Outside;

        return Hit.Inside;
    }
*/

// FrustumPlanes extracts the six clip planes from the projection-view matrix
// (Gribb-Hartmann). Each plane is (a, b, c, d) of ax+by+cz+d=0, normalised and
// pointing inwards, so plane.Dot(vec4(p, 1)) is the signed distance from p to
// the plane and a point is inside the frustum when all six are >= 0.
func (c *PerspectiveTransform) FrustumPlanes() [6]mgl32.Vec4 {
	projView := c.GetProjectionViewMatrix()
	if projView == c.cachedPlanesKey {
		return c.cachedPlanes
	}
	r0, r1, r2, r3 := projView.Rows()
	planes := [6]mgl32.Vec4{
		r3.Add(r0), // left
		r3.Sub(r0), // right
		r3.Add(r1), // bottom
		r3.Sub(r1), // top
		r3.Add(r2), // near
		r3.Sub(r2), // far
	}
	for i, p := range planes {
		length := (mgl32.Vec3{p.X(), p.Y(), p.Z()}).Len()
		if length > 0 {
			planes[i] = p.Mul(1 / length)
		}
	}
	c.cachedPlanes = planes
	c.cachedPlanesKey = projView
	return planes
}

func (c *PerspectiveTransform) IsInViewFrustum(point mgl32.Vec3) bool {
	for _, plane := range c.FrustumPlanes() {
		if plane.Dot(point.Vec4(1)) < 0 {
			return false
		}
	}
	return true
}

// IsCubeInViewFrustum tests an axis aligned box against the frustum with the
// positive-vertex test: for each plane only the box corner furthest along the
// plane normal has to be checked. The old version tested whether any corner was
// inside, which culled every box that straddled the frustum without putting a
// corner in it - the common case for a chunk right in front of the camera.
//
// ponytail: plane-vs-AABB is conservative at the frustum corners, so a box just
// outside a corner can survive. Add a corner-vs-box pass if the false positives
// ever show up in the chunk counts.
func (c *PerspectiveTransform) IsCubeInViewFrustum(minVals, maxVals mgl32.Vec3) bool {
	for _, plane := range c.FrustumPlanes() {
		positive := mgl32.Vec3{minVals.X(), minVals.Y(), minVals.Z()}
		if plane.X() >= 0 {
			positive[0] = maxVals.X()
		}
		if plane.Y() >= 0 {
			positive[1] = maxVals.Y()
		}
		if plane.Z() >= 0 {
			positive[2] = maxVals.Z()
		}
		if plane.Dot(positive.Vec4(1)) < 0 {
			return false // the whole box is behind this plane
		}
	}
	return true
}

func (c *PerspectiveTransform) IsSphereInViewFrustum(centerWorldSpace mgl32.Vec3, radiusWorldSpace float32) bool {
	for _, plane := range c.FrustumPlanes() {
		if plane.Dot(centerWorldSpace.Vec4(1)) < -radiusWorldSpace {
			return false
		}
	}
	return true
}

func (c *PerspectiveTransform) tanG() float32 {
	return float32(math.Tan(c.halfFovAngle()))
}

func (c *PerspectiveTransform) halfFovAngle() float64 {
	return float64(mgl32.DegToRad(c.fov / 2))
}
func (c *PerspectiveTransform) GetNearPlaneDist() float32 {
	return c.nearPlaneDist
}

func (c *PerspectiveTransform) GetFarPlaneDist() float32 {
	return c.farPlaneDist
}

// ConeCosine is the cosine of the half FOV: the widest angle off the forward axis
// that a spotlight can light and still have depth information for it in its own
// shadow map.
func (c *PerspectiveTransform) ConeCosine() float32 {
	return float32(math.Cos(c.halfFovAngle()))
}

func (c *PerspectiveTransform) GetAspectRatio() float32 {
	return float32(c.windowWidth) / float32(c.windowHeight)
}

func (c *PerspectiveTransform) GetScreenWidth() int {
	return c.windowWidth
}

func (c *PerspectiveTransform) GetScreenHeight() int {
	return c.windowHeight
}

func (c *PerspectiveTransform) ChangeFOV(change int, minimum uint) {
	minFOV := float32(minimum)
	maxFOV := c.defaultFOV
	newFOV := c.fov + float32(change)
	if newFOV < minFOV {
		newFOV = minFOV
	}
	if newFOV > maxFOV {
		newFOV = maxFOV
	}
	c.fov = newFOV
}

func (c *PerspectiveTransform) ResetFOV() {
	c.fov = c.defaultFOV
}
func (c *PerspectiveTransform) ProjectWorldToScreen(worldPos mgl32.Vec3) mgl32.Vec3 {
	viewMatrix := c.GetViewMatrix()
	projectionMatrix := c.GetProjectionMatrix()
	// TEST
	//projectionMatrix = mgl32.Scale2D(0.5, 0.5).Mat4().Mul4(projectionMatrix)
	// TEST END
	screenPos := projectionMatrix.Mul4x1(viewMatrix.Mul4x1(mgl32.Vec4{worldPos.X(), worldPos.Y(), worldPos.Z(), 1}))
	return mgl32.Vec3{screenPos.X() / screenPos.W(), screenPos.Y() / screenPos.W(), screenPos.Z() / screenPos.W()}
}

func (c *PerspectiveTransform) ProjectWorldToScreenPixels(worldPos mgl32.Vec3) (int, int) {
	screenPos := c.ProjectWorldToScreen(worldPos)
	//screenX := screenPos.X() * (1 / c.UIScale.X())
	//screenY := screenPos.Y() * (1 / c.UIScale.Y())
	screenX := screenPos.X()
	screenY := screenPos.Y()
	return int((screenX + 1) * float32(c.windowWidth) / 2), int((1 - screenY) * float32(c.windowHeight) / 2)
}
