package core

import (
    "encoding/json"
    "github.com/go-gl/mathgl/mgl32"
    "math"
)

type Transformer interface {
    GetTransformMatrix() mgl32.Mat4
    GetPosition() mgl32.Vec3
    GetRotationMatrix() mgl32.Mat4
}

type Transform struct {
    parent      Transformer
    translation mgl32.Vec3
    rotation    mgl32.Quat
    scale       mgl32.Vec3
}

func (t *Transform) SetParent(parent Transformer) {
    t.parent = parent
}
func NewDefaultTransform() *Transform {
    return &Transform{
        translation: mgl32.Vec3{0, 0, 0},
        rotation:    mgl32.QuatIdent(),
        scale:       mgl32.Vec3{1, 1, 1},
    }
}

func NewScaledTransform(scale float32) *Transform {
    return &Transform{
        translation: mgl32.Vec3{0, 0, 0},
        rotation:    mgl32.QuatIdent(),
        scale:       mgl32.Vec3{scale, scale, scale},
    }
}

func NewTransform(position mgl32.Vec3, rotation mgl32.Quat, scale mgl32.Vec3) *Transform {
    return &Transform{
        translation: position,
        rotation:    rotation,
        scale:       scale,
    }
}
func NewTransformFromTopDown(position mgl32.Vec3, viewingAngle, rotationAngle float32) *Transform {
    t := &Transform{
        translation: position,
        rotation:    mgl32.QuatIdent(),
        scale:       mgl32.Vec3{1, 1, 1},
    }
    t.SetTopdownRotation(viewingAngle, rotationAngle)
    return t
}
func NewTransformFromForward(position mgl32.Vec3, forward mgl32.Vec3) *Transform {
    t := &Transform{
        translation: position,
        rotation:    mgl32.QuatIdent(),
        scale:       mgl32.Vec3{1, 1, 1},
    }
    t.SetForward(forward)
    return t
}

func NewTransformFromLookAt(position, target, up mgl32.Vec3) *Transform {
    t := &Transform{
        translation: position,
        rotation:    mgl32.QuatIdent(),
        scale:       mgl32.Vec3{1, 1, 1},
    }
    t.SetLookAt(target, up)
    return t
}

func (t *Transform) MarshalJSON() ([]byte, error) {
    return json.Marshal(struct {
        Position mgl32.Vec3 `json:"translation"`
        Rotation mgl32.Quat `json:"rotation"`
        Scale    mgl32.Vec3 `json:"scale"`
    }{
        Position: t.translation,
        Rotation: t.rotation,
        Scale:    t.scale,
    })
}

func (t *Transform) UnmarshalJSON(data []byte) error {
    var tmp struct {
        Position mgl32.Vec3 `json:"translation"`
        Rotation mgl32.Quat `json:"rotation"`
        Scale    mgl32.Vec3 `json:"scale"`
    }
    err := json.Unmarshal(data, &tmp)
    if err != nil {
        return err
    }
    t.translation = tmp.Position
    t.rotation = tmp.Rotation
    t.scale = tmp.Scale
    return nil
}

// GetTransformMatrix uses the translation, rotation, and scale to create a matrix that represents the transformation of the object.
// The Transformation Matrix will transform coordinates from local space to world space.
func (t *Transform) GetTransformMatrix() mgl32.Mat4 {
    local := t.GetLocalTransform()
    if t.parent != nil {
        return t.parent.GetTransformMatrix().Mul4(local)
    }
    return local
}

func (t *Transform) GetLocalTransform() mgl32.Mat4 {
    translation := t.GetTranslationMatrix()
    rotation := t.GetRotationMatrix()
    scale := t.GetScaleMatrix()
    return translation.Mul4(rotation).Mul4(scale) // This actually represents S * R * T.. order is reversed because of how matrices work

}

// GetViewMatrix returns the inverse of the transform matrix. Use this for cameras.
// The View Matrix will transform coordinates from world space to local space.
func (t *Transform) GetViewMatrix() mgl32.Mat4 {
    transformMatrix := t.GetTransformMatrix()
    return transformMatrix.Inv()
}

func (t *Transform) GetScaleMatrix() mgl32.Mat4 {
    return mgl32.Scale3D(t.scale.X(), t.scale.Y(), t.scale.Z())
}

func (t *Transform) GetRotationMatrix() mgl32.Mat4 {
    return t.rotation.Mat4()
}

func (t *Transform) GetTranslationMatrix() mgl32.Mat4 {
    return mgl32.Translate3D(t.translation.X(), t.translation.Y(), t.translation.Z())
}
func (t *Transform) GetPosition() mgl32.Vec3 {
    if t.parent != nil {
        return t.parent.GetTransformMatrix().Mul4x1(mgl32.Vec4{t.translation.X(), t.translation.Y(), t.translation.Z(), 1}).Vec3()
    }
    return t.translation
}

func (t *Transform) GetBlockPosition() Int3 {
    return PositionToGridInt3(t.GetPosition())
}

func (t *Transform) GetRotation() mgl32.Quat {
    return t.rotation
}
func (t *Transform) SetRotation(rotation mgl32.Quat) {
    t.rotation = rotation
}

