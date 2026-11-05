package musicassistant

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

type FavoriteHA interface {
	MusicFavorite(ctx context.Context, player string) (homeassistant.Favorite, error)
	SetMusicFavorite(ctx context.Context, player string, on bool) (homeassistant.Favorite, error)
}

type LikesOptions struct {
	HA      FavoriteHA
	Player  func() string
	Changed func()
}

type Likes struct {
	o LikesOptions

	mu     sync.Mutex
	key    key
	known  bool
	liked  bool
	asking bool
	fails  int
	until  time.Time
}

func NewLikes(o LikesOptions) *Likes { return &Likes{o: o} }

func (l *Likes) Liked(now nowplaying.Now) (liked, known bool) {
	if now.Title == "" {
		return false, false
	}
	k := key{now.Title, now.Artist, now.Album}
	l.mu.Lock()
	if k != l.key {
		l.key, l.known, l.liked, l.asking = k, false, false, false
	}
	ask := !l.known && !l.asking && time.Now().After(l.until)
	if ask {
		l.asking = true
	}
	liked, known = l.liked, l.known
	l.mu.Unlock()
	if ask {
		safe.Go("music assistant favorite", func() { l.ask(k, nil) })
	}
	return liked, known
}

func (l *Likes) SetLiked(now nowplaying.Now, on bool) {
	k := key{now.Title, now.Artist, now.Album}
	l.mu.Lock()
	if k != l.key {
		l.mu.Unlock()
		return
	}
	l.liked, l.asking = on, true
	l.mu.Unlock()
	l.changed()
	safe.Go("music assistant favorite", func() { l.ask(k, &on) })
}

func (l *Likes) ask(k key, set *bool) {
	ctx, cancel := context.WithTimeout(context.Background(), askTime)
	defer cancel()
	var (
		got homeassistant.Favorite
		err error
	)
	if set != nil {
		got, err = l.o.HA.SetMusicFavorite(ctx, l.o.Player(), *set)
	} else {
		got, err = l.o.HA.MusicFavorite(ctx, l.o.Player())
	}
	l.mu.Lock()
	if k != l.key {
		l.mu.Unlock()
		return
	}
	l.asking = false
	if err != nil {
		l.fails++
		l.until = time.Now().Add(backoff(l.fails, err))
		l.mu.Unlock()
		slog.Debug("music assistant favorite", "title", k.title, "err", err)
		l.changed()
		return
	}
	l.fails, l.until = 0, time.Time{}
	l.known, l.liked = got.Known, got.Favorite
	l.mu.Unlock()
	l.changed()
}

func (l *Likes) changed() {
	if l.o.Changed != nil {
		l.o.Changed()
	}
}
