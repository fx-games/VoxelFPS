//go:build darwin

package quads

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void goraySetPresentationHidden(int hidden) {
    if (hidden) {
        // HideMenuBar is only legal together with HideDock.
        [NSApp setPresentationOptions:NSApplicationPresentationHideMenuBar | NSApplicationPresentationHideDock];
    } else {
        [NSApp setPresentationOptions:NSApplicationPresentationDefault];
    }
}
*/
import "C"

// CanConstrainToWorkArea reports whether this platform has system chrome that a
// borderless window would otherwise cover, which is what makes the choice worth
// offering in a menu.
const CanConstrainToWorkArea = true

// hideSystemChrome hides the macOS menu bar and Dock, so a borderless window
// covering the screen is not overdrawn by them. GLFW cannot do this: its Floating
// attribute puts the window at NSFloatingWindowLevel, which is still below the menu
// bar's own level, and the bar composites on top.
//
// Must be called on the main thread, which is where the GLFW window calls run.
func hideSystemChrome() { C.goraySetPresentationHidden(1) }

// showSystemChrome puts the menu bar and Dock back.
func showSystemChrome() { C.goraySetPresentationHidden(0) }
