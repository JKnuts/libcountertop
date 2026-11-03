package radarview

import (
	"fmt"
	"image"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/nws"
	"github.com/ygelfand/libcountertop/pkg/radar"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
	"github.com/ygelfand/libcountertop/pkg/voice"
)

const (
	step = 500 * time.Millisecond
	hold = 1500 * time.Millisecond

	countyLinesMiles = 300
)

type View struct{}

func (*View) Covers() bool { return true }

func (*View) Wakes() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Options struct {
	UI       *widgets.UI
	Shell    *shell.Shell
	Radar    *radar.Radar
	Settings storage.Section[schema.Weather]
	Clock24  func() bool
	Again    func(w *gogui.Window, after time.Duration)
	Gate     func(*gogui.Window) gogui.View
}

type Radar struct {
	o    Options
	view *View

	mu     sync.Mutex
	focus  *nws.Alert
	rng    schema.RadarRange
	paused bool
	pick   time.Time
	start  time.Time
	built  time.Time
	images map[string]string
	back   backdrop
	pan    *[2]float64
	drag   [2]float32
	held   widgets.Drag
	placed placement
}

type placement struct {
	view  radar.View
	focus image.Point
	scale float64
}

func New(o Options) *Radar { return &Radar{o: o, view: &View{}} }

func (r *Radar) App() apps.App {
	return apps.App{
		ID:   schema.AppRadar,
		Name: func() string { return say.T("radar.app") },
		Icon: "mdi:radar",
		Open: func() { r.Open(nil) },
	}
}

var spoken = regexp.MustCompile(`^(?:(?:open|show)(?: me)? )?(?:the )?(?:weather )?radar$`)

func (r *Radar) Command() voice.Hook {
	return voice.Hook{
		Name:     "radar",
		Keywords: []string{"radar"},
		Match:    spoken,
		Heard:    func(voice.Heard) { r.Open(nil) },
	}
}

func (r *Radar) Open(focus *nws.Alert) {
	r.mu.Lock()
	r.focus, r.rng, r.paused, r.start = focus, r.o.Settings.Read().RadarRange, true, time.Now()
	r.pick = time.Time{}
	r.pan, r.drag = nil, [2]float32{}
	r.mu.Unlock()
	r.o.Shell.Push(r.view)
}

func (r *Radar) Screen(v shell.View) *navigation.Screen {
	if v != r.view {
		return nil
	}
	return &navigation.Screen{View: v, Clear: true, Fixed: true, Build: r.build}
}

