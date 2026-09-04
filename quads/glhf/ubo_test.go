package glhf

import "testing"

// The layout the main UBO relies on today, plus the vec3 case that used to write
// 16 bytes out of a 12-byte value and clobber the member behind it.
func TestStd140Layout(t *testing.T) {
	offsets, size := std140Layout([]Attr{
		{"fogRange", Vec2},
		{"camPosition", Vec4},
		{"camProjectionView", Mat4},
		{"tint", Vec3},
		{"strength", Float},
	})
	want := []int{0, 16, 32, 96, 108}
	for i := range want {
		if offsets[i] != want[i] {
			t.Fatalf("offset %d = %d, want %d", i, offsets[i], want[i])
		}
	}
	if size != 112 {
		t.Fatalf("block size = %d, want 112", size)
	}
	if got := Vec3.Std140Size(); got != 12 {
		t.Fatalf("Vec3 std140 size = %d, want 12", got)
	}
	if got := Vec3.Std140Alignment(); got != 16 {
		t.Fatalf("Vec3 std140 alignment = %d, want 16", got)
	}
}

func TestStd140RejectsUnpackableMatrix(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("mat3 in a UBO should panic, it needs per-column padding")
		}
	}()
	std140Layout([]Attr{{"m", Mat3}})
}
