package voxel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	lv "github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
)

// CubeBlock is the one block every solid cell is: a cube whose six properties
// name the atlas slot each face draws (see faceProps).
const CubeBlock = "voxelfps:cube"

// faceProps is indexed by CubeSide.
var faceProps = [6]string{East: "east", West: "west", Top: "top", Bottom: "bottom", South: "south", North: "north"}

// NewRegistry is a VoxelLib registry that knows CubeBlock.
func NewRegistry() *lv.Registry {
	reg := lv.NewRegistry()
	reg.Register(&lv.BlockDef{Name: CubeBlock, Shape: lv.ShapeCube, Opaque: true, Collides: true, LightOpacity: 15})
	return reg
}

// World is the VoxelLib world holding the map's blocks. The GL chunks are a
// render cache of it until the client moves to voxel/render; every cell write
// goes through Chunk.SetBlock or the texture painters, which keep both in step.
func (m *Map) World() *lv.World { return m.world }

// Registry is the block registry of World.
func (m *Map) Registry() *lv.Registry { return m.reg }

// resetWorld makes an empty world covering the voxel grid.
func (m *Map) resetWorld() {
	if m.reg == nil {
		m.reg = NewRegistry()
	}
	x, y, z := m.VoxelDimensions()
	w := lv.NewWorld(m.reg, 0, max(16, int(y+15)&^15))
	for cx := 0; cx <= int(x-1)>>4; cx++ {
		for cz := 0; cz <= int(z-1)>>4; cz++ {
			w.AddChunk(w.NewChunk(lv.ChunkPos{X: cx, Z: cz}))
		}
	}
	m.world = w
}

// cubeState is the world state of a GL block.
func (m *Map) cubeState(b *Block) lv.StateID {
	if b == nil || !b.IsSolid {
		return lv.Air
	}
	props := make(map[string]string, 6)
	for cs := CubeSide(0); cs < 6; cs++ {
		props[faceProps[cs]] = strconv.Itoa(int(b.GetTexture(cs)))
	}
	return m.reg.State(CubeBlock, props)
}

// blockOf is the GL block of a world state.
func (m *Map) blockOf(id lv.StateID) *Block {
	if id == lv.Air {
		return NewAirBlock()
	}
	props := m.reg.StateProps(id)
	tex := make(map[CubeSide]uint16, 6)
	for cs := CubeSide(0); cs < 6; cs++ {
		n, _ := strconv.Atoi(props[faceProps[cs]])
		tex[cs] = uint16(n)
	}
	return NewBlockWithSides(tex)
}

func (m *Map) syncCell(x, y, z int32) {
	if m.world == nil || !m.Contains(x, y, z) {
		return
	}
	m.world.SetBlock(int(x), int(y), int(z), m.cubeState(m.GetGlobalBlock(x, y, z)))
}

// syncWorld rebuilds the world from the GL chunks, after a bulk change.
func (m *Map) syncWorld() {
	m.resetWorld()
	for _, c := range m.chunks {
		if c == nil {
			continue
		}
		ox, oy, oz := c.chunkPosX*m.ChunkSizeHorizontal, c.chunkPosY*m.ChunkSizeHeight, c.chunkPosZ*m.ChunkSizeHorizontal
		for i, b := range c.data {
			if b == nil || !b.IsSolid {
				continue
			}
			x, y, z := int32(i)%m.ChunkSizeHorizontal, int32(i)/m.ChunkSizeHorizontal%m.ChunkSizeHeight, int32(i)/(m.ChunkSizeHorizontal*m.ChunkSizeHeight)
			m.world.SetBlock(int(ox+x), int(oy+y), int(oz+z), m.cubeState(b))
		}
	}
}

const boundsFile = "bounds.json"

type regionHeader struct {
	ChunkSizeHorizontal, ChunkSizeHeight int32
	Width, Height, Depth                 int32 // in GL chunks
	Bounds                               Int3
}

// SaveRegions writes the world as region files plus the grid layout to dir.
func (m *Map) SaveRegions(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "region"), 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(regionHeader{m.ChunkSizeHorizontal, m.ChunkSizeHeight, m.width, m.height, m.depth, m.Bounds()})
	if err := os.WriteFile(filepath.Join(dir, boundsFile), b, 0o644); err != nil {
		return err
	}
	st := anvil.OpenStore(dir, true)
	defer st.Close()
	cs := map[[2]int]anvil.Compound{}
	for _, p := range m.world.Chunks() {
		cs[[2]int{p.X, p.Z}] = anvil.EncodeChunk(m.world, m.world.Chunk(p))
	}
	if err := st.WriteAll(cs); err != nil {
		return err
	}
	return st.Sync()
}

// HasRegions reports whether dir holds a map saved by SaveRegions.
func HasRegions(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, boundsFile))
	return err == nil
}

// LoadRegions reads a map saved by SaveRegions and rebuilds the GL chunks from it.
func (m *Map) LoadRegions(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, boundsFile))
	if err != nil {
		return err
	}
	var h regionHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		return fmt.Errorf("%s: %w", boundsFile, err)
	}
	m.ChunkSizeHorizontal, m.ChunkSizeHeight = h.ChunkSizeHorizontal, h.ChunkSizeHeight
	m.ChunkSizeCube = m.ChunkSizeHorizontal * m.ChunkSizeHeight * m.ChunkSizeHorizontal
	m.width, m.height, m.depth, m.bounds = h.Width, h.Height, h.Depth, h.Bounds
	m.maxChunkHeightForDraw = m.height - 1
	m.chunks = make([]*Chunk, m.width*m.height*m.depth)
	for i := range m.chunks {
		x, y, z := int32(i)%m.width, int32(i)/m.width%m.height, int32(i)/(m.width*m.height)
		m.chunks[i] = NewChunk(m, x, y, z)
	}
	m.resetWorld()
	st := anvil.OpenStore(dir, false)
	defer st.Close()
	for _, p := range m.world.Chunks() {
		root, err := st.Read(p.X, p.Z)
		if err != nil {
			return err
		}
		if root == nil {
			continue
		}
		ch, err := anvil.DecodeChunk(m.world, root)
		if err != nil {
			return fmt.Errorf("chunk %v: %w", p, err)
		}
		m.world.RemoveChunk(p)
		m.world.AddChunk(ch)
	}
	sx, sy, sz := m.VoxelDimensions()
	for x := int32(0); x < sx; x++ {
		for z := int32(0); z < sz; z++ {
			for y := int32(0); y < sy; y++ {
				if id := m.world.Block(int(x), int(y), int(z)); id != lv.Air {
					c := m.GetChunkFromBlock(x, y, z)
					c.data[c.blockIndex(x%m.ChunkSizeHorizontal, y%m.ChunkSizeHeight, z%m.ChunkSizeHorizontal)] = m.blockOf(id)
				}
			}
		}
	}
	for _, c := range m.chunks {
		c.SetDirty()
	}
	m.textureBufferIsDirty = true
	return nil
}
