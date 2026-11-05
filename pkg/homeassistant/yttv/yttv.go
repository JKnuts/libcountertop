package yttv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant/states"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

const (
	guideAction = "yttv.guide"
	castAction  = "yttv.cast"
	stopAction  = "yttv.stop"
	reply       = "{{ response | to_json }}"
	askTime     = 15 * time.Second
	stale       = time.Minute
	maxPages    = 64
	// ytimg serves *_live.jpg with max-age=300.
	liveEvery = 5 * time.Minute
)

type Airing struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

func (a Airing) Span() (time.Time, time.Time, bool) {
	start, err1 := time.Parse(time.RFC3339, a.StartsAt)
	end, err2 := time.Parse(time.RFC3339, a.EndsAt)
	if err1 != nil || err2 != nil || !end.After(start) {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

type Channel struct {
	Name      string   `json:"name"`
	Station   string   `json:"station"`
	Video     string   `json:"video"`
	Icon      string   `json:"icon"`
	Thumb     string   `json:"thumb"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle"`
	StartsAt  string   `json:"starts_at"`
	EndsAt    string   `json:"ends_at"`
	Live      bool     `json:"live"`
	Pinned    bool     `json:"pinned"`
	LiveThumb string   `json:"live_thumb"`
	Next      []Airing `json:"next"`
}

func (c Channel) Key() string {
	if c.Station != "" {
		return c.Station
	}
	return c.Name
}

func (c Channel) LiveFrame(now time.Time) string {
	if c.LiveThumb == "" {
		return ""
	}
	return c.LiveThumb + "?ts=" + strconv.FormatInt(now.Unix()/int64(liveEvery.Seconds()), 10)
}

func (c Channel) Airing() bool { return c.Title != "" }

func (c Channel) Current() Airing {
	return Airing{Title: c.Title, Subtitle: c.Subtitle, StartsAt: c.StartsAt, EndsAt: c.EndsAt}
}

func (c Channel) Airings() []Airing {
	if !c.Airing() {
		return c.Next
	}
	return append([]Airing{c.Current()}, c.Next...)
}

func (c Channel) At(now time.Time) (Airing, bool) {
	for _, a := range c.Airings() {
		if start, end, ok := a.Span(); ok && !start.After(now) && end.After(now) {
			return a, true
		}
	}
	return Airing{}, false
}

func (a Airing) Progress(now time.Time) (float64, bool) {
	start, end, ok := a.Span()
	if !ok {
		return 0, false
	}
	return min(max(now.Sub(start).Seconds()/end.Sub(start).Seconds(), 0), 1), true
}

type TV struct {
	Entity string `json:"entity_id"`
	Name   string `json:"name"`
	UUID   string `json:"uuid"`
	Host   string `json:"host"`
}

type Status struct {
	State    string
	Channel  string
	Title    string
	Subtitle string
	Picture  string
	Volume   float64
}

func (s Status) Playing() bool { return s.State == "playing" }

func (s Status) Active() bool {
	return s.State == "playing" || s.State == "paused" || s.State == "buffering"
}

type HA interface {
	Ask(
		ctx context.Context,
		action string,
		data, templates map[string]string,
		tmpl string,
	) (json.RawMessage, error)
	Perform(ctx context.Context, action string, data, templates map[string]string) error
}

type Follower interface {
	Follow(entity, attribute string) *states.Value
}

type Options struct {
	HA       HA
	States   Follower
	Settings storage.Section[schema.TV]
	Self     func() string
	Redraw   func()
	DeviceID uint32
}

type Guide struct {
	o     Options
	knobs *setting.Controls[schema.TV]

	mu       sync.Mutex
	channels []Channel
	tvs      []TV
	err      error
	loaded   bool
	asking   bool
	asked    time.Time
}

func New(o Options) *Guide {
	g := &Guide{o: o}
	g.knobs = &setting.Controls[schema.TV]{
		Table:  NewTable(),
		Device: o.DeviceID,
		Read:   func() schema.TV { return g.o.Settings.Read() },
		Save:   g.store,
	}
	return g
}

func (g *Guide) Settings() *setting.Controls[schema.TV] { return g.knobs }

func (g *Guide) Default() string { return g.o.Settings.Read().Default }

func (g *Guide) SetDefault(entity string) error {
	return g.o.Settings.Update(func(c *schema.TV) { c.Default = entity })
}

func (g *Guide) Favorites() []string { return g.o.Settings.Read().Favorites }

func (g *Guide) SetFavorite(channel string, on bool) error {
	return g.o.Settings.Update(func(c *schema.TV) {
		c.Favorites = slices.DeleteFunc(
			c.Favorites,
			func(have string) bool { return have == channel },
		)
		if on {
			c.Favorites = append(c.Favorites, channel)
		}
	})
}

func (g *Guide) store(s Setting, v string) error {
	if err := setting.Store(g.o.Settings, s, v); err != nil {
		return err
	}
	if g.o.Redraw != nil {
		g.o.Redraw()
	}
	return nil
}

func (g *Guide) Load() (channels []Channel, tvs []TV, err error, loaded bool) {
	g.mu.Lock()
	due := !g.asking && time.Since(g.asked) >= stale
	if due {
		g.asking, g.asked = true, time.Now()
	}
	channels, tvs, err, loaded = g.channels, g.tvs, g.err, g.loaded
	g.mu.Unlock()
	if due {
		safe.Go("yttv guide", g.fetch)
	}
	return channels, tvs, err, loaded
}

func (g *Guide) Refresh() {
	g.mu.Lock()
	g.asked = time.Time{}
	g.mu.Unlock()
}

func (g *Guide) fetch() {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	channels, tvs, err := g.pages(ctx)
	if err != nil {
		slog.Debug("yttv guide", "err", err)
	}
	g.mu.Lock()
	g.asking, g.loaded, g.err = false, true, err
	if err == nil {
		g.channels, g.tvs = channels, tvs
	}
	g.mu.Unlock()
	if g.o.Redraw != nil {
		g.o.Redraw()
	}
}

func (g *Guide) pages(ctx context.Context) ([]Channel, []TV, error) {
	var (
		channels []Channel
		tvs      []TV
	)
	for start, n := 0, 0; n < maxPages; n++ {
		raw, err := g.o.HA.Ask(ctx, guideAction,
			map[string]string{"start": strconv.Itoa(start)}, nil, reply)
		if err != nil {
			return nil, nil, err
		}
		page, err := parse(raw)
		if err != nil {
			return nil, nil, err
		}
		channels = append(channels, page.Channels...)
		if start == 0 {
			tvs = page.TVs
		}
		if page.Next == nil {
			return channels, tvs, nil
		}
		if *page.Next <= start {
			return nil, nil, fmt.Errorf("yttv: guide page at %d continues at %d", start, *page.Next)
		}
		start = *page.Next
	}
	return nil, nil, fmt.Errorf("yttv: guide has more than %d pages", maxPages)
}

type page struct {
	Next     *int      `json:"next"`
	Channels []Channel `json:"channels"`
	TVs      []TV      `json:"tvs"`
}

func parse(raw json.RawMessage) (page, error) {
	var p page
	if err := json.Unmarshal(raw, &p); err != nil {
		return page{}, fmt.Errorf("yttv: the guide: %w", err)
	}
	return p, nil
}

func (g *Guide) Mine(t TV) bool {
	if g.o.Self == nil {
		return false
	}
	self := g.o.Self()
	return self != "" && strings.ReplaceAll(strings.ToLower(t.UUID), "-", "") == self
}

func (g *Guide) Cast(ctx context.Context, tv string, c Channel) error {
	data := map[string]string{"device": tv, "channel": c.Name}
	if c.Video != "" {
		data = map[string]string{"device": tv, "video_id": c.Video}
	}
	return g.o.HA.Perform(ctx, castAction, data, nil)
}

func (g *Guide) Stop(ctx context.Context, tv string) error {
	return g.o.HA.Perform(ctx, stopAction, map[string]string{"device": tv}, nil)
}

func (g *Guide) Play(ctx context.Context, tv string) error {
	return g.o.HA.Perform(ctx, "media_player.media_play", map[string]string{"entity_id": tv}, nil)
}

func (g *Guide) Pause(ctx context.Context, tv string) error {
	return g.o.HA.Perform(ctx, "media_player.media_pause", map[string]string{"entity_id": tv}, nil)
}

func (g *Guide) SetVolume(ctx context.Context, tv string, level float64) error {
	return g.o.HA.Perform(ctx, "media_player.volume_set", map[string]string{
		"entity_id":    tv,
		"volume_level": strconv.FormatFloat(min(max(level, 0), 1), 'f', 2, 64),
	}, nil)
}

func (g *Guide) Status(tv string) Status {
	if g.o.States == nil || tv == "" {
		return Status{}
	}
	read := func(attribute string) string {
		v, _ := g.o.States.Follow(tv, attribute).Get()
		return v
	}
	volume, _ := strconv.ParseFloat(read("volume_level"), 64)
	return Status{
		State:    read(""),
		Channel:  read("source"),
		Title:    read("media_title"),
		Subtitle: read("media_artist"),
		Picture:  read("entity_picture"),
		Volume:   volume,
	}
}
