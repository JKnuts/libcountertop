package schema

import (
	"strconv"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/say"
)

type Gesture string

const GestureTap Gesture = "tap"

const MostFingers = 5

var (
	gestureMoves = []string{
		"tap",
		"double-tap",
		"hold",
		"swipe-up",
		"swipe-down",
		"swipe-left",
		"swipe-right",
	}
	gesturePinches = []Gesture{"pinch-in", "pinch-out"}
)

func GestureOf(move string, fingers int) Gesture {
	if fingers <= 1 {
		return Gesture(move)
	}
	return Gesture(move + "-" + strconv.Itoa(fingers))
}

func Gestures() []Gesture {
	var out []Gesture
	for _, move := range gestureMoves {
		from := 1
		if move == "hold" || strings.HasPrefix(move, "swipe-") {
			from = 2
		}
		for n := from; n <= MostFingers; n++ {
			out = append(out, GestureOf(move, n))
		}
	}
	return append(out, gesturePinches...)
}

func (g Gesture) parts() (string, int) {
	s := string(g)
	if i := strings.LastIndexByte(s, '-'); i > 0 {
		if n, err := strconv.Atoi(s[i+1:]); err == nil {
			return s[:i], n
		}
	}
	return s, 1
}

func (g Gesture) Label() string {
	move, n := g.parts()
	name := say.T("gesture." + move)
	if n <= 1 || strings.HasPrefix(move, "pinch-") {
		return name
	}
	return say.F("gesture.fingers", map[string]any{"N": n, "Move": name})
}
