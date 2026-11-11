package a2dp

import (
	"fmt"
	"log/slog"
	"time"

	bt "github.com/ygelfand/libcountertop/pkg/bluetooth"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avrcp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

const listing = 50

const (
	labelCount = iota
	labelFolder
	labelItem
)

const ahead = 6

func (s *Sink) open() {
	if peer := s.selected(); peer != nil {
		peer.open()
		return
	}
	if s.browsable() {
		return
	}
	s.tell("opening the browsing channel", (*bt.Sink).OpenBrowsing)
}

const patience = 5 * time.Second

func (s *Sink) list() {
	if peer := s.selected(); peer != nil {
		peer.list()
		return
	}
	if !s.browsable() {
		return
	}

	s.mu.Lock()
	busy := time.Since(s.asking) < patience
	if !busy {
		s.asking = time.Now()
	}
	s.mu.Unlock()

	if busy {
		return
	}

	s.tell("counting the now playing list", func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Browse([]avrcp.Transport{
			avrcp.TotalItems(avrcp.ScopeNowPlaying).Request(labelCount),
		})
	})
}

func (s *Sink) folder(n uint32) {
	s.tell(fmt.Sprintf("asking for %d queued", n), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Browse([]avrcp.Transport{
			avrcp.FolderItems(avrcp.ScopeNowPlaying, 0, n-1).Request(labelFolder),
		})
	})
}

func (s *Sink) listed(b avrcp.Browse) {
	switch b.ID {
	case avrcp.BrowseTotalItems:
		n, err := avrcp.ParseTotalItems(b.Params)
		if err != nil {
			slog.Warn("bluetooth browsing", "err", err)
			s.done()
			return
		}

		if n == 0 {
			s.holds(nil)
			s.done()
			return
		}
		s.folder(min(n, listing))

	case avrcp.BrowseItemAttrs:
		s.attributed(b)

	case avrcp.BrowseFolderItems:
		s.done()

		items, err := avrcp.ParseFolderItems(b.Params)
		if err != nil {
			slog.Warn("bluetooth browsing listing", "err", err, "params", brief(b.Params))
			return
		}

		counter, _ := avrcp.Counter(b.Params)
		s.walk(counter, items)

		slog.Debug("bluetooth queued", "items", len(items))
		if tracing() {
			for i, it := range items {
				slog.Info("bluetooth queued", "at", i, "uid", it.UID,
					"title", it.Track.Title, "artist", it.Track.Artist, "art", it.Track.Art)
			}
		}
		s.holds(items)

	default:
		status := byte(0)
		if len(b.Params) > 0 {
			status = b.Params[0]
		}
		slog.Debug("bluetooth browsing answered", "pdu", fmt.Sprintf("%#02x", b.ID),
			"status", fmt.Sprintf("%#02x", status))
	}
}

func (s *Sink) done() {
	s.mu.Lock()
	s.asking = time.Time{}
	s.mu.Unlock()
}

func (s *Sink) holds(items []avrcp.Item) {
	s.mu.Lock()
	s.queue, s.handles = items, nil
	s.mu.Unlock()

	s.options.Media.Changed()
}

func (s *Sink) browsable() bool {
	s.mu.Lock()
	sink := s.sink
	s.mu.Unlock()

	return sink != nil && sink.Browsing()
}

// All ones is no track selected.
const nothing = ^uint64(0)

func (s *Sink) queued(state avrcp.State) []nowplaying.Track {
	s.mu.Lock()
	items := s.queue
	s.mu.Unlock()

	s.mu.Lock()
	handles := s.handles
	s.mu.Unlock()

	out := make([]nowplaying.Track, 0, len(items))
	for _, it := range after(items, state) {
		raw, id := s.image(handles[it.UID])
		out = append(out, nowplaying.Track{
			Title:  it.Track.Title,
			Artist: it.Track.Artist,
			Length: it.Track.Duration,
			Art:    raw,
			ArtID:  id,
		})
	}
	return out
}

func after(items []avrcp.Item, state avrcp.State) []avrcp.Item {
	known := state.UID != 0 && state.UID != nothing

	for i, it := range items {
		switch {
		case known && it.UID == state.UID:
			return items[i+1:]
		case !known && state.Track.Title != "" && it.Track.Title == state.Track.Title:
			return items[i+1:]
		}
	}
	return items
}

func (s *Sink) walk(counter uint16, items []avrcp.Item) {
	uids := make([]uint64, 0, ahead)
	for _, it := range items {
		if it.Kind != avrcp.ItemElement || len(uids) == ahead {
			continue
		}
		uids = append(uids, it.UID)
	}

	s.mu.Lock()
	running := s.at != 0 && time.Since(s.walked) < patience
	if running {
		s.later, s.waiting = uids, counter
	} else {
		s.counter, s.asks = counter, uids
		s.asked, s.got = len(uids), 0
	}
	s.mu.Unlock()

	s.expire()

	if !running {
		s.describe()
	}
}

func (s *Sink) describe() {
	s.mu.Lock()
	if len(s.asks) == 0 {
		s.at = 0
		got, asked, queued := s.got, s.asked, len(s.queue)

		next, counter := s.later, s.waiting
		s.later = nil

		if len(next) > 0 {
			s.counter, s.asks = counter, next
			s.asked, s.got = len(next), 0
		}
		s.mu.Unlock()

		slog.Debug("bluetooth entry artwork", "found", got, "asked", asked, "queued", queued)

		if len(next) > 0 {
			s.describe()
		}
		return
	}

	uid, counter := s.asks[0], s.counter
	s.at, s.asks, s.walked = uid, s.asks[1:], time.Now()
	s.mu.Unlock()

	s.tell(fmt.Sprintf("asking about entry %d", uid), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Browse([]avrcp.Transport{
			avrcp.ItemAttributes(avrcp.ScopeNowPlaying, uid, counter).Request(labelItem),
		})
	})
}

func (s *Sink) attributed(b avrcp.Browse) {
	s.mu.Lock()
	uid := s.at
	s.mu.Unlock()

	track, err := avrcp.ParseItemAttributes(b.Params)
	if err != nil {
		slog.Warn("bluetooth browsing entry", "uid", uid, "err", err,
			"answer", fmt.Sprintf("% x", b.Params))
		s.describe()
		return
	}

	if tracing() {
		slog.Info("bluetooth entry", "uid", uid, "title", track.Title, "artist", track.Artist,
			"album", track.Album, "art", track.Art, "answer", fmt.Sprintf("% x", b.Params))
	}

	if track.Art != "" {
		s.mu.Lock()
		if s.handles == nil {
			s.handles = map[uint64]string{}
		}
		s.handles[uid] = track.Art
		s.got++
		s.mu.Unlock()

		s.wants(track.Art, false)
	}

	s.describe()
}
