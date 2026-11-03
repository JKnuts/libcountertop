package musicassistant

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/states"
)

const (
	keepSlots   = 32
	browseFresh = 5 * time.Minute
	queueFresh  = 15 * time.Second
	homeFresh   = 2 * time.Minute

	playAction     = "music_assistant.play_media"
	transferAction = "music_assistant.transfer_queue"
	joinAction     = "media_player.join"
	unjoinAction   = "media_player.unjoin"
	volumeAction   = "media_player.volume_set"
)

type Enqueue string

const (
	PlayNow  Enqueue = "play"
	PlayNext Enqueue = "next"
	AddLast  Enqueue = "add"
)

type Shelf string

const (
	Playlists Shelf = "playlist"
	Albums    Shelf = "album"
	Artists   Shelf = "artist"
	Radio     Shelf = "radio"
)

type Client interface {
	Perform(ctx context.Context, action string, data, templates map[string]string) error
	MusicSearch(ctx context.Context, query string) ([]homeassistant.MusicItem, error)
	MusicLibrary(ctx context.Context, kind string) ([]homeassistant.MusicItem, error)
	MusicPlayers(ctx context.Context, player string) ([]homeassistant.MusicSpeaker, error)
	MusicQueue(ctx context.Context, player string) ([]homeassistant.QueuedTrack, error)
	MusicPlay(ctx context.Context, player, item string) error
	MusicRecent(ctx context.Context) ([]homeassistant.MusicItem, error)
	MusicRecommendations(ctx context.Context) ([]homeassistant.MusicFolder, error)
	MusicBrowse(ctx context.Context, uri string) (homeassistant.MusicContents, error)
}

type Follower interface {
	Follow(entity, attribute string) *states.Value
}

type MusicOptions struct {
	HA      Client
	States  Follower
	Player  func() string
	Changed func()
}

type Home struct {
	Recent  []homeassistant.MusicItem
	Folders []homeassistant.MusicFolder
}

type Music struct {
	o MusicOptions

	speakers *loader[[]*Speaker]
	queues   *loader[[]homeassistant.QueuedTrack]
	searches *loader[[]homeassistant.MusicItem]
	shelves  *loader[[]homeassistant.MusicItem]
	home     *loader[Home]
	contents *loader[homeassistant.MusicContents]
}

func NewMusic(o MusicOptions) *Music {
	m := &Music{o: o}
	m.speakers = newLoader("speakers", browseFresh, m.fetchSpeakers, o.Changed)
	m.queues = newLoader("queue", queueFresh, o.HA.MusicQueue, o.Changed)
	m.searches = newLoader("search", browseFresh, m.search, o.Changed)
	m.shelves = newLoader("library", browseFresh, o.HA.MusicLibrary, o.Changed)
	m.home = newLoader("home", homeFresh, m.fetchHome, o.Changed)
	m.contents = newLoader("browse", browseFresh, o.HA.MusicBrowse, o.Changed)
	return m
}

func (m *Music) Contents(uri string) (homeassistant.MusicContents, error, bool) {
	return m.contents.get(uri)
}

func (m *Music) Speakers() ([]*Speaker, error, bool) { return m.speakers.get("") }

func (m *Music) Queue(entity string) ([]homeassistant.QueuedTrack, error, bool) {
	return m.queues.get(entity)
}

func (m *Music) Search(query string) ([]homeassistant.MusicItem, error, bool) {
	return m.searches.get(strings.TrimSpace(query))
}

func (m *Music) Library(s Shelf) ([]homeassistant.MusicItem, error, bool) {
	return m.shelves.get(string(s))
}

func (m *Music) Home() (Home, error, bool) { return m.home.get("") }

func (m *Music) Refresh() {
	m.speakers.drop("")
	m.home.drop("")
}

func (m *Music) Jump(ctx context.Context, entity, item string) error {
	defer m.queues.drop(entity)
	return m.o.HA.MusicPlay(ctx, entity, item)
}

func (m *Music) Play(
	ctx context.Context,
	entity string,
	it homeassistant.MusicItem,
	how Enqueue,
	radio bool,
) error {
	defer m.queues.drop(entity)
	templates := map[string]string{}
	if radio {
		templates["radio_mode"] = "{{ true }}"
	}
	return m.o.HA.Perform(ctx, playAction, map[string]string{
		"entity_id":  entity,
		"media_id":   it.URI,
		"media_type": it.Type,
		"enqueue":    string(how),
	}, templates)
}

