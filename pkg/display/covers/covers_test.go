package covers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"
)

func TestABiggerAskGetsANewImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 640, 360))); err != nil {
		t.Fatal(err)
	}
	c := New(Options{
		Shape: Wide,
		Get:   func(context.Context, string) ([]byte, error) { return buf.Bytes(), nil },
	})
	settle := func(side int, not string) string {
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if src := c.Src("https://example.com/frame", side); src != "" && src != not {
				return src
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("no new picture at %d", side)
		return ""
	}
	small := settle(100, "")
	large := settle(400, small)
	if w, _, _, ok := gogui.LookupImage(large); !ok || w != 400 {
		t.Errorf("the larger picture is %d wide (%v), want 400", w, ok)
	}
	if gogui.HasImage(strings.TrimPrefix(small, "mem:")) {
		t.Error("the smaller picture was kept")
	}
}

func TestOnlyAFewPicturesDownloadAtOnce(t *testing.T) {
	var (
		running, most atomic.Int32
		release       = make(chan struct{})
		done          sync.WaitGroup
	)
	c := New(Options{Get: func(context.Context, string) ([]byte, error) {
		defer done.Done()
		n := running.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		<-release
		running.Add(-1)
		return nil, errors.New("not a picture")
	}})
	done.Add(parallel)
	for i := range 3 * parallel {
		c.Src(fmt.Sprintf("https://example.com/%d", i), 64)
	}
	deadline := time.Now().Add(time.Second)
	for running.Load() < parallel && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	done.Wait()
	if got := most.Load(); got != parallel {
		t.Errorf("%d downloads ran at once, want %d", got, parallel)
	}
}
