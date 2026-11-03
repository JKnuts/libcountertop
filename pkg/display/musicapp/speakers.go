package musicapp

import (
	"context"
	"slices"
	"strings"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/musicassistant"
	"github.com/ygelfand/libcountertop/pkg/network/peer"
	"github.com/ygelfand/libcountertop/pkg/network/peer/pictures"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func (a *App) speakers(
	w *gogui.Window,
	list []*musicassistant.Speaker,
	current *musicassistant.Speaker,
) gogui.View {
	vw, _ := w.WindowSize()
	reach := style.Reach()
	grouped := current.Now().Members
	rows := make([]gogui.View, 0, len(list)+1)
	a.mu.Lock()
	problem, filter := a.problem, a.filter
	a.mu.Unlock()
	if problem != "" {
		rows = append(rows, gogui.Label(problem, widgets.Secondary()))
	}
	for _, s := range list {
		if !matches(s.Name, filter) {
			continue
		}
		rows = append(
			rows,
			a.speakerRow(w, s, current, slices.Contains(grouped, s.Entity), float32(vw), reach),
		)
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		Content: []gogui.View{
			a.o.UI.Input(gogui.InputCfg{
				ID:          "music-speaker-search",
				Text:        filter,
				Placeholder: say.T("music.find"),
				Sizing:      gogui.FillFit,
				OnTextChanged: func(text string, e gogui.EventCtx) {
					a.mu.Lock()
					a.filter = text
					a.mu.Unlock()
					e.Window.InvalidateLayout()
				},
			}, nil),
			gogui.Column(gogui.ContainerCfg{
				ID:         "music-speakers",
				Sizing:     gogui.FillFill,
				Padding:    gogui.NewPadding(0, reach*0.4, 0, 0),
				Spacing:    gogui.SpacingMedium,
				Scrollable: true,
				OnGesture:  a.o.UI.HoldStill,
				Content:    rows,
			}),
		},
	})
}

func matches(name, filter string) bool {
	return strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(filter)))
}

func (a *App) speakerRow(
	w *gogui.Window,
	s, current *musicassistant.Speaker,
	grouped bool,
	vw, reach float32,
) gogui.View {
	now := s.Now()
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	room := vw * 0.25
	line := say.T("music.quiet")
	if now.Active() && now.Title != "" {
		line = now.Title
		if now.Artist != "" {
			line += " · " + now.Artist
		}
	}
	entity := s.Entity
	name := a.o.UI.Toolkit.Pressable(gogui.Row, gogui.ContainerCfg{
		ID:      "music-pick-" + entity,
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			a.speakerCover(s, now.Picture, reach*1.4),
			gogui.Column(gogui.ContainerCfg{
				Sizing: gogui.FillFit, Padding: gogui.NoPadding, Clip: true,
				Content: []gogui.View{
					gogui.Label(widgets.Ellipsize(w, s.Name, body, room), body),
					gogui.Label(
						widgets.Ellipsize(w, line, widgets.Secondary(), room),
						widgets.Secondary(),
					),
				},
			}),
		},
	}, func(e gogui.EventCtx) {
		a.choose(entity, playingTab)
		e.Consume()
	})
	content := []gogui.View{name, a.slider("music-volume-"+entity, s, vw*0.22)}
	if s.Entity != current.Entity {
		content = append(
			content,
			a.toggle("music-group-"+entity, "mdi:link-variant", say.T("music.group"), grouped,
				func() { a.group(current, s, grouped) }),
			a.toggle(
				"music-here-"+entity,
				"mdi:arrow-right-circle-outline",
				say.T("music.here"),
				false,
				func() { a.move(current, s) },
			),
		)
	}
	cfg := gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.PaddingSmall,
		Spacing: gogui.SpacingMedium,
		Radius:  gogui.RadiusMedium,
		VAlign:  gogui.VAlignMiddle,
		Content: content,
	}
	a.o.UI.Toolkit.ListRow(&cfg, widgets.Chosen(s.Entity == current.Entity))
	return gogui.Row(cfg)
}

func (a *App) toggle(id, icon, label string, on bool, do func()) gogui.View {
	reach := style.Reach()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family, glyph.Size = icons.Family, reach*0.55
	cfg := gogui.ContainerCfg{
		ID:      id,
		Sizing:  gogui.FitFit,
		Padding: gogui.NewPadding(reach*0.15, reach*0.3, reach*0.15, reach*0.3),
		Spacing: gogui.SpacingSmall,
		VAlign:  gogui.VAlignMiddle,
	}
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	a.o.UI.Toolkit.Tab(&cfg, &st, widgets.Chosen(on))
	glyph.Color = st.Color
	cfg.Content = []gogui.View{
		gogui.Label(icons.Glyph(icons.Named(icon)), glyph),
		gogui.Label(label, st),
	}
	return a.o.UI.Toolkit.Pressable(gogui.Row, cfg, func(e gogui.EventCtx) {
		do()
		e.Consume()
	})
}

func (a *App) group(current, s *musicassistant.Speaker, grouped bool) {
	leader, member := current.Entity, s.Entity
	a.act(func(ctx context.Context) error {
		if grouped {
			return a.o.Music.Unjoin(ctx, member)
		}
		return a.o.Music.Join(ctx, leader, []string{member})
	})
}

func (a *App) move(current, s *musicassistant.Speaker) {
	from, to := current.Entity, s.Entity
	a.act(func(ctx context.Context) error { return a.o.Music.Transfer(ctx, from, to) })
	a.choose(to, playingTab)
}

func (a *App) speakerCover(s *musicassistant.Speaker, picture string, side float32) gogui.View {
	p, ours := a.device(s)
	if picture != "" || !ours {
		return a.cover(picture, side)
	}
	key, svg := pictures.For(p.Board)
	return gogui.Column(gogui.ContainerCfg{
		Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
		HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Svg(gogui.SvgCfg{
			ID:      "music-device-" + key,
			SvgData: svg,
			Width:   side * pictures.Aspect,
			Height:  side,
		})},
	})
}

func (a *App) device(s *musicassistant.Speaker) (peer.Peer, bool) {
	if a.o.Peers == nil {
		return peer.Peer{}, false
	}
	for _, p := range a.o.Peers.Peers() {
		if slices.Contains(s.IDs, p.ID) {
			return p, true
		}
	}
	return peer.Peer{}, false
}
