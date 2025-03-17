package main

import (
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
)

type InputReceiver interface {
    HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool)
    HandleMousePosEvents(xpos float64, ypos float64) (consumed bool)
    HandleMouseButtonEvents(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool)
    HandleScrollEvents(xoff float64, yoff float64) (consumed bool)
    Draw()
    ShouldClose() bool
    HandleDrop(names []string, x float64, y float64) bool
}

func (g *Game) pollInput(elapsed float64) {
    if g.focusedWidget != nil && g.focusedIsBlocking {
        return
    }
    moveDir := [2]int{0, 0}
    upDown := int(0)
    inputSrc := g.GlApplication.Window
    if inputSrc.GetKey(glfw.KeyW) == glfw.Press {
        moveDir[1] = 1
    } else if inputSrc.GetKey(glfw.KeyS) == glfw.Press {
        moveDir[1] = -1
    }

    if inputSrc.GetKey(glfw.KeyA) == glfw.Press {
        moveDir[0] = -1
    } else if inputSrc.GetKey(glfw.KeyD) == glfw.Press {
        moveDir[0] = 1
    }

    if inputSrc.GetKey(glfw.KeyC) == glfw.Press {
        upDown = -1
    } else if inputSrc.GetKey(glfw.KeySpace) == glfw.Press {
        upDown = 1
    }

    var velocityChange = mgl32.Vec3{0, -9, 0}
    if upDown > 0 {
        jumpVelocity := mgl32.Vec3{0, 1, 0}.Mul(14)
        velocityChange = velocityChange.Add(jumpVelocity)
    }

    if moveDir[0] != 0 || moveDir[1] != 0 {
        moveSpeed := 7.0
        //f.player.SetVelocity(mgl32.Vec3{float32(float32(elapsed) * float32(-moveDir[0]) * float32(moveSpeed)), 0, float32(float32(elapsed) * float32(moveDir[1]) * float32(moveSpeed))})
        newPos := g.Camera.GetDesiredPosition(float32(moveSpeed), moveDir)
        moveVelocity := newPos.Sub(g.Camera.GetPosition()).Normalize().Mul(float32(moveSpeed))
        velocityChange = velocityChange.Add(moveVelocity)
        //newPosWithCorrection := quads.CollideWithMap(mgl32.Vec2{newPos.X(), newPos.Z()}, 0.2, f.IsWallAt)
        //f.camera.SetPosition(mgl32.Vec3{newPosWithCorrection.X(), newPos.Y(), newPosWithCorrection.Y()})
    }
    g.player.Physics().SetVelocity(velocityChange)
}

func (g *Game) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
    if g.focusedWidget != nil {
        if g.focusedWidget.HandleKeyEvents(key, scancode, action, mods) || g.focusedIsBlocking {
            return
        }
    }

    if action != glfw.Repeat && action != glfw.Press {
        return
    }
    if key == glfw.KeyEscape {
        g.Window.SetShouldClose(true)
    }
    if key == glfw.KeyF1 {
        g.openWizardMenu()
    }

    if key == glfw.KeyF2 {
        g.toggleMouseCapture()
    }

    if key == glfw.KeyF3 {
        g.toggleSpawnMode()
    }
    /*
       if key == glfw.KeyF2 {

           // enable
           gl.Enable(gl.CULL_FACE)
           gl.CullFace(gl.BACK)
           // disable
           gl.Disable(gl.CULL_FACE)
       }

    */

    if key == glfw.KeyF4 {
        g.Camera.SetFOV(g.Camera.GetFOV() + 5)
        g.Camera.LookSensitivity += 0.01
    }
    if key == glfw.KeyF5 {
        g.Camera.SetFOV(g.Camera.GetFOV() - 5)
        g.Camera.LookSensitivity -= 0.01
    }

    if key == glfw.KeyF7 {
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
    }
    if key == glfw.KeyF8 {
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
    }

    if key == glfw.KeyL {
        g.uiList.SelectNext()
    }
    if key == glfw.KeyK {
        g.uiList.SelectPrevious()
    }

    if key == glfw.KeyO {
        g.uiList.ScrollUp()
    }
    if key == glfw.KeyP {
        g.uiList.ScrollDown()
    }
}

func (g *Game) HandleMousePosEvents(xpos float64, ypos float64) {
    if g.focusedWidget != nil {
        if g.focusedWidget.HandleMousePosEvents(xpos, ypos) || g.focusedIsBlocking {
            return
        }
    }
    oldX := g.lastMouseX
    oldY := g.lastMouseY
    g.lastMouseX = xpos
    g.lastMouseY = ypos

    if !g.isMouseCursorCaptured {
        return
    }

    dx := xpos - oldX
    dy := ypos - oldY

    g.Camera.ChangeAngles(float32(dx), float32(dy))
}

func (g *Game) HandleMouseButtonEvents(button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
    if g.focusedWidget != nil {
        if g.focusedWidget.HandleMouseButtonEvents(button, action, mods) || g.focusedIsBlocking {
            return
        }
    }

    if action == glfw.Press && button == glfw.MouseButtonLeft {
        g.startWeaponFireAnimation()
    }
}

func (g *Game) HandleScrollEvents(xoff float64, yoff float64) {
    if g.focusedWidget != nil {
        if g.focusedWidget.HandleScrollEvents(xoff, yoff) || g.focusedIsBlocking {
            return
        }
    }
}
