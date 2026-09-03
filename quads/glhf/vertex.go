package glhf

import (
    "errors"
    "runtime"
    "unsafe"

    "github.com/go-gl/gl/v3.3-core/gl"
)

const SizeOfFloat32 = 4

// VertexSlice points to a portion of (or possibly whole) vertex array. It is used as a pointer,
// contrary to Go's builtin slices. This is, so that append can be 'in-place'. That's for the good,
// because Begin/End-ing a VertexSlice would become super confusing, if append returned a new
// VertexSlice.
//
// It also implements all basic slice-like operations: appending, sub-slicing, etc.
//
// Note that you need to Begin a VertexSlice before getting or updating it's elements or drawing it.
// After you're done with it, you need to End it.

type GlUInt uint32
type GlFloat float32

type VertexArray[V any] struct {
    vao, vbo          binder
    format            AttrFormat
    vertexSizeInBytes int
    attrOffsets       []int

    ibo                     binder
    primitiveType           uint32
    arrayElementSizeInBytes int
    indexType               uint32
    vertexCount             int
    indexCount              int
    use                     uint32
    currentBufferSize       int

    DebugName string
}

const vertexArrayMinCap = 4

func NewVertexArray[V any](shader *Shader, isDynamic bool) *VertexArray[V] {
    usageHint := uint32(gl.STATIC_DRAW)
    if isDynamic {
        usageHint = gl.DYNAMIC_DRAW
    }
    glError := gl.GetError()
    if glError != gl.NO_ERROR {
        println("failure before binding buffers", glError)
    }
    var dummy V
    va := &VertexArray[V]{
        primitiveType: gl.TRIANGLES,
        vao: binder{
            restoreLoc: gl.VERTEX_ARRAY_BINDING,
            bindFunc: func(obj uint32) {
                gl.BindVertexArray(obj)
            },
        },
        vbo: binder{
            restoreLoc: gl.ARRAY_BUFFER_BINDING,
            bindFunc: func(obj uint32) {
                gl.BindBuffer(gl.ARRAY_BUFFER, obj)
            },
        },
        ibo: binder{
            restoreLoc: gl.ELEMENT_ARRAY_BUFFER_BINDING,
            bindFunc: func(obj uint32) {
                gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, obj)
            },
        },
        format:                  shader.VertexFormat(),
        vertexSizeInBytes:       shader.VertexFormat().Size(),
        arrayElementSizeInBytes: int(unsafe.Sizeof(dummy)),
        attrOffsets:             make([]int, len(shader.VertexFormat())),
        use:                     usageHint,
    }

    offset := 0
    for i, attr := range va.format {
        switch attr.Type {
        case Int, UInt, Float, Vec2, Vec3, Vec4, Mat4:
        default:
            panic(errors.New("failed to create vertex array: invalid attribute type"))
        }
        va.attrOffsets[i] = offset
        offset += attr.Type.Size()
    }
    glError = gl.GetError()
    if glError != gl.NO_ERROR {
        println("failure before array buffers", glError)
    }
    gl.GenVertexArrays(1, &va.vao.obj) // create a vertex array object

    va.vao.bind()

    gl.GenBuffers(1, &va.vbo.obj) // create buffer
    defer va.vbo.bind().restore()

    println("BUFFER name created", va.DebugName, va.vbo.obj)

    glError = gl.GetError()
    if glError != gl.NO_ERROR {
        println("BufferData:", glError)
    }
    va.setAttributesForArray(shader)

    glError = gl.GetError()
    if glError != gl.NO_ERROR {
        println("setAttributesForArray:", glError)
    }

    va.vao.restore()

    runtime.SetFinalizer(va, (*VertexArray[V]).delete)

    return va
}

func (va *VertexArray[V]) CreateQuadIndexBuffer(quadCount int) {
    indices := make([]uint32, quadCount*6)
    for i := 0; i < quadCount; i++ {
        indexOffset := i * 6
        vertexOffset := i * 4
        InsertQuadIndices(indices, indexOffset, vertexOffset)
    }
    va.SetIndices32(indices)
}

func InsertQuadIndices(indices []uint32, atIndex int, vertexOffset int) {
    // Top-left
    indices[atIndex+0] = uint32(vertexOffset + 0)
    // Bottom-left
    indices[atIndex+1] = uint32(vertexOffset + 2)
    // Bottom-right

    indices[atIndex+2] = uint32(vertexOffset + 3)
    // Top-left
    indices[atIndex+3] = uint32(vertexOffset + 0)
    // Bottom-right
    indices[atIndex+4] = uint32(vertexOffset + 3)
    // Top-right
    indices[atIndex+5] = uint32(vertexOffset + 1)
}

