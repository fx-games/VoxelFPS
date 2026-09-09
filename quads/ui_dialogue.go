package quads

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

// DialogueChoice is one reply on offer. Next is the line it leads to, or nil
// to end the conversation when picked.
type DialogueChoice struct {
	Text string
	Next *DialogueLine
	// Do is what picking this reply does in the world, as opposed to what it
	// says - hiring a companion, dismissing one. Run once, when the reply is
	// picked, before the conversation moves on to Next. nil for the ordinary
	// reply that only talks.
	Do func()
}

// DialogueLine is one thing an NPC says and what the player can say back -
// a node in a branching tree kept as plain Go structs, the same way the F1
// menu's own options are plain Go closures rather than a script format.
type DialogueLine struct {
	Speaker string
	Text    string
	Choices []DialogueChoice
}

// The Fallout 3 shape: NPC text across the top, replies as a list across the
// bottom, both as fractions of the screen.
const (
	dialogueTopHeightFrac    = 0.28
	dialogueBottomHeightFrac = 0.32
	dialoguePadding          = 20
)

// GuiDialogue is the conversation screen: an NPC's line in a box at the top
// (GuiMessageBox, just handed a shorter screen height so it centers within
// the top band instead of the whole screen) and the player's replies as a
// selectable list at the bottom (GuiList, positioned the same way
// GuiInventory already positions its own list pane). Nothing here is a new
// widget, only a new arrangement of two existing ones.
type GuiDialogue struct {
	uiContext UIContext
	npcText   *GuiMessageBox
	replies   *GuiList

	current     *DialogueLine
	shouldClose bool
	onClose     func()
	screen      mgl32.Vec2
}

func NewGuiDialogue(uiContext UIContext) *GuiDialogue {
	return &GuiDialogue{
		uiContext: uiContext,
		npcText:   NewGuiMessageBox(uiContext),
		replies:   NewGuiList(uiContext),
	}
}

func (d *GuiDialogue) SetOnClose(callback func()) { d.onClose = callback }
func (d *GuiDialogue) ShouldClose() bool          { return d.shouldClose }

func (d *GuiDialogue) Close() {
	d.shouldClose = true
	if d.onClose != nil {
		d.onClose()
	}
}

// Show starts, or continues, the conversation at line.
func (d *GuiDialogue) Show(line *DialogueLine) {
	d.current = line
	topHeight := int(d.screen.Y() * dialogueTopHeightFrac)
	d.npcText.Show(line.Text, int(d.screen.X()), topHeight)

	choices := make([]string, len(line.Choices))
	for i, choice := range line.Choices {
		choices[i] = choice.Text
	}
	if len(choices) == 0 {
		choices = []string{"[End conversation]"}
	}
	d.replies.SetItems(choices)
	d.replies.SetSelectedIndex(0)
}

func (d *GuiDialogue) HandleResize(screenWidth, screenHeight int, screenCoordScale mgl32.Vec2) {
	d.screen = mgl32.Vec2{float32(screenWidth), float32(screenHeight)}

	bottomHeight := float32(screenHeight) * dialogueBottomHeightFrac
	d.replies.SetOrigin(dialoguePadding, float32(screenHeight)-bottomHeight-dialoguePadding)
	d.replies.SetBounds(float32(screenWidth)-2*dialoguePadding, bottomHeight)

	if d.current != nil {
		d.Show(d.current)
	}
}

func (d *GuiDialogue) HandleKeyEvents(key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) bool {
	if action != glfw.Press && action != glfw.Repeat {
		return false
	}
	switch key {
	case glfw.KeyEscape:
		d.Close()
	case glfw.KeyDown, glfw.KeyS:
		d.replies.SelectNext()
	case glfw.KeyUp, glfw.KeyW:
		d.replies.SelectPrevious()
	case glfw.KeyEnter, glfw.KeyKPEnter, glfw.KeySpace:
		d.choose()
	default:
		return false
	}
	return true
}

// choose acts on the highlighted reply: moves on to the line it leads to, or
// ends the conversation for a choice (or an empty list) that leads nowhere.
func (d *GuiDialogue) choose() {
	if d.current == nil || len(d.current.Choices) == 0 {
		d.Close()
		return
	}
	index := d.replies.GetSelectedIndex()
	if index < 0 || index >= len(d.current.Choices) {
		return
	}
	choice := d.current.Choices[index]
	if choice.Do != nil {
		choice.Do()
	}
	next := choice.Next
	if next == nil {
		d.Close()
		return
	}
	d.Show(next)
}

func (d *GuiDialogue) HandleMousePosEvents(xpos, ypos float64) bool {
	return d.replies.HandleMousePosEvents(xpos, ypos)
}

func (d *GuiDialogue) HandleMouseButtonEvents(xpos, ypos float64, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) bool {
	if action != glfw.Press || button != glfw.MouseButtonLeft {
		return false
	}
	if !d.replies.HandleMousePosEvents(xpos, ypos) {
		return false
	}
	d.choose()
	return true
}

func (d *GuiDialogue) HandleScrollEvents(xoff, yoff float64) bool {
	return d.replies.HandleScrollEvents(xoff, yoff)
}

func (d *GuiDialogue) HandleDrop(names []string, x, y float64) bool { return false }

func (d *GuiDialogue) Draw() {
	if d.current == nil {
		return
	}
	d.npcText.Draw()
	d.replies.Draw()
}
