//go:build debugGL

package glhf

import (
	"fmt"

	"github.com/memmaker/goray/platform/gl"
)

// CheckGLError panics on a pending GL error. Only compiled in with -tags debugGL:
// glGetError drains the driver's command queue, so calling it per draw or per
// uniform costs a full pipeline sync and is far too expensive for the hot path.
func CheckGLError(context string) {
	if err := gl.GetError(); err != gl.NO_ERROR {
		panic(fmt.Sprintf("GL error 0x%x at %s", err, context))
	}
}
