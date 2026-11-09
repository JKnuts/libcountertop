package ringsview

import (
	"math"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/display/theme"
	ha "github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func tree() *homecontrol.Ring {
	return homecontrol.BuildRings([]ha.Entity{
		{ID: "light.a", Name: "A", State: "on", AreaID: "bed", Area: "Bed"},
		{ID: "light.b", Name: "B", State: "off", AreaID: "bed", Area: "Bed"},
		{ID: "switch.c", Name: "C", State: "off", AreaID: "bed", Area: "Bed"},
		{ID: "light.d", Name: "D", State: "on", AreaID: "porch", Area: "Porch"},
	}, schema.Rings{}, func(string, string) (string, bool) { return "", false })
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func TestFlattenSplitsByDevices(t *testing.T) {
	segs := flatten(tree())
	bed := segs[find(segs, "home/area:bed")]
	if !near(bed.a0, 0) || !near(bed.a1, 0.75) || bed.depth != 1 {
		t.Errorf("bed spans %v..%v at depth %d, want 0..0.75 at 1", bed.a0, bed.a1, bed.depth)
	}
	c := segs[find(segs, "device:switch.c")]
	if !near(c.a0, 0.5) || !near(c.a1, 0.75) || c.depth != 3 {
		t.Errorf("switch spans %v..%v at depth %d, want 0.5..0.75 at 3", c.a0, c.a1, c.depth)
	}
}

func TestFrameZoomsIntoFocus(t *testing.T) {
	segs := flatten(tree())
	home := frameFor(segs, 0, schema.RingsDeepest)
	if len(home.bands) != 4 || !near(home.bands[3], 1) {
		t.Fatalf("home bands %v, want three rings ending at 1", home.bands)
	}
	if home.bands[3]-home.bands[2] <= home.bands[1]-home.bands[0] {
		t.Errorf(
			"device ring %v is not wider than the area ring %v",
			home.bands[3]-home.bands[2],
			home.bands[1],
		)
	}
	bed := frameFor(segs, find(segs, "home/area:bed"), schema.RingsDeepest)
	porch := bed.place(segs[find(segs, "home/area:porch")])
	if porch.shown() {
		t.Errorf("porch still shows when zoomed into bed: %+v", porch)
	}
	a := bed.place(segs[find(segs, "device:light.a")])
	if !near(a.a0, 0) || !near(a.a1, 1.0/3) || !near(a.r1, 1) {
		t.Errorf("lamp A in bed frame %+v, want first third of the outer ring", a)
	}
	if got := bed.place(segs[find(segs, "home/area:bed")]); got.shown() {
		t.Errorf("focus itself shows as a ring: %+v", got)
	}
}

func TestDepthLimitsTheRings(t *testing.T) {
	segs := flatten(tree())
	shallow := frameFor(segs, 0, 1)
	if len(shallow.bands) != 2 {
		t.Fatalf("depth 1 bands %v, want one ring", shallow.bands)
	}
	if got := shallow.place(segs[find(segs, "device:light.d")]); got.shown() {
		t.Errorf("device shows at depth 1: %+v", got)
	}
	bed := frameFor(segs, find(segs, "home/area:bed"), 1)
	if got := bed.place(segs[find(segs, "home/area:bed/kind:lights")]); !got.shown() {
		t.Errorf("zooming into bed at depth 1 hides its kinds: %+v", got)
	}
}

func TestHitFindsTheTappedRing(t *testing.T) {
	segs := flatten(tree())
	f := frameFor(segs, 0, schema.RingsDeepest)
	i, ok := hit(segs, f, 0.9, 0.95)
	if !ok || segs[i].ring.Key != "device:light.d" {
		t.Errorf("outer ring at 0.9 hit %v %v, want light.d", ok, segs[i].ring.Key)
	}
	i, ok = hit(segs, f, 0.1, 0.1)
	if !ok || segs[i].ring.Key != "home/area:bed" {
		t.Errorf("inner ring at 0.1 hit %v %v, want bed", ok, segs[i].ring.Key)
	}
}

func TestRoomColorFollowsTheAreaOnly(t *testing.T) {
	pal := theme.Theme{Roles: theme.Roles{
		Warm: theme.Color{R: 200}, Heat: theme.Color{G: 200}, Cool: theme.Color{B: 200},
		Air: theme.Color{R: 90}, Motion: theme.Color{G: 90}, Media: theme.Color{B: 90},
		Choice: theme.Color{R: 40}, Level: theme.Color{G: 40},
	}}
	entities := []ha.Entity{
		{ID: "light.a", Name: "A", State: "off", AreaID: "bed", Area: "Bed"},
		{ID: "light.d", Name: "D", State: "off", AreaID: "porch", Area: "Porch"},
	}
	off := func(string, string) (string, bool) { return "", false }
	full := homecontrol.BuildRings(entities, schema.Rings{}, off)
	hidden := homecontrol.BuildRings(entities, schema.Rings{Areas: []string{"bed"}}, off)
	porch := toneOf(pal, full.Find("home/area:porch")).fill
	if got := toneOf(pal, hidden.Find("home/area:porch")).fill; got != porch {
		t.Errorf("porch is %v with bed shown and %v with bed hidden", porch, got)
	}
	if bed := toneOf(pal, full.Find("home/area:bed")).fill; bed == porch {
		t.Errorf("neighbors bed and porch share %v", bed)
	}
	if got := full.Find("device:light.d").Hue; got != full.Find("home/area:porch").Hue {
		t.Errorf("porch's device hue %d, want the room's", got)
	}
}

func TestLooksLocateWhatTheyPlace(t *testing.T) {
	for _, look := range schema.RingsLooks() {
		g := lay(look, 1280, 800)
		for _, p := range [][2]float32{{0.1, 0.2}, {0.37, 0.5}, {0.62, 0.9}, {0.88, 0.05}} {
			x, y := g.point(p[0], p[1])
			a, r := g.locate(x, y)
			if !near(a, p[0]) || math.Abs(float64(r-p[1])) > 1e-3 {
				t.Errorf("%s: (%v,%v) -> (%v,%v) -> (%v,%v)", look, p[0], p[1], x, y, a, r)
			}
		}
		bx := g.label()
		if _, r := g.locate(bx.x+bx.w/2, bx.y+bx.h/2); r >= 0 {
			t.Errorf("%s: the hub's middle reads as ring %v", look, r)
		}
	}
}
