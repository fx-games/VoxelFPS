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
	// PivotOverride, when set, is where this sprite anchors on its transform,
	// as an offset from the frame's own center in fractions of Size - the
	// same convention SpriteOrigin's two fixed choices already use, just not
	// limited to them. A world actor sets this every frame from its picture's
	// own hotspot, because a spawned effect's whole sense of motion is often
	// carried entirely in where each frame's art puts that point, not in
	// anything the script itself says to do.
	PivotOverride *mgl32.Vec2
	// Hidden keeps a sprite out of the pass without taking it out of the world.
	// A script has frames that show nothing, and a spawned thing that is
	// between pictures still has to go on running.
	Hidden bool
	// NoShadow keeps a sprite out of the depth pass, and out of the reckoning
	// of whether the scene has moved. Muzzle smoke drifts every frame, and a
	// hundred puffs that cast would have every light redraw its shadow map
	// every frame for something that in Doom never cast a shadow anyway.
	NoShadow bool
	// Emissive is how far past white the sprite glows, 0 for a normal one. It is
	// added after the light loop, so an emissive sprite ignores shadows and lights
	// itself - see the emissive term in quadFragmentShaderSource.
	Emissive float32
	// Additive draws this sprite with ZDoom's Add render style: what it paints
	// is added to what is already there rather than blended over it. A haze
	// authored at six percent alpha is a whisper drawn that way and a wall of
	// grey drawn the other, because forty overlapping quads of near-black at
	// six percent cover the scene almost completely.
	Additive bool
	// BakedLight is the flood-filled voxel light level, 0 to 15, where this
	// sprite stands. It lights the sprite the way the same number lights the
	// walls around it - Doom lights a thing by the sector it is in, and a
	// sprite that ignored the baked light was black in a room lit by torches.
	// Whoever moves the sprite sets it; a sprite that never moves never has to.
	BakedLight        float32
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
	// Only capture a follow-up target when this is not already the interrupt
	// playing: a second hit landing mid-flinch would otherwise capture
	// currentAnimation == animationIndex as its own follow-up, and Update's
	// end-of-animation branch would resume into that same animation forever -
	// an actor stuck replaying its hit/hurt frame instead of returning to
	// whatever it was doing before the first hit.
	if s.currentAnimation != animationIndex {
		s.followUpAnimation = s.currentAnimation
		s.followUpLoop = s.loop
	}
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

	if s.Size != (mgl32.Vec2{1, 1}) {
		transformMatrix = transformMatrix.Mul4(mgl32.Scale3D(s.Size.X(), s.Size.Y(), 1))
	}
	return transformMatrix
}

func (s *SpriteComponent) DrawMode() SpriteMode {
	return s.CameraFollowMode
}

func (s *SpriteComponent) Origin() SpriteOrigin {
	return s.SpriteOrigin
}

// Hide and Show keep a sprite in the world but out of the pass.
func (s *SpriteComponent) Hide() { s.Hidden = true }
func (s *SpriteComponent) Show() { s.Hidden = false }

func (s *SpriteComponent) TextureIndex(viewDirection int) (uint16, bool) {
	if s.Hidden {
		return 0, false
	}
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
	if s.Hidden {
		return 0, false
	}
	return frames[s.currentFrame], true
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
