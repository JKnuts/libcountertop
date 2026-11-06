package netcmd

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	harness "github.com/ygelfand/libcountertop/pkg/runtime/control"
)

const (
	snaplen      = 65535
	maxCapture   = 10 * time.Minute
	watchFor     = 30 * time.Second
	watchCount   = 10
	pingCount    = 5
	maxPingCount = 100
)

func Command(iface string) *cobra.Command {
	n := harness.Group("net", "Look at the network from this device", "")
	arp := harness.Group("arp", "ARP as this device sees and sends it", "")
	arp.AddCommand(
		harness.ContextSays(&cobra.Command{
			Use:   "watch [COUNT [SECONDS]]",
			Short: "Each ARP frame received: destination, request or reply, and addresses",
			Args:  cobra.MaximumNArgs(2),
		}, func(ctx context.Context, args []string) (string, error) {
			count, err := number(args, 0, watchCount, 1, 1000)
			if err != nil {
				return "", err
			}
			wait, err := duration(args, 1, watchFor)
			if err != nil {
				return "", err
			}
			return arpWatch(ctx, iface, count, wait)
		}),
		harness.ContextSays(&cobra.Command{
			Use:   "ping IP [COUNT]",
			Short: "Send ARP requests for an address and time the replies",
			Args:  cobra.RangeArgs(1, 2),
		}, func(ctx context.Context, args []string) (string, error) {
			ip, err := netip.ParseAddr(args[0])
			if err != nil || !ip.Is4() {
				return "", fmt.Errorf("%q is not an IPv4 address", args[0])
			}
			count, err := number(args, 1, pingCount, 1, maxPingCount)
			if err != nil {
				return "", err
			}
			return arpPing(ctx, iface, ip, count)
		}),
	)
	n.AddCommand(
		harness.ContextSays(&cobra.Command{
			Use:   "capture FILTER SECONDS PATH",
			Short: "Write the packets matching a tcpdump-style filter to a pcap file",
			Args:  cobra.ExactArgs(3),
		}, func(ctx context.Context, args []string) (string, error) {
			if args[0] == "" {
				return "", fmt.Errorf("capture needs a filter")
			}
			wait, err := duration(args, 1, 0)
			if err != nil {
				return "", err
			}
			return capture(ctx, iface, args[0], wait, args[2])
		}),
		arp,
		harness.Says(&cobra.Command{
			Use:   "neigh",
			Short: "The neighbor table, with how long ago each entry was confirmed, used and updated",
			Args:  cobra.NoArgs,
		}, func([]string) (string, error) { return neighbors(iface) }),
		harness.Says(&cobra.Command{
			Use:   "stats",
			Short: "The interface counters, drops and errors included",
			Args:  cobra.NoArgs,
		}, func([]string) (string, error) { return stats(iface) }),
		harness.Says(&cobra.Command{
			Use:   "wifi",
			Short: "The access point, signal, rates and power save",
			Args:  cobra.NoArgs,
		}, func([]string) (string, error) { return radio(iface) }),
	)
	return n
}

func number(args []string, at, fallback, lo, hi int) (int, error) {
	if at >= len(args) {
		return fallback, nil
	}
	n, err := strconv.Atoi(args[at])
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%q is not a number from %d to %d", args[at], lo, hi)
	}
	return n, nil
}

func duration(args []string, at int, fallback time.Duration) (time.Duration, error) {
	if at >= len(args) {
		return fallback, nil
	}
	n, err := strconv.Atoi(args[at])
	if err != nil || n < 1 || time.Duration(n)*time.Second > maxCapture {
		return 0, fmt.Errorf(
			"%q is not a number of seconds from 1 to %d",
			args[at],
			int(maxCapture.Seconds()),
		)
	}
	return time.Duration(n) * time.Second, nil
}
