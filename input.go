package main

import (
	"fmt"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/events"
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
	if g.autoWalk || inputSrc.GetKey(g.key(ActionMoveForward)) == glfw.Press {
		moveDir[1] = 1
	} else if inputSrc.GetKey(g.key(ActionMoveBackward)) == glfw.Press {
		moveDir[1] = -1
	}

	if inputSrc.GetKey(g.key(ActionMoveLeft)) == glfw.Press {
		moveDir[0] = -1
	} else if inputSrc.GetKey(g.key(ActionMoveRight)) == glfw.Press {
		moveDir[0] = 1
	}

	if inputSrc.GetKey(g.key(ActionCrouch)) == glfw.Press {
		upDown = -1
	} else if inputSrc.GetKey(g.key(ActionJump)) == glfw.Press {
		upDown = 1
	}

	// Roll is the suit's alone; on the ground the horizon is gravity's and
	// these two keys do nothing. See evaMove.
	roll := int(0)
	if inputSrc.GetKey(g.key(ActionRollLeft)) == glfw.Press {
		roll = -1
	} else if inputSrc.GetKey(g.key(ActionRollRight)) == glfw.Press {
		roll = 1
	}

	physics := g.player.Physics()

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

	if g.gravitationEnabled && g.climbVelocity(physics, wishDir, moveDir, upDown) {
		return
	}

	// Outside the hull the walk does not apply - see eva.go. It comes after the
	// ladder for the same reason the ladder comes first: both of them switch
	// gravity, and the one that is actually happening has to be the one that
	// writes it last.
	if g.gravitationEnabled && g.evaMove(physics, moveDir, upDown, roll, elapsed) {
		return
	}

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

	if key == g.key(ActionToggleEmissiveTexture) && g.lastHitResult != nil && g.lastHitResult.HitInfo.Hit {
		hit := g.lastHitResult.HitInfo
		if textureIndex, nowGlowing, ok := g.ToggleEmissiveTextureAt(hit.CollisionGridPosition, hit.Side); ok {
			g.Toast(fmt.Sprintf("Texture %d %s", textureIndex, map[bool]string{true: "glowing", false: "back to normal"}[nowGlowing]))
		}
	}

	if g.editor.active {
		// Ctrl+S, because the one keyboard shortcut everybody already knows is
		// worth more than a rebindable action nobody would find. It is not in
		// the bindings table for the same reason the number row is not: it is
		// not a choice.
		if key == glfw.KeyS && mods&glfw.ModControl != 0 {
			g.saveMapWithToast()
		}
		// Editor mode owns the number row, Tab and the two new keys. Which of
		// its three tools they belong to is the mode, and the mode is on screen
		// - see drawEditorHUD, which is the whole of the editor's interface.
		if key == g.key(ActionEditorCycleMode) {
			g.cycleEditorMode()
		}
		if key == g.key(ActionEditorTabAction) {
			g.editorTabAction()
		}
		if key == g.key(ActionEditorRotate) {
			g.rotateProp()
		}
		// One key, every mode: the browser for whatever this mode places. The
		// palettes are hundreds of pieces long now, so the wheel is for the
		// piece next door and this is for the one three folders away.
		if key == g.key(ActionEditorOpenPicker) {
			g.openModePicker()
		}
		if key >= glfw.Key1 && key <= glfw.Key9 {
			g.editorNumberKey(int(key - glfw.Key1))
		}
	} else if key == g.key(ActionEditorTabAction) && g.editor.voxelBlock != nil && g.lastHitResult != nil && g.lastHitResult.HitInfo.Hit {
		rayHitBlock := g.lastHitResult.HitInfo.CollisionGridPosition
		hitBlock := g.CubeMap.GetGlobalBlock(rayHitBlock.X, rayHitBlock.Y, rayHitBlock.Z)
		g.editor.voxelBlock.Textures = maps.Clone(hitBlock.Textures)
		g.Toast(fmt.Sprintf("Block textures copied from %v", rayHitBlock))
	}

	if !g.editor.active && key >= glfw.Key1 && key <= glfw.Key9 {
		// The binding is the weapons folder's, not ours.
		g.selectWeaponByKey(int(key-glfw.Key1) + 1)
	}

	if key == g.key(ActionToggleEditorMode) {
		g.toggleEditorMode()
	}

	// Reload is a press the weapon's script may or may not act on: a gun with
	// no Reload state, like a pump shotgun that works itself, ignores it.
	if key == g.key(ActionReload) && g.isMouseCursorCaptured {
		g.reloadPressed = true
	}

	// Nine number keys against a hundred and forty migrated weapons, so , and .
	// walk through the whole folder.
	if key == g.key(ActionPreviousWeapon) {
		g.cycleWeapon(-1)
	}
	if key == g.key(ActionNextWeapon) {
		g.cycleWeapon(1)
	}

	// The inventory: what has been picked up, and a way back into anything
	// that is not on a number key.
	if key == g.key(ActionOpenInventory) {
		g.openInventory()
	}

	if key == g.key(ActionOpenEscapeMenu) {
		g.openEscapeMenu()
	}
	if key == g.key(ActionOpenWizardMenu) {
		g.openWizardMenu()
	}

	if key == g.key(ActionToggleMouseCapture) {
		g.toggleMouseCapture()
	}

	if key == g.key(ActionToggleSpawnMode) {
		g.toggleSpawnMode()
	}
	if key == g.key(ActionChangeDecorationTexture) {
		g.changeDecorationTexture()
	}

	if key == g.key(ActionOpenCubeTextureMapper) {
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

	// The field of view the player plays at, which a scope divides rather than
	// replaces - so widening the view while scoped widens what unscoping goes
	// back to, and does not fight the scope for the camera.
	if key == g.key(ActionIncreaseFOV) {
		g.changeBaseFOV(5)
		g.Camera.LookSensitivity += 0.01
	}
	if key == g.key(ActionDecreaseFOV) {
		g.changeBaseFOV(-5)
		g.Camera.LookSensitivity -= 0.01
	}

	if key == g.key(ActionWireframeOff) {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.FILL)
	}
	if key == g.key(ActionWireframeOn) {
		gl.PolygonMode(gl.FRONT_AND_BACK, gl.LINE)
	}

	if key == g.key(ActionTestAtlasPicker) {
		g.testPackedAtlasPicker()
	}

	if key == g.key(ActionSaveScreenshot) {
		g.SaveScreenshot("screenshot00.png")
		//g.SaveShadowMaps("./temp")
	}

	if key == g.key(ActionToggleGravity) {
		g.setGravity(!g.gravitationEnabled)
	}
	if key == g.key(ActionCyclePlayerLight) {
		// One key, one slot: off -> flashlight -> torch -> off. The player carries a
		// single light, so picking one puts the other away.
		g.CyclePlayerLight()
	}
	// One key for "do the thing I am looking at": an item the selection beam
	// has picked out is taken, and anything else is interacted with. The item
	// wins because it is the nearer of the two - the beam only holds one that
	// is in front of whatever the ray hit.
	if key == g.key(ActionInteract) && !g.takeHoveredPickup() && g.isHoveringObject() {
		events.Publish(events.EventPlayerInteractWith, g.hoveredObj())
	}
	// One key for "put that exactly there": it takes what is being looked at
	// into the player's hands and puts down what is already in them. See
	// carry.go.
	if key == g.key(ActionCarryItem) {
		g.toggleCarry()
	}
	// Quickplay: the audio log picked up most recently starts talking, and the
	// player keeps walking. Only while the game has the mouse, the same
	// condition Reload is under - the key it shares with the entity menu's own
	// cursor is only ever the entity menu's while the cursor is free.
	if key == g.key(ActionQuickplayLog) && g.isMouseCursorCaptured {
		g.quickplayLog()
	}
	if key == g.key(ActionSpawnGameObject) {
		g.placeProp()
	}

	if key == g.key(ActionOpenVoxelEditMenu) && (!g.isMouseCursorCaptured || g.editor.active) {
		g.openVoxelEditMenu()
	}

	if key == g.key(ActionEntityMenuNext) {
		g.entityMenu.SelectNext()
	}
	if key == g.key(ActionEntityMenuPrevious) {
		g.entityMenu.SelectPrevious()
	}

	if key == g.key(ActionEntityMenuScrollUp) {
		g.entityMenu.ScrollUp()
	}
	if key == g.key(ActionEntityMenuScrollDown) {
		g.entityMenu.ScrollDown()
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
	if g.editor.active {
		// The wheel walks whatever the number row picks - the shape, the
		// placement palette, the kind of actor - so a palette longer than the
		// number row still needs no menu.
		g.editorScroll(yoff)
		return
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

	// The right button is the way back into mouse capture whenever the mouse is
	// already free, whatever else is going on. F2 does it from either side.
	if button == glfw.MouseButtonRight && !g.isMouseCursorCaptured {
		if action == glfw.Press {
			g.toggleMouseCapture()
		}
		return
	}

	// In editor mode both buttons belong to the current tool, and nothing is
	// held to fire anyway.
	if g.editor.active && g.isMouseCursorCaptured {
		if action == glfw.Press {
			g.editorClick(button)
		}
		return
	}

	if button == glfw.MouseButtonRight {
		// While the game has the mouse, the right button is ZDoom's
		// +altattack: a scope, a bayonet, a grenade under the barrel.
		g.altHeld = action == glfw.Press
		return
	}

	if g.editor.gameObjectForSpawning != nil && button == glfw.MouseButtonLeft && action == glfw.Press {
		g.placeProp()
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
	}
}

// editorClick is what the two mouse buttons do, which - like every other key in
// editor mode - is whatever the current mode says. Left builds, right takes
// back: a pending voxel corner, a prop, the last waypoint of a route.
func (g *Game) editorClick(button glfw.MouseButton) {
	if g.editor.mode.placesProps() {
		if button == glfw.MouseButtonRight {
			g.deleteHovered()
			return
		}
		if button == glfw.MouseButtonLeft {
			g.placeProp()
		}
		return
	}
	switch g.editor.mode {
	case ModeActors:
		if button == glfw.MouseButtonRight {
			g.dropLastWaypoint()
			return
		}
		if button == glfw.MouseButtonLeft {
			g.actorClick()
		}
	default:
		if button == glfw.MouseButtonRight {
			g.cancelEditorSelection()
			return
		}
		if button != glfw.MouseButtonLeft {
			return
		}
		// A paint, dig or fill tool is picked off the same number row as the
		// shapes, so having one in hand is what a click means until a shape
		// puts it away again.
		if g.editor.tool != EditorToolNone {
			g.applyEditorTool()
			return
		}
		g.applyEditorEdit()
	}
}

func (g *Game) captureMouse(capture bool) {
	g.SetMouseCapture(capture)
	g.isMouseCursorCaptured = capture
}
