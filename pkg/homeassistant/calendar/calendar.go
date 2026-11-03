package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	fetchWait    = 10 * time.Second
	stale        = 5 * time.Minute
	eventsAction = "calendar.get_events"
	eventsShape  = "{{ response | to_json }}"
	dayLayout    = "2006-01-02"
	wireLayout   = "2006-01-02 15:04:05"
)

type Event struct {
	Calendar string
	Summary  string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
}

type Options struct {
	Controller *homeassistant.Controller
	States     *states.States
	Settings   storage.Section[schema.Calendar]
	Redraw     func()
	DeviceID   uint32
}

type Calendar struct {
	opts  Options
	knobs *setting.Controls[schema.Calendar]

	mu       sync.Mutex
	offered  []homeassistant.Entity
	failed   error
	fetching bool
	fetched  bool

	following string
	events    []Event
	loaded    bool
	problem   error
	asked     time.Time
	asking    bool
}

func New(opts Options) *Calendar {
	c := &Calendar{opts: opts}
	c.knobs = &setting.Controls[schema.Calendar]{
		Table:  NewTable(),
		Device: opts.DeviceID,
		Read:   opts.Settings.Read,
		Save:   c.store,
	}
	return c
}

func (c *Calendar) Offered() ([]homeassistant.Entity, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offered, c.failed, c.fetched
}

func (c *Calendar) Fetch() {
	c.mu.Lock()
	if c.fetching {
		c.mu.Unlock()
		return
	}
	c.fetching = true
	c.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
		defer cancel()
		list, err := c.opts.Controller.Entities(
			ctx,
			homeassistant.Filter{Domains: []string{"calendar"}},
		)
		c.mu.Lock()
		c.offered, c.failed, c.fetching, c.fetched = list, err, false, true
		c.mu.Unlock()
		if err != nil {
			slog.Info("calendars", "err", err)
		}
		c.opts.Redraw()
	}()
}

func (c *Calendar) Chosen() []string { return c.opts.Settings.Read().List() }

func (c *Calendar) Span() schema.CalendarSpan { return c.opts.Settings.Read().Span }

func (c *Calendar) Title(entity string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.offered {
		if e.ID == entity && e.Name != "" {
			return e.Name
		}
	}
	return entity
}

func (c *Calendar) Toggle(entity string) {
	list := c.Chosen()
	if i := slices.Index(list, entity); i >= 0 {
		list = slices.Delete(list, i, i+1)
	} else {
		list = append(list, entity)
	}
	if err := schema.NewCalendarWriter(c.opts.Settings).Entities(list); err != nil {
		slog.Error("saving the calendars failed", "err", err)
	}
	c.changed()
}

func (c *Calendar) SetSpan(v schema.CalendarSpan) {
	if err := schema.NewCalendarWriter(c.opts.Settings).Span(v); err != nil {
		slog.Error("saving the calendar span failed", "err", err)
	}
	c.changed()
}

func (c *Calendar) changed() {
	c.knobs.Publish()
	c.mu.Lock()
	c.asked = time.Time{}
	c.mu.Unlock()
	c.opts.Redraw()
}

func (c *Calendar) Events() ([]Event, error, bool) {
	chosen := c.Chosen()
	if len(chosen) == 0 {
		return nil, nil, false
	}
	key := strings.Join(chosen, ",") + "/" + string(c.Span())
	c.mu.Lock()
	if c.following != key {
		c.following, c.events, c.loaded, c.problem, c.asked = key, nil, false, nil, time.Time{}
		c.mu.Unlock()
		for _, e := range chosen {
			c.opts.States.Follow(e, "").Changed.Listen(func(string) { c.Refresh() })
		}
		c.mu.Lock()
	}
	due := !c.asking && time.Since(c.asked) > stale
	events, problem, loaded := slices.Clone(c.events), c.problem, c.loaded
	c.mu.Unlock()
	if due {
		c.Refresh()
	}
	return events, problem, loaded
}

func (c *Calendar) Refresh() {
	chosen := c.Chosen()
	if len(chosen) == 0 {
		return
	}
	c.mu.Lock()
	if c.asking {
		c.mu.Unlock()
		return
	}
	c.asking, c.asked = true, time.Now()
	key := c.following
	c.mu.Unlock()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, c.Span().Days())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
		defer cancel()
		raw, err := c.opts.Controller.Ask(ctx, eventsAction, map[string]string{
			"entity_id":       strings.Join(chosen, ","),
			"start_date_time": start.Format(wireLayout),
			"end_date_time":   end.Format(wireLayout),
		}, nil, eventsShape)
		var events []Event
		if err == nil {
			events, err = parse(raw, now.Location())
		}
		c.mu.Lock()
		c.asking = false
		if c.following == key {
			if err == nil {
				c.events = events
			}
			c.problem, c.loaded = err, true
		}
		c.mu.Unlock()
		if err != nil {
			slog.Info("calendar events", "err", err)
		}
		c.opts.Redraw()
	}()
}

type wire struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Summary     string `json:"summary"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

func parse(raw json.RawMessage, loc *time.Location) ([]Event, error) {
	var lists map[string]struct {
		Events []wire `json:"events"`
	}
	if err := json.Unmarshal(raw, &lists); err != nil {
		return nil, fmt.Errorf("calendar: the events: %w", err)
	}
	var out []Event
	for entity, list := range lists {
		for _, w := range list.Events {
			start, allDay, err := when(w.Start, loc)
			if err != nil {
				continue
			}
			end, _, err := when(w.End, loc)
			if err != nil {
				end = start
			}
			out = append(out, Event{
				Calendar: entity,
				Summary:  w.Summary,
				Location: w.Location,
				Start:    start,
				End:      end,
				AllDay:   allDay,
			})
		}
	}
	slices.SortFunc(out, func(a, b Event) int {
		if a.AllDay != b.AllDay && a.Start.Equal(b.Start) {
			if a.AllDay {
				return -1
			}
			return 1
		}
		return a.Start.Compare(b.Start)
	})
	return out, nil
}

func when(s string, loc *time.Location) (time.Time, bool, error) {
	if t, err := time.ParseInLocation(dayLayout, s, loc); err == nil {
		return t, true, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	return t.In(loc), false, err
}

func (c *Calendar) store(s Setting, v string) error {
	if err := setting.Store(c.opts.Settings, s, v); err != nil {
		return err
	}
	c.changed()
	return nil
}
