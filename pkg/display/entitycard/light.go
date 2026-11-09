package entitycard

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func lightModes(e Entity) []string { return e.Texts("supported_color_modes") }

func lightDims(e Entity) bool {
	m := lightModes(e)
	return len(m) != 1 || m[0] != "onoff"
}

func lightHue(e Entity) bool {
	return slices.ContainsFunc(lightModes(e), func(m string) bool {
		switch m {
		case "hs", "rgb", "rgbw", "rgbww", "xy":
			return true
		}
		return false
	})
}

func lightWarmth(e Entity) bool { return slices.Contains(lightModes(e), "color_temp") }

func (c *Card) lightBody(w *gogui.Window, e Entity, width float32) []gogui.View {
	var out []gogui.View
	now := time.Now()
	if lightDims(e) {
		b := c.held(c.levelDial(e), now)
		if e.State != "on" {
			b = 0
		}
		out = append(out, c.labelled(say.T("card.light.brightness"),
			strconv.Itoa(int(math.Round(b*100)))+" %", width,
			c.slider(w, "card-level", e, width)))
	}
	if lightHue(e) {
		out = append(out, c.colorWheel(e, width)...)
	}
	if lightWarmth(e) {
		lo, hi := e.number("min_color_temp_kelvin", 0), e.number("max_color_temp_kelvin", 0)
		k, known := e.Number("color_temp_kelvin")
		ready := hi > lo
		real := 0.0
		if ready && known {
			real = (k - lo) / (hi - lo)
		}
		d := dial{
			key:  e.ID + "/kelvin",
			real: real,
			live: e.Available,
			commit: func(x float64) {
				kelvin := int(math.Round(lo + x*(hi-lo)))
				c.send(
					e,
					"light.turn_on",
					map[string]string{"color_temp_kelvin": strconv.Itoa(kelvin)},
					nil,
				)
			},
		}
		x := c.held(d, now)
		says := say.T("card.light.waiting")
		if ready {
			says = strconv.Itoa(int(math.Round(lo+x*(hi-lo)))) + " K"
		}
		out = append(out, c.labelled(say.T("card.light.warmth"), says, width,
			c.dialSlider(w, "card-kelvin", d, x, width, !e.Available || !ready)))
	}
	return out
}

type preset struct {
	hue, sat float64
	kelvin   float64
}

var presets = []preset{
	{kelvin: 2700, hue: 35, sat: 40},
	{hue: 0, sat: 100},
	{hue: 25, sat: 100},
	{hue: 50, sat: 100},
	{hue: 120, sat: 100},
	{hue: 185, sat: 100},
	{hue: 225, sat: 100},
	{hue: 275, sat: 100},
	{hue: 320, sat: 50},
}

func (c *Card) colorWheel(e Entity, width float32) []gogui.View {
	hue, sat := 0.0, 0.0
	if hs := e.Numbers("hs_color"); len(hs) == 2 {
		hue, sat = hs[0], hs[1]
	}
	satDial := dial{
		key:    e.ID + "/sat",
		real:   sat / 100,
		live:   e.Available,
		commit: func(float64) {},
	}
	hueDial := dial{
		key:  e.ID + "/hue",
		real: hue / 360,
		live: e.Available,
		commit: func(x float64) {
			s := c.held(satDial, time.Now())
			c.send(e, "light.turn_on", nil, map[string]string{
				"hs_color": fmt.Sprintf("[%d,%d]", int(math.Round(x*360)), int(math.Round(s*100))),
			})
		},
	}
	now := time.Now()
	h, s := c.held(hueDial, now), c.held(satDial, now)
	reach := style.Reach()
	size := min(width*0.7, reach*8)
	wheel := gogui.ColorWheel(gogui.ColorWheelCfg{
		ID:            "card-wheel",
		Value:         gogui.HSLA{H: float32(h * 360), S: float32(s), L: 0.5, A: 1},
		Size:          size,
		MarkerSize:    reach * 0.55,
		FocusDisabled: true,
		OnChange: func(col gogui.HSLA, ev gogui.EventCtx) {
			if !e.Available {
				return
			}
			c.nudge(ev.Window, satDial, float64(col.S))
			c.nudge(ev.Window, hueDial, float64(col.H)/360)
			ev.Window.InvalidateLayout()
		},
	})
	var swatches []gogui.View
	side := min(reach*1.1, (width-reach*0.2*float32(len(presets)-1))/float32(len(presets)))
	warm := lightWarmth(e)
	for i, p := range presets {
		swatches = append(swatches, gogui.ColorSwatch(gogui.ColorSwatchCfg{
			ID:     fmt.Sprintf("card-swatch-%d", i),
			Color:  p.color(),
			Width:  side,
			Height: side,
			Radius: gogui.RadiusLarge,
			OnClick: func(ev gogui.EventCtx) {
				ev.Consume()
				if !e.Available {
					return
				}
				if p.kelvin > 0 && warm {
					lo, hi := e.number(
						"min_color_temp_kelvin",
						0,
					), e.number(
						"max_color_temp_kelvin",
						0,
					)
					k := p.kelvin
					if hi > lo {
						k = min(max(k, lo), hi)
					}
					c.send(
						e,
						"light.turn_on",
						map[string]string{"color_temp_kelvin": strconv.Itoa(int(k))},
						nil,
					)
					return
				}
				c.nudge(ev.Window, satDial, p.sat/100)
				c.nudge(ev.Window, hueDial, p.hue/360)
				c.letGo(ev.Window, hueDial)
				c.letGo(ev.Window, satDial)
			},
		}))
	}
	return []gogui.View{
		c.labelled(say.T("card.light.hue"),
			fmt.Sprintf("%d° · %d %%", int(math.Round(h*360)), int(math.Round(s*100))), width,
			gogui.Row(gogui.ContainerCfg{
				Width:   width,
				Sizing:  gogui.FixedFit,
				Padding: gogui.PadVH(reach*0.2, 0),
				HAlign:  gogui.HAlignCenter,
				Content: []gogui.View{wheel},
			})),
		gogui.Row(gogui.ContainerCfg{
			Width:   width,
			Sizing:  gogui.FixedFit,
			Padding: gogui.NoPadding,
			Spacing: gogui.SpacingPx(reach * 0.2),
			HAlign:  gogui.HAlignCenter,
			Content: swatches,
		}),
	}
}

func (p preset) color() gogui.Color {
	light := float32(0.5)
	if p.sat < 100 {
		light = 0.75
	}
	return gogui.HSLA{H: float32(p.hue), S: float32(p.sat) / 100, L: light, A: 1}.Color()
}
