package tvapp

import (
	"context"
	"fmt"
	"slices"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	guideID   = "tv-guide"
	guideSpan = 150 * time.Minute
	guideSlot = 30 * time.Minute
	rowUnits  = 1.3
	nameUnits = 3.4
	nameShare = 0.3
	overscan  = 8
	blockGap  = 2
)

type window struct {
	from  time.Time
	width float32
}

func windowAt(now time.Time, width float32) window {
	from := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(),
		now.Minute()/30*30, 0, 0, now.Location())
	return window{from: from, width: width}
}

func (g window) to() time.Time { return g.from.Add(guideSpan) }

func (g window) x(t time.Time) float32 {
	switch {
	case !t.After(g.from):
		return 0
	case !t.Before(g.to()):
		return g.width
	}
	return float32(t.Sub(g.from).Seconds()/guideSpan.Seconds()) * g.width
}

type block struct {
	airing     yttv.Airing
	gap, width float32
	now        bool
}

func (g window) blocks(airings []yttv.Airing, now time.Time) []block {
	type span struct {
		a          yttv.Airing
		start, end time.Time
	}
	var spans []span
	for _, a := range airings {
		if start, end, ok := a.Span(); ok && end.After(g.from) && start.Before(g.to()) {
			spans = append(spans, span{a, start, end})
		}
	}
	slices.SortFunc(spans, func(x, y span) int { return x.start.Compare(y.start) })
	var (
		out    []block
		cursor float32
	)
	for _, s := range spans {
		left, right := max(g.x(s.start), cursor), g.x(s.end)
		if right <= left {
			continue
		}
		out = append(out, block{
			airing: s.a,
			gap:    left - cursor,
			width:  right - left,
			now:    !s.start.After(now) && s.end.After(now),
		})
		cursor = right
	}
	return out
}

func (a *App) guide(
	w *gogui.Window,
	tv yttv.TV,
	channels []yttv.Channel,
	width float32,
) gogui.View {
	reach := style.Reach()
	gap := gogui.CurrentTheme().Cfg.SpacingSmall
	pitch := reach * rowUnits
	l := lanes{name: min(reach*nameUnits, width*nameShare), gap: gap, height: pitch - gap}
	l.frame = (l.height - 2*gogui.PadSmall) * 16 / 9
	now := time.Now()
	g := windowAt(now, width-l.lead())
	_, vh := w.WindowSize()
	first, last := gogui.ListVisibleRange(len(channels), pitch, float32(vh), scrolled(w), overscan)
	var rows []gogui.View
	if first > 0 {
		rows = append(rows, spacer(width, float32(first)*pitch))
	}
	for i := first; i <= last && i < len(channels); i++ {
		rows = append(rows, a.row(w, tv, channels[i], g, l, pitch, now))
	}
	if rest := len(channels) - 1 - last; last >= 0 && rest > 0 {
		rows = append(rows, spacer(width, float32(rest)*pitch))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			a.times(g, l.lead(), float32(vh), now),
			gogui.Column(gogui.ContainerCfg{
				ID:         guideID,
				Sizing:     gogui.FillFill,
				Padding:    gogui.NoPadding,
				Spacing:    gogui.NoSpacing,
				Scrollable: true,
				ScrollMode: gogui.ScrollVerticalOnly,
				Clip:       true,
				Content:    rows,
			}),
		},
	})
}

type lanes struct {
	name, frame, gap, height float32
}

func (l lanes) lead() float32 { return l.name + l.gap + l.frame + l.gap }

func scrolled(w *gogui.Window) float32 {
	for _, id := range w.ResolveID(guideID) {
		return w.ScrollVerticalOffset(id)
	}
	return 0
}

func spacer(width, height float32) gogui.View {
	return gogui.Row(gogui.ContainerCfg{
		Width: width, Height: height, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
	})
}

