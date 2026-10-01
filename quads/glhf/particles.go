package glhf

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/platform/gl"
	"math"
	"math/rand"
)

type ParticleProperties struct {
	Origin, PositionVariation mgl32.Vec3
	VelocityVariation         mgl32.Vec3
	VelocityFromPosition      func(origin, pos mgl32.Vec3) mgl32.Vec3
	//RotationVariation                 float32
	ColorBegin, ColorEnd              mgl32.Vec3
	ColorVariation                    float32
	SizeBegin, SizeEnd, SizeVariation float32
	Lifetime                          float32
	MaxDistance                       float32
	SpreadLifetime                    float32
}

func (p ParticleProperties) WithOrigin(newPos mgl32.Vec3) ParticleProperties {
	p.Origin = newPos
	return p
}

func (p ParticleProperties) WithLifeTime(lifetime float32) ParticleProperties {
	p.Lifetime = lifetime
	return p
}

func (p ParticleProperties) WithSizeBegin(size float32) ParticleProperties {
	p.SizeBegin = size
	return p
}

func (p ParticleProperties) WithSizeEnd(size float32) ParticleProperties {
	p.SizeEnd = size
	return p
}

type Particle struct {
	id                   uint64
	Position, Velocity   mgl32.Vec3
	ColorBegin, ColorEnd mgl32.Vec3
	//Rotation               float32
	SizeBegin, SizeEnd     float32
	Lifetime, LifetimeLeft float32
	IsActive               bool
}

func (p Particle) GetID() uint64 {
	return p.id
}

type ParticleSystem struct {
	frontBuffer *VertexArray[GlFloat]
	backBuffer  *VertexArray[GlFloat]

	frontIsSource bool

	currentOffset           int
	activeVertexCount       int
	maxVertexCount          int
	transformFeedbackShader *Shader
	particleShader          *Shader
	primitiveType           uint32
	xfbo                    uint32

	lastParticleLifetime float32
	flatData             []GlFloat
	totalTime            float32
	isInfiteEmitter      bool
}

// idea: use transform feedback, so we have two buffers and can swap them
// We'll write from the transform vertex transformFeedbackShader to the first buffer, then read from the second buffer in the fragment transformFeedbackShader
// Then we'll swap the buffers and repeat

func NewParticleSystem(particleCount int) *ParticleSystem {
	particleVertexFormat := getParticleVertexFormat()
	v := &ParticleSystem{
		maxVertexCount:          particleCount,
		transformFeedbackShader: loadTransformFeedbackShader(particleVertexFormat),
		particleShader:          loadParticleShader(particleVertexFormat),
		frontIsSource:           false,
		flatData:                make([]GlFloat, particleCount*(particleVertexFormat.Size()/SizeOfFloat32)),
	}
	v.initializeBuffers(particleCount)
	return v
}

func (v *ParticleSystem) initializeBuffers(particleCount int) {
	var xfb uint32
	gl.GenTransformFeedbacks(1, &xfb)
	gl.BindTransformFeedback(gl.TRANSFORM_FEEDBACK, xfb)

	glError := gl.GetError()
	if glError != gl.NO_ERROR {
		println("BindTransformFeedback:", glError)
	}

	v.xfbo = xfb

	primitiveType := uint32(gl.POINTS)
	//vertexSize := v.transformFeedbackShader.VertexFormat().Size()
	//neededBufferSizeInBytes := vertexSize * particleCount
	//floatCount := neededBufferSizeInBytes / SizeOfFloat32

	v.primitiveType = primitiveType

	v.backBuffer = NewVertexArray[GlFloat](v.transformFeedbackShader, true)
	v.backBuffer.SetPrimitiveType(primitiveType)
	v.backBuffer.DebugName = "Particle_BackBuffer"

	v.frontBuffer = NewVertexArray[GlFloat](v.transformFeedbackShader, true)
	v.frontBuffer.SetPrimitiveType(primitiveType)
	v.frontBuffer.DebugName = "Particle_FrontBuffer"

	// Both buffers have to hold every particle from the start: the transform
	// feedback pass always reads maxVertexCount vertices, and a buffer sized to
	// whatever was emitted first would be read past its end. Zeroed particles
	// have no lifetime left, so they are discarded in the geometry shader.
	emptyParticles := make([]GlFloat, particleCount*(v.transformFeedbackShader.VertexFormat().Size()/SizeOfFloat32))
	for _, buffer := range []*VertexArray[GlFloat]{v.backBuffer, v.frontBuffer} {
		buffer.Begin()
		buffer.SetVertexData(emptyParticles)
		buffer.End()
	}

	glError = gl.GetError()
	if glError != gl.NO_ERROR {
		println("failed to initializeBuffers:", glError)
	}
}

