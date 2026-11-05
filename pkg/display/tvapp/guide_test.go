package tvapp

import (
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
)

func at(clock string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-11-04T"+clock+":00Z")
	return t
}

func airing(title, from, to string) yttv.Airing {
	return yttv.Airing{
		Title:    title,
		StartsAt: at(from).Format(time.RFC3339),
		EndsAt:   at(to).Format(time.RFC3339),
	}
}

func TestTheWindowStartsOnTheHalfHour(t *testing.T) {
	if got := windowAt(at("15:47"), 150).from; !got.Equal(at("15:30")) {
		t.Errorf("window starts %v, want 15:30", got)
	}
}

func TestBlocksSpanTheirTimeAndLeaveGaps(t *testing.T) {
	g := windowAt(at("15:10"), 150)
	got := g.blocks([]yttv.Airing{
		airing("Late", "16:30", "17:00"),
		airing("Now", "14:30", "15:30"),
		airing("Next", "15:30", "16:00"),
		airing("Broken", "16:00", "15:00"),
		airing("Gone", "13:00", "14:00"),
	}, at("15:10"))
	want := []struct {
		title      string
		gap, width float32
		now        bool
	}{
		{"Now", 0, 30, true},
		{"Next", 0, 30, false},
		{"Late", 30, 30, false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d blocks, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		b := got[i]
		if b.airing.Title != w.title || b.gap != w.gap || b.width != w.width || b.now != w.now {
			t.Errorf("block %d = %s gap %v width %v now %v, want %+v",
				i, b.airing.Title, b.gap, b.width, b.now, w)
		}
	}
}

func TestABlockPastTheWindowIsCut(t *testing.T) {
	g := windowAt(at("15:00"), 150)
	got := g.blocks([]yttv.Airing{airing("Movie", "16:00", "19:00")}, at("15:00"))
	if len(got) != 1 || got[0].gap != 60 || got[0].width != 90 {
		t.Errorf("blocks %+v, want gap 60 width 90", got)
	}
}

func TestOverlappingAiringsDoNotOverlapOnScreen(t *testing.T) {
	g := windowAt(at("15:00"), 150)
	got := g.blocks([]yttv.Airing{
		airing("First", "15:00", "16:00"),
		airing("Overlap", "15:30", "16:30"),
	}, at("15:00"))
	if len(got) != 2 || got[1].gap != 0 || got[1].width != 30 {
		t.Errorf("blocks %+v, want the second to start where the first ends", got)
	}
}
