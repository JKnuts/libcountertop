package touch

import (
	"testing"
	"time"
)

const (
	screenW = 1920
	screenH = 1200
)

func down(id, x, y int) Contact {
	return Contact{ID: id, X: x, Y: y, Phase: Down, At: time.Now()}
}

func up(id, x, y int) Contact {
	return Contact{ID: id, X: x, Y: y, Phase: Up, At: time.Now()}
}

func journeyOf(t *testing.T, r *Recognizer, id, x1, y1, x2, y2 int) Gesture {
	t.Helper()

	if _, ok := r.Feed(down(id, x1, y1)); ok {
		t.Fatal("a gesture was reported before the finger lifted")
	}
	r.Feed(Contact{ID: id, X: (x1 + x2) / 2, Y: (y1 + y2) / 2, Phase: Move, At: time.Now()})

	g, ok := r.Feed(up(id, x2, y2))
	if !ok {
		t.Fatal("lifting reported no gesture")
	}
	return g
}

func TestTap(t *testing.T) {
	r := NewRecognizer(screenW, screenH)

	g := journeyOf(t, r, 1, 900, 600, 903, 604)
	if g.Kind != Tap {
		t.Errorf("a finger that went nowhere is a %v, want a tap", g.Kind)
	}
}

func TestSwipeDirections(t *testing.T) {
	tests := []struct {
		name           string
		x1, y1, x2, y2 int
		want           Edge
	}{
		{"rightward", 200, 600, 900, 600, Right},
		{"leftward", 900, 600, 200, 600, Left},
		{"downward", 900, 200, 900, 900, Bottom},
		{"upward", 900, 900, 900, 200, Top},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRecognizer(screenW, screenH)

			g := journeyOf(t, r, 1, tt.x1, tt.y1, tt.x2, tt.y2)
			if g.Kind != Swipe {
				t.Fatalf("got a %v, want a swipe", g.Kind)
			}
			if g.Toward != tt.want {
				t.Errorf("headed %v, want %v", g.Toward, tt.want)
			}
		})
	}
}

