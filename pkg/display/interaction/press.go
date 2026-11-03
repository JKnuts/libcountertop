package interaction

import (
	"math"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"
)

const (
	tapDelay   = 150 * time.Millisecond
	pressFlash = 64 * time.Millisecond
	tapShow    = 150 * time.Millisecond
	rippleGrow = 300 * time.Millisecond
	rippleFade = 250 * time.Millisecond
	rippleTick = 16 * time.Millisecond
	inkAlpha   = 80
)

type Presses struct {
	armed   string
	pending func(gogui.EventCtx)
	wave    ripple
	waveGen int
	moving  bool
}

func New() *Presses { return &Presses{} }

type ripple struct {
	id         string
	x, y, w, h float32
	radius     float32
	ink        gogui.Color
	lit, up    time.Time
	gen        int
}

func (r ripple) done(now time.Time) bool {
	return r.id == "" || r.lit.IsZero() || !r.up.IsZero() && now.Sub(r.up) >= rippleFade
}

func (r ripple) still(now time.Time) bool {
	return r.done(now) || r.up.IsZero() && now.Sub(r.lit) >= rippleGrow
}

func (p *Presses) animate(w *gogui.Window) {
	if p.moving {
		return
	}
	p.moving = true
	var step func()
	step = func() {
		w.QueueCommand(func(w *gogui.Window) {
			w.InvalidateLayout()
			if p.wave.still(time.Now()) {
				p.moving = false
				return
			}
			time.AfterFunc(rippleTick, step)
		})
	}
	step()
}

func (p *Presses) light(w *gogui.Window, gen int) {
	if p.wave.gen != gen || !p.wave.lit.IsZero() {
		return
	}
	p.wave.lit = time.Now()
	p.animate(w)
}

func (p *Presses) Release(e gogui.EventCtx) {
	if p.armed == "" {
		return
	}
	p.armed, p.pending = "", nil
	if !p.wave.lit.IsZero() && p.wave.up.IsZero() {
		p.wave.up = time.Now()
		p.animate(e.Window)
	} else {
		p.wave = ripple{}
	}
	e.Window.InvalidateLayout()
}

func overlay(r ripple, now time.Time) gogui.View {
	reach := float32(math.Hypot(float64(max(r.x, r.w-r.x)), float64(max(r.y, r.h-r.y))))
	grown := min(float32(now.Sub(r.lit))/float32(rippleGrow), 1)
	grown = 1 - (1-grown)*(1-grown)
	alpha := float32(1)
	if !r.up.IsZero() && now.After(r.up) {
		alpha = max(1-float32(now.Sub(r.up))/float32(rippleFade), 0)
	}
	ink := r.ink
	ink.A = uint8(inkAlpha * min(alpha, 1))
	return gogui.DrawCanvas(gogui.DrawCanvasCfg{
		Width: r.w, Height: r.h, Clip: true, Radius: r.radius,
		Version: uint64(now.UnixNano()),
		OnDraw: func(dc *gogui.DrawContext) {
			if grown >= 1 {
				dc.FilledRoundedRect(0, 0, r.w, r.h, r.radius, ink)
				return
			}
			dc.FilledPolygon(Disc(r.x, r.y, reach*grown), ink)
		},
	})
}

func inkFor(cfg gogui.ContainerCfg) gogui.Color {
	if cfg.Color.A > 0 {
		return Contrast(cfg.Color)
	}
	return gogui.CurrentTheme().Cfg.TextStyleDef.Color
}

func Contrast(c gogui.Color) gogui.Color {
	if 0.299*float32(c.R)+0.587*float32(c.G)+0.114*float32(c.B) > 140 {
		return gogui.RGB(0, 0, 0)
	}
	return gogui.RGB(0xff, 0xff, 0xff)
}

func Disc(cx, cy, radius float32) []float32 {
	const sides = 96
	points := make([]float32, 0, sides*2)
	for i := range sides {
		a := 2 * math.Pi * float64(i) / sides
		points = append(points, cx+radius*float32(math.Cos(a)), cy+radius*float32(math.Sin(a)))
	}
	return points
}

