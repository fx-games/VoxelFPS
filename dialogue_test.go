package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// The Scientist's tree used to be Go structs in this file. It is data now, so
// the check is that the data still builds the same conversation: the shape the
// player walks, the shared ending three replies lead to, and the two replies
// that hire and dismiss actually reaching the actor.
func TestScientistDialogueLoadsFromDisk(t *testing.T) {
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{})
	actor.Name = "Scientist"

	greeting := dialogueFor(actor)
	if greeting == nil {
		t.Fatal("no dialogue loaded for the Scientist")
	}
	if greeting.Speaker != "Scientist" {
		t.Fatalf("speaker is %q, want Scientist", greeting.Speaker)
	}
	if len(greeting.Choices) != 4 {
		t.Fatalf("greeting offers %d replies, want 4", len(greeting.Choices))
	}

	// "What happened here?" and "I'm just passing through." are different
	// branches that end on the same line - the flat file is what lets them
	// share it instead of holding two copies.
	viaBranch := greeting.Choices[0].Next.Choices[0].Next
	direct := greeting.Choices[3].Next
	if viaBranch != direct {
		t.Error("the two ways to the closing line built two separate lines")
	}

	hire, dismiss := greeting.Choices[1], greeting.Choices[2]
	if hire.Do == nil || dismiss.Do == nil {
		t.Fatal("the hire and dismiss replies came back with nothing to do")
	}
	hire.Do()
	if !actor.following {
		t.Error("picking the hire reply did not make the actor follow")
	}
	dismiss.Do()
	if actor.following {
		t.Error("picking the dismiss reply did not stop the actor following")
	}
}

// An actor with no file of its own is every monster in the game, not a fault.
func TestActorWithNoDialogueFileIsMute(t *testing.T) {
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{})
	actor.Name = "Imp"
	if line := dialogueFor(actor); line != nil {
		t.Fatalf("the Imp answered with %q", line.Text)
	}
}

func TestDialogueRejectsBadData(t *testing.T) {
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{})
	for name, data := range map[string]string{
		"reply to a line that is not there": `{"start":"a","lines":{"a":{"choices":[{"next":"nowhere"}]}}}`,
		"action nothing is bound to":        `{"start":"a","lines":{"a":{"choices":[{"do":"explode"}]}}}`,
		"start that is not there":           `{"start":"b","lines":{"a":{}}}`,
		"no start at all":                   `{"lines":{"a":{}}}`,
	} {
		if _, err := parseDialogue([]byte(data), actor); err == nil {
			t.Errorf("%s: loaded without complaint", name)
		}
	}
}

// A conversation that loops - "anything else?" back to the opening line - is a
// tree the two-pass build handles and a recursive one would follow for ever.
func TestDialogueCanLoopBackOnItself(t *testing.T) {
	actor := testActor(t, defaultActorHealth, mgl32.Vec3{})
	data := `{"speaker":"S","start":"a","lines":{
		"a":{"text":"one","choices":[{"text":"go","next":"b"}]},
		"b":{"text":"two","choices":[{"text":"back","next":"a"}]}}}`
	start, err := parseDialogue([]byte(data), actor)
	if err != nil {
		t.Fatal(err)
	}
	if start.Choices[0].Next.Choices[0].Next != start {
		t.Error("the loop did not come back to the line it started on")
	}
}
