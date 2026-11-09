package ringsview

import (
	"math"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type box struct{ x, y, w, h float32 }

type geometry interface {
	point(a, r float32) (x, y float32)
	locate(x, y float32) (a, r float32)
	hub() []float32
	label() box
	steps(span float32) int
}

func lay(look schema.RingsLook, w, h float32) geometry {
	switch look {
	case schema.RingsIcicle:
		return icicle{w: w, h: h, top: h * 0.14}
	case schema.RingsBurst:
		hw, hh := w/2, h/2
		b := min(hw, hh) * 0.18
		return burst{cx: hw, cy: hh, hw: hw, hh: hh, bw: b * min(max(hw/hh, 1), 1.3), bh: b}
	}
	r := min(w, h)/2 - 2
	return wheel{cx: w / 2, cy: h / 2, inner: r * 0.24, outer: r}
}

func angle(a float32) float64 { return -math.Pi/2 + 2*math.Pi*float64(a) }

func turn(x, y float32) float32 {
	t := math.Atan2(float64(y), float64(x)) + math.Pi/2
	if t < 0 {
		t += 2 * math.Pi
	}
	return float32(t / (2 * math.Pi))
}

func radial(span float32) int { return int(span*240) + 2 }

type wheel struct{ cx, cy, inner, outer float32 }

func (g wheel) point(a, r float32) (float32, float32) {
	t := angle(a)
	d := g.inner + r*(g.outer-g.inner)
	return g.cx + d*float32(math.Cos(t)), g.cy + d*float32(math.Sin(t))
}

func (g wheel) locate(x, y float32) (float32, float32) {
	dx, dy := x-g.cx, y-g.cy
	d := float32(math.Hypot(float64(dx), float64(dy)))
	return turn(dx, dy), (d - g.inner) / (g.outer - g.inner)
}

func (g wheel) hub() []float32 {
	var out []float32
	for i := range 96 {
		t := 2 * math.Pi * float64(i) / 96
		out = append(out, g.cx+g.inner*float32(math.Cos(t)), g.cy+g.inner*float32(math.Sin(t)))
	}
	return out
}

func (g wheel) label() box {
	s := g.inner * 1.4
	return box{g.cx - s/2, g.cy - s/2, s, s}
}

func (wheel) steps(span float32) int { return radial(span) }

type burst struct{ cx, cy, hw, hh, bw, bh float32 }

func (g burst) edge(t float64) float32 {
	c, s := math.Abs(math.Cos(t)), math.Abs(math.Sin(t))
	d := math.Inf(1)
	if c > 1e-9 {
		d = float64(g.hw) / c
	}
	if s > 1e-9 {
		d = math.Min(d, float64(g.hh)/s)
	}
	return float32(d)
}

func (g burst) core(t float64) float32 {
	c, s := math.Abs(math.Cos(t)), math.Abs(math.Sin(t))
	return float32(math.Pow(math.Pow(c/float64(g.bw), 4)+math.Pow(s/float64(g.bh), 4), -0.25))
}

func (g burst) point(a, r float32) (float32, float32) {
	t := angle(a)
	d := g.core(t) + r*(g.edge(t)-g.core(t))
	return g.cx + d*float32(math.Cos(t)), g.cy + d*float32(math.Sin(t))
}

func (g burst) locate(x, y float32) (float32, float32) {
	dx, dy := x-g.cx, y-g.cy
	a := turn(dx, dy)
	t := angle(a)
	d := float32(math.Hypot(float64(dx), float64(dy)))
	return a, (d - g.core(t)) / (g.edge(t) - g.core(t))
}

func (g burst) hub() []float32 {
	var out []float32
	for i := range 96 {
		t := 2 * math.Pi * float64(i) / 96
		d := g.core(t)
		out = append(out, g.cx+d*float32(math.Cos(t)), g.cy+d*float32(math.Sin(t)))
	}
	return out
}

func (g burst) label() box {
	w, h := g.bw*1.68, g.bh*1.68
	return box{g.cx - w/2, g.cy - h/2, w, h}
}

func (burst) steps(span float32) int { return radial(span) }

type icicle struct{ w, h, top float32 }

func (g icicle) point(a, r float32) (float32, float32) {
	return a * g.w, g.top + r*(g.h-g.top)
}

func (g icicle) locate(x, y float32) (float32, float32) {
	return x / g.w, (y - g.top) / (g.h - g.top)
}

func (g icicle) hub() []float32 { return []float32{0, 0, g.w, 0, g.w, g.top, 0, g.top} }

func (g icicle) label() box { return box{0, 0, g.w, g.top} }

func (icicle) steps(float32) int { return 2 }
