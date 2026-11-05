package yttv

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type fakeHA struct {
	mu        sync.Mutex
	pages     map[string]string
	asked     []string
	performed []map[string]string
	err       error
}

func (f *fakeHA) Ask(
	_ context.Context,
	action string,
	data, _ map[string]string,
	_ string,
) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, action+" "+data["start"])
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(f.pages[data["start"]]), nil
}

func (f *fakeHA) Perform(_ context.Context, action string, data, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data["action"] = action
	f.performed = append(f.performed, data)
	return f.err
}

func TestTheGuideJoinsEveryPage(t *testing.T) {
	ha := &fakeHA{pages: map[string]string{
		"0": `{"version":1,"next":2,"channels":[{"name":"ESPN"},{"name":"CNN"}],"tvs":[{"entity_id":"media_player.yttv_den","uuid":"AB-CD"}]}`,
		"2": `{"version":1,"next":null,"channels":[{"name":"PBS"}]}`,
	}}
	g := New(Options{HA: ha})
	channels, tvs, err := g.pages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 3 || channels[2].Name != "PBS" {
		t.Errorf("channels %+v", channels)
	}
	if len(tvs) != 1 || tvs[0].Entity != "media_player.yttv_den" {
		t.Errorf("tvs %+v", tvs)
	}
}

func TestAGuidePageThatGoesBackIsRefused(t *testing.T) {
	ha := &fakeHA{pages: map[string]string{
		"0": `{"next":0,"channels":[{"name":"ESPN"}]}`,
	}}
	if _, _, err := New(Options{HA: ha}).pages(context.Background()); err == nil {
		t.Fatal("a page pointing back at itself was accepted")
	}
}

func TestAMissingGuideActionIsAnError(t *testing.T) {
	ha := &fakeHA{err: errors.New("action yttv.guide not found")}
	if _, _, err := New(Options{HA: ha}).pages(context.Background()); err == nil {
		t.Fatal("the error was swallowed")
	}
}

func TestThisPanelIsFoundByItsCastID(t *testing.T) {
	g := New(Options{Self: func() string { return "0123456789abcdef0123456789abcdef" }})
	if !g.Mine(TV{UUID: "01234567-89AB-CDEF-0123-456789ABCDEF"}) {
		t.Error("our own Chromecast uuid did not match")
	}
	if g.Mine(TV{UUID: "ffffffff-89ab-cdef-0123-456789abcdef"}) {
		t.Error("another Chromecast matched")
	}
	if New(Options{}).Mine(TV{UUID: ""}) {
		t.Error("an unknown identity matched")
	}
}

func TestProgressFollowsTheAiring(t *testing.T) {
	a := Airing{StartsAt: "2026-11-04T01:00:00Z", EndsAt: "2026-11-04T02:00:00Z"}
	at, _ := time.Parse(time.RFC3339, "2026-11-04T01:15:00Z")
	if got, ok := a.Progress(at); !ok || got != 0.25 {
		t.Errorf("progress %v %v, want 0.25", got, ok)
	}
	if _, ok := (Airing{}).Progress(at); ok {
		t.Error("an airing with no times had progress")
	}
}

func TestTheLiveFrameChangesEveryFiveMinutes(t *testing.T) {
	c := Channel{LiveThumb: "https://i.ytimg.com/vi/x/maxresdefault_live.jpg"}
	at, _ := time.Parse(time.RFC3339, "2026-11-04T01:00:10Z")
	first := c.LiveFrame(at)
	if first != c.LiveFrame(at.Add(4*time.Minute)) {
		t.Error("the frame changed inside one five-minute bucket")
	}
	if first == c.LiveFrame(at.Add(5*time.Minute)) {
		t.Error("the frame did not change after five minutes")
	}
	if (Channel{}).LiveFrame(at) != "" {
		t.Error("a channel with no live thumbnail got a frame")
	}
}

func TestWhatIsOnNowSkipsAnAiringThatEnded(t *testing.T) {
	c := Channel{
		Title: "Ended", StartsAt: "2026-11-04T00:00:00Z", EndsAt: "2026-11-04T01:00:00Z",
		Next: []Airing{{
			Title: "Later", StartsAt: "2026-11-04T01:00:00Z", EndsAt: "2026-11-04T02:00:00Z",
		}},
	}
	at, _ := time.Parse(time.RFC3339, "2026-11-04T01:15:00Z")
	if got, ok := c.At(at); !ok || got.Title != "Later" {
		t.Errorf("on now %q %v, want Later", got.Title, ok)
	}
	c.Next = nil
	if got, ok := c.At(at); ok {
		t.Errorf("an ended airing counted as on now: %q", got.Title)
	}
}

type memory struct{ tv schema.TV }

func (m *memory) Read() schema.TV { return m.tv }

func (m *memory) Update(change func(*schema.TV)) error {
	change(&m.tv)
	return nil
}

func TestFavoritesAreSavedOnceAndRemoved(t *testing.T) {
	g := New(Options{Settings: &memory{}})
	for _, step := range []struct {
		channel string
		on      bool
		want    string
	}{
		{"ESPN", true, "ESPN"},
		{"PBS", true, "ESPN PBS"},
		{"ESPN", true, "PBS ESPN"},
		{"PBS", false, "ESPN"},
	} {
		if err := g.SetFavorite(step.channel, step.on); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(g.Favorites(), " "); got != step.want {
			t.Errorf("after %s %v favorites %q, want %q", step.channel, step.on, got, step.want)
		}
	}
}

func TestCastPlaysTheChannelsOwnVideo(t *testing.T) {
	ha := &fakeHA{}
	g := New(Options{HA: ha})
	ctx := context.Background()
	if err := g.Cast(
		ctx,
		"media_player.yttv_den",
		Channel{Name: "ESPN", Video: "_4UIhtz0pF4"},
	); err != nil {
		t.Fatal(err)
	}
	if err := g.Cast(ctx, "media_player.yttv_den", Channel{Name: "ESPN"}); err != nil {
		t.Fatal(err)
	}
	byVideo, byName := ha.performed[0], ha.performed[1]
	if byVideo["action"] != "yttv.cast" || byVideo["device"] != "media_player.yttv_den" ||
		byVideo["video_id"] != "_4UIhtz0pF4" || byVideo["channel"] != "" {
		t.Errorf("cast with a video performed %v", byVideo)
	}
	if byName["channel"] != "ESPN" || byName["video_id"] != "" {
		t.Errorf("cast without a video performed %v", byName)
	}
}

func TestStationsWithOneNameHaveTheirOwnKeys(t *testing.T) {
	espn := Channel{Name: "ESPN", Station: "UCW7W_WAogi3qWDbO9PqOmZQ"}
	game := Channel{Name: "ESPN", Station: "UCqegy-6-vcKoba8wpBgcmDg"}
	if espn.Key() == game.Key() {
		t.Error("two ESPN stations share a key")
	}
	if (Channel{Name: "PBS"}).Key() != "PBS" {
		t.Error("a channel without a station is not keyed by its name")
	}
}
