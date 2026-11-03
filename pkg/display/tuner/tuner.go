package tuner

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/radio"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const page = "radio"

type View struct{}

func (*View) Covers() bool { return true }

func (*View) Presents() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type folder struct{ id, title string }

func (*folder) Covers() bool { return true }

func (*folder) Timeout() time.Duration { return shell.AppTimeout }

type Player interface {
	Now() nowplaying.Now
	Transport() nowplaying.Controller
}

type Options struct {
	UI     *widgets.UI
	Shell  *shell.Shell
	Radio  *radio.Radio
	Player Player
	Gate   func(*gogui.Window) gogui.View
	Redraw func()
}

type Tuner struct {
	o    Options
	view *View

	mu      sync.Mutex
	folders []*folder
	problem string
}

func New(o Options) *Tuner { return &Tuner{o: o, view: &View{}} }

func (t *Tuner) App() apps.App {
	return apps.App{
		ID:   schema.AppRadio,
		Name: func() string { return say.T("radio.app") },
		Icon: "mdi:radio",
		Open: func() { t.o.Shell.Push(t.view) },
	}
}

func (t *Tuner) Screen(v shell.View) *navigation.Screen {
	if v == t.view {
		return &navigation.Screen{Title: say.T("radio.app"), View: v, Build: t.build}
	}
	f, ok := v.(*folder)
	if !ok || !t.owns(f) {
		return nil
	}
	return &navigation.Screen{Title: f.title, View: v, Build: func(w *gogui.Window) gogui.View {
		return t.browse(w, f)
	}}
}

func (t *Tuner) owns(f *folder) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Contains(t.folders, f)
}

func (t *Tuner) build(w *gogui.Window) gogui.View {
	if gate := t.o.Gate(w); gate != nil {
		return gate
	}
	vw, _ := w.WindowSize()
	reach := style.Reach()
	width := min(float32(vw)-2*reach, reach*16)
	saved := t.o.Radio.Settings()
	tuned := current(saved)
	now := t.o.Player.Now()

	views := []gogui.View{}
	if len(saved.Favorites) == 0 {
		views = append(views, t.o.UI.Row("radio-none", page, menu.Row{
			Glyph: "mdi:star-outline", Label: say.T("radio.empty"),
		}, nil, true))
	} else {
		views = append(views, t.dial(saved.Favorites, tuned, width))
	}
	views = append(views, t.station(tuned, now, width), t.keys(saved, tuned, now, width))
	t.mu.Lock()
	problem := t.problem
	t.mu.Unlock()
	if problem != "" {
		views = append(views, gogui.Label(problem, widgets.Secondary()))
	}
	views = append(views,
		t.o.UI.Row("radio-local", page, menu.Row{
			Glyph: "mdi:map-marker-radius", Label: say.T("radio.local"),
		}, func(int) { t.open(radio.Local, say.T("radio.local")) }, true),
		t.o.UI.Row("radio-popular", page, menu.Row{
			Glyph: "mdi:trending-up", Label: say.T("radio.popular"),
		}, func(int) { t.open(radio.Popular, say.T("radio.popular")) }, true),
	)
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingLarge,
		HAlign:  gogui.HAlignCenter,
		Content: views,
	})
}

