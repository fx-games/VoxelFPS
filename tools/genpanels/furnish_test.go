package main

import (
	"math"
	"testing"
)

// Every face of every furnishing reads its texture at the same number of
// texels per unit across as down.
func TestFurnishingTexelsAreSquare(t *testing.T) {
	cell := [4]float32{0.1, 0.2, 0.6, 0.7}
	for _, piece := range furnishings() {
		for i, p := range piece.parts {
			var positions, normals [][3]float32
			var uvs [][2]float32
			var indices []uint16
			texturedBox(p.min, p.max, cell, cell, &positions, &normals, &uvs, &indices)
			for f := 0; f < len(positions); f += 4 {
				across := (uvs[f+2][0] - uvs[f+1][0]) / span(positions[f+1], positions[f+2])
				down := (uvs[f+1][1] - uvs[f][1]) / span(positions[f], positions[f+1])
				if math.Abs(float64(across/down-1)) > 1e-3 {
					t.Errorf("%s part %d face %d: %g texels across a unit, %g down", piece.name, i, f/4, across, down)
				}
			}
		}
	}
}
