package lyrics

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseLRCSortsRepeatsAndOffset(t *testing.T) {
	got := ParseLRC(
		"[ti:Song]\n[offset:500]\n[00:12.50]second\n[00:01.00][00:20.00]chorus\n[00:05]\nnot a line\n",
	)
	want := []Line{
		{At: 500 * time.Millisecond, Text: "chorus"},
		{At: 4500 * time.Millisecond},
		{At: 12 * time.Second, Text: "second"},
		{At: 19500 * time.Millisecond, Text: "chorus"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseLRCKeepsInstrumentalGaps(t *testing.T) {
	got := ParseLRC("[00:01.00]one\n[00:04.00]\n[00:09.00]two")
	if len(got) != 3 || got[1].Text != "" || got[1].At != 4*time.Second {
		t.Errorf("got %v", got)
	}
}

func TestCurrentLine(t *testing.T) {
	s := Sheet{
		Synced: true,
		Lines:  []Line{{At: time.Second}, {At: 5 * time.Second}, {At: 9 * time.Second}},
	}
	for _, c := range []struct {
		at   time.Duration
		want int
	}{{0, -1}, {time.Second, 0}, {6 * time.Second, 1}, {time.Minute, 2}} {
		if got := s.Current(c.at); got != c.want {
			t.Errorf("Current(%v) = %d, want %d", c.at, got, c.want)
		}
	}
	if got := (Sheet{Lines: s.Lines}).Current(6 * time.Second); got != -1 {
		t.Errorf("plain sheet Current = %d, want -1", got)
	}
}

var godzilla = Query{
	Title:  "Godzilla",
	Artist: "Eminem",
	Album:  "Music To Be Murdered By",
	Length: 211 * time.Second,
}

func found(t *testing.T, f *Finder, changed <-chan struct{}, q Query) Sheet {
	t.Helper()
	if s := f.Find(q); !s.Empty() {
		t.Fatalf("first Find returned %v before asking", s)
	}
	select {
	case <-changed:
	case <-time.After(10 * time.Second):
		t.Fatal("never changed")
	}
	return f.Find(q)
}

func finder(srv *httptest.Server) (*Finder, chan struct{}) {
	changed := make(chan struct{}, 1)
	return New(Options{
		Client:  srv.Client(),
		Base:    srv.URL,
		Agent:   "agent",
		Changed: func() { changed <- struct{}{} },
	}), changed
}

func TestFindSearchesTitleAndArtistAndPicksByLength(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.Header.Get("User-Agent") != "agent" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte(`[
			{"duration":313,"albumName":"Music To Be Murdered By","syncedLyrics":"[00:01.00]wrong length"},
			{"duration":211,"albumName":"Top Hits 2020","plainLyrics":"plain only"},
			{"duration":212,"albumName":"Top Hits 2020","syncedLyrics":"[00:01.00]right one"}
		]`))
	}))
	defer srv.Close()
	f, changed := finder(srv)
	got := found(t, f, changed, godzilla)
	if !got.Synced || len(got.Lines) != 1 || got.Lines[0].Text != "right one" {
		t.Errorf("got %v", got)
	}
	want := []string{"/api/search?artist_name=Eminem&track_name=Godzilla"}
	if !slices.Equal(paths, want) {
		t.Errorf("asked %v, want %v", paths, want)
	}
}

func TestFindPrefersOurAlbum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"duration":211,"albumName":"Top Hits 2020","syncedLyrics":"[00:01.00]compilation"},
			{"duration":210,"albumName":"music to be murdered by","syncedLyrics":"[00:01.00]album"}
		]`))
	}))
	defer srv.Close()
	f, changed := finder(srv)
	if got := found(t, f, changed, godzilla); got.Lines[0].Text != "album" {
		t.Errorf("got %v", got)
	}
}

func TestFindFallsBackToAlbumSearchThenExact(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/search":
			_, _ = w.Write([]byte(`[{"duration":313,"syncedLyrics":"[00:01.00]wrong length"}]`))
		case "/api/get":
			if q := r.URL.Query(); q.Get("duration") != "211" ||
				q.Get("album_name") != godzilla.Album {
				t.Errorf("asked %v", q)
			}
			_, _ = w.Write([]byte(`{"duration":211,"syncedLyrics":"[00:01.00]exact"}`))
		}
	}))
	defer srv.Close()
	f, changed := finder(srv)
	if got := found(t, f, changed, godzilla); got.Lines[0].Text != "exact" {
		t.Errorf("got %v", got)
	}
	want := []string{"/api/search", "/api/search", "/api/get"}
	if !slices.Equal(paths, want) {
		t.Errorf("asked %v, want %v", paths, want)
	}
}

func TestOverloadBacksOffWithoutAskingAgain(t *testing.T) {
	var asks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asks.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	f, _ := finder(srv)
	f.Find(godzilla)
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		e := f.known[godzilla]
		f.mu.Unlock()
		if !e.asking {
			if e.fails != 1 || time.Until(e.until) < 4*time.Second {
				t.Fatalf("fails %d, next try in %v", e.fails, time.Until(e.until))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never finished asking")
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.Find(godzilla)
	if n := asks.Load(); n != 1 {
		t.Errorf("asked %d times, want 1 until the backoff passes", n)
	}
}

func TestBackoffGrowsCapsAndHonorsRetryAfter(t *testing.T) {
	for _, c := range []struct {
		fails    int
		err      error
		min, max time.Duration
	}{
		{1, nil, 5 * time.Second, 6500 * time.Millisecond},
		{2, nil, 15 * time.Second, 19500 * time.Millisecond},
		{9, nil, 2 * time.Minute, 156 * time.Second},
		{1, busy{wait: 20 * time.Second}, 20 * time.Second, 26 * time.Second},
	} {
		if got := backoff(c.fails, c.err); got < c.min || got > c.max {
			t.Errorf("backoff(%d, %v) = %v, want %v to %v", c.fails, c.err, got, c.min, c.max)
		}
	}
}

func TestPickPrefersSyncedOverOurAlbum(t *testing.T) {
	r, ok := pick([]record{
		{Duration: 264, AlbumName: "Whitey Ford Sings the Blues", PlainLyrics: "plain"},
		{Duration: 264, AlbumName: "The Mod Squad", SyncedLyrics: "[00:01.00]synced"},
	}, Query{Album: "Whitey Ford Sings the Blues", Length: 264 * time.Second})
	if !ok || r.SyncedLyrics == "" {
		t.Errorf("picked %v", r)
	}
}

func TestPickSkipsInstrumental(t *testing.T) {
	_, ok := pick(
		[]record{{Duration: 100, Instrumental: true, SyncedLyrics: "[00:01.00]x"}},
		Query{Length: 100 * time.Second},
	)
	if ok {
		t.Error("picked an instrumental")
	}
}