func (r *Radar) build(w *gogui.Window) gogui.View {
	vw, vh := w.WindowSize()
	if r.o.Gate != nil {
		if gate := r.o.Gate(w); gate != nil {
			return gogui.Column(gogui.ContainerCfg{
				ID:      "radar",
				Sizing:  gogui.FillFill,
				Padding: gogui.NoPadding,
				Content: []gogui.View{widgets.Placed(ui.Rect{W: vw, H: vh}, gate), r.close()},
			})
		}
	}
	r.mu.Lock()
	focus, rng, paused, pick, start := r.focus, r.rng, r.paused, r.pick, r.start
	pan, drag := r.pan, r.drag
	r.mu.Unlock()
	target := pan
	if target == nil {
		target = center(focus)
	}
	req := radar.Request{Size: image.Pt(vw, vh), Miles: rng.Miles(), Center: target}
	m, err, ok := r.o.Radar.Get(req)
	r.forget(m)
	box := ui.Rect{W: vw, H: vh}
	var layers []gogui.View
	if len(m.Frames) > 0 && m.View.Size == req.Size {
		i := frame(m.Frames, latest(m), paused, pick, start)
		f := m.Frames[i]
		slot := fmt.Sprintf("radar/%d/%d", m.Built.UnixNano(), f.At.Unix())
		key := fmt.Sprintf("%s/%p", slot, f.Image)
		r.keep(slot, key)
		p := place(m, target, rng.Miles())
		r.mu.Lock()
		r.placed = p
		r.mu.Unlock()
		shown := p.rect(vw, vh, drag)
		if back, ok := r.behind(m, ok, req.Size); ok {
			under := place(back.m, target, rng.Miles()).rect(vw, vh, drag)
			layers = append(
				layers,
				widgets.Placed(
					under,
					widgets.Picture(widgets.ImageSource(back.key, back.img), under.W, under.H),
				),
			)
		}
		layers = append(
			layers,
			widgets.Placed(
				shown,
				widgets.Picture(widgets.ImageSource(key, f.Image), shown.W, shown.H),
			),
		)
		if shown.W == vw && shown.H == vh {
			layers = append(layers, widgets.Placed(shown, r.marks(m, focus, vw, vh)))
		}
		layers = append(layers, r.panner(box))
		layers = append(layers, r.credit(m, box))
		when := r.clock(f.At)
		if f.At.After(m.Now) {
			when = say.F("radar.forecast", map[string]any{"Time": when})
		}
		if !ok {
			when += " · " + say.T("radar.updating")
		}
		layers = append(layers, r.chip(when))
		if len(m.Times) > 1 {
			layers = append(layers, r.scrubber(box, i, m, pick))
		}
		if !paused {
			r.o.Again(w, step/2)
		}
	} else {
		if err != nil {
			layers = append(layers, widgets.Placed(box, gogui.Column(gogui.ContainerCfg{
				Sizing:  gogui.FillFill,
				HAlign:  gogui.HAlignCenter,
				VAlign:  gogui.VAlignMiddle,
				Content: []gogui.View{gogui.Label(say.T("radar.failed"), widgets.Secondary())},
			})))
			r.o.Again(w, time.Second)
		} else {
			layers = append(layers, widgets.Placed(box, widgets.Spinner(vw, vh, time.Since(start))))
			r.o.Again(w, widgets.SpinStep)
		}
	}
	atHome := (pan == nil && focus == nil) || (pan != nil && m.Home != nil && *pan == *m.Home)
	layers = append(layers, r.keys(box, rng, paused, atHome, m.Home), r.close())
	return gogui.Column(gogui.ContainerCfg{
		ID:      "radar",
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Color:   gogui.RGB(0, 0, 0),
		Content: layers,
	})
}

func place(m radar.Map, target *[2]float64, miles int) placement {
	p := placement{view: m.View, focus: image.Pt(m.View.Size.X/2, m.View.Size.Y/2), scale: 1}
	if m.Miles > 0 && miles > 0 {
		p.scale = float64(m.Miles) / float64(miles)
	}
	switch {
	case target != nil:
		p.focus = m.View.Pixel(target[0], target[1])
	case m.Home != nil:
		p.focus = m.View.Pixel(m.Home[0], m.Home[1])
	}
	return p
}

func (p placement) rect(vw, vh int, drag [2]float32) ui.Rect {
	w, h := int(float64(vw)*p.scale), int(float64(vh)*p.scale)
	return ui.Rect{
		X: int(float64(vw)/2-float64(p.focus.X)*p.scale) + int(drag[0]),
		Y: int(float64(vh)/2-float64(p.focus.Y)*p.scale) + int(drag[1]),
		W: w,
		H: h,
	}
}

func (p placement) moved(drag [2]float32) [2]float64 {
	x := float64(p.focus.X) - float64(drag[0])/p.scale
	y := float64(p.focus.Y) - float64(drag[1])/p.scale
	lat, lon := p.view.LatLon(x, y)
	return [2]float64{lat, lon}
}

func (r *Radar) panner(box ui.Rect) gogui.View {
	return widgets.Placed(box, gogui.Column(gogui.ContainerCfg{
		ID:     "radar-pan",
		Sizing: gogui.FillFill,
		OnGesture: func(e gogui.EventCtx) {
			kind := e.Event.GestureType
			if kind != gogui.GesturePan && kind != gogui.GestureSwipe {
				return
			}
			if kind == gogui.GesturePan && (e.Event.GesturePhase == gogui.GesturePhaseBegan ||
				e.Event.GesturePhase == gogui.GesturePhaseChanged) {
				if !r.held.Active() {
					r.held.Begin(e.Window, r.drop)
				}
				r.mu.Lock()
				r.drag[0] += e.Event.GestureDX
				r.drag[1] += e.Event.GestureDY
				r.mu.Unlock()
			} else {
				r.held.End(e.Window, 0)
			}
			e.Consume()
			e.Window.InvalidateLayout()
		},
	}))
}

