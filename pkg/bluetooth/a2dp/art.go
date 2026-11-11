package a2dp

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	bt "github.com/ygelfand/libcountertop/pkg/bluetooth"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/obex"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
)

type session struct {
	mu sync.Mutex

	connection uint32
	open       bool

	body  []byte
	asked string
	began time.Time

	psm uint16

	have map[string]*kept
	want []string
}

type kept struct {
	raw  []byte
	id   uint64
	used time.Time
}

var artIDs atomic.Uint64

const stays = 10 * time.Minute

const waits = 30 * time.Second

func (a *session) clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.connection, a.open, a.body, a.asked = 0, false, nil, ""
	a.have, a.want, a.psm, a.began = nil, nil, 0, time.Time{}
}

func (s *Sink) looked(records []sdp.Record) {
	for _, r := range records {
		psm, served := sdp.MorePSM(r, sdp.UUIDOBEX)
		if !served || psm == 0 {
			continue
		}

		s.art.mu.Lock()
		first := s.art.psm != psm
		s.art.psm = psm
		s.art.mu.Unlock()

		if first {
			slog.Debug("bluetooth images are served", "psm", fmt.Sprintf("%#04x", psm))
		}

		if !s.imaging() {
			s.Art(psm)
		}
		return
	}
}

func (s *Sink) wants(handle string, first bool) {
	if handle == "" {
		return
	}

	s.art.mu.Lock()
	queued := false
	if k := s.art.have[handle]; k != nil {
		k.used = time.Now()
		queued = true
	}
	for _, h := range s.art.want {
		queued = queued || h == handle
	}

	switch {
	case queued:
	case first:
		s.art.want = append([]string{handle}, s.art.want...)
	default:
		s.art.want = append(s.art.want, handle)
	}
	s.art.mu.Unlock()

	s.pump()
}

func (s *Sink) expire() {
	s.art.mu.Lock()
	defer s.art.mu.Unlock()

	for h, k := range s.art.have {
		if time.Since(k.used) > stays {
			delete(s.art.have, h)
		}
	}
}

func (s *Sink) image(handle string) ([]byte, uint64) {
	if handle == "" {
		return nil, 0
	}

	s.art.mu.Lock()
	defer s.art.mu.Unlock()

	k := s.art.have[handle]
	if k == nil {
		return nil, 0
	}
	k.used = time.Now()
	return k.raw, k.id
}

func (s *Sink) cover(handle string) ([]byte, uint64) {
	if raw, id := s.image(handle); raw != nil {
		return raw, id
	}

	s.wants(handle, true)
	return nil, 0
}

func (s *Sink) pump() {
	s.art.mu.Lock()
	id, open := s.art.connection, s.art.open

	if s.art.asked != "" {
		if time.Since(s.art.began) < waits {
			s.art.mu.Unlock()
			return
		}

		gave := s.art.asked
		s.art.want = without(s.art.want, gave)
		s.art.asked, s.art.body = "", nil

		slog.Warn("bluetooth the image never came", "handle", gave)
	}

	if !open || len(s.art.want) == 0 {
		s.art.mu.Unlock()
		return
	}

	handle := s.art.want[0]
	s.art.asked, s.art.body, s.art.began = handle, nil, time.Now()
	s.art.mu.Unlock()

	s.tell(fmt.Sprintf("asking for image %s", handle), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Art(obex.Thumbnail(id, handle).Marshal())
	})
}

func (s *Sink) fetched(handle string, raw []byte) {
	s.art.mu.Lock()
	s.art.asked, s.art.body = "", nil

	if raw != nil {
		if s.art.have == nil {
			s.art.have = map[string]*kept{}
		}
		s.art.have[handle] = &kept{raw: raw, id: artIDs.Add(1), used: time.Now()}
	}

	s.art.want = without(s.art.want, handle)
	s.art.mu.Unlock()

	if raw != nil {
		s.options.Media.Changed()
	}
	s.pump()
}

func without(queue []string, handle string) []string {
	out := queue[:0]
	for _, h := range queue {
		if h != handle {
			out = append(out, h)
		}
	}
	return out
}

func (s *Sink) imaging() bool {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()

	return sink != nil && sink.Imaging()
}

func (s *Sink) Art(psm uint16) {
	if peer := s.selected(); peer != nil {
		peer.Art(psm)
		return
	}
	s.art.mu.Lock()
	s.art.connection, s.art.open, s.art.body, s.art.asked = 0, false, nil, ""
	s.art.psm = psm
	s.art.mu.Unlock()

	s.tell(fmt.Sprintf("opening the image channel on %#04x", psm),
		func(sink *bt.Sink) ([]l2cap.Frame, error) { return sink.OpenArt(psm) })
}

func (s *Sink) Hello() {
	s.tell("connecting to the image service", func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Art(obex.Connect(0x0200, obex.CoverArtTarget).Marshal())
	})
}

func (s *Sink) Thumbnail(handle string) {
	if peer := s.selected(); peer != nil {
		peer.Thumbnail(handle)
		return
	}
	s.wants(handle, true)
}

func (s *Sink) imaged(packet []byte) {
	s.art.mu.Lock()
	connecting, handle := !s.art.open, s.art.asked
	s.art.mu.Unlock()

	p, err := obex.ParseAnswer(packet, connecting)
	if err != nil {
		slog.Warn("bluetooth image answer", "err", err)
		return
	}

	if p.Code != obex.OK && p.Code != obex.Continue {
		slog.Warn("bluetooth image refused", "handle", handle,
			"code", fmt.Sprintf("%#02x", p.Code))
		s.fetched(handle, nil)
		return
	}

	if connecting {
		id, ok := p.Connection()
		if !ok {
			slog.Warn("bluetooth the image service gave no session")
			return
		}

		s.art.mu.Lock()
		s.art.connection, s.art.open, s.art.asked = id, true, ""
		s.art.mu.Unlock()

		slog.Debug("bluetooth image session", "connection", fmt.Sprintf("%#08x", id), "mtu", p.MTU)
		s.pump()
		return
	}

	chunk, last := p.Body()

	s.art.mu.Lock()
	s.art.body = append(s.art.body, chunk...)
	whole, id := s.art.body, s.art.connection
	s.art.mu.Unlock()

	if !last {
		s.tell("asking for the rest of the image", func(sink *bt.Sink) ([]l2cap.Frame, error) {
			return sink.Art(obex.More(id).Marshal())
		})
		return
	}

	img, err := ui.Decode(whole)
	if err != nil {
		slog.Warn("bluetooth image", "handle", handle, "bytes", len(whole), "err", err)
		s.fetched(handle, nil)
		return
	}

	w, h := img.Size()
	slog.Debug("bluetooth image", "handle", handle, "bytes", len(whole),
		"size", fmt.Sprintf("%dx%d", w, h))

	s.fetched(handle, whole)
}
