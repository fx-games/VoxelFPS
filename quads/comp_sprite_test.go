package quads

import "testing"

// A second hit landing while the first hit's flinch animation is still
// playing used to get stuck: PlayAnimationInterrupting captured
// currentAnimation as the "resume to" target, and on the second call that
// was already the hit animation itself, so Update's end-of-animation branch
// kept resuming into the hit animation forever instead of back to whatever
// the actor was doing before the first hit.
func TestRepeatedInterruptDoesNotGetStuckOnTheInterruptAnimation(t *testing.T) {
	const walk, hit = 1, 3
	s := &SpriteComponent{currentAnimation: -1}

	s.PlayAnimationLooping(walk)
	s.PlayAnimationInterrupting(hit)
	s.PlayAnimationInterrupting(hit) // second hit lands mid-flinch

	frames := []uint16{0, 1}
	showTimes := []float32{0.1, 0.1}
	// Run the hit animation out to its end and past it.
	for i := 0; i < 4; i++ {
		s.Update(frames, showTimes, 0.1)
	}

	if s.CurrentAnimation() != walk {
		t.Fatalf("expected the sprite to resume walking after the hit animation finished, stuck on animation %d", s.CurrentAnimation())
	}
	if !s.loop {
		t.Fatal("expected the resumed walk animation to loop again, same as before the hit")
	}
}

// A sprite of a kind that registered its own animations plays its own, not the
// first kind's. Every character imported out of a DOS game brings six
// animations with it and they all live in SpriteDrawer's one list, so AnimWalk
// is a different slot for a guard than it is for a mutant; AnimBase is what
// turns the one into the other, and getting it wrong shows one thing playing
// another thing's death.
func TestAnimBaseTurnsAKindsOwnAnimationIntoItsSlot(t *testing.T) {
	const walk, hit = int(AnimWalk), int(AnimHit)
	s := &SpriteComponent{currentAnimation: -1, AnimBase: 60}

	s.PlayAnimationLooping(walk)
	if got := s.CurrentAnimation(); got != 60+walk {
		t.Fatalf("walk landed on slot %d, want %d", got, 60+walk)
	}
	if !s.IsPlaying(walk) {
		t.Fatal("the sprite does not report playing its own walk")
	}
	if s.IsPlaying(hit) {
		t.Fatal("the sprite reports playing its own hit while walking")
	}

	// An interrupt has to resolve the base exactly once. Adding it twice sent
	// the cursor into whatever kind happens to be registered six slots later.
	s.PlayAnimationInterrupting(hit)
	if got := s.CurrentAnimation(); got != 60+hit {
		t.Fatalf("the interrupt landed on slot %d, want %d", got, 60+hit)
	}

	frames := []uint16{0, 1}
	showTimes := []float32{0.1, 0.1}
	for i := 0; i < 4; i++ {
		s.Update(frames, showTimes, 0.1)
	}
	if got := s.CurrentAnimation(); got != 60+walk {
		t.Fatalf("after the interrupt the sprite resumed on slot %d, want its own walk at %d", got, 60+walk)
	}
}

// A sprite that belongs to no kind keeps the numbers it always used, which is
// what everything written before per-kind animations passes in.
func TestAZeroBaseIsTheAnimationNumberItself(t *testing.T) {
	s := &SpriteComponent{currentAnimation: -1}
	s.PlayAnimation(int(AnimDying))
	if got := s.CurrentAnimation(); got != int(AnimDying) {
		t.Fatalf("with no base, dying landed on %d, want %d", got, AnimDying)
	}
}
