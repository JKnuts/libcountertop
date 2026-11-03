package homeassistant

import "testing"

func TestDecodeMusicTakesTheRenderedTemplateString(t *testing.T) {
	raw := []byte(
		`{"response":"{\"folders\":[{\"name\":\"Mixes\",\"icon\":\"mdi-playlist\",` +
			`\"items\":[{\"uri\":\"spotify://playlist/1\",\"type\":\"playlist\",\"name\":\"Daily\"}]}]}"}`,
	)
	var out struct {
		Folders []MusicFolder `json:"folders"`
	}
	if err := decodeMusic(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Folders) != 1 || out.Folders[0].Items[0].URI != "spotify://playlist/1" {
		t.Errorf("folders = %+v", out.Folders)
	}
}

func TestDecodeMusicTakesAPlainObject(t *testing.T) {
	var out struct {
		Players []MusicSpeaker `json:"players"`
	}
	raw := []byte(
		`{"response":{"players":[{"entity_id":"media_player.athena_2","name":"athena","mine":true}]}}`,
	)
	if err := decodeMusic(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Players) != 1 || !out.Players[0].Mine ||
		out.Players[0].Entity != "media_player.athena_2" {
		t.Errorf("players = %+v", out.Players)
	}
}
