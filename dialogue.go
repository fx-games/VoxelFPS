package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/goray/quads"
)

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

// dialogueDir is where an actor's conversation is, one file to a speaker.
const dialogueDir = "assets/dialogue"

// dialogueActions are the things a reply can do in the world, as opposed to
// what it says. A file names one by key; anything a conversation wants to do
// has to be listed here, so a dialogue file can never reach further into the
// game than the actor it belongs to.
// The line the reply leads to is handed over as well, for an action whose
// answer is only known when it is asked - a guide saying where on its route
// it has got to. It writes into the line it is about to be shown, which the
// widget reads after running this - see GuiDialogue.choose.
var dialogueActions = map[string]func(*Actor, *quads.DialogueLine){
	"follow": func(a *Actor, _ *quads.DialogueLine) { a.follow() },
	"stay":   func(a *Actor, _ *quads.DialogueLine) { a.stopFollowing() },
	"tour":   func(a *Actor, _ *quads.DialogueLine) { a.startTour() },
	"waypoint": func(a *Actor, next *quads.DialogueLine) {
		if next != nil {
			next.Text = a.waypointText()
		}
	},
}

// dialogueFile is the on-disk shape: nodes in a flat map rather than nested,
// so two replies can lead to the same line - "thanks" is the end of three
// different branches - without it being written out three times.
type dialogueFile struct {
	Speaker string                  `json:"speaker"`
	Start   string                  `json:"start"`
	Lines   map[string]dialogueNode `json:"lines"`
}

type dialogueNode struct {
	Text    string           `json:"text"`
	Choices []dialogueOption `json:"choices"`
}

type dialogueOption struct {
	Text string `json:"text"`
	Next string `json:"next"`
	Do   string `json:"do"`
}

// dialogueFor reads the conversation for an actor, or nothing when it has no
// file - most actors are monsters, and a monster with no dialogue.json is not
// a mistake. A file that is there but will not load leaves the actor mute and
// says why: content is edited while the game runs, and one bad comma should
// not take the map down with it.
//
// It takes the actor rather than its name because a reply can act on the
// speaker - hiring it, dismissing it - and the tree is built once, at spawn,
// so the actor is what its actions have to be bound to.
func dialogueFor(actor *Actor) *quads.DialogueLine {
	path := filepath.Join(dialogueDir, strings.ToLower(actor.Name)+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		log.Printf("[dialogue] %s: %v", path, err)
		return nil
	}
	line, err := parseDialogue(data, actor)
	if err != nil {
		log.Printf("[dialogue] %s: %v", path, err)
		return nil
	}
	return line
}

// parseDialogue turns the file into the pointer tree the widget walks. Two
// passes: every line becomes a node first, then the choices are wired up, so a
// reply can point at a line written after it - or back at one already passed,
// which a single recursive pass would follow for ever.
func parseDialogue(data []byte, actor *Actor) (*quads.DialogueLine, error) {
	var file dialogueFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Start == "" {
		return nil, fmt.Errorf("no start line named")
	}

	nodes := make(map[string]*quads.DialogueLine, len(file.Lines))
	for id, node := range file.Lines {
		nodes[id] = &quads.DialogueLine{Speaker: file.Speaker, Text: node.Text}
	}
	for id, node := range file.Lines {
		for _, choice := range node.Choices {
			next, known := nodes[choice.Next]
			if choice.Next != "" && !known {
				return nil, fmt.Errorf("line %q replies to unknown line %q", id, choice.Next)
			}
			do, err := dialogueAction(choice.Do, actor, next)
			if err != nil {
				return nil, fmt.Errorf("line %q: %w", id, err)
			}
			nodes[id].Choices = append(nodes[id].Choices,
				quads.DialogueChoice{Text: choice.Text, Next: next, Do: do})
		}
	}

	start, known := nodes[file.Start]
	if !known {
		return nil, fmt.Errorf("start line %q is not in the file", file.Start)
	}
	return start, nil
}

// dialogueAction binds a named action to this actor. An empty name is the
// ordinary reply that only talks.
func dialogueAction(name string, actor *Actor, next *quads.DialogueLine) (func(), error) {
	if name == "" {
		return nil, nil
	}
	action, known := dialogueActions[name]
	if !known {
		return nil, fmt.Errorf("unknown action %q", name)
	}
	return func() { action(actor, next) }, nil
}
