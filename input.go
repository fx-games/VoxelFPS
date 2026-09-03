package main

import (
    "fmt"
    "github.com/go-gl/gl/v3.3-core/gl"
    "github.com/go-gl/glfw/v3.3/glfw"
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/events"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/voxel"
    "maps"
)

type InputReceiver interface {
    HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) (consumed bool)
    HandleMousePosEvents(xpos float64, ypos float64) (consumed bool)
    HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) (consumed bool)
    HandleScrollEvents(xoff float64, yoff float64) (consumed bool)
    Draw()
    ShouldClose() bool
    HandleDrop(names []string, x float64, y float64) bool
    HandleResize(screenWidth int, screenHeight int, screenCoordScale mgl32.Vec2)
}

func (g *Game) pollInput(elapsed float64) {
    if g.widgetsAreBlockingMovement {
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

    const moveSpeed = 7.0
    const jumpSpeed = 8.0

    var walkVelocity mgl32.Vec3
    if moveDir[0] != 0 || moveDir[1] != 0 {
        desiredPos := g.Camera.GetDesiredPosition(moveSpeed, moveDir)
        walkVelocity = desiredPos.Sub(g.Camera.GetPosition()).Normalize().Mul(moveSpeed)
    }

    physics := g.player.Physics()
    if !g.gravitationEnabled {
        // Free flight: space and C move straight up and down.
        g.player.Physics().SetGrounded(false)
        physics.SetVelocity(walkVelocity.Add(mgl32.Vec3{0, float32(upDown) * moveSpeed, 0}))
        return
    }

    // Gravity does the vertical part, so only the horizontal velocity is
    // replaced. Jumping needs something to push off from.
    fallVelocity := physics.Velocity().Y()
    if upDown > 0 && physics.IsGrounded() {
        fallVelocity = jumpSpeed
    }
    physics.SetVelocity(mgl32.Vec3{walkVelocity.X(), fallVelocity, walkVelocity.Z()})
}

func (g *Game) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
    if len(g.widgetStack) > 0 {
        if g.focusedWidget().HandleKeyEvents(key, scancode, action, mods) || g.widgetsAreBlockingMovement {
            return
        }
    }
    if g.entityMenu != nil && g.entityMenu.IsVisible() && g.entityMenu.HandleKeyEvents(key, scancode, action, mods) {
        return
    }
    if action != glfw.Repeat && action != glfw.Press {
        return
    }

    if key == glfw.KeyTab && g.voxelBlock != nil && g.lastHitResult != nil && g.lastHitResult.HitInfo.Hit {
        rayHitBlock := g.lastHitResult.HitInfo.CollisionGridPosition
        hitBlock := g.CubeMap.GetGlobalBlock(rayHitBlock.X, rayHitBlock.Y, rayHitBlock.Z)
        g.voxelBlock.Textures = maps.Clone(hitBlock.Textures)
        g.Toast(fmt.Sprintf("Block textures copied from %v", rayHitBlock))
    }

    if key >= glfw.Key1 && key <= glfw.Key9 {
        g.selectWeapon(int(key - glfw.Key1))
    }

    if key == glfw.KeyEscape {
        g.openEscapeMenu()
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
    if key == glfw.KeyF4 {
        g.changeDecorationTexture()
    }

    if key == glfw.KeyF5 {
        g.startVoxelEditing()

    }

    if key == glfw.KeyF6 {
        g.openCubeTextureMapper()
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

    if key == glfw.KeyF9 {
        g.Camera.SetFOV(g.Camera.GetFOV() + 5)
        g.Camera.LookSensitivity += 0.01
    }
    if key == glfw.KeyF10 {
        g.Camera.SetFOV(g.Camera.GetFOV() - 5)
        g.Camera.LookSensitivity -= 0.01
    }

    if key == glfw.KeyF7 {
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
    }
    if key == glfw.KeyF8 {
        gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
    }

    if key == glfw.KeyF11 {
        g.testPackedAtlasPicker()
    }

    if key == glfw.KeyF12 {
        g.SaveScreenshot("screenshot00.png")
        //g.SaveShadowMaps("./temp")
    }

    if key == glfw.KeyF {
        g.gravitationEnabled = !g.gravitationEnabled
        g.physicsSystem.SetGravityEnabled(g.gravitationEnabled)
        g.Toast(fmt.Sprintf("Gravity %s", map[bool]string{true: "on", false: "off"}[g.gravitationEnabled]))
    }
    if key == glfw.KeyE && g.isHoveringObject() {
        events.Publish(events.EventPlayerInteractWith, g.hoveredObj())
    }
    if key == glfw.KeyV {
        g.spawnCurrentGameObject()
    }

    if key == glfw.KeyR {
        g.openVoxelEditMenu()
    }

    if key == glfw.KeyL {
        g.entityMenu.SelectNext()
    }
    if key == glfw.KeyK {
        g.entityMenu.SelectPrevious()
    }

    if key == glfw.KeyO {
        g.entityMenu.ScrollUp()
    }
    if key == glfw.KeyP {
        g.entityMenu.ScrollDown()
    }
}

func (g *Game) spawnCurrentGameObject() {
    if g.gameObjectForSpawning != nil {
        decoToSpawn := g.gameObjectForSpawning
        sprite := decoToSpawn.Sprite()
        g.Sprites.Remove(sprite)
        sprite.Tint = mgl32.Vec4{1, 1, 1, 1}
        g.Sprites.Add(sprite)

        aabb := voxel.NewAABB(sprite.AtlasTexture.AABBFromOpaqueAtIndex(int(sprite.TextureIndices[0])))
        physicsComponent := quads.NewPhysicsComponent(false, aabb)
        decoToSpawn.SetComponent(physicsComponent)
        g.physicsSystem.AddObject(physicsComponent)

        g.gameObjectForSpawning = g.spawnEntity(decoToSpawn.Transform().GetPosition(), core.MapEntity{
            Name:           decoToSpawn.DisplayName,
            TextureIndices: sprite.TextureIndices,
            CreateCollider: false,
            IsTransparent:  true,
        })
        g.gameObjectForSpawning.Sprite().Tint = mgl32.Vec4{1, 1, 1, 0.5}
    }
}

func (g *Game) quitGame() {
    g.Window.SetShouldClose(true)
}

func (g *Game) HandleMousePosEvents(xpos float64, ypos float64) {
    if len(g.widgetStack) > 0 {
        if g.focusedWidget().HandleMousePosEvents(xpos, ypos) || g.widgetsAreBlockingMovement {
            return
        }
    }
    oldX := g.lastMouseX
    oldY := g.lastMouseY
    g.lastMouseX = xpos
    g.lastMouseY = ypos

    if !g.isMouseCursorCaptured {
        if g.isMouseBeingDragged {
            g.HandleDragMove(xpos, ypos)
        } else {
            g.HandleMouseMove(xpos, ypos)
        }
        return
    }

    dx := xpos - oldX
    dy := ypos - oldY

    g.Camera.ChangeAngles(float32(dx), float32(dy))
}

func (g *Game) HandleScrollEvents(xoff float64, yoff float64) {
    if len(g.widgetStack) > 0 {
        if g.focusedWidget().HandleScrollEvents(xoff, yoff) || g.widgetsAreBlockingMovement {
            return
        }
    }
    if g.entityMenu != nil && g.entityMenu.IsVisible() {
        g.entityMenu.HandleScrollEvents(xoff, yoff)
    }

}
func (g *Game) HandleMouseButtonEvents(xpos float64, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
    if len(g.widgetStack) > 0 {
        if g.focusedWidget().HandleMouseButtonEvents(xpos, ypos, button, action, mods) || g.widgetsAreBlockingMovement {
            return
        }
    }

    if button == glfw.MouseButtonRight && action == glfw.Press {
        g.toggleMouseCapture()
        return
    }

    if g.gameObjectForSpawning != nil && button == glfw.MouseButtonLeft && action == glfw.Press {
        g.spawnCurrentGameObject()
        return
    }

    if g.isMouseCursorCaptured {
        if action == glfw.Press && button == glfw.MouseButtonLeft {
            g.startWeaponFireAnimation()
        }
    } else if button == glfw.MouseButtonLeft {
        switch action {
        case glfw.Press:
            g.isMouseBeingDragged = true
            g.HandleDragStart(g.Window.GetCursorPos())
        case glfw.Release:
            if g.isMouseBeingDragged {
                g.HandleDragStop(g.Window.GetCursorPos())
                g.isMouseBeingDragged = false
            }
        }
    }
}

// brushTarget is the voxel the brush is pointing at: the solid one under the
// cursor, or the empty one in front of it when placing. Reports false when the
// ray hit nothing, so the brush does not act on a bogus grid position.
func (g *Game) brushTarget(x, y float64) (voxel.Int3, bool) {
    if g.voxelBrush == nil {
        return voxel.Int3{}, false
    }
    result := g.rayCastFromScreenPos(x, y)
    if !result.HitInfo.Hit {
        return voxel.Int3{}, false
    }
    if g.voxelSelectionEmpty {
        return result.HitInfo.PreviousGridPosition, true
    }
    return result.HitInfo.CollisionGridPosition, true
}

func (g *Game) HandleMouseMove(x float64, y float64) {
    if blockHit, hit := g.brushTarget(x, y); hit {
        g.voxelSelection = g.voxelBrush.StartDrag(blockHit)
    }
}

func (g *Game) HandleDragStart(x float64, y float64) {
    if blockHit, hit := g.brushTarget(x, y); hit {
        g.voxelSelection = g.voxelBrush.StartDrag(blockHit)
    }
}

func (g *Game) HandleDragMove(x float64, y float64) {
    if blockHit, hit := g.brushTarget(x, y); hit {
        g.voxelSelection = g.voxelBrush.Drag(blockHit)
    }
}

func (g *Game) HandleDragStop(x float64, y float64) {
    if g.voxelBrush == nil {
        return
    }
    // A drag that ends off the map keeps the selection it had, but still commits.
    if blockHit, hit := g.brushTarget(x, y); hit {
        g.voxelSelection = g.voxelBrush.Drag(blockHit)
    }
    if g.voxelSelectionAction != nil {
        g.voxelSelectionAction(g.voxelSelection)
    }
}

func (g *Game) captureMouse(capture bool) {
    g.SetMouseCapture(capture)
    g.isMouseCursorCaptured = capture
}
