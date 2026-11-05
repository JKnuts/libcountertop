package playerview

import (
	"fmt"
	"strings"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/interaction"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	videoplayer "github.com/ygelfand/libcountertop/pkg/display/videopage"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

const (
	seekSettle = 250 * time.Millisecond
	seekLand   = 3 * time.Second
)

type scrub struct {
	mu    sync.Mutex
	sk    nowplaying.Seeker
	to    time.Duration
	held  bool
	until time.Time
	timer *time.Timer
}

func (s *scrub) move(sk nowplaying.Seeker, to time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sk, s.to, s.held = sk, to, true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(seekSettle, s.land)
}

func (s *scrub) land() {
	s.mu.Lock()
	sk, to := s.sk, s.to
	s.held, s.until = false, time.Now().Add(seekLand)
	s.mu.Unlock()
	if sk != nil && sk.CanSeek() {
		sk.Seek(to)
	}
}

func (s *scrub) shown(elapsed time.Duration) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held || time.Now().Before(s.until) {
		return s.to
	}
	return elapsed
}

func Still(img *ui.Image) string {
	w, h := img.Size()
	key := fmt.Sprintf("art/%p/%dx%d", img, w, h)
	if gogui.HasImage(key) {
		return "mem:" + key
	}
	return gogui.UseImage(key, w, h, widgets.NRGBA(img))
}

func Squared(img *ui.Image) string {
	w, h := img.Size()
	side := min(w, h)
	x0, y0 := (w-side)/2, (h-side)/2
	key := fmt.Sprintf("cover/%p/%dx%d", img, w, h)
	if gogui.HasImage(key) {
		return "mem:" + key
	}
	pix := make([]byte, 0, side*side*4)
	for y := range side {
		for x := range side {
			c := img.At(x0+x, y0+y)
			pix = append(pix, c.R, c.G, c.B, 255)
		}
	}
	return gogui.UseImage(key, side, side, pix)
}

func ClockText(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func clockWidth(w *gogui.Window, length time.Duration, st gogui.TextStyle) float32 {
	shape := ClockText(length)
	var widest float32
	for d := '0'; d <= '9'; d++ {
		text := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return d
			}
			return r
		}, shape)
		widest = max(widest, w.TextWidth(text, st))
	}
	return widest
}

func clockLabel(
	text string,
	width float32,
	st gogui.TextStyle,
	align gogui.HorizontalAlign,
) gogui.View {
	return gogui.Row(
		gogui.ContainerCfg{
			Width:   width,
			Sizing:  gogui.FixedFit,
			Padding: gogui.NoPadding,
			HAlign:  align,
			Content: []gogui.View{gogui.Label(text, st)},
		},
	)
}

func (p *Renderer) progress(
	w *gogui.Window,
	now nowplaying.Now,
	sk nowplaying.Seeker,
	st gogui.TextStyle,
	marks []videoplayer.Mark,
) gogui.View {
	at := p.seeking.shown(now.Elapsed)
	canSeek := sk != nil && sk.CanSeek()
	lo, hi := time.Duration(0), now.Length
	painted := spans(marks, now.Length)
	if now.Edge > 0 {
		lo, hi = now.Earliest, now.Edge
		painted = append(
			painted,
			outOfReach(now.Length, 0, lo),
			outOfReach(now.Length, hi, now.Length),
		)
	}
	cfg := gogui.SliderCfg{
		ID:       "seek",
		Height:   reach(),
		Sizing:   gogui.FillFit,
		Value:    float32(at.Seconds()),
		Max:      float32(now.Length.Seconds()),
		Disabled: !canSeek,
		OnChange: func(v float32, e gogui.EventCtx) {
			p.seeking.move(sk, min(max(time.Duration(v*float32(time.Second)), lo), hi))
			e.Window.InvalidateLayout()
		},
	}
	bar := p.options.UI.Grip(p.kit.SeekBar(cfg, painted))
	content := []gogui.View{bar}
	if now.LiveWithin == 0 || now.Edge > 0 {
		wide := clockWidth(w, now.Length, st)
		content = []gogui.View{
			clockLabel(ClockText(at), wide, st, gogui.HAlignRight),
			bar,
			clockLabel(ClockText(now.Length), wide, st, gogui.HAlignLeft),
		}
	}
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingMedium,
		Content: content,
	})
}

type control struct {
	glyph   string
	do      func(nowplaying.Controller)
	need    nowplaying.Controls
	primary bool
}

