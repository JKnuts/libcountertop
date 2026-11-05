package musicassistant

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

type fakeFavorites struct {
	mu    sync.Mutex
	liked bool
	err   error
	reads int
	sets  []bool
}

func (f *fakeFavorites) MusicFavorite(context.Context, string) (homeassistant.Favorite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return homeassistant.Favorite{Known: true, Favorite: f.liked}, f.err
}

func (f *fakeFavorites) SetMusicFavorite(
	_ context.Context,
	_ string,
	on bool,
) (homeassistant.Favorite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, on)
	f.liked = on
	return homeassistant.Favorite{Known: true, Favorite: on}, f.err
}

func likes(ha FavoriteHA) (*Likes, chan struct{}) {
	changed := make(chan struct{}, 8)
	return NewLikes(LikesOptions{
		HA:      ha,
		Player:  func() string { return "p1" },
		Changed: func() { changed <- struct{}{} },
	}), changed
}

func settle(t *testing.T, changed chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("no answer from the fake Home Assistant")
	}
}

var song = nowplaying.Now{Title: "Song", Artist: "Band"}

func TestALikedTrackIsReadOncePerTrack(t *testing.T) {
	ha := &fakeFavorites{liked: true}
	l, changed := likes(ha)
	if _, known := l.Liked(song); known {
		t.Fatal("known before Home Assistant answered")
	}
	settle(t, changed)
	if liked, known := l.Liked(song); !liked || !known {
		t.Fatalf("liked=%v known=%v, want both", liked, known)
	}
	l.Liked(song)
	if ha.reads != 1 {
		t.Errorf("read %d times for one track", ha.reads)
	}
}

func TestLikingShowsAtOnceAndIsSent(t *testing.T) {
	ha := &fakeFavorites{}
	l, changed := likes(ha)
	l.Liked(song)
	settle(t, changed)
	l.SetLiked(song, true)
	if liked, _ := l.Liked(song); !liked {
		t.Error("the tap did not show before the answer")
	}
	settle(t, changed)
	settle(t, changed)
	if len(ha.sets) != 1 || !ha.sets[0] {
		t.Errorf("sent %v, want one like", ha.sets)
	}
}

func TestAFailedReadWaitsBeforeAskingAgain(t *testing.T) {
	ha := &fakeFavorites{err: errors.New("offline")}
	l, changed := likes(ha)
	l.Liked(song)
	settle(t, changed)
	l.Liked(song)
	l.Liked(song)
	if ha.reads != 1 {
		t.Errorf("asked %d times right after a failure", ha.reads)
	}
}
