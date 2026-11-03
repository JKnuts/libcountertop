package playerview

import (
	"testing"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

type stopped struct {
	quiet
	stops int
}

func (s *stopped) Stop() { s.stops++ }

type stoppable struct {
	playing
	transport *stopped
}

func (s stoppable) Transport() nowplaying.Controller { return s.transport }

func glyphs(controls []miniControl) []string {
	out := make([]string, len(controls))
	for i, c := range controls {
		out[i] = c.glyph
	}
	return out
}

func TestMiniShowsStopOnlyWhenTheSourceCanStop(t *testing.T) {
	transport := &stopped{}
	p := &Renderer{options: Dependencies{Player: stoppable{transport: transport}}}

	now := nowplaying.Now{Playing: true, Can: nowplaying.CanNext}
	if got := glyphs(p.miniControls(now)); len(got) != 3 || got[2] != gogui.IconExpand {
		t.Fatalf("without CanStop: %q", got)
	}

	now.Can |= nowplaying.CanStop
	controls := p.miniControls(now)
	want := []string{gogui.IconPause, gogui.IconFastForward, gogui.IconStop, gogui.IconExpand}
	got := glyphs(controls)
	if len(got) != len(want) {
		t.Fatalf("with CanStop: %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("with CanStop: %q, want %q", got, want)
		}
	}
	controls[2].do()
	if transport.stops != 1 {
		t.Fatalf("stop pressed %d times, want 1", transport.stops)
	}
}
