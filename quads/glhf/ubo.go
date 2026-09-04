package glhf

import (
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"runtime"
	"unsafe"
)

type UniformBufferObject struct {
	ubo           binder
	attributes    []Attr
	std140Offsets []int
}

// std140Layout returns the byte offset of every attribute inside a std140 uniform
// block, plus the size of the block.
func std140Layout(attributes []Attr) (offsets []int, blockSize int) {
	offsets = make([]int, len(attributes))
	offset := 0
	for i, attr := range attributes {
		if !attr.Type.Std140MatchesGoLayout() {
			// The small matrix types pad every column to a vec4 in the block but not
			// in mgl32, so SetAttribute cannot upload them as one packed write.
			panic(fmt.Sprintf("UBO attribute %s: %s has no packed Go equivalent in std140", attr.Name, attr.Type.GLSLName()))
		}
		align := attr.Type.Std140Alignment()
		if offset%align != 0 {
			offset += align - (offset % align)
		}
		offsets[i] = offset
		offset += attr.Type.Std140Size()
	}
	// A std140 block is rounded up to a multiple of its largest base alignment, 16.
	if offset%16 != 0 {
		offset += 16 - (offset % 16)
	}
	return offsets, offset
}

func NewUBOWithAttributes(attributes []Attr) *UniformBufferObject {
	std140Offsets, bufferSizeNeeded := std140Layout(attributes)
	u := NewUBO(bufferSizeNeeded)

	u.attributes = attributes
	u.std140Offsets = std140Offsets

	return u
}

func NewUBO(bufferSizeNeeded int) *UniformBufferObject {
	u := &UniformBufferObject{
		ubo: binder{
			restoreLoc: gl.UNIFORM_BUFFER_BINDING,
			bindFunc: func(obj uint32) {
				gl.BindBuffer(gl.UNIFORM_BUFFER, obj)
			},
		},
	}
	u.generateBuffer(bufferSizeNeeded)

	runtime.SetFinalizer(u, (*UniformBufferObject).delete)
	return u
}

func (u *UniformBufferObject) delete() {
	CallNonBlock(func() {
		gl.DeleteBuffers(1, &u.ubo.obj)
	})
}
func (u *UniformBufferObject) generateBuffer(bufferSizeInBytes int) {
	gl.GenBuffers(1, &u.ubo.obj)
	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	u.ubo.bind()
	gl.BufferData(gl.UNIFORM_BUFFER, bufferSizeInBytes, nil, gl.STREAM_DRAW)

	glErr = gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
	u.ubo.restore()
}

/*
unsigned int lights_index = glGetUniformBlockIndex(shaderA.ID, "Lights");
glUniformBlockBinding(shaderA.ID, lights_index, 2);
*/

// BindUniformBlockTo binds the shader block with the given name to the binding index.
func BindUniformBlockTo(shaderID uint32, blockName string, bindingIndex uint32) {
	blockIndex := gl.GetUniformBlockIndex(shaderID, gl.Str(blockName+"\x00"))

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
	gl.UniformBlockBinding(shaderID, blockIndex, bindingIndex)

	glErr = gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
}

// BindUBOTo binds the UBO to the given binding index.
func (u *UniformBufferObject) BindUBOTo(bindingIndex uint32) {
	gl.BindBufferBase(gl.UNIFORM_BUFFER, bindingIndex, u.ubo.obj)

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}
}

// SetAttribute sets the value of the attribute with the given name.
// NOTE: Requires Begin() and End() to be called before and after setting attributes.
func (u *UniformBufferObject) SetAttribute(attrIndex int, value any) {
	offset := u.std140Offsets[attrIndex]
	size := u.attributes[attrIndex].Type.Std140Size()
	var ptrToValue unsafe.Pointer
	switch v := value.(type) {
	case mgl32.Mat4:
		ptrToValue = gl.Ptr(&v[0])
	case mgl32.Vec4:
		ptrToValue = gl.Ptr(&v[0])
	case mgl32.Vec3:
		ptrToValue = gl.Ptr(&v[0])
	case mgl32.Vec2:
		ptrToValue = gl.Ptr(&v[0])
	default:
		ptrToValue = gl.Ptr(v)
	}

	gl.BufferSubData(gl.UNIFORM_BUFFER, offset, size, ptrToValue)

}

func (u *UniformBufferObject) GetShaderBlockSource(blockName string) string {
	/*

	   layout (std140) uniform Main
	   {
	       vec2 fogRange;
	       vec4 camPosition;
	       vec4 camDirection;
	       vec4 camLightColor;
	       vec4 globalLightColor;
	       vec4 fogColor;
	       mat4 camProjectionView;
	   };
	*/

	var source string
	source += "layout (std140) uniform " + blockName + "\n{\n"
	for _, attr := range u.attributes {
		source += fmt.Sprintf("    %s %s;\n", attr.Type.GLSLName(), attr.Name)
	}
	source += "};\n"

	return source
}

func (u *UniformBufferObject) Begin() {
	u.ubo.bind()
}

func (u *UniformBufferObject) End() {
	u.ubo.restore()
}
