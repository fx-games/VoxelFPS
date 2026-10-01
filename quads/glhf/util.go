package glhf

import "github.com/memmaker/goray/platform/gl"

// bindKey names one GL binding slot. Most targets have exactly one slot per
// context, but a texture binding belongs to a texture unit and the element array
// buffer binding belongs to a vertex array object, so those two need the scope
// they live in as well.
type bindKey struct {
	loc   uint32
	scope uint32
}

// boundObjects remembers what glhf has bound, so bind() no longer has to ask the
// driver. It used to call gl.GetIntegerv on every single Begin() in the engine,
// and a state query is a synchronous round trip that costs far more than the bind
// it was guarding. A miss still asks once, which covers whatever the context
// happened to start with.
//
// This is only correct while every bind goes through glhf. ActiveTexture,
// BindDefaultFramebuffer and the alias handling in recordBind are what keep that
// true; a raw gl.Bind* call somewhere else would leave a stale entry behind.
var boundObjects = map[bindKey]uint32{}

// activeTextureUnit shadows glActiveTexture. GL starts on unit 0.
var activeTextureUnit uint32 = gl.TEXTURE0

// ActiveTexture selects the texture unit that later texture binds apply to. Call
// this rather than gl.ActiveTexture, so the shadow above knows which unit a
// texture binding belongs to.
func ActiveTexture(unit uint32) {
	activeTextureUnit = unit
	gl.ActiveTexture(unit)
}

// BindDefaultFramebuffer draws to the screen again. Call this rather than
// gl.BindFramebuffer, for the same reason as ActiveTexture.
func BindDefaultFramebuffer() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	recordBind(gl.FRAMEBUFFER_BINDING, 0)
}

// scopeOf answers which slot of a target a binding lands in.
func scopeOf(loc uint32) uint32 {
	switch loc {
	case gl.TEXTURE_BINDING_2D, gl.TEXTURE_BINDING_2D_ARRAY, gl.TEXTURE_BINDING_3D,
		gl.TEXTURE_BINDING_CUBE_MAP, gl.TEXTURE_BINDING_CUBE_MAP_ARRAY,
		gl.TEXTURE_BINDING_BUFFER, gl.TEXTURE_BINDING_RECTANGLE:
		return activeTextureUnit
	case gl.ELEMENT_ARRAY_BUFFER_BINDING:
		// The element array binding is vertex array object state, not context
		// state, so binding another VAO silently changes it.
		return currentBinding(gl.VERTEX_ARRAY_BINDING)
	}
	return 0
}

// currentBinding reports what is bound to a target, asking the driver only the
// first time a slot is seen.
func currentBinding(loc uint32) uint32 {
	key := bindKey{loc: loc, scope: scopeOf(loc)}
	if obj, known := boundObjects[key]; known {
		return obj
	}
	var obj int32
	gl.GetIntegerv(loc, &obj)
	boundObjects[key] = uint32(obj)
	return uint32(obj)
}

// recordBind notes an object as bound, including the slots GL changes alongside
// the one that was asked for.
func recordBind(loc, obj uint32) {
	boundObjects[bindKey{loc: loc, scope: scopeOf(loc)}] = obj
	if loc == gl.FRAMEBUFFER_BINDING {
		// Binding GL_FRAMEBUFFER binds the read and the draw framebuffer both.
		boundObjects[bindKey{loc: gl.READ_FRAMEBUFFER_BINDING}] = obj
		boundObjects[bindKey{loc: gl.DRAW_FRAMEBUFFER_BINDING}] = obj
	}
}

type binder struct {
	restoreLoc uint32
	bindFunc   func(uint32)

	obj uint32

	prev []uint32
}

func (b *binder) bind() *binder {
	prev := currentBinding(b.restoreLoc)
	b.prev = append(b.prev, prev)

	if prev != b.obj {
		b.bindFunc(b.obj)
		recordBind(b.restoreLoc, b.obj)
	}
	return b
}
func (b *binder) bindOnly() *binder {
	b.prev = append(b.prev, 0)
	if b.obj != 0 {
		b.bindFunc(b.obj)
		recordBind(b.restoreLoc, b.obj)
	}
	return b
}
func (b *binder) restore() *binder {
	prev := b.prev[len(b.prev)-1]
	b.prev = b.prev[:len(b.prev)-1]

	if prev != b.obj {
		b.bindFunc(prev)
		recordBind(b.restoreLoc, prev)
	}
	return b
}
