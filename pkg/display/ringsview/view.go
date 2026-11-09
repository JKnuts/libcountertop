package ringsview

import (
	"slices"
	"sync"
	"time"

	"github.com/go-gui-org/go-glyph"
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/entitycard"
	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	zoomTime  = 450 * time.Millisecond
	panelTime = 300 * time.Millisecond
	page      = "rings"
)

type Options struct {
	UI    *widgets.UI
	Home  *homecontrol.Engine
	Clock func(w *gogui.Window, at ui.Rect) gogui.View
}

type View struct {
	o Options

	mu     sync.Mutex
	from   string
	focus  string
	zoom   float32
	picked string
	open   float32
	press  *point
	card   *entitycard.Card
}

type point struct{ x, y float32 }

func New(o Options) *View {
	return &View{
		o:     o,
		from:  "home",
		focus: "home",
		zoom:  1,
		card:  entitycard.New(entitycard.Options{UI: o.UI, Perform: o.Home.Perform}),
	}
}

func (v *View) Draw(w *gogui.Window, at ui.Rect) gogui.View {
	root, loading, err := v.o.Home.Rings()
	pal := v.o.UI.Palette()
	vw, vh := float32(at.W), float32(at.H)
	tall := vh > vw
	side := min(vw, vh)

	v.mu.Lock()
	if root != nil && root.Find(v.focus) == nil {
		v.from, v.focus, v.zoom = "home", "home", 1
	}
	if root != nil && v.picked != "" && root.Find("device:"+v.picked) == nil {
		v.picked = ""
	}
	open := v.open
	v.mu.Unlock()

	cw, ch := lerp(vw, side, open), vh
	if tall {
		cw, ch = vw, lerp(vh, side, open)
	}
	canvas := gogui.DrawCanvas(gogui.DrawCanvasCfg{
		ID:           "rings-canvas",
		Width:        cw,
		Height:       ch,
		Sizing:       gogui.FixedFixed,
		AlwaysRedraw: true,
		OnDraw: func(dc *gogui.DrawContext) {
			v.paint(dc, root, loading, err, pal, cw, ch)
		},
		OnClick: func(e gogui.EventCtx) {
			s := e.Layout.Shape
			v.arm(e.Event.MouseX-s.X, e.Event.MouseY-s.Y)
			e.Consume()
		},
		OnMouseUp: func(e gogui.EventCtx) {
			if x, y, ok := v.disarm(); ok {
				s := e.Layout.Shape
				v.tap(e.Window, root, err, x, y, s.Width, s.Height)
				e.Consume()
			}
		},
		OnGesture: func(e gogui.EventCtx) {
			if e.Event != nil && e.Event.GestureType == gogui.GesturePan {
				v.disarm()
			}
		},
	})
	content := []gogui.View{canvas}
	var ox, oy float32
	if open > 0 {
		pw, ph := vw-cw, vh
		if tall {
			pw, ph = vw, vh-ch
		}
		content = append(content, v.panel(w, root, pal, pw, ph))
		if v.o.Home.RingsSide() == schema.SideLeft {
			slices.Reverse(content)
			if tall {
				oy = ph
			} else {
				ox = pw
			}
		}
	}
	if v.clocked(root, loading, err) {
		f, _ := split(lay(v.o.Home.RingsLook(), cw, ch).label())
		at := ui.Rect{X: int(ox + f.x), Y: int(oy + f.y), W: int(f.w), H: int(f.h)}
		content = append(content, v.o.Clock(w, at))
	}
	if root != nil {
		content = append(content, v.reload(pal, ox, oy, cw, open > 0 && ox > 0))
	}
	frame := gogui.ContainerCfg{
		ID:      "rings",
		Width:   vw,
		Height:  vh,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Color:   widgets.Color(pal.Background),
		Clip:    true,
		Content: content,
	}
	if tall {
		return gogui.Column(frame)
	}
	return gogui.Row(frame)
}

func (v *View) state() (from, focus string, zoom float32) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.from, v.focus, v.zoom
}

func (v *View) paint(
	dc *gogui.DrawContext,
	root *homecontrol.Ring,
	loading bool,
	err error,
	pal theme.Theme,
	w, h float32,
) {
	g := lay(v.o.Home.RingsLook(), w, h)
	segs := flatten(root)
	from, focus, zoom := v.state()
	depth := v.o.Home.RingsDepth()
	before, after := frameFor(
		segs,
		find(segs, from),
		depth,
	), frameFor(
		segs,
		find(segs, focus),
		depth,
	)
	st := gogui.CurrentTheme().TextStyleDef
	st.Size *= 0.8
	bg := widgets.Color(pal.Background)
	for _, s := range segs {
		sh := blend(before.place(s), after.place(s), zoom)
		if !sh.shown() {
			continue
		}
		t := toneOf(pal, s.ring)
		p := wedge(g, sh)
		dc.FillPath(p, widgets.Color(t.fill), gogui.FillNonzero)
		dc.StrokePath(p, bg, 2, gogui.StrokeStyle{Join: gogui.StrokeRoundJoin})
		label := st
		label.Color = widgets.Color(t.ink)
		if s.ring.Level != homecontrol.LevelDevice {
			label.Typeface = glyph.TypefaceBold
		}
		caption(dc, g, sh, s.ring.Name, label)
	}
	v.hub(dc, g, segs, find(segs, focus), v.clocked(root, loading, err), loading, err, pal)
}

