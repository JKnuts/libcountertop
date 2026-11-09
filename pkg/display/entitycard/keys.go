package entitycard

import (
	"math"
	"strconv"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	editDelay = 700 * time.Millisecond
	editMost  = 10 * time.Second

	coverOpen  = 1
	coverClose = 2
	coverStop  = 8
	lockOpen   = 1
)

type key struct {
	glyph    rune
	action   string
	data     map[string]string
	checked  bool
	disabled bool
}

type edit struct {
	value float64
	sent  time.Time
	timer *time.Timer
}

func coverKeys(e Entity) []key {
	sideways := false
	switch e.Text("device_class") {
	case "curtain", "awning", "door", "gate":
		sideways = true
	}
	open, closed := e.State == "open", e.State == "closed"
	if p, ok := e.Number("current_position"); ok {
		open, closed = p >= 99.5, p <= 0.5
	}
	up, down := rune(0xF005D), rune(0xF0045)
	if sideways {
		up, down = 0xF084E, 0xF084C
	}
	features := coverOpen | coverClose | coverStop
	if f := e.features(); f > 0 {
		features = f
	}
	var keys []key
	if features&coverOpen != 0 {
		keys = append(keys, key{
			glyph:    up,
			action:   "cover.open_cover",
			checked:  e.State == "opening",
			disabled: open && e.State != "closing",
		})
	}
	if features&coverStop != 0 {
		keys = append(keys, key{glyph: 0xF04DB, action: "cover.stop_cover"})
	}
	if features&coverClose != 0 {
		keys = append(keys, key{
			glyph:    down,
			action:   "cover.close_cover",
			checked:  e.State == "closing",
			disabled: closed && e.State != "opening",
		})
	}
	return keys
}

func keyStyle() gogui.TextStyle {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Family = icons.Family
	return st
}

func (c *Card) panelKey(id string, k key, width float32, do func(gogui.EventCtx)) gogui.View {
	state := style.Rest
	if k.checked {
		state = style.Chosen
	}
	return c.o.UI.Toolkit.Keyed(gogui.ContainerCfg{
		ID:       id,
		Width:    width,
		Height:   style.Reach(),
		Sizing:   gogui.FixedFixed,
		Radius:   gogui.RadiusLarge,
		Color:    widgets.KeyFill(c.o.UI.Palette()),
		Disabled: do == nil,
	}, icons.Glyph(k.glyph), keyStyle(), state, do)
}

func (c *Card) keys(id string, e Entity, keys []key, room float32) gogui.View {
	gap := style.Reach() * 0.15
	width := min(style.Reach()*1.2, (room-float32(len(keys)-1)*gap)/float32(max(1, len(keys))))
	views := make([]gogui.View, 0, len(keys))
	for i, k := range keys {
		do := func(gogui.EventCtx) { c.send(e, k.action, k.data, nil) }
		if k.disabled || !e.Available {
			do = nil
		}
		views = append(views, c.panelKey(id+"-key-"+strconv.Itoa(i), k, width, do))
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(gap),
		Content: views,
	})
}

func (c *Card) wordKey(
	id, text string,
	width float32,
	chosen bool,
	do func(gogui.EventCtx),
) gogui.View {
	state := style.Rest
	if chosen {
		state = style.Chosen
	}
	return c.o.UI.Toolkit.Keyed(gogui.ContainerCfg{
		ID:       id,
		Width:    width,
		Height:   style.Reach(),
		Sizing:   gogui.FixedFixed,
		Radius:   gogui.RadiusLarge,
		Color:    widgets.KeyFill(c.o.UI.Palette()),
		Disabled: do == nil,
	}, text, gogui.CurrentTheme().Cfg.TextStyleDef, state, do)
}

