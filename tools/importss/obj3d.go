package main

// System Shock's own 3D models, out of obj3d.res.
//
// A model is not a mesh in the file - it is a little program. The resource
// holds an eight byte header and then a stream of two-byte commands with no
// framing at all, so there is no skipping one that is not understood: an
// interpreter has to know every command or it loses the stream. What the
// program builds is a list of vertices and a binary tree of nodes whose leaves
// carry faces, each face either painted one palette colour or mapped to one of
// the sixty-four object textures in citmat.res.
//
// The tree is a BSP for the game's own renderer - which side of a plane the
// camera is on decides the order faces are drawn in. Nothing here needs that:
// every face in the whole tree is collected and handed to a depth buffer, which
// is what the tree existed to stand in for.
//
// The format is written down in inkyblackness/ss-specs, media/Geometry.md.

import (
	"encoding/binary"
	"fmt"
	"math"
)

// The commands. Everything the shipped models use is here; a command outside
// this set stops the model rather than being skipped, because skipping is not
// possible - see the note above.
const (
	cmdEndOfNode      = 0x0000
	cmdFaceAnchor     = 0x0001
	cmdVerticesMany   = 0x0003
	cmdColouredFace   = 0x0004
	cmdSetColour      = 0x0005
	cmdNodeAnchor     = 0x0006
	cmdVertexOffsetX  = 0x000A
	cmdVertexOffsetY  = 0x000B
	cmdVertexOffsetZ  = 0x000C
	cmdVertexOffsetXY = 0x000D
	cmdVertexOffsetXZ = 0x000E
	cmdVertexOffsetYZ = 0x000F
	cmdVertexSingle   = 0x0015
	cmdSetColourShade = 0x001C
	cmdTextureMapping = 0x0025
	cmdTexturedFace   = 0x0026
)

// modelHeaderSize is the eight bytes before the first command.
const modelHeaderSize = 8

// model3D is one decoded object: its vertices and every face in its tree.
type model3D struct {
	vertices [][3]float32
	faces    []face3D
}

// face3D is one polygon. Texture is the citmat texture it is mapped to, or -1
// for a face painted in Colour instead. UVs, when the model gave them, are one
// per vertex of this face.
type face3D struct {
	indices []int
	normal  [3]float32
	texture int
	colour  byte
	uvs     [][2]float32
}

// decodeModel3D runs one model's command stream. It is deliberately tolerant at
// the tail: several of the shipped resources run a few bytes past their own
// last command, and stopping there is not an error.
func decodeModel3D(data []byte) (*model3D, error) {
	if len(data) < modelHeaderSize {
		return nil, fmt.Errorf("model is %d bytes, shorter than its header", len(data))
	}
	m := &model3D{}
	// mapping is the texture-coordinate table the next textured face uses, by
	// vertex index. It is set by its own command and read by the face after it.
	mapping := map[int][2]float32{}
	colour := byte(0)

	at := modelHeaderSize
	for at+2 <= len(data) {
		command := int(int16(binary.LittleEndian.Uint16(data[at:])))
		next := at + 2
		switch command {
		case cmdEndOfNode:
			// The end of a node, not of the model: the stream carries on with
			// whichever branch was pending. Nothing here needs the tree, so
			// there is nothing to pop.

		case cmdVertexSingle:
			if next+2+12 > len(data) {
				return m, nil
			}
			m.vertices = append(m.vertices, readVector(data[next+2:]))
			next += 2 + 12

		case cmdVerticesMany:
			if next+4 > len(data) {
				return m, nil
			}
			count := int(int16(binary.LittleEndian.Uint16(data[next:])))
			at := next + 4
			for i := 0; i < count; i++ {
				if at+12 > len(data) {
					return m, nil
				}
				m.vertices = append(m.vertices, readVector(data[at:]))
				at += 12
			}
			next = at

		case cmdVertexOffsetX, cmdVertexOffsetY, cmdVertexOffsetZ:
			if next+8 > len(data) {
				return m, nil
			}
			reference := int(int16(binary.LittleEndian.Uint16(data[next+2:])))
			offset := readFixed(data[next+4:])
			vertex, ok := m.vertexAt(reference)
			if !ok {
				return m, nil
			}
			vertex[command-cmdVertexOffsetX] += offset
			m.vertices = append(m.vertices, vertex)
			next += 8

		case cmdVertexOffsetXY, cmdVertexOffsetXZ, cmdVertexOffsetYZ:
			if next+12 > len(data) {
				return m, nil
			}
			reference := int(int16(binary.LittleEndian.Uint16(data[next+2:])))
			first := readFixed(data[next+4:])
			second := readFixed(data[next+8:])
			vertex, ok := m.vertexAt(reference)
			if !ok {
				return m, nil
			}
			// XY, XZ, YZ - the pair the identifier names, in that order.
			pairs := [3][2]int{{0, 1}, {0, 2}, {1, 2}}[command-cmdVertexOffsetXY]
			vertex[pairs[0]] += first
			vertex[pairs[1]] += second
			m.vertices = append(m.vertices, vertex)
			next += 12

		case cmdNodeAnchor:
			// A splitting plane and two branch offsets. Both branches are in
			// the stream that follows, so walking straight on reaches every
			// face in the tree.
			next += 12 + 12 + 2 + 2

		case cmdFaceAnchor:
			// A length and a plane. The faces themselves are the commands
			// inside it, which the loop reaches next.
			next += 2 + 12 + 12

		case cmdSetColour:
			if next+2 > len(data) {
				return m, nil
			}
			colour = byte(binary.LittleEndian.Uint16(data[next:]))
			next += 2

		case cmdSetColourShade:
			if next+4 > len(data) {
				return m, nil
			}
			if painted, ok := crossIndexColour(int(int16(binary.LittleEndian.Uint16(data[next:])))); ok {
				colour = painted
			}
			next += 4

		case cmdColouredFace:
			if next+2 > len(data) {
				return m, nil
			}
			count := int(int16(binary.LittleEndian.Uint16(data[next:])))
			indices, after, ok := readIndices(data, next+2, count)
			if !ok {
				return m, nil
			}
			m.addFace(face3D{indices: indices, texture: -1, colour: colour})
			next = after

		case cmdTextureMapping:
			if next+2 > len(data) {
				return m, nil
			}
			count := int(int16(binary.LittleEndian.Uint16(data[next:])))
			at := next + 2
			mapping = map[int][2]float32{}
			for i := 0; i < count; i++ {
				if at+10 > len(data) {
					return m, nil
				}
				index := int(int16(binary.LittleEndian.Uint16(data[at:])))
				mapping[index] = [2]float32{readFixed(data[at+2:]), readFixed(data[at+6:])}
				at += 10
			}
			next = at

		case cmdTexturedFace:
			if next+4 > len(data) {
				return m, nil
			}
			texture := int(int16(binary.LittleEndian.Uint16(data[next:])))
			count := int(int16(binary.LittleEndian.Uint16(data[next+2:])))
			indices, after, ok := readIndices(data, next+4, count)
			if !ok {
				return m, nil
			}
			uvs := make([][2]float32, len(indices))
			for i, index := range indices {
				uvs[i] = mapping[index]
			}
			m.addFace(face3D{indices: indices, texture: texture, uvs: uvs})
			next = after

		default:
			// An unknown command means the stream is lost. Whatever was built
			// so far is still a model; going on would be reading noise.
			return m, nil
		}
		at = next
	}
	return m, nil
}

