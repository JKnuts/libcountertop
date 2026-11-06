package photoframe

import (
	"hash/fnv"
	"image"
	"math"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/media/photos"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	autoSlack  = 1.25
	frameShare = 0.84
	matShare   = 0.022
	kenZoom    = 1.18
	kenLeg     = 30 * time.Second
	facePad    = 30
	faceShare  = 40
)

type Look struct {
	Back   theme.Color
	Fit    schema.PhotoFit
	Change schema.PhotoTransition
	Span   time.Duration
}

type backing int

const (
	backNone backing = iota
	backBlur
	backSolid
)

type placement struct {
	dst, src, blur [4]float32
	back           backing
	mat, shade     float32
}

func Resolve(fit schema.PhotoFit, src image.Rectangle, w, h int) schema.PhotoFit {
	if fit != schema.FitAuto {
		return fit
	}
	if src.Dx() <= 0 || src.Dy() <= 0 || w <= 0 || h <= 0 {
		return schema.FitCover
	}
	ratio := (float64(src.Dx()) / float64(src.Dy())) / (float64(w) / float64(h))
	if ratio <= autoSlack && ratio >= 1/autoSlack {
		return schema.FitCover
	}
	return schema.FitContain
}

func focusOf(faces []photos.Box, w, h int) image.Rectangle {
	rects := make([]image.Rectangle, 0, len(faces))
	for _, f := range faces {
		rects = append(rects, image.Rect(
			int(f.X1*float64(w)), int(f.Y1*float64(h)),
			int(f.X2*float64(w)), int(f.Y2*float64(h)),
		))
	}
	return faceArea(rects)
}

func faceArea(faces []image.Rectangle) image.Rectangle {
	biggest := 0
	for _, f := range faces {
		biggest = max(biggest, f.Dx())
	}
	var all image.Rectangle
	for _, f := range faces {
		if f.Empty() || f.Dx()*100 < biggest*faceShare {
			continue
		}
		pad := max(f.Dx(), f.Dy()) * facePad / 100
		all = all.Union(f.Inset(-pad))
	}
	return all
}

func place(
	fit schema.PhotoFit,
	iw, ih, w, h int,
	focus image.Rectangle,
	id string,
	elapsed, span time.Duration,
) placement {
	img := image.Rect(0, 0, iw, ih)
	focus = focus.Intersect(img)
	whole := [4]float32{0, 0, float32(w), float32(h)}
	crop := cover(img, w, h, focus)
	switch Resolve(fit, img, w, h) {
	case schema.FitContain:
		return placement{
			dst:  rect(contain(img, image.Rect(0, 0, w, h))),
			src:  [4]float32{0, 0, 1, 1},
			back: backBlur,
			blur: uv(crop, iw, ih),
		}
	case schema.FitBars:
		return placement{
			dst:  rect(contain(img, image.Rect(0, 0, w, h))),
			src:  [4]float32{0, 0, 1, 1},
			back: backSolid,
		}
	case schema.FitFramed:
		side := float64(min(w, h))
		return placement{
			dst:   rect(contain(img, shrink(image.Rect(0, 0, w, h), frameShare))),
			src:   [4]float32{0, 0, 1, 1},
			back:  backBlur,
			blur:  uv(crop, iw, ih),
			mat:   float32(math.Max(2, matShare*side)),
			shade: float32(side/60 + 2),
		}
	case schema.FitKenBurns:
		if p, ok := zoomOut(iw, ih, w, h, crop, focus); ok {
			return p
		}
		return placement{dst: whole, src: drift(crop, focus, iw, ih, id, elapsed, span)}
	}
	if p, ok := zoomOut(iw, ih, w, h, crop, focus); ok {
		return p
	}
	return placement{dst: whole, src: uv(crop, iw, ih)}
}

func zoomOut(iw, ih, w, h int, crop, focus image.Rectangle) (placement, bool) {
	if focus.Empty() || focus.Dx() <= crop.Dx() && focus.Dy() <= crop.Dy() {
		return placement{}, false
	}
	fw, fh := float64(w), float64(h)
	scale := max(
		min(fw/float64(focus.Dx()), fh/float64(focus.Dy())),
		min(fw/float64(iw), fh/float64(ih)),
	)
	vw, vh := fw/scale, fh/scale
	x0 := centerOn(float64(focus.Min.X+focus.Max.X)/2, vw, float64(iw)) - vw/2
	y0 := centerOn(float64(focus.Min.Y+focus.Max.Y)/2, vh, float64(ih)) - vh/2
	sx0, sy0 := max(x0, 0), max(y0, 0)
	sx1, sy1 := min(x0+vw, float64(iw)), min(y0+vh, float64(ih))
	return placement{
		dst: [4]float32{
			float32((sx0 - x0) * scale),
			float32((sy0 - y0) * scale),
			float32((sx1 - sx0) * scale),
			float32((sy1 - sy0) * scale),
		},
		src: [4]float32{
			float32(sx0 / float64(iw)),
			float32(sy0 / float64(ih)),
			float32((sx1 - sx0) / float64(iw)),
			float32((sy1 - sy0) / float64(ih)),
		},
		back: backBlur,
		blur: uv(crop, iw, ih),
	}, true
}