func TestSwipeFromAnEdge(t *testing.T) {
	tests := []struct {
		name string
		x, y int
		want Edge
	}{
		{"hard against the left", 0, 600, Left},
		{"just inside the left", 20, 600, Left},
		{"hard against the right", screenW - 1, 600, Right},
		{"the top", 900, 2, Top},
		{"the bottom", 900, screenH - 1, Bottom},
		{"the middle is no edge", 900, 600, NoEdge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRecognizer(screenW, screenH)

			if got := r.edgeOf(tt.x, tt.y); got != tt.want {
				t.Errorf("edgeOf(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestEdgeSwipeCarriesBothEnds(t *testing.T) {
	r := NewRecognizer(screenW, screenH)

	g := journeyOf(t, r, 1, 4, 600, 700, 610)

	if g.Kind != Swipe {
		t.Fatalf("got a %v, want a swipe", g.Kind)
	}
	if g.From != Left {
		t.Errorf("came from %v, want the left edge", g.From)
	}
	if g.Toward != Right {
		t.Errorf("headed %v, want right", g.Toward)
	}
}

func TestTapAtAnEdgeIsStillATap(t *testing.T) {
	r := NewRecognizer(screenW, screenH)

	g := journeyOf(t, r, 1, 4, 600, 8, 604)
	if g.Kind != Tap {
		t.Errorf("a short contact at the edge is a %v, want a tap", g.Kind)
	}
	if g.From != Left {
		t.Errorf("it started at %v, want the left edge recorded anyway", g.From)
	}
}

func TestJourneysAreTrackedByID(t *testing.T) {
	r := NewRecognizer(screenW, screenH)

	r.Feed(Contact{Slot: 0, ID: 1, X: 100, Y: 600, Phase: Down, At: time.Now()})
	r.Feed(Contact{Slot: 0, ID: 1, X: 100, Y: 600, Phase: Up, At: time.Now()})

	r.Feed(Contact{Slot: 0, ID: 2, X: 1800, Y: 600, Phase: Down, At: time.Now()})
	g, ok := r.Feed(Contact{Slot: 0, ID: 2, X: 1805, Y: 604, Phase: Up, At: time.Now()})

	if !ok {
		t.Fatal("the second finger reported no gesture")
	}
	if g.Kind != Tap {
		t.Errorf(
			"the second finger came out as a %v, want a tap from where it actually started",
			g.Kind,
		)
	}
	if g.StartX != 1800 {
		t.Errorf("it started at %d, want 1800", g.StartX)
	}
}

func together(r *Recognizer, from, to [][2]int, held time.Duration) (Gesture, bool) {
	start := time.Now()
	for i, p := range from {
		r.Feed(Contact{ID: i + 1, X: p[0], Y: p[1], Phase: Down, At: start})
	}
	var g Gesture
	var ok bool
	for i, p := range to {
		g, ok = r.Feed(Contact{ID: i + 1, X: p[0], Y: p[1], Phase: Up, At: start.Add(held)})
	}
	return g, ok
}

func TestFingersDownTogetherMakeOneGesture(t *testing.T) {
	r := NewRecognizer(screenW, screenH)
	r.Feed(down(1, 800, 600))
	r.Feed(down(2, 1000, 600))
	if _, ok := r.Feed(up(1, 800, 600)); ok {
		t.Fatal("a gesture was reported while a finger was still down")
	}
	g, ok := r.Feed(up(2, 1000, 600))
	if !ok || g.Kind != Tap || g.Fingers != 2 {
		t.Errorf("came out as %+v, want a two-finger tap", g)
	}
}

func TestMultiFingerGestures(t *testing.T) {
	cases := []struct {
		name    string
		from    [][2]int
		to      [][2]int
		held    time.Duration
		kind    Kind
		toward  Edge
		fingers int
	}{
		{
			"three-finger tap",
			[][2]int{{700, 600}, {900, 600}, {1100, 600}},
			[][2]int{{702, 603}, {901, 598}, {1104, 601}},
			100 * time.Millisecond, Tap, NoEdge, 3,
		},
		{
			"five-finger tap",
			[][2]int{{500, 600}, {700, 500}, {900, 480}, {1100, 500}, {1300, 650}},
			[][2]int{{500, 600}, {700, 500}, {900, 480}, {1100, 500}, {1300, 650}},
			100 * time.Millisecond, Tap, NoEdge, 5,
		},
		{
			"two-finger hold",
			[][2]int{{800, 600}, {1000, 600}},
			[][2]int{{801, 600}, {1000, 602}},
			time.Second, Hold, NoEdge, 2,
		},
		{
			"two-finger swipe down",
			[][2]int{{800, 300}, {1000, 300}},
			[][2]int{{800, 800}, {1000, 800}},
			200 * time.Millisecond, Swipe, Bottom, 2,
		},
		{
			"four-finger swipe left",
			[][2]int{{1400, 400}, {1400, 550}, {1400, 700}, {1400, 850}},
			[][2]int{{600, 400}, {600, 550}, {600, 700}, {600, 850}},
			200 * time.Millisecond, Swipe, Left, 4,
		},
		{
			"pinch out",
			[][2]int{{900, 600}, {1000, 600}},
			[][2]int{{600, 600}, {1300, 600}},
			300 * time.Millisecond, PinchOut, NoEdge, 2,
		},
		{
			"pinch in",
			[][2]int{{500, 600}, {1400, 600}},
			[][2]int{{900, 600}, {1000, 600}},
			300 * time.Millisecond, PinchIn, NoEdge, 2,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, ok := together(NewRecognizer(screenW, screenH), c.from, c.to, c.held)
			if !ok || g.Kind != c.kind || g.Toward != c.toward || g.Fingers != c.fingers {
				t.Errorf("came out as %+v, want %v toward %v with %d fingers",
					g, c.kind, c.toward, c.fingers)
			}
			if g.From != NoEdge {
				t.Errorf("a multi-finger gesture started at the %v edge", g.From)
			}
		})
	}
}

func TestTwoTapsInARowMakeADoubleTap(t *testing.T) {
	r := NewRecognizer(screenW, screenH)
	pair := [][2]int{{800, 600}, {1000, 600}}
	if g, _ := together(r, pair, pair, 50*time.Millisecond); g.Kind != Tap {
		t.Fatalf("the first tap came out as %v", g.Kind)
	}
	if g, _ := together(r, pair, pair, 50*time.Millisecond); g.Kind != DoubleTap || g.Fingers != 2 {
		t.Errorf("the second tap came out as %+v, want a two-finger double tap", g)
	}
	if g, _ := together(r, pair, pair, 50*time.Millisecond); g.Kind != Tap {
		t.Errorf("a third tap came out as %v, want a fresh tap", g.Kind)
	}
}

func TestTapsWithDifferentFingersAreNotADoubleTap(t *testing.T) {
	r := NewRecognizer(screenW, screenH)
	together(r, [][2]int{{800, 600}}, [][2]int{{800, 600}}, 50*time.Millisecond)
	g, _ := together(
		r,
		[][2]int{{800, 600}, {1000, 600}},
		[][2]int{{800, 600}, {1000, 600}},
		50*time.Millisecond,
	)
	if g.Kind != Tap {
		t.Errorf("came out as %v, want a tap", g.Kind)
	}
}

func TestLiftWithoutADown(t *testing.T) {
	r := NewRecognizer(screenW, screenH)

	if _, ok := r.Feed(up(9, 100, 100)); ok {
		t.Error("a lift with no down reported a gesture")
	}
}

func TestForget(t *testing.T) {
	r := NewRecognizer(screenW, screenH)
	r.Feed(down(1, 100, 600))

	r.Forget()

	if _, ok := r.Feed(up(1, 900, 600)); ok {
		t.Error("a gesture survived Forget")
	}
}

func TestWorksInPortrait(t *testing.T) {
	r := NewRecognizer(screenH, screenW)

	g := journeyOf(t, r, 1, 4, 900, 700, 910)
	if g.From != Left || g.Toward != Right {
		t.Errorf("portrait swipe came out from %v toward %v", g.From, g.Toward)
	}
}
