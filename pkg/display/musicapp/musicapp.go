package musicapp

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/musicassistant"
	"github.com/ygelfand/libcountertop/pkg/network/peer"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const FetchTime = askTime

const (
	askTime    = 15 * time.Second
	volumeWait = 300 * time.Millisecond
	startWait  = 15 * time.Second
	artLimit   = 4 << 20
)

type tab int

const (
	playingTab tab = iota
	homeTab
	searchTab
	libraryTab
	speakersTab
)

type Peers interface {
	Peers() []peer.Peer
}

type Options struct {
	UI     *widgets.UI
	Shell  *shell.Shell
	Music  *musicassistant.Music
	Peers  Peers
	Logo   func() image.Image
	Site   func() string
	Client *http.Client
	Gate   func(*gogui.Window) gogui.View
	Redraw func()
}

type View struct{}

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

func (*View) Presents() bool { return true }

type App struct {
	o    Options
	view *View

	mu      sync.Mutex
	tab     tab
	chosen  string
	filter  string
	query   string
	typed   string
	pause   *time.Timer
	shelf   musicassistant.Shelf
	opened  []homeassistant.MusicItem
	pending pending
	problem string
	volumes map[string]float64
	timers  map[string]*time.Timer
	covers  *covers
}

func New(o Options) *App {
	a := &App{
		o:       o,
		view:    &View{},
		volumes: map[string]float64{},
		timers:  map[string]*time.Timer{},
	}
	a.covers = newCovers(a.download, a.redraw)
	return a
}

func (a *App) App() apps.App {
	return apps.App{
		ID:   schema.AppMusic,
		Name: func() string { return say.T("music.app") },
		Icon: "mdi:music-box-multiple",
		Open: func() {
			a.o.Music.Refresh()
			a.o.Shell.Push(a.view)
		},
	}
}

func (a *App) Screen(v shell.View) *navigation.Screen {
	if v != a.view {
		return nil
	}
	return &navigation.Screen{View: v, Build: a.build}
}

func (a *App) build(w *gogui.Window) gogui.View {
	vw, vh := w.WindowSize()
	reach := style.Reach()
	wide := vw >= vh
	content := a.page(w)
	if gate := a.o.Gate(w); gate != nil {
		content = gate
	}
	body := gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NewPadding(reach*0.4, reach*0.5, reach*0.4, reach*0.5),
		Content: []gogui.View{content},
	})
	bar := a.tabs(wide)
	outer := gogui.ContainerCfg{
		Width:   float32(vw),
		Height:  float32(vh),
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Color:   gogui.CurrentTheme().Cfg.ColorBackground,
	}
	if wide {
		outer.Content = []gogui.View{bar, body}
		return gogui.Row(outer)
	}
	outer.Content = []gogui.View{body, bar}
	return gogui.Column(outer)
}

func (a *App) page(w *gogui.Window) gogui.View {
	a.mu.Lock()
	t := a.tab
	var opened *homeassistant.MusicItem
	if n := len(a.opened); n > 0 {
		opened = &a.opened[n-1]
	}
	a.mu.Unlock()
	list, err, loaded := a.o.Music.Speakers()
	switch {
	case !loaded:
		return note(say.T("home.loading"))
	case errors.Is(err, homeassistant.ErrNoMusic):
		return a.notice(say.T("music.integration"))
	case err != nil:
		return note(say.T("music.unavailable"))
	case len(list) == 0:
		return note(say.T("music.none"))
	}
	current := a.current(list)
	switch {
	case t == speakersTab:
		return a.speakers(w, list, current)
	case t == playingTab:
		return a.playing(w, list, current)
	case opened != nil:
		return a.collection(w, *opened, current)
	case t == homeTab:
		return a.home(w, current)
	case t == searchTab:
		return a.search(w, current)
	}
	return a.library(w, current)
}

func (a *App) current(list []*musicassistant.Speaker) *musicassistant.Speaker {
	a.mu.Lock()
	chosen := a.chosen
	a.mu.Unlock()
	var mine, playing *musicassistant.Speaker
	for _, s := range list {
		switch {
		case s.Entity == chosen:
			return s
		case s.Mine:
			mine = s
		case playing == nil && s.Now().State == "playing":
			playing = s
		}
	}
	switch {
	case mine != nil:
		return mine
	case playing != nil:
		return playing
	}
	return list[0]
}

func (a *App) choose(entity string, t tab) {
	a.mu.Lock()
	a.chosen, a.tab = entity, t
	a.mu.Unlock()
	a.redraw()
}

func (a *App) open(t tab) {
	a.mu.Lock()
	a.tab, a.opened = t, nil
	a.mu.Unlock()
	a.redraw()
}

