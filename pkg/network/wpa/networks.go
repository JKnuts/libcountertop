package wpa

import (
	"context"
	"fmt"
	"strings"
)

type Connection interface {
	Cmd(string) (string, error)
	Close() error
}
type NetworksClient struct{ Dial func() (Connection, error) }

// The control interface takes commands uppercase; wpa_cli uppercases them.
type Commands struct{ Connection }

func (s Commands) Cmd(words ...string) (string, error) {
	if len(words) == 0 {
		return "", fmt.Errorf("wifi: no command")
	}

	cmd := strings.ToUpper(words[0])
	if len(words) > 1 {
		cmd += " " + strings.Join(words[1:], " ")
	}

	out, err := s.Connection.Cmd(cmd)
	if err != nil {
		return out, err
	}
	if strings.HasPrefix(out, "FAIL") {
		return out, fmt.Errorf("wifi: %s said %q", words[0], strings.TrimSpace(out))
	}
	return out, nil
}

// A scan holds the control socket for its whole run.
func (n NetworksClient) Scan(ctx context.Context) ([]Network, error) {
	c, err := n.Dial()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return Scanned(ctx, Commands{c})
}

func (n NetworksClient) Networks() ([]string, error) {
	c, err := n.Dial()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	return Configured(Commands{c})
}

func (n NetworksClient) Join(ctx context.Context, ssid, passphrase string) error {
	c, err := n.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	return Switch(ctx, Commands{c}, ssid, passphrase)
}

func (n NetworksClient) Move(
	ctx context.Context,
	ssid, passphrase string,
	ready func(context.Context) error,
) error {
	c, err := n.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	return Move(ctx, Commands{c}, ssid, passphrase, ready)
}

func (n NetworksClient) Forget(ssid string) error {
	c, err := n.Dial()
	if err != nil {
		return err
	}
	defer c.Close()

	now, err := Status(Commands{c})
	if err != nil {
		return err
	}
	if now.SSID == ssid {
		return fmt.Errorf("wifi: %q is the network in use", ssid)
	}
	return Remove(Commands{c}, ssid)
}
