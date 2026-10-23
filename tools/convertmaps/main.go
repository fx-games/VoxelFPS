// Command convertmaps turns every level's old map.bin (gzipped GL chunks) into
// region files in the same folder, then removes the map.bin.
//
//	go run ./tools/convertmaps
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/memmaker/voxelfps/quads/voxel"
)

func main() {
	bins, _ := filepath.Glob("assets/maps/*/map.bin")
	for _, bin := range bins {
		m := voxel.NewMapWithEmptyChunks(1, 1, 1, 32, 32)
		m.LoadFromDisk(bin)
		dir := filepath.Dir(bin)
		if err := m.SaveRegions(dir); err != nil {
			fmt.Fprintln(os.Stderr, bin, err)
			os.Exit(1)
		}
		check := voxel.NewMapWithEmptyChunks(1, 1, 1, 32, 32)
		if err := check.LoadRegions(dir); err != nil {
			fmt.Fprintln(os.Stderr, dir, err)
			os.Exit(1)
		}
		if n := diff(m, check); n > 0 {
			fmt.Fprintf(os.Stderr, "%s: %d cells differ after the round trip\n", dir, n)
			os.Exit(1)
		}
		if err := os.Remove(bin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(bin, "->", dir, m.Bounds())
	}
}

// diff counts cells whose solidity or face textures differ.
func diff(a, b *voxel.Map) int {
	n := 0
	x, y, z := a.VoxelDimensions()
	for i := int32(0); i < x; i++ {
		for j := int32(0); j < y; j++ {
			for k := int32(0); k < z; k++ {
				ba, bb := a.GetGlobalBlock(i, j, k), b.GetGlobalBlock(i, j, k)
				if ba.IsSolid != bb.IsSolid {
					n++
					continue
				}
				for cs := voxel.CubeSide(0); ba.IsSolid && cs < 6; cs++ {
					if ba.GetTexture(cs) != bb.GetTexture(cs) {
						n++
						break
					}
				}
			}
		}
	}
	return n
}