func centerOn(c, win, size float64) float64 {
	if win >= size {
		return size / 2
	}
	return min(max(c, win/2), size-win/2)
}

type kenPlan struct {
	in           bool
	fromX, fromY float64
	toX, toY     float64
}

func planFor(id string) kenPlan {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	n := h.Sum32()
	corner := func(bits uint32) (float64, float64) {
		return float64(bits&1) * 0.9, float64(bits>>1&1) * 0.9
	}
	fx, fy := corner(n)
	tx, ty := corner(n >> 2)
	if fx == tx && fy == ty {
		tx, ty = 0.9-fx, 0.9-fy
	}
	return kenPlan{
		in:    n>>4&1 == 1,
		fromX: fx + 0.05,
		fromY: fy + 0.05,
		toX:   tx + 0.05,
		toY:   ty + 0.05,
	}
}

func drift(
	crop, focus image.Rectangle,
	iw, ih int,
	id string,
	elapsed, span time.Duration,
) [4]float32 {
	leg := span
	if leg <= 0 || leg > kenLeg {
		leg = kenLeg
	}
	legs, t := math.Modf(max(float64(elapsed)/float64(leg), 0))
	if int(legs)%2 == 1 {
		t = 1 - t
	}
	ease := t * t * (3 - 2*t)
	plan := planFor(id)
	zoom := 1 + (kenZoom-1)*ease
	if !plan.in {
		zoom = kenZoom - (kenZoom-1)*ease
	}
	cw, ch := float64(crop.Dx()), float64(crop.Dy())
	ww, wh := cw/zoom, ch/zoom
	px := plan.fromX + (plan.toX-plan.fromX)*ease
	py := plan.fromY + (plan.toY-plan.fromY)*ease
	if !focus.Empty() {
		px = within(px, float64(crop.Min.X), cw, ww, float64(focus.Min.X), float64(focus.Max.X))
		py = within(py, float64(crop.Min.Y), ch, wh, float64(focus.Min.Y), float64(focus.Max.Y))
	}
	x := float64(crop.Min.X) + (cw-ww)*px
	y := float64(crop.Min.Y) + (ch-wh)*py
	return [4]float32{
		float32(x / float64(iw)),
		float32(y / float64(ih)),
		float32(ww / float64(iw)),
		float32(wh / float64(ih)),
	}
}

func within(p, start, crop, win, lo, hi float64) float64 {
	slack := crop - win
	if slack <= 0 {
		return p
	}
	from := max((hi-win-start)/slack, 0)
	to := min((lo-start)/slack, 1)
	if from > to {
		return min(max(((lo+hi-win)/2-start)/slack, 0), 1)
	}
	return from + (to-from)*p
}

func cover(src image.Rectangle, w, h int, focus image.Rectangle) image.Rectangle {
	sw, sh := src.Dx(), src.Dy()
	if sw*h > sh*w {
		cw := sh * w / h
		x := src.Min.X + (sw-cw)/2
		if !focus.Empty() {
			x = clampInt((focus.Min.X+focus.Max.X-cw)/2, src.Min.X, src.Max.X-cw)
		}
		return image.Rect(x, src.Min.Y, x+cw, src.Max.Y)
	}
	ch := sw * h / w
	y := src.Min.Y + (sh-ch)/2
	if !focus.Empty() {
		y = clampInt((focus.Min.Y+focus.Max.Y-ch)/2, src.Min.Y, src.Max.Y-ch)
	}
	return image.Rect(src.Min.X, y, src.Max.X, y+ch)
}

func clampInt(v, lo, hi int) int { return min(max(v, lo), hi) }

func contain(src, area image.Rectangle) image.Rectangle {
	sw, sh := src.Dx(), src.Dy()
	aw, ah := area.Dx(), area.Dy()
	w, h := aw, sh*aw/max(sw, 1)
	if h > ah {
		w, h = sw*ah/max(sh, 1), ah
	}
	x := area.Min.X + (aw-w)/2
	y := area.Min.Y + (ah-h)/2
	return image.Rect(x, y, x+w, y+h)
}

func shrink(r image.Rectangle, share float64) image.Rectangle {
	w := int(float64(r.Dx()) * share)
	h := int(float64(r.Dy()) * share)
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + (r.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}

func rect(r image.Rectangle) [4]float32 {
	return [4]float32{float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy())}
}

func uv(r image.Rectangle, iw, ih int) [4]float32 {
	return [4]float32{
		float32(r.Min.X) / float32(iw),
		float32(r.Min.Y) / float32(ih),
		float32(r.Dx()) / float32(iw),
		float32(r.Dy()) / float32(ih),
	}
}
