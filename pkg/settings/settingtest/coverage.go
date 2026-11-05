package settingtest

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const schemaPkg = "github.com/ygelfand/libcountertop/pkg/settings/schema"

type Coverage struct {
	Rows   map[string]string
	Many   map[string][]string
	ByHand map[string]string
}

func (c Coverage) With(more Coverage) Coverage {
	out := Coverage{
		Rows:   map[string]string{},
		Many:   map[string][]string{},
		ByHand: map[string]string{},
	}
	for _, from := range []Coverage{c, more} {
		maps.Copy(out.Rows, from.Rows)
		maps.Copy(out.Many, from.Many)
		maps.Copy(out.ByHand, from.ByHand)
	}
	return out
}

func Fields(root reflect.Type) []string { return leaves(root, root.PkgPath(), "") }

func leaves(t reflect.Type, home, at string) []string {
	var out []string
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous {
			continue
		}
		name := at + f.Name
		if f.Type.Kind() == reflect.Struct &&
			(f.Type.PkgPath() == home || f.Type.PkgPath() == schemaPkg) {
			out = append(out, leaves(f.Type, home, name+".")...)
			continue
		}
		out = append(out, name)
	}
	return out
}

func (c Coverage) Accounts(t *testing.T, root reflect.Type) {
	t.Helper()
	all := Fields(root)
	var missing []string
	for _, field := range all {
		_, row := c.Rows[field]
		_, many := c.Many[field]
		_, hand := c.ByHand[field]
		if !row && !many && !hand {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		t.Errorf("nothing reaches these settings, and ByHand does not say why:\n\t%s",
			strings.Join(missing, "\n\t"))
	}
	for _, kind := range []struct {
		name string
		keys []string
	}{
		{"Rows", slices.Collect(maps.Keys(c.Rows))},
		{"Many", slices.Collect(maps.Keys(c.Many))},
		{"ByHand", slices.Collect(maps.Keys(c.ByHand))},
	} {
		for _, field := range kind.keys {
			if !slices.Contains(all, field) {
				t.Errorf("%s lists %s, which is not in the config", kind.name, field)
			}
		}
	}
}

func (c Coverage) Names(t *testing.T, registered []string) {
	t.Helper()
	rows := slices.Collect(maps.Values(c.Rows))
	var prefixes []string
	for _, ps := range c.Many {
		prefixes = append(prefixes, ps...)
	}
	for _, name := range registered {
		if slices.Contains(rows, name) ||
			slices.ContainsFunc(
				prefixes,
				func(p string) bool { return strings.HasPrefix(name, p) },
			) {
			continue
		}
		t.Errorf("%s does not say which setting it changes", name)
	}
}
