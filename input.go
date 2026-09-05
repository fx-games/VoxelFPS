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

	physics := g.player.Physics()

	if !g.gravitationEnabled {
		// Free flight: no Quake physics, space and C move straight up and down.
		const flySpeed = 7.0
		var flyVelocity mgl32.Vec3
		if moveDir[0] != 0 || moveDir[1] != 0 {
			desiredPos := g.Camera.GetDesiredPosition(flySpeed, moveDir)
			flyVelocity = desiredPos.Sub(g.Camera.GetPosition()).Normalize().Mul(flySpeed)
		}
		physics.SetGrounded(false)
		physics.SetVelocity(flyVelocity.Add(mgl32.Vec3{0, float32(upDown) * flySpeed, 0}))
		return
	}

	// The direction the player is asking to move in, flattened: looking up must
	// not slow you down.
	var wishDir mgl32.Vec3
	if moveDir[0] != 0 || moveDir[1] != 0 {
		desired := g.Camera.GetDesiredPosition(1, moveDir).Sub(g.Camera.GetPosition())
		wishDir = mgl32.Vec3{desired.X(), 0, desired.Z()}
		if wishDir.Len() > 0 {
			wishDir = wishDir.Normalize()
		}
	}

	velocity := physics.Velocity()
	if g.move.CheckJump(upDown > 0, physics.IsGrounded()) {
		velocity = mgl32.Vec3{velocity.X(), pmJumpVelocity, velocity.Z()}
		// The jump has already left the ground, the next physics step only
		// finds out afterwards.
		physics.SetGrounded(false)
	}
	physics.SetVelocity(g.move.Advance(velocity, wishDir, physics.IsGrounded(), elapsed))
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

	if key == glfw.KeyG && g.lastHitResult != nil && g.lastHitResult.HitInfo.Hit {
		hit := g.lastHitResult.HitInfo
		if textureIndex, nowGlowing, ok := g.ToggleEmissiveTextureAt(hit.CollisionGridPosition, hit.Side); ok {
			g.Toast(fmt.Sprintf("Texture %d %s", textureIndex, map[bool]string{true: "glowing", false: "back to normal"}[nowGlowing]))
		}
	}

	if key == glfw.KeyTab && g.editor.voxelBlock != nil && g.lastHitResult != nil && g.lastHitResult.HitInfo.Hit {
		rayHitBlock := g.lastHitResult.HitInfo.CollisionGridPosition
		hitBlock := g.CubeMap.GetGlobalBlock(rayHitBlock.X, rayHitBlock.Y, rayHitBlock.Z)
		g.editor.voxelBlock.Textures = maps.Clone(hitBlock.Textures)
		g.Toast(fmt.Sprintf("Block textures copied from %v", rayHitBlock))
	}

	if key >= glfw.Key1 && key <= glfw.Key9 {
		// The binding is the weapons folder's, not ours.
		g.selectWeaponByKey(int(key-glfw.Key1) + 1)
	}

	// Reload is a press the weapon's script may or may not act on: a gun with
	// no Reload state, like a pump shotgun that works itself, ignores it.
	if key == glfw.KeyR && g.isMouseCursorCaptured {
		g.reloadPressed = true
	}

	// Nine number keys against a hundred and forty migrated weapons, so , and .
	// walk through the whole folder.
	if key == glfw.KeyComma {
		g.cycleWeapon(-1)
	}
	if key == glfw.KeyPeriod {
		g.cycleWeapon(1)
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
	if key == glfw.KeyT {
		// One key, one slot: off -> flashlight -> torch -> off. The player carries a
		// single light, so picking one puts the other away.
		g.Toast(fmt.Sprintf("Player light: %s", g.CyclePlayerLight()))
	}
	if key == glfw.KeyE && g.isHoveringObject() {
		events.Publish(events.EventPlayerInteractWith, g.hoveredObj())
	}
	if key == glfw.KeyV {
		g.spawnCurrentGameObject()
	}

	if key == glfw.KeyR && !g.isMouseCursorCaptured {
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
	if g.editor.gameObjectForSpawning != nil {
		decoToSpawn := g.editor.gameObjectForSpawning
		if !decoToSpawn.HasComponent(quads.ComponentTypeSprite) {
			// A mesh ghost - a torch. It is already the object it will be, so it
			// only has to stop being the ghost: its light goes into the map and a
			// fresh ghost takes over the crosshair.
			g.dropTorch(decoToSpawn.Transform().GetPosition())
			g.editor.gameObjectForSpawning = g.spawnTorch(decoToSpawn.Transform().GetPosition())
			return
		}
		sprite := decoToSpawn.Sprite()
		g.Sprites.Remove(sprite)
		sprite.Tint = mgl32.Vec4{1, 1, 1, 1}
		g.Sprites.Add(sprite)

		aabb := voxel.NewAABB(sprite.AtlasTexture.AABBFromOpaqueAtIndex(int(sprite.TextureIndices[0])))
		physicsComponent := quads.NewPhysicsComponent(false, aabb)
		decoToSpawn.SetComponent(physicsComponent)
		g.physicsSystem.AddObject(physicsComponent)

		g.editor.gameObjectForSpawning = g.spawnEntity(decoToSpawn.Transform().GetPosition(), core.MapEntity{
			Name:           decoToSpawn.DisplayName,
			TextureIndices: sprite.TextureIndices,
			CreateCollider: false,
			IsTransparent:  true,
		})
		g.editor.gameObjectForSpawning.Sprite().Tint = mgl32.Vec4{1, 1, 1, 0.5}
	}
}

func (g *Game) quitGame() {
	g.Window.SetShouldClose(true)
}

func (g *Game) HandleMousePosEvents(xpos float64, ypos float64) {
	if len(g.widgetStack) > 0 {
		// Widgets are laid out in GUI coordinates, so hit testing has to be too.
		guiX, guiY := g.GuiCoords(xpos, ypos)
		if g.focusedWidget().HandleMousePosEvents(guiX, guiY) || g.widgetsAreBlockingMovement {
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
		guiX, guiY := g.GuiCoords(xpos, ypos)
		if g.focusedWidget().HandleMouseButtonEvents(guiX, guiY, button, action, mods) || g.widgetsAreBlockingMovement {
			return
		}
	}

	if button == glfw.MouseButtonRight && action == glfw.Press {
		g.toggleMouseCapture()
		return
	}

	if g.editor.gameObjectForSpawning != nil && button == glfw.MouseButtonLeft && action == glfw.Press {
		g.spawnCurrentGameObject()
		return
	}

	if g.isMouseCursorCaptured {
		if button != glfw.MouseButtonLeft {
			return
		}
		if g.editor.tool != EditorToolNone {
			if action == glfw.Press {
				g.applyEditorTool()
			}
			return
		}
		// The trigger is held, not pulled: a weapon's script decides what a
		// held trigger does, and several of them keep firing while it is down.
		g.triggerHeld = action == glfw.Press
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
	if g.editor.voxelBrush == nil {
		return voxel.Int3{}, false
	}
	result := g.rayCastFromScreenPos(x, y)
	if !result.HitInfo.Hit {
		return voxel.Int3{}, false
	}
	if g.editor.voxelSelectionEmpty {
		return result.HitInfo.PreviousGridPosition, true
	}
	return result.HitInfo.CollisionGridPosition, true
}

func (g *Game) HandleMouseMove(x float64, y float64) {
	if blockHit, hit := g.brushTarget(x, y); hit {
		g.editor.voxelSelection = g.editor.voxelBrush.StartDrag(blockHit)
	}
}

func (g *Game) HandleDragStart(x float64, y float64) {
	if blockHit, hit := g.brushTarget(x, y); hit {
		g.editor.voxelSelection = g.editor.voxelBrush.StartDrag(blockHit)
	}
}

func (g *Game) HandleDragMove(x float64, y float64) {
	if blockHit, hit := g.brushTarget(x, y); hit {
		g.editor.voxelSelection = g.editor.voxelBrush.Drag(blockHit)
	}
}

func (g *Game) HandleDragStop(x float64, y float64) {
	if g.editor.voxelBrush == nil {
		return
	}
	// A drag that ends off the map keeps the selection it had, but still commits.
	if blockHit, hit := g.brushTarget(x, y); hit {
		g.editor.voxelSelection = g.editor.voxelBrush.Drag(blockHit)
	}
	if g.editor.voxelSelectionAction != nil {
		g.editor.voxelSelectionAction(g.editor.voxelSelection)
	}
}

func (g *Game) captureMouse(capture bool) {
	g.SetMouseCapture(capture)
	g.isMouseCursorCaptured = capture
}
