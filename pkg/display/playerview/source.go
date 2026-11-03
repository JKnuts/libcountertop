package playerview

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
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
	if p.options.Player.From() == nowplaying.FromBluetooth {
		if name := p.options.Player.Named(); name != "" {
			content = append(content,
				gogui.Label(widgets.Ellipsize(w, name, l.dim, max(room-side, 0)), l.dim))
		}
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingMedium,
		Content: content,
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
