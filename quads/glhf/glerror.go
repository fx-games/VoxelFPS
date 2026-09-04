//go:build !debugGL

package glhf

// CheckGLError is a no-op unless built with -tags debugGL. See glerror_debug.go.
func CheckGLError(context string) {}
