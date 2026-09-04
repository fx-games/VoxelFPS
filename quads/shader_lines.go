package quads

import (
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/quads/glhf"
)

const (
	LineShaderCamProjectionView = iota
	LineShaderModelTransform
	LineShaderDrawColor
)

func LoadLineShader(cam core.Camera) *glhf.Shader {
	var (
		vertexFormat = glhf.AttrFormat{
			{Name: "position", Type: glhf.Vec3},
		}
		uniformFormat = glhf.AttrFormat{
			glhf.Attr{Name: "camProjectionView", Type: glhf.Mat4},
			glhf.Attr{Name: "model", Type: glhf.Mat4},
			glhf.Attr{Name: "drawColor", Type: glhf.Vec3},
		}
		shader *glhf.Shader
	)
	var err error
	shader, err = glhf.NewBasicShader(vertexFormat, uniformFormat, lineVertexShaderSource, lineFragmentShaderSource)

	if err != nil {
		panic(err)
	}

	if errorMsg, isValid := shader.IsUniformsValid(); !isValid {
		panic(errorMsg)
	}
	shader.Begin()
	shader.SetUniformAttr(LineShaderCamProjectionView, cam.GetProjectionViewMatrix())
	model := mgl32.Ident4()
	shader.SetUniformAttr(LineShaderModelTransform, model)
	shader.SetUniformAttr(LineShaderDrawColor, mgl32.Vec3{0.8, 2, 0.8})
	shader.End()
	gl.Enable(gl.LINE_SMOOTH)
	gl.LineWidth(1)

	glErr := gl.GetError()
	if glErr != gl.NO_ERROR {
		panic(glErr)
	}

	return shader
}

func appendLine(rawVerts []glhf.GlFloat, start, end mgl32.Vec3) []glhf.GlFloat {
	rawVerts = append(rawVerts,
		glhf.GlFloat(start.X()), glhf.GlFloat(start.Y()), glhf.GlFloat(start.Z()),
		glhf.GlFloat(end.X()), glhf.GlFloat(end.Y()), glhf.GlFloat(end.Z()),
	)
	return rawVerts
}

func NewVertexSliceFromLines(lineShader *glhf.Shader, rawVerts []glhf.GlFloat) *glhf.VertexArray[glhf.GlFloat] {
	vertices := glhf.NewVertexArray[glhf.GlFloat](lineShader, false)
	vertices.DebugName = "LineVertexArray"
	vertices.SetPrimitiveType(gl.LINES)
	vertices.Begin()
	vertices.SetVertexData(rawVerts)
	vertices.End()
	return vertices
}

const lineVertexShaderSource = `#version 400 core

in vec3 position;

uniform mat4 camProjectionView;
uniform mat4 model;


void main() {
    gl_Position =  camProjectionView * model * vec4(position, 1.0);
}`

const lineFragmentShaderSource = `#version 400 core

out vec4 color;
uniform vec3 drawColor;
void main() {
    color = vec4(drawColor, 1.0);
}`
