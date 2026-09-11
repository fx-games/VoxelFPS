package main

import (
	"slices"
	"testing"
)

// A save bakes the atlas down to what is kept and renumbers it. The block list
// has to survive that: its slots are kept even when no wall in the map uses
// them, and afterwards they point at the same pictures under their new numbers.
func TestBlockListSurvivesTheBake(t *testing.T) {
	designs := []blockDesign{{7, 7, 3, 3, 9, 9}, {40, 40, 40, 40, 40, 40}}
	kept := keptThroughBake([]uint16{0, 3, 12}, designs)
	if want := []uint16{0, 3, 7, 9, 12, 40}; !slices.Equal(kept, want) {
		t.Fatalf("kept %v, want %v", kept, want)
	}
	// What Bake hands back: each kept slot moves to its place in that list.
	remap := make(map[uint16]uint16)
	for now, old := range kept {
		remap[old] = uint16(now)
	}
	remapDesigns(designs, remap)
	if want := (blockDesign{2, 2, 1, 1, 3, 3}); designs[0] != want {
		t.Errorf("first design %v, want %v", designs[0], want)
	}
	if want := (blockDesign{5, 5, 5, 5, 5, 5}); designs[1] != want {
		t.Errorf("second design %v, want %v", designs[1], want)
	}
	if block := designs[0].block(); designOf(block) != designs[0] || !block.IsSolid {
		t.Errorf("design to block and back gave %v", designOf(block))
	}
}
