package playerview

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	backend "github.com/ygelfand/libcountertop/pkg/display/gogui"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type screenSection struct{ s schema.Screen }

func (c *screenSection) Read() schema.Screen { return c.s }

func (c *screenSection) Update(change func(*schema.Screen)) error {
	change(&c.s)
	return nil
}

type quiet struct{}

func (quiet) Play()     {}
func (quiet) Pause()    {}
func (quiet) Stop()     {}
func (quiet) Next()     {}
func (quiet) Previous() {}

type playing struct{ now nowplaying.Now }

func (p playing) Now() nowplaying.Now            { return p.now }
func (playing) Named() string                    { return "AirPlay" }
func (playing) From() nowplaying.Kind            { return nowplaying.FromCast }
func (playing) Transport() nowplaying.Controller { return quiet{} }
func (playing) Open()                            {}

type level struct{ v int }

func (l *level) Level() int             { return l.v }
func (l *level) Set(v int)              { l.v = v }
func (l *level) Watch(func(int)) func() { return func() {} }

type drawn struct{ frames int }

func (*drawn) Program(int, string, string) error                          { return nil }
func (*drawn) Texture(uint32, int, int, int, int, int, int, []byte) error { return nil }
func (d *drawn) Frame(int, [4]float32, []float32, []uint32) (uint32, uint32, error) {
	d.frames++
	return 0, 0, nil
}

func cover(t *testing.T, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTheBackdropIsTheCoverDimmed(t *testing.T) {
	img := ui.Picture(cover(t, color.NRGBA{R: 255, G: 255, B: 255, A: 255}), 9001)
	if img == nil {
		t.Fatal("the test cover did not decode")
	}
	src := Backdrop(img)
	if src == "" || !gogui.HasImage(src[len("mem:"):]) {
		t.Fatalf("backdrop %q was not registered", src)
	}
	small := shrink(img, 64, 64)
	for range blurPasses {
		small = blur(small, backdropW, backdropH, 1, 0)
		small = blur(small, backdropW, backdropH, 0, 1)
	}
	for i, v := range small {
		if v < 254.9 || v > 255.1 {
			t.Fatalf("channel %d of a white cover blurred to %v", i, v)
		}
	}
}

func TestThePlayerDrawsBothWays(t *testing.T) {
	now := nowplaying.Now{
		Playing: true,
		Title:   "Night Shift",
		Artist:  "Lucy Dacus",
		Album:   "Historian",
		Art:     cover(t, color.NRGBA{R: 160, G: 30, B: 30, A: 255}),
		ArtID:   9002,
		Elapsed: 3 * time.Second,
		Length:  6*time.Minute + 31*time.Second,
		Can:     nowplaying.CanPause | nowplaying.CanNext | nowplaying.CanPrevious | nowplaying.CanStop,
		Queue: []nowplaying.Track{
			{Title: "Souvenir", Artist: "Lucy Dacus", Length: 3*time.Minute + 32*time.Second},
			{
				Title:  "A very long track title that has to be cut short to fit its row",
				Artist: "Someone",
				Art:    cover(t, color.NRGBA{R: 20, G: 90, B: 160, A: 255}),
				ArtID:  9003,
			},
		},
	}
	for _, size := range []struct{ w, h int }{{1280, 800}, {800, 1280}} {
		presentation := widgets.NewUI(&screenSection{s: schema.DefaultScreen()})
		presentation.Reset()
		p := New(Dependencies{
			Player: playing{now: now},
			Volume: &level{v: 40},
			Shell:  shell.New(),
			UI:     presentation,
		})
		w := gogui.SimpleWindow("player", size.w, size.h, &struct{}{}, func(w *gogui.Window) {
			w.SetView(p.Body)
		})
		sink := &drawn{}
		r, err := backend.New(sink, w)
		if err != nil {
			t.Fatal(err)
		}
		w.Config.OnInit(w)
		if _, err := r.Render(w); err != nil {
			t.Fatalf("%dx%d: %v", size.w, size.h, err)
		}
		if sink.frames == 0 {
			t.Errorf("%dx%d drew nothing", size.w, size.h)
		}
	}
}
