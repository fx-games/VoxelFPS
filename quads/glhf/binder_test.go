package glhf

import (
	"testing"

	"github.com/memmaker/voxelfps/platform/gl"
)

// resetBindings puts the shadow back to a known context with nothing bound, so a
// test never falls through to gl.GetIntegerv. There is no GL context here.
func resetBindings(locs ...uint32) {
	boundObjects = map[bindKey]uint32{}
	activeTextureUnit = gl.TEXTURE0
	for _, loc := range locs {
		boundObjects[bindKey{loc: loc}] = 0
	}
}

// newTestBinder records the binds it makes rather than issuing them.
func newTestBinder(loc, obj uint32, log *[]uint32) *binder {
	return &binder{
		restoreLoc: loc,
		obj:        obj,
		bindFunc:   func(o uint32) { *log = append(*log, o) },
	}
}

// The whole point of the shadow: a target that already holds the object is not
// bound again, and nothing asks the driver what is bound.
func TestBindSkipsTheBindWhenTheObjectIsAlreadyBound(t *testing.T) {
	resetBindings(gl.CURRENT_PROGRAM)
	var log []uint32
	shader := newTestBinder(gl.CURRENT_PROGRAM, 7, &log)

	shader.bind().restore()
	shader.bind().restore()

	want := []uint32{7, 0, 7, 0}
	if len(log) != len(want) {
		t.Fatalf("binds = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("binds = %v, want %v", log, want)
		}
	}
}

// A nested Begin of the same program must not re-bind, and End must leave the
// outer one bound.
func TestNestedBindOfTheSameObjectLeavesItBound(t *testing.T) {
	resetBindings(gl.CURRENT_PROGRAM)
	var log []uint32
	outer := newTestBinder(gl.CURRENT_PROGRAM, 7, &log)
	inner := newTestBinder(gl.CURRENT_PROGRAM, 7, &log)

	outer.bind()
	inner.bind()
	inner.restore()

	if len(log) != 1 || log[0] != 7 {
		t.Fatalf("binds = %v, want one bind of 7", log)
	}
	if got := currentBinding(gl.CURRENT_PROGRAM); got != 7 {
		t.Errorf("bound program = %d, want 7 still bound after the inner restore", got)
	}
}

// Texture bindings belong to a texture unit. The same texture bound on unit 1
// must not convince unit 0 that it holds it.
func TestTextureBindingsAreScopedToTheActiveUnit(t *testing.T) {
	resetBindings()
	boundObjects[bindKey{loc: gl.TEXTURE_BINDING_2D, scope: gl.TEXTURE0}] = 0
	boundObjects[bindKey{loc: gl.TEXTURE_BINDING_2D, scope: gl.TEXTURE1}] = 0
	var log []uint32
	atlas := newTestBinder(gl.TEXTURE_BINDING_2D, 3, &log)

	// Setting the shadow is all ActiveTexture does beyond the GL call itself,
	// and there is no context here to make that call against.
	activeTextureUnit = gl.TEXTURE1
	atlas.bind()

	activeTextureUnit = gl.TEXTURE0
	if got := currentBinding(gl.TEXTURE_BINDING_2D); got != 0 {
		t.Errorf("texture on unit 0 = %d, want 0 - unit 1 was the one that got bound", got)
	}

	activeTextureUnit = gl.TEXTURE1
	if got := currentBinding(gl.TEXTURE_BINDING_2D); got != 3 {
		t.Errorf("texture on unit 1 = %d, want 3", got)
	}
}

// glBindFramebuffer(GL_FRAMEBUFFER) sets the read and the draw binding both, so
// a Blit must not think the old ones are still there.
func TestBindingTheFramebufferBindsReadAndDraw(t *testing.T) {
	resetBindings(gl.FRAMEBUFFER_BINDING, gl.READ_FRAMEBUFFER_BINDING, gl.DRAW_FRAMEBUFFER_BINDING)
	var log []uint32
	frame := newTestBinder(gl.FRAMEBUFFER_BINDING, 4, &log)

	frame.bind()

	if got := currentBinding(gl.READ_FRAMEBUFFER_BINDING); got != 4 {
		t.Errorf("read framebuffer = %d, want 4", got)
	}
	if got := currentBinding(gl.DRAW_FRAMEBUFFER_BINDING); got != 4 {
		t.Errorf("draw framebuffer = %d, want 4", got)
	}
}

// The element array buffer binding lives in the vertex array object, so binding
// another VAO changes it without anyone calling glBindBuffer.
func TestElementArrayBindingIsScopedToTheVertexArray(t *testing.T) {
	resetBindings(gl.VERTEX_ARRAY_BINDING)
	// One entry per vertex array object, since that is what the binding hangs off.
	for _, vao := range []uint32{0, 1, 2} {
		boundObjects[bindKey{loc: gl.ELEMENT_ARRAY_BUFFER_BINDING, scope: vao}] = 0
	}
	var log []uint32
	firstVAO := newTestBinder(gl.VERTEX_ARRAY_BINDING, 1, &log)
	secondVAO := newTestBinder(gl.VERTEX_ARRAY_BINDING, 2, &log)
	indices := newTestBinder(gl.ELEMENT_ARRAY_BUFFER_BINDING, 9, &log)

	firstVAO.bind()
	indices.bind()

	secondVAO.bind()
	if got := currentBinding(gl.ELEMENT_ARRAY_BUFFER_BINDING); got != 0 {
		t.Errorf("element buffer under the second vertex array = %d, want 0", got)
	}

	secondVAO.restore()
	if got := currentBinding(gl.ELEMENT_ARRAY_BUFFER_BINDING); got != 9 {
		t.Errorf("element buffer back under the first vertex array = %d, want 9", got)
	}
}
