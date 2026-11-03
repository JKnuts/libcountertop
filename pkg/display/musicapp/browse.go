package musicapp

import (
	"context"
	"strconv"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/musicassistant"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	cardUnits = 3.4
	homeRows  = 1
	typePause = 600 * time.Millisecond
)

var searchOrder = []string{"artist", "album", "track", "playlist", "radio"}

var shelves = []struct {
	shelf musicassistant.Shelf
	label string
}{
	{musicassistant.Playlists, "music.playlists"},
	{musicassistant.Albums, "music.albums"},
	{musicassistant.Artists, "music.artists"},
	{musicassistant.Radio, "music.radio"},
}

func opens(it homeassistant.MusicItem) bool {
	return it.Type == "album" || it.Type == "playlist" || it.Type == "artist"
}

func (a *App) width(w *gogui.Window) float32 {
	vw, vh := w.WindowSize()
	reach := style.Reach()
	if vw >= vh {
		return float32(vw) - reach*3.6
	}
	return float32(vw) - reach*1.2
}

func (a *App) home(w *gogui.Window, s *musicassistant.Speaker) gogui.View {
	h, err, loaded := a.o.Music.Home()
	switch {
	case !loaded:
		return note(say.T("home.loading"))
	case err != nil:
		return note(say.T("music.unavailable"))
	}
	width := a.width(w)
	views := []gogui.View{}
	if len(h.Recent) > 0 {
		views = append(views, a.section(w, "recent", say.T("music.recent"), h.Recent, width, s))
	}
	for i, f := range h.Folders {
		views = append(views, a.section(w, "folder-"+strconv.Itoa(i), f.Name, f.Items, width, s))
	}
	if len(views) == 0 {
		return note(say.T("music.nothing"))
	}
	return a.scroller("music-home", views)
}

func (a *App) section(
	w *gogui.Window,
	id, title string,
	items []homeassistant.MusicItem,
	width float32,
	s *musicassistant.Speaker,
) gogui.View {
	per := columns(width)
	shown := items[:min(len(items), per*homeRows)]
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			gogui.Label(title, widgets.Secondary()),
			a.grid(w, "music-"+id, shown, width, s),
		},
	})
}

func (a *App) search(w *gogui.Window, s *musicassistant.Speaker) gogui.View {
	a.mu.Lock()
	query, typed := a.query, a.typed
	a.mu.Unlock()
	field := a.o.UI.Search(gogui.InputCfg{
		ID:          "music-search",
		Text:        typed,
		Placeholder: say.T("music.searching"),
		Sizing:      gogui.FillFit,
		OnTextChanged: func(text string, e gogui.EventCtx) {
			a.typing(text)
			e.Window.InvalidateLayout()
		},
	}, a.commit)
	views := []gogui.View{field}
	if query == "" {
		return gogui.Column(column(views))
	}
	found, err, loaded := a.o.Music.Search(query)
	width := a.width(w)
	switch {
	case !loaded:
		views = append(views, gogui.Label(say.T("home.loading"), widgets.Secondary()))
	case err != nil:
		views = append(views, gogui.Label(say.T("music.unavailable"), widgets.Secondary()))
	case len(found) == 0:
		views = append(views, gogui.Label(say.T("music.nothing"), widgets.Secondary()))
	}
	var results []gogui.View
	for _, kind := range searchOrder {
		var of []homeassistant.MusicItem
		for _, it := range found {
			if it.Type == kind {
				of = append(of, it)
			}
		}
		if len(of) > 0 {
			results = append(results, a.section(w, "found-"+kind, say.T("music.kind."+kind),
				of[:min(len(of), columns(width)*2)], width, s))
		}
	}
	views = append(views, a.scroller("music-found", results))
	return gogui.Column(column(views))
}

