package glhf

import "github.com/go-gl/gl/v3.3-core/gl"

// InstanceAttr is one float-typed vertex attribute that advances once per
// instance instead of once per vertex. The location is given explicitly rather
// than looked up by name, because the same vertex array is drawn with more than
// one program (the lit pass and the shadow depth pass) and glGetAttribLocation
// only agrees between them when the shader pins the location itself.
type InstanceAttr struct {
	Location uint32
	Floats   int // 1..4, or 16 for a mat4 - which spans four consecutive locations
}

// SetInstanceFormat attaches a second buffer to this array's VAO holding the
// per-instance attributes. Call it once, before the first SetInstanceData.
func (va *VertexArray[V]) SetInstanceFormat(attrs []InstanceAttr) {
	stride := 0
	for _, attr := range attrs {
		stride += attr.Floats
	}
	va.instanceStride = stride * SizeOfFloat32

	va.instanceVbo = &binder{
		restoreLoc: gl.ARRAY_BUFFER_BINDING,
		bindFunc: func(obj uint32) {
			gl.BindBuffer(gl.ARRAY_BUFFER, obj)
		},
	}
	gl.GenBuffers(1, &va.instanceVbo.obj)

	va.vao.bind()
	va.instanceVbo.bind()

	offset := 0
	for _, attr := range attrs {
		// A mat4 does not fit in one attribute slot. It occupies four
		// consecutive vec4 locations, each with its own pointer and divisor.
		slots, width := 1, attr.Floats
		if attr.Floats == 16 {
			slots, width = 4, 4
		}
		for slot := 0; slot < slots; slot++ {
			loc := attr.Location + uint32(slot)
			gl.VertexAttribPointerWithOffset(loc, int32(width), gl.FLOAT, false, int32(va.instanceStride), uintptr(offset))
			gl.VertexAttribDivisor(loc, 1)
			gl.EnableVertexAttribArray(loc)
			offset += width * SizeOfFloat32
		}
	}

	va.instanceVbo.restore()
	va.vao.restore()
}

// SetInstanceData uploads the per-instance attribute data. The slice is the
// concatenation of every instance's attributes in the order given to
// SetInstanceFormat.
func (va *VertexArray[V]) SetInstanceData(data []float32) {
	if va.instanceVbo == nil || len(data) == 0 {
		return
	}
	va.instanceVbo.bind()
	sizeInBytes := len(data) * SizeOfFloat32
	if sizeInBytes > va.instanceBufferSize {
		// Grow to exactly what is needed. The sprite count settles after a few
		// frames, so this reallocates a handful of times and then never again.
		gl.BufferData(gl.ARRAY_BUFFER, sizeInBytes, gl.Ptr(data), gl.DYNAMIC_DRAW)
		va.instanceBufferSize = sizeInBytes
	} else {
		gl.BufferSubData(gl.ARRAY_BUFFER, 0, sizeInBytes, gl.Ptr(data))
	}
	va.instanceVbo.restore()
}

// DrawInstanced draws the whole indexed primitive instanceCount times.
func (va *VertexArray[V]) DrawInstanced(instanceCount int32) {
	if instanceCount <= 0 {
		return
	}
	gl.DrawElementsInstanced(va.primitiveType, int32(va.indexCount), va.indexType, gl.PtrOffset(0), instanceCount)
}
