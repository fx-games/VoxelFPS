package quads

import "testing"

func TestWrapTextBreaksOnWhitespaceOnly(t *testing.T) {
	lines := wrapText("the quick brown fox jumps", 11)
	want := []string{"the quick", "brown fox", "jumps"}
	if len(lines) != len(want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("got %v, want %v", lines, want)
		}
	}
}

func TestWrapTextKeepsForcedNewlines(t *testing.T) {
	lines := wrapText("line one\nline two", 20)
	if len(lines) != 2 || lines[0] != "line one" || lines[1] != "line two" {
		t.Fatalf("got %v", lines)
	}
}

func TestCenterLinesPadsToLongest(t *testing.T) {
	lines := centerLines([]string{"hi", "hello"})
	if lines[0] != " hi" {
		t.Fatalf("expected \" hi\" (1 leading space), got %q", lines[0])
	}
	if lines[1] != "hello" {
		t.Fatalf("longest line should stay unpadded, got %q", lines[1])
	}
}
