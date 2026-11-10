package wifi

import (
	"context"
	"strings"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/wpa"
)

const scanMost = 10 * time.Second

func (r *Radio) Scan(ctx context.Context) ([]wpa.Network, error) {
	done := make(chan struct{}, 1)
	stop := r.scanned.Listen(func(struct{}) {
		select {
		case done <- struct{}{}:
		default:
		}
	})
	defer stop()

	c, err := r.Dial()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	cmds := wpa.Commands{Connection: c}

	// FAIL-BUSY: the supplicant is already scanning.
	if out, err := cmds.Cmd("scan"); err != nil && !strings.Contains(out, "FAIL-BUSY") {
		return nil, err
	}

	wait := time.NewTimer(scanMost)
	defer wait.Stop()
	select {
	case <-done:
	case <-wait.C:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return wpa.Results(cmds)
}

func (r *Radio) Networks() ([]string, error) { return r.networks.Networks() }

func (r *Radio) Join(ctx context.Context, ssid, passphrase string) error {
	return r.networks.Join(ctx, ssid, passphrase)
}

func (r *Radio) Move(
	ctx context.Context,
	ssid, passphrase string,
	ready func(context.Context) error,
) error {
	return r.networks.Move(ctx, ssid, passphrase, ready)
}

func (r *Radio) Forget(ssid string) error { return r.networks.Forget(ssid) }
