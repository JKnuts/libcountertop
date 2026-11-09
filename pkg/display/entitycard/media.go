package entitycard

import (
	"fmt"
	"strconv"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

func playTime(seconds float64) string {
	s := int(max(0, seconds))
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (c *Card) mediaBody(w *gogui.Window, e Entity, width float32) []gogui.View {
	reach := style.Reach()
	var out []gogui.View
	title := gogui.CurrentTheme().Cfg.TextStyleDef
	title.Size *= 1.4
	if s := e.Text("media_title"); s != "" {
		out = append(out, gogui.Label(widgets.Ellipsize(w, s, title, width), title))
	}
	if s := e.Text("media_artist"); s != "" {
		out = append(out, gogui.Label(widgets.Ellipsize(w, s, secondary(), width), secondary()))
	}
	if dur, ok := e.Number("media_duration"); ok && dur > 0 {
		pos, _ := e.Number("media_position")
		if at, err := time.Parse(
			time.RFC3339Nano,
			e.Text("media_position_updated_at"),
		); err == nil &&
			e.State == "playing" {
			pos += time.Since(at).Seconds()
			c.tick(w)
		}
		pos = min(max(pos, 0), dur)
		out = append(out, c.progress(width, float32(pos/dur)),
			gogui.Label(playTime(pos)+" / "+playTime(dur), secondary()))
	}
	call := func(action string) func(gogui.EventCtx) {
		if !e.Available {
			return nil
		}
		return func(ev gogui.EventCtx) {
			c.send(e, "media_player."+action, nil, nil)
			ev.Window.InvalidateLayout()
		}
	}
	middle := key{glyph: 0xF040A}
	if e.State == "playing" {
		middle = key{glyph: 0xF03E4}
	}
	out = append(out, gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.3),
		Content: []gogui.View{
			c.panelKey(
				"card-media-prev",
				key{glyph: 0xF04AE},
				reach*1.4,
				call("media_previous_track"),
			),
			c.panelKey("card-media-play", middle, reach*1.8, call("media_play_pause")),
			c.panelKey("card-media-next", key{glyph: 0xF04AD}, reach*1.4, call("media_next_track")),
		},
	}))
	if _, ok := e.Number("volume_level"); ok {
		muted, _ := e.Bool("is_volume_muted")
		glyph := rune(0xF057E)
		if muted {
			glyph = 0xF075F
		}
		var mute func(gogui.EventCtx)
		if e.Available {
			mute = func(ev gogui.EventCtx) {
				c.send(e, "media_player.volume_mute",
					map[string]string{"is_volume_muted": strconv.FormatBool(!muted)}, nil)
				ev.Window.InvalidateLayout()
			}
		}
		out = append(out, gogui.Row(gogui.ContainerCfg{
			Width:   width,
			Sizing:  gogui.FixedFit,
			Padding: gogui.NoPadding,
			Spacing: gogui.SpacingPx(reach * 0.3),
			VAlign:  gogui.VAlignMiddle,
			Content: []gogui.View{
				c.panelKey("card-media-mute", key{glyph: glyph, checked: muted}, reach, mute),
				c.slider(w, "card-level", e, width-reach*1.3),
			},
		}))
	}
	if sources := e.Texts("source_list"); len(sources) > 0 {
		out = append(out, c.choices(w, "card-source", sources, e.Text("source"), width, e.Available,
			func(s string) {
				c.send(e, "media_player.select_source", map[string]string{"source": s}, nil)
			}))
	}
	return out
}

func (c *Card) progress(width, done float32) gogui.View {
	pal := c.o.UI.Palette()
	track := gogui.RGB(pal.Muted.R, pal.Muted.G, pal.Muted.B).WithOpacity(0.25)
	fill := gogui.CurrentTheme().Cfg.ColorAccent
	height := max(4, style.Reach()*0.08)
	return gogui.DrawCanvas(gogui.DrawCanvasCfg{
		ID:      "card-media-progress",
		Width:   width,
		Height:  height,
		Sizing:  gogui.FixedFixed,
		Version: uint64(done * 10000),
		OnDraw: func(dc *gogui.DrawContext) {
			dc.FilledRoundedRect(0, 0, width, height, height/2, track)
			dc.FilledRoundedRect(0, 0, max(height, width*done), height, height/2, fill)
		},
	})
}

func (c *Card) tick(w *gogui.Window) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ticking {
		return
	}
	c.ticking = true
	time.AfterFunc(time.Second, func() {
		c.mu.Lock()
		c.ticking = false
		c.mu.Unlock()
		w.QueueCommand(func(w *gogui.Window) { w.InvalidateLayout() })
	})
}
