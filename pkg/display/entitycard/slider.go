package entitycard

import (
	"math"
	"strconv"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
)

const (
	holdMost   = 8 * time.Second
	holdNear   = 0.03
	tapRelease = 500 * time.Millisecond
)

type grip struct {
	value  float64
	sent   time.Time
	commit *time.Timer
}

type dial struct {
	key    string
	real   float64
	live   bool
	commit func(float64)
}

func level(e Entity) (float64, bool) {
	switch e.Domain() {
	case "light":
		b, ok := e.Number("brightness")
		return b / 255, ok || e.State == "off"
	case "fan":
		p, ok := e.Number("percentage")
		return p / 100, ok || e.State == "off"
	case "cover":
		p, ok := e.Number("current_position")
		if !ok {
			return 1, false
		}
		return (100 - p) / 100, true
	case "media_player":
		v, ok := e.Number("volume_level")
		return v, ok
	}
	return 0, false
}

func levelAction(e Entity, value float64) (string, map[string]string) {
	switch e.Domain() {
	case "light":
		return "light.turn_on", map[string]string{
			"brightness": strconv.Itoa(max(3, int(math.Round(value*255)))),
		}
	case "fan":
		return "fan.set_percentage", map[string]string{
			"percentage": strconv.Itoa(int(math.Round(value * 100))),
		}
	case "cover":
		return "cover.set_cover_position", map[string]string{
			"position": strconv.Itoa(100 - int(math.Round(value*100))),
		}
	case "media_player":
		return "media_player.volume_set", map[string]string{
			"volume_level": strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64),
		}
	}
	return "", nil
}

func (c *Card) levelDial(e Entity) dial {
	real, _ := level(e)
	return dial{
		key:  e.ID,
		real: real,
		live: e.Available,
		commit: func(x float64) {
			if action, data := levelAction(e, x); action != "" {
				c.send(e, action, data, nil)
			}
		},
	}
}

func (c *Card) held(d dial, now time.Time) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, ok := c.grips[d.key]
	switch {
	case !ok:
		return d.real
	case g.sent.IsZero():
		return g.value
	case now.Sub(g.sent) > holdMost || math.Abs(d.real-g.value) <= holdNear || !d.live:
		delete(c.grips, d.key)
		return d.real
	}
	return g.value
}

func (c *Card) slider(w *gogui.Window, id string, e Entity, width float32) gogui.View {
	d := c.levelDial(e)
	value := c.held(d, time.Now())
	if e.Domain() == "light" && e.State != "on" {
		value = 0
	}
	return c.dialSlider(w, id, d, value, width, !e.Available)
}

func (c *Card) dialSlider(
	w *gogui.Window,
	id string,
	d dial,
	value float64,
	width float32,
	disabled bool,
) gogui.View {
	thumb := gogui.CurrentTheme().Cfg.SizeSliderThumb
	cfg := gogui.SliderCfg{
		ID:     id + "-slider",
		Width:  max(1, width-thumb),
		Height: style.Reach(),
		Sizing: gogui.FixedFit,
		Value:  float32(value * 1000),
		Max:    1000,
		OnChange: func(x float32, ev gogui.EventCtx) {
			c.nudge(ev.Window, d, float64(x)/1000)
			ev.Window.InvalidateLayout()
		},
		Disabled: disabled,
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.PadVH(0, thumb/2),
		Spacing: gogui.NoSpacing,
		Content: []gogui.View{c.o.UI.Toolkit.Slider(cfg)},
		OnGesture: func(ev gogui.EventCtx) {
			if ev.Event == nil || ev.Event.GestureType != gogui.GesturePan {
				return
			}
			ev.Consume()
			switch ev.Event.GesturePhase {
			case gogui.GesturePhaseEnded, gogui.GesturePhaseCancelled:
				c.letGo(w, d)
			}
		},
	})
}

func (c *Card) nudge(w *gogui.Window, d dial, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	g, ok := c.grips[d.key]
	if !ok || !g.sent.IsZero() {
		g = &grip{}
		c.grips[d.key] = g
	}
	g.value = min(max(value, 0), 1)
	if g.commit != nil {
		g.commit.Stop()
	}
	g.commit = time.AfterFunc(tapRelease, func() {
		w.QueueCommand(func(w *gogui.Window) { c.letGo(w, d) })
	})
}

func (c *Card) letGo(w *gogui.Window, d dial) {
	c.mu.Lock()
	g, ok := c.grips[d.key]
	if !ok || !g.sent.IsZero() {
		c.mu.Unlock()
		return
	}
	if g.commit != nil {
		g.commit.Stop()
	}
	g.sent = time.Now()
	value := g.value
	c.mu.Unlock()
	d.commit(value)
	w.InvalidateLayout()
}
