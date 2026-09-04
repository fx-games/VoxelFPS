package glhf

import (
	"runtime"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type Buffer struct {
	binder
	bufferSize int
	usage      uint32
}

func NewInstanceDataBuffer(data []byte) *Buffer {
	buffer := NewBuffer(gl.ARRAY_BUFFER_BINDING, gl.ARRAY_BUFFER, gl.STREAM_DRAW)
	buffer.SetBytes(data)
	return buffer
}

func NewBuffer(restoreLoc, target, usage uint32) *Buffer {
	buffer := &Buffer{
		binder: binder{
			restoreLoc: restoreLoc,
			bindFunc: func(obj uint32) {
				gl.BindBuffer(target, obj)
			},
		},
		usage: usage,
	}

	gl.GenBuffers(1, &buffer.obj)

	glError := gl.GetError()
	if glError != gl.NO_ERROR {
		panic(glError)
	}

	glError = gl.GetError()
	if glError != gl.NO_ERROR {
		panic(glError)
	}

	runtime.SetFinalizer(buffer, (*Buffer).delete)
	return buffer
}

func (t *Buffer) delete() {
	CallNonBlock(func() {
		gl.DeleteBuffers(1, &t.obj)
	})
}

func (t *Buffer) Orphanize() {
	t.bind()
	gl.BufferData(t.obj, t.bufferSize, nil, t.usage)
	t.restore()
}

func (t *Buffer) SetBytes(data []byte) {
	t.bind()
	bufSize := len(data)
	gl.BufferData(t.obj, bufSize, gl.Ptr(data), t.usage)
	t.restore()
}

func (t *Buffer) SetBytesWithOffset(offset int, data []byte) {
	t.bind()
	gl.BufferSubData(t.obj, offset, len(data), gl.Ptr(data))
	t.restore()
}

// ID returns the OpenGL ID of this Buffer.
func (t *Buffer) ID() uint32 {
	return t.obj
}

// Begin binds the Buffer. This is necessary before using the Buffer.
func (t *Buffer) Begin() {
	t.bind()
}

// End unbinds the Buffer and restores the previous one.
func (t *Buffer) End() {
	t.restore()
}
