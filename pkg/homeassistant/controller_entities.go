package homeassistant

import (
	"context"
)

func (h *Controller) Entities(ctx context.Context, f Filter) ([]Entity, error) {
	out, err := listAll[Entity](ctx, h.Template, f.listing())
	if err != nil {
		return nil, err
	}
	sortEntities(out)
	return out, nil
}
