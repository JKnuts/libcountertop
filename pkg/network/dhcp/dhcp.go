package dhcp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/hook"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const Interface = "wlan0"

type Client struct {
	options Options
	Leased  hook.Hook[*Lease]

	mu    sync.Mutex
	lease *Lease
}

func (c *Client) Name() string { return "dhcp" }

func (c *Client) Wait(ctx context.Context) bool {
	got := make(chan struct{}, 1)
	stop := c.Leased.Listen(func(l *Lease) {
		if l == nil {
			return
		}
		select {
		case got <- struct{}{}:
		default:
		}
	})
	defer stop()
	if c.Lease() != nil {
		return true
	}
	select {
	case <-got:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Client) Fresh(ctx context.Context) error {
	got := make(chan struct{}, 1)
	stop := c.Leased.Listen(func(l *Lease) {
		if l == nil {
			return
		}
		select {
		case got <- struct{}{}:
		default:
		}
	})
	defer stop()
	select {
	case <-got:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("dhcp: no address: %w", ctx.Err())
	}
}

func (c *Client) Lease() *Lease {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lease
}

func (c *Client) Run(ctx context.Context) error {
	for {
		if err := c.waitLink(ctx); err != nil {
			return err
		}

		lease, err := c.obtain(ctx)
		if err != nil {
			slog.Warn("no lease", "err", err)
			if err := sleep(ctx, 5*time.Second); err != nil {
				return err
			}
			continue
		}

		if taken, err := c.contested(ctx, lease); err != nil {
			return err
		} else if taken {
			continue
		}

		if err := apply(c.iface(), lease); err != nil {
			return err
		}
		useDNS(lease.DNS)
		if err := useNetd(c.iface(), c.Lease(), lease); err != nil {
			slog.Warn("netd would not take the lease", "err", err)
		}

		safe.Go("arp announce", func() {
			if err := announce(c.iface(), lease.Address.IP); err != nil {
				slog.Warn("could not announce the address", "err", err)
			}
		})
		c.set(lease)
		c.remember(lease.Address.IP)
		slog.Info("address", "lease", lease.String())

		watch, stop := context.WithCancel(ctx)
		taken := make(chan struct{}, 1)
		safe.Go("arp defend", func() {
			err := watching(watch, c.iface(), lease.Address.IP, func() {
				select {
				case taken <- struct{}{}:
				default:
				}
			})
			if err != nil {
				slog.Warn("not watching for address conflicts", "err", err)
			}
		})

		err = c.hold(ctx, lease, c.network(), taken)
		stop()

		if err != nil {
			return err
		}
	}
}

func (c *Client) obtain(ctx context.Context) (*Lease, error) {
	held := c.Lease()
	if held == nil {
		return dhcpRequest(ctx, c.iface(), c.hostname(), c.previous())
	}

	got, err := extend(ctx, c.iface(), held, c.hostname())
	switch {
	case err == nil:
		return got, nil
	case ctx.Err() != nil:
		return nil, err
	}

	slog.Warn("could not renew the address", "address", held.Address.IP, "err", err)
	c.set(nil)
	release(c.iface())

	return dhcpRequest(ctx, c.iface(), c.hostname(), c.previous())
}

func (c *Client) Close() error {
	if lease := c.Lease(); lease != nil {
		if err := releasing(c.iface(), lease); err != nil {
			slog.Warn("could not release the address", "err", err)
		}
	}

	c.set(nil)
	release(c.iface())
	return nil
}

func (c *Client) Release() error { return nil }

func (c *Client) set(l *Lease) {
	c.mu.Lock()
	was := c.lease
	c.lease = l
	c.mu.Unlock()
	if l != was {
		c.Leased.Emit(l)
	}
}

// RFC 2131 3.1.5: wait at least ten seconds after a DHCPDECLINE.
const backoff = 10 * time.Second

var (
	probing   = probe
	declining = sendDecline

	releasing = sendRelease
)

func (c *Client) contested(ctx context.Context, lease *Lease) (bool, error) {
	if held := c.Lease(); held != nil && held.Address.IP.Equal(lease.Address.IP) {
		return false, nil
	}

	taken, err := probing(c.iface(), lease.Address.IP)
	if err != nil {
		slog.Warn(
			"could not check whether the address is free",
			"address",
			lease.Address.IP,
			"err",
			err,
		)
		return false, nil
	}
	if !taken {
		return false, nil
	}

	slog.Warn(
		"refusing an address something else is using",
		"address",
		lease.Address.IP,
		"server",
		lease.Server,
	)

	if err := declining(
		c.iface(),
		lease.Address.IP,
		lease.Server,
		"address already in use",
	); err != nil {
		slog.Warn("could not tell the server", "err", err)
	}
	return true, sleep(ctx, backoff)
}

func (c *Client) hold(ctx context.Context, lease *Lease, on string, taken <-chan struct{}) error {
	renew := time.NewTimer(time.Until(lease.Renew))
	defer renew.Stop()

	check := time.NewTicker(15 * time.Second)
	defer check.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-renew.C:
			return nil
		case <-taken:
			if err := declining(
				c.iface(),
				lease.Address.IP,
				lease.Server,
				"address in use elsewhere",
			); err != nil {
				slog.Warn("could not tell the server", "err", err)
			}
			c.set(nil)
			release(c.iface())
			return sleep(ctx, backoff)
		case <-check.C:
			if !linkUp(c.iface()) {
				slog.Warn("link went away, asking again")
				c.set(nil)
				return nil
			}
			now := c.network()
			if on == "" {
				on = now
			} else if now != "" && now != on {
				slog.Warn("network changed, asking again", "was", on, "now", now)
				c.set(nil)
				release(c.iface())
				c.remember(nil)
				return nil
			}
		}
	}
}

func (c *Client) waitLink(ctx context.Context) error {
	for {
		if linkUp(c.iface()) {
			return nil
		}
		if err := sleep(ctx, time.Second); err != nil {
			return err
		}
	}
}

func linkUp(iface string) bool {
	b, err := os.ReadFile("/sys/class/net/" + iface + "/operstate")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "up"
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
