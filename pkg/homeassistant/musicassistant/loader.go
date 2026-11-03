package musicassistant

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

type slot[T any] struct {
	value  T
	err    error
	loaded bool
	asking bool
	asked  time.Time
}

type loader[T any] struct {
	name    string
	fresh   time.Duration
	fetch   func(ctx context.Context, key string) (T, error)
	changed func()

	mu    sync.Mutex
	slots map[string]*slot[T]
}

func newLoader[T any](
	name string,
	fresh time.Duration,
	fetch func(context.Context, string) (T, error),
	changed func(),
) *loader[T] {
	return &loader[T]{
		name:    name,
		fresh:   fresh,
		fetch:   fetch,
		changed: changed,
		slots:   map[string]*slot[T]{},
	}
}

func (l *loader[T]) get(key string) (T, error, bool) {
	l.mu.Lock()
	s := l.slots[key]
	if s == nil {
		if len(l.slots) >= keepSlots {
			l.slots = map[string]*slot[T]{}
		}
		s = &slot[T]{}
		l.slots[key] = s
	}
	due := !s.asking && (!s.loaded || time.Since(s.asked) >= l.fresh)
	if due {
		s.asking, s.asked = true, time.Now()
	}
	value, err, loaded := s.value, s.err, s.loaded
	l.mu.Unlock()
	if due {
		safe.Go("music assistant "+l.name, func() { l.load(key, s) })
	}
	return value, err, loaded
}

func (l *loader[T]) drop(key string) {
	l.mu.Lock()
	delete(l.slots, key)
	l.mu.Unlock()
	if l.changed != nil {
		l.changed()
	}
}

func (l *loader[T]) load(key string, s *slot[T]) {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	value, err := l.fetch(ctx, key)
	if err != nil {
		slog.Debug("music assistant", "what", l.name, "key", key, "err", err)
	}
	l.mu.Lock()
	s.asking, s.loaded, s.err = false, true, err
	if err == nil {
		s.value = value
	}
	l.mu.Unlock()
	if l.changed != nil {
		l.changed()
	}
}