func (t *Transform) GetForward() mgl32.Vec3 {
    return t.rotation.Rotate(mgl32.Vec3{0, 0, -1})
}

func (t *Transform) GetUp() mgl32.Vec3 {
    return t.rotation.Rotate(mgl32.Vec3{0, 1, 0})
}

func (t *Transform) GetRight() mgl32.Vec3 {
    return t.rotation.Rotate(mgl32.Vec3{1, 0, 0})
}
func (t *Transform) GetForward2DDiagonal() Int3 {
    forward := t.GetForward()
    gridForward := DirectionToGridInt3(forward)
    cardinalForward := gridForward.ToDiagonalDirection()
    return cardinalForward
}

func (t *Transform) GetForward2DCardinal() Int3 {
    forward := t.GetForward()
    forward2D := mgl32.Vec2{forward.X(), forward.Z()}.Normalize()
    //gridForward := DirectionToGridInt3(forward)
    absX := math.Abs(float64(forward2D.X()))
    absZ := math.Abs(float64(forward2D.Y()))

    if absX > absZ {
        cardinalForward := Int3{int32(float64(forward2D.X()) / absX), 0, 0}
        return cardinalForward
    } else {
        cardinalForward := Int3{0, 0, int32(float64(forward2D.Y()) / absZ)}
        return cardinalForward
    }
}
func (t *Transform) IsPointingUp() bool {
    absX := math.Abs(float64(t.GetForward().X()))
    absY := math.Abs(float64(t.GetForward().Y()))
    absZ := math.Abs(float64(t.GetForward().Z()))

    return absY > absX && absY > absZ && t.GetForward().Y() > 0
}

func (t *Transform) IsPointingDown() bool {
    absX := math.Abs(float64(t.GetForward().X()))
    absY := math.Abs(float64(t.GetForward().Y()))
    absZ := math.Abs(float64(t.GetForward().Z()))

    return (absY > absX && absY > absZ) && t.GetForward().Y() < 0
}
func (t *Transform) GetScale() mgl32.Vec3 {
    return t.scale
}

func (t *Transform) setYRotationAngle(angle float32) {
    t.rotation = mgl32.QuatRotate(angle, mgl32.Vec3{0, 1, 0})
    //println(fmt.Sprintf("[Transform] SetYRotationAngle %v", angle))
}

func (t *Transform) SetForward2DDiagonal(forward Int3) {
    t.SetForward2D(forward.ToDiagonalDirection().ToVec3())
}
func (t *Transform) SetForward2DCardinal(forward Int3) {
    t.SetForward2D(forward.ToCardinalDirection().ToVec3())
}
func (t *Transform) SetForward2D(forward mgl32.Vec3) {
    forward = mgl32.Vec3{forward.X(), 0, forward.Z()}
    t.SetForward(forward)
}

func (t *Transform) SetForward(direction mgl32.Vec3) {
    t.rotation = mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, direction)
    //println(fmt.Sprintf("[Transform] SetForward for %s: %v", t.Name(), direction))
}

func (t *Transform) SetBlockPosition(position Int3) {
    t.SetPosition(position.ToBlockCenterVec3())
}
func (t *Transform) SetPosition(position mgl32.Vec3) {
    t.translation = position
}

func (t *Transform) SetTopdownRotation(viewingAngle, rotationAngle float32) {
    t.rotation = mgl32.AnglesToQuat(mgl32.DegToRad(viewingAngle), mgl32.DegToRad(rotationAngle), 0, mgl32.XYZ)
}
func (t *Transform) SetLookAt(target, up mgl32.Vec3) {
    t.rotation = mgl32.QuatLookAtV(t.translation, target, up).Inverse()
}

func (t *Transform) SetLookAt2D(target mgl32.Vec3) {
    t.rotation = mgl32.QuatLookAtV(t.translation, target, mgl32.Vec3{0, 1, 0}).Inverse()
}

func (t *Transform) SetScale(scale mgl32.Vec3) {
    t.scale = scale
}

func (t *Transform) GetForward2D() mgl32.Vec2 {
    forward := t.GetForward()
    return mgl32.Vec2{forward.X(), forward.Z()}
}

func (t *Transform) GetRelativePosition(pos mgl32.Vec3) mgl32.Vec3 {
    return t.GetTransformMatrix().Inv().Mul4x1(mgl32.Vec4{pos.X(), pos.Y(), pos.Z(), 1}).Vec3()
}

// PositionToGridInt3 converts a Vec3 to a Int3 by flooring the values.
// DO NOT USE THIS FOR NEGATIVE VALUES, eg. direction vectors
func PositionToGridInt3(pos mgl32.Vec3) Int3 {
    return Int3{int32(math.Floor(float64(pos.X()))), int32(math.Floor(float64(pos.Y()))), int32(math.Floor(float64(pos.Z())))}
}
func DirectionToGridInt3(dir mgl32.Vec3) Int3 {
    return Int3{int32(math.Round(float64(dir.X()))), int32(math.Round(float64(dir.Y()))), int32(math.Round(float64(dir.Z())))}
}
