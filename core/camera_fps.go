package core

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"math"
	"math/rand"
)

const DEG2RAD = math.Pi / 180.0

type FPSCamera struct {
	*PerspectiveTransform
	cameraFront      mgl32.Vec3
	cameraRight      mgl32.Vec3
	cameraUp         mgl32.Vec3
	FpsWalkDirection mgl32.Vec3
	rotatex          float32
	rotatey          float32
	// roll is the horizon's tilt about the view direction, in degrees. Gravity
	// says which way up is and keeps this at zero; the suit is the only thing
	// that turns it, because in vacuum there is no up to be level with. See
	// eva.go.
	roll float32
	// freeLook is the suit's way of steering: see SetFreeLook.
	freeLook bool
	LookSensitivity  float32
	invertedY        bool
	parent           Transformer
}

func (c *FPSCamera) GetUp() mgl32.Vec3 {
	return c.cameraUp
}

func (c *FPSCamera) SetTransform(transform Transform) {
	c.Transform = &transform
}

func (c *FPSCamera) RotateRight(deltaTime float64) {

}

func (c *FPSCamera) RotateLeft(deltaTime float64) {

}

func (c *FPSCamera) GetPickingRayFromScreenPosition(x float64, y float64) (origin mgl32.Vec3, direction mgl32.Vec3) {
	// normalize x and y to -1..1
	normalizedX := (float32(x))*2 - 1
	normalizedY := ((float32(y))*2 - 1) * -1

	return GetRayFromCameraPlane(c, normalizedX, normalizedY)
}

func (c *FPSCamera) GetTransform() Transform {
	return *c.Transform
}

func (c *FPSCamera) MoveInDirection(delta float32, dir [2]int) {
	movePosition := c.GetDesiredPosition(delta, dir)
	c.SetPosition(movePosition)
	c.updateTransform()
}

func (c *FPSCamera) SetForward2D(forward mgl32.Vec3) {
	c.Transform.SetForward2D(forward)
	c.updateMouseOffset()
	c.updateTransform()
}

func (c *FPSCamera) GetDesiredPosition(delta float32, dir [2]int) mgl32.Vec3 {
	currentPos := c.GetPosition()
	moveVector := mgl32.Vec3{0, 0, 0}
	if dir[0] != 0 {
		moveVector = moveVector.Add(c.LeftRight(float32(dir[0]) * delta))
	}
	if dir[1] != 0 {
		moveVector = moveVector.Add(c.PlanarForwardBackward(float32(dir[1]) * delta))
	}
	movePosition := currentPos.Add(moveVector)
	return movePosition
}

func (c *FPSCamera) MoveVertical(delta float32) {
	currentPos := c.GetPosition()
	c.SetPosition(currentPos.Add(c.UpDown(delta)))
	c.updateTransform()
}
func NewFPSCamera(pos mgl32.Vec3, windowWidth, windowHeight int, sensitivity float32) *FPSCamera {
	f := &FPSCamera{
		PerspectiveTransform: NewDefaultPerspectiveTransform(windowWidth, windowHeight),
		cameraFront:          mgl32.Vec3{0, 0, -1},
		cameraUp:             mgl32.Vec3{0, 1, 0},
		LookSensitivity:      sensitivity,
		rotatey:              0,
		rotatex:              -90,
		invertedY:            true,
	}
	f.SetPosition(pos)
	f.updateTransform()
	return f
}

func (c *FPSCamera) GetViewMatrix() mgl32.Mat4 {
	if c.parent != nil {
		parentTransform := c.parent.GetTransformMatrix()
		offsetTrans := mgl32.Translate3D(0, 0.15, 0.2) // right behind our parent
		transformationMatrix := parentTransform.Mul4(offsetTrans)
		return transformationMatrix.Inv()
	}

	return c.Transform.GetViewMatrix()
}
func (c *FPSCamera) SetMouseInvertedY(inverted bool) {
	c.invertedY = inverted
}

