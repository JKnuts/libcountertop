package webapi

import (
	"bytes"
	"image/jpeg"
	"image/png"
	"log/slog"
	"mime"
	"net/http"
	"sync"
	"time"
)

const (
	screenFresh   = time.Second
	screenQuality = 80
)

type screenCache struct {
	mu    sync.Mutex
	taken time.Time
	jpeg  []byte
}

func (a *API) screen(w http.ResponseWriter, r *http.Request) {
	if a.o.Screen == nil {
		http.NotFound(w, r)
		return
	}
	a.shot.mu.Lock()
	defer a.shot.mu.Unlock()
	if now := a.o.Now(); now.Sub(a.shot.taken) >= screenFresh {
		img, err := a.o.Screen()
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		var b bytes.Buffer
		if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: screenQuality}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.shot.taken, a.shot.jpeg = now, b.Bytes()
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(a.shot.jpeg); err != nil {
		slog.Debug("writing the screen failed", "err", err)
	}
}

func (a *API) screenshot(w http.ResponseWriter, r *http.Request) {
	if a.o.Screen == nil {
		http.NotFound(w, r)
		return
	}
	img, err := a.o.Screen()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := a.o.Device().Name + " " + a.o.Now().Format("2006-01-02 15.04.05") + ".png"
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().
		Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if _, err := w.Write(b.Bytes()); err != nil {
		slog.Debug("writing the screenshot failed", "err", err)
	}
}
