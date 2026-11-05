package schema

import "encoding/json"

type Cast struct {
	Receiver bool `json:"receiver"`

	Oracle string `json:"oracle,omitempty"`
	Bundle string `json:"bundle,omitempty"`

	YouTube YouTube `json:"youtube,omitzero"`
}

type YouTube struct {
	Skip []string `json:"skip"`
}

func DefaultCast() Cast {
	return Cast{Receiver: false, YouTube: YouTube{Skip: []string{"sponsor"}}}
}

type Lounge struct {
	Device string `json:"device,omitempty"`
	Music  string `json:"music,omitempty"`
	Video  string `json:"video,omitempty"`

	OnDemand bool `json:"onDemand,omitempty"`
}

func MigrateLounge(data []byte, lounge *Lounge) error {
	var old struct {
		Lounge *json.RawMessage `json:"lounge"`
		Cast   struct {
			YouTube Lounge `json:"youtube"`
		} `json:"cast"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return err
	}
	if old.Lounge == nil {
		*lounge = old.Cast.YouTube
	}
	return nil
}
