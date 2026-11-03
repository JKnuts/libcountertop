package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	MusicProtocol = 5

	musicBrowseAction          = "echolocal.music_browse"
	musicLibraryAction         = "echolocal.music_library"
	musicSearchAction          = "echolocal.music_search"
	musicPlayersAction         = "echolocal.music_players"
	musicQueueAction           = "echolocal.music_queue"
	musicPlayAction            = "echolocal.music_play"
	musicRecentAction          = "echolocal.music_recent"
	musicRecommendationsAction = "echolocal.music_recommendations"
)

var ErrNoMusic = errors.New(
	"homeassistant: no connected Home Assistant reaches Music Assistant for this device",
)

type QueuedTrack struct {
	ID       string
	Title    string
	Artist   string
	Image    string
	Duration time.Duration
}

type MusicSpeaker struct {
	Entity string   `json:"entity_id"`
	Name   string   `json:"name"`
	Mine   bool     `json:"mine"`
	IDs    []string `json:"ids"`
}

type MusicItem struct {
	URI    string `json:"uri"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Artist string `json:"artist"`
	Image  string `json:"image"`
}

type MusicFolder struct {
	Name  string      `json:"name"`
	Icon  string      `json:"icon"`
	Items []MusicItem `json:"items"`
}

type MusicContents struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Items []MusicItem `json:"items"`
}

func (h *Controller) MusicBrowse(ctx context.Context, uri string) (MusicContents, error) {
	var out MusicContents
	err := h.music(ctx, musicBrowseAction, map[string]string{"uri": uri}, &out)
	return out, err
}

func (h *Controller) MusicSearch(ctx context.Context, query string) ([]MusicItem, error) {
	return h.musicItems(ctx, musicSearchAction, map[string]string{"query": query})
}

func (h *Controller) MusicLibrary(ctx context.Context, kind string) ([]MusicItem, error) {
	return h.musicItems(ctx, musicLibraryAction, map[string]string{"media_type": kind})
}

func (h *Controller) MusicPlayers(ctx context.Context, player string) ([]MusicSpeaker, error) {
	var out struct {
		Players []MusicSpeaker `json:"players"`
	}
	err := h.music(ctx, musicPlayersAction, map[string]string{"player": player}, &out)
	return out.Players, err
}

func (h *Controller) MusicQueue(ctx context.Context, player string) ([]QueuedTrack, error) {
	var out struct {
		Items []struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Artist   string  `json:"artist"`
			Image    string  `json:"image"`
			Duration float64 `json:"duration"`
		} `json:"items"`
	}
	if err := h.music(
		ctx,
		musicQueueAction,
		map[string]string{"player": player},
		&out,
	); err != nil {
		return nil, err
	}
	tracks := make([]QueuedTrack, 0, len(out.Items))
	for _, it := range out.Items {
		tracks = append(tracks, QueuedTrack{
			ID:       it.ID,
			Title:    it.Title,
			Artist:   it.Artist,
			Image:    it.Image,
			Duration: time.Duration(it.Duration * float64(time.Second)),
		})
	}
	return tracks, nil
}

func (h *Controller) MusicPlay(ctx context.Context, player, item string) error {
	return h.music(ctx, musicPlayAction, map[string]string{
		"player":        player,
		"queue_item_id": item,
	}, nil)
}

func (h *Controller) MusicRecent(ctx context.Context) ([]MusicItem, error) {
	return h.musicItems(ctx, musicRecentAction, nil)
}

func (h *Controller) musicItems(
	ctx context.Context,
	action string,
	data map[string]string,
) ([]MusicItem, error) {
	var out struct {
		Items []MusicItem `json:"items"`
	}
	err := h.music(ctx, action, data, &out)
	return out.Items, err
}

func (h *Controller) MusicRecommendations(ctx context.Context) ([]MusicFolder, error) {
	var out struct {
		Folders []MusicFolder `json:"folders"`
	}
	err := h.music(ctx, musicRecommendationsAction, nil, &out)
	return out.Folders, err
}

func (h *Controller) music(
	ctx context.Context,
	action string,
	data map[string]string,
	into any,
) error {
	conn, ok := h.speaker(MusicProtocol)
	if !ok {
		return ErrNoMusic
	}
	tmpl := ""
	if into != nil {
		tmpl = pingReply
	}
	raw, err := h.send(ctx, []sender{conn}, request(action, data, nil, tmpl))
	if err != nil || into == nil {
		return err
	}
	return decodeMusic(raw, into)
}

func decodeMusic(raw []byte, into any) error {
	var out struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("homeassistant: the music response: %w", err)
	}
	if err := json.Unmarshal(unwrap(out.Response), into); err != nil {
		return fmt.Errorf("homeassistant: the music answer: %w", err)
	}
	return nil
}
