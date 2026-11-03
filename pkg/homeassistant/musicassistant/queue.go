package musicassistant

import (
	"context"
	"errors"
	"fmt"
	"hash/maphash"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const (
	askTime   = 15 * time.Second
	stale     = 30 * time.Second
	recheck   = 10 * time.Minute
	pictured  = 12
	keepArt   = 64
	artLimit  = 4 << 20
	firstWait = 5 * time.Second
	lastWait  = 5 * time.Minute
)

type HA interface {
	MusicQueue(ctx context.Context, player string) ([]homeassistant.QueuedTrack, error)
	MusicPlay(ctx context.Context, player, item string) error
}

type Options struct {
	HA      HA
	Player  func() string
	Client  *http.Client
	Changed func()
}

type Queue struct {
	o Options

	mu     sync.Mutex
	key    key
	tracks []homeassistant.QueuedTrack
	asking bool
	asked  time.Time
	fails  int
	until  time.Time
	art    map[string][]byte
}

type key struct{ title, artist, album string }

func New(o Options) *Queue {
	return &Queue{o: o, art: map[string][]byte{}}
}

func (q *Queue) Ahead(now nowplaying.Now) []nowplaying.Track {
	if now.Title == "" {
		return nil
	}
	k := key{now.Title, now.Artist, now.Album}
	t := time.Now()
	q.mu.Lock()
	if k != q.key {
		q.key, q.tracks, q.asked, q.fails, q.until = k, nil, time.Time{}, 0, time.Time{}
	}
	due := !q.asking && t.After(q.until) && t.Sub(q.asked) >= stale
	if due {
		q.asking, q.asked = true, t
	}
	out := q.shown()
	q.mu.Unlock()
	if due {
		safe.Go("music assistant queue", func() { q.fetch(k) })
	}
	return out
}

func (q *Queue) shown() []nowplaying.Track {
	if len(q.tracks) == 0 {
		return nil
	}
	out := make([]nowplaying.Track, 0, len(q.tracks))
	for _, it := range q.tracks {
		tr := nowplaying.Track{Title: it.Title, Artist: it.Artist, Length: it.Duration}
		if raw := q.art[it.Image]; len(raw) > 0 {
			tr.Art, tr.ArtID = raw, artID(raw)
		}
		if id := it.ID; id != "" {
			tr.Play = func() { safe.Go("music assistant play", func() { q.play(id) }) }
		}
		out = append(out, tr)
	}
	return out
}

func (q *Queue) fetch(k key) {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	tracks, err := q.o.HA.MusicQueue(ctx, q.o.Player())
	slog.Debug("music assistant queue", "title", k.title, "tracks", len(tracks), "err", err)
	q.mu.Lock()
	q.asking = false
	if k != q.key {
		q.mu.Unlock()
		return
	}
	if err != nil {
		q.fails++
		q.until = time.Now().Add(backoff(q.fails, err))
		q.mu.Unlock()
		return
	}
	q.fails, q.until, q.tracks = 0, time.Time{}, tracks
	q.mu.Unlock()
	q.changed()
	q.picture(tracks)
}

func (q *Queue) play(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	if err := q.o.HA.MusicPlay(ctx, q.o.Player(), id); err != nil {
		slog.Warn("playing from the music assistant queue", "id", id, "err", err)
		return
	}
	q.mu.Lock()
	q.asked = time.Time{}
	q.mu.Unlock()
}

func (q *Queue) picture(tracks []homeassistant.QueuedTrack) {
	if q.o.Client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	drew := false
	for _, it := range tracks[:min(len(tracks), pictured)] {
		if it.Image == "" {
			continue
		}
		q.mu.Lock()
		_, have := q.art[it.Image]
		q.mu.Unlock()
		if have {
			continue
		}
		raw, err := q.download(ctx, it.Image)
		if err != nil {
			slog.Debug("music assistant queue art", "url", it.Image, "err", err)
		}
		q.mu.Lock()
		if len(q.art) >= keepArt {
			q.art = map[string][]byte{}
		}
		q.art[it.Image] = raw
		q.mu.Unlock()
		drew = drew || len(raw) > 0
	}
	if drew {
		q.changed()
	}
}

func (q *Queue) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := q.o.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicassistant: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, artLimit))
}

func (q *Queue) changed() {
	if q.o.Changed != nil {
		q.o.Changed()
	}
}

func backoff(fails int, err error) time.Duration {
	if errors.Is(err, homeassistant.ErrNoMusic) {
		return recheck
	}
	return min(firstWait<<min(fails-1, 6), lastWait)
}

var artSeed = maphash.MakeSeed()

func artID(art []byte) uint64 { return maphash.Bytes(artSeed, art) }