func (r *Radar) drop(w *gogui.Window, _ float32) {
	r.mu.Lock()
	if r.drag != [2]float32{} {
		at := r.placed.moved(r.drag)
		r.pan = &at
	}
	r.drag = [2]float32{}
	r.mu.Unlock()
	w.InvalidateLayout()
}

func swallow(e gogui.EventCtx) {
	if e.Event.GestureType == gogui.GesturePan || e.Event.GestureType == gogui.GestureSwipe {
		e.Consume()
	}
}

func (r *Radar) home(to *[2]float64) {
	r.mu.Lock()
	if r.focus != nil && to != nil {
		r.pan = to
	} else {
		r.pan = nil
	}
	r.mu.Unlock()
}

func latest(m radar.Map) int {
	at := 0
	for i, f := range m.Frames {
		if !f.At.After(m.Now) {
			at = i
		}
	}
	return at
}

func frame(frames []radar.Frame, now int, paused bool, pick, start time.Time) int {
	n := len(frames)
	if paused {
		if pick.IsZero() {
			return now
		}
		return nearest(frames, pick)
	}
	if n == 1 {
		return 0
	}
	cycle := time.Duration(n-1)*step + hold
	at := time.Since(start) % cycle
	return min(int(at/step), n-1)
}

type nameReach struct {
	counties bool
	states   bool
	cityRank int
}

func namesAt(miles int) nameReach {
	switch {
	case miles <= 50:
		return nameReach{counties: true, states: true, cityRank: 10}
	case miles <= 100:
		return nameReach{counties: true, states: true, cityRank: 8}
	case miles <= 200:
		return nameReach{states: true, cityRank: 6}
	default:
		return nameReach{cityRank: -1}
	}
}

type area struct{ x, y, w, h float32 }

func (a area) overlaps(b area) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

func (a area) holds(b area) bool {
	return b.x >= a.x && b.y >= a.y && b.x+b.w <= a.x+a.w && b.y+b.h <= a.y+a.h
}

type placer struct {
	bounds area
	taken  []area
}

func (p *placer) claim(r area) bool {
	if !p.bounds.holds(r) || slices.ContainsFunc(p.taken, r.overlaps) {
		return false
	}
	p.taken = append(p.taken, r)
	return true
}

func haloText(dc *gogui.DrawContext, x, y float32, text string, st gogui.TextStyle) {
	shade := st
	shade.Color = gogui.RGBA(0, 0, 0, 0xb0)
	d := max(1, st.Size*0.08)
	for _, o := range [][2]float32{{-d, 0}, {d, 0}, {0, -d}, {0, d}} {
		dc.Text(x+o[0], y+o[1], text, shade)
	}
	dc.Text(x, y, text, st)
}