func (c *Card) lockKeys(e Entity, width float32) gogui.View {
	reach := style.Reach()
	key := (width - reach*0.4) / 3
	lock := func(gogui.EventCtx) { c.send(e, "lock.lock", nil, nil) }
	unlock := func(gogui.EventCtx) { c.send(e, "lock.unlock", nil, nil) }
	if !e.Available {
		lock, unlock = nil, nil
	}
	keys := []gogui.View{
		c.wordKey("card-lock", say.T("card.lock.lock"), key, e.State == "locked", lock),
		c.wordKey("card-unlock", say.T("card.lock.unlock"), key, e.State == "unlocked", unlock),
	}
	if e.features()&lockOpen != 0 {
		var open func(gogui.EventCtx)
		if e.Available {
			open = func(gogui.EventCtx) { c.send(e, "lock.open", nil, nil) }
		}
		keys = append(keys, c.wordKey("card-open", say.T("card.lock.open"), key, false, open))
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.2),
		Content: keys,
	})
}

func (c *Card) choices(
	w *gogui.Window,
	id string,
	options []string,
	current string,
	width float32,
	enabled bool,
	do func(string),
) gogui.View {
	reach := style.Reach()
	gap := reach * 0.15
	per := max(1, min(len(options), int((width+gap)/(reach*3+gap))))
	size := (width - float32(per-1)*gap) / float32(per)
	var rows, row []gogui.View
	for i, o := range options {
		var tap func(gogui.EventCtx)
		if enabled {
			tap = func(ev gogui.EventCtx) {
				do(o)
				ev.Window.InvalidateLayout()
			}
		}
		label := widgets.Ellipsize(
			w,
			pretty(o),
			gogui.CurrentTheme().Cfg.TextStyleDef,
			size-reach*0.4,
		)
		row = append(row, c.wordKey(id+"-"+strconv.Itoa(i), label, size, o == current, tap))
		if len(row) == per || i == len(options)-1 {
			rows = append(rows, gogui.Row(gogui.ContainerCfg{
				Sizing:  gogui.FitFit,
				Padding: gogui.NoPadding,
				Spacing: gogui.SpacingPx(gap),
				Content: row,
			}))
			row = nil
		}
	}
	return gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(gap),
		HAlign:  gogui.HAlignCenter,
		Content: rows,
	})
}

func (c *Card) setpoint(
	id, slot string,
	value, step, lo, hi float64,
	suffix string,
	enabled bool,
	commit func(float64),
) gogui.View {
	shown := c.editingAt(slot, value)
	press := func(direction int) func(gogui.EventCtx) {
		if !enabled {
			return nil
		}
		return func(ev gogui.EventCtx) {
			c.stepAt(slot, stepValue(shown, step, lo, hi, direction), commit)
			ev.Window.InvalidateLayout()
		}
	}
	big := gogui.CurrentTheme().Cfg.TextStyleDef
	big.Size *= 2.2
	reach := style.Reach()
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.5),
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			c.panelKey(id+"-down", key{glyph: 0xF0374}, reach*1.2, press(-1)),
			gogui.Label(formatStep(shown, step)+suffix, big),
			c.panelKey(id+"-up", key{glyph: 0xF0415}, reach*1.2, press(1)),
		},
	})
}

func (c *Card) editingAt(slot string, real float64) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.edits[slot]
	switch {
	case !ok:
		return real
	case e.sent.IsZero():
		return e.value
	case real == e.value || time.Since(e.sent) > editMost:
		delete(c.edits, slot)
		return real
	}
	return e.value
}

func (c *Card) stepAt(slot string, value float64, commit func(float64)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.edits[slot]
	if !ok || !e.sent.IsZero() {
		e = &edit{}
		c.edits[slot] = e
	}
	e.value = value
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(editDelay, func() {
		c.mu.Lock()
		e.sent = time.Now()
		sent := e.value
		c.mu.Unlock()
		commit(sent)
	})
}

func stepValue(current, step, lo, hi float64, direction int) float64 {
	if step <= 0 {
		step = 1
	}
	next := lo + math.Round((current+float64(direction)*step-lo)/step)*step
	return min(max(next, lo), hi)
}

func formatStep(value, step float64) string {
	digits := 0
	if step < 1 {
		digits = 1
	}
	return strconv.FormatFloat(value, 'f', digits, 64)
}

func num(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