func (a *App) times(g window, lead, height float32, now time.Time) gogui.View {
	small := widgets.Secondary()
	slot := g.width * float32(guideSlot.Seconds()/guideSpan.Seconds())
	labels := []gogui.View{spacer(lead, 1)}
	for t := g.from; t.Before(g.to()); t = t.Add(guideSlot) {
		labels = append(labels, gogui.Row(gogui.ContainerCfg{
			Width: slot, Sizing: gogui.FixedFit, Padding: gogui.NoPadding,
			Content: []gogui.View{gogui.Label(a.clock(t), small)},
		}))
	}
	color := widgets.Color(a.o.UI.Palette().Danger)
	arrow := widgets.IconStyle(color)
	arrow.Family, arrow.Size = icons.Family, style.Reach()*0.4
	line := max(style.Reach()*0.03, 2)
	at := lead + g.x(now)
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing,
		Content: []gogui.View{
			gogui.Row(gogui.ContainerCfg{
				Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing,
				Content: labels,
			}),
			gogui.Row(gogui.ContainerCfg{
				Height: arrow.Size * 0.5, Sizing: gogui.FillFixed,
				Padding: gogui.NoPadding, Spacing: gogui.NoSpacing,
				Content: []gogui.View{
					gogui.Row(gogui.ContainerCfg{
						Float:        true,
						FloatOffsetX: at - arrow.Size/2,
						FloatOffsetY: -arrow.Size * 0.3,
						Padding:      gogui.NoPadding,
						Content: []gogui.View{
							gogui.Label(icons.Glyph(icons.Named("mdi:menu-down")), arrow),
						},
					}),
					gogui.Row(gogui.ContainerCfg{
						Float:        true,
						FloatOffsetX: at - line/2,
						FloatOffsetY: arrow.Size * 0.3,
						Width:        line,
						Height:       height,
						Sizing:       gogui.FixedFixed,
						Padding:      gogui.NoPadding,
						Color:        color,
					}),
				},
			}),
		},
	})
}

func (a *App) clock(t time.Time) string {
	if a.o.Clock24 != nil && a.o.Clock24() {
		return t.Format("15:04")
	}
	return t.Format("3:04")
}

func (a *App) row(
	w *gogui.Window,
	tv yttv.TV,
	c yttv.Channel,
	g window,
	l lanes,
	pitch float32,
	now time.Time,
) gogui.View {
	name, height := l.name, l.height
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	fill := gogui.CurrentTheme().Cfg.ColorPanel
	if _, on := c.At(now); !on {
		body, fill = widgets.Secondary(), gogui.Color{}
	}
	logo := style.Reach() * 0.6
	text := name - logo - style.Reach()*0.4
	shown := []gogui.View{a.picture(a.logo, c.Icon, logo, logo, "mdi:television-classic")}
	if slices.Contains(a.o.Guide.Favorites(), c.Key()) {
		star := widgets.IconStyle(gogui.CurrentTheme().Cfg.ColorAccent)
		star.Family, star.Size = icons.Family, style.Reach()*0.32
		shown = append(shown, gogui.Label(icons.Glyph(icons.Named("mdi:star")), star))
		text -= star.Size + gogui.PadSmall
	}
	shown = append(shown, gogui.Label(widgets.Ellipsize(w, c.Name, body, text), body))
	label := gogui.ContainerCfg{
		ID: "tv-name-" + c.Key(), Width: name, Height: height, Sizing: gogui.FixedFixed,
		Padding: gogui.PaddingSmall, Spacing: gogui.SpacingSmall, VAlign: gogui.VAlignMiddle,
		Radius: gogui.RadiusMedium, Color: fill,
		Content: shown,
	}
	open := a.opener(c.Key())
	preview := spacer(l.frame, height)
	if src := c.LiveFrame(now); src != "" {
		frameH := height - 2*gogui.PadSmall
		shown := []gogui.View{a.picture(a.thumb, src, l.frame, frameH, "mdi:television")}
		if on, ok := c.At(now); ok {
			if done, ok := on.Progress(now); ok {
				shown = append(shown, bar(l.frame, l.frame*float32(done)))
			}
		}
		preview = a.o.UI.Toolkit.Pressable(gogui.Column, gogui.ContainerCfg{
			ID: "tv-frame-" + c.Key(), Width: l.frame, Height: height, Sizing: gogui.FixedFixed,
			Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, VAlign: gogui.VAlignMiddle,
			Content: shown,
		}, open)
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFixed,
		Height:  pitch,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			a.o.UI.Toolkit.Pressable(gogui.Row, label, open),
			preview,
			a.timeline(w, c, g, height, now, a.caster(tv, c)),
		},
	})
}

