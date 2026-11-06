package address

import (
	"context"
	"net"
	"testing"
	"time"
)

func reads(each ...[]string) func() []net.IP {
	var at int
	return func() []net.IP {
		v := each[min(at, len(each)-1)]
		at++

		out := make([]net.IP, 0, len(v))
		for _, s := range v {
			out = append(out, net.ParseIP(s))
		}
		return out
	}
}

const quick = time.Millisecond

func TestItWaitsForDHCPToAnswer(t *testing.T) {
	read := reads(
		[]string{"fd06::1"},
		[]string{"fd06::1", "10.0.0.5"},
		[]string{"fd06::1", "10.0.0.5"},
	)

	got := settled(context.Background(), quick, 0, read)
	if key := AddressKey(got); key != "10.0.0.5,fd06::1" {
		t.Errorf("settled on %q, want both addresses", key)
	}
}

func TestAStableIPv6AddressWaitsForIPv4(t *testing.T) {
	read := reads(
		[]string{"fd06::1"},
		[]string{"fd06::1"},
		[]string{"fd06::1"},
		[]string{"fd06::1", "10.0.0.5"},
		[]string{"fd06::1", "10.0.0.5"},
	)

	got := settled(context.Background(), quick, time.Hour, read)
	if key := AddressKey(got); key != "10.0.0.5,fd06::1" {
		t.Errorf("settled on %q, want it to have waited for IPv4", key)
	}
}

func TestAnIPv6OnlyNetworkSettlesAfterTheGrace(t *testing.T) {
	got := settled(context.Background(), quick, 5*quick, reads([]string{"fd06::1"}))
	if key := AddressKey(got); key != "fd06::1" {
		t.Errorf("settled on %q, want the IPv6 address once the grace ran out", key)
	}
}

func TestAStableDeviceSettlesAtOnce(t *testing.T) {
	read := reads([]string{"10.0.0.5"})

	start := time.Now()
	got := settled(context.Background(), 20*time.Millisecond, 0, read)

	if key := AddressKey(got); key != "10.0.0.5" {
		t.Errorf("settled on %q", key)
	}
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Errorf("took %s to settle on an address that never changed", took)
	}
}

func TestNoAddressIsNotSettled(t *testing.T) {
	read := reads(
		nil,
		nil,
		nil,
		[]string{"10.0.0.5"},
		[]string{"10.0.0.5"},
	)

	if key := AddressKey(settled(context.Background(), quick, 0, read)); key != "10.0.0.5" {
		t.Errorf("settled on %q, want it to have waited for an address", key)
	}
}

func TestGivingUpReturnsNothing(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	stop()

	if got := settled(ctx, time.Hour, 0, reads([]string{"10.0.0.5"})); got != nil {
		t.Errorf("returned %v after being told to stop", got)
	}
}

func TestAChangingAddressIsNeverSettled(t *testing.T) {
	var at int
	read := func() []net.IP {
		at++
		return []net.IP{net.ParseIP("10.0.0." + string(rune('0'+at%8)))}
	}

	ctx, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()

	if got := settled(ctx, quick, 0, read); got != nil {
		t.Errorf("settled on %v while the address was still moving", got)
	}
}