func (a *App) tabs(wide bool) gogui.View {
	reach := style.Reach()
	a.mu.Lock()
	open := a.tab
	a.mu.Unlock()
	glyph := gogui.CurrentTheme().TextStyleIconMedium
	glyph.Family = icons.Family
	glyph.Size = reach * 0.7
	small := widgets.Secondary()
	key := func(id, icon, label string, chosen bool, do func()) gogui.View {
		cfg := gogui.ContainerCfg{
			ID:      id,
			Sizing:  gogui.FillFit,
			Padding: gogui.NewPadding(reach*0.25, reach*0.1, reach*0.25, reach*0.1),
			Spacing: gogui.SpacingSmall,
			HAlign:  gogui.HAlignCenter,
		}
		g, l := glyph, small
		a.o.UI.Toolkit.Tab(&cfg, &g, widgets.Chosen(chosen))
		l.Color = g.Color
		cfg.Content = []gogui.View{
			gogui.Label(icons.Glyph(icons.Named(icon)), g),
			gogui.Label(label, l),
		}
		return a.o.UI.Toolkit.Pressable(gogui.Column, cfg, func(e gogui.EventCtx) {
			do()
			e.Consume()
		})
	}
	keys := []gogui.View{
		key("music-close", "mdi:close", say.T("music.close"), false,
			func() { a.o.Shell.Remove(a.view) }),
	}
	for _, t := range []struct {
		tab         tab
		id, icon, k string
	}{
		{playingTab, "playing", "mdi:play-circle-outline", "music.now"},
		{homeTab, "home", "mdi:home-variant-outline", "music.home"},
		{searchTab, "search", "mdi:magnify", "music.search"},
		{libraryTab, "library", "mdi:bookshelf", "music.library"},
		{speakersTab, "speakers", "mdi:speaker-multiple", "music.speakers"},
	} {
		keys = append(keys, key("music-tab-"+t.id, t.icon, say.T(t.k), open == t.tab,
			func() { a.open(t.tab) }))
	}
	cfg := gogui.ContainerCfg{
		ID:      "music-tabs",
		Padding: gogui.PaddingSmall,
		Spacing: gogui.SpacingSmall,
		Color:   gogui.CurrentTheme().Cfg.ColorPanel,
		Content: keys,
	}
	if wide {
		cfg.Width, cfg.Sizing, cfg.Scrollable = reach*2.4, gogui.FixedFill, true
		return gogui.Column(cfg)
	}
	cfg.Sizing = gogui.FillFit
	return gogui.Row(cfg)
}

func (a *App) act(do func(ctx context.Context) error) {
	safe.Go("music app", func() {
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

type pending struct {
	entity, item, title, was string
	at                       time.Time
}

func (a *App) start(s *musicassistant.Speaker, item, title string) {
	a.mu.Lock()
	a.pending = pending{
		entity: s.Entity,
		item:   item,
		title:  title,
		was:    s.Now().Title,
		at:     time.Now(),
	}
	a.mu.Unlock()
	time.AfterFunc(startWait, a.redraw)
	a.redraw()
}

func (a *App) starting(s *musicassistant.Speaker) (pending, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pending
	if p.entity != s.Entity {
		return pending{}, false
	}
	if s.Now().Title != p.was || time.Since(p.at) >= startWait {
		a.pending = pending{}
		return pending{}, false
	}
	return p, true
}

func (a *App) setVolume(entity string, level float64) {
	a.mu.Lock()
	a.volumes[entity] = level
	if t := a.timers[entity]; t != nil {
		t.Stop()
	}
	a.timers[entity] = time.AfterFunc(volumeWait, func() {
		a.act(func(ctx context.Context) error {
			defer func() {
				a.mu.Lock()
				delete(a.volumes, entity)
				a.mu.Unlock()
			}()
			return a.o.Music.Volume(ctx, entity, level)
		})
	})
	a.mu.Unlock()
	a.redraw()
}

func (a *App) volume(s *musicassistant.Speaker) float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v, ok := a.volumes[s.Entity]; ok {
		return v
	}
	return s.Now().Volume
}

func (a *App) absolute(url string) string {
	if strings.HasPrefix(url, "/") && a.o.Site != nil {
		return a.o.Site() + url
	}
	return url
}

func (a *App) download(ctx context.Context, url string) ([]byte, error) {
	url = a.absolute(url)
	if a.o.Client == nil || !strings.HasPrefix(url, "http") {
		return nil, fmt.Errorf("musicapp: no way to download %q", url)
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
		return nil, fmt.Errorf("musicapp: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, artLimit))
}

func (a *App) cover(url string, side float32) gogui.View {
	if src := a.covers.src(url, int(side)); src != "" {
		return gogui.Column(gogui.ContainerCfg{
			Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
			Radius: gogui.RadiusPx(side * 0.04), Clip: true,
			Content: []gogui.View{
				gogui.Image(gogui.ImageCfg{Src: src, Width: side, Height: side}),
			},
		})
	}
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Family, st.Size = icons.Family, side*0.4
	st.Color = widgets.Secondary().Color
	return gogui.Column(gogui.ContainerCfg{
		Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
		Radius: gogui.RadiusPx(side * 0.04), Color: gogui.CurrentTheme().Cfg.ColorPanel,
		HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(icons.Glyph(icons.Named("mdi:music")), st)},
	})
}

func (a *App) redraw() {
	if a.o.Redraw != nil {
		a.o.Redraw()
	}
}

func (a *App) notice(text string) gogui.View {
	if a.o.Logo == nil {
		return note(text)
	}
	t := gogui.CurrentTheme().Cfg
	tall := t.TextStyleDef.Size * 2.5
	mark := a.o.Logo()
	mw, mh := mark.Bounds().Dx(), mark.Bounds().Dy()
	card := gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FitFit,
		VAlign:  gogui.VAlignMiddle,
		Padding: gogui.PaddingLarge,
		Spacing: gogui.SpacingLarge,
		Radius:  gogui.RadiusLarge,
		Color:   widgets.Color(a.o.UI.Palette().Surface),
		Content: []gogui.View{
			gogui.Image(gogui.ImageCfg{
				Src:    widgets.ImageSource(fmt.Sprintf("music-logo-%p", mark), mark),
				Width:  tall * float32(mw) / float32(max(mh, 1)),
				Height: tall,
			}),
			gogui.Label(text, t.TextStyleDef),
		},
	})
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{card},
	})
}

func note(text string) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(text, widgets.Secondary())},
	})
}
