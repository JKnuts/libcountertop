package musicassistant

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
)

func TestMembersReadPythonListStrings(t *testing.T) {
	got := members("['media_player.athena_2', 'media_player.kitchen']")
	want := []string{"media_player.athena_2", "media_player.kitchen"}
	if !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if got := members(""); len(got) != 0 {
		t.Errorf("members of nothing = %v", got)
	}
}

type fakeClient struct {
	fakeHA
	mu        sync.Mutex
	asked     []string
	templates map[string]string
	performed []string
	data      map[string]string
}

func (f *fakeClient) MusicSearch(
	_ context.Context,
	query string,
) ([]homeassistant.MusicItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, query)
	return []homeassistant.MusicItem{
		{URI: "spotify://artist/1", Type: "artist", Name: "Eminem"},
	}, nil
}

func (f *fakeClient) MusicLibrary(
	_ context.Context,
	kind string,
) ([]homeassistant.MusicItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, kind)
	return nil, nil
}

func (f *fakeClient) Perform(
	_ context.Context,
	action string,
	data, templates map[string]string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.performed = append(f.performed, action)
	f.data, f.templates = data, templates
	return nil
}

func (f *fakeClient) MusicPlayers(
	context.Context,
	string,
) ([]homeassistant.MusicSpeaker, error) {
	return []homeassistant.MusicSpeaker{
		{Entity: "media_player.athena_2", Name: "athena", Mine: true},
	}, nil
}

func (f *fakeClient) MusicBrowse(context.Context, string) (homeassistant.MusicContents, error) {
	return homeassistant.MusicContents{}, nil
}

func (f *fakeClient) MusicRecent(context.Context) ([]homeassistant.MusicItem, error) {
	return nil, nil
}

func (f *fakeClient) MusicRecommendations(context.Context) ([]homeassistant.MusicFolder, error) {
	return nil, nil
}

func music(f *fakeClient) (*Music, chan struct{}) {
	changed := make(chan struct{}, 8)
	return NewMusic(MusicOptions{
		HA:      f,
		Player:  func() string { return "aabbccddeeff" },
		Changed: func() { changed <- struct{}{} },
	}), changed
}

func wait(t *testing.T, changed chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("never changed")
	}
}

func TestSearchAsksTheIntegrationWithATrimmedQuery(t *testing.T) {
	f := &fakeClient{}
	m, changed := music(f)
	if _, _, loaded := m.Search(" eminem "); loaded {
		t.Fatal("loaded before asking")
	}
	wait(t, changed)
	items, err, loaded := m.Search("eminem")
	if !loaded || err != nil || len(items) != 1 || items[0].Name != "Eminem" {
		t.Fatalf("search = %v, %v, %v", items, err, loaded)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.asked, []string{"eminem"}) {
		t.Errorf("asked %v", f.asked)
	}
}

func TestSpeakersComeFromTheIntegration(t *testing.T) {
	m, changed := music(&fakeClient{})
	if _, _, loaded := m.Speakers(); loaded {
		t.Fatal("loaded before asking")
	}
	wait(t, changed)
	list, err, _ := m.Speakers()
	if err != nil || len(list) != 1 || !list[0].Mine || list[0].Now().State != "" {
		t.Fatalf("speakers = %+v, %v", list, err)
	}
}

func TestJoinAndPlayShapeTheirData(t *testing.T) {
	f := &fakeClient{}
	m, _ := music(f)
	ctx := context.Background()
	if err := m.Join(
		ctx,
		"media_player.a",
		[]string{"media_player.b", "media_player.c"},
	); err != nil {
		t.Fatal(err)
	}
	if got := f.templates["group_members"]; got != `{{ ["media_player.b", "media_player.c"] }}` {
		t.Errorf("group_members = %q", got)
	}
	item := homeassistant.MusicItem{URI: "spotify://album/1", Type: "album"}
	if err := m.Play(ctx, "media_player.a", item, PlayNext, true); err != nil {
		t.Fatal(err)
	}
	if f.data["enqueue"] != "next" || f.data["media_id"] != item.URI ||
		f.templates["radio_mode"] != "{{ true }}" {
		t.Errorf("play data %v, templates %v", f.data, f.templates)
	}
	if err := m.Volume(ctx, "media_player.a", 0.425); err != nil {
		t.Fatal(err)
	}
	if f.templates["volume_level"] != "{{ 0.42 }}" && f.templates["volume_level"] != "{{ 0.43 }}" {
		t.Errorf("volume_level = %q", f.templates["volume_level"])
	}
}
