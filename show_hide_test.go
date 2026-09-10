package main

import (
	"strings"
	"testing"

	"github.com/memmaker/goray/quads"
)

// The performance readout is a render-pass toggle like the rest, so it belongs in
// the same menu - and it has to be bound to its own field, which is the half of a
// menu entry a compiler cannot check.
func TestShowHideMenuTogglesPerformance(t *testing.T) {
	game := &Game{GlApplication: &quads.GlApplication{ShouldShowPerformance: true}}

	find := func() string {
		for _, option := range game.showHideOptions() {
			if line := option.String(); strings.HasPrefix(line, "Show Performance:") {
				return line
			}
		}
		return ""
	}

	if got := find(); got != "Show Performance: true" {
		t.Fatalf("Show/Hide menu shows %q, want it on by default", got)
	}

	game.ShouldShowPerformance = false
	if got := find(); got != "Show Performance: false" {
		t.Errorf("Show/Hide menu shows %q, want it bound to ShouldShowPerformance", got)
	}
}
