package wpa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Settle = 20 * time.Second

var ErrRefused = errors.New("wpa: the network refused the password")

func Switch(ctx context.Context, c Conn, ssid, passphrase string) error {
	within, stop := context.WithTimeout(ctx, Settle)
	defer stop()
	return Move(within, c, ssid, passphrase, nil)
}

func Move(
	ctx context.Context,
	c Conn,
	ssid, passphrase string,
	ready func(context.Context) error,
) error {
	if ssid == "" {
		return fmt.Errorf("wpa: no network named")
	}

	was, err := Status(c)
	if err != nil {
		return err
	}
	if was.Joined(ssid) {
		return fmt.Errorf("wpa: already on %q", ssid)
	}

	id, err := Add(c, ssid, passphrase)
	if err != nil {
		return err
	}

	if _, err := c.Cmd("enable_network", id); err != nil {
		_, _ = c.Cmd("remove_network", id)
		return err
	}
	if _, err := c.Cmd("select_network", id); err != nil {
		_, _ = c.Cmd("remove_network", id)
		return err
	}

	// select_network disables every other network.
	if err := joining(ctx, c, ssid, id); err != nil {
		return back(c, id, was, err)
	}
	if ready != nil {
		if err := ready(ctx); err != nil {
			return back(c, id, was, err)
		}
	}

	return Only(c, id)
}

func joining(ctx context.Context, c Conn, ssid, id string) error {
	within, stop := context.WithTimeout(ctx, Settle)
	defer stop()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	var last State
	for {
		if s, err := Status(c); err == nil {
			last = s
			if s.Joined(ssid) {
				return nil
			}
		}
		if refused(c, id) {
			return ErrRefused
		}

		select {
		case <-within.Done():
			return fmt.Errorf("wpa: did not connect (%s): %w", last, within.Err())
		case <-tick.C:
		}
	}
}

// After failed authentication the supplicant flags the network [TEMP-DISABLED] in list_networks.
func refused(c Conn, id string) bool {
	out, err := c.Cmd("list_networks")
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) >= 4 && fields[0] == id {
			return strings.Contains(fields[3], "TEMP-DISABLED")
		}
	}
	return false
}

func back(c Conn, id string, was State, why error) error {
	_, _ = c.Cmd("remove_network", id)
	_, _ = c.Cmd("enable_network", "all")
	_, _ = c.Cmd("reassociate")

	if was.SSID == "" {
		return fmt.Errorf("wpa: did not join %w", why)
	}
	return fmt.Errorf("wpa: did not join, back on %q: %w", was.SSID, why)
}