// crossIndexColour is what a Set Colour And Shade command paints with.
//
// Its operand is not a palette index. It is a cross index, 1 to 6, into a
// six-entry table the object's own properties carry - the vcolor table, which
// the game fills before it interprets the model and reads back through
// do_getvscolor. Zero means "leave the colour alone", which is TSSHP's reading
// and matches the game skipping the slot.
//
// Only cyberspace models use the command, and only cyberspace objects carry
// that table: it is in CyberCritterProps, CyberSmallstuffProps and
// SlowPhysicsProps and nowhere else, so there is no table to look up for a
// model taken out of the game and dropped into a level of ours.
//
// So this is the game's own stand-in for that case, the one its authors left
// behind a #ifdef in frsetup.c for running without the real thing:
//
//	g3_set_vcolor(i, 0x33 + (i << 2));
//
// Six spaced entries out of the palette, which is what a cyberspace object
// looks like: flat, bright, and a different colour per part. The shade that
// follows the index is dropped - it selects a row of the game's colour lookup
// table and there is no lookup table here - so a shaded face comes out at full
// brightness rather than one of four steps down from it.
func crossIndexColour(cross int) (byte, bool) {
	if cross < 1 || cross > 6 {
		return 0, false
	}
	return byte(0x33 + (cross-1)<<2), true
}

// vertexAt is a copy of one vertex, or false for an index the model has not
// defined yet - which is a stream that has gone wrong.
func (m *model3D) vertexAt(index int) ([3]float32, bool) {
	if index < 0 || index >= len(m.vertices) {
		return [3]float32{}, false
	}
	return m.vertices[index], true
}

// addFace keeps a face that is actually a polygon and works out which way it
// faces from its own corners - the anchor's normal belongs to the splitting
// plane, not to the face.
func (m *model3D) addFace(f face3D) {
	if len(f.indices) < 3 {
		return
	}
	for _, index := range f.indices {
		if index < 0 || index >= len(m.vertices) {
			return
		}
	}
	f.normal = m.faceNormal(f.indices)
	m.faces = append(m.faces, f)
}

func (m *model3D) faceNormal(indices []int) [3]float32 {
	a, b, c := m.vertices[indices[0]], m.vertices[indices[1]], m.vertices[indices[2]]
	u := [3]float32{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float32{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	n := [3]float32{
		u[1]*v[2] - u[2]*v[1],
		u[2]*v[0] - u[0]*v[2],
		u[0]*v[1] - u[1]*v[0],
	}
	length := float32(math.Sqrt(float64(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])))
	if length < 1e-6 {
		return [3]float32{0, 1, 0}
	}
	return [3]float32{n[0] / length, n[1] / length, n[2] / length}
}

// readIndices reads a face's vertex list.
func readIndices(data []byte, at, count int) ([]int, int, bool) {
	if count <= 0 || at+2*count > len(data) {
		return nil, at, false
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		out[i] = int(int16(binary.LittleEndian.Uint16(data[at+2*i:])))
	}
	return out, at + 2*count, true
}

// readVector reads three coordinates.
func readVector(data []byte) [3]float32 {
	return [3]float32{readFixed(data), readFixed(data[4:]), readFixed(data[8:])}
}

// readFixed reads the format the file stores every coordinate in: a signed
// 32-bit value that is really 16 bits of whole units and 16 of fraction.
func readFixed(data []byte) float32 {
	return float32(int32(binary.LittleEndian.Uint32(data))) / 65536.0
}
