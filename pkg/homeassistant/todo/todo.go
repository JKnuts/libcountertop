package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/states"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

const (
	fetchWait   = 10 * time.Second
	itemsAction = "todo.get_items"
	itemsShape  = "{{ response | to_json }}"
	preferred   = "todo.shopping_list"
	statusDone  = "completed"
	statusOpen  = "needs_action"
)

type Item struct {
	UID     string `json:"uid"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func (i Item) Done() bool { return i.Status == statusDone }

type Options struct {
	Controller *homeassistant.Controller
	States     *states.States
	Settings   storage.Section[schema.Todo]
	Redraw     func()
	DeviceID   uint32
}

type Todo struct {
	opts  Options
	knobs *setting.Controls[schema.Todo]

	mu       sync.Mutex
	offered  []homeassistant.Entity
	failed   error
	fetching bool
	fetched  bool

	showing   string
	names     map[string]*states.Value
	following string
	count     *states.Value
	items     []Item
	loaded    bool
	problem   error
}

func New(opts Options) *Todo {
	t := &Todo{opts: opts}
	t.knobs = &setting.Controls[schema.Todo]{
		Table:  NewTable(),
		Device: opts.DeviceID,
		Read:   opts.Settings.Read,
		Save:   t.store,
	}
	return t
}

func (t *Todo) Offered() ([]homeassistant.Entity, error, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.offered, t.failed, t.fetched
}

func (t *Todo) Fetch() {
	t.mu.Lock()
	if t.fetching {
		t.mu.Unlock()
		return
	}
	t.fetching = true
	t.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
		defer cancel()
		list, err := t.opts.Controller.Entities(
			ctx,
			homeassistant.Filter{Domains: []string{"todo"}},
		)
		t.mu.Lock()
		t.offered, t.failed, t.fetching, t.fetched = list, err, false, true
		t.mu.Unlock()
		if err != nil {
			slog.Info("todo lists", "err", err)
		}
		if err == nil && len(list) > 0 && len(t.Lists()) == 0 {
			t.SetShown(pick(list), true)
		}
		t.opts.Redraw()
	}()
}

func pick(list []homeassistant.Entity) string {
	if i := slices.IndexFunc(
		list,
		func(e homeassistant.Entity) bool { return e.ID == preferred },
	); i >= 0 {
		return preferred
	}
	return list[0].ID
}

func (t *Todo) Lists() []string { return slices.Clone(t.opts.Settings.Read().Entities) }

func (t *Todo) Shown(entity string) bool { return slices.Contains(t.Lists(), entity) }

func (t *Todo) Showing() string {
	lists := t.Lists()
	if len(lists) == 0 {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if slices.Contains(lists, t.showing) {
		return t.showing
	}
	return lists[0]
}

func (t *Todo) Show(entity string) {
	t.mu.Lock()
	t.showing = entity
	t.mu.Unlock()
	t.opts.Redraw()
}

func (t *Todo) Title(entity string) string {
	if name, ok := t.name(entity).Get(); ok && name != "" {
		return name
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range t.offered {
		if e.ID == entity && e.Name != "" {
			return e.Name
		}
	}
	return entity
}

func (t *Todo) name(entity string) *states.Value {
	t.mu.Lock()
	v, ok := t.names[entity]
	if !ok {
		if t.names == nil {
			t.names = map[string]*states.Value{}
		}
		v = t.opts.States.Follow(entity, "friendly_name")
		t.names[entity] = v
	}
	t.mu.Unlock()
	if !ok {
		v.Changed.Listen(func(string) { t.opts.Redraw() })
	}
	return v
}

func (t *Todo) Items() ([]Item, error, bool) {
	entity := t.Showing()
	if entity == "" {
		if _, _, fetched := t.Offered(); !fetched &&
			t.opts.Controller.Access() == homeassistant.Allowed {
			t.Fetch()
		}
		return nil, nil, false
	}
	t.follow(entity)
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.items), t.problem, t.loaded
}

func (t *Todo) follow(entity string) {
	t.mu.Lock()
	if t.following == entity {
		t.mu.Unlock()
		return
	}
	t.following, t.items, t.loaded, t.problem = entity, nil, false, nil
	t.count = t.opts.States.Follow(entity, "")
	count := t.count
	t.mu.Unlock()
	count.Changed.Listen(func(string) { t.Refresh() })
	t.Refresh()
}

func (t *Todo) Refresh() {
	entity := t.Showing()
	if entity == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
		defer cancel()
		raw, err := t.opts.Controller.Ask(
			ctx,
			itemsAction,
			map[string]string{"entity_id": entity},
			nil,
			itemsShape,
		)
		var items []Item
		if err == nil {
			items, err = parse(raw, entity)
		}
		t.mu.Lock()
		if t.following == entity {
			if err == nil {
				t.items = items
			}
			t.problem, t.loaded = err, true
		}
		t.mu.Unlock()
		if err != nil {
			slog.Info("todo items", "entity", entity, "err", err)
		}
		t.opts.Redraw()
	}()
}

func parse(raw json.RawMessage, entity string) ([]Item, error) {
	var lists map[string]struct {
		Items []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &lists); err != nil {
		return nil, fmt.Errorf("todo: the items: %w", err)
	}
	return lists[entity].Items, nil
}

func (t *Todo) Add(summary string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	t.perform("todo.add_item", map[string]string{"item": summary})
}

func (t *Todo) Toggle(i Item) {
	status := statusDone
	if i.Done() {
		status = statusOpen
	}
	t.mark(i.UID, status)
	t.perform("todo.update_item", map[string]string{"item": i.UID, "status": status})
}

func (t *Todo) ClearDone() { t.perform("todo.remove_completed_items", nil) }

func (t *Todo) mark(uid, status string) {
	t.mu.Lock()
	for n := range t.items {
		if t.items[n].UID == uid {
			t.items[n].Status = status
		}
	}
	t.mu.Unlock()
	t.opts.Redraw()
}

func (t *Todo) perform(action string, data map[string]string) {
	entity := t.Showing()
	if entity == "" {
		return
	}
	fields := map[string]string{"entity_id": entity}
	maps.Copy(fields, data)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
		defer cancel()
		if err := t.opts.Controller.Call(ctx, action, fields); err != nil {
			slog.Warn("todo action failed", "action", action, "err", err)
		}
		t.Refresh()
	}()
}

func (t *Todo) Completed() bool { return t.opts.Settings.Read().Completed }

func (t *Todo) SetCompleted(on bool) {
	if err := schema.NewTodoWriter(t.opts.Settings).Completed(on); err != nil {
		slog.Error("saving the todo completed setting failed", "err", err)
	}
	t.knobs.Publish()
	t.opts.Redraw()
}

func (t *Todo) SetShown(entity string, on bool) {
	if err := schema.NewTodoWriter(t.opts.Settings).Show(entity, on); err != nil {
		slog.Error("saving the todo lists failed", "err", err)
	}
	t.knobs.Publish()
	t.opts.Redraw()
}