func (p *Presses) Pressable(
	layout func(gogui.ContainerCfg) gogui.View,
	cfg gogui.ContainerCfg,
	do func(gogui.EventCtx),
) gogui.View {
	return p.Inked(layout, cfg, inkFor(cfg), do)
}

func (p *Presses) Inked(
	layout func(gogui.ContainerCfg) gogui.View,
	cfg gogui.ContainerCfg,
	ink gogui.Color,
	do func(gogui.EventCtx),
) gogui.View {
	id := cfg.ID
	radius := cfg.Radius.Or(gogui.CurrentTheme().Cfg.RadiusMedium)
	if do == nil || cfg.Disabled {
		return layout(cfg)
	}
	if now := time.Now(); p.wave.id == id && !p.wave.done(now) {
		cfg.Content = append(cfg.Content, gogui.Column(gogui.ContainerCfg{
			Float: true, Padding: gogui.NoPadding,
			Content: []gogui.View{overlay(p.wave, now)},
		}))
	}
	cfg.OnClick = func(e gogui.EventCtx) {
		p.armed = id
		p.waveGen++
		gen := p.waveGen
		s := e.Layout.Shape
		p.wave = ripple{
			id:     id,
			x:      e.Event.MouseX - s.X,
			y:      e.Event.MouseY - s.Y,
			w:      s.Width,
			h:      s.Height,
			radius: radius,
			ink:    ink,
			gen:    gen,
		}
		p.pending = do
		w := e.Window
		time.AfterFunc(
			tapDelay,
			func() { w.QueueCommand(func(w *gogui.Window) { p.light(w, gen) }) },
		)
		e.Consume()
	}
	fire := func(e gogui.EventCtx) {
		if p.armed == id {
			p.Lift(e.Window)
			e.Consume()
		}
	}
	cfg.OnMouseUp = fire
	inner := cfg.OnGesture
	cfg.OnGesture = func(e gogui.EventCtx) {
		if inner != nil {
			inner(e)
		}
		switch {
		case e.Event == nil:
		case e.Event.IsHandled:
			if p.armed == id {
				p.Release(e)
			}
		case e.Event.GestureType == gogui.GesturePan:
			if p.armed == id {
				p.Release(e)
			}
		case e.Event.GestureType == gogui.GestureTap,
			e.Event.GestureType == gogui.GestureLongPress && e.Event.GesturePhase == gogui.GesturePhaseEnded:
			fire(e)
		}
	}
	return layout(cfg)
}

func (p *Presses) Lift(w *gogui.Window) {
	if p.armed == "" {
		return
	}
	p.armed = ""
	do := p.pending
	p.pending = nil
	if !p.wave.lit.IsZero() {
		p.wave.up = time.Now()
		p.animate(w)
		do(gogui.EventCtx{Window: w})
		w.InvalidateLayout()
		return
	}
	now := time.Now()
	p.wave.lit = now.Add(-rippleGrow)
	p.wave.up = now.Add(tapShow)
	p.animate(w)
	gen := p.wave.gen
	time.AfterFunc(pressFlash, func() {
		w.QueueCommand(func(w *gogui.Window) {
			if p.wave.gen == gen {
				do(gogui.EventCtx{Window: w})
				w.InvalidateLayout()
			}
		})
	})
}

func Tapped(tap func(int), level int) func(gogui.EventCtx) {
	if tap == nil {
		return nil
	}
	return func(gogui.EventCtx) { tap(level) }
}

func (p *Presses) Button(
	id string,
	glyph string,
	st gogui.TextStyle,
	fill gogui.Color,
	do func(gogui.EventCtx),
) gogui.View {
	side := st.Size * 2
	return p.Pressable(gogui.Row, gogui.ContainerCfg{
		ID:      id,
		Width:   side,
		Height:  side,
		Sizing:  gogui.FixedFixed,
		Color:   fill,
		Radius:  gogui.RadiusLarge,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(glyph, st)},
	}, do)
}
