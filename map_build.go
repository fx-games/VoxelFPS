package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads/voxel"
)

// Painting a level, for the generated test maps.
//
// There used to be a tile grid in the map format - a 2D description of the
// level that the loader expanded into voxels on every load - and it was the
// second of two shapes a level had. It could not say anything the editor's
// block tools could say, so a map had geometry you could edit and geometry you
// could save, and they were not the same geometry. It is gone: a level is
// blocks, whether it came off disk or out of one of these three calls.

// levelChunkSize is the edge of a chunk on every axis. It is what a level's
// buffer is rounded up to, which is why a level also has to say how big it
// actually is - see voxel.Map.SetBounds.
const levelChunkSize = 32

// newLevel is an empty level of the given size in voxels.
func newLevel(size voxel.Int3) *voxel.Map {
	chunksFor := func(voxels int32) int32 {
		return int32(math.Ceil(float64(voxels) / levelChunkSize))
	}
	blocks := voxel.NewMapWithEmptyChunks(
		chunksFor(size.X), chunksFor(size.Y), chunksFor(size.Z),
		levelChunkSize, levelChunkSize,
	)
	blocks.SetBounds(size)
	return blocks
}

// roomBlock is what a generated level is built out of: one texture on the
// sides, one on top, one underneath. Greedy meshing only draws the faces that
// are actually exposed, so the floor slab shows its top and the ceiling slab
// its bottom - one block does for all three surfaces.
func roomBlock(wallTex, floorTex, ceilTex uint16) *voxel.Block {
	return voxel.NewBlockWithSides(map[voxel.CubeSide]uint16{
		voxel.North: wallTex, voxel.South: wallTex,
		voxel.East: wallTex, voxel.West: wallTex,
		voxel.Top: floorTex, voxel.Bottom: ceilTex,
	})
}

// fillBlocks makes every block in the inclusive box between two corners solid.
func fillBlocks(blocks *voxel.Map, from, to voxel.Int3, block *voxel.Block) {
	var cells []voxel.Int3
	for x := min(from.X, to.X); x <= max(from.X, to.X); x++ {
		for y := min(from.Y, to.Y); y <= max(from.Y, to.Y); y++ {
			for z := min(from.Z, to.Z); z <= max(from.Z, to.Z); z++ {
				cells = append(cells, voxel.Int3{X: x, Y: y, Z: z})
			}
		}
	}
	blocks.SetBlocks(cells, block)
}

// paintRoom is the shape every test map starts from: a floor slab at the
// bottom, a ceiling at the top, and four walls round the edge between them.
func paintRoom(blocks *voxel.Map, block *voxel.Block) {
	size := blocks.Bounds()
	lastX, lastY, lastZ := size.X-1, size.Y-1, size.Z-1

	fillBlocks(blocks, voxel.Int3{}, voxel.Int3{X: lastX, Z: lastZ}, block)
	fillBlocks(blocks, voxel.Int3{Y: lastY}, voxel.Int3{X: lastX, Y: lastY, Z: lastZ}, block)

	fillBlocks(blocks, voxel.Int3{Y: 1}, voxel.Int3{X: lastX, Y: lastY - 1}, block)
	fillBlocks(blocks, voxel.Int3{Y: 1, Z: lastZ}, voxel.Int3{X: lastX, Y: lastY - 1, Z: lastZ}, block)
	fillBlocks(blocks, voxel.Int3{Y: 1}, voxel.Int3{Y: lastY - 1, Z: lastZ}, block)
	fillBlocks(blocks, voxel.Int3{X: lastX, Y: 1}, voxel.Int3{X: lastX, Y: lastY - 1, Z: lastZ}, block)
}

// groundLevel is the walkable height in a level with one floor slab under it,
// which is every generated map: the slab is y=0, so what stands on it is at 1.
const groundLevel = 1

// tileCentre is the middle of the column at x,z, standing at height y. Entities
// are placed at absolute positions now, and a generator that lays a level out on
// a grid still wants to say "the middle of that square".
func tileCentre(x, z int, y float32) mgl32.Vec3 {
	return mgl32.Vec3{float32(x) + 0.5, y, float32(z) + 0.5}
}
