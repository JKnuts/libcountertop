package schema

import (
	"slices"
	"testing"
)

func TestGesturesCoverEveryFingerCount(t *testing.T) {
	got := Gestures()
	if len(got) != 32 {
		t.Errorf("%d gestures, want 32", len(got))
	}
	for _, want := range []Gesture{
		"tap", "tap-5", "double-tap", "double-tap-3", "hold-2", "hold-5",
		"swipe-down-2", "swipe-left-5", "pinch-in", "pinch-out",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("%q missing", want)
		}
	}
	for _, never := range []Gesture{"hold", "swipe-up", "swipe-left", "tap-6"} {
		if slices.Contains(got, never) {
			t.Errorf("%q is offered", never)
		}
	}
}

func TestGestureParts(t *testing.T) {
	cases := map[Gesture]struct {
		move    string
		fingers int
	}{
		"tap":          {"tap", 1},
		"double-tap-4": {"double-tap", 4},
		"swipe-up-2":   {"swipe-up", 2},
		"pinch-out":    {"pinch-out", 1},
	}
	for g, want := range cases {
		if move, n := g.parts(); move != want.move || n != want.fingers {
			t.Errorf("%q split into %q and %d", g, move, n)
		}
	}
}
