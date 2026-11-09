package ringsview

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-gui-org/go-glyph"
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/entitycard"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func (v *View) panel(
	w *gogui.Window,
	root *homecontrol.Ring,
	pal theme.Theme,
	pw, ph float32,
) gogui.View {
	_, focus, _ := v.state()
	v.mu.Lock()
	picked := v.picked
	v.mu.Unlock()

	var views []gogui.View
	group := root.Find(focus)
	if path := root.Path("device:" + picked); len(path) >= 2 && path[len(path)-1].Device != nil {
		bar := gogui.CurrentTheme().ScrollbarStyle
		inner := pw - gogui.PaddingMedium.Left - gogui.PaddingMedium.Right - bar.Size - 2*bar.GapEdge
		views = append(views, v.detail(w, *path[len(path)-1].Device, pal, inner)...)
		views = append(views, widgets.Divider())
		group = path[len(path)-2]
		heading := group.Name
		if len(path) >= 3 {
			heading = path[len(path)-3].Name + " · " + heading
		}
		st := gogui.CurrentTheme().TextStyleDef
		st.Size *= 0.75
		st.Color = widgets.Color(pal.Muted)
		views = append(views, gogui.Text(gogui.TextCfg{
			Text:      strings.ToUpper(heading),
			TextStyle: st,
			Mode:      gogui.TextModeWrap,
		}))
	}
	if group != nil {
		for _, d := range group.Leaves() {
			row := menu.Row{
				Label:  d.Name,
				Value:  stateText(*d),
				Chosen: d.ID == picked,
				Dim:    !d.Available,
			}
			views = append(
				views,
				v.o.UI.Row("rings-list-"+d.ID, page, row, func(int) { v.pick(w, d.ID) }, false),
			)
		}
	}
	body := gogui.Column(gogui.ContainerCfg{
		ID:         "rings-panel-body",
		Sizing:     gogui.FillFill,
		Padding:    gogui.NoPadding,
		Spacing:    gogui.SpacingSmall,
		Clip:       true,
		Scrollable: true,
		ScrollMode: gogui.ScrollVerticalOnly,
		Content:    views,
	})
	content := []gogui.View{v.header(w, root.Path(focus), pal), body}
	return gogui.Column(gogui.ContainerCfg{
		ID:      "rings-panel",
		Width:   pw,
		Height:  ph,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.PaddingMedium,
		Spacing: gogui.SpacingSmall,
		Color:   widgets.Color(pal.Surface),
		Clip:    true,
		Content: content,
	})
}

func (v *View) header(w *gogui.Window, path []*homecontrol.Ring, pal theme.Theme) gogui.View {
	names := make([]string, 0, len(path))
	for _, r := range path {
		names = append(names, r.Name)
	}
	crumb := gogui.CurrentTheme().TextStyleDef
	crumb.Size *= 0.75
	crumb.Color = widgets.Color(pal.Muted)
	title := gogui.CurrentTheme().TextStyleDef
	title.Typeface = glyph.TypefaceBold
	title.Size *= 1.3
	title.Color = widgets.Color(pal.Text)
	wrap := func(s string, st gogui.TextStyle) gogui.View {
		return gogui.Text(gogui.TextCfg{Text: s, TextStyle: st, Mode: gogui.TextModeWrap})
	}
	var text []gogui.View
	if len(names) > 1 {
		text = append(text, wrap(strings.Join(names[:len(names)-1], " › "), crumb))
	}
	if len(path) > 0 {
		at := path[len(path)-1]
		text = append(
			text,
			wrap(at.Name, title),
			wrap(say.F("rings.summary", map[string]any{"On": at.Active, "N": at.Devices}), crumb),
		)
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			gogui.Column(
				gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Content: text},
			),
			v.shut(pal),
		},
	})
}

func (v *View) shut(pal theme.Theme) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Color = widgets.Color(pal.Muted)
	inset := style.Reach() * 0.3
	return gogui.Column(gogui.ContainerCfg{
		Padding: gogui.NewPadding(inset, inset, inset, inset),
		Content: []gogui.View{v.o.UI.Toolkit.Pressable(gogui.Column, gogui.ContainerCfg{
			ID:      "rings-close",
			Width:   style.Reach() * 1.2,
			Height:  style.Reach() * 1.2,
			Sizing:  gogui.FixedFixed,
			HAlign:  gogui.HAlignCenter,
			VAlign:  gogui.VAlignMiddle,
			Padding: gogui.NoPadding,
			Radius:  gogui.RadiusMedium,
			Color:   widgets.Color(mix(pal.Surface, pal.Text, 0.06)),
			Content: []gogui.View{gogui.Label(gogui.IconClose, st)},
		}, func(e gogui.EventCtx) {
			v.close(e.Window)
			e.Consume()
		})},
	})
}

func (v *View) detail(
	w *gogui.Window,
	d homecontrol.Device,
	pal theme.Theme,
	width float32,
) []gogui.View {
	e := entitycard.Entity{ID: d.ID, Name: d.Name, State: d.State, Available: d.Available}
	if facts, known := v.o.Home.Facts(d.ID); known {
		e.State, e.Attributes = facts.State, facts.Attributes
	}
	title := gogui.CurrentTheme().TextStyleDef
	title.Typeface = glyph.TypefaceBold
	title.Color = widgets.Color(pal.Text)
	out := []gogui.View{
		gogui.Text(gogui.TextCfg{Text: d.Name, TextStyle: title, Mode: gogui.TextModeWrap}),
	}
	return append(out, gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: v.card.Body(w, e, width),
	}))
}

func stateText(d homecontrol.Device) string {
	switch {
	case d.Kind == "climate" && d.Current != "" && d.Target != "":
		return degrees(d.Current) + " → " + degrees(d.Target)
	case d.Kind == "lights" && d.Level >= 0:
		return humanize(d.State) + " · " + strconv.Itoa(d.Level) + "%"
	}
	return humanize(d.State)
}

func degrees(v string) string {
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return strconv.FormatFloat(n, 'f', -1, 64) + "°"
	}
	return v
}

func humanize(state string) string {
	state = strings.ReplaceAll(state, "_", " ")
	r, size := utf8.DecodeRuneInString(state)
	if size == 0 {
		return state
	}
	return string(unicode.ToUpper(r)) + state[size:]
}
