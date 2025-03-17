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

func NewUniformBufferObject(attributes []Attr) *UniformBufferObject {
    var std140Offsets []int
    println("*** Main UBO ***")
    for i, attr := range attributes {
        ownSize := attr.Type.Std140BaseAlignment()
        if i == 0 {
            std140Offsets = append(std140Offsets, 0)
            println(fmt.Sprintf("%d %s (%d): %d", i, attr.Name, ownSize, 0))
        } else {
            prevSize := attributes[i-1].Type.Std140BaseAlignment()
            alignedOffset := std140Offsets[i-1] + prevSize
            // check if alignedOffset is a multiple of ownSize
            baseAlignmentForMultipleCheck := min(16, ownSize)
            if alignedOffset%baseAlignmentForMultipleCheck != 0 {
                alignedOffset += baseAlignmentForMultipleCheck - (alignedOffset % baseAlignmentForMultipleCheck)
            }
            println(fmt.Sprintf("%d %s (%d): %d", i, attr.Name, ownSize, alignedOffset))
            std140Offsets = append(std140Offsets, alignedOffset)
        }
    }
    bufferSizeNeeded := std140Offsets[len(std140Offsets)-1] + attributes[len(attributes)-1].Type.Std140BaseAlignment()
    u := &UniformBufferObject{
        ubo: binder{
            restoreLoc: gl.UNIFORM_BUFFER_BINDING,
            bindFunc: func(obj uint32) {
                gl.BindBuffer(gl.UNIFORM_BUFFER, obj)
            },
        },
        attributes:    attributes,
        std140Offsets: std140Offsets,
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
    gl.BufferData(gl.UNIFORM_BUFFER, bufferSizeInBytes, nil, gl.STATIC_DRAW)

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
func (u *UniformBufferObject) BindUniformBlockTo(shaderID uint32, blockName string, bindingIndex uint32) {
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
    size := u.attributes[attrIndex].Type.Std140BaseAlignment()
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

    glErr := gl.GetError()
    if glErr != gl.NO_ERROR {
        panic(glErr)
    }
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
