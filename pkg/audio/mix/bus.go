package mix

import (
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type Bus struct {
	mu    sync.Mutex
	lanes map[schema.Stream]*lane
	src   Source

	srcBuf []int16
	parts  []Part
}

type Part struct {
	Stream  schema.Stream
	Samples []int16
	Level   float32
}

type lane struct {
	Queue
	gain  float32
	muted bool
	heard time.Time
}

func (l *lane) level() float32 {
	if l.muted {
		return 0
	}
	return l.gain
}

func (b *Bus) lane(s schema.Stream) *lane {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.held(s)
}

func (b *Bus) held(s schema.Stream) *lane {
	if b.lanes == nil {
		b.lanes = map[schema.Stream]*lane{}
	}
	l, ok := b.lanes[s]
	if !ok {
		l = &lane{gain: 1}
		b.lanes[s] = l
	}
	return l
}

func (b *Bus) Push(s schema.Stream, samples []int16) { b.lane(s).Push(samples) }

func (b *Bus) Overlay(s schema.Stream, samples []int16) { b.lane(s).Overlay(samples) }

func (b *Bus) Take(s schema.Stream) []int16 { return b.lane(s).Take() }

func (b *Bus) Adjust(s schema.Stream, rewrite func([]int16)) { b.lane(s).Adjust(rewrite) }

func (b *Bus) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, l := range b.lanes {
		l.Clear()
	}
}

func (b *Bus) ClearStream(s schema.Stream) { b.lane(s).Clear() }

func (b *Bus) StreamLen(s schema.Stream) int { return b.lane(s).Len() }

func (b *Bus) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	var most int
	for _, l := range b.lanes {
		most = max(most, l.Len())
	}
	return most
}

func (b *Bus) SetGain(s schema.Stream, gain float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held(s).gain = max(gain, 0)
}

func (b *Bus) Gain(s schema.Stream) float32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.held(s).level()
}

func (b *Bus) Mute(s schema.Stream, on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held(s).muted = on
}

func (b *Bus) Muted(s schema.Stream) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.held(s).muted
}

func (b *Bus) Heard(s schema.Stream, within time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.lanes[s]
	return ok && !l.heard.IsZero() && time.Since(l.heard) <= within
}

func (b *Bus) Attach(src Source) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.src = src
}

func (b *Bus) Next(at uint64, out []int32) (queued int, rendered bool) {
	queued, rendered = b.gather(at, len(out))
	b.sum(out)
	return queued, rendered
}

// Parts and their samples are reused by the next call.
func (b *Bus) Parts(at uint64, n int) (parts []Part, queued int, rendered bool) {
	queued, rendered = b.gather(at, n)
	return b.parts, queued, rendered
}

func (b *Bus) gather(at uint64, n int) (queued int, rendered bool) {
	b.parts = b.parts[:0]
	now := time.Now()
	b.mu.Lock()
	for s, l := range b.lanes {
		chunk := l.Next(n)
		queued = max(queued, len(chunk))
		if len(chunk) > 0 {
			b.parts = append(b.parts, Part{Stream: s, Samples: chunk, Level: l.level()})
			l.heard = now
		}
	}
	src, media := b.src, b.held(schema.StreamMedia).level()
	if src != nil {
		b.held(schema.StreamMedia).heard = now
	}
	b.mu.Unlock()

	if src == nil {
		return queued, false
	}
	if cap(b.srcBuf) < n {
		b.srcBuf = make([]int16, n)
	}
	buf := b.srcBuf[:n]
	clear(buf)
	src.Render(at, buf)
	b.parts = append(b.parts, Part{Stream: schema.StreamMedia, Samples: buf, Level: media})
	return queued, true
}

func (b *Bus) sum(out []int32) {
	clear(out)
	for _, p := range b.parts {
		add(out, p.Samples, p.Level)
	}
}

func add(into []int32, samples []int16, gain float32) {
	switch gain {
	case 0:
	case 1:
		for i, v := range samples {
			into[i] += int32(v)
		}
	default:
		for i, v := range samples {
			into[i] += int32(float32(v) * gain)
		}
	}
}
