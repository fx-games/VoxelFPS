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
    UIScale       mgl32.Vec2
}

func (c *PerspectiveTransform) SetUIProjectionScale(uiScale mgl32.Vec2) {
    c.UIScale = uiScale
}

// GetProjectionMatrix returns the projection matrix for the camera.
// A projection matrix will transform a point from camera space to screen space. (3D -> 2D)
func (c *PerspectiveTransform) GetProjectionMatrix() mgl32.Mat4 {
    return mgl32.Perspective(mgl32.DegToRad(c.fov), float32(c.windowWidth)/float32(c.windowHeight), c.nearPlaneDist, c.farPlaneDist)
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

func (c *PerspectiveTransform) IsInViewFrustum(point mgl32.Vec3) bool {
    // Compute vector from camera position to p
    v := point.Sub(c.GetPosition())

    // compute and test the Z coordinate
    pcz := v.Dot(c.GetForward())
    if pcz > c.farPlaneDist || pcz < c.nearPlaneDist {
        return false
    }

    // Compute and test the Y coordinate
    pcy := v.Dot(c.GetUp())
    vAux := pcz * c.tanG()
    if pcy > vAux || pcy < -vAux {
        return false
    }

    pcx := v.Dot(c.GetRight())
    hAux := vAux * c.GetAspectRatio()
    return pcx <= hAux && pcx >= -hAux
}

func (c *PerspectiveTransform) IsCubeInViewFrustum(minVals, maxVals mgl32.Vec3) bool {
    // test 8 corners of the cube

    center := minVals.Add(maxVals).Mul(0.5)

    if c.IsInViewFrustum(center) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{minVals.X(), minVals.Y(), minVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{minVals.X(), minVals.Y(), maxVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{minVals.X(), maxVals.Y(), minVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{minVals.X(), maxVals.Y(), maxVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{maxVals.X(), minVals.Y(), minVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{maxVals.X(), minVals.Y(), maxVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{maxVals.X(), maxVals.Y(), minVals.Z()}) {
        return true
    }

    if c.IsInViewFrustum(mgl32.Vec3{maxVals.X(), maxVals.Y(), maxVals.Z()}) {
        return true
    }

    return false
}

func (c *PerspectiveTransform) sphereFactor() mgl32.Vec2 {
    _sphereFactorY := 1.0 / math.Cos(c.halfFovAngle())

    // if the Aspect ratio is not 1/1 the sphereFactor of X needs to be adjusted for the coordinate system.
    // This makes "Sphere" intersections work by representing the spheres as ovals
    var angleX = math.Atan(float64(c.tanG() * c.GetAspectRatio()))
    _sphereFactorX := 1.0 / math.Cos(angleX)
    return mgl32.Vec2{float32(_sphereFactorX), float32(_sphereFactorY)}
}

func (c *PerspectiveTransform) IsSphereInViewFrustum(centerWorldSpace mgl32.Vec3, radiusWorldSpace float32) bool {
    center := centerWorldSpace
    radius := radiusWorldSpace

    sphereFactor := c.sphereFactor()

    v := center.Sub(c.GetPosition())

    az := v.Dot(c.GetForward())
    if az > c.farPlaneDist+radius || az < c.nearPlaneDist-radius {
        return false
    }

    ax := v.Dot(c.GetRight())
    azX := az * c.tanG() * c.GetAspectRatio()
    radiusX := sphereFactor.X() * radius

    if ax > azX+radiusX || ax < -azX-radiusX {
        return false
    }

    ay := v.Dot(c.GetUp())
    azY := az * c.tanG()
    radiusY := sphereFactor.Y() * radius
    if ay > azY+radiusY || ay < -azY-radiusY {
        return false
    }

    if az > c.farPlaneDist-radius || az < c.nearPlaneDist+radius {
        return true
    }

    if ay > azY-radiusY || ay < -azY+radiusY {
        return true
    }

    return ax <= azX+radiusX && ax >= -azX-radiusX
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
