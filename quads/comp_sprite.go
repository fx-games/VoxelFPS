package quads

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
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
	// BeamTo is the far end of a beam sprite, in world coordinates: the near
	// end is the sprite's own position. Only SpriteModeBeam reads it. Size.Y
	// is how thick the beam is; Size.X is ignored, because its length is the
	// distance between the two ends.
	BeamTo mgl32.Vec3
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
	BakedLight float32
	// AnimBase is where this sprite's kind registered its own animations in
	// SpriteDrawer's single list. Zero is the engine's own set, which is what
	// everything that predates per-kind animations uses.
	AnimBase          int
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

// Global turns one of this sprite's own animation numbers - AnimIdle, AnimWalk
// and the rest - into the number SpriteDrawer knows it by. Every kind of thing
// registers its own six animations at startup and gets a different AnimBase, so
// "walk" is a different entry in the drawer's one list for a guard than it is
// for a mutant. Callers keep saying AnimWalk; this is where that becomes a
// slot.
func (s *SpriteComponent) Global(animIndex int) int {
	if animIndex < 0 {
		return animIndex
	}
	return s.AnimBase + animIndex
}

// LocalAnimation is which of this sprite's own six is playing - AnimIdle,
// AnimWalk and the rest - rather than the slot that occupies in SpriteDrawer's
// one list. Anything keyed by animation has to be keyed by this: the slot moves
// with the kind, and only the local number means the same thing for all of them.
func (s *SpriteComponent) LocalAnimation() int {
	if s.currentAnimation < 0 {
		return s.currentAnimation
	}
	return s.currentAnimation - s.AnimBase
}

// IsPlaying asks the question the other way round: is this sprite currently
// playing its own AnimWalk? Comparing CurrentAnimation() to AnimWalk directly
// is only right for a sprite whose base happens to be zero.
func (s *SpriteComponent) IsPlaying(animIndex int) bool {
	return s.currentAnimation == s.Global(animIndex)
}

func (s *SpriteComponent) PlayAnimation(animIndex int) {
	s.playGlobal(s.Global(animIndex))
}

// playGlobal is the body every Play* shares, taking a slot that has already
// been resolved. Resolving in one place keeps a base from being added twice.
func (s *SpriteComponent) playGlobal(animIndex int) {
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
	animationIndex = s.Global(animationIndex)
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
	s.playGlobal(animationIndex)
}

func (s *SpriteComponent) Position() mgl32.Vec3 {
	return s.resolver.Transform().GetPosition()
}

func (s *SpriteComponent) Transform(observer core.Transformer) mgl32.Mat4 {
	transformMatrix := s.Orientation(observer)
	if s.DrawMode() == SpriteModeBeam {
		// A beam's own axes are already as long as the beam and as thick as it
		// was asked to be - see beamTransform - so there is no size to apply.
		return transformMatrix
	}
	if s.Size != (mgl32.Vec2{1, 1}) {
		transformMatrix = transformMatrix.Mul4(mgl32.Scale3D(s.Size.X(), s.Size.Y(), 1))
	}
	return transformMatrix
}

// Orientation is the same matrix without the quad's own size: where the sprite
// stands and which way it turns to be looked at.
//
// It is what carries a box that was already measured in world units, which is
// every box this engine takes off a picture - SpriteAABB multiplies by Size the
// moment it measures one. Handing such a box the full Transform sizes it by the
// sprite twice: a first aid kit drawn a third of a unit wide was picked out, and
// outlined, a ninth of a unit wide, and a lifter drawn half again as wide as a
// voxel got a box two voxels across.
func (s *SpriteComponent) Orientation(observer core.Transformer) mgl32.Mat4 {
	if s.DrawMode() == SpriteModeBeam {
		return s.beamTransform(observer)
	}
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
	return transformMatrix
}

// beamTransform builds the matrix for a quad stretched along a line and turned
// about it to face the observer.
//
// The quad is a unit square in its own XY plane, so the matrix is three axes
// and a position: local X along the beam and as long as it, local Y across the
// beam and as thick as it, local Z the way the quad faces. Turning about the
// beam rather than about the camera is the whole point - what has to end up
// pointing at the observer is the face, and the length has to stay where it
// was put.
func (s *SpriteComponent) beamTransform(observer core.Transformer) mgl32.Mat4 {
	from, to := s.Position(), s.BeamTo
	along := to.Sub(from)
	length := along.Len()
	if length < 1e-5 {
		return mgl32.Translate3D(from.X(), from.Y(), from.Z())
	}
	direction := along.Mul(1 / length)
	middle := from.Add(to).Mul(0.5)

	// Across the beam and across the line of sight at once, which is the one
	// direction that keeps the whole length of it visible.
	// across the beam and across the line of sight at once, in this order: the
	// three axes have to come out right handed or the quad's one triangle pair
	// winds the wrong way and the face cull throws it away - which is a beam
	// that is drawn every frame and never appears.
	toObserver := observer.GetPosition().Sub(middle)
	across := toObserver.Cross(direction)
	if across.Len() < 1e-5 {
		// Looking straight down the beam: there is no such direction, and any
		// perpendicular will do because the quad is about to be edge on
		// anyway.
		across = mgl32.Vec3{0, 1, 0}.Cross(direction)
		if across.Len() < 1e-5 {
			across = mgl32.Vec3{1, 0, 0}.Cross(direction)
		}
	}
	across = across.Normalize()
	facing := direction.Cross(across)

	thickness := s.Size.Y()
	if thickness <= 0 {
		thickness = 1
	}
	return mgl32.Mat4FromCols(
		direction.Mul(length).Vec4(0),
		across.Mul(thickness).Vec4(0),
		facing.Vec4(0),
		middle.Vec4(1),
	)
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

// SpriteAABB is the box a sprite is picked out and bumped into by: the opaque
// part of one of its frames, at the size the quad is actually drawn.
//
// The atlas measures a frame in its own tile - x and y each running the width
// and the height of that tile - so the box only means world units once it has
// been through the same size the quad is scaled by. Depth takes the horizontal
// scale: a billboard has none of its own, and the sliver it is given has to
// grow with the thing rather than stay a fixed five hundredths.
func SpriteAABB(atlas glhf.TextureAtlas, frame int, size mgl32.Vec2) voxel.AABB {
	min, max := atlas.AABBFromOpaqueAtIndex(frame)
	return ScaledSpriteBox(geometry.Box{Min: min, Max: max}, size)
}

// ScaledSpriteBox is the same step for a box that was measured somewhere else -
// an imported kind's, worked out from its sheet and written into its folder, so
// that a spawn never has to read the card back at all.
func ScaledSpriteBox(box geometry.Box, size mgl32.Vec2) voxel.AABB {
	scale := mgl32.Vec3{size.X(), size.Y(), size.X()}
	return voxel.NewAABB(scaleEach(box.Min, scale), scaleEach(box.Max, scale))
}

func scaleEach(v, by mgl32.Vec3) mgl32.Vec3 {
	return mgl32.Vec3{v.X() * by.X(), v.Y() * by.Y(), v.Z() * by.Z()}
}
