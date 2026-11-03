package touch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/geometry"
	"github.com/ygelfand/libcountertop/pkg/hook"
	"github.com/ygelfand/libcountertop/pkg/runtime/startup"
)

type Phase int

const (
	Down Phase = iota
	Move
	Up
)

func (p Phase) String() string {
	switch p {
	case Down:
		return "down"
	case Move:
		return "move"
	case Up:
		return "up"
	}
	return "unknown"
}

type Contact struct {
	Slot  int
	ID    int
	X, Y  int
	Phase Phase

	At time.Time
}

func (c Contact) String() string {
	return fmt.Sprintf("%s at %d,%d (slot %d, id %d)", c.Phase, c.X, c.Y, c.Slot, c.ID)
}

type Panel interface {
	Native() (w, h int)
	Orientation() geometry.Orientation
}

type view struct {
	rot  geometry.Orientation
	w, h int
}

type Screen struct {
	Contacts hook.Hook[Contact]

	Gestures hook.Hook[Gesture]

	device string
	panel  Panel

	feed sync.Mutex

	mu   sync.Mutex
	f    *os.File
	err  error
	rec  *Recognizer
	view view

	started time.Time
	last    time.Time

	unplaced sync.Once
}

func New(device string, panel Panel) *Screen {
	return &Screen{device: device, panel: panel, started: time.Now()}
}

func (s *Screen) Name() string { return "touch" }

func (s *Screen) Since() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.last.IsZero() {
		return time.Since(s.started)
	}
	return time.Since(s.last)
}

func (s *Screen) Startup() startup.Progress {
	s.mu.Lock()
	open, err := s.f != nil, s.err
	s.mu.Unlock()

	switch {
	case open:
		return startup.Progress{Done: true}
	case err != nil:
		return startup.Progress{Failed: true, Doing: err.Error()}
	}
	return startup.Progress{Doing: "taking the touchscreen"}
}

func (s *Screen) Start(context.Context) error {
	f, err := open(s.device)

	s.mu.Lock()
	s.f, s.err = f, err
	s.mu.Unlock()

	if err == nil {
		slog.Info("touchscreen", "name", s.device, "path", f.Name())
	}
	return err
}

func open(name string) (*os.File, error) {
	path, err := Find(name)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("touch: %s: %w", path, err)
	}
	return f, nil
}

func (s *Screen) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

func (s *Screen) Run(ctx context.Context) error {
	s.mu.Lock()
	f := s.f
	s.mu.Unlock()

	if f == nil {
		<-ctx.Done()
		return nil
	}

	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()

	d := decoder{place: s.place}
	err := read(f, func(e rawEvent) {
		for _, c := range d.event(e, time.Now()) {
			s.Feed(c)
		}
	})
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *Screen) Feed(c Contact) {
	s.feed.Lock()
	defer s.feed.Unlock()

	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()

	s.Contacts.Emit(c)

	if g, ok := s.recognize(c); ok {
		slog.Debug("gesture", "kind", g.Kind, "from", g.From, "toward", g.Toward,
			"at", fmt.Sprintf("%d,%d", g.EndX, g.EndY))
		s.Gestures.Emit(g)
	}
}

func (s *Screen) recognize(c Contact) (Gesture, bool) {
	w, h := s.panel.Native()
	v := view{rot: s.panel.Orientation(), w: w, h: h}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rec == nil || s.view != v {
		s.rec, s.view = NewRecognizer(v.rot.Size(w, h)), v
	}
	return s.rec.Feed(c)
}

func (s *Screen) place(px, py int) (int, int) {
	w, h := s.panel.Native()
	if w == 0 {
		s.unplaced.Do(func() {
			slog.Warn("no panel, so touches are reported where the hardware put them")
		})
		return px, py
	}
	return s.panel.Orientation().Unproject(w, h, px, py)
}
