package entitycard

import (
	"slices"
	"testing"
)

func TestEntityReadsAttributes(t *testing.T) {
	e := Entity{ID: "light.desk", Attributes: map[string]any{
		"brightness":            float64(128),
		"supported_color_modes": []any{"hs", "color_temp"},
		"hs_color":              []any{float64(30), float64(50)},
		"friendly":              "Desk",
	}}
	if e.Domain() != "light" {
		t.Errorf("domain %q", e.Domain())
	}
	if b, ok := e.Number("brightness"); !ok || b != 128 {
		t.Errorf("brightness %v %v", b, ok)
	}
	if got := e.Texts("supported_color_modes"); !slices.Equal(got, []string{"hs", "color_temp"}) {
		t.Errorf("modes %v", got)
	}
	if got := e.Numbers("hs_color"); !slices.Equal(got, []float64{30, 50}) {
		t.Errorf("hs %v", got)
	}
	if !lightHue(e) || !lightWarmth(e) || !lightDims(e) {
		t.Errorf("hue %v warmth %v dims %v", lightHue(e), lightWarmth(e), lightDims(e))
	}
}

func TestLevelActions(t *testing.T) {
	for _, tt := range []struct {
		e           Entity
		value       float64
		action, key string
		want        string
	}{
		{Entity{ID: "light.a"}, 0.5, "light.turn_on", "brightness", "128"},
		{Entity{ID: "light.a"}, 0, "light.turn_on", "brightness", "3"},
		{Entity{ID: "fan.a"}, 0.33, "fan.set_percentage", "percentage", "33"},
		{Entity{ID: "cover.a"}, 0.25, "cover.set_cover_position", "position", "75"},
		{Entity{ID: "media_player.a"}, 0.456, "media_player.volume_set", "volume_level", "0.46"},
	} {
		action, data := levelAction(tt.e, tt.value)
		if action != tt.action || data[tt.key] != tt.want {
			t.Errorf(
				"%s at %v: %s %v, want %s %s=%s",
				tt.e.ID,
				tt.value,
				action,
				data,
				tt.action,
				tt.key,
				tt.want,
			)
		}
	}
}

func TestCoverKeysFollowFeaturesAndPosition(t *testing.T) {
	open := Entity{ID: "cover.a", State: "open", Attributes: map[string]any{
		"current_position":   float64(100),
		"supported_features": float64(coverOpen | coverClose),
	}}
	keys := coverKeys(open)
	if len(keys) != 2 || !keys[0].disabled || keys[1].disabled {
		t.Errorf("open cover keys %+v, want open disabled and close enabled, no stop", keys)
	}
	if keys := coverKeys(Entity{ID: "cover.b", State: "closed"}); len(keys) != 3 {
		t.Errorf("cover without features has %d keys, want 3", len(keys))
	}
}

func TestStepValueSnapsAndClamps(t *testing.T) {
	for _, tt := range []struct {
		current, step, lo, hi float64
		direction             int
		want                  float64
	}{
		{21, 0.5, 7, 35, 1, 21.5},
		{21.2, 0.5, 7, 35, -1, 20.5},
		{35, 1, 7, 35, 1, 35},
		{7, 1, 7, 35, -1, 7},
	} {
		if got := stepValue(tt.current, tt.step, tt.lo, tt.hi, tt.direction); got != tt.want {
			t.Errorf("step %v by %d: %v, want %v", tt.current, tt.direction, got, tt.want)
		}
	}
}
