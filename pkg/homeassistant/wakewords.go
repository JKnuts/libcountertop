package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Home Assistant's option for an empty wake word slot.
const NoWakeWord = "no_wake_word"

// Home Assistant's option for the preferred Assist pipeline.
const PreferredPipeline = "preferred"

type WakeWords struct {
	Controller *Controller
	MAC        func() string
}

func (w WakeWords) Allowed() bool { return w.Controller.Access() == Allowed }

func (w WakeWords) Choose(ctx context.Context, slot int, phrase string) error {
	if phrase == "" {
		phrase = NoWakeWord
	}
	return choose(ctx, w.Controller, slotSelect(w.MAC(), "_wake_word", slot), phrase)
}

type Pipelines struct {
	Controller *Controller
	MAC        func() string
}

func (p Pipelines) Allowed() bool { return p.Controller.Access() == Allowed }

func (p Pipelines) Read(
	ctx context.Context,
	slot int,
) (chosen string, options []string, err error) {
	raw, err := p.Controller.Template(ctx, findSelect(p.MAC(), "_assistant", slot)+
		`{{ {'state': states(ns.e), 'options': state_attr(ns.e, 'options') or []} | to_json }}`)
	if err != nil {
		return "", nil, err
	}
	var got struct {
		State   string   `json:"state"`
		Options []string `json:"options"`
	}
	if err := json.Unmarshal(unwrap(raw), &got); err != nil {
		return "", nil, fmt.Errorf("reading the pipeline select: %w", err)
	}
	if len(got.Options) == 0 {
		return "", nil, fmt.Errorf("no pipeline select for assistant %d", slot+1)
	}
	return got.State, got.Options, nil
}

func (p Pipelines) Choose(ctx context.Context, slot int, option string) error {
	return choose(ctx, p.Controller, slotSelect(p.MAC(), "_assistant", slot), option)
}

func choose(ctx context.Context, c *Controller, entity, option string) error {
	return c.Perform(
		ctx,
		"select.select_option",
		map[string]string{"option": option},
		map[string]string{"entity_id": entity},
	)
}

func slotSelect(mac, base string, slot int) string {
	return findSelect(mac, base, slot) + `{{ ns.e }}`
}

func findSelect(mac, base string, slot int) string {
	suffix := base
	if slot > 0 {
		suffix += "_" + strconv.Itoa(slot+1)
	}
	return `{%- set ns = namespace(e='') -%}` +
		`{%- for e in integration_entities('esphome') if e is match('select[.].*` + suffix + `$') and '` +
		strings.ToLower(mac) + `' in (device_attr(e, 'connections') | string) -%}` +
		`{%- set ns.e = e -%}{%- endfor -%}`
}
