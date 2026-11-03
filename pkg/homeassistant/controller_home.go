package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const homeShape = `{{ [state_attr('zone.home', 'latitude'), state_attr('zone.home', 'longitude')] | to_json }}`

var ErrNoHome = errors.New("homeassistant: the home zone has no location")

func (h *Controller) Home(ctx context.Context) (lat, lon float64, err error) {
	raw, err := h.Template(ctx, homeShape)
	if err != nil {
		return 0, 0, fmt.Errorf("homeassistant: the home zone: %w", err)
	}
	var at []*float64
	if err := json.Unmarshal(raw, &at); err != nil || len(at) != 2 || at[0] == nil || at[1] == nil {
		return 0, 0, ErrNoHome
	}
	return *at[0], *at[1], nil
}