func places(dc *gogui.DrawContext, v radar.View, miles int, home *image.Point, vw, vh int) {
	at := namesAt(miles)
	if at.cityRank < 0 {
		return
	}
	reach := style.Reach()
	base := gogui.CurrentTheme().Cfg.TextStyleDef
	p := &placer{
		bounds: area{reach * 0.2, reach * 0.2, float32(vw) - reach*0.4, float32(vh) - reach*2.4},
		taken:  []area{{0, 0, reach * 1.6, reach * 1.6}},
	}
	if home != nil {
		r := reach * 0.2
		p.taken = append(p.taken, area{float32(home.X) - r, float32(home.Y) - r, 2 * r, 2 * r})
	}
	centered := func(l radar.Label, text string, st gogui.TextStyle) {
		pt := v.Pixel(float64(l.Lat), float64(l.Lon))
		w, h := dc.TextWidth(text, st), st.Size*1.25
		x, y := float32(pt.X)-w/2, float32(pt.Y)-h/2
		if p.claim(area{x, y, w, h}) {
			haloText(dc, x, y, text, st)
		}
	}
	if at.states {
		st := base
		st.Size *= 0.85
		st.Color = gogui.RGBA(0xff, 0xff, 0xff, 0xd0)
		for _, l := range radar.StateNames() {
			if l.In(v) {
				centered(l, strings.ToUpper(l.Name), st)
			}
		}
	}
	city := base
	city.Size *= 0.75
	city.Color = gogui.RGB(0xff, 0xff, 0xff)
	dot := reach * 0.06
	for _, l := range radar.CityNames() {
		if l.Rank > at.cityRank {
			break
		}
		if !l.In(v) {
			continue
		}
		pt := v.Pixel(float64(l.Lat), float64(l.Lon))
		x, y := float32(pt.X), float32(pt.Y)
		w, h := dc.TextWidth(l.Name, city), city.Size*1.25
		if !p.claim(area{x - dot, y - h/2, w + 3*dot, h}) {
			continue
		}
		dc.FilledCircle(x, y, dot*1.5, gogui.RGBA(0, 0, 0, 0xb0))
		dc.FilledCircle(x, y, dot, gogui.RGB(0xff, 0xff, 0xff))
		haloText(dc, x+2*dot, y-h/2, l.Name, city)
	}
	if at.counties {
		st := base
		st.Size *= 0.6
		st.Color = gogui.RGBA(0xff, 0xff, 0xff, 0xa0)
		for _, l := range radar.CountyNames() {
			if l.In(v) {
				centered(l, l.Name, st)
			}
		}
	}
}

func borders(
	dc *gogui.DrawContext,
	v radar.View,
	set []radar.Outline,
	tone gogui.Color,
	width float32,
) {
	for _, o := range set {
		if !o.Touches(v) {
			continue
		}
		pts := make([]float32, 0, 2*len(o.Points))
		for _, p := range o.Points {
			at := v.Pixel(float64(p[0]), float64(p[1]))
			pts = append(pts, float32(at.X), float32(at.Y))
		}
		dc.Polyline(pts, tone, width)
	}
}

func (r *Radar) marks(m radar.Map, focus *nws.Alert, vw, vh int) gogui.View {
	reach := style.Reach()
	accent := r.o.UI.Palette().Accent
	ring := gogui.RGB(accent.R, accent.G, accent.B)
	var outline []image.Point
	if focus != nil {
		for _, p := range focus.Outline {
			outline = append(outline, m.View.Pixel(p[0], p[1]))
		}
	}
	var home *image.Point
	if m.Home != nil {
		p := m.View.Pixel(m.Home[0], m.Home[1])
		home = &p
	}
	tone := gogui.RGB(0xff, 0x8a, 0x1f)
	if focus != nil && focus.Severity >= nws.Extreme {
		tone = gogui.RGB(0xff, 0x3b, 0x30)
	}
	return gogui.DrawCanvas(gogui.DrawCanvasCfg{
		ID:      "radar-marks",
		Width:   float32(vw),
		Height:  float32(vh),
		Sizing:  gogui.FixedFixed,
		Version: uint64(m.Built.UnixNano()),
		OnDraw: func(dc *gogui.DrawContext) {
			if m.Miles <= countyLinesMiles {
				borders(
					dc,
					m.View,
					radar.Counties(),
					gogui.RGBA(0xff, 0xff, 0xff, 0x40),
					max(1, reach*0.015),
				)
			}
			borders(
				dc,
				m.View,
				radar.States(),
				gogui.RGBA(0xff, 0xff, 0xff, 0x99),
				max(1.5, reach*0.03),
			)
			if len(outline) > 1 {
				pts := make([]float32, 0, 2*len(outline)+2)
				for _, p := range append(outline, outline[0]) {
					pts = append(pts, float32(p.X), float32(p.Y))
				}
				dc.Polyline(pts, tone, max(3, reach*0.06))
			}
			places(dc, m.View, m.Miles, home, vw, vh)
			if home != nil {
				x, y := float32(home.X), float32(home.Y)
				dc.FilledCircle(x, y, reach*0.16, gogui.RGB(0xff, 0xff, 0xff))
				dc.FilledCircle(x, y, reach*0.11, ring)
			}
		},
	})
}

