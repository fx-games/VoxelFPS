package main

import "testing"

// The menu shows these labels and applies the step at the same index, so the two
// lists drifting apart would silently run the world at the wrong speed.
func TestTimeFactorLabelsMatchTheSteps(t *testing.T) {
	want := []string{"0.1x", "0.25x", "0.5x", "1.0x", "2.0x", "4.0x"}

	if len(timeFactorLabels) != len(timeFactorSteps) {
		t.Fatalf("%d labels for %d steps", len(timeFactorLabels), len(timeFactorSteps))
	}
	for i, label := range want {
		if timeFactorLabels[i] != label {
			t.Errorf("label %d = %q, want %q", i, timeFactorLabels[i], label)
		}
	}
}

// Nothing sets TimeFactor on a normal run, so the menu has to open on the 1.0
// the engine already starts at, or the first keypress jumps the world's speed.
func TestDefaultTimeFactorIndexIsRealTime(t *testing.T) {
	if timeFactorSteps[defaultTimeFactorIndex] != 1.0 {
		t.Errorf("default index points at %vx, want 1.0x", timeFactorSteps[defaultTimeFactorIndex])
	}
}

// -time-factor takes any number; the menu only holds the steps. It has to open on
// the one nearest what is running, and never off the end of the table.
func TestNearestTimeFactorIndex(t *testing.T) {
	for _, c := range []struct {
		factor float64
		want   int
	}{
		{0.5, 2}, {1, 3}, {4, 5},
		{0.4, 2},  // between 0.25 and 0.5, closer to 0.5
		{0.01, 0}, // below the table
		{100, 5},  // above it
	} {
		if got := nearestTimeFactorIndex(c.factor); got != c.want {
			t.Errorf("nearestTimeFactorIndex(%v) = %d (%vx), want %d (%vx)",
				c.factor, got, timeFactorSteps[got], c.want, timeFactorSteps[c.want])
		}
	}
}
