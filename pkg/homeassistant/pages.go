package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// An encrypted ESPHome frame carries at most 65,515 bytes.
const pageBudget = 40000

const pageTemplate = `{%- set ns = namespace(seen=0, parts=[], size=2, next=none) -%}
{%- for s in SOURCE if WHERE -%}
{%- if ns.seen >= START -%}
PREPARE
{%- set j = ITEM | to_json(ensure_ascii=True) -%}
{%- if j | length > BUDGET -%}
{%- set j = SMALL | to_json(ensure_ascii=True) -%}
{%- endif -%}
{%- if ns.parts and ns.size + (j | length) + 1 > BUDGET -%}
{%- set ns.next = ns.seen -%}
{%- break -%}
{%- endif -%}
{%- set ns.parts = ns.parts + [j] -%}
{%- set ns.size = ns.size + (j | length) + 1 -%}
{%- endif -%}
{%- set ns.seen = ns.seen + 1 -%}
{%- endfor -%}
{"next": {{ ns.next | to_json }}, "items": [{{ ns.parts | join(',') }}]}`

type listing struct {
	source  string
	where   string
	prepare string
	item    string
	small   string
}

func (l listing) page(start int) string {
	where := l.where
	if where == "" {
		where = "true"
	}
	return strings.NewReplacer(
		"SOURCE", l.source,
		"WHERE", where,
		"START", strconv.Itoa(start),
		"PREPARE", l.prepare,
		"ITEM", l.item,
		"SMALL", l.small,
		"BUDGET", strconv.Itoa(pageBudget),
	).Replace(pageTemplate)
}

type render func(ctx context.Context, tmpl string) (json.RawMessage, error)

func listAll[T any](ctx context.Context, ask render, l listing) ([]T, error) {
	out := []T{}
	for start := 0; ; {
		raw, err := ask(ctx, l.page(start))
		if err != nil {
			return nil, err
		}
		var page struct {
			Next  *int `json:"next"`
			Items []T  `json:"items"`
		}
		if err := json.Unmarshal(unwrap(raw), &page); err != nil {
			return nil, fmt.Errorf("homeassistant: page at %d: %w", start, err)
		}
		out = append(out, page.Items...)
		if page.Next == nil {
			return out, nil
		}
		if *page.Next <= start {
			return nil, fmt.Errorf("homeassistant: page at %d continues at %d", start, *page.Next)
		}
		start = *page.Next
	}
}