// ElementsPerVertex returns the number of float32/int32 elements occupied by one vertex.
func (va *VertexArray[V]) ElementsPerVertex() int {
    return va.vertexSizeInBytes / va.arrayElementSizeInBytes
}

// setIndices Creates an index buffer object and sets the indices for the vertex array.
func (va *VertexArray[V]) setIndices8(indices []uint8) {
    if len(indices) > 0 {
        va.indexType = gl.UNSIGNED_BYTE
        gl.GenBuffers(1, &va.ibo.obj) // create buffer and get the name
        glError := gl.GetError()
        if glError != gl.NO_ERROR {
            println("failed to create vertex array: failed to generate index buffer:", glError)
        }
        va.ibo.bind()
        gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices), gl.Ptr(indices), gl.STATIC_DRAW) // set the index buffer data
        glError = gl.GetError()
        if glError != gl.NO_ERROR {
            println("failed to create vertex array: failed to set index buffer data:", glError)
        }
        va.ibo.restore()
        va.indexCount = len(indices)
    }
}
func (va *VertexArray[V]) SetIndices32(indices []uint32) {
    if len(indices) > 0 {
        va.indexType = gl.UNSIGNED_INT
        if va.ibo.obj == 0 { // reuse the buffer when the indices are replaced, eg. when a batch grows
            gl.GenBuffers(1, &va.ibo.obj) // create buffer and get the name
            glError := gl.GetError()
            if glError != gl.NO_ERROR {
                println("failed to create vertex array: failed to generate index buffer:", glError)
            }
        }
        va.ibo.bind()
        gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW) // set the index buffer data
        glError := gl.GetError()
        if glError != gl.NO_ERROR {
            println("failed to create vertex array: failed to set index buffer data:", glError)
        }
        va.ibo.restore()
        va.indexCount = len(indices)
    }
}
func (va *VertexArray[V]) setAttributesForArray(shader *Shader) {
    for i, attr := range va.format {
        loc := gl.GetAttribLocation(shader.program.obj, gl.Str(attr.Name+"\x00")) // get variable location index from shader

        var size int32
        glType := uint32(gl.FLOAT)
        isFloat := true
        isArrayOfArrays := false
        switch attr.Type {
        case Int:
            size = 1
            glType = gl.INT
            isFloat = false
        case UInt:
            size = 1
            glType = gl.UNSIGNED_INT
            isFloat = false
        case Float:
            size = 1
        case Vec2:
            size = 2
        case IVec2:
            size = 2
            glType = gl.INT
            isFloat = false
        case IVec4:
            size = 4
            glType = gl.INT
            isFloat = false
        case Vec3:
            size = 3
        case Vec4:
            size = 4
        case Mat4:
            size = 4
            isArrayOfArrays = true
        }

        if isArrayOfArrays {
            startLocation := uint32(loc)
            for matrixOffset := uint32(0); matrixOffset < 4; matrixOffset++ {
                gl.VertexAttribPointerWithOffset(startLocation+matrixOffset, size, gl.FLOAT, false, int32(va.vertexSizeInBytes), uintptr(matrixOffset*uint32(size)*SizeOfFloat32))
                gl.VertexAttribDivisor(startLocation+matrixOffset, 0)
                gl.EnableVertexAttribArray(startLocation + matrixOffset)
            }
        } else if isFloat {
            glError := gl.GetError()
            if glError != gl.NO_ERROR {
                println("Before VertexAttribPointerWithOffset:", glError)
            }
            gl.VertexAttribPointerWithOffset(
                uint32(loc),
                size,
                glType,
                false,
                int32(va.vertexSizeInBytes),
                uintptr(va.attrOffsets[i]),
            )
            glError = gl.GetError()
            if glError != gl.NO_ERROR {
                println("After VertexAttribPointerWithOffset:", glError)
            }
            gl.EnableVertexAttribArray(uint32(loc)) // Enable and use this attribute for rendering the associated array
            glError = gl.GetError()
            if glError != gl.NO_ERROR {
                println("EnableVertexAttribArray:", glError)
            }
        } else {
            gl.VertexAttribIPointerWithOffset(
                uint32(loc),
                size,
                glType,
                int32(va.vertexSizeInBytes),
                uintptr(va.attrOffsets[i]),
            )
            gl.EnableVertexAttribArray(uint32(loc)) // Enable and use this attribute for rendering the associated array
        }
    }
    glError := gl.GetError()
    if glError != gl.NO_ERROR {
        println("After Vertex Attributes:", glError)
    }
}
func (va *VertexArray[V]) delete() {
    CallNonBlock(func() {
        gl.DeleteVertexArrays(1, &va.vao.obj)
        gl.DeleteBuffers(1, &va.vbo.obj)
        gl.DeleteBuffers(1, &va.ibo.obj)
    })
}

