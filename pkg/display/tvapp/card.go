package tvapp

import (
	"context"
	"slices"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	cardUnits  = 13
	cardShare  = 0.85
	frameShare = 0.45
)

func (a *App) channel(w *gogui.Window) gogui.View {
	a.mu.Lock()
	name := a.opened
	a.mu.Unlock()
	vw, vh := w.WindowSize()
	scrim := gogui.ContainerCfg{
		Width:   float32(vw),
		Height:  float32(vh),
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Color:   gogui.Black.WithOpacity(0.5),
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		OnClick: func(e gogui.EventCtx) {
			a.o.Shell.Remove(a.card)
			e.Consume()
		},
	}
	channels, tvs, _, _ := a.o.Guide.Load()
	i := slices.IndexFunc(channels, func(c yttv.Channel) bool { return c.Key() == name })
	if i >= 0 && len(tvs) > 0 {
		width := min(float32(vw)*cardShare, style.Reach()*cardUnits)
		scrim.Content = []gogui.View{
			a.details(w, channels[i], a.current(tvs), width, float32(vh)*frameShare),
		}
	}
	return gogui.Column(scrim)
}

func (a *App) details(
	w *gogui.Window,
	c yttv.Channel,
	tv yttv.TV,
	width, tallest float32,
) gogui.View {
	reach := style.Reach()
	t := gogui.CurrentTheme()
	body, small := t.Cfg.TextStyleDef, widgets.Secondary()
	inner := width - 2*gogui.PadLarge
	now := time.Now()

	frameW := min(inner, tallest*16/9)
	src := c.LiveFrame(now)
	if src == "" {
		src = c.Thumb
	}
	views := []gogui.View{gogui.Row(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, HAlign: gogui.HAlignCenter,
		Content: []gogui.View{a.picture(a.thumb, src, frameW, frameW*9/16, "mdi:television")},
	})}

	favorite := slices.Contains(a.o.Guide.Favorites(), c.Key())
	star := "mdi:star-outline"
	if favorite {
		star = "mdi:star"
	}
	logo := reach * 0.7
	views = append(views, gogui.Row(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
		VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{
			a.picture(a.logo, c.Icon, logo, logo, "mdi:television-classic"),
			gogui.Label(widgets.Ellipsize(w, c.Name, t.TextStyleTitle, inner-logo-reach*2),
				t.TextStyleTitle),
			gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding}),
			a.mdiButton("tv-favorite", star, func() {
				if err := a.o.Guide.SetFavorite(c.Key(), !favorite); err == nil {
					a.redraw()
				}
			}),
		},
	}))

	if on, ok := c.At(now); ok {
		views = append(views, gogui.Label(widgets.Ellipsize(w, on.Title, body, inner), body))
		if on.Subtitle != "" {
			views = append(
				views,
				gogui.Label(widgets.Ellipsize(w, on.Subtitle, small, inner), small),
			)
		}
		if start, end, ok := on.Span(); ok {
			views = append(views,
				gogui.Label(a.clock(start.Local())+" – "+a.clock(end.Local()), small),
				a.progress(on, inner+reach*0.3, now))
		}
	} else {
		views = append(views, gogui.Label(say.T("tv.empty"), small))
	}
	if next, ok := upcoming(c, now); ok {
		start, _, _ := next.Span()
		views = append(views, gogui.Label(widgets.Ellipsize(w,
			say.F("tv.next", map[string]any{"Time": a.clock(start.Local()), "Title": next.Title}),
			small, inner), small))
	}

	views = append(views, a.watch(tv, c))

	return gogui.Column(widgets.Blocking(a.o.UI.Toolkit.Panel(gogui.ContainerCfg{
		ID:      "tv-card",
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.PaddingLarge,
		Spacing: gogui.SpacingMedium,
		Radius:  gogui.RadiusLarge,
		Clip:    true,
		Content: views,
	})))
}

func (a *App) watch(tv yttv.TV, channel yttv.Channel) gogui.View {
	cfg := gogui.ContainerCfg{
		ID:      "tv-watch",
		Sizing:  gogui.FillFit,
		Padding: gogui.PaddingMedium,
		Radius:  gogui.RadiusLarge,
		HAlign:  gogui.HAlignCenter,
	}
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	a.o.UI.Toolkit.Tab(&cfg, &st, widgets.Chosen(true))
	cfg.Content = []gogui.View{
		gogui.Label(say.F("tv.watch", map[string]any{"TV": a.label(tv)}), st),
	}
	entity := tv.Entity
	return a.o.UI.Toolkit.Pressable(gogui.Row, cfg, func(e gogui.EventCtx) {
		a.act(func(ctx context.Context) error { return a.o.Guide.Cast(ctx, entity, channel) })
		a.o.Shell.Remove(a.card)
		e.Consume()
	})
}

func upcoming(c yttv.Channel, now time.Time) (yttv.Airing, bool) {
	var (
		best  yttv.Airing
		first time.Time
	)
	for _, on := range c.Airings() {
		if start, _, ok := on.Span(); ok && start.After(now) &&
			(first.IsZero() || start.Before(first)) {
			best, first = on, start
		}
	}
	return best, !first.IsZero()
}