func (a *App) library(w *gogui.Window, s *musicassistant.Speaker) gogui.View {
	a.mu.Lock()
	open := a.shelf
	a.mu.Unlock()
	if open == "" {
		open = musicassistant.Playlists
	}
	chips := make([]gogui.View, 0, len(shelves))
	for _, sh := range shelves {
		chips = append(chips, a.toggle("music-shelf-"+string(sh.shelf), shelfIcon(sh.shelf),
			say.T(sh.label), sh.shelf == open, func() {
				a.mu.Lock()
				a.shelf = sh.shelf
				a.mu.Unlock()
				a.redraw()
			}))
	}
	views := []gogui.View{gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall,
		Content: chips,
	})}
	items, err, loaded := a.o.Music.Library(open)
	switch {
	case !loaded:
		views = append(views, gogui.Label(say.T("home.loading"), widgets.Secondary()))
	case err != nil:
		views = append(views, gogui.Label(say.T("music.unavailable"), widgets.Secondary()))
	case len(items) == 0:
		views = append(views, gogui.Label(say.T("music.nothing"), widgets.Secondary()))
	default:
		views = append(views, a.scroller("music-shelf",
			[]gogui.View{a.grid(w, "music-shelf-items", items, a.width(w), s)}))
	}
	return gogui.Column(column(views))
}

func (a *App) collection(
	w *gogui.Window,
	it homeassistant.MusicItem,
	s *musicassistant.Speaker,
) gogui.View {
	reach := style.Reach()
	width := a.width(w)
	title := gogui.CurrentTheme().Cfg.TextStyleDef
	title.Size *= 1.5
	kind := say.T("music.kind." + it.Type)
	if it.Artist != "" {
		kind += " · " + it.Artist
	}
	actions := gogui.Row(gogui.ContainerCfg{
		Sizing: gogui.FitFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall,
		Content: []gogui.View{
			a.toggle("music-play-now", "mdi:play", say.T("music.play"), true,
				func() { a.play(s, it, musicassistant.PlayNow, false) }),
			a.toggle("music-play-next", "mdi:playlist-play", say.T("music.next"), false,
				func() { a.play(s, it, musicassistant.PlayNext, false) }),
			a.toggle("music-play-add", "mdi:playlist-plus", say.T("music.add"), false,
				func() { a.play(s, it, musicassistant.AddLast, false) }),
			a.toggle("music-play-radio", "mdi:radio-tower", say.T("music.similar"), false,
				func() { a.play(s, it, musicassistant.PlayNow, true) }),
		},
	})
	head := gogui.Row(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingLarge,
		VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{
			a.toggle("music-back", "mdi:arrow-left", say.T("music.back"), false, a.back),
			a.cover(it.Image, reach*3),
			gogui.Column(gogui.ContainerCfg{
				Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall,
				Content: []gogui.View{
					gogui.Label(widgets.Ellipsize(w, it.Name, title, width*0.5), title),
					gogui.Label(widgets.Ellipsize(w, kind, widgets.Secondary(), width*0.5),
						widgets.Secondary()),
					actions,
				},
			}),
		},
	})
	in, err, loaded := a.o.Music.Contents(it.URI)
	views := []gogui.View{head}
	switch {
	case !loaded:
		views = append(views, gogui.Label(say.T("home.loading"), widgets.Secondary()))
	case err != nil:
		views = append(views, gogui.Label(say.T("music.unavailable"), widgets.Secondary()))
	}
	var tracks, held []homeassistant.MusicItem
	for _, x := range in.Items {
		if opens(x) {
			held = append(held, x)
		} else {
			tracks = append(tracks, x)
		}
	}
	var body []gogui.View
	if len(held) > 0 {
		body = append(body, a.grid(w, "music-collection-held", held, width, s))
	}
	for i, t := range tracks {
		body = append(body, a.trackRow(w, "music-collection-"+strconv.Itoa(i), t, width, s))
	}
	views = append(views, a.scroller("music-collection", body))
	return gogui.Column(column(views))
}

func (a *App) trackRow(
	w *gogui.Window,
	id string,
	t homeassistant.MusicItem,
	width float32,
	s *musicassistant.Speaker,
) gogui.View {
	reach := style.Reach()
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	lines := []gogui.View{gogui.Label(widgets.Ellipsize(w, t.Name, body, width*0.7), body)}
	if t.Artist != "" {
		lines = append(lines, gogui.Label(
			widgets.Ellipsize(w, t.Artist, widgets.Secondary(), width*0.7), widgets.Secondary()))
	}
	return a.o.UI.Toolkit.Pressable(gogui.Row, gogui.ContainerCfg{
		ID: id, Sizing: gogui.FillFit, Padding: gogui.PaddingSmall,
		Spacing: gogui.SpacingMedium, Radius: gogui.RadiusMedium, VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{
			a.cover(t.Image, reach*1.2),
			gogui.Column(gogui.ContainerCfg{
				Sizing: gogui.FillFit, Padding: gogui.NoPadding, Clip: true, Content: lines,
			}),
		},
	}, func(e gogui.EventCtx) {
		a.play(s, t, musicassistant.PlayNow, false)
		e.Consume()
	})
}