func (p *Renderer) transport(now nowplaying.Now, l look) gogui.View {
	middle := control{glyph: gogui.IconPlay, do: nowplaying.Controller.Play, primary: true}
	if now.Playing && !now.Paused {
		middle = control{
			glyph:   gogui.IconPause,
			do:      nowplaying.Controller.Pause,
			need:    nowplaying.CanPause,
			primary: true,
		}
	}
	var shown []gogui.View
	for i, b := range []control{
		{glyph: gogui.IconFastBackward, do: nowplaying.Controller.Previous, need: nowplaying.CanPrevious},
		middle,
		{glyph: gogui.IconFastForward, do: nowplaying.Controller.Next, need: nowplaying.CanNext},
	} {
		if b.need != 0 && !now.Can.Has(b.need) {
			continue
		}
		do := b.do
		st := iconStyle(l.body.Color)
		st.Size = l.unit * keyUnits
		side, fill := l.unit*(keyUnits+keyRoom), gogui.ColorTransparent
		if b.primary {
			st.Color = gogui.CurrentTheme().Cfg.ColorBackground
			if l.dark {
				st.Color = gogui.RGB(0x10, 0x10, 0x12)
			}
			st.Size = l.unit * playIcon
			side, fill = l.unit*playUnits, l.body.Color
		}
		shown = append(shown, p.round(fmt.Sprintf("transport-%d", i), b.glyph, st, side, fill,
			func() { do(p.options.Player.Transport()) }))
	}
	left, right := p.extras(now, l)
	center := gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingLarge,
		Content: shown,
	})
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{left, stretch(), center, stretch(), right},
	})
}

func (p *Renderer) extras(now nowplaying.Now, l look) (gogui.View, gogui.View) {
	st := iconStyle(l.dim.Color)
	st.Size = l.unit * extraUnits
	side := l.unit * (extraUnits + keyRoom)
	mdi := st
	mdi.Family = icons.Family
	vol, words, stop := slot(side), slot(side), slot(side)
	if loud := p.options.Loud; loud != nil {
		vol = p.round("player-volume", icons.Glyph(icons.Named("mdi:volume-high")), mdi, side,
			gogui.ColorTransparent, loud)
	}
	if !p.sheet(now).Empty() {
		lit := mdi
		if p.pane == lyricsPane {
			lit.Color = l.body.Color
		}
		words = p.round("player-lyrics", icons.Glyph(icons.Named("mdi:microphone-variant")), lit,
			side, gogui.ColorTransparent, p.flip)
	}
	if now.Can.Has(nowplaying.CanStop) {
		stop = p.round("player-stop", gogui.IconStop, st, side, gogui.ColorTransparent,
			func() { p.options.Player.Transport().Stop() })
	}
	toggle := func(id, name string, on bool, do func()) gogui.View {
		lit := mdi
		if on {
			lit.Color = l.body.Color
		}
		return p.round(id, icons.Glyph(icons.Named(name)), lit, side, gogui.ColorTransparent, do)
	}
	shuffle, repeat, like := slot(side), slot(side), slot(side)
	t := p.options.Player.Transport()
	if s, ok := t.(nowplaying.Shuffler); ok && now.Can.Has(nowplaying.CanShuffle) {
		name := "mdi:shuffle-disabled"
		if now.Shuffle {
			name = "mdi:shuffle-variant"
		}
		shuffle = toggle("player-shuffle", name, now.Shuffle,
			func() { s.SetShuffle(!now.Shuffle) })
	}
	if r, ok := t.(nowplaying.Repeater); ok && now.Can.Has(nowplaying.CanRepeat) {
		name := map[nowplaying.Repeat]string{
			nowplaying.RepeatAll: "mdi:repeat",
			nowplaying.RepeatOne: "mdi:repeat-once",
		}[now.Repeat]
		if name == "" {
			name = "mdi:repeat-off"
		}
		repeat = toggle("player-repeat", name, now.Repeat == nowplaying.RepeatAll ||
			now.Repeat == nowplaying.RepeatOne, func() { r.SetRepeat(now.Repeat.Next()) })
	}
	if k, ok := t.(nowplaying.Liker); ok && now.Can.Has(nowplaying.CanLike) {
		name := "mdi:heart-outline"
		if now.Liked {
			name = "mdi:heart"
		}
		like = toggle("player-like", name, now.Liked, func() { k.SetLiked(!now.Liked) })
	}
	return group(vol, words, shuffle), group(repeat, like, stop)
}

func group(views ...gogui.View) gogui.View {
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Content: views,
	})
}

func slot(side float32) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
	})
}

func stretch() gogui.View {
	return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding})
}

func (p *Renderer) round(
	id, glyph string,
	st gogui.TextStyle,
	side float32,
	fill gogui.Color,
	do func(),
) gogui.View {
	ink := st.Color
	if fill.A > 0 {
		ink = interaction.Contrast(fill)
	}
	return p.options.UI.Presses.Inked(gogui.Column, gogui.ContainerCfg{
		ID:      id,
		Width:   side,
		Height:  side,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Radius:  gogui.RadiusPx(side / 2),
		Color:   fill,
		Content: []gogui.View{gogui.Label(glyph, st)},
	}, ink, func(gogui.EventCtx) { do() })
}

const PriorityMini = 5