func (t *Tuner) dial(favorites []schema.Station, tuned schema.Station, width float32) gogui.View {
	reach := style.Reach()
	shown := favorites[:min(len(favorites), max(1, int(width/(reach*2.4))))]
	if i := slices.IndexFunc(
		favorites,
		func(s schema.Station) bool { return s.ID == tuned.ID },
	); i >= len(
		shown,
	) {
		start := min(i-len(shown)/2, len(favorites)-len(shown))
		shown = favorites[start : start+len(shown)]
	}
	cell := width / float32(len(shown))
	at := slices.IndexFunc(shown, func(s schema.Station) bool { return s.ID == tuned.ID })
	th := gogui.CurrentTheme().Cfg
	ink := th.TextStyleDef.Color
	accent := t.o.UI.Palette().Accent
	needle := gogui.RGB(accent.R, accent.G, accent.B)
	height := reach * 0.9
	scale := gogui.DrawCanvas(gogui.DrawCanvasCfg{
		ID:      "radio-dial",
		Width:   width,
		Height:  height,
		Sizing:  gogui.FixedFixed,
		Version: uint64(at+1)<<32 | uint64(len(shown))<<16 | uint64(width),
		OnDraw: func(dc *gogui.DrawContext) {
			base := height * 0.75
			dc.Line(0, base, width, base, ink.WithOpacity(0.4), max(1, reach*0.02))
			ticks := len(shown) * 8
			for i := 0; i <= ticks; i++ {
				x := width * float32(i) / float32(ticks)
				long := i%8 == 4
				top := base - height*0.18
				if long {
					top = base - height*0.4
				}
				dc.Line(x, top, x, base, ink.WithOpacity(0.5), max(1, reach*0.02))
			}
			if at >= 0 {
				x := cell*float32(at) + cell/2
				dc.Line(x, 0, x, height, needle, max(2, reach*0.06))
			}
		},
	})
	labels := make([]gogui.View, 0, len(shown))
	for i, s := range shown {
		text := th.TextStyleDef
		if i == at {
			text.Color = needle
		}
		labels = append(labels, t.o.UI.Toolkit.Pressable(gogui.Column, gogui.ContainerCfg{
			ID:      "radio-preset-" + strconv.Itoa(i),
			Width:   cell,
			Sizing:  gogui.FixedFit,
			Padding: gogui.NoPadding,
			HAlign:  gogui.HAlignCenter,
			Content: []gogui.View{gogui.Label(s.Title, text)},
		}, func(gogui.EventCtx) { t.tune(s) }))
	}
	return gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			scale,
			gogui.Row(gogui.ContainerCfg{
				Width:   width,
				Sizing:  gogui.FixedFit,
				Padding: gogui.NoPadding,
				Content: labels,
			}),
		},
	})
}

func (t *Tuner) station(tuned schema.Station, now nowplaying.Now, width float32) gogui.View {
	reach := style.Reach()
	th := gogui.CurrentTheme()
	glyph := th.TextStyleIconMedium
	glyph.Family = icons.Family
	glyph.Size = reach * 1.6
	name := th.Cfg.TextStyleDef
	name.Size *= 1.6
	title := tuned.Title
	if title == "" {
		title = say.T("radio.idle")
	}
	state := say.T("radio.stopped")
	if now.Playing {
		state = say.T("radio.playing")
		if now.Title != "" && now.Title != tuned.Title {
			state = now.Title
		}
	}
	return gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		HAlign:  gogui.HAlignCenter,
		Content: []gogui.View{
			gogui.Label(icons.Glyph(icons.Named("radio")), glyph),
			gogui.Label(title, name),
			gogui.Label(state, widgets.Secondary()),
		},
	})
}

func (t *Tuner) keys(
	saved schema.Radio,
	tuned schema.Station,
	now nowplaying.Now,
	width float32,
) gogui.View {
	reach := style.Reach()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family = icons.Family
	glyph.Size = reach * 0.6
	height := reach * 1.4
	key := func(id, icon string, state style.State, do func()) gogui.View {
		return t.o.UI.Toolkit.WideKey(id, icons.Glyph(icons.Named(icon)), glyph, height, state,
			func(e gogui.EventCtx) {
				do()
				e.Consume()
			})
	}
	play := key("radio-play", "play", style.Chosen, func() {
		if tuned.ID != "" {
			t.tune(tuned)
		}
	})
	if now.Playing {
		play = key("radio-stop", "stop", style.Chosen, func() { t.o.Player.Transport().Stop() })
	}
	star := "star-outline"
	if saved.Favorite(tuned.ID) {
		star = "star"
	}
	content := []gogui.View{
		key("radio-back", "skip-previous", style.Partial, func() { t.step(saved, tuned, -1) }),
		play,
		key("radio-next", "skip-next", style.Partial, func() { t.step(saved, tuned, 1) }),
	}
	if tuned.ID != "" {
		content = append(content, key("radio-star", star, style.Partial, func() {
			if err := t.o.Radio.Favorite(tuned); err != nil {
				t.fail(err)
			}
			t.redraw()
		}))
	}
	return gogui.Row(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingPx(reach * 0.3),
		Content: content,
	})
}

