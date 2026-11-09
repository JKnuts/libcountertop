package homecontrol

import (
	"slices"
	"testing"

	ha "github.com/ygelfand/libcountertop/pkg/homeassistant"
	config "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func none(string, string) (string, bool) { return "", false }

func names(rs []*Ring) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

func sample() []ha.Entity {
	return []ha.Entity{
		{
			ID:      "light.lamp",
			Name:    "Lamp",
			State:   "on",
			AreaID:  "bed",
			Area:    "Bedroom",
			FloorID: "up",
			Floor:   "Upstairs",
		},
		{
			ID:      "switch.fan",
			Name:    "Fan plug",
			State:   "off",
			AreaID:  "bed",
			Area:    "Bedroom",
			FloorID: "up",
			Floor:   "Upstairs",
		},
		{
			ID:      "light.porch",
			Name:    "Porch",
			State:   "off",
			AreaID:  "porch",
			Area:    "Porch",
			FloorID: "up",
			Floor:   "Upstairs",
		},
		{
			ID:         "binary_sensor.door",
			Name:       "Front door",
			State:      "on",
			AreaID:     "porch",
			Area:       "Porch",
			FloorID:    "up",
			Floor:      "Upstairs",
			Attributes: map[string]string{"device_class": "door"},
		},
		{
			ID: "binary_sensor.smoke", Name: "Smoke", State: "off", AreaID: "porch", Area: "Porch",
			Attributes: map[string]string{"device_class": "smoke"},
		},
		{ID: "sensor.temp", Name: "Temp", State: "21", AreaID: "bed", Area: "Bedroom"},
		{ID: "media_player.tv", Name: "TV", State: "playing"},
	}
}

func TestBuildRingsSkipsFloorsWithOneFloor(t *testing.T) {
	root := BuildRings(sample(), config.Rings{}, none)
	if got, want := names(
		root.Children,
	), []string{
		"Bedroom",
		"Porch",
		"No area",
	}; !slices.Equal(
		got,
		want,
	) {
		t.Fatalf("top ring %v, want %v", got, want)
	}
	if root.Devices != 5 || root.Active != 3 {
		t.Errorf("home holds %d devices with %d on, want 5 with 3", root.Devices, root.Active)
	}
	bed := root.Children[0]
	if got, want := names(bed.Children), []string{"Lights", "Switches"}; !slices.Equal(got, want) {
		t.Errorf("bedroom kinds %v, want %v", got, want)
	}
	if bed.Children[0].Level != LevelKind || bed.Children[0].Children[0].Level != LevelDevice {
		t.Errorf("bedroom levels %v / %v", bed.Children[0].Level, bed.Children[0].Children[0].Level)
	}
}

func TestSensorsSplitIntoAccessAndPresence(t *testing.T) {
	for _, tt := range []struct {
		class, want string
	}{
		{"door", "access"},
		{"garage_door", "access"},
		{"window", "access"},
		{"motion", "presence"},
		{"occupancy", "presence"},
		{"presence", "presence"},
		{"smoke", ""},
	} {
		got, _ := kindOf(
			ha.Entity{
				ID:         "binary_sensor.x",
				Attributes: map[string]string{"device_class": tt.class},
			},
		)
		if got != tt.want {
			t.Errorf("%s sensor is %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestBuildRingsAddsFloorsFromTwo(t *testing.T) {
	entities := append(sample(), ha.Entity{
		ID:      "light.attic",
		Name:    "Attic",
		State:   "off",
		AreaID:  "attic",
		Area:    "Attic",
		FloorID: "top",
		Floor:   "Attic",
	})
	root := BuildRings(entities, config.Rings{}, none)
	if got, want := names(
		root.Children,
	), []string{
		"Attic",
		"Upstairs",
		"No floor",
	}; !slices.Equal(
		got,
		want,
	) {
		t.Fatalf("floors %v, want %v", got, want)
	}
	if root.Children[0].Level != LevelFloor || root.Children[0].Children[0].Level != LevelArea {
		t.Errorf("floor levels %v / %v", root.Children[0].Level, root.Children[0].Children[0].Level)
	}
	hues := []int{root.Children[0].Hue, root.Children[1].Hue, root.Children[2].Hue}
	if want := []int{1, 2, 0}; !slices.Equal(hues, want) {
		t.Errorf("floor hues %v, want %v", hues, want)
	}
	hidden := BuildRings(entities, config.Rings{Areas: []string{"bed"}}, none)
	if got := hidden.Find("floor:up").Hue; got != 2 {
		t.Errorf("upstairs hue %d with the bedroom hidden, want 2", got)
	}
}

func TestBuildRingsBySystemPutsTypesFirstWithoutFloors(t *testing.T) {
	entities := append(sample(), ha.Entity{
		ID: "light.attic", Name: "Attic", State: "off", AreaID: "attic", Area: "Attic",
		FloorID: "top", Floor: "Attic",
	})
	root := BuildRings(entities, config.Rings{Group: config.BySystem}, none)
	if got, want := names(root.Children), []string{
		"Lights", "Switches", "Media", "Access",
	}; !slices.Equal(got, want) {
		t.Fatalf("types %v, want %v", got, want)
	}
	lights := root.Children[0]
	if lights.Level != LevelKind || lights.Hue != 0 {
		t.Errorf(
			"lights level %v hue %d, want a kind ring without a room hue",
			lights.Level,
			lights.Hue,
		)
	}
	if got, want := names(
		lights.Children,
	), []string{
		"Attic",
		"Bedroom",
		"Porch",
	}; !slices.Equal(
		got,
		want,
	) {
		t.Errorf("light rooms %v, want %v", got, want)
	}
	byRoom := BuildRings(entities, config.Rings{}, none)
	if got, want := root.Find(
		"kind:lights/area:bed",
	).Hue, byRoom.Find(
		"floor:up/area:bed",
	).Hue; got != want {
		t.Errorf("bedroom hue %d by system, %d by room", got, want)
	}
	if lights.Children[1].Children[0].Device == nil {
		t.Error("rooms by system hold devices directly")
	}
}

func TestBuildRingsHidesAreasKindsAndFollowsLabels(t *testing.T) {
	entities := sample()
	entities[0].Labels = []string{"kitchen"}
	root := BuildRings(
		entities,
		config.Rings{Areas: []string{"porch"}, Kinds: []string{"media"}},
		none,
	)
	if got, want := names(root.Children), []string{"Bedroom"}; !slices.Equal(got, want) {
		t.Errorf("shown %v, want %v", got, want)
	}
	root = BuildRings(entities, config.Rings{Areas: []string{NoArea}}, none)
	if got, want := names(root.Children), []string{"Bedroom", "Porch"}; !slices.Equal(got, want) {
		t.Errorf("with no area hidden %v, want %v", got, want)
	}
	root = BuildRings(entities, config.Rings{Labels: []string{"kitchen"}}, none)
	if leaves := root.Leaves(); len(leaves) != 1 || leaves[0].ID != "light.lamp" {
		t.Errorf("labelled leaves %v, want only the lamp", leaves)
	}
}

func TestBuildRingsReadsLiveValues(t *testing.T) {
	read := func(entity, attribute string) (string, bool) {
		switch entity + "#" + attribute {
		case "light.porch#":
			return "on", true
		case "light.porch#brightness":
			return "128", true
		}
		return "", false
	}
	porch := BuildRings(sample(), config.Rings{}, read).Find("device:light.porch").Device
	if !porch.Active || porch.Level != 50 {
		t.Errorf("porch active %v level %d, want on at 50", porch.Active, porch.Level)
	}
}
