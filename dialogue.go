package main

import "github.com/memmaker/goray/quads"

// openDialogue puts the Fallout-3-shaped conversation screen up, starting at
// line, the same modal-widget-stack way openInventory and openWizardMenu do.
func (g *Game) openDialogue(line *quads.DialogueLine) {
	previousMouseState := g.setupModal()
	dialogue := quads.NewGuiDialogue(g.UiContext())
	guiWidth, guiHeight := g.GuiSize()
	dialogue.HandleResize(guiWidth, guiHeight, g.UIScale)
	dialogue.Show(line)
	dialogue.SetOnClose(func() {
		g.captureMouse(previousMouseState)
		g.popFromWidgetStack()
	})
	g.pushToWidgetStack(dialogue)
}

// dialogueFor is a demo hook: a named actor gets a hardcoded conversation, or
// none. Real content wants this data-driven - a tree read from a file beside
// the actor, the way weapons and scenery already are - but nothing needs
// that yet: only the test map's one Scientist talks.
func dialogueFor(name string) *quads.DialogueLine {
	if name != "Scientist" {
		return nil
	}
	return scientistDialogue()
}

func scientistDialogue() *quads.DialogueLine {
	thanks := &quads.DialogueLine{
		Speaker: "Scientist",
		Text:    "Just be careful. Whatever got out down here didn't come from any of our tanks.",
	}
	whatHappened := &quads.DialogueLine{
		Speaker: "Scientist",
		Text:    "Power failed on sublevel three. By the time backup systems kicked in, half the containment logs were already gone.",
		Choices: []quads.DialogueChoice{
			{Text: "Thanks for the warning.", Next: thanks},
		},
	}
	return &quads.DialogueLine{
		Speaker: "Scientist",
		Text:    "You shouldn't be down here. This whole sub-level is off limits.",
		Choices: []quads.DialogueChoice{
			{Text: "What happened here?", Next: whatHappened},
			{Text: "I'm just passing through.", Next: thanks},
		},
	}
}