func (t *Tuner) browse(w *gogui.Window, f *folder) gogui.View {
	if gate := t.o.Gate(w); gate != nil {
		return gate
	}
	ui := t.o.UI
	note := func(id, label string) gogui.View {
		return ui.Row(id, page, menu.Row{Label: label}, nil, false)
	}
	p, err, loaded := t.o.Radio.Browse(f.id)
	var views []gogui.View
	switch {
	case !loaded:
		views = append(views, note("loading", say.T("home.loading")))
	case err != nil:
		views = append(
			views,
			ui.Row(
				"failed",
				page,
				menu.Row{Label: say.T("radio.unavailable"), Hint: err.Error()},
				nil,
				false,
			),
			ui.Row("retry", page, menu.Row{Label: say.T("home.retry")},
				func(int) { t.o.Radio.Refresh(f.id) }, false),
		)
	case len(p.Items) == 0:
		views = append(views, note("empty", say.T("radio.nothing")))
	}
	saved := t.o.Radio.Settings()
	for i, it := range p.Items {
		row := menu.Row{Glyph: "mdi:folder-outline", Label: it.Title}
		tap := func(int) { t.open(it.ID, it.Title) }
		if it.Playable {
			row.Glyph = "mdi:radio"
			if saved.Favorite(it.ID) {
				row.Glyph = "mdi:star"
			}
			s := schema.Station{ID: it.ID, Title: it.Title, Thumb: it.Thumb}
			tap = func(int) {
				t.tune(s)
				t.close()
			}
		}
		views = append(views, ui.Row("item-"+strconv.Itoa(i), page, row, tap, true))
	}
	if p.Total > len(p.Items) {
		views = append(
			views,
			note("more", say.F("radio.more", map[string]any{"N": p.Total - len(p.Items)})),
		)
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		Content: views,
	})
}

func (t *Tuner) open(id, title string) {
	f := &folder{id: id, title: title}
	t.mu.Lock()
	t.folders = append(t.folders, f)
	t.mu.Unlock()
	t.o.Shell.Push(f)
}

func (t *Tuner) close() {
	t.mu.Lock()
	folders := t.folders
	t.folders = nil
	t.mu.Unlock()
	for _, f := range slices.Backward(folders) {
		t.o.Shell.Remove(f)
	}
}

func (t *Tuner) tune(s schema.Station) {
	t.mu.Lock()
	t.problem = ""
	t.mu.Unlock()
	go func() {
		if err := t.o.Radio.Play(s); err != nil {
			t.fail(err)
		}
		t.redraw()
	}()
	t.redraw()
}

func (t *Tuner) step(saved schema.Radio, tuned schema.Station, by int) {
	n := len(saved.Favorites)
	if n == 0 {
		return
	}
	i := slices.IndexFunc(saved.Favorites, func(s schema.Station) bool { return s.ID == tuned.ID })
	if i < 0 && by < 0 {
		i = 0
	}
	t.tune(saved.Favorites[((i+by)%n+n)%n])
}

func (t *Tuner) fail(err error) {
	t.mu.Lock()
	t.problem = fmt.Sprint(err)
	t.mu.Unlock()
}

func (t *Tuner) redraw() {
	if t.o.Redraw != nil {
		t.o.Redraw()
	}
}

func current(r schema.Radio) schema.Station {
	for _, s := range r.Favorites {
		if s.ID == r.Last {
			return s
		}
	}
	if r.Last != "" {
		return schema.Station{ID: r.Last, Title: r.LastTitle}
	}
	return schema.Station{}
}
