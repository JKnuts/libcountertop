package a2dp

import (
	"github.com/ygelfand/libcountertop/pkg/audio/analysis"
	"github.com/ygelfand/libcountertop/pkg/audio/ducking"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avrcp"
	"github.com/ygelfand/libcountertop/pkg/media/pcm"
)

var _ pcm.Producer = (*Sink)(nil)

func (s *Sink) Stand(down bool) {
	s.mu.Lock()
	first := down && !s.down
	s.down = down
	if first {
		s.drift.Reset()
	}
	s.mu.Unlock()

	if !first || !s.audible() {
		return
	}

	s.options.Queue.Drain()
	s.press(avrcp.OpPause)
}

func (s *Sink) Duck(on bool) {
	gain := float32(1)
	if on {
		gain = ducking.Gain(s.options.Ducking.Read().DuckDB)
	}

	s.mu.Lock()
	s.gain = gain
	s.mu.Unlock()
}

func (s *Sink) Requeue() {
	s.mu.Lock()
	gain := s.gain
	s.mu.Unlock()

	if gain == 1 || !s.audible() {
		return
	}
	s.options.Queue.Adjust(func(samples []int16) { analysis.Scale(samples, gain) })
}
