package screens

import (
	"testing"

	"github.com/ygelfand/libcountertop/pkg/input/touch"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func TestEveryWakeGestureCanBeMade(t *testing.T) {
	made := map[schema.Gesture]bool{}
	edges := []touch.Edge{touch.Top, touch.Bottom, touch.Left, touch.Right}
	for n := 1; n <= schema.MostFingers; n++ {
		for _, k := range []touch.Kind{touch.Tap, touch.DoubleTap, touch.Hold, touch.PinchIn, touch.PinchOut} {
			made[gestureSetting(touch.Gesture{Kind: k, Fingers: n})] = true
		}
		for _, e := range edges {
			made[gestureSetting(touch.Gesture{Kind: touch.Swipe, Fingers: n, Toward: e})] = true
		}
	}
	for _, g := range schema.Gestures() {
		if !made[g] {
			t.Errorf("no touch gesture makes %q", g)
		}
	}
}

func TestSwipeSettingNamesTheDirection(t *testing.T) {
	g := touch.Gesture{Kind: touch.Swipe, Fingers: 3, Toward: touch.Bottom}
	if got := gestureSetting(g); got != "swipe-down-3" {
		t.Errorf("got %q", got)
	}
}
