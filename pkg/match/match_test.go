package match

import "testing"

var devices = []Candidate{
	{Key: "bb", Name: "Kitchen Blueberry"},
	{Key: "at", Name: "Athena"},
	{Key: "cr", Name: "Crown"},
	{Key: "oe", Name: "Office Echo"},
	{Key: "os", Name: "Office Show"},
	{Key: "lr", Name: "Living Room"},
}

func TestSpokenNamesFindTheirDevice(t *testing.T) {
	for _, c := range []struct {
		said, want string
	}{
		{"blueberry", "bb"},
		{"blue berry", "bb"},
		{"Blue-Berry", "bb"},
		{"kitchen blueberry", "bb"},
		{"the athena", "at"},
		{"athena", "at"},
		{"athina", "at"},
		{"crown", "cr"},
		{"living room", "lr"},
		{"livingroom", "lr"},
		{"office echo", "oe"},
		{"at", "at"},
	} {
		got, ok := Best(c.said, devices)
		if !ok || got != c.want {
			t.Errorf("Best(%q) = %q, %v; want %q", c.said, got, ok, c.want)
		}
	}
}

func TestAnExactNameBeatsANearOne(t *testing.T) {
	near := []Candidate{{Key: "one", Name: "ESPN"}, {Key: "two", Name: "ESPN2"}}
	for said, want := range map[string]string{"espn": "one", "espn two": "two"} {
		if got, ok := Best(said, near); !ok || got != want {
			t.Errorf("Best(%q) = %q, %v; want %q", said, got, ok, want)
		}
	}
}

func TestAWholeNameBeatsAWordOfAnother(t *testing.T) {
	channels := []Candidate{{Key: "espn", Name: "ESPN"}, {Key: "dep", Name: "ESPN Deportes"}}
	if got, ok := Best("espn", channels); !ok || got != "espn" {
		t.Errorf("Best(espn) = %q, %v; want espn", got, ok)
	}
}

func TestClosestTakesTheTopOfANearTie(t *testing.T) {
	if got, ok := Closest("office", devices); !ok || (got != "oe" && got != "os") {
		t.Errorf("Closest(office) = %q, %v; want an office device", got, ok)
	}
	if got, ok := Closest("garage", devices); ok {
		t.Errorf("Closest(garage) = %q, want no match", got)
	}
}

func TestUnknownOrAmbiguousNamesFindNothing(t *testing.T) {
	for _, said := range []string{"mom", "garage", "office", "", "the"} {
		if got, ok := Best(said, devices); ok {
			t.Errorf("Best(%q) = %q, want no match", said, got)
		}
	}
}
