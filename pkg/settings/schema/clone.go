package schema

import (
	"maps"
	"slices"
)

func (w Wake) Clone() Wake {
	w.Words = slices.Clone(w.Words)
	return w
}

func (c Cast) Clone() Cast {
	c.YouTube.Skip = slices.Clone(c.YouTube.Skip)
	return c
}

func (c Camera) Clone() Camera {
	c.Settings = maps.Clone(c.Settings)
	return c
}

func (h Home) Clone() Home {
	h.Control = maps.Clone(h.Control)
	h.Group = maps.Clone(h.Group)
	h.Picks = maps.Clone(h.Picks)
	for key, p := range h.Picks {
		h.Picks[key] = p.Clone()
	}
	h.Rings = h.Rings.Clone()
	return h
}
