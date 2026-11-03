package geometry

import "math"

const MaxZoom = 4.0

type Zoom struct {
	Scale  float64
	DX, DY float64
}

func (z Zoom) Zoomed() bool { return z.Scale > 1 }

func (z Zoom) Apply(r Rect) Rect {
	if !z.Zoomed() {
		return r
	}
	w, h := float64(r.W)*z.Scale, float64(r.H)*z.Scale
	x := float64(r.X) + float64(r.W)/2 - w/2 + z.DX
	y := float64(r.Y) + float64(r.H)/2 - h/2 + z.DY
	return Rect{
		X: int(math.Round(x)),
		Y: int(math.Round(y)),
		W: int(math.Round(w)),
		H: int(math.Round(h)),
	}
}

func (z Zoom) Pinch(r, view Rect, scale, px, py float64) Zoom {
	scale = min(max(scale, 1), MaxZoom)
	was := z
	if !was.Zoomed() {
		was = Zoom{Scale: 1}
	}
	at := was.Apply(r)
	k := scale / was.Scale
	x := px - (px-float64(at.X))*k
	y := py - (py-float64(at.Y))*k
	w, h := float64(r.W)*scale, float64(r.H)*scale
	next := Zoom{
		Scale: scale,
		DX:    x - (float64(r.X) + float64(r.W)/2 - w/2),
		DY:    y - (float64(r.Y) + float64(r.H)/2 - h/2),
	}
	return next.inside(r, view)
}

func (z Zoom) Pan(r, view Rect, dx, dy float64) Zoom {
	if !z.Zoomed() {
		return z
	}
	z.DX += dx
	z.DY += dy
	return z.inside(r, view)
}

func (z Zoom) inside(r, view Rect) Zoom {
	if !z.Zoomed() {
		return Zoom{}
	}
	z.DX = keep(
		float64(r.X)+float64(r.W)/2,
		float64(r.W)*z.Scale,
		float64(view.X),
		float64(view.W),
		z.DX,
	)
	z.DY = keep(
		float64(r.Y)+float64(r.H)/2,
		float64(r.H)*z.Scale,
		float64(view.Y),
		float64(view.H),
		z.DY,
	)
	return z
}

func keep(center, size, from, span, shift float64) float64 {
	if size <= span {
		return from + span/2 - center
	}
	lo := center - size/2 + shift
	lo = min(max(lo, from+span-size), from)
	return lo - (center - size/2)
}