func (v *ParticleSystem) currentBackBuffer() *VertexArray[GlFloat] {
	if v.frontIsSource {
		return v.backBuffer
	}
	return v.frontBuffer
}

func (v *ParticleSystem) currentFrontBuffer() *VertexArray[GlFloat] {
	if v.frontIsSource {
		return v.frontBuffer
	}
	return v.backBuffer
}

func (v *ParticleSystem) doTransfer(src, dest *VertexArray[GlFloat], deltaTime float64) {
	gl.BindTransformFeedback(gl.TRANSFORM_FEEDBACK, v.xfbo)

	gl.Enable(gl.RASTERIZER_DISCARD) // no need for fragment transformFeedbackShader, this is just between the vertex transformFeedbackShader and these two buffers

	v.transformFeedbackShader.Begin()
	v.transformFeedbackShader.SetUniformAttr(0, float32(deltaTime))
	src.Begin()

	gl.BindBufferBase(gl.TRANSFORM_FEEDBACK_BUFFER, 0, dest.vbo.obj)

	gl.BeginTransformFeedback(v.primitiveType)
	src.DrawWithOffset(0, int32(v.activeVertexCount))
	gl.EndTransformFeedback()

	gl.BindBufferBase(gl.TRANSFORM_FEEDBACK_BUFFER, 0, 0)

	//srcData := src.vertexData(0, 2)

	src.End()
	v.transformFeedbackShader.End()

	gl.Disable(gl.RASTERIZER_DISCARD) // TODO: CHECK IF THIS DOES WHAT I THINK IT DOES.. If so, why does gl.DrawBuffers(NONE) exist?

	//gl.Flush()
	/*
	   dest.begin()
	   dstData := dest.vertexData(0, 2)
	   dest.end()

	   for i := 0; i < len(srcData); i++ {
	       if srcData[i] != dstData[i] {
	           println("srcData[", i, "] != dstData[", i, "]")
	           println("srcData[", i, "] = ", srcData[i])
	           println("dstData[", i, "] = ", dstData[i])
	       }
	   }

	*/
}

// Draw reads the buffer that holds the current state, which is the one Emit also
// writes into: after Update's swap the transform feedback destination has become
// the source for the next frame.
func (v *ParticleSystem) Draw(cam core.Camera) {
	if v.activeVertexCount == 0 {
		return
	}
	v.draw(cam, v.currentBackBuffer())
}
func (v *ParticleSystem) Update(deltaTime float64) {
	v.totalTime += float32(deltaTime)
	if v.lastParticleLifetime <= 0.00 && !v.isInfiteEmitter {
		// Everything this system emitted has expired. Dropping the active count
		// here is what lets both the feedback pass and the draw be skipped
		// entirely: an idle system used to keep stepping and rasterising its full
		// buffer of dead particles every frame.
		v.activeVertexCount = 0
		v.currentOffset = 0
		return
	}
	v.lastParticleLifetime -= float32(deltaTime)

	// Swap after the transfer, not before. Swapping first made the buffer Emit had
	// just written into the transform feedback destination, so every particle
	// emitted this frame was overwritten before it was ever stepped or drawn.
	v.doTransfer(v.currentBackBuffer(), v.currentFrontBuffer(), deltaTime)
	v.frontIsSource = !v.frontIsSource
}

