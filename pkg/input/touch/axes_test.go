package touch

import (
	"os"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/display/geometry"
)

func TestScaleAxis(t *testing.T) {
	a := axisRange{-100, 1372}
	for _, tc := range []struct{ raw, pixel int }{
		{-101, 0}, {-100, 0}, {636, 400}, {1372, 799}, {1373, 799},
	} {
		if got := a.pixel(tc.raw, 800); got != tc.pixel {
			t.Errorf("pixel(%d) = %d, want %d", tc.raw, got, tc.pixel)
		}
	}
}

func TestReadAxesRejectsNonEvdevFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-evdev")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := readAxes(f); err == nil {
		t.Fatal("readAxes accepted a regular file")
	}
}

func TestNormalizeElanBeforeRotation(t *testing.T) {
	panel := &fakePanel{w: 800, h: 1280}
	s := NewWithAxisNormalization("Elan Touchscreen", panel)
	s.axes = axes{axisRange{0, 1472}, axisRange{0, 2368}}
	for _, rot := range []geometry.Orientation{
		geometry.Rotate0, geometry.Rotate90, geometry.Rotate180, geometry.Rotate270,
	} {
		panel.turn(rot)
		for _, tc := range []struct{ rawX, rawY, x, y int }{
			{0, 0, 0, 0}, {0, 2368, 0, 1279},
			{1472, 0, 799, 0}, {1472, 2368, 799, 1279},
			{736, 1184, 400, 640},
		} {
			wantX, wantY := rot.Unproject(800, 1280, tc.x, tc.y)
			x, y := s.place(tc.rawX, tc.rawY)
			if x != wantX || y != wantY {
				t.Errorf("rotation %v at %d,%d = %d,%d, want %d,%d",
					rot, tc.rawX, tc.rawY, x, y, wantX, wantY)
			}
		}
	}
}

func TestElanNormalizesBothSlots(t *testing.T) {
	panel := &fakePanel{w: 800, h: 1280, rot: geometry.Rotate90}
	s := NewWithAxisNormalization("Elan Touchscreen", panel)
	s.axes = axes{axisRange{0, 1472}, axisRange{0, 2368}}
	d := decoder{place: s.place}
	got := feed(&d,
		ev(evAbs, absMTSlot, 0),
		ev(evAbs, absMTTrackingID, 10),
		ev(evAbs, absMTPositionX, 0),
		ev(evAbs, absMTPositionY, 2368),
		ev(evAbs, absMTSlot, 1),
		ev(evAbs, absMTTrackingID, 11),
		ev(evAbs, absMTPositionX, 1472),
		ev(evAbs, absMTPositionY, 0),
		syn(),
	)
	if len(got) != 2 {
		t.Fatalf("got %v, want two contacts", got)
	}
	for _, c := range got {
		if c.Phase != Down || c.ID != 10+c.Slot || c.X != c.Slot*1279 || c.Y != c.Slot*799 {
			t.Errorf("unexpected contact: %+v", c)
		}
	}
}
