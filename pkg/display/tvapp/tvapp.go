package tvapp

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/covers"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	askTime    = 15 * time.Second
	artLimit   = 4 << 20
	volumeStep = 0.05
	logos      = 512
)

type Player interface {
	Now() nowplaying.Now
	Transport() nowplaying.Controller
}

type Options struct {
	UI      *widgets.UI
	Shell   *shell.Shell
	Guide   *yttv.Guide
	Player  Player
	Loud    func()
	Client  *http.Client
	Gate    func(*gogui.Window) gogui.View
	Clock24 func() bool
	Redraw  func()
}

type View struct{}

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Card struct{}

func (*Card) Covers() bool { return false }

func (*Card) Timeout() time.Duration { return shell.AppTimeout }

type App struct {
	o     Options
	view  *View
	card  *Card
	thumb *covers.Covers
	logo  *covers.Covers

	mu      sync.Mutex
	chosen  string
	filter  string
	problem string
	opened  string
}

func New(o Options) *App {
	a := &App{o: o, view: &View{}, card: &Card{}}
	a.thumb = covers.New(covers.Options{
		Prefix: "tvapp", Shape: covers.Wide, Get: a.download, Redraw: a.redraw,
	})
	a.logo = covers.New(covers.Options{
		Prefix: "tvapp-logo", Get: a.download, Redraw: a.redraw, Keep: logos,
	})
	return a
}

func (a *App) App() apps.App {
	return apps.App{
		ID:   schema.AppTV,
		Name: func() string { return say.T("tv.app") },
		Icon: "mdi:television-classic",
		Open: func() {
			a.o.Guide.Refresh()
			a.o.Shell.Push(a.view)
		},
	}
}

func (a *App) Screen(v shell.View) *navigation.Screen {
	switch v {
	case a.card:
		return &navigation.Screen{View: v, Build: a.channel}
	case a.view:
	default:
		return nil
	}
	return &navigation.Screen{
		Title:    say.T("tv.app"),
		View:     v,
		Build:    a.build,
		Fixed:    true,
		Settings: func() { a.o.Shell.OpenSettings(a.Page()) },
	}
}

func (a *App) build(w *gogui.Window) gogui.View {
	if a.o.Gate != nil {
		if gate := a.o.Gate(w); gate != nil {
			return gate
		}
	}
	channels, tvs, err, loaded := a.o.Guide.Load()
	switch {
	case !loaded:
		return note(say.T("home.loading"))
	case err != nil && len(tvs) == 0:
		return column(
			note(say.T("tv.failed")),
			gogui.Label(say.T("tv.update"), widgets.Secondary()),
		)
	case len(tvs) == 0:
		return note(say.T("tv.none"))
	}
	width := widgets.PageWidth(w)
	tv := a.current(tvs)
	a.mu.Lock()
	filter, problem := a.filter, a.problem
	a.mu.Unlock()
	views := []gogui.View{a.picker(tvs, tv, width), a.now(w, tv, width)}
	if problem != "" {
		views = append(views, gogui.Label(problem, widgets.Secondary()))
	}
	views = append(views,
		a.o.UI.Input(gogui.InputCfg{
			ID:          "tv-find",
			Text:        filter,
			Placeholder: say.T("tv.find"),
			Sizing:      gogui.FillFit,
			OnTextChanged: func(text string, e gogui.EventCtx) {
				a.mu.Lock()
				a.filter = text
				a.mu.Unlock()
				e.Window.InvalidateLayout()
			},
		}, nil),
		a.guide(w, tv, order(channels, filter, a.o.Guide.Favorites()), width),
	)
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
		Content: views,
	})
}

func (a *App) current(tvs []yttv.TV) yttv.TV {
	a.mu.Lock()
	chosen := a.chosen
	a.mu.Unlock()
	for _, want := range []string{chosen, a.o.Guide.Default()} {
		if i := slices.IndexFunc(
			tvs,
			func(t yttv.TV) bool { return t.Entity == want },
		); want != "" &&
			i >= 0 {
			return tvs[i]
		}
	}
	if i := slices.IndexFunc(tvs, a.o.Guide.Mine); i >= 0 {
		return tvs[i]
	}
	return tvs[0]
}