func (c *FPSCamera) GetRandomRayInCircleFrustum(accuracy float64) (origin mgl32.Vec3, direction mgl32.Vec3) {
	randX := rand.Float64()*2.0 - 1.0
	randY := rand.Float64()*2.0 - 1.0
	randDir := mgl32.Vec2{float32(randX), float32(randY)}.Normalize()
	gapToPerfection := 1.0 - accuracy
	randomPercent := rand.Float64()
	accuracyAchieved := accuracy + (gapToPerfection * randomPercent)
	return c.GetRayInCircleFrustum(randDir, accuracyAchieved)
}

// GetRayInCircleFrustum returns a ray from the camera's position to the camera's plane.
// Expects x and y to be in the range -1..1
func (c *FPSCamera) GetRayInCircleFrustum(normalizedDir mgl32.Vec2, accuracy float64) (origin mgl32.Vec3, direction mgl32.Vec3) {
	accuracy = Clamp(accuracy, 0.0, 1.0)
	lengthOfVector := 1.0 - accuracy // 0.01..1.0
	x := normalizedDir.X() * float32(lengthOfVector)
	y := normalizedDir.Y() * float32(lengthOfVector)
	return GetRayFromCameraPlane(c, x, y)
}

func Clamp(val float64, minVal, maxVal float64) float64 {
	return min(max(val, minVal), maxVal)
}

func Clamp32(val float32, minVal, maxVal float32) float32 {
	return float32(Clamp(float64(val), float64(minVal), float64(maxVal)))
}

// ChangeAngles changes the camera's angles by dx and dy.
// Used for mouse look in FPS look mode.
func (c *FPSCamera) ChangeAngles(dx, dy float32) {
	if mgl32.Abs(dx) > 200 || mgl32.Abs(dy) > 200 {
		return
	}
	if c.freeLook {
		c.freeLookBy(dx, dy)
		return
	}
	c.rotatex += dx * c.LookSensitivity
	yChange := dy * c.LookSensitivity
	if c.invertedY {
		c.rotatey -= yChange
	} else {
		c.rotatey += yChange
	}

	c.updateTransform()
}

// SetFreeLook switches between the two ways this camera can be steered, and
// everything that moves it - the mouse, a weapon's recoil - goes through
// ChangeAngles either way.
//
// On the ground it is two angles about the world: yaw about world up, pitch off
// the world horizon, clamped short of vertical so a walking player can never end
// up on their head. That clamp is the thing that makes walking feel like
// walking, and it is also exactly what has no meaning in vacuum, where the mouse
// has to turn the *visor* - yaw about the helmet's own up, pitch about its own
// right, with nothing to be upside down relative to. See eva.go.
//
// Leaving free look reads the two angles back off wherever the eyes ended up, so
// the view does not jump when gravity takes over. A player who comes back inside
// looking past vertical cannot be written as a yaw and a pitch at all and has
// their pitch clamped into range - which is what levelling the horizon is going
// to do to them a second later anyway.
func (c *FPSCamera) SetFreeLook(on bool) {
	if c.freeLook == on {
		return
	}
	c.freeLook = on
	if !on {
		c.rotatex = mgl32.RadToDeg(float32(math.Atan2(float64(c.cameraFront.Z()), float64(c.cameraFront.X()))))
		c.rotatey = mgl32.RadToDeg(float32(math.Asin(float64(c.cameraFront.Y()))))
		c.updateTransform()
	}
}

// freeLookBy turns the basis itself, which is what having no up to measure
// against means: the mouse deltas are the same ones the ground uses, but they
// are spent on the camera's own axes instead of the world's.
func (c *FPSCamera) freeLookBy(dx, dy float32) {
	pitchAngle := dy * c.LookSensitivity
	if c.invertedY {
		pitchAngle = -pitchAngle
	}
	turn := mgl32.QuatRotate(mgl32.DegToRad(-dx*c.LookSensitivity), c.cameraUp).
		Mul(mgl32.QuatRotate(mgl32.DegToRad(pitchAngle), c.cameraRight))
	c.setBasis(turn.Rotate(c.cameraFront), turn.Rotate(c.cameraUp))
}

