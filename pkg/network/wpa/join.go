package wpa

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

func Scanned(ctx context.Context, c Conn) ([]Network, error) {
	// FAIL-BUSY: the supplicant is already scanning.
	if out, err := c.Cmd("scan"); err != nil && !strings.Contains(out, "FAIL-BUSY") {
		return nil, err
	}

	var best map[string]Network
	for range 4 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		out, err := c.Cmd("scan_results")
		if err != nil {
			return nil, err
		}

		found := ParseScan(out)
		if len(found) > len(best) {
			best = found
			continue
		}
		if len(best) > 0 {
			break
		}
	}

	return sorted(best), nil
}

func Results(c Conn) ([]Network, error) {
	out, err := c.Cmd("scan_results")
	if err != nil {
		return nil, err
	}
	return sorted(ParseScan(out)), nil
}

func sorted(found map[string]Network) []Network {
	networks := make([]Network, 0, len(found))
	for _, n := range found {
		networks = append(networks, n)
	}
	slices.SortFunc(networks, func(a, b Network) int { return b.Signal - a.Signal })
	return networks
}

func Configured(c Conn) ([]string, error) {
	out, err := c.Cmd("list_networks")
	if err != nil {
		return nil, err
	}
	return ParseNetworks(out), nil
}

func Remove(c Conn, ssid string) error {
	out, err := c.Cmd("list_networks")
	if err != nil {
		return err
	}

	ids := IDs(out, ssid)
	for _, id := range ids {
		if _, err := c.Cmd("remove_network", id); err != nil {
			return err
		}
	}

	if len(ids) == 0 {
		return nil
	}
	_, err = c.Cmd("save_config")
	return err
}

// scan_ssid=1 is required to join a hidden network.
func Add(c Conn, ssid, passphrase string) (string, error) {
	out, err := c.Cmd("add_network")
	if err != nil {
		return "", err
	}

	id := strings.TrimSpace(LastLine(out))
	if _, err := strconv.Atoi(id); err != nil {
		return "", fmt.Errorf("wpa: add_network said %q", strings.TrimSpace(out))
	}

	// wpa_supplicant takes ssid and psk as unquoted hex.
	set := [][]string{{"ssid", hex.EncodeToString([]byte(ssid))}, {"scan_ssid", "1"}}
	switch passphrase {
	case "":
		set = append(set, []string{"key_mgmt", "NONE"})
	default:
		psk, err := Key(ssid, passphrase)
		if err != nil {
			_, _ = c.Cmd("remove_network", id)
			return "", err
		}
		set = append(set, []string{"psk", psk})
	}

	for _, kv := range set {
		if _, err := c.Cmd("set_network", id, kv[0], kv[1]); err != nil {
			_, _ = c.Cmd("remove_network", id)
			return "", err
		}
	}
	return id, nil
}

func Only(c Conn, id string) error {
	if id == "" {
		return fmt.Errorf("wpa: no network to keep")
	}
	out, err := c.Cmd("list_networks")
	if err != nil {
		return err
	}

	for line := range strings.SplitSeq(out, "\n") {
		other, _, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if _, err := strconv.Atoi(other); err != nil || other == id {
			continue
		}
		if _, err := c.Cmd("remove_network", other); err != nil {
			return err
		}
	}

	_, err = c.Cmd("save_config")
	return err
}

// RECONNECT only acts after DISCONNECT and answers OK either way.
func Reconnect(c Conn) error {
	for _, cmd := range [][]string{{"enable_network", "all"}, {"reassociate"}} {
		if _, err := c.Cmd(cmd...); err != nil {
			return err
		}
	}
	return nil
}

func WPS(c Conn) error {
	if _, err := c.Cmd("wps_pbc"); err != nil {
		return err
	}
	_, err := c.Cmd("save_config")
	return err
}

func Status(c Conn) (State, error) {
	out, err := c.Cmd("status")
	if err != nil {
		return State{}, err
	}
	return ParseStatus(out), nil
}

func Wait(ctx context.Context, c Conn, ssid string) (State, error) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var last State
	for {
		s, err := Status(c)
		if err == nil {
			last = s
			if s.Joined(ssid) {
				return s, nil
			}
		}

		select {
		case <-ctx.Done():
			return last, fmt.Errorf("wpa: did not connect (%s): %w", last, ctx.Err())
		case <-tick.C:
		}
	}
}
