package ringsview

import (
	"math"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
)

type tone struct{ fill, ink theme.Color }

func kindColor(pal theme.Theme, kind string) theme.Color {
	switch kind {
	case "lights":
		return pal.Roles.Warm
	case "switches":
		return pal.Success
	case "media":
		return pal.Roles.Media
	case "climate":
		return pal.Roles.Heat
	case "fans":
		return pal.Roles.Air
	case "covers":
		return pal.Roles.Cool
	case "locks":
		return pal.Danger
	case "access":
		return pal.Warning
	case "presence":
		return pal.Roles.Motion
	case "cameras":
		return pal.Roles.Choice
	}
	return pal.Accent
}

func toneOf(pal theme.Theme, r *homecontrol.Ring) tone {
	room, hued := roomColor(pal, r.Hue)
	shade := func(t float32) theme.Color {
		if !hued {
			return mix(pal.Surface, pal.Background, t*0.4)
		}
		return mix(room, pal.Surface, t)
	}
	var fill theme.Color
	switch {
	case r.Device != nil && r.Device.Active:
		fill = kindColor(pal, r.Kind)
	case r.Device != nil && !r.Device.Available:
		fill = mix(pal.Surface, pal.Background, 0.55)
	case r.Device != nil:
		fill = shade(0.82)
	case r.Level == homecontrol.LevelKind && r.Active > 0:
		fill = mix(kindColor(pal, r.Kind), pal.Background, 0.35)
	case r.Level == homecontrol.LevelKind:
		fill = shade(0.7)
	case r.Level == homecontrol.LevelArea:
		fill = shade(0.5)
	case r.Level == homecontrol.LevelFloor && r.Hue > 0:
		hues := pal.Roles.All()
		fill = mix(hues[len(hues)-1-(r.Hue-1)%len(hues)], pal.Surface, 0.3)
	default:
		fill = mix(pal.Surface, pal.Text, 0.06)
	}
	return tone{fill: fill, ink: pal.Contrast(fill)}
}

func roomColor(pal theme.Theme, hue int) (theme.Color, bool) {
	if hue <= 0 {
		return theme.Color{}, false
	}
	hues := pal.Roles.All()
	return hues[(hue-1)%len(hues)], true
}

func mix(a, b theme.Color, t float32) theme.Color {
	m := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return theme.Color{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B)}
}

func wedge(g geometry, sh shape) *gogui.CanvasPath {
	var p gogui.CanvasPath
	n := g.steps(sh.a1 - sh.a0)
	for i := 0; i <= n; i++ {
		x, y := g.point(lerp(sh.a0, sh.a1, float32(i)/float32(n)), sh.r1)
		if i == 0 {
			p.MoveTo(x, y)
			continue
		}
		p.LineTo(x, y)
	}
	for i := n; i >= 0; i-- {
		x, y := g.point(lerp(sh.a0, sh.a1, float32(i)/float32(n)), sh.r0)
		p.LineTo(x, y)
	}
	p.Close()
	return &p
}

func caption(dc *gogui.DrawContext, g geometry, sh shape, text string, st gogui.TextStyle) {
	am, rm := (sh.a0+sh.a1)/2, (sh.r0+sh.r1)/2
	ix, iy := g.point(am, sh.r0)
	ox, oy := g.point(am, sh.r1)
	lx, ly := g.point(sh.a0, rm)
	rx, ry := g.point(sh.a1, rm)
	along := float32(math.Hypot(float64(ox-ix), float64(oy-iy)))
	across := float32(math.Hypot(float64(rx-lx), float64(ry-ly)))
	ux, uy := (ox-ix)/max(along, 1e-6), (oy-iy)/max(along, 1e-6)
	room, thick := along, across
	if across > along {
		ux, uy = (rx-lx)/max(across, 1e-6), (ry-ly)/max(across, 1e-6)
		room, thick = across, along
	}
	if ux < 0 || (ux == 0 && uy < 0) {
		ux, uy = -ux, -uy
	}
	pad := st.Size * 0.6
	room -= 2 * pad
	if thick < st.Size*1.1 || room < st.Size*2 {
		return
	}
	text = fit(dc, text, st, room)
	if text == "" {
		return
	}
	mx, my := g.point(am, rm)
	tw, th := dc.TextWidth(text, st), dc.FontHeight(st)
	vx, vy := -uy, ux
	st.RotationRadians = float32(math.Atan2(float64(uy), float64(ux)))
	dc.Text(mx-ux*tw/2-vx*th/2, my-uy*tw/2-vy*th/2, text, st)
}

func fit(dc *gogui.DrawContext, text string, st gogui.TextStyle, room float32) string {
	if dc.TextWidth(text, st) <= room {
		return text
	}
	runes := []rune(text)
	for n := len(runes) - 1; n > 0; n-- {
		if t := string(runes[:n]) + "…"; dc.TextWidth(t, st) <= room {
			return t
		}
	}
	return ""
}