// RollBy turns the horizon about the view direction, in whichever way the camera
// is being steered: on the basis while the suit has it, on the angle otherwise.
func (c *FPSCamera) RollBy(degrees float32) {
	if !c.freeLook {
		c.SetRoll(c.roll + degrees)
		return
	}
	turn := mgl32.QuatRotate(mgl32.DegToRad(degrees), c.cameraFront)
	c.setBasis(c.cameraFront, turn.Rotate(c.cameraUp))
}

// setBasis takes a look direction and an up, squares them up against each other
// and writes the camera from them. It is the free-look half of updateTransform,
// which builds the same three vectors out of two angles instead.
func (c *FPSCamera) setBasis(front, up mgl32.Vec3) {
	c.cameraFront = front.Normalize()
	c.cameraRight = c.cameraFront.Cross(up).Normalize()
	c.cameraUp = c.cameraRight.Cross(c.cameraFront).Normalize()
	// Walking is done on the ground, where the basis comes from the angles
	// instead; this keeps the field sane for anything that reads it meanwhile.
	if walk := (mgl32.Vec3{0, 1, 0}).Cross(c.cameraRight); walk.Len() > 0.0001 {
		c.FpsWalkDirection = walk.Normalize()
	}
	c.roll = c.rollFromBasis()
	c.writeTransform()
}

// rollFromBasis is how far the horizon has been turned from level, measured the
// only way it can be while the basis is the truth: the signed angle from the up
// a level head would have, about the direction being looked in. A camera looking
// straight up or down has no level head to compare with and keeps what it had.
func (c *FPSCamera) rollFromBasis() float32 {
	levelRight := c.cameraFront.Cross(mgl32.Vec3{0, 1, 0})
	if levelRight.Len() < 0.0001 {
		return c.roll
	}
	levelUp := levelRight.Normalize().Cross(c.cameraFront).Normalize()
	turned := levelUp.Cross(c.cameraUp).Dot(c.cameraFront)
	return mgl32.RadToDeg(float32(math.Atan2(float64(turned), float64(levelUp.Dot(c.cameraUp)))))
}

// writeTransform puts the basis into the transform the renderer reads.
func (c *FPSCamera) writeTransform() {
	cameraPosition := c.GetPosition()
	c.Transform.SetRotation(ExtractRotation(mgl32.LookAtV(cameraPosition, cameraPosition.Add(c.cameraFront), c.cameraUp).Inv()))
}
func (c *FPSCamera) ForwardBackward(delta float32) mgl32.Vec3 {
	return c.cameraFront.Mul(delta)
}

func (c *FPSCamera) PlanarForwardBackward(delta float32) mgl32.Vec3 {
	return c.FpsWalkDirection.Mul(delta)
}

func (c *FPSCamera) LeftRight(delta float32) mgl32.Vec3 {
	return c.cameraRight.Mul(delta)
}

func (c *FPSCamera) UpDown(delta float32) mgl32.Vec3 {
	return mgl32.Vec3{0, 1, 0}.Mul(delta)
}

func (c *FPSCamera) updateMouseOffset() {
	front := c.GetForward()
	c.rotatex = mgl32.RadToDeg(float32(math.Atan2(float64(front.Z()), float64(front.X()))))
	c.rotatey = mgl32.RadToDeg(float32(math.Asin(float64(front.Y()))))
}

// Roll is the horizon's tilt in degrees, and SetRoll turns it, the short way
// round: a suit that has rolled all the way over reads as -170 rather than 190,
// so easing it back to zero takes the near side home. See Game.levelHorizon.
func (c *FPSCamera) Roll() float32 { return c.roll }

func (c *FPSCamera) SetRoll(degrees float32) {
	for degrees > 180 {
		degrees -= 360
	}
	for degrees <= -180 {
		degrees += 360
	}
	if degrees == c.roll {
		// Nothing to turn. Rebuilding the matrix anyway is not free of
		// consequence: LookAtV normalises a difference of world positions, so
		// the same angles rebuilt at a different position come back a rounding
		// error apart, which is a frame that no longer matches the one before it.
		return
	}
	c.roll = degrees
	c.updateTransform()
}

func (c *FPSCamera) GetRotation() (float32, float32) {
	return c.rotatex, c.rotatey
}

