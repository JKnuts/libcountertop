package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const (
	LRCLIB    = "https://lrclib.net"
	askTime   = 15 * time.Second
	keep      = 128
	lengthFit = 2
	jitter    = 30
)

var (
	errMissing = errors.New("lyrics: not found")
	steps      = []time.Duration{
		5 * time.Second,
		15 * time.Second,
		30 * time.Second,
		time.Minute,
		2 * time.Minute,
	}
)

type Query struct {
	Title  string
	Artist string
	Album  string
	Length time.Duration
}

type Options struct {
	Client  *http.Client
	Base    string
	Agent   string
	Changed func()
}

type Finder struct {
	o     Options
	mu    sync.Mutex
	known map[Query]entry
}

type entry struct {
	sheet  Sheet
	asking bool
	fails  int
	until  time.Time
}

func New(o Options) *Finder {
	if o.Base == "" {
		o.Base = LRCLIB
	}
	return &Finder{o: o, known: map[Query]entry{}}
}

func (f *Finder) Find(q Query) Sheet {
	if q.Title == "" {
		return Sheet{}
	}
	q.Length = q.Length.Round(time.Second)
	f.mu.Lock()
	e, ok := f.known[q]
	if ok && (e.asking || e.until.IsZero() || time.Now().Before(e.until)) {
		f.mu.Unlock()
		return e.sheet
	}
	if len(f.known) >= keep {
		f.known = map[Query]entry{}
	}
	f.known[q] = entry{asking: true, fails: e.fails}
	f.mu.Unlock()
	safe.Go("lyrics", func() { f.ask(q, e.fails) })
	return Sheet{}
}

func (f *Finder) ask(q Query, fails int) {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	sheet, via, err := f.lookup(ctx, q)
	e := entry{sheet: sheet}
	if err != nil && !errors.Is(err, errMissing) {
		e.fails = fails + 1
		e.until = time.Now().Add(backoff(e.fails, err))
	}
	slog.Debug("lyrics",
		"title", q.Title,
		"artist", q.Artist,
		"album", q.Album,
		"length", q.Length,
		"via", via,
		"lines", len(sheet.Lines),
		"synced", sheet.Synced,
		"err", err,
	)
	f.mu.Lock()
	f.known[q] = e
	f.mu.Unlock()
	if !sheet.Empty() && f.o.Changed != nil {
		f.o.Changed()
	}
}

type record struct {
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r record) sheet() Sheet {
	if r.Instrumental {
		return Sheet{}
	}
	if lines := ParseLRC(r.SyncedLyrics); len(lines) > 0 {
		return Sheet{Lines: lines, Synced: true}
	}
	return Sheet{Lines: ParsePlain(r.PlainLyrics)}
}

func (f *Finder) lookup(ctx context.Context, q Query) (Sheet, string, error) {
	asks := []url.Values{{"track_name": {q.Title}, "artist_name": {q.Artist}}}
	if q.Album != "" {
		asks = append(asks, url.Values{
			"track_name":  {q.Title},
			"artist_name": {q.Artist},
			"album_name":  {q.Album},
		})
	}
	for i, v := range asks {
		via := fmt.Sprintf("search %d", i+1)
		var found []record
		if err := f.get(ctx, "/api/search", v, &found); err != nil {
			return Sheet{}, via, err
		}
		if r, ok := pick(found, q); ok {
			return r.sheet(), via, nil
		}
	}
	if q.Length <= 0 {
		return Sheet{}, "", nil
	}
	exact := url.Values{
		"track_name":  {q.Title},
		"artist_name": {q.Artist},
		"duration":    {fmt.Sprint(int(q.Length.Seconds()))},
	}
	if q.Album != "" {
		exact.Set("album_name", q.Album)
	}
	var r record
	if err := f.get(ctx, "/api/get", exact, &r); err != nil {
		if errors.Is(err, errMissing) {
			return Sheet{}, "exact", nil
		}
		return Sheet{}, "exact", err
	}
	return r.sheet(), "exact", nil
}

func pick(found []record, q Query) (record, bool) {
	var best record
	score := -1
	for _, r := range found {
		if r.Instrumental || (r.SyncedLyrics == "" && r.PlainLyrics == "") {
			continue
		}
		if q.Length > 0 && math.Abs(r.Duration-q.Length.Seconds()) > lengthFit {
			continue
		}
		s := 0
		if r.SyncedLyrics != "" {
			s += 2
		}
		if q.Album != "" && strings.EqualFold(r.AlbumName, q.Album) {
			s++
		}
		if s > score {
			best, score = r, s
		}
	}
	return best, score >= 0
}

func (f *Finder) get(ctx context.Context, path string, v url.Values, into any) error {
	resp, err := f.send(ctx, path, v)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into)
	case http.StatusNotFound:
		return errMissing
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return busy{status: resp.Status, wait: after(resp.Header.Get("Retry-After"))}
	}
	return fmt.Errorf("lyrics: %s", resp.Status)
}

func (f *Finder) send(ctx context.Context, path string, v url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.o.Base+path+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if f.o.Agent != "" {
		req.Header.Set("User-Agent", f.o.Agent)
	}
	return f.o.Client.Do(req)
}

type busy struct {
	status string
	wait   time.Duration
}

func (b busy) Error() string { return "lyrics: " + b.status }

func after(header string) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	return time.Second
}

func backoff(fails int, err error) time.Duration {
	wait := steps[min(fails, len(steps))-1]
	if b, ok := errors.AsType[busy](err); ok {
		wait = max(wait, b.wait)
	}
	return wait + rand.N(wait*jitter/100+1)
}
