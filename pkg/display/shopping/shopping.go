package shopping

import (
	"slices"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/todo"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const page = "shopping"

type View struct{ draft string }

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Options struct {
	UI       *widgets.UI
	Shell    *shell.Shell
	Todo     *todo.Todo
	Gate     func(*gogui.Window) gogui.View
	Settings func() *shell.Page
}

type Shopping struct {
	o    Options
	view *View
}

func New(o Options) *Shopping { return &Shopping{o: o, view: &View{}} }

func (s *Shopping) App() apps.App {
	return apps.App{
		ID:   schema.AppShopping,
		Name: func() string { return say.T("todo.app") },
		Icon: "mdi:cart",
		Open: func() { s.o.Shell.Push(s.view) },
	}
}

func (s *Shopping) Screen(v shell.View) *navigation.Screen {
	if v != s.view {
		return nil
	}
	screen := &navigation.Screen{Title: s.title(), View: v, Build: s.build}
	if s.o.Settings != nil {
		screen.Settings = func() { s.o.Shell.Push(s.o.Settings()) }
	}
	return screen
}

func (s *Shopping) title() string {
	if lists := s.o.Todo.Lists(); len(lists) == 1 {
		return s.o.Todo.Title(lists[0])
	}
	return say.T("todo.app")
}

func (s *Shopping) build(w *gogui.Window) gogui.View {
	if gate := s.o.Gate(w); gate != nil {
		return gate
	}
	ui := s.o.UI
	items, err, loaded := s.o.Todo.Items()
	var views []gogui.View
	if lists := s.o.Todo.Lists(); len(lists) > 1 {
		views = append(views, s.tabs(lists))
	}
	views = append(views, s.field())
	switch {
	case s.o.Todo.Showing() == "":
		views = append(views, ui.Row("none", page, menu.Row{Label: say.T("todo.pick")}, nil, false))
	case !loaded:
		views = append(
			views,
			ui.Row("loading", page, menu.Row{Label: say.T("home.loading")}, nil, false),
		)
	case err != nil:
		views = append(
			views,
			ui.Row(
				"failed",
				page,
				menu.Row{Label: say.T("home.failed"), Hint: err.Error()},
				nil,
				false,
			),
			ui.Row(
				"retry",
				page,
				menu.Row{Label: say.T("home.retry")},
				func(int) { s.o.Todo.Refresh() },
				false,
			),
		)
	case len(items) == 0:
		views = append(
			views,
			ui.Row("empty", page, menu.Row{Label: say.T("todo.empty")}, nil, false),
		)
	}

	open := slices.DeleteFunc(slices.Clone(items), todo.Item.Done)
	done := slices.DeleteFunc(slices.Clone(items), func(i todo.Item) bool { return !i.Done() })
	for _, i := range open {
		views = append(views, s.item(i))
	}
	if len(done) > 0 && s.o.Todo.Completed() {
		views = append(views, widgets.Divider())
		for _, i := range done {
			views = append(views, s.item(i))
		}
		views = append(views, ui.Row(
			"clear", page,
			menu.Row{Glyph: "mdi:broom", Label: say.T("todo.clear")},
			func(int) { s.o.Todo.ClearDone() },
			true,
		))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		Content: views,
	})
}

func (s *Shopping) tabs(lists []string) gogui.View {
	tabs := make([]widgets.Tab, 0, len(lists))
	for _, entity := range lists {
		tabs = append(tabs, widgets.Tab{Key: entity, Label: s.o.Todo.Title(entity)})
	}
	return s.o.UI.TabStrip(widgets.TabStripCfg{
		ID:     "shopping-tabs",
		Tabs:   tabs,
		Chosen: s.o.Todo.Showing(),
		Pick:   s.o.Todo.Show,
	})
}

func (s *Shopping) item(i todo.Item) gogui.View {
	row := menu.Row{Glyph: "mdi:checkbox-blank-outline", Label: i.Summary}
	if i.Done() {
		row.Glyph, row.Dim = "mdi:checkbox-marked", true
	}
	return s.o.UI.Row("item-"+i.UID, page, row, func(int) { s.o.Todo.Toggle(i) }, true)
}

func (s *Shopping) field() gogui.View {
	add := func() {
		s.o.Todo.Add(s.view.draft)
		s.view.draft = ""
	}
	return s.o.UI.Input(gogui.InputCfg{
		ID:            "shopping-add",
		Text:          s.view.draft,
		Placeholder:   say.T("todo.add"),
		Sizing:        gogui.FillFit,
		OnTextChanged: func(text string, e gogui.EventCtx) { s.view.draft = text; e.Window.InvalidateLayout() },
		OnTextCommit: func(_ string, _ gogui.InputCommitReason, e gogui.EventCtx) {
			add()
			e.Window.InvalidateLayout()
		},
	}, add)
}
