package homeview

import (
	"slices"
	"strconv"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/settings"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
	"github.com/ygelfand/libcountertop/pkg/say"
	schema "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func (view *Renderer) ringsRows() ([]menu.Row, []func(int)) {
	engine := view.options.Engine
	cfg := engine.RingsShown()
	kinds := homecontrol.Kinds()
	areas, _ := engine.RingAreas()
	floors, _ := engine.RingFloors()
	labels := say.T("home.all")
	if len(cfg.Labels) > 0 {
		labels = say.F("home.picked.labels", map[string]any{"N": len(cfg.Labels)})
	}
	rows := []menu.Row{
		settings.ModeRow(
			settings.ModeMarks{},
			say.T("rings.look"),
			schema.RingsLooks(),
			engine.RingsLook(),
		),
		settings.ModeRow(
			settings.ModeMarks{},
			say.T("rings.group"),
			schema.RingsGroups(),
			engine.RingsGroup(),
		),
		depthRow(engine.RingsDepth()),
		settings.ModeRow(
			settings.ModeMarks{},
			say.T("rings.side"),
			schema.RingsSides(),
			engine.RingsSide(),
		),
		{
			Label: say.T("rings.floors"),
			Kind:  menu.Chevron,
			Value: count(len(floors)-hidden(floors, cfg.Floors), len(floors)),
			Open:  func() shell.View { return view.ringFloors() },
		},
		{
			Label: say.T("rings.areas"),
			Kind:  menu.Chevron,
			Value: count(len(areas)-hidden(areas, cfg.Areas), len(areas)),
			Open:  func() shell.View { return view.ringAreas() },
		},
		{
			Label: say.T("rings.kinds"),
			Kind:  menu.Chevron,
			Value: count(len(kinds)-len(cfg.Kinds), len(kinds)),
			Open:  func() shell.View { return view.ringKinds() },
		},
		{
			Label: say.T("home.tab.labels"),
			Kind:  menu.Chevron,
			Value: labels,
			Open:  func() shell.View { return view.ringLabels() },
		},
	}
	taps := []func(int){
		settings.ModePick(schema.RingsLooks(), engine.SetRingsLook),
		settings.ModePick(schema.RingsGroups(), engine.SetRingsGroup),
		func(level int) { engine.SetRingsDepth(depthOf(level)) },
		settings.ModePick(schema.RingsSides(), engine.SetRingsSide),
		nil, nil, nil, nil,
	}
	return rows, taps
}

func depthRow(n int) menu.Row {
	return menu.Row{
		Label: say.T("rings.depth"),
		Kind:  menu.Slider,
		Level: depthLevel(n),
		Snap:  func(level int) int { return depthLevel(depthOf(level)) },
		Value: strconv.Itoa(n),
	}
}

func depthOf(level int) int {
	return min(max(1+(level*(schema.RingsDeepest-1)+50)/100, 1), schema.RingsDeepest)
}

func depthLevel(n int) int { return (n - 1) * 100 / (schema.RingsDeepest - 1) }

func count(n, of int) string {
	if n == of {
		return say.T("home.all")
	}
	return say.F("home.area.count", map[string]any{"N": n, "Of": of})
}

func hidden(places []homecontrol.Place, ids []string) int {
	n := 0
	for _, p := range places {
		if slices.Contains(ids, p.ID) {
			n++
		}
	}
	return n
}

func (view *Renderer) ringFloors() *shell.Page {
	engine := view.options.Engine
	return &shell.Page{
		Title: say.T("rings.floors"),
		Build: func() ([]menu.Row, []func(int)) {
			floors, loading := engine.RingFloors()
			off := engine.RingsShown().Floors
			return toggles(
				floors,
				loading,
				func(id string) bool { return !slices.Contains(off, id) },
				engine.ShowRingFloor,
			)
		},
	}
}

func (view *Renderer) ringAreas() *shell.Page {
	engine := view.options.Engine
	return &shell.Page{
		Title: say.T("rings.areas"),
		Build: func() ([]menu.Row, []func(int)) {
			areas, loading := engine.RingAreas()
			off := engine.RingsShown().Areas
			return toggles(
				areas,
				loading,
				func(id string) bool { return !slices.Contains(off, id) },
				engine.ShowRingArea,
			)
		},
	}
}

func (view *Renderer) ringKinds() *shell.Page {
	engine := view.options.Engine
	return &shell.Page{
		Title: say.T("rings.kinds"),
		Build: func() ([]menu.Row, []func(int)) {
			var kinds []homecontrol.Place
			for _, k := range homecontrol.Kinds() {
				kinds = append(kinds, homecontrol.Place{ID: k, Name: homecontrol.KindName(k)})
			}
			off := engine.RingsShown().Kinds
			return toggles(
				kinds,
				false,
				func(id string) bool { return !slices.Contains(off, id) },
				engine.ShowRingKind,
			)
		},
	}
}

func (view *Renderer) ringLabels() *shell.Page {
	engine := view.options.Engine
	return &shell.Page{
		Title: say.T("home.tab.labels"),
		Build: func() ([]menu.Row, []func(int)) {
			labels, loading := engine.RingLabels()
			on := engine.RingsShown().Labels
			return toggles(
				labels,
				loading,
				func(id string) bool { return slices.Contains(on, id) },
				engine.PickRingLabel,
			)
		},
	}
}

func toggles(
	places []homecontrol.Place,
	loading bool,
	on func(string) bool,
	set func(string, bool),
) ([]menu.Row, []func(int)) {
	switch {
	case loading:
		return []menu.Row{{Label: say.T("home.loading")}}, []func(int){nil}
	case len(places) == 0:
		return []menu.Row{{Label: say.T("home.empty")}}, []func(int){nil}
	}
	rows := make([]menu.Row, 0, len(places))
	taps := make([]func(int), 0, len(places))
	for _, p := range places {
		shown := on(p.ID)
		rows = append(rows, menu.Row{Label: p.Name, Kind: menu.Toggle, On: shown})
		taps = append(taps, func(int) { set(p.ID, !shown) })
	}
	return rows, taps
}
