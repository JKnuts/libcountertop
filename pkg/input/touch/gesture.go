package touch

import (
	"math"
	"time"
)

type Kind int

const (
	Tap Kind = iota

	Swipe
	DoubleTap
	Hold
	PinchIn
	PinchOut
)

func (k Kind) String() string {
	switch k {
	case Swipe:
		return "swipe"
	case DoubleTap:
		return "double tap"
	case Hold:
		return "hold"
	case PinchIn:
		return "pinch in"
	case PinchOut:
		return "pinch out"
	}
	return "tap"
}

type Edge int

const (
	NoEdge Edge = iota
	Left
	Right
	Top
	Bottom
)

func (e Edge) String() string {
	switch e {
	case Left:
		return "left"
	case Right:
		return "right"
	case Top:
		return "top"
	case Bottom:
		return "bottom"
	}
	return "none"
}

type Gesture struct {
	Kind    Kind
	Fingers int

	From Edge

	Toward Edge

	StartX, StartY int
	EndX, EndY     int

	Held time.Duration
}

const (
	edgeMargin = 0.06

	swipeDistance = 0.10

	holdFor = 500 * time.Millisecond

	doubleWithin = 400 * time.Millisecond
)

type Recognizer struct {
	W, H int

	tracking map[int]*journey
	ended    []journey
	fingers  int
	began    time.Time
	tapped   Gesture
	tappedAt time.Time
}

type journey struct {
	x, y       int
	endX, endY int
	at         time.Time
	edge       Edge
}

func NewRecognizer(w, h int) *Recognizer {
	return &Recognizer{W: w, H: h, tracking: map[int]*journey{}}
}

func (r *Recognizer) Feed(c Contact) (Gesture, bool) {
	if r.tracking == nil {
		r.tracking = map[int]*journey{}
	}

	switch c.Phase {
	case Down:
		if len(r.tracking) == 0 {
			r.ended, r.fingers, r.began = nil, 0, at(c)
		}
		r.tracking[c.ID] = &journey{
			x: c.X, y: c.Y, endX: c.X, endY: c.Y, at: at(c), edge: r.edgeOf(c.X, c.Y),
		}
		r.fingers = max(r.fingers, len(r.tracking))
	case Move:
		if j, ok := r.tracking[c.ID]; ok {
			j.endX, j.endY = c.X, c.Y
		}
	case Up:
		j, ok := r.tracking[c.ID]
		if !ok {
			return Gesture{}, false
		}
		delete(r.tracking, c.ID)
		j.endX, j.endY = c.X, c.Y
		r.ended = append(r.ended, *j)
		if len(r.tracking) > 0 {
			return Gesture{}, false
		}
		g := r.finish(at(c))
		r.ended = nil
		return g, true
	}
	return Gesture{}, false
}

func (r *Recognizer) Forget() {
	r.tracking, r.ended, r.fingers = map[int]*journey{}, nil, 0
	r.tapped, r.tappedAt = Gesture{}, time.Time{}
}

func (r *Recognizer) finish(now time.Time) Gesture {
	var sx, sy, ex, ey, dx, dy int
	for _, j := range r.ended {
		sx, sy, ex, ey = sx+j.x, sy+j.y, ex+j.endX, ey+j.endY
	}
	n := len(r.ended)
	g := Gesture{
		Fingers: r.fingers,
		StartX:  sx / n, StartY: sy / n,
		EndX: ex / n, EndY: ey / n,
		Held: now.Sub(r.began),
	}
	if r.fingers == 1 {
		g.From = r.ended[0].edge
	}
	dx, dy = g.EndX-g.StartX, g.EndY-g.StartY
	far := int(float64(min(r.W, r.H)) * swipeDistance)

	if r.fingers == 2 && n == 2 {
		a, b := r.ended[0], r.ended[1]
		was := spread(a.x, a.y, b.x, b.y)
		now := spread(a.endX, a.endY, b.endX, b.endY)
		switch {
		case now-was > float64(far):
			g.Kind = PinchOut
			return g
		case was-now > float64(far):
			g.Kind = PinchIn
			return g
		}
	}

	if abs(dx) < far && abs(dy) < far {
		return r.still(g, now, far)
	}

	g.Kind = Swipe
	if abs(dx) >= abs(dy) {
		g.Toward = Right
		if dx < 0 {
			g.Toward = Left
		}
		return g
	}

	g.Toward = Bottom
	if dy < 0 {
		g.Toward = Top
	}
	return g
}

func (r *Recognizer) still(g Gesture, now time.Time, far int) Gesture {
	if g.Held >= holdFor {
		g.Kind = Hold
		r.tappedAt = time.Time{}
		return g
	}
	g.Kind = Tap
	last := r.tapped
	if !r.tappedAt.IsZero() && now.Sub(r.tappedAt) <= doubleWithin &&
		last.Fingers == g.Fingers &&
		abs(last.EndX-g.EndX) < far && abs(last.EndY-g.EndY) < far {
		g.Kind = DoubleTap
		r.tappedAt = time.Time{}
		return g
	}
	r.tapped, r.tappedAt = g, now
	return g
}

func spread(x1, y1, x2, y2 int) float64 {
	return math.Hypot(float64(x2-x1), float64(y2-y1))
}

func (r *Recognizer) edgeOf(x, y int) Edge {
	margin := int(float64(min(r.W, r.H)) * edgeMargin)

	near := map[Edge]int{Left: x, Right: r.W - 1 - x, Top: y, Bottom: r.H - 1 - y}

	best, shortest := NoEdge, margin
	for edge, d := range near {
		if d < shortest || (d == shortest && best != NoEdge && edge < best) {
			best, shortest = edge, d
		}
	}
	return best
}

func at(c Contact) time.Time {
	if c.At.IsZero() {
		return time.Now()
	}
	return c.At
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
