package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type item struct {
	ID string `json:"id"`
}

func pages(t *testing.T, l listing, answers map[int]string) render {
	t.Helper()
	return func(_ context.Context, tmpl string) (json.RawMessage, error) {
		for start, answer := range answers {
			if tmpl == l.page(start) {
				return json.RawMessage(answer), nil
			}
		}
		t.Fatalf("asked for a page nobody expected:\n%s", tmpl)
		return nil, nil
	}
}

func TestPagesJoinUntilThereIsNoNext(t *testing.T) {
	l := listing{source: "states", item: "{'id': s.entity_id}", small: "{}"}
	got, err := listAll[item](context.Background(), pages(t, l, map[int]string{
		0: `{"next": 2, "items": [{"id":"a"},{"id":"b"}]}`,
		2: `{"next": 3, "items": [{"id":"c"}]}`,
		3: `{"next": null, "items": [{"id":"d"}]}`,
	}), l)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, "") != "abcd" {
		t.Errorf("joined %v, want a b c d", ids)
	}
}

func TestAPageThatDoesNotMoveOnIsRefused(t *testing.T) {
	l := listing{source: "states", item: "{}", small: "{}"}
	_, err := listAll[item](context.Background(), pages(t, l, map[int]string{
		0: `{"next": 2, "items": [{"id":"a"}]}`,
		2: `{"next": 2, "items": [{"id":"b"}]}`,
	}), l)
	if err == nil {
		t.Fatal("a page pointing back at itself was accepted")
	}
}

func TestAFailedPageFailsTheList(t *testing.T) {
	want := errors.New("not connected")
	l := listing{source: "states", item: "{}", small: "{}"}
	_, err := listAll[item](
		context.Background(),
		func(context.Context, string) (json.RawMessage, error) { return nil, want },
		l,
	)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestPageTemplateCarriesItsPosition(t *testing.T) {
	tmpl := listing{source: "states", where: "s.state != 'off'", item: "{}", small: "{}"}.page(312)
	for _, want := range []string{
		"for s in states if s.state != 'off'",
		"ns.seen >= 312",
		"> 40000",
		"to_json(ensure_ascii=True)",
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("page template is missing %q:\n%s", want, tmpl)
		}
	}
}