func (v *View) clocked(root *homecontrol.Ring, loading bool, err error) bool {
	if v.o.Clock == nil || root == nil || loading || err != nil || root.Devices == 0 {
		return false
	}
	_, focus, _ := v.state()
	return focus == root.Key
}

func split(b box) (face, line box) {
	lh := min(summaryStyle().Size*1.5, b.h*0.3)
	return box{b.x, b.y, b.w, b.h - lh}, box{b.x, b.y + b.h - lh, b.w, lh}
}

func summaryStyle() gogui.TextStyle {
	st := gogui.CurrentTheme().TextStyleDef
	st.Size *= 0.75
	return st
}

func (v *View) hub(
	dc *gogui.DrawContext,
	g geometry,
	segs []seg,
	focus int,
	clock bool,
	loading bool,
	err error,
	pal theme.Theme,
) {
	dc.FilledPolygon(g.hub(), widgets.Color(pal.Surface))
	b := g.label()
	if clock {
		_, b = split(b)
	}
	title := gogui.CurrentTheme().TextStyleDef
	title.Typeface = glyph.TypefaceBold
	title.Color = widgets.Color(pal.Text)
	small := summaryStyle()
	small.Color = widgets.Color(pal.Muted)
	var lines []struct {
		text string
		st   gogui.TextStyle
	}
	add := func(text string, st gogui.TextStyle) {
		if text = fit(dc, text, st, b.w*0.92); text != "" {
			lines = append(lines, struct {
				text string
				st   gogui.TextStyle
			}{text, st})
		}
	}
	switch {
	case loading:
		add(say.T("home.loading"), title)
	case err != nil:
		add(say.T("home.failed"), title)
		add(say.T("home.retry"), small)
	case len(segs) == 0 || segs[0].ring.Devices == 0:
		add(say.T("home.empty"), title)
	default:
		r := segs[focus].ring
		if !clock {
			add(r.Name, title)
		}
		sum := say.F("rings.summary", map[string]any{"On": r.Active, "N": r.Devices})
		st := small
		if w, room := dc.TextWidth(sum, st), b.w*0.92; w > room {
			st.Size *= room / w
		}
		add(sum, st)
		if focus != 0 {
			arrow := small
			arrow.Family = gogui.CurrentTheme().TextStyleIconMedium.Family
			add(gogui.IconArrowUp, arrow)
		}
	}
	var total float32
	for _, l := range lines {
		total += dc.FontHeight(l.st)
	}
	y := b.y + (b.h-total)/2
	for _, l := range lines {
		dc.Text(b.x+(b.w-dc.TextWidth(l.text, l.st))/2, y, l.text, l.st)
		y += dc.FontHeight(l.st)
	}
}

func (v *View) tap(w *gogui.Window, root *homecontrol.Ring, err error, x, y, cw, ch float32) {
	if root == nil {
		if err != nil {
			v.o.Home.Refresh()
		}
		return
	}
	g := lay(v.o.Home.RingsLook(), cw, ch)
	segs := flatten(root)
	_, focus, _ := v.state()
	at := find(segs, focus)
	a, r := g.locate(x, y)
	if r < 0 {
		if at != 0 {
			v.zoomTo(w, segs[segs[at].parent].ring.Key)
			return
		}
		v.close(w)
		return
	}
	i, ok := hit(segs, frameFor(segs, at, v.o.Home.RingsDepth()), a, r)
	if !ok {
		return
	}
	switch s := segs[i].ring; {
	case s.Device != nil:
		v.pick(w, s.Device.ID)
	case len(s.Children) > 0:
		v.zoomTo(w, s.Key)
	}
}

func (v *View) arm(x, y float32) {
	v.mu.Lock()
	v.press = &point{x, y}
	v.mu.Unlock()
}

func (v *View) disarm() (float32, float32, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.press
	v.press = nil
	if p == nil {
		return 0, 0, false
	}
	return p.x, p.y, true
}

func (v *View) zoomTo(w *gogui.Window, key string) {
	v.mu.Lock()
	v.from, v.focus, v.zoom = v.focus, key, 0
	v.mu.Unlock()
	tw := gogui.NewTweenAnimation("rings-zoom", 0, 1, func(at float32, _ *gogui.Window) {
		v.mu.Lock()
		v.zoom = at
		v.mu.Unlock()
	})
	tw.Duration = zoomTime
	tw.Easing = gogui.EaseInOutCubic
	tw.Refresh = gogui.AnimationRefreshRenderOnly
	w.AnimationAdd(tw)
}

func (v *View) pick(w *gogui.Window, id string) {
	v.mu.Lock()
	v.picked = id
	from := v.open
	v.mu.Unlock()
	v.slide(w, from, 1, nil)
}

func (v *View) close(w *gogui.Window) {
	v.mu.Lock()
	from := v.open
	v.mu.Unlock()
	v.slide(w, from, 0, func(w *gogui.Window) {
		v.mu.Lock()
		v.picked = ""
		v.mu.Unlock()
		w.InvalidateLayout()
	})
}

func (v *View) slide(w *gogui.Window, from, to float32, done func(*gogui.Window)) {
	tw := gogui.NewTweenAnimation("rings-panel", from, to, func(at float32, _ *gogui.Window) {
		v.mu.Lock()
		v.open = at
		v.mu.Unlock()
	})
	tw.Duration = panelTime
	tw.OnDone = done
	w.AnimationAdd(tw)
}
