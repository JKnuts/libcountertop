package entitycard

import (
	gogui "github.com/go-gui-org/go-gui/gui"
)

func (c *Card) climateBody(w *gogui.Window, e Entity, width float32) []gogui.View {
	enabled := e.Available
	step := e.number("target_temp_step", 0.5)
	lo, hi := e.number("min_temp", 7), e.number("max_temp", 35)
	set := e.Domain() + ".set_temperature"
	send := func(data map[string]string) { c.send(e, set, data, nil) }
	var out []gogui.View
	low, lowOK := e.Number("target_temp_low")
	high, highOK := e.Number("target_temp_high")
	target, targetOK := e.Number("temperature")
	switch {
	case lowOK && highOK:
		out = append(
			out,
			c.setpoint("card-low", e.ID+"/low", low, step, lo, hi, "°", enabled, func(x float64) {
				send(map[string]string{"target_temp_low": num(x), "target_temp_high": num(high)})
			}),
			c.setpoint(
				"card-high",
				e.ID+"/high",
				high,
				step,
				lo,
				hi,
				"°",
				enabled,
				func(x float64) {
					send(map[string]string{"target_temp_low": num(low), "target_temp_high": num(x)})
				},
			),
		)
	case targetOK:
		out = append(
			out,
			c.setpoint("card-target", e.ID+"/target", target, step, lo, hi, "°", enabled,
				func(x float64) { send(map[string]string{"temperature": num(x)}) }),
		)
	}
	var modes []string
	for _, m := range e.Texts("hvac_modes") {
		if m != "off" {
			modes = append(modes, m)
		}
	}
	if len(modes) > 0 {
		out = append(out, c.choices(w, "card-mode", modes, e.State, width, enabled, func(m string) {
			c.send(e, "climate.set_hvac_mode", map[string]string{"hvac_mode": m}, nil)
		}))
	}
	for _, row := range []struct{ list, current, action, field string }{
		{"fan_modes", "fan_mode", "climate.set_fan_mode", "fan_mode"},
		{"swing_modes", "swing_mode", "climate.set_swing_mode", "swing_mode"},
	} {
		if options := e.Texts(row.list); len(options) > 0 {
			out = append(
				out,
				c.choices(w, "card-"+row.field, options, e.Text(row.current), width, enabled,
					func(o string) { c.send(e, row.action, map[string]string{row.field: o}, nil) }),
			)
		}
	}
	return out
}