func (v *ParticleSystem) draw(cam core.Camera, drawBuffer *VertexArray[GlFloat]) {
	view := cam.GetViewMatrix()
	proj := cam.GetProjectionMatrix()

	v.particleShader.Begin()
	v.particleShader.SetUniformAttr(0, proj)
	modelMatrix := mgl32.Ident4()
	modelView := view.Mul4(modelMatrix)
	v.particleShader.SetUniformAttr(1, modelView)

	drawBuffer.Begin()
	// DrawTransformFeedback, which would take the vertex count from the feedback
	// object, raises GL_INVALID_OPERATION on this driver and draws nothing. Every
	// slot up to activeVertexCount has been written (dead particles are zeroed and
	// the geometry shader discards them), so a plain DrawArrays over that range is
	// equivalent.
	gl.DrawArrays(gl.POINTS, 0, int32(v.activeVertexCount))
	drawBuffer.End()

	v.particleShader.End()
}
func (v *ParticleSystem) Emit(props ParticleProperties, count int) int {
	if props.Lifetime > v.lastParticleLifetime {
		v.lastParticleLifetime = props.Lifetime
	} else if props.Lifetime < 0 {
		v.isInfiteEmitter = true
	}
	vertexOffset := v.currentOffset
	if count >= v.maxVertexCount {
		count = v.maxVertexCount
		vertexOffset = 0
	}

	buffer := v.currentBackBuffer()
	flatStride := v.particleShader.VertexFormat().Size() / SizeOfFloat32 // distance between two particles in a list of GlFloats == number of floats per particle/vertex

	flatOffset := vertexOffset * flatStride
	flatCount := count * flatStride
	for index := 0; index < count; index++ {
		flatIndex := (flatOffset + index*flatStride) % len(v.flatData)
		v.writeParticle(v.flatData[flatIndex:flatIndex+flatStride], props, index)
	}
	buffer.Begin()
	if vertexOffset+count > v.maxVertexCount {
		flatSpaceAtTheEnd := (v.maxVertexCount - vertexOffset) * flatStride
		flatRemainingSpace := flatCount - flatSpaceAtTheEnd
		buffer.setVertexDataWithOffset(vertexOffset, v.flatData[flatOffset:flatOffset+flatSpaceAtTheEnd])
		buffer.setVertexDataWithOffset(0, v.flatData[0:flatRemainingSpace])
	} else {
		buffer.setVertexDataWithOffset(vertexOffset, v.flatData[flatOffset:flatOffset+flatCount])
	}
	buffer.End()

	// hmm3: we will overwrite the properties of existing particles being emitted the moment here..
	v.particleShader.Begin()
	v.particleShader.SetUniformAttr(2, props.Lifetime)
	v.particleShader.SetUniformAttr(3, props.ColorEnd)
	v.particleShader.SetUniformAttr(4, props.SizeEnd)
	v.particleShader.End()

	v.transformFeedbackShader.Begin()
	v.transformFeedbackShader.SetUniformAttr(1, props.MaxDistance)
	v.transformFeedbackShader.SetUniformAttr(2, props.Lifetime)
	v.transformFeedbackShader.End()

	if vertexOffset+count > v.activeVertexCount {
		v.activeVertexCount = vertexOffset + count
	}
	if v.activeVertexCount > v.maxVertexCount {
		v.activeVertexCount = v.maxVertexCount
	}
	v.currentOffset = (vertexOffset + count) % v.maxVertexCount

	return vertexOffset
}

