package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/wifi"
	"github.com/ygelfand/libcountertop/pkg/network/wpa"
)

const wifiScanMost = 20 * time.Second

type Wifi interface {
	Network() string
	Scan(ctx context.Context) ([]wpa.Network, error)
	Start(ssid, passphrase string) error
	State() wifi.MoveState
}

type wifiReply struct {
	Network string    `json:"network"`
	Move    *wifiMove `json:"move,omitempty"`
	Now     time.Time `json:"now"`
}

type wifiMove struct {
	SSID   string    `json:"ssid"`
	Until  time.Time `json:"until"`
	Moving bool      `json:"moving"`
	Failed string    `json:"failed,omitempty"`
}

type wifiNetwork struct {
	SSID      string `json:"ssid"`
	Signal    int    `json:"signal"`
	Security  string `json:"security"`
	Supported bool   `json:"supported"`
}

func (a *API) wifiState(w http.ResponseWriter, r *http.Request) {
	if a.o.Wifi == nil {
		http.NotFound(w, r)
		return
	}
	out := wifiReply{Network: a.o.Wifi.Network(), Now: a.o.Now()}
	if s := a.o.Wifi.State(); s.SSID != "" {
		out.Move = &wifiMove{SSID: s.SSID, Until: s.Until, Moving: s.Moving, Failed: s.Failed}
	}
	reply(w, out)
}

func (a *API) wifiScan(w http.ResponseWriter, r *http.Request) {
	if a.o.Wifi == nil {
		http.NotFound(w, r)
		return
	}
	ctx, stop := context.WithTimeout(r.Context(), wifiScanMost)
	defer stop()
	found, err := a.o.Wifi.Scan(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	out := make([]wifiNetwork, 0, len(found))
	for _, n := range found {
		if n.SSID == "" {
			continue
		}
		out = append(out, wifiNetwork{
			SSID:      n.SSID,
			Signal:    n.Signal,
			Security:  n.Security.String(),
			Supported: n.Security.Supported(),
		})
	}
	reply(w, out)
}

func (a *API) wifiMove(w http.ResponseWriter, r *http.Request) {
	if a.o.Wifi == nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		SSID     string `json:"ssid"`
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, loginMost)).Decode(&body) != nil ||
		body.SSID == "" {
		http.Error(w, "name a network", http.StatusBadRequest)
		return
	}
	err := a.o.Wifi.Start(body.SSID, body.Password)
	switch {
	case errors.Is(err, wifi.ErrMoving):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
