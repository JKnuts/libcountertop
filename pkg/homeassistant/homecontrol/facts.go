package homecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

type Facts struct {
	ID         string
	Name       string
	State      string
	Attributes map[string]any
}

type fact struct {
	facts  Facts
	mark   string
	known  bool
	asking bool
}

type factBook struct {
	mu  sync.Mutex
	got map[string]*fact
}

var follows = map[string][]string{
	"light":        {"brightness", "color_temp_kelvin", "hs_color"},
	"fan":          {"percentage"},
	"cover":        {"current_position", "current_tilt_position"},
	"media_player": {"volume_level", "is_volume_muted", "media_title", "source"},
	"climate": {
		"temperature",
		"target_temp_low",
		"target_temp_high",
		"fan_mode",
		"swing_mode",
	},
	"water_heater": {"temperature"},
}

const factsTemplate = `{%%- set s = expand(%s) | first -%%}
{%%- if s -%%}{{ {'state': s.state, 'name': s.name, 'attributes': s.attributes} | to_json }}{%%- else -%%}{}{%%- endif -%%}`

func (e *Engine) Facts(id string) (Facts, bool) {
	mark := e.factMark(id)
	b := &e.facts
	b.mu.Lock()
	f := b.got[id]
	if f == nil {
		f = &fact{facts: Facts{ID: id}}
		b.got[id] = f
	}
	stale := f.mark != mark && !f.asking
	if stale {
		f.asking = true
	}
	out, known := f.facts, f.known
	b.mu.Unlock()
	if stale {
		e.board.mu.Lock()
		closed := e.board.closed
		if !closed {
			e.board.workers.Add(1)
		}
		e.board.mu.Unlock()
		if !closed {
			go func() {
				defer e.board.workers.Done()
				e.fetchFacts(id, mark)
			}()
		}
	}
	return out, known
}

func (e *Engine) factMark(id string) string {
	parts := []string{e.board.read(id, "")}
	domain, _, _ := strings.Cut(id, ".")
	for _, attr := range follows[domain] {
		parts = append(parts, e.board.read(id, attr))
	}
	return strings.Join(parts, "\x00")
}

func (e *Engine) fetchFacts(id, mark string) {
	ctx, cancel := context.WithTimeout(e.ctx, fetchWait)
	defer cancel()
	quoted, _ := json.Marshal(id)
	raw, err := e.options.Client.Template(ctx, fmt.Sprintf(factsTemplate, quoted))
	var got struct {
		State      string         `json:"state"`
		Name       string         `json:"name"`
		Attributes map[string]any `json:"attributes"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &got)
	}
	b := &e.facts
	b.mu.Lock()
	f := b.got[id]
	f.asking, f.mark = false, mark
	if err != nil {
		slog.Warn("Home Assistant did not describe the entity", "entity", id, "err", err)
	} else {
		f.facts = Facts{ID: id, Name: got.Name, State: got.State, Attributes: got.Attributes}
		f.known = true
	}
	b.mu.Unlock()
	e.options.Shell.Redraw()
}

func (e *Engine) Perform(action string, data, templates map[string]string) {
	e.board.perform(action, data, templates)
}
