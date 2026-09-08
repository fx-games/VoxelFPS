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
