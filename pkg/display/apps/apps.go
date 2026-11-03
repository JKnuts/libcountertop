package apps

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/display/drawer"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type App struct {
	ID   string
	Name func() string
	Icon string
	Open func()
}

func (a App) Label() string {
	if a.Name == nil {
		return a.ID
	}
	return a.Name()
}

type Options struct {
	Settings storage.Section[schema.Apps]
	DeviceID uint32
	Shell    *shell.Shell
}

type View struct{ apps *Apps }

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Apps struct {
	options Options
	knobs   *setting.Controls[schema.Apps]
	view    *View

	mu   sync.Mutex
	list []App
	drag drag
	held widgets.Drag
}

type drag struct {
	from    int
	dx, dy  float32
	dropped time.Time
}

func New(options Options) *Apps {
	a := &Apps{options: options, drag: drag{from: -1}}
	a.view = &View{apps: a}
	a.knobs = &setting.Controls[schema.Apps]{
		Table:  NewTable(),
		Device: options.DeviceID,
		Read:   options.Settings.Read,
		Save:   a.save,
	}
	return a
}

func (a *Apps) Name() string { return "apps" }

func (a *Apps) Add(app App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.list = append(a.list, app)
}

func (a *Apps) List() []App {
	a.mu.Lock()
	list := slices.Clone(a.list)
	a.mu.Unlock()
	return ordered(list, a.options.Settings.Read().Order)
}

func ordered(list []App, order []string) []App {
	out := make([]App, 0, len(list))
	for _, id := range order {
		at := slices.IndexFunc(list, func(app App) bool { return app.ID == id })
		if at >= 0 {
			out = append(out, list[at])
			list = slices.Delete(list, at, at+1)
		}
	}
	return append(out, list...)
}

func (a *Apps) Docked(id string) bool {
	return slices.Contains(a.options.Settings.Read().Dock, id)
}

func (a *Apps) SetDocked(id string, on bool) {
	a.set("dock", func(c *schema.Apps) {
		dock := slices.DeleteFunc(
			slices.Clone(c.Dock),
			func(have string) bool { return have == id },
		)
		if on {
			dock = append(dock, id)
		}
		c.Dock = dock
	})
}

func (a *Apps) Move(from, to int) {
	list := a.List()
	if from == to || from < 0 || to < 0 || from >= len(list) || to >= len(list) {
		return
	}
	moved := list[from]
	list = slices.Insert(slices.Delete(list, from, from+1), to, moved)
	order := make([]string, len(list))
	for n, app := range list {
		order[n] = app.ID
	}
	a.set("order", func(c *schema.Apps) { c.Order = order })
}

func (a *Apps) Rail() drawer.Entry {
	return drawer.Entry{
		Name:  func() string { return say.T("apps.title") },
		Order: drawer.OrderApps,
		Icon:  "mdi:apps",
		Open:  a.open,
	}
}

func (a *Apps) open() {
	a.mu.Lock()
	a.drag = drag{from: -1}
	a.mu.Unlock()
	a.options.Shell.Push(a.view)
}

func (a *Apps) Dock() []drawer.Entry {
	dock := a.options.Settings.Read().Dock
	var out []drawer.Entry
	for _, app := range a.List() {
		if !slices.Contains(dock, app.ID) {
			continue
		}
		out = append(out, drawer.Entry{
			Name:  app.Label,
			Order: drawer.OrderFavorites + len(out),
			Icon:  app.Icon,
			Open:  app.Open,
		})
	}
	return out
}

func (a *Apps) Entities() []esphome.Entity { return a.knobs.Entities() }

func (a *Apps) Settings() *setting.Controls[schema.Apps] { return a.knobs }

func (a *Apps) Restore(c schema.Apps) { a.knobs.PublishFrom(c) }

func (a *Apps) save(s Setting, v string) error {
	if err := setting.Store(a.options.Settings, s, v); err != nil {
		return err
	}
	a.options.Shell.Redraw()
	return nil
}

func (a *Apps) set(what string, change func(*schema.Apps)) {
	if err := a.options.Settings.Update(change); err != nil {
		slog.Error("saving a setting failed", "setting", what, "err", err)
	}
	a.knobs.Publish()
	a.options.Shell.Redraw()
}