func (a *App) grid(
	w *gogui.Window,
	id string,
	items []homeassistant.MusicItem,
	width float32,
	s *musicassistant.Speaker,
) gogui.View {
	per := columns(width)
	gap := gogui.CurrentTheme().Cfg.SpacingMedium
	side := (width - gap*float32(per-1)) / float32(per)
	var rows []gogui.View
	for start := 0; start < len(items); start += per {
		var cards []gogui.View
		for i, it := range items[start:min(start+per, len(items))] {
			cards = append(cards, a.card(w, id+"-"+strconv.Itoa(start+i), it, side, s))
		}
		rows = append(rows, gogui.Row(gogui.ContainerCfg{
			Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
			Content: cards,
		}))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
		Content: rows,
	})
}

func (a *App) card(
	w *gogui.Window,
	id string,
	it homeassistant.MusicItem,
	side float32,
	s *musicassistant.Speaker,
) gogui.View {
	body := gogui.CurrentTheme().Cfg.TextStyleDef
	lines := []gogui.View{
		a.cover(it.Image, side),
		gogui.Label(widgets.Ellipsize(w, it.Name, body, side), body),
	}
	under := it.Artist
	if under == "" {
		under = say.T("music.kind." + it.Type)
	}
	lines = append(lines,
		gogui.Label(widgets.Ellipsize(w, under, widgets.Secondary(), side), widgets.Secondary()))
	return a.o.UI.Toolkit.Pressable(gogui.Column, gogui.ContainerCfg{
		ID: id, Width: side, Sizing: gogui.FixedFit, Padding: gogui.NoPadding,
		Spacing: gogui.SpacingSmall, Radius: gogui.RadiusMedium, Content: lines,
	}, func(e gogui.EventCtx) {
		if opens(it) {
			a.push(it)
		} else {
			a.play(s, it, musicassistant.PlayNow, false)
		}
		e.Consume()
	})
}

func (a *App) play(
	s *musicassistant.Speaker,
	it homeassistant.MusicItem,
	how musicassistant.Enqueue,
	radio bool,
) {
	entity := s.Entity
	if how == musicassistant.PlayNow {
		a.start(s, "", it.Name)
		a.open(playingTab)
	}
	a.act(func(ctx context.Context) error { return a.o.Music.Play(ctx, entity, it, how, radio) })
}

func (a *App) typing(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.typed = text
	if a.pause != nil {
		a.pause.Stop()
	}
	a.pause = time.AfterFunc(typePause, a.commit)
}

func (a *App) commit() {
	a.mu.Lock()
	if a.pause != nil {
		a.pause.Stop()
	}
	changed := a.query != a.typed
	a.query = a.typed
	a.mu.Unlock()
	if changed {
		a.redraw()
	}
}

func (a *App) push(it homeassistant.MusicItem) {
	a.mu.Lock()
	a.opened = append(a.opened, it)
	a.mu.Unlock()
	a.redraw()
}

func (a *App) back() {
	a.mu.Lock()
	if n := len(a.opened); n > 0 {
		a.opened = a.opened[:n-1]
	}
	a.mu.Unlock()
	a.redraw()
}

func columns(width float32) int {
	return max(2, int(width/(style.Reach()*cardUnits)))
}

func shelfIcon(s musicassistant.Shelf) string {
	switch s {
	case musicassistant.Albums:
		return "mdi:album"
	case musicassistant.Artists:
		return "mdi:account-music"
	case musicassistant.Radio:
		return "mdi:radio"
	}
	return "mdi:playlist-music"
}

func column(views []gogui.View) gogui.ContainerCfg {
	return gogui.ContainerCfg{
		Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.SpacingMedium,
		Content: views,
	}
}

func (a *App) scroller(id string, views []gogui.View) gogui.View {
	return gogui.Column(gogui.ContainerCfg{
		ID: id, Sizing: gogui.FillFill, Padding: gogui.NewPadding(0, style.Reach()*0.4, 0, 0),
		Spacing: gogui.SpacingLarge, Scrollable: true, OnGesture: a.o.UI.HoldStill,
		Content: views,
	})
}
