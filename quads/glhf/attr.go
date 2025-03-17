package glhf

// AttrFormat defines names and types of OpenGL attributes (vertex format, uniform format, etc.).
//
// Example:
//   AttrFormat{{"position", Vec2}, {"color", Vec4}, {"texCoord": Vec2}}
type AttrFormat []Attr

// Size returns the total size of all attributes of the AttrFormat.
func (af AttrFormat) Size() int {
    total := 0
    for _, attr := range af {
        total += attr.Type.Size()
    }
    return total
}

// Attr represents an arbitrary OpenGL attribute, such as a vertex attribute or a shader
// uniform attribute.
type Attr struct {
    Name string
    Type AttrType
}

// AttrType represents the type of an OpenGL attribute.
type AttrType int

// List of all possible attribute types.
const (
    Int AttrType = iota
    UInt
    Float
    Vec2
    IVec2
    Vec3
    Vec4
    Mat2
    Mat23
    Mat24
    Mat3
    Mat32
    Mat34
    Mat4
    Mat42
    Mat43
)

// Size returns the size of a type in bytes.
func (at AttrType) Size() int {
    switch at {
    case Int:
        return 4
    case UInt:
        return 4
    case Float:
        return 4
    case Vec2:
        return 2 * 4
    case IVec2:
        return 2 * 4
    case Vec3:
        return 3 * 4
    case Vec4:
        return 4 * 4
    case Mat2:
        return 2 * 2 * 4
    case Mat23:
        return 2 * 3 * 4
    case Mat24:
        return 2 * 4 * 4
    case Mat3:
        return 3 * 3 * 4
    case Mat32:
        return 3 * 2 * 4
    case Mat34:
        return 3 * 4 * 4
    case Mat4:
        return 4 * 4 * 4
    case Mat42:
        return 4 * 2 * 4
    case Mat43:
        return 4 * 3 * 4
    default:
        panic("size of vertex attribute type: invalid type")
    }
}

func (at AttrType) Std140BaseAlignment() int {
    switch at {
    case Int:
        return 4
    case UInt:
        return 4
    case Float:
        return 4
    case Vec2:
        return 2 * 4
    case IVec2:
        return 2 * 4
    case Vec3:
        return 4 * 4
    case Vec4:
        return 4 * 4
    case Mat2:
        return 2 * 2 * 4
    case Mat23:
        return 2 * 4 * 4
    case Mat24:
        return 2 * 4 * 4
    case Mat3:
        return 4 * 4 * 4
    case Mat32:
        return 4 * 4 * 4
    case Mat34:
        return 4 * 4 * 4
    case Mat4:
        return 4 * 4 * 4
    case Mat42:
        return 4 * 4 * 4
    case Mat43:
        return 4 * 4 * 4
    default:
        panic("size of vertex attribute type: invalid type")
    }
}

func (at AttrType) GLSLName() string {
    switch at {
    case Int:
        return "int"
    case UInt:
        return "uint"
    case Float:
        return "float"
    case Vec2:
        return "vec2"
    case IVec2:
        return "ivec2"
    case Vec3:
        return "vec3"
    case Vec4:
        return "vec4"
    case Mat2:
        return "mat2"
    case Mat23:
        return "mat2x3"
    case Mat24:
        return "mat2x4"
    case Mat3:
        return "mat3"
    case Mat32:
        return "mat3x2"
    case Mat34:
        return "mat3x4"
    case Mat4:
        return "mat4"
    case Mat42:
        return "mat4x2"
    case Mat43:
        return "mat4x3"
    default:
        panic("size of vertex attribute type: invalid type")
    }
}