func (va *VertexArray[V]) Begin() {
    va.vao.bind()
    va.vbo.bind()
    if va.isIndexed() {
        va.ibo.bind()
    }
}

func (va *VertexArray[V]) isIndexed() bool {
    return va.ibo.obj > 0
}

func (va *VertexArray[V]) End() {
    if va.isIndexed() {
        va.ibo.restore()
    }
    va.vbo.restore()
    va.vao.restore()
}

// DrawWithOffset draws the content of the VertexSlice.
// Will result in ONE OpenGL draw call.
// In order to submit one triangle, you need to set the vertex count to 3.
func (va *VertexArray[V]) DrawWithOffset(startIndex, vertexCount int32) {
    if va.isIndexed() {
        gl.DrawElementsWithOffset(va.primitiveType, int32(vertexCount), va.indexType, uintptr(startIndex))
    } else {
        gl.DrawArrays(va.primitiveType, int32(startIndex), int32(vertexCount))
    }
}
func (va *VertexArray[V]) Draw() {
    var totalCount int32
    if va.isIndexed() {
        totalCount = int32(va.indexCount)
    } else {
        totalCount = int32(va.vertexCount)
    }
    va.DrawWithOffset(0, totalCount)
}

func (va *VertexArray[V]) drawInstanced(startIndex, endIndex int, instanceCount int32) {
    count := endIndex - startIndex
    sizeOfUnsignedInt := 4
    gl.DrawElementsInstanced(va.primitiveType, int32(count), va.indexType, gl.Ptr(startIndex*sizeOfUnsignedInt), instanceCount)
}
func (va *VertexArray[V]) drawFromFeedback() {
    gl.DrawTransformFeedback(va.primitiveType, va.vbo.obj)
}
func (va *VertexArray[V]) setVertexDataWithOffset(offset int, data []V) {
    if len(data) == 0 {
        // avoid setting 0 bytes of buffer data
        return
    }
    if va.currentBufferSize == 0 {
        va.setNewBuffer(data)
        return
    }
    dataSizeInBytes := len(data) * va.arrayElementSizeInBytes
    startingOffset := offset * va.vertexSizeInBytes
    requiredSize := (dataSizeInBytes) + startingOffset

    if requiredSize > va.currentBufferSize {
        va.Orphanize()
        va.setNewBuffer(data)
        return
    }
    va.vertexCount = len(data) / va.ElementsPerVertex()
    gl.BufferSubData(gl.ARRAY_BUFFER, startingOffset, dataSizeInBytes, gl.Ptr(data))
    //println("BUFFER update", va.vbo.obj, va.DebugName, "size", dataSizeInBytes)
}

func (va *VertexArray[V]) Orphanize() {
    gl.BufferData(gl.ARRAY_BUFFER, va.currentBufferSize, nil, va.use)
    println("BUFFER orphanized", va.DebugName, va.vbo.obj)
}
func (va *VertexArray[V]) SetVertexData(data []V) {
    va.setVertexDataWithOffset(0, data)
}

func (va *VertexArray[V]) setNewBuffer(data []V) {
    va.vertexCount = len(data) / va.ElementsPerVertex()
    va.currentBufferSize = len(data) * va.arrayElementSizeInBytes
    gl.BufferData(gl.ARRAY_BUFFER, va.currentBufferSize, gl.Ptr(data), va.use)
    println("BUFFER created", va.vbo.obj, va.DebugName, "size", va.currentBufferSize)
}

func (va *VertexArray[V]) VertexData() []V {
    return va.VertexDataFrom(0, va.vertexCount)
}
func (va *VertexArray[V]) VertexDataFrom(i, j int) []V {
    if j-i == 0 {
        // avoid getting 0 bytes of buffer data
        return nil
    }
    data := make([]V, (j-i)*va.vertexSizeInBytes/va.arrayElementSizeInBytes)
    gl.GetBufferSubData(gl.ARRAY_BUFFER, i*va.vertexSizeInBytes, len(data)*va.arrayElementSizeInBytes, gl.Ptr(data))
    return data
}

func (va *VertexArray[V]) SetPrimitiveType(primitiveType uint32) {
    va.primitiveType = primitiveType
}

func (va *VertexArray[V]) TriangleCount() int {
    return (va.vertexCount / 4) / 2
}
