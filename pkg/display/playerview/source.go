package playerview

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	badgeUnits = 5
	badgeRound = 1.2
)

func (p *Renderer) badge(w *gogui.Window, now nowplaying.Now, l look, room float32) gogui.View {
	side := l.unit * badgeUnits
	var mark gogui.View
	switch logo := ui.Picture(now.Logo, now.LogoID); {
	case now.Brand == nowplaying.AirPlay:
		mark = svgMark("source-airplay", icons.AirPlay, side, l.body.Color)
	case logo != nil:
		mark = rounded(gogui.Image(gogui.ImageCfg{Src: Squared(logo), Width: side, Height: side}),
			side, l.unit*badgeRound)
	default:
		mark = p.kindMark(l, side)
	}
	if mark == nil {
		return nil
	}
	content := []gogui.View{mark}
	cfg := gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingMedium,
	}
	if p.options.Player.From() != nowplaying.FromBluetooth {
		cfg.Content = content
		return gogui.Row(cfg)
	}

	name := p.options.Player.Named()
	picker := p.options.BluetoothSources
	nameRoom := room - side - gogui.CurrentTheme().Cfg.SpacingMedium
	if picker != nil {
		if name == "" {
			name = say.T("features.speaker")
		}
		nameRoom -= l.dim.Size + gogui.CurrentTheme().Cfg.SpacingMedium
	}
	if name != "" {
		content = append(
			content,
			gogui.Label(widgets.Ellipsize(w, name, l.dim, max(nameRoom, 0)), l.dim),
		)
	}
	if picker == nil {
		cfg.Content = content
		return gogui.Row(cfg)
	}

	cfg.ID = "bluetooth-source-picker"
	cfg.Radius = gogui.RadiusMedium
	cfg.Content = append(content, gogui.Label(gogui.IconArrowDown, iconStyle(l.dim.Color)))
	return p.options.UI.Presses.Pressable(gogui.Row, cfg, func(e gogui.EventCtx) {
		if page := picker(); page != nil {
			p.options.Shell.Push(page)
		}
		e.Consume()
	})
}

func (p *Renderer) kindMark(l look, side float32) gogui.View {
	switch p.options.Player.From() {
	case nowplaying.FromQueue:
		return svgMark("source-home", icons.HomeAssistant, side, gogui.Color{})
	case nowplaying.FromGroup:
		return svgMark("source-group", icons.MusicAssistant, side, gogui.Color{})
	case nowplaying.FromBluetooth:
		return glyphMark("mdi:bluetooth", side, l.body.Color)
	case nowplaying.FromCast:
		return glyphMark("mdi:cast", side, l.body.Color)
	}
	return nil
}

func svgMark(id, data string, side float32, tint gogui.Color) gogui.View {
	return gogui.Svg(gogui.SvgCfg{ID: id, SvgData: data, Width: side, Height: side, Color: tint})
}

func glyphMark(name string, side float32, color gogui.Color) gogui.View {
	st := iconStyle(color)
	st.Family = icons.Family
	st.Size = side
	return gogui.Label(icons.Glyph(icons.Named(name)), st)
}

func rounded(v gogui.View, side, radius float32) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		Width:   side,
		Height:  side,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Radius:  gogui.RadiusPx(radius),
		Clip:    true,
		Content: []gogui.View{v},
	})
}
