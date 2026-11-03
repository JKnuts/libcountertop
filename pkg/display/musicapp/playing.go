package musicapp

import (
	"context"
	"strconv"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/playerview"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/musicassistant"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func (a *App) playing(
	w *gogui.Window,
	list []*musicassistant.Speaker,
	s *musicassistant.Speaker,
) gogui.View {
	vw, vh := w.WindowSize()
	reach := style.Reach()
	wide := vw >= vh
	now := s.Now()
	side := min(float32(vh)*0.42, float32(vw)*0.3)
	if !wide {
		side = min(float32(vw)*0.55, float32(vh)*0.3)
	}
	title := gogui.CurrentTheme().Cfg.TextStyleDef
	title.Size *= 1.5
	room := side * 1.6
	shown, under := orQuiet(now.Title), now.Artist
	if p, ok := a.starting(s); ok {
		under = say.T("music.starting")
		if p.title != "" {
			shown = p.title
		}
	}
	text := []gogui.View{
		gogui.Label(widgets.Ellipsize(w, shown, title, room), title),
	}
	if under != "" {
		text = append(text, gogui.Label(
			widgets.Ellipsize(w, under, widgets.Secondary(), room), widgets.Secondary()))
	}
	controls := gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		HAlign:  gogui.HAlignCenter,
		Content: append([]gogui.View{
			a.chip(w, s, len(list)),
			a.cover(now.Picture, side),
		}, append(text,
			a.transport(s, now, reach),
			a.slider("music-volume", s, room),
		)...),
	})
	queue := a.queue(w, s, reach)
	cfg := gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.6),
		Content: []gogui.View{controls, queue},
	}
	if wide {
		cfg.VAlign = gogui.VAlignMiddle
		return gogui.Row(cfg)
	}
	cfg.HAlign = gogui.HAlignCenter
	return gogui.Column(cfg)
}

func (a *App) chip(w *gogui.Window, s *musicassistant.Speaker, n int) gogui.View {
	reach := style.Reach()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family, glyph.Size = icons.Family, reach*0.45
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	cfg := gogui.ContainerCfg{
		ID:      "music-speaker",
		Sizing:  gogui.FitFit,
		Padding: gogui.NewPadding(reach*0.15, reach*0.4, reach*0.15, reach*0.3),
		Spacing: gogui.SpacingSmall,
		VAlign:  gogui.VAlignMiddle,
	}
	a.o.UI.Toolkit.Tab(&cfg, &st, widgets.Chosen(false))
	glyph.Color = st.Color
	content := []gogui.View{
		gogui.Label(icons.Glyph(icons.Named("mdi:speaker")), glyph),
		gogui.Label(widgets.Ellipsize(w, s.Name, st, reach*6), st),
	}
	if n > 1 {
		content = append(content, gogui.Label(icons.Glyph(icons.Named("mdi:menu-down")), glyph))
	}
	cfg.Content = content
	return a.o.UI.Toolkit.Pressable(gogui.Row, cfg, func(e gogui.EventCtx) {
		a.open(speakersTab)
		e.Consume()
	})
}

func (a *App) transport(
	s *musicassistant.Speaker,
	now musicassistant.Playing,
	reach float32,
) gogui.View {
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family, glyph.Size = icons.Family, reach*0.7
	key := func(id, icon, action string, state style.State) gogui.View {
		return a.o.UI.Toolkit.KeyButton(id, icons.Glyph(icons.Named(icon)), glyph, state,
			func(e gogui.EventCtx) {
				entity := s.Entity
				if action != "media_play_pause" {
					a.start(s, "", "")
				}
				a.act(
					func(ctx context.Context) error { return a.o.Music.Command(ctx, entity, action) },
				)
				e.Consume()
			})
	}
	middle := "mdi:play"
	if now.State == "playing" {
		middle = "mdi:pause"
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.4),
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			key("music-previous", "mdi:skip-previous", "media_previous_track", style.Rest),
			key("music-play", middle, "media_play_pause", style.Chosen),
			key("music-next", "mdi:skip-next", "media_next_track", style.Rest),
		},
	})
}

func (a *App) slider(id string, s *musicassistant.Speaker, width float32) gogui.View {
	reach := style.Reach()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family, glyph.Size = icons.Family, reach*0.5
	glyph.Color = widgets.Secondary().Color
	entity := s.Entity
	bar := a.o.UI.Grip(a.o.UI.Toolkit.Slider(gogui.SliderCfg{
		ID:     id,
		Height: reach,
		Sizing: gogui.FillFixed,
		Value:  float32(a.volume(s)),
		Max:    1,
		OnChange: func(v float32, e gogui.EventCtx) {
			a.setVolume(entity, float64(v))
			e.Window.InvalidateLayout()
		},
	}))
	return gogui.Row(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(icons.Glyph(icons.Named("mdi:volume-high")), glyph), bar},
	})
}

func (a *App) queue(w *gogui.Window, s *musicassistant.Speaker, reach float32) gogui.View {
	tracks, err, loaded := a.o.Music.Queue(s.Entity)
	head := gogui.Label(say.T("music.queue"), widgets.Secondary())
	rows := []gogui.View{head}
	switch {
	case !loaded:
		rows = append(rows, gogui.Label(say.T("home.loading"), widgets.Secondary()))
	case err != nil:
		rows = append(rows, gogui.Label(say.T("music.unavailable"), widgets.Secondary()))
	}
	vw, _ := w.WindowSize()
	room := float32(vw) * 0.4
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	waiting, _ := a.starting(s)
	for i, t := range tracks {
		lines := []gogui.View{gogui.Label(widgets.Ellipsize(w, t.Title, body, room), body)}
		if t.Artist != "" {
			lines = append(lines, gogui.Label(
				widgets.Ellipsize(w, t.Artist, widgets.Secondary(), room), widgets.Secondary()))
		}
		content := []gogui.View{
			a.cover(t.Image, reach*1.2),
			gogui.Column(gogui.ContainerCfg{
				Sizing: gogui.FillFit, Padding: gogui.NoPadding, Clip: true, Content: lines,
			}),
		}
		if t.Duration > 0 {
			content = append(
				content,
				gogui.Label(playerview.ClockText(t.Duration), widgets.Secondary()),
			)
		}
		entity, id, name := s.Entity, t.ID, t.Title
		row := gogui.ContainerCfg{
			ID:      "music-queue-" + strconv.Itoa(i),
			Sizing:  gogui.FillFit,
			Padding: gogui.PaddingSmall,
			Spacing: gogui.SpacingMedium,
			Radius:  gogui.RadiusMedium,
			VAlign:  gogui.VAlignMiddle,
			Content: content,
		}
		if waiting.item != "" && waiting.item == id {
			a.o.UI.Toolkit.ListRow(&row, style.Chosen)
		}
		rows = append(rows, a.o.UI.Toolkit.Pressable(gogui.Row, row, func(e gogui.EventCtx) {
			a.start(s, id, name)
			a.act(func(ctx context.Context) error { return a.o.Music.Jump(ctx, entity, id) })
			e.Consume()
		}))
	}
	return gogui.Column(gogui.ContainerCfg{
		ID:         "music-queue",
		Sizing:     gogui.FillFill,
		Padding:    gogui.NewPadding(0, reach*0.4, 0, 0),
		Spacing:    gogui.SpacingSmall,
		Scrollable: true,
		OnGesture:  a.o.UI.HoldStill,
		Content:    rows,
	})
}

func orQuiet(title string) string {
	if title == "" {
		return say.T("music.quiet")
	}
	return title
}