// SetAngles points the camera without moving it: ChangeAngles' sibling, for
// somewhere that knows the angle it wants rather than a mouse delta.
//
// It takes no position on purpose. What stood here took one, and a camera
// parented to something - which this one is, to the player - has no single
// position to hand it: GetPosition returns where the camera is in the *world*,
// through its parent's matrix, while SetPosition writes where it is *relative*
// to that parent. Reading one and writing it back therefore moved the camera by
// the whole of the player's own position, which put a capture asked for a
// downward pitch outside the level looking at the sky.
func (c *FPSCamera) SetAngles(rotX float32, rotY float32) {
	c.rotatex = rotX
	c.rotatey = rotY
	c.updateTransform()
}
func (c *FPSCamera) AttachTo(t Transformer) {
	c.parent = t
}
func (c *FPSCamera) SetFOV(fov float32) {
	c.fov = fov
}

func (c *FPSCamera) GetFOV() float32 {
	return c.fov
}

func (c *FPSCamera) GetHorizontalFOV() float32 {
	return c.fov * float32(c.windowWidth) / float32(c.windowHeight)
}
func (c *FPSCamera) updateTransform() {
	if c.rotatey > 89 {
		c.rotatey = 89
	}
	if c.rotatey < -89 {
		c.rotatey = -89
	}
	front := mgl32.Vec3{
		float32(math.Cos(float64(c.rotatey*DEG2RAD)) * math.Cos(float64(c.rotatex*DEG2RAD))),
		float32(math.Sin(float64(c.rotatey * DEG2RAD))),
		float32(math.Cos(float64(c.rotatey*DEG2RAD)) * math.Sin(float64(c.rotatex*DEG2RAD))),
	}
	c.cameraFront = front.Normalize()
	c.cameraRight = c.cameraFront.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
	c.cameraUp = c.cameraRight.Cross(c.cameraFront).Normalize()
	// Off the upright basis, before any roll: walking is done with the feet and
	// a tilted head does not lean the deck over.
	c.FpsWalkDirection = mgl32.Vec3{0, 1, 0}.Cross(c.cameraRight).Normalize()
	if c.roll != 0 {
		turn := mgl32.QuatRotate(mgl32.DegToRad(c.roll), c.cameraFront)
		c.cameraRight = turn.Rotate(c.cameraRight).Normalize()
		c.cameraUp = turn.Rotate(c.cameraUp).Normalize()
	}

	c.writeTransform()
}

func (c *FPSCamera) Detach() {
	c.parent = nil
}

func (c *FPSCamera) DebugAim() string {
	pos := c.GetPosition()
	return fmt.Sprintf("Pos: (%0.2f, %0.2f, %0.2f) Aim: (%0.2f, %0.2f)", pos.X(), pos.Y(), pos.Z(), c.rotatex, c.rotatey)
}

func (c *FPSCamera) SetScreenSize(width int, height int) {
	c.windowWidth = width
	c.windowHeight = height
	c.updateTransform()
}

func (c *FPSCamera) GetFarPlaneDist() float32 {
	return c.farPlaneDist
}

func (c *FPSCamera) GetNearAndFar() (near float32, far float32) {
	return c.nearPlaneDist, c.farPlaneDist
}

// ViewDirections is how many angles a thing is drawn from: Doom's eight, and
// the length of the frame lists that go with them.
const ViewDirections = 8

