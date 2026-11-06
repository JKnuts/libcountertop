package address

import (
	"context"
	"net"
	"slices"
	"time"
)

const ipv4Grace = 30 * time.Second

func Settled(ctx context.Context, within time.Duration) []net.IP {
	return settled(ctx, within, ipv4Grace, Addresses)
}

func settled(
	ctx context.Context,
	within, grace time.Duration,
	read func() []net.IP,
) []net.IP {
	deadline := time.Now().Add(grace)
	was := read()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(within):
		}

		now := read()
		if len(now) > 0 && AddressKey(now) == AddressKey(was) &&
			(hasIPv4(now) || !time.Now().Before(deadline)) {
			return now
		}
		was = now
	}
}

func hasIPv4(ips []net.IP) bool {
	return slices.ContainsFunc(ips, func(ip net.IP) bool { return ip.To4() != nil })
}
