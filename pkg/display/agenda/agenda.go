package agenda

import (
	"strconv"
	"strings"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/calendar"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const page = "agenda"

type View struct{}

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Options struct {
	UI         *widgets.UI
	Shell      *shell.Shell
	Calendar   *calendar.Calendar
	Gate       func(*gogui.Window) gogui.View
	TwentyFour func() bool
	Settings   func() *shell.Page
}

type Agenda struct {
	o    Options
	view *View
}

func New(o Options) *Agenda { return &Agenda{o: o, view: &View{}} }

func (a *Agenda) App() apps.App {
	return apps.App{
		ID:   schema.AppCalendar,
		Name: func() string { return say.T("calendar.app") },
		Icon: "mdi:calendar",
		Open: func() { a.o.Shell.Push(a.view) },
	}
}

func (a *Agenda) Screen(v shell.View) *navigation.Screen {
	if v != a.view {
		return nil
	}
	return &navigation.Screen{
		Title:    say.T("calendar.app"),
		View:     v,
		Build:    a.build,
		Settings: a.settings(),
	}
}

func (a *Agenda) settings() func() {
	if a.o.Settings == nil {
		return nil
	}
	return func() { a.o.Shell.Push(a.o.Settings()) }
}

func (a *Agenda) build(w *gogui.Window) gogui.View {
	if gate := a.o.Gate(w); gate != nil {
		return gate
	}
	ui := a.o.UI
	note := func(id, label string) gogui.View {
		return ui.Row(id, page, menu.Row{Label: label}, nil, false)
	}
	events, err, loaded := a.o.Calendar.Events()
	var views []gogui.View
	switch {
	case len(a.o.Calendar.Chosen()) == 0:
		var pick func(int)
		if open := a.settings(); open != nil {
			pick = func(int) { open() }
		}
		views = append(
			views,
			ui.Row("none", page, menu.Row{Label: say.T("calendar.pick")}, pick, false),
		)
	case !loaded:
		views = append(views, note("loading", say.T("home.loading")))
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
			ui.Row("retry", page, menu.Row{Label: say.T("home.retry")},
				func(int) { a.o.Calendar.Refresh() }, false),
		)
	case len(events) == 0:
		views = append(views, note("empty", say.T("calendar.empty")))
	}

	now := time.Now()
	today := day(now)
	var shown time.Time
	for n, e := range events {
		if !e.AllDay && e.End.Before(now) {
			continue
		}
		d := day(e.Start)
		if d.Before(today) {
			d = today
		}
		if !d.Equal(shown) {
			if !shown.IsZero() {
				views = append(views, widgets.Divider())
			}
			views = append(views, heading(label(d, today)))
			shown = d
		}
		views = append(views, a.event(n, e))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Spacing: gogui.SpacingMedium,
		Content: views,
	})
}

func (a *Agenda) event(n int, e calendar.Event) gogui.View {
	at := say.T("calendar.allday")
	if !e.AllDay {
		at = e.Start.Format(a.layout())
	}
	hint := a.o.Calendar.Title(e.Calendar)
	if l := strings.TrimSpace(e.Location); l != "" {
		hint += " · " + l
	}
	return a.o.UI.Row(
		"event-"+strconv.Itoa(n), page,
		menu.Row{Label: e.Summary, Hint: hint, Value: at},
		nil, false,
	)
}

func (a *Agenda) layout() string {
	if a.o.TwentyFour != nil && a.o.TwentyFour() {
		return "15:04"
	}
	return "3:04 PM"
}

func heading(text string) gogui.View {
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	st.Size *= 1.15
	return gogui.Label(text, st)
}

func label(d, today time.Time) string {
	switch {
	case d.Equal(today):
		return say.T("calendar.today")
	case d.Equal(today.AddDate(0, 0, 1)):
		return say.T("calendar.tomorrow")
	}
	return d.Format("Monday, January 2")
}

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