// writeParticle fills one vertex worth of flatData in place. It used to return a
// freshly allocated slice that the caller copied out of, which put one heap
// allocation per particle on the emit path - a few hundred per burst, all garbage
// before the frame was over.
func (v *ParticleSystem) writeParticle(dst []GlFloat, props ParticleProperties, index int) {
	x := props.Origin.X() + props.PositionVariation.X()*(rand.Float32()-0.5)
	y := props.Origin.Y() + props.PositionVariation.Y()*(rand.Float32()-0.5)
	z := props.Origin.Z() + props.PositionVariation.Z()*(rand.Float32()-0.5)
	velocity := mgl32.Vec3{}
	if props.VelocityFromPosition != nil {
		velocity = props.VelocityFromPosition(props.Origin, mgl32.Vec3{x, y, z})
	}
	velocityX := GlFloat(velocity.X() + props.VelocityVariation.X()*(rand.Float32()-0.5))
	velocityY := GlFloat(velocity.Y() + props.VelocityVariation.Y()*(rand.Float32()-0.5))
	velocityZ := GlFloat(velocity.Z() + props.VelocityVariation.Z()*(rand.Float32()-0.5))
	colorVariation := props.ColorVariation * (rand.Float32() - 0.5)
	lifeTimeAbs := math.Abs(float64(props.Lifetime))
	if props.SpreadLifetime > 0.0 {
		lifeTimeAbs = lifeTimeAbs - (float64(index) * float64(props.SpreadLifetime))
	}
	lifetimeLeft := GlFloat(lifeTimeAbs)

	// position x,y,z
	dst[0], dst[1], dst[2] = GlFloat(x), GlFloat(y), GlFloat(z)
	// lifetime left
	dst[3] = lifetimeLeft
	// velocity x,y,z
	dst[4], dst[5], dst[6] = velocityX, velocityY, velocityZ
	// size begin
	dst[7] = GlFloat(props.SizeBegin + props.SizeVariation*(rand.Float32()-0.5))
	// color begin r,g,b
	dst[8] = GlFloat(mgl32.Clamp(props.ColorBegin.X()+colorVariation, 0, 1))
	dst[9] = GlFloat(mgl32.Clamp(props.ColorBegin.Y()+colorVariation, 0, 1))
	dst[10] = GlFloat(mgl32.Clamp(props.ColorBegin.Z()+colorVariation, 0, 1))
	// origin x,y,z: the particle's own spawn point, not the emitter's, so
	// MaxDistance measures how far it personally drifted
	dst[11], dst[12], dst[13] = GlFloat(x), GlFloat(y), GlFloat(z)
}

func (v *ParticleSystem) Clear(vertexOffset int, vertexCount int) {
	// we want to write a zero to the lifetime of each particle
	//buffer := v.currentBackBuffer()
	flatStride := v.particleShader.VertexFormat().Size() / SizeOfFloat32 // distance between two particles in a list of GlFloats == number of floats per particle/vertex
	flatOffset := vertexOffset * flatStride
	flatCount := vertexCount * flatStride
	for index := 0; index < vertexCount; index++ {
		flatIndex := (flatOffset + index*flatStride) % len(v.flatData)
		for i := 0; i < flatStride; i++ {
			v.flatData[flatIndex+i] = 0
		}
	}
	for _, buffer := range []*VertexArray[GlFloat]{v.frontBuffer, v.backBuffer} {
		buffer.Begin()
		if vertexOffset+vertexCount > v.maxVertexCount {
			flatSpaceAtTheEnd := (v.maxVertexCount - vertexOffset) * flatStride
			flatRemainingSpace := flatCount - flatSpaceAtTheEnd
			buffer.setVertexDataWithOffset(vertexOffset, v.flatData[flatOffset:flatOffset+flatSpaceAtTheEnd])
			buffer.setVertexDataWithOffset(0, v.flatData[0:flatRemainingSpace])
		} else {
			buffer.setVertexDataWithOffset(vertexOffset, v.flatData[flatOffset:flatOffset+flatCount])
		}
		buffer.End()
	}
}
