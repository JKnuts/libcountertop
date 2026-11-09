package entitycard

import (
	"strings"
	"sync"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

type Options struct {
	UI      *widgets.UI
	Perform func(action string, data, templates map[string]string)
}

type Card struct {
	o Options

	mu      sync.Mutex
	grips   map[string]*grip
	edits   map[string]*edit
	ticking bool
}

func New(o Options) *Card {
	return &Card{o: o, grips: map[string]*grip{}, edits: map[string]*edit{}}
}

func (c *Card) Body(w *gogui.Window, e Entity, width float32) []gogui.View {
	var out []gogui.View
	switch e.Domain() {
	case "light":
		out = append(out, c.power(e, e.State == "on"))
		out = append(out, c.lightBody(w, e, width)...)
	case "switch", "input_boolean":
		out = append(out, c.power(e, e.State == "on"))
	case "fan":
		out = append(out, c.power(e, e.State == "on"))
		if _, ok := e.Number("percentage"); ok {
			out = append(out, c.slider(w, "card-level", e, width))
		}
	case "cover":
		if _, ok := e.Number("current_position"); ok {
			out = append(out, c.slider(w, "card-level", e, width))
		}
		out = append(out, c.keys("card-cover", e, coverKeys(e), width))
	case "lock":
		out = append(out, c.lockKeys(e, width))
	case "climate", "water_heater":
		out = append(out, c.power(e, e.State != "off"))
		out = append(out, c.climateBody(w, e, width)...)
	case "media_player":
		out = append(out, c.power(e, e.State != "off" && e.State != "standby"))
		out = append(out, c.mediaBody(w, e, width)...)
	}
	return out
}

func (c *Card) send(e Entity, action string, data, templates map[string]string) {
	if data == nil {
		data = map[string]string{}
	}
	data["entity_id"] = e.ID
	c.o.Perform(action, data, templates)
}

func (c *Card) power(e Entity, on bool) gogui.View {
	action := e.Domain() + ".turn_on"
	if on {
		action = e.Domain() + ".turn_off"
	}
	toggle := c.o.UI.Presses.Pressable(gogui.Row, gogui.ContainerCfg{
		ID:       "card-power",
		Sizing:   gogui.FitFit,
		Padding:  gogui.NoPadding,
		Disabled: !e.Available,
		Content:  []gogui.View{c.o.UI.Kit().Toggle("card-power-switch", on)},
	}, func(gogui.EventCtx) { c.send(e, action, nil, nil) })
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			gogui.Label(pretty(e.State), gogui.CurrentTheme().Cfg.TextStyleDef),
			gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding}),
			toggle,
		},
	})
}

func (c *Card) labelled(name, value string, width float32, control gogui.View) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(style.Reach() * 0.1),
		Content: []gogui.View{
			gogui.Row(gogui.ContainerCfg{
				Width:   width,
				Sizing:  gogui.FixedFit,
				Padding: gogui.NoPadding,
				Content: []gogui.View{
					gogui.Label(name, gogui.CurrentTheme().Cfg.TextStyleDef),
					gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding}),
					gogui.Label(value, secondary()),
				},
			}),
			control,
		},
	})
}

func secondary() gogui.TextStyle {
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	st.Color = gogui.CurrentTheme().Cfg.ColorTextSecondary
	return st
}

func pretty(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
