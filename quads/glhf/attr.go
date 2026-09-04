package glhf

// AttrFormat defines names and types of OpenGL attributes (vertex format, uniform format, etc.).
//
// Example:
//
//	AttrFormat{{"position", Vec2}, {"color", Vec4}, {"texCoord": Vec2}}
type AttrFormat []Attr

// Size returns the total size in bytes of all attributes of the AttrFormat.
func (af AttrFormat) Size() int {
	total := 0
	for _, attr := range af {
		total += attr.Type.Size()
	}
	return total
}

func (af AttrFormat) ElementsPerVertex() int {
	total := 0
	for _, attr := range af {
		total += attr.Type.ArrayElements()
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
	IVec4
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
	case IVec4:
		return 4 * 4
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

// ArrayElements returns the number of elements in an vertex array for this type.
func (at AttrType) ArrayElements() int {
	switch at {
	case Int:
		return 1
	case UInt:
		return 1
	case Float:
		return 1
	case Vec2:
		return 2
	case IVec2:
		return 2
	case IVec4:
		return 4
	case Vec3:
		return 3
	case Vec4:
		return 4
	case Mat2:
		return 2 * 2
	case Mat23:
		return 2 * 3
	case Mat24:
		return 2 * 4
	case Mat3:
		return 3 * 3
	case Mat32:
		return 3 * 2
	case Mat34:
		return 3 * 4
	case Mat4:
		return 4 * 4
	case Mat42:
		return 4 * 2
	case Mat43:
		return 4 * 3
	default:
		panic("size of vertex attribute type: invalid type")
	}
}

// Std140Alignment returns the base alignment of a type inside a std140 uniform
// block. Scalars align to 4, vec2 to 8, everything else - vec3, vec4 and every
// matrix, which std140 treats as an array of column vectors - to 16.
func (at AttrType) Std140Alignment() int {
	switch at {
	case Int, UInt, Float:
		return 4
	case Vec2, IVec2:
		return 8
	case Vec3, Vec4, IVec4:
		return 16
	case Mat2, Mat23, Mat24, Mat3, Mat32, Mat34, Mat4, Mat42, Mat43:
		return 16
	default:
		panic("std140 alignment of attribute type: invalid type")
	}
}

// Std140Size returns the number of bytes a type occupies inside a std140 uniform
// block. This is NOT the same as its alignment: a vec3 aligns to 16 but occupies
// 12, and a mat3 aligns to 16 but occupies 48 (three columns padded to vec4).
// Conflating the two writes past the end of the Go value and over the member that
// follows it.
func (at AttrType) Std140Size() int {
	switch at {
	case Int, UInt, Float:
		return 4
	case Vec2, IVec2:
		return 8
	case Vec3:
		return 12
	case Vec4, IVec4:
		return 16
	case Mat2, Mat23, Mat24:
		return 2 * 16
	case Mat3, Mat32, Mat34:
		return 3 * 16
	case Mat4, Mat42, Mat43:
		return 4 * 16
	default:
		panic("std140 size of attribute type: invalid type")
	}
}

// Std140MatchesGoLayout reports whether a Go value of this type can be uploaded to
// a std140 block as one tightly packed block of bytes. False for the small matrix
// types, whose columns are padded to vec4 in the block but not in mgl32.
func (at AttrType) Std140MatchesGoLayout() bool {
	return at.Std140Size() == at.ArrayElements()*4
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
	case IVec4:
		return "ivec4"
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
