package a2dp

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	bt "github.com/ygelfand/libcountertop/pkg/bluetooth"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avrcp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

// The phone is the A2DP source but the AVRCP target.

type source struct{ sink *Sink }

func (s source) Now() nowplaying.Now {
	remote := s.sink.remote()
	if remote == nil {
		return nowplaying.Now{}
	}

	state := remote.State()
	now := nowOf(state, s.sink.playing(), s.sink.queued(state))
	now.Art, now.ArtID = s.sink.cover(state.Track.Art)
	return now
}

func (s source) Playing() (playing, paused bool) {
	now := s.Now()
	return now.Playing, now.Paused
}

func (s *Sink) playing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streaming
}

func nowOf(state avrcp.State, streaming bool, queue []nowplaying.Track) nowplaying.Now {
	return nowplaying.Now{
		Playing: streaming && state.Progress.Status != avrcp.StatusPaused,
		Paused:  state.Progress.Status == avrcp.StatusPaused,

		Title:  state.Track.Title,
		Artist: state.Track.Artist,
		Album:  state.Track.Album,

		Elapsed: state.Progress.Position,
		Length:  state.Progress.Length,

		Queue: queue,

		Can: nowplaying.CanPause | nowplaying.CanNext | nowplaying.CanPrevious,
	}
}

func (source) Kind() nowplaying.Kind { return nowplaying.FromBluetooth }
func (s source) Label() string       { return s.sink.Label() }

func (s source) Play()     { s.sink.press(avrcp.OpPlay) }
func (s source) Pause()    { s.sink.press(avrcp.OpPause) }
func (s source) Stop()     { s.sink.press(avrcp.OpStop) }
func (s source) Next()     { s.sink.press(avrcp.OpNext) }
func (s source) Previous() { s.sink.press(avrcp.OpPrevious) }

func (s *Sink) remote() *avrcp.Controller {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sink == nil {
		return nil
	}
	return s.sink.Remote()
}

func (s *Sink) press(op byte) {
	s.tell(fmt.Sprintf("button %#02x", op), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Press(op)
	})
}

func (s *Sink) ask() {
	s.tell("asking what is playing", (*bt.Sink).Ask)
}

var traced atomic.Int64

func Trace(within time.Duration) { traced.Store(time.Now().Add(within).UnixNano()) }

func tracing() bool {
	until := traced.Load()
	return until != 0 && time.Now().UnixNano() < until
}

func (s *Sink) Attributes(ids ...uint32) {
	s.tell(
		fmt.Sprintf("asking for attributes %v", ids),
		func(sink *bt.Sink) ([]l2cap.Frame, error) {
			return sink.Attributes(ids...)
		},
	)
}

func (s *Sink) Browsing() { s.open() }

func (s *Sink) Queue() { s.list() }

func (s *Sink) Look(class uint16) {
	s.tell(fmt.Sprintf("looking up %#04x", class), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Discover(sdp.Search{
			Pattern:    []uint32{uint32(class)},
			MaxBytes:   0x03f0,
			Attributes: []sdp.AttrRange{{First: 0x0000, Last: 0xffff}},
		})
	})
}

func (s *Sink) Looking() {
	s.tell("opening a lookup channel", (*bt.Sink).OpenDiscovery)
}

func (s *Sink) look() {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()

	if sink == nil {
		return
	}
	if !sink.Finding() {
		s.Looking()
		return
	}
	s.Look(sdp.UUIDAVRemoteControlTarget)
}

func (s *Sink) tell(what string, build func(*bt.Sink) ([]l2cap.Frame, error)) {
	if peer := s.selected(); peer != nil {
		peer.tell(what, build)
		return
	}
	s.mu.Lock()
	sink, send, handle := s.sink, s.send, s.handle
	s.mu.Unlock()

	if sink == nil || send == nil || handle == 0 {
		return
	}

	frames, err := build(sink)
	if err != nil {
		slog.Warn("bluetooth not sent", "what", what, "err", err)
		return
	}

	for _, f := range frames {
		if tracing() {
			slog.Info("bluetooth sent", "what", what,
				"cid", fmt.Sprintf("%#04x", f.CID), "pdu", brief(f.Payload))
		}

		if err := s.put(send, handle, f.Marshal()); err != nil {
			slog.Warn("bluetooth not sent", "what", what, "err", err)
			return
		}
	}
}

func (s *Sink) put(send func(uint16, []byte) error, handle uint16, pdu []byte) error {
	s.mu.Lock()
	if s.answering {
		s.held = append(s.held, pdu)
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	return send(handle, pdu)
}

func (s *Sink) attach() {
	s.options.Media.Began(source{sink: s})
}

func (s *Sink) detach() {
	if s.options.Media == nil {
		return
	}
	s.options.Media.Ended(source{sink: s})
	s.options.Media.Release(source{sink: s})
}
