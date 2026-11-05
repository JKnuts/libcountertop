package tvapp

import (
	"slices"
	"strconv"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func (a *App) Page() *shell.Page {
	return &shell.Page{
		Title: say.T("tv.youtube"),
		Build: func() ([]menu.Row, []func(int)) {
			_, tvs, _, _ := a.o.Guide.Load()
			chosen := ""
			if i := slices.IndexFunc(tvs, func(t yttv.TV) bool {
				return t.Entity == a.o.Guide.Default()
			}); i >= 0 {
				chosen = a.label(tvs[i])
			}
			favorites := strconv.Itoa(len(a.o.Guide.Favorites()))
			return []menu.Row{
				{Label: say.T("tv.setting.default"), Kind: menu.Chevron, Value: chosen},
				{Label: say.T("tv.setting.favorites"), Kind: menu.Chevron, Value: favorites},
			}, []func(int){
				func(int) { a.o.Shell.Push(a.defaultPage()) },
				func(int) { a.o.Shell.Push(a.favoritesPage()) },
			}
		},
	}
}

func (a *App) defaultPage() *shell.Page {
	return &shell.Page{
		Title: say.T("tv.setting.default"),
		Build: func() ([]menu.Row, []func(int)) {
			_, tvs, _, _ := a.o.Guide.Load()
			now := a.o.Guide.Default()
			var (
				rows []menu.Row
				taps []func(int)
			)
			for _, t := range a.sorted(tvs) {
				entity := t.Entity
				rows = append(rows, menu.Row{Label: a.label(t), Chosen: entity == now})
				taps = append(taps, func(int) {
					if err := a.o.Guide.SetDefault(entity); err == nil {
						a.redraw()
					}
				})
			}
			return rows, taps
		},
	}
}

func (a *App) favoritesPage() *shell.Page {
	query := new(string)
	return &shell.Page{
		Title: say.T("tv.setting.favorites"),
		Build: func() ([]menu.Row, []func(int)) { return a.channelRows("") },
		Search: &shell.Search{
			Query:       query,
			Placeholder: say.T("tv.find"),
			Results: func() *shell.Page {
				return &shell.Page{
					Title: say.T("tv.setting.favorites"),
					Build: func() ([]menu.Row, []func(int)) { return a.channelRows(*query) },
				}
			},
		},
	}
}

func (a *App) channelRows(filter string) ([]menu.Row, []func(int)) {
	channels, _, err, loaded := a.o.Guide.Load()
	switch {
	case !loaded:
		return []menu.Row{{Label: say.T("home.loading")}}, []func(int){nil}
	case err != nil && len(channels) == 0:
		return []menu.Row{{Label: say.T("tv.failed"), Hint: err.Error()}}, []func(int){nil}
	}
	favorites := a.o.Guide.Favorites()
	listed := order(channels, filter, favorites)
	now := time.Now()
	rows := make([]menu.Row, 0, len(listed))
	taps := make([]func(int), 0, len(listed))
	for _, c := range listed {
		on := slices.Contains(favorites, c.Key())
		row := menu.Row{Label: c.Name, Kind: menu.Toggle, On: on}
		if airing, ok := c.At(now); ok {
			row.Hint = airing.Title
		}
		key := c.Key()
		rows = append(rows, row)
		taps = append(taps, func(int) {
			if err := a.o.Guide.SetFavorite(key, !on); err == nil {
				a.redraw()
			}
		})
	}
	return rows, taps
}
