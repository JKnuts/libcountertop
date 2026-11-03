package musicassistant

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

type fakeHA struct {
	mu     sync.Mutex
	asks   int
	err    error
	played []string
}

func (f *fakeHA) MusicQueue(_ context.Context, player string) ([]homeassistant.QueuedTrack, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asks++
	if f.err != nil {
		return nil, f.err
	}
	return []homeassistant.QueuedTrack{
		{ID: "q3", Title: "Three", Artist: "C", Duration: 187 * time.Second},
		{ID: "q4", Title: "Four", Artist: player},
	}, nil
}

func (f *fakeHA) MusicPlay(_ context.Context, player, item string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.played = append(f.played, player+"/"+item)
	return nil
}

func (f *fakeHA) state() (int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asks, append([]string(nil), f.played...)
}

var two = nowplaying.Now{Title: "Two", Artist: "B"}

func queue(ha *fakeHA) (*Queue, chan struct{}) {
	changed := make(chan struct{}, 4)
	return New(Options{
		HA:      ha,
		Player:  func() string { return "aabbccddeeff" },
		Changed: func() { changed <- struct{}{} },
	}), changed
}

func until(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheQueueFillsAndPlaysByItemID(t *testing.T) {
	ha := &fakeHA{}
	q, changed := queue(ha)
	if got := q.Ahead(two); got != nil {
		t.Fatalf("first Ahead = %v before asking", got)
	}
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("never changed")
	}
	got := q.Ahead(two)
	if len(got) != 2 || got[0].Title != "Three" || got[0].Length != 187*time.Second ||
		got[1].Artist != "aabbccddeeff" {
		t.Fatalf("queue = %+v", got)
	}
	got[1].Play()
	until(t, func() bool { _, played := ha.state(); return len(played) == 1 })
	if _, played := ha.state(); played[0] != "aabbccddeeff/q4" {
		t.Errorf("played %v", played)
	}
}

func TestWithoutTheIntegrationItWaitsBeforeAskingAgain(t *testing.T) {
	ha := &fakeHA{err: homeassistant.ErrNoMusic}
	q, _ := queue(ha)
	q.Ahead(two)
	until(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return !q.asking && q.fails == 1
	})
	q.mu.Lock()
	wait := time.Until(q.until)
	q.mu.Unlock()
	if wait < recheck-time.Second {
		t.Errorf("next try in %v, want about %v", wait, recheck)
	}
	q.Ahead(two)
	if asks, _ := ha.state(); asks != 1 {
		t.Errorf("asked %d times, want 1", asks)
	}
}
