package screen

import (
	"strings"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func table() *Table { return NewTable(func() string { return schema.UISizeLarge }) }

func TestTheEntityIdentifiersDoNotMove(t *testing.T) {
	want := map[string]string{
		"backlight":   "backlight",
		"auto":        "screen_mode",
		"theme":       "theme",
		"style":       "screen_style",
		"size":        "screen_size",
		"drawer":      "drawer_edge",
		"drawer.hide": "dock_hide",
		"alert":       "screen_alert",
		"timer":       "screen_timer",
		"keyboard":    "screen_keyboard",
		"language":    "screen_language",
		"marks":       "privacy_marks",
		"touches":     "show_touches",
	}
	for _, field := range theme.CustomFields {
		want["custom."+field] = "theme_custom_" + field
	}

	tb := table()
	rows := tb.Rows()
	if len(rows) != len(want) {
		t.Fatalf("%d rows against %d identifiers held here", len(rows), len(want))
	}

	for _, s := range rows {
		at, held := want[s.Name]
		if !held {
			t.Errorf("%s is new, so decide what Home Assistant calls it and hold it here", s.Name)
			continue
		}
		if got := s.Object(tb.Domain); got != at {
			t.Errorf("%s is %q, want %q", s.Name, got, at)
		}
	}
}

func TestEveryRowIsNamed(t *testing.T) {
	for _, s := range table().Rows() {
		if got := s.Named(); got == "" || strings.HasPrefix(got, "screen.") {
			t.Errorf("%s has no text: %q", s.Name, got)
		}
		for _, o := range s.Options {
			if got := s.Label(o); got == "" || strings.HasPrefix(got, "screen.") {
				t.Errorf("%s option %q has no text: %q", s.Name, o.Value, got)
			}
		}
	}
}

func TestAnUnsetSizeReadsAsTheBoardsSize(t *testing.T) {
	tb := NewTable(func() string { return schema.UISizeMini })
	c := schema.DefaultScreen()
	if got := tb.MustRow("size").Read(&c); got != schema.UISizeMini {
		t.Errorf("an unset size reads %q, want %q", got, schema.UISizeMini)
	}
}

func TestARowWritesOnlyItsOwnField(t *testing.T) {
	tb := table()
	for _, s := range tb.Rows() {
		was := schema.DefaultScreen()
		moved := was

		other, ok := elsewhere(s.Name)
		if !ok {
			continue
		}
		if err := s.Write(&moved, other); err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}

		for _, at := range tb.Rows() {
			if at.Name == s.Name {
				continue
			}
			if before, after := at.Read(&was), at.Read(&moved); before != after {
				t.Errorf("writing %s moved %s from %q to %q", s.Name, at.Name, before, after)
			}
		}
	}
}

func elsewhere(name string) (string, bool) {
	switch name {
	case "backlight":
		return "13", true
	case "auto":
		return "off", true
	case "theme":
		return "Paper", true
	case "drawer":
		return string(schema.EdgeTop), true
	}
	return "", false
}
