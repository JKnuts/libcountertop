package screens

import (
	"github.com/ygelfand/libcountertop/pkg/display/idleview"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/visualview"
	"github.com/ygelfand/libcountertop/pkg/input/touch"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func resting(v shell.View) bool {
	switch v.(type) {
	case *idleview.View, *visualview.View:
		return true
	}
	return false
}

func gestureSetting(g touch.Gesture) schema.Gesture {
	switch g.Kind {
	case touch.Tap:
		return schema.GestureOf("tap", g.Fingers)
	case touch.DoubleTap:
		return schema.GestureOf("double-tap", g.Fingers)
	case touch.Hold:
		return schema.GestureOf("hold", g.Fingers)
	case touch.PinchIn:
		return "pinch-in"
	case touch.PinchOut:
		return "pinch-out"
	case touch.Swipe:
		toward := map[touch.Edge]string{
			touch.Top: "up", touch.Bottom: "down", touch.Left: "left", touch.Right: "right",
		}[g.Toward]
		return schema.GestureOf("swipe-"+toward, g.Fingers)
	}
	return ""
}

func (a *App) wakeBy() (schema.Gesture, bool) {
	want := a.o.Settings().Idle.Wake
	if want == "" || want == schema.GestureTap {
		return "", false
	}
	return want, resting(a.o.Shell.Top())
}