func (a *App) caster(tv yttv.TV, channel yttv.Channel) func(gogui.EventCtx) {
	entity := tv.Entity
	return func(e gogui.EventCtx) {
		a.act(func(ctx context.Context) error { return a.o.Guide.Cast(ctx, entity, channel) })
		e.Consume()
	}
}

func (a *App) opener(channel string) func(gogui.EventCtx) {
	return func(e gogui.EventCtx) {
		a.mu.Lock()
		a.opened = channel
		a.mu.Unlock()
		a.o.Shell.Push(a.card)
		e.Consume()
	}
}

func (a *App) timeline(
	w *gogui.Window,
	c yttv.Channel,
	g window,
	height float32,
	now time.Time,
	cast func(gogui.EventCtx),
) gogui.View {
	blocks := g.blocks(c.Airings(), now)
	small := widgets.Secondary()
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	if len(blocks) == 0 {
		return gogui.Row(gogui.ContainerCfg{
			Width: g.width, Height: height, Sizing: gogui.FixedFixed,
			Padding: gogui.PaddingSmall, VAlign: gogui.VAlignMiddle,
			Content: []gogui.View{gogui.Label(say.T("tv.empty"), small)},
		})
	}
	var cells []gogui.View
	for i, b := range blocks {
		if b.gap > 0 {
			cells = append(cells, spacer(b.gap, height))
		}
		inner := max(b.width-blockGap, 1)
		room := inner - style.Reach()*0.3
		title := body
		if !b.now {
			title = small
		}
		var text []gogui.View
		if room > style.Reach()*0.5 {
			text = append(
				text,
				gogui.Label(widgets.Ellipsize(w, b.airing.Title, title, room), title),
			)
			if b.airing.Subtitle != "" {
				text = append(text,
					gogui.Label(widgets.Ellipsize(w, b.airing.Subtitle, small, room), small))
			}
		}
		cfg := gogui.ContainerCfg{
			ID:    fmt.Sprintf("tv-air-%s-%d", c.Key(), i),
			Width: inner, Height: height, Sizing: gogui.FixedFixed,
			Padding: gogui.PaddingSmall, VAlign: gogui.VAlignMiddle, Clip: true,
			Radius: gogui.RadiusMedium, Color: gogui.CurrentTheme().Cfg.ColorPanel,
			Content: text,
		}
		cell := gogui.Column(cfg)
		if b.now {
			cell = a.o.UI.Toolkit.Pressable(gogui.Column, cfg, cast)
		}
		cells = append(cells, cell, spacer(b.width-inner, height))
	}
	return gogui.Row(gogui.ContainerCfg{
		Width: g.width, Height: height, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
		Spacing: gogui.NoSpacing, Clip: true, Content: cells,
	})
}

func (a *App) progress(on yttv.Airing, width float32, now time.Time) gogui.View {
	done, ok := on.Progress(now)
	if !ok {
		return spacer(1, 1)
	}
	width = max(width-style.Reach()*0.3, 1)
	return bar(width, width*float32(done))
}

func bar(width, filled float32) gogui.View {
	t := gogui.CurrentTheme().Cfg
	height := style.Reach() * 0.05
	return gogui.Row(gogui.ContainerCfg{
		Width: width, Height: height, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
		Radius: gogui.RadiusPx(height / 2), Color: t.ColorBackground, Clip: true,
		Content: []gogui.View{gogui.Row(gogui.ContainerCfg{
			Width: filled, Height: height, Sizing: gogui.FixedFixed,
			Padding: gogui.NoPadding, Color: t.ColorAccent,
		})},
	})
}
