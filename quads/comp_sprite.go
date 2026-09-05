package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
)

type SpriteComponent struct {
	baseComponent
	CameraFollowMode SpriteMode
	SpriteOrigin     SpriteOrigin
	TextureIndices   []uint16
	AtlasTexture     glhf.TextureAtlas
	Tint             mgl32.Vec4
	// Size is how big the quad is in world units. One by one unless something
	// says otherwise, which is what a sprite out of a packed atlas needs: its
	// frames are all different sizes and the quad has to match.
	Size mgl32.Vec2
	// Emissive is how far past white the sprite glows, 0 for a normal one. It is
	// added after the light loop, so an emissive sprite ignores shadows and lights
	// itself - see the emissive term in quadFragmentShaderSource.
	Emissive          float32
	currentFrame      int
	currentAnimation  int
	currentFrameTime  float32
	loop              bool
	followUpAnimation int
	followUpLoop      bool
	freezeLastFrame   bool
	dieAfterLastFrame bool
}

func (s *SpriteComponent) Type() ComponentType {
	return ComponentTypeSprite
}

func NewSpriteFloor(texture glhf.TextureAtlas, texIndex []uint16) *SpriteComponent {
	return &SpriteComponent{
		CameraFollowMode: SpriteModeFollowHorizontal,
		SpriteOrigin:     SpriteOriginBottomCenter,
		TextureIndices:   texIndex,
		AtlasTexture:     texture,
		Tint:             mgl32.Vec4{1, 1, 1, 1},
		Size:             mgl32.Vec2{1, 1},
		currentAnimation: -1,
	}
}

func NewSprite(texture glhf.TextureAtlas, texIndex []uint16) *SpriteComponent {
	return &SpriteComponent{
		CameraFollowMode: SpriteModeFollowHorizontalAndVertical,
		SpriteOrigin:     SpriteOriginCenter,
		TextureIndices:   texIndex,
		AtlasTexture:     texture,
		Tint:             mgl32.Vec4{1, 1, 1, 1},
		Size:             mgl32.Vec2{1, 1},
		currentAnimation: -1,
	}
}

func (s *SpriteComponent) PlayAnimation(animIndex int) {
	s.currentAnimation = animIndex
	s.currentFrame = 0
	s.currentFrameTime = 0
	s.freezeLastFrame = false
}

func (s *SpriteComponent) PlayAnimationLooping(animationIndex int) {
	s.loop = true
	s.PlayAnimation(animationIndex)
}
func (s *SpriteComponent) PlayAnimationFreezeLastFrame(animationIndex int) {
	s.followUpAnimation = -1
	s.followUpLoop = false
	s.loop = false
	s.PlayAnimation(animationIndex)
	s.freezeLastFrame = true
}
func (s *SpriteComponent) PlayAnimationDieAfterLastFrame(animationIndex int) {
	s.PlayAnimationFreezeLastFrame(animationIndex)
	s.dieAfterLastFrame = true
}
func (s *SpriteComponent) PlayAnimationInterrupting(animationIndex int) {
	s.followUpAnimation = s.currentAnimation
	s.followUpLoop = s.loop
	s.loop = false
	s.PlayAnimation(animationIndex)
}

func (s *SpriteComponent) Position() mgl32.Vec3 {
	return s.resolver.Transform().GetPosition()
}

func (s *SpriteComponent) Transform(observer core.Transformer) mgl32.Mat4 {
	transformMatrix := mgl32.Translate3D(s.Position().X(), s.Position().Y(), s.Position().Z())
	switch s.DrawMode() {
	case SpriteModeFollowHorizontal:
		lookAngle := s.Position().Sub(observer.GetPosition()).Normalize()
		lookAngle = mgl32.Vec3{lookAngle.X(), 0, lookAngle.Z()}
		rotation := mgl32.QuatBetweenVectors(mgl32.Vec3{0, 0, -1}, lookAngle)
		transformMatrix = transformMatrix.Mul4(rotation.Mat4())
	case SpriteModeFollowHorizontalAndVertical:
		rotation := observer.GetRotationMatrix()
		transformMatrix = transformMatrix.Mul4(rotation)
	}

	if size := s.quadSize(); size != (mgl32.Vec2{1, 1}) {
		transformMatrix = transformMatrix.Mul4(mgl32.Scale3D(size.X(), size.Y(), 1))
	}
	return transformMatrix
}

// quadSize is how big to draw the quad, defaulting to one by one so a sprite
// made before there was a size still comes out the size it did.
func (s *SpriteComponent) quadSize() mgl32.Vec2 {
	if s.Size == (mgl32.Vec2{}) {
		return mgl32.Vec2{1, 1}
	}
	return s.Size
}

func (s *SpriteComponent) DrawMode() SpriteMode {
	return s.CameraFollowMode
}

func (s *SpriteComponent) Origin() SpriteOrigin {
	return s.SpriteOrigin
}

func (s *SpriteComponent) TextureIndex(viewDirection int) (uint16, bool) {
	var texIndex uint16
	if len(s.TextureIndices) == 1 {
		texIndex = s.TextureIndices[0]
	} else {
		texIndex = s.TextureIndices[viewDirection]
	}
	return texIndex, true
}
func (s *SpriteComponent) Update(frames []uint16, showTimes []float32, elapsed float64) {
	s.currentFrameTime += float32(elapsed)

	if s.currentFrameTime >= showTimes[s.currentFrame] {
		s.currentFrameTime = 0
		s.currentFrame += 1
		if s.currentFrame >= len(frames) {
			if s.dieAfterLastFrame {
				// Stay on the last frame: the sprite can still be drawn in the
				// frame it dies in.
				s.currentFrame = len(frames) - 1
				s.GameObject().Kill()
			} else if s.freezeLastFrame {
				s.currentFrame = len(frames) - 1
			} else if s.loop {
				s.currentFrame = 0
			} else {
				s.currentFrame = 0
				s.currentFrameTime = 0
				s.currentAnimation = s.followUpAnimation
				s.loop = s.followUpLoop
			}
		}
	}
}
func (s *SpriteComponent) TextureIndexAnimated(frames []uint16) (uint16, bool) {
	textureIndexToShowThisFrame := frames[s.currentFrame]
	return textureIndexToShowThisFrame, true
}
func (s *SpriteComponent) Atlas() glhf.TextureAtlas {
	return s.AtlasTexture
}

func (s *SpriteComponent) TintColor() mgl32.Vec4 {
	return s.Tint
}

func (s *SpriteComponent) CurrentAnimation() int {
	return s.currentAnimation
}

func (s *SpriteComponent) LookDirection() mgl32.Vec2 {
	return s.GameObject().Transform().GetForward2D()
}

func (s *SpriteComponent) IsDirectional() bool {
	return len(s.TextureIndices) > 1
}

func (s *SpriteComponent) CurrentFrame() int {
	return s.currentFrame
}
