package ringsview

import (
	"github.com/ygelfand/libcountertop/pkg/homeassistant/homecontrol"
)

const deviceWidth = 2.2

type seg struct {
	ring   *homecontrol.Ring
	parent int
	depth  int
	a0, a1 float32
}

type shape struct{ a0, a1, r0, r1 float32 }

func (s shape) shown() bool { return s.r1-s.r0 > 0.001 && s.a1-s.a0 > 0.0005 }

func (s shape) holds(a, r float32) bool { return a >= s.a0 && a < s.a1 && r >= s.r0 && r < s.r1 }

func flatten(root *homecontrol.Ring) []seg {
	if root == nil {
		return nil
	}
	out := []seg{{ring: root, parent: -1, a1: 1}}
	var walk func(at int)
	walk = func(at int) {
		s := out[at]
		total := float32(s.ring.Devices)
		if total == 0 {
			return
		}
		from := s.a0
		for _, c := range s.ring.Children {
			span := (s.a1 - s.a0) * float32(c.Devices) / total
			out = append(
				out,
				seg{ring: c, parent: at, depth: s.depth + 1, a0: from, a1: from + span},
			)
			from += span
			walk(len(out) - 1)
		}
	}
	walk(0)
	return out
}

func find(segs []seg, key string) int {
	for i, s := range segs {
		if s.ring.Key == key {
			return i
		}
	}
	return 0
}

type frame struct {
	focus seg
	bands []float32
}

func frameFor(segs []seg, focus, depth int) frame {
	if len(segs) == 0 {
		return frame{bands: []float32{0, 1}}
	}
	f := segs[focus]
	widths := []float32{}
	for k := 1; k <= depth; k++ {
		level, ok := levelAt(segs, focus, f.depth+k)
		if !ok {
			break
		}
		w := float32(1)
		if level == homecontrol.LevelDevice {
			w = deviceWidth
		}
		widths = append(widths, w)
	}
	var total float32
	for _, w := range widths {
		total += w
	}
	bands := []float32{0}
	var at float32
	for _, w := range widths {
		at += w
		bands = append(bands, at/max(total, 1e-6))
	}
	return frame{focus: f, bands: bands}
}

func levelAt(segs []seg, focus, depth int) (homecontrol.Level, bool) {
	for i := focus + 1; i < len(segs); i++ {
		s := segs[i]
		if s.depth <= segs[focus].depth {
			break
		}
		if s.depth == depth {
			return s.ring.Level, true
		}
	}
	return 0, false
}

func (f frame) place(s seg) shape {
	w := f.focus.a1 - f.focus.a0
	if w <= 0 {
		return shape{}
	}
	sh := shape{
		a0: clamp01((s.a0 - f.focus.a0) / w),
		a1: clamp01((s.a1 - f.focus.a0) / w),
	}
	rel := s.depth - f.focus.depth
	rings := len(f.bands) - 1
	switch {
	case rel <= 0:
	case rel > rings:
		sh.r0, sh.r1 = 1, 1
	default:
		sh.r0, sh.r1 = f.bands[rel-1], f.bands[rel]
	}
	return sh
}

func blend(a, b shape, t float32) shape {
	return shape{
		a0: lerp(a.a0, b.a0, t),
		a1: lerp(a.a1, b.a1, t),
		r0: lerp(a.r0, b.r0, t),
		r1: lerp(a.r1, b.r1, t),
	}
}

func hit(segs []seg, f frame, a, r float32) (int, bool) {
	for i := len(segs) - 1; i >= 0; i-- {
		if sh := f.place(segs[i]); sh.shown() && sh.holds(a, r) {
			return i, true
		}
	}
	return 0, false
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func clamp01(v float32) float32 { return min(max(v, 0), 1) }