func (r *Radar) clock(t time.Time) string {
	if r.o.Clock24 != nil && r.o.Clock24() {
		return t.Local().Format("15:04")
	}
	return t.Local().Format("3:04")
}

func (r *Radar) chip(words string) gogui.View {
	reach := style.Reach()
	text := gogui.CurrentTheme().Cfg.TextStyleDef
	text.Color = gogui.RGB(0xff, 0xff, 0xff)
	return gogui.Column(gogui.ContainerCfg{
		Float:        true,
		FloatOffsetX: reach * 1.6,
		FloatOffsetY: reach * 0.4,
		Sizing:       gogui.FitFit,
		Padding:      gogui.NewPadding(reach*0.15, reach*0.3, reach*0.15, reach*0.3),
		Radius:       gogui.RadiusMedium,
		Color:        gogui.RGBA(0, 0, 0, 0x99),
		Content:      []gogui.View{gogui.Label(words, text)},
	})
}

func (r *Radar) credit(m radar.Map, box ui.Rect) gogui.View {
	small := widgets.Secondary()
	small.Size *= 0.75
	return widgets.Placed(
		ui.Rect{X: 0, Y: box.H - int(small.Size*2), W: box.W, H: int(small.Size * 2)},
		gogui.Row(gogui.ContainerCfg{
			Sizing:  gogui.FillFill,
			Padding: gogui.NewPadding(0, small.Size, 0, 0),
			HAlign:  gogui.HAlignRight,
			Content: []gogui.View{gogui.Label(say.T("radar.credit."+m.Credit), small)},
		}),
	)
}

func (r *Radar) keys(
	box ui.Rect,
	rng schema.RadarRange,
	paused, atHome bool,
	home *[2]float64,
) gogui.View {
	reach := style.Reach()
	ranges := schema.RadarRanges()
	at := max(0, slices.Index(ranges, rng))
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family = icons.Family
	glyph.Size = reach * 0.55
	glyph.Color = gogui.RGB(0xff, 0xff, 0xff)
	key := func(id, icon string, do func()) gogui.View {
		st := glyph
		cfg := gogui.ContainerCfg{
			ID:     id,
			Width:  reach * 1.1,
			Height: reach * 1.1,
			Sizing: gogui.FixedFixed,
			Radius: gogui.RadiusMedium,
			Color:  gogui.RGBA(0, 0, 0, 0x99),
			HAlign: gogui.HAlignCenter,
			VAlign: gogui.VAlignMiddle,
		}
		if do == nil {
			st.Color = gogui.RGBA(0xff, 0xff, 0xff, 0x55)
		} else {
			cfg.OnClick = func(e gogui.EventCtx) {
				do()
				e.Consume()
			}
		}
		cfg.Content = []gogui.View{gogui.Label(icons.Glyph(icons.Named(icon)), st)}
		return gogui.Column(cfg)
	}
	var goHome func()
	if !atHome {
		goHome = func() { r.home(home) }
	}
	label := gogui.CurrentTheme().Cfg.TextStyleDef
	label.Color = gogui.RGB(0xff, 0xff, 0xff)
	play := "pause"
	if paused {
		play = "play"
	}
	row := []gogui.View{
		key("radar-out", "minus", func() { r.zoom(ranges, at+1) }),
		gogui.Column(gogui.ContainerCfg{
			Height:  reach * 1.1,
			Sizing:  gogui.FitFixed,
			Padding: gogui.NewPadding(0, reach*0.2, 0, reach*0.2),
			VAlign:  gogui.VAlignMiddle,
			Content: []gogui.View{gogui.Label(rng.Label(), label)},
		}),
		key("radar-in", "plus", func() { r.zoom(ranges, at-1) }),
		key("radar-play", play, r.toggle),
		key("radar-home", "home", goHome),
	}
	return widgets.Placed(
		ui.Rect{X: int(reach * 0.4), Y: box.H - int(reach*1.8), W: box.W / 2, H: int(reach * 1.4)},
		gogui.Row(gogui.ContainerCfg{
			Sizing:    gogui.FitFit,
			Padding:   gogui.NoPadding,
			Spacing:   gogui.SpacingPx(reach * 0.2),
			VAlign:    gogui.VAlignMiddle,
			Content:   row,
			OnGesture: swallow,
		}))
}

