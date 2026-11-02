package lovelaceview

import (
	"testing"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
)

func TestANewHomeAssistantVersionReloadsTheDashboard(t *testing.T) {
	var f Feature
	steps := []struct {
		version string
		reload  bool
	}{
		{"2026.10.3", false},
		{"2026.10.3", false},
		{"2026.10.4", true},
		{"2026.10.4", false},
	}
	for _, s := range steps {
		if got := f.listed(homeassistant.Dashboards{Version: s.version}); got != s.reload {
			t.Errorf("list from %s: reload %v, want %v", s.version, got, s.reload)
		}
	}
	if f.list.Version != "2026.10.4" {
		t.Errorf("kept version %q", f.list.Version)
	}
}
