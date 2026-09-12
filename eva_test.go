package main

import "testing"

// The horizon eases home a step at a time and then stops dead on level: a step
// that would overshoot lands on zero instead of wobbling either side of it.
func TestLevelledRollSettlesOnLevel(t *testing.T) {
	for _, c := range []struct{ roll, step, want float32 }{
		{90, 30, 60},
		{-90, 30, -60},
		{20, 30, 0},
		{-20, 30, 0},
		{0, 30, 0},
	} {
		if got := levelledRoll(c.roll, c.step); got != c.want {
			t.Errorf("levelledRoll(%v, %v) = %v, want %v", c.roll, c.step, got, c.want)
		}
	}
}