func (a *App) label(t yttv.TV) string {
	if a.o.Guide.Mine(t) {
		return say.T("tv.here")
	}
	return t.Name
}

func (a *App) sorted(tvs []yttv.TV) []yttv.TV {
	out := slices.Clone(tvs)
	slices.SortStableFunc(out, func(x, y yttv.TV) int {
		mx, my := a.o.Guide.Mine(x), a.o.Guide.Mine(y)
		if mx != my {
			if mx {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	return out
}

func (a *App) picker(tvs []yttv.TV, chosen yttv.TV, width float32) gogui.View {
	sorted := a.sorted(tvs)
	tabs := make([]widgets.Tab, 0, len(sorted))
	for _, t := range sorted {
		tabs = append(tabs, widgets.Tab{Key: t.Entity, Label: a.label(t)})
	}
	return a.o.UI.TabStrip(widgets.TabStripCfg{
		ID:     "tv-picker",
		Tabs:   tabs,
		Chosen: chosen.Entity,
		Width:  width,
		Pick: func(entity string) {
			a.mu.Lock()
			a.chosen, a.problem = entity, ""
			a.mu.Unlock()
		},
	})
}

func (a *App) now(w *gogui.Window, tv yttv.TV, width float32) gogui.View {
	reach := style.Reach()
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	var (
		title, under, picture string
		playing, active       bool
		controls              []gogui.View
	)
	if a.o.Guide.Mine(tv) && a.o.Player != nil {
		n := a.o.Player.Now()
		title, under, active, playing = n.Title, n.Artist, n.Title != "", n.Playing && !n.Paused
		t := a.o.Player.Transport()
		controls = append(controls,
			a.button("tv-play", playGlyph(playing), func() {
				if playing {
					t.Pause()
				} else {
					t.Play()
				}
			}),
			a.button("tv-stop", gogui.IconStop, t.Stop),
		)
		if a.o.Loud != nil {
			controls = append(controls, a.mdiButton("tv-volume", "mdi:volume-high", a.o.Loud))
		}
	} else {
		st := a.o.Guide.Status(tv.Entity)
		title, under, picture = st.Title, st.Subtitle, st.Picture
		if st.Channel != "" {
			under = strings.TrimPrefix(st.Channel+" · "+under, " · ")
		}
		active, playing = st.Active(), st.Playing()
		entity := tv.Entity
		controls = append(controls,
			a.button("tv-play", playGlyph(playing), func() {
				a.act(func(ctx context.Context) error {
					if playing {
						return a.o.Guide.Pause(ctx, entity)
					}
					return a.o.Guide.Play(ctx, entity)
				})
			}),
			a.button("tv-stop", gogui.IconStop, func() {
				a.act(func(ctx context.Context) error { return a.o.Guide.Stop(ctx, entity) })
			}),
			a.mdiButton("tv-quieter", "mdi:volume-minus", func() {
				a.act(func(ctx context.Context) error {
					return a.o.Guide.SetVolume(ctx, entity, st.Volume-volumeStep)
				})
			}),
			a.mdiButton("tv-louder", "mdi:volume-plus", func() {
				a.act(func(ctx context.Context) error {
					return a.o.Guide.SetVolume(ctx, entity, st.Volume+volumeStep)
				})
			}),
		)
	}
	if !active {
		return gogui.Label(say.T("tv.quiet"), widgets.Secondary())
	}
	room := width - reach*6
	text := gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Clip: true,
		Content: []gogui.View{
			gogui.Label(widgets.Ellipsize(w, title, body, room), body),
			gogui.Label(
				widgets.Ellipsize(w, under, widgets.Secondary(), room),
				widgets.Secondary(),
			),
		},
	})
	row := []gogui.View{}
	if picture != "" {
		row = append(row, a.picture(a.thumb, picture, reach*2.4, reach*2.4*9/16, "mdi:television"))
	}
	row = append(row, text)
	row = append(row, controls...)
	return gogui.Row(gogui.ContainerCfg{
		ID:      "tv-now",
		Sizing:  gogui.FillFit,
		Padding: gogui.PaddingMedium,
		Spacing: gogui.SpacingMedium,
		VAlign:  gogui.VAlignMiddle,
		Radius:  gogui.RadiusLarge,
		Color:   gogui.CurrentTheme().Cfg.ColorPanel,
		Content: row,
	})
}

func playGlyph(playing bool) string {
	if playing {
		return gogui.IconPause
	}
	return gogui.IconPlay
}

func (a *App) button(id, glyph string, do func()) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Size = style.Reach() * 0.5
	return a.o.UI.Presses.Button(id, glyph, st, gogui.Color{}, func(e gogui.EventCtx) {
		do()
		e.Consume()
	})
}

func (a *App) mdiButton(id, name string, do func()) gogui.View {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Family, st.Size = icons.Family, style.Reach()*0.5
	return a.o.UI.Presses.Button(id, icons.Glyph(icons.Named(name)), st, gogui.Color{},
		func(e gogui.EventCtx) {
			do()
			e.Consume()
		})
}

func order(channels []yttv.Channel, filter string, favorites []string) []yttv.Channel {
	want := strings.ToLower(strings.TrimSpace(filter))
	var out []yttv.Channel
	for _, c := range channels {
		if want == "" || matches(c, want) {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(x, y yttv.Channel) int {
		fx, fy := slices.Contains(favorites, x.Key()), slices.Contains(favorites, y.Key())
		if fx != fy {
			if fx {
				return -1
			}
			return 1
		}
		return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	return out
}

func matches(c yttv.Channel, want string) bool {
	if strings.Contains(strings.ToLower(c.Name), want) {
		return true
	}
	return slices.ContainsFunc(c.Airings(), func(a yttv.Airing) bool {
		return strings.Contains(strings.ToLower(a.Title), want)
	})
}

func (a *App) picture(from *covers.Covers, url string, w, h float32, glyph string) gogui.View {
	if src := from.Src(url, int(w)); src != "" {
		return gogui.Column(gogui.ContainerCfg{
			Width: w, Height: h, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
			Radius: gogui.RadiusPx(min(w, h) * 0.06), Clip: true,
			Content: []gogui.View{gogui.Image(gogui.ImageCfg{Src: src, Width: w, Height: h})},
		})
	}
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Family, st.Size = icons.Family, min(w, h)*0.45
	st.Color = widgets.Secondary().Color
	return gogui.Column(gogui.ContainerCfg{
		Width: w, Height: h, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
		Radius: gogui.RadiusPx(min(w, h) * 0.06), Color: gogui.CurrentTheme().Cfg.ColorPanel,
		HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(icons.Glyph(icons.Named(glyph)), st)},
	})
}

func (a *App) act(do func(ctx context.Context) error) {
	safe.Go("tv app", func() {
		ctx, cancel := context.WithTimeout(context.Background(), askTime)
		defer cancel()
		err := do(ctx)
		a.mu.Lock()
		a.problem = ""
		if err != nil {
			a.problem = fmt.Sprint(err)
		}
		a.mu.Unlock()
		a.redraw()
	})
}

func (a *App) download(ctx context.Context, url string) ([]byte, error) {
	if a.o.Client == nil || !strings.HasPrefix(url, "http") {
		return nil, fmt.Errorf("tvapp: no way to download %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tvapp: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, artLimit))
}

func (a *App) redraw() {
	if a.o.Redraw != nil {
		a.o.Redraw()
	}
}

func note(text string) gogui.View {
	return gogui.Label(text, gogui.CurrentTheme().Cfg.TextStyleDef)
}

func column(views ...gogui.View) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
		Content: views,
	})
}