func (r *Radar) close() gogui.View {
	reach := style.Reach()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family = icons.Family
	glyph.Size = reach * 0.6
	glyph.Color = gogui.RGB(0xff, 0xff, 0xff)
	return widgets.Placed(
		ui.Rect{X: int(reach * 0.3), Y: int(reach * 0.3), W: int(reach * 1.1), H: int(reach * 1.1)},
		gogui.Column(gogui.ContainerCfg{
			ID:      "radar-close",
			Sizing:  gogui.FillFill,
			Radius:  gogui.RadiusMedium,
			Color:   gogui.RGBA(0, 0, 0, 0x99),
			HAlign:  gogui.HAlignCenter,
			VAlign:  gogui.VAlignMiddle,
			Content: []gogui.View{gogui.Label(icons.Glyph(icons.Named("arrow-left")), glyph)},
			OnClick: func(e gogui.EventCtx) {
				r.o.Shell.Remove(r.view)
				e.Consume()
			},
		}),
	)
}

func (r *Radar) zoom(ranges []schema.RadarRange, i int) {
	if i < 0 || i >= len(ranges) {
		return
	}
	r.mu.Lock()
	r.rng, r.start = ranges[i], time.Now()
	r.mu.Unlock()
}

func (r *Radar) toggle() {
	r.mu.Lock()
	r.paused = !r.paused
	r.pick = time.Time{}
	r.start = time.Now()
	r.mu.Unlock()
}

func (r *Radar) scrub(at time.Time) {
	r.mu.Lock()
	r.paused, r.pick = true, at
	r.mu.Unlock()
}

func (r *Radar) scrubber(box ui.Rect, at int, m radar.Map, pick time.Time) gogui.View {
	reach := style.Reach()
	first := m.Times[0]
	shown := m.Frames[at].At
	if !pick.IsZero() {
		shown = pick
	}
	bar := r.o.UI.Grip(r.o.UI.Toolkit.SeekBar(gogui.SliderCfg{
		ID:     "radar-scrub",
		Height: reach,
		Sizing: gogui.FillFit,
		Value:  float32(shown.Sub(first).Minutes()),
		Max:    float32(m.Times[len(m.Times)-1].Sub(first).Minutes()),
		OnChange: func(v float32, e gogui.EventCtx) {
			r.scrub(closest(m.Times, first.Add(time.Duration(float64(v)*float64(time.Minute)))))
			e.Window.InvalidateLayout()
		},
	}, nil))
	side := int(reach * 0.6)
	pad := reach * 0.3
	width := float32(box.W-2*side) - 2*pad
	tall := reach * 0.85
	return widgets.Placed(
		ui.Rect{X: side, Y: box.H - int(reach*3.8), W: box.W - 2*side, H: int(reach*1.1 + tall)},
		gogui.Column(gogui.ContainerCfg{
			Sizing:    gogui.FillFill,
			Padding:   gogui.NewPadding(0, pad, 0, pad),
			Radius:    gogui.RadiusMedium,
			Color:     gogui.RGBA(0, 0, 0, 0x66),
			Content:   []gogui.View{bar, r.ticks(m, shown, width, tall)},
			OnGesture: swallow,
		}))
}

func closest(times []time.Time, t time.Time) time.Time {
	best := times[0]
	for _, c := range times {
		if c.Sub(t).Abs() < best.Sub(t).Abs() {
			best = c
		}
	}
	return best
}

func nearest(frames []radar.Frame, t time.Time) int {
	best := 0
	for i, f := range frames {
		if f.At.Sub(t).Abs() < frames[best].At.Sub(t).Abs() {
			best = i
		}
	}
	return best
}

