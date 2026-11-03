package radio

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

const (
	Local        = "media-source://radio_browser/local"
	Popular      = "media-source://radio_browser/popular"
	browseAction = "media_player.browse_media"
	playAction   = "media_player.play_media"
	fetchWait    = 15 * time.Second
	stale        = 10 * time.Minute
)

const browseShape = `{%- set r = response.values() | first -%}` +
	`{%- set ns = namespace(i=[]) -%}` +
	`{%- for c in (r.children or [])[:300] -%}` +
	`{%- set ns.i = ns.i + [[c.title, c.media_content_id, c.can_play, c.can_expand, c.thumbnail or '']] -%}` +
	`{%- endfor -%}` +
	`{{ {'t': r.title, 'n': (r.children or []) | length, 'i': ns.i} | to_json }}`

type Item struct {
	Title      string
	ID         string
	Thumb      string
	Playable   bool
	Expandable bool
}

type Page struct {
	Title string
	Items []Item
	Total int
}

type Options struct {
	Controller *homeassistant.Controller
	MAC        func() string
	Settings   storage.Section[schema.Radio]
	Redraw     func()
}

type Radio struct {
	opts   Options
	writer schema.RadioWriter

	mu    sync.Mutex
	pages map[string]*entry
}

type entry struct {
	page   Page
	err    error
	loaded bool
	asking bool
	asked  time.Time
}

func New(opts Options) *Radio {
	return &Radio{
		opts:   opts,
		writer: schema.NewRadioWriter(opts.Settings),
		pages:  map[string]*entry{},
	}
}

func (r *Radio) Settings() schema.Radio { return r.opts.Settings.Read() }

func (r *Radio) Browse(id string) (Page, error, bool) {
	r.mu.Lock()
	e := r.pages[id]
	if e == nil {
		e = &entry{}
		r.pages[id] = e
	}
	page, err, loaded := e.page, e.err, e.loaded
	due := !e.asking && (!e.loaded || e.err != nil && time.Since(e.asked) > fetchWait ||
		time.Since(e.asked) > stale)
	if due {
		e.asking, e.asked = true, time.Now()
	}
	r.mu.Unlock()
	if due {
		go r.fetch(id)
	}
	return page, err, loaded
}

func (r *Radio) Refresh(id string) {
	r.mu.Lock()
	delete(r.pages, id)
	r.mu.Unlock()
	r.redraw()
}

func (r *Radio) Play(s schema.Station) error {
	ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
	defer cancel()
	err := r.opts.Controller.Perform(ctx, playAction, map[string]string{
		"media_content_id":   s.ID,
		"media_content_type": "music",
	}, map[string]string{"entity_id": r.self()})
	if err != nil {
		return fmt.Errorf("radio: playing %s: %w", s.Title, err)
	}
	if err := r.writer.Last(s); err != nil {
		slog.Warn("saving the last station", "err", err)
	}
	return nil
}

func (r *Radio) Favorite(s schema.Station) error { return r.writer.Toggle(s) }

func (r *Radio) fetch(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchWait)
	defer cancel()
	raw, err := r.opts.Controller.Ask(ctx, browseAction, map[string]string{
		"media_content_id":   id,
		"media_content_type": "music",
	}, map[string]string{"entity_id": r.self()}, browseShape)
	var page Page
	if err == nil {
		page, err = parse(raw)
	}
	if err != nil {
		slog.Warn("browsing radio stations", "id", id, "err", err)
	}
	r.mu.Lock()
	if e := r.pages[id]; e != nil {
		e.asking, e.loaded = false, true
		e.err = err
		if err == nil {
			e.page = page
		}
	}
	r.mu.Unlock()
	r.redraw()
}

func (r *Radio) self() string {
	mac := strings.ToLower(r.opts.MAC())
	return `{%- set ns = namespace(e='') -%}` +
		`{%- for e in integration_entities('esphome') if e is match('media_player[.]') and '` +
		mac + `' in (device_attr(e, 'connections') | string) -%}` +
		`{%- set ns.e = e -%}{%- endfor -%}{{ ns.e }}`
}

func (r *Radio) redraw() {
	if r.opts.Redraw != nil {
		r.opts.Redraw()
	}
}

func parse(raw json.RawMessage) (Page, error) {
	var w struct {
		T string              `json:"t"`
		N int                 `json:"n"`
		I [][]json.RawMessage `json:"i"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		slog.Debug("radio stations reply", "raw", string(raw[:min(len(raw), 400)]))
		return Page{}, fmt.Errorf("radio: the stations: %w", err)
	}
	page := Page{Title: w.T, Total: w.N}
	for _, row := range w.I {
		if len(row) < 5 {
			continue
		}
		var it Item
		_ = json.Unmarshal(row[0], &it.Title)
		_ = json.Unmarshal(row[1], &it.ID)
		_ = json.Unmarshal(row[2], &it.Playable)
		_ = json.Unmarshal(row[3], &it.Expandable)
		_ = json.Unmarshal(row[4], &it.Thumb)
		if it.ID != "" {
			page.Items = append(page.Items, it)
		}
	}
	return page, nil
}
