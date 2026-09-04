//go:build !darwin

package quads

const CanConstrainToWorkArea = false

// Only macOS draws a menu bar over a borderless fullscreen window. Elsewhere the
// window manager leaves it alone.
func hideSystemChrome() {}
func showSystemChrome() {}