func (r *Radar) ticks(m radar.Map, current time.Time, width, height float32) gogui.View {
	reach := style.Reach()
	inset := reach * 0.35
	text := gogui.CurrentTheme().Cfg.TextStyleDef
	text.Size *= 0.7
	text.Color = gogui.RGB(0xff, 0xff, 0xff)
	accent := r.o.UI.Palette().Accent
	marked := gogui.RGB(accent.R, accent.G, accent.B)
	past := gogui.RGBA(0xff, 0xff, 0xff, 0x99)
	ahead := gogui.RGBA(0xff, 0xff, 0xff, 0x50)
	waiting := gogui.RGBA(0xff, 0xff, 0xff, 0x22)
	frames := m.Frames
	loaded := make(map[int64]bool, len(frames))
	for _, f := range frames {
		loaded[f.At.Unix()] = true
	}
	first, last := m.Times[0], m.Times[len(m.Times)-1]
	return gogui.DrawCanvas(gogui.DrawCanvasCfg{
		ID:      "radar-ticks",
		Width:   width,
		Height:  height,
		Sizing:  gogui.FixedFixed,
		Version: uint64(current.Unix())<<20 ^ uint64(len(frames)) ^ uint64(m.Built.UnixNano()),
		OnDraw: func(dc *gogui.DrawContext) {
			span := last.Sub(first)
			if span <= 0 {
				return
			}
			track := width - 2*inset
			xAt := func(t time.Time) float32 {
				return inset + track*float32(t.Sub(first))/float32(span)
			}
			thin := max(2, reach*0.04)
			for _, t := range m.Times {
				tone := past
				switch {
				case t.Equal(current):
					tone = marked
				case !loaded[t.Unix()]:
					tone = waiting
				case t.After(m.Now):
					tone = ahead
				}
				x := xAt(t)
				dc.Line(x, 0, x, height*0.22, tone, thin)
			}
			now := xAt(m.Now)
			dc.Line(now, 0, now, height*0.5, gogui.RGB(0xff, 0xff, 0xff), thin*1.5)
			every := 30 * time.Minute
			wide := dc.TextWidth(r.clock(first), text) * 1.6
			for every < 6*time.Hour && track*float32(every)/float32(span) < wide {
				every *= 2
			}
			for t := first.Truncate(every).Add(every); !t.After(last); t = t.Add(every) {
				x := xAt(t)
				label := r.clock(t)
				w := dc.TextWidth(label, text)
				dc.Line(x, 0, x, height*0.4, past, thin)
				dc.Text(min(max(0, x-w/2), width-w), height*0.4+reach*0.04, label, text)
			}
		},
	})
}

func (r *Radar) keep(slot, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.images == nil {
		r.images = map[string]string{}
	}
	if old, ok := r.images[slot]; ok && old != key {
		gogui.DropImage(old)
	}
	r.images[slot] = key
}

type backdrop struct {
	m   radar.Map
	key string
	img *image.NRGBA
}

func (r *Radar) behind(m radar.Map, complete bool, size image.Point) (backdrop, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if complete {
		if !r.back.m.Built.Equal(m.Built) {
			if r.back.key != "" {
				gogui.DropImage(r.back.key)
			}
			img := m.Frames[latest(m)].Image
			r.back = backdrop{m: m, img: img, key: fmt.Sprintf("radar-back/%p", img)}
		}
		return backdrop{}, false
	}
	b := r.back
	return b, b.img != nil && !b.m.Built.Equal(m.Built) && b.m.View.Size == size
}

func (r *Radar) forget(m radar.Map) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.Built.Equal(r.built) {
		return
	}
	for _, k := range r.images {
		gogui.DropImage(k)
	}
	r.images, r.built = nil, m.Built
}

func center(a *nws.Alert) *[2]float64 {
	if a == nil || len(a.Outline) == 0 {
		return nil
	}
	var lat, lon float64
	for _, p := range a.Outline {
		lat += p[0]
		lon += p[1]
	}
	n := float64(len(a.Outline))
	return &[2]float64{lat / n, lon / n}
}
