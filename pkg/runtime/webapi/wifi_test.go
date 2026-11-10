package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/network/wifi"
	"github.com/ygelfand/libcountertop/pkg/network/wpa"
)

type radio struct {
	started []string
	moving  bool
}

func (*radio) Network() string { return "home" }
func (*radio) Scan(context.Context) ([]wpa.Network, error) {
	return []wpa.Network{
		{SSID: "cabin", Signal: -50, Security: wpa.PSK},
		{SSID: "", Signal: -40, Security: wpa.Open},
		{SSID: "office", Signal: -70, Security: wpa.Enterprise},
	}, nil
}
func (r *radio) Start(ssid, passphrase string) error {
	if r.moving {
		return wifi.ErrMoving
	}
	r.started = append(r.started, ssid+"/"+passphrase)
	r.moving = true
	return nil
}
func (r *radio) State() wifi.MoveState {
	if !r.moving {
		return wifi.MoveState{}
	}
	return wifi.MoveState{SSID: "cabin", Moving: true}
}

func TestWifiMovesOnlyWithThePassword(t *testing.T) {
	a, settings := controlFixture(t)
	r := &radio{}
	a.o.Wifi = r
	pass := settings.v.Password

	w := controlRequest(a, http.MethodGet, "/wifi/networks", "", nil, pass)
	var found []wifiNetwork
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&found) != nil {
		t.Fatalf("networks = %d: %s", w.Code, w.Body.String())
	}
	if len(found) != 2 || found[0].SSID != "cabin" || !found[0].Supported || found[1].Supported {
		t.Errorf("networks %+v, want cabin (supported) and office (not), no hidden ones", found)
	}

	body := `{"ssid":"cabin","password":"hunter22"}`
	if w := controlRequest(
		a,
		http.MethodPost,
		"/wifi",
		body,
		nil,
		"wrong",
	); w.Code != http.StatusUnauthorized {
		t.Errorf("move with a wrong password = %d, want 401", w.Code)
	}
	if w := controlRequest(
		a,
		http.MethodPost,
		"/wifi",
		body,
		nil,
		pass,
	); w.Code != http.StatusNoContent {
		t.Fatalf("move = %d: %s", w.Code, w.Body.String())
	}
	if len(r.started) != 1 || r.started[0] != "cabin/hunter22" {
		t.Errorf("started %v, want one move to cabin with its password", r.started)
	}
	if w := controlRequest(
		a,
		http.MethodPost,
		"/wifi",
		body,
		nil,
		pass,
	); w.Code != http.StatusConflict {
		t.Errorf("a second move during the first = %d, want 409", w.Code)
	}

	w = controlRequest(a, http.MethodGet, "/wifi", "", nil, pass)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "hunter22") ||
		!strings.Contains(w.Body.String(), `"moving":true`) {
		t.Errorf("state = %d %s, want the move without the password", w.Code, w.Body.String())
	}

	if w := controlRequest(
		a,
		http.MethodPost,
		"/wifi",
		`{"ssid":""}`,
		nil,
		pass,
	); w.Code != http.StatusBadRequest {
		t.Errorf("move to no network = %d, want 400", w.Code)
	}
}

func TestWifiIsMissingWithoutARadio(t *testing.T) {
	a, settings := controlFixture(t)
	if w := controlRequest(
		a,
		http.MethodGet,
		"/wifi",
		"",
		nil,
		settings.v.Password,
	); w.Code != http.StatusNotFound {
		t.Errorf("wifi without a radio = %d, want 404", w.Code)
	}
}
