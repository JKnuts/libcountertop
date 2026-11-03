package musicapp

import (
	"context"
	"errors"
	"image"
	"log/slog"
	"sync"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"
	xdraw "golang.org/x/image/draw"

	"github.com/ygelfand/libcountertop/pkg/display/picture"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const (
	keepCovers = 96
	coverRetry = time.Minute
)

type covers struct {
	get    func(ctx context.Context, url string) ([]byte, error)
	redraw func()

	mu    sync.Mutex
	byURL map[string]*cover
	order []string
}

type cover struct {
	raw    []byte
	src    string
	side   int
	busy   bool
	failed time.Time
}

func newCovers(get func(context.Context, string) ([]byte, error), redraw func()) *covers {
	return &covers{get: get, redraw: redraw, byURL: map[string]*cover{}}
}

func (c *covers) src(url string, side int) string {
	if url == "" || side <= 0 {
		return ""
	}
	c.mu.Lock()
	k := c.byURL[url]
	if k == nil {
		k = &cover{}
		c.add(url, k)
	}
	if k.src != "" && !gogui.HasImage(coverKey(url)) {
		k.src, k.side = "", 0
	}
	due := !k.busy &&
		(k.raw == nil && time.Since(k.failed) >= coverRetry || k.raw != nil && side > k.side)
	if due {
		k.busy = true
	}
	src, raw := k.src, k.raw
	c.mu.Unlock()
	if due {
		safe.Go("music cover", func() { c.load(url, k, raw, side) })
	}
	return src
}

func (c *covers) add(url string, k *cover) {
	if len(c.order) >= keepCovers {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.byURL, old)
		gogui.DropImage(coverKey(old))
	}
	c.byURL[url] = k
	c.order = append(c.order, url)
}

func (c *covers) load(url string, k *cover, raw []byte, side int) {
	var err error
	if raw == nil {
		ctx, cancel := context.WithTimeout(context.Background(), askTime)
		raw, err = c.get(ctx, url)
		cancel()
	}
	src := ""
	if err == nil {
		src, err = register(url, raw, side)
	}
	c.mu.Lock()
	k.busy = false
	if err == nil {
		k.raw, k.src, k.side = raw, src, side
	} else {
		k.failed = time.Now()
	}
	c.mu.Unlock()
	if err != nil {
		slog.Debug("music cover", "url", url, "err", err)
		return
	}
	c.redraw()
}

func register(url string, raw []byte, side int) (string, error) {
	img, err := picture.Decode(raw, image.Point{X: side, Y: side}, picture.Smooth)
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	edge := min(b.Dx(), b.Dy())
	if edge <= 0 {
		return "", errors.New("musicapp: empty cover")
	}
	crop := image.Rect(0, 0, edge, edge).Add(b.Min).
		Add(image.Pt((b.Dx()-edge)/2, (b.Dy()-edge)/2))
	side = min(side, edge)
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	xdraw.ApproxBiLinear.Scale(out, out.Rect, img, crop, xdraw.Src, nil)
	return gogui.UseImage(coverKey(url), side, side, out.Pix), nil
}

func coverKey(url string) string { return "musicapp/" + url }