func (m *Music) Command(ctx context.Context, entity, action string) error {
	return m.o.HA.Perform(ctx, "media_player."+action, map[string]string{"entity_id": entity}, nil)
}

func (m *Music) Volume(ctx context.Context, entity string, level float64) error {
	return m.o.HA.Perform(ctx, volumeAction, map[string]string{"entity_id": entity},
		map[string]string{"volume_level": "{{ " + strconv.FormatFloat(level, 'f', 2, 64) + " }}"})
}

func (m *Music) Join(ctx context.Context, leader string, members []string) error {
	quoted := make([]string, len(members))
	for i, e := range members {
		quoted[i] = strconv.Quote(e)
	}
	return m.o.HA.Perform(ctx, joinAction, map[string]string{"entity_id": leader},
		map[string]string{"group_members": "{{ [" + strings.Join(quoted, ", ") + "] }}"})
}

func (m *Music) Unjoin(ctx context.Context, entity string) error {
	return m.o.HA.Perform(ctx, unjoinAction, map[string]string{"entity_id": entity}, nil)
}

func (m *Music) Transfer(ctx context.Context, from, to string) error {
	defer m.queues.drop(to)
	return m.o.HA.Perform(ctx, transferAction, map[string]string{
		"entity_id":     to,
		"source_player": from,
	}, nil)
}

func (m *Music) fetchSpeakers(ctx context.Context, _ string) ([]*Speaker, error) {
	found, err := m.o.HA.MusicPlayers(ctx, m.o.Player())
	if err != nil {
		return nil, err
	}
	out := make([]*Speaker, 0, len(found))
	for _, f := range found {
		out = append(out, m.speaker(f))
	}
	return out, nil
}

func (m *Music) speaker(f homeassistant.MusicSpeaker) *Speaker {
	s := &Speaker{Entity: f.Entity, Name: f.Name, Mine: f.Mine, IDs: f.IDs}
	if m.o.States == nil {
		return s
	}
	s.state = m.o.States.Follow(f.Entity, "")
	s.title = m.o.States.Follow(f.Entity, "media_title")
	s.artist = m.o.States.Follow(f.Entity, "media_artist")
	s.picture = m.o.States.Follow(f.Entity, "entity_picture")
	s.volume = m.o.States.Follow(f.Entity, "volume_level")
	s.members = m.o.States.Follow(f.Entity, "group_members")
	for _, v := range []*states.Value{s.state, s.title, s.artist, s.picture, s.volume, s.members} {
		v.Changed.Listen(func(string) { m.changed() })
	}
	return s
}

func (m *Music) fetchHome(ctx context.Context, _ string) (Home, error) {
	recent, err := m.o.HA.MusicRecent(ctx)
	if err != nil {
		return Home{}, err
	}
	folders, err := m.o.HA.MusicRecommendations(ctx)
	return Home{Recent: recent, Folders: folders}, err
}

func (m *Music) search(ctx context.Context, query string) ([]homeassistant.MusicItem, error) {
	if query == "" {
		return nil, nil
	}
	return m.o.HA.MusicSearch(ctx, query)
}

func (m *Music) changed() {
	if m.o.Changed != nil {
		m.o.Changed()
	}
}

type Speaker struct {
	Entity string
	Name   string
	Mine   bool
	IDs    []string

	state, title, artist, picture, volume, members *states.Value
}

type Playing struct {
	State   string
	Title   string
	Artist  string
	Picture string
	Volume  float64
	Members []string
}

func (p Playing) Active() bool { return p.State == "playing" || p.State == "paused" }

func (s *Speaker) Now() Playing {
	if s.state == nil {
		return Playing{}
	}
	p := Playing{}
	p.State, _ = s.state.Get()
	p.Title, _ = s.title.Get()
	p.Artist, _ = s.artist.Get()
	p.Picture, _ = s.picture.Get()
	p.Volume, _ = s.volume.Float()
	raw, _ := s.members.Get()
	p.Members = members(raw)
	return p
}

var quotedEntity = regexp.MustCompile(`media_player\.[a-z0-9_]+`)

func members(raw string) []string { return quotedEntity.FindAllString(raw, -1) }
