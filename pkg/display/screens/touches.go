package screens

import (
	"fmt"
	"image"
	"slices"
	"sync"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/input/touch"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const priorityTouches = priorityMarks + 50

type touches struct {
	mu   sync.Mutex
	down map[int]image.Point
	last string
}

func (t *touches) contact(c touch.Contact) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.down == nil {
		t.down = map[int]image.Point{}
	}
	switch c.Phase {
	case touch.Down, touch.Move:
		t.down[c.ID] = image.Pt(c.X, c.Y)
	case touch.Up:
		delete(t.down, c.ID)
	}
}

func (t *touches) gesture(name string) {
	t.mu.Lock()
	t.last = name
	t.mu.Unlock()
}

func (t *touches) now() (map[int]image.Point, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[int]image.Point, len(t.down))
	for id, at := range t.down {
		out[id] = at
	}
	return out, t.last
}

func gestureName(g touch.Gesture) string {
	if g.From != touch.NoEdge {
		return fmt.Sprintf("%s %s → %s", g.Kind, g.From, g.Toward)
	}
	if s := gestureSetting(g); s != "" {
		return s.Label()
	}
	return g.Kind.String()
}

func (a *App) touchMarks(w *gogui.Window) gogui.View {
	if !a.o.Settings().Screen.Touches {
		return nil
	}
	down, last := a.touched.now()
	vw, vh := w.WindowSize()
	reach := style.Reach()
	blue := gogui.RGB(0x29, 0x9d, 0xff)
	fill := gogui.RGBA(0x29, 0x9d, 0xff, 0x70)
	white := gogui.RGB(0xff, 0xff, 0xff)
	shade := gogui.RGBA(0, 0, 0, 0xc0)
	text := gogui.CurrentTheme().Cfg.TextStyleDef
	text.Color = white
	text.Size *= 1.3
	ids := make([]int, 0, len(down))
	for id := range down {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	lines := []string{say.F("debug.touches", map[string]any{"N": len(down)})}
	if last != "" {
		lines = append(lines, say.F("debug.touches.gesture", map[string]any{"Name": last}))
	}
	return gogui.DrawCanvas(gogui.DrawCanvasCfg{
		Width:  float32(vw),
		Height: float32(vh),
		OnDraw: func(dc *gogui.DrawContext) {
			line := max(reach*0.04, 2)
			radius := reach * 1.1
			for _, id := range ids {
				at := down[id]
				x, y := float32(at.X), float32(at.Y)
				dc.FilledRect(0, y-line/2, float32(vw), line, blue)
				dc.FilledRect(x-line/2, 0, line, float32(vh), blue)
				dc.FilledCircle(x, y, radius, fill)
				dc.Circle(x, y, radius, shade, line*3)
				dc.Circle(x, y, radius, white, line*1.5)
				label := fmt.Sprintf("%d", id)
				lx, ly := x+radius*0.75, y-radius*0.75-text.Size*1.6
				dc.FilledRoundedRect(lx, ly, text.Size*2, text.Size*1.6, text.Size*0.4, shade)
				dc.Text(lx+text.Size*0.5, ly+text.Size*0.25, label, text)
			}
			row := text.Size * 1.6
			dc.FilledRoundedRect(
				reach*0.2,
				reach*0.2,
				reach*7,
				row*float32(len(lines))+text.Size*0.6,
				text.Size*0.4,
				shade,
			)
			for i, l := range lines {
				dc.Text(reach*0.2+text.Size*0.6, reach*0.2+text.Size*0.4+float32(i)*row, l, text)
			}
		},
	})
}
