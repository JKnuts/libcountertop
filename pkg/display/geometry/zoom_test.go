package geometry

import "testing"

func TestAPinchKeepsThePointUnderTheFingers(t *testing.T) {
	view := Rect{W: 1000, H: 600}
	r := Rect{X: 0, Y: 0, W: 1000, H: 600}
	z := Zoom{}.Pinch(r, view, 2, 250, 150)
	if got := z.Apply(r); got != (Rect{X: -250, Y: -150, W: 2000, H: 1200}) {
		t.Errorf("zoomed to %+v", got)
	}
}

func TestZoomStaysWithinItsLimits(t *testing.T) {
	view := Rect{W: 1000, H: 600}
	r := view
	if z := (Zoom{}).Pinch(r, view, 10, 500, 300); z.Scale != MaxZoom {
		t.Errorf("scale %v, want %v", z.Scale, MaxZoom)
	}
	if z := (Zoom{Scale: 2}).Pinch(r, view, 0.5, 500, 300); z.Zoomed() {
		t.Errorf("pinched below 1 kept %+v", z)
	}
}

func TestPanningStopsAtThePictureEdge(t *testing.T) {
	view := Rect{W: 1000, H: 600}
	r := view
	z := Zoom{Scale: 2}.Pan(r, view, 5000, -5000)
	if got := z.Apply(r); got != (Rect{X: 0, Y: -600, W: 2000, H: 1200}) {
		t.Errorf("panned to %+v", got)
	}
	if z := (Zoom{}).Pan(r, view, 100, 100); z.Zoomed() || z.DX != 0 {
		t.Errorf("panned an unzoomed picture: %+v", z)
	}
}

func TestANarrowPictureStaysCenteredAcross(t *testing.T) {
	view := Rect{W: 1000, H: 600}
	r := Rect{X: 300, Y: 0, W: 400, H: 600}
	z := Zoom{Scale: 2}.Pan(r, view, 300, 0)
	if got := z.Apply(r); got.X != 100 || got.W != 800 {
		t.Errorf("zoomed to %+v, want x 100 w 800", got)
	}
}