// GetViewDirection is which of the eight views of a thing the observer is
// looking at: 0 the front, 4 the back, running round the thing's own left.
// Doom numbers the same eight 1 to 8 in that order, so a Doom rotation is this
// plus one - see TestGetViewDirectionRunsRoundTheThingsOwnLeft, which is what
// the migration writes a spawned actor's frame lists against.
func GetViewDirection(viewerPos, viewerForward, spritePosition mgl32.Vec3, spriteLookDir mgl32.Vec2) int {
	camPosition2d := mgl32.Vec2{viewerPos.X(), viewerPos.Z()}
	spritePosition2d := mgl32.Vec2{spritePosition.X(), spritePosition.Z()}

	// Which of the eight views you are looking at is a question about where you
	// stand relative to the thing, and a thing standing where you do has no
	// answer: the player's own muzzle flash hangs a hand's breadth from the eye,
	// so the scrap of horizontal offset left when the gun is pointed steeply up
	// or down says "seen from the side" and the flash is drawn edge on - a bright
	// bar across the screen. Inside your own radius the question is answered by
	// which way you are facing instead, and a viewer looking straight at the
	// floor falls back to the thing's own facing, which is the view from behind
	// it: what you see of anything your own gun just spawned.
	separation := spritePosition2d.Sub(camPosition2d)
	if separation.Len() < spriteAtTheViewer {
		separation = mgl32.Vec2{viewerForward.X(), viewerForward.Z()}
	}
	if separation.Len() < 0.0001 {
		separation = spriteLookDir
	}
	if separation.Len() < 0.0001 {
		return 0
	}
	direction := separation.Normalize()

	camDirection := math.Atan2(float64(direction.Y()), float64(direction.X()))
	spriteDirection := math.Atan2(float64(-spriteLookDir.Y()), float64(-spriteLookDir.X()))

	relativeAngle := spriteDirection - camDirection // Reverse subtraction order

	// Normalize angle to range [0, 2π)
	relativeAngle = math.Mod(relativeAngle+2*math.Pi, 2*math.Pi)

	sectorSize := math.Pi / 4 // 8-directional sectors
	return int(math.Mod(math.Round(relativeAngle/sectorSize), 8))
}

type Camera interface {
	GetTransformMatrix() mgl32.Mat4
	GetViewMatrix() mgl32.Mat4
	GetProjectionMatrix() mgl32.Mat4
	GetProjectionViewMatrix() mgl32.Mat4
	GetForward() mgl32.Vec3
	GetPosition() mgl32.Vec3
	MoveInDirection(delta float32, dir [2]int)
	GetNearPlaneDist() float32
	SetPosition(position mgl32.Vec3)
	SetRotation(rotation mgl32.Quat)
	GetTransform() Transform
	SetTransform(transform Transform)
	GetPickingRayFromScreenPosition(x, y float64) (mgl32.Vec3, mgl32.Vec3)
	RotateRight(deltaTime float64)
	RotateLeft(deltaTime float64)
	GetUp() mgl32.Vec3
	GetFarPlaneDist() float32
	GetAspectRatio() float32
	SetParent(transform Transformer)
	IsInViewFrustum(point mgl32.Vec3) bool
	IsSphereInViewFrustum(centerWorldSpace mgl32.Vec3, radiusWorldSpace float32) bool
	IsCubeInViewFrustum(minVals, maxVals mgl32.Vec3) bool
}

// spriteAtTheViewer is how near, measured flat, counts as standing where the
// viewer does. It is the player's own radius: anything inside that is at you
// rather than in front of you or beside you.
const spriteAtTheViewer = 0.5

// GetRayFromCameraPlane returns a ray from the camera plane to the far plane.
// NOTE: Length of ray is hardcoded to 100 units.
// Expect normalizedX and normalizedY to be in range -1..1
func GetRayFromCameraPlane(cam Camera, normalizedX float32, normalizedY float32) (origin mgl32.Vec3, direction mgl32.Vec3) {
	normalizedNearPos := mgl32.Vec4{normalizedX, normalizedY, cam.GetNearPlaneDist(), 1}
	normalizedFarPos := mgl32.Vec4{normalizedX, normalizedY, cam.GetFarPlaneDist(), 1}

	projViewInverted := cam.GetProjectionViewMatrix().Inv()

	// project point from camera space to world space
	nearWorldPos := projViewInverted.Mul4x1(normalizedNearPos)
	farWorldPos := projViewInverted.Mul4x1(normalizedFarPos)

	// perspective divide
	rayStart := nearWorldPos.Vec3().Mul(1 / nearWorldPos.W())
	farPosCorrected := farWorldPos.Vec3().Mul(1 / farWorldPos.W())

	dir := rayStart.Sub(farPosCorrected).Normalize()

	return rayStart, dir
}

func ExtractRotation(transformationMatrix mgl32.Mat4) mgl32.Quat {
	return mgl32.Mat4ToQuat(transformationMatrix.Mat3().Mat4())
}
