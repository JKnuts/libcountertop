//go:build linux

package netcmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	pcap "github.com/huatuo-ai/go-pcap"
	"github.com/jsimonetti/rtnetlink"
	"github.com/mdlayher/arp"
	"github.com/mdlayher/wifi"
	"golang.org/x/sys/unix"

	sharedwifi "github.com/ygelfand/libcountertop/pkg/network/wifi"
)

const (
	readWait = 500 * time.Millisecond
	pingWait = time.Second
	userHZ   = 100
)

func open(ctx context.Context, iface, filter string) (*pcap.Handle, error) {
	h, err := pcap.OpenLiveWithContext(ctx, iface, snaplen, false, readWait, false)
	if err != nil {
		return nil, fmt.Errorf("capturing on %s: %w", iface, err)
	}
	if err := h.SetBPFFilter(filter); err != nil {
		h.Close()
		return nil, fmt.Errorf("filter %q: %w", filter, err)
	}
	return h, nil
}

func read(ctx context.Context, h *pcap.Handle) ([]byte, gopacket.CaptureInfo, bool, error) {
	for ctx.Err() == nil {
		data, ci, err := h.ReadPacketData()
		switch {
		case err != nil && !errors.Is(err, context.DeadlineExceeded):
			return nil, ci, false, err
		case len(data) > 0:
			return data, ci, true, nil
		}
	}
	return nil, gopacket.CaptureInfo{}, false, nil
}

func capture(
	ctx context.Context,
	iface, filter string,
	wait time.Duration,
	path string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	h, err := open(ctx, iface, filter)
	if err != nil {
		return "", err
	}
	defer h.Close()

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(snaplen, layers.LinkTypeEthernet); err != nil {
		return "", err
	}

	var count int
	for {
		data, ci, ok, err := read(ctx, h)
		if err != nil {
			return "", err
		}
		if !ok {
			break
		}
		if err := w.WritePacket(ci, data); err != nil {
			return "", err
		}
		count++
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d packets matching %q in %s to %s", count, filter, wait, path), nil
}

func arpWatch(ctx context.Context, iface string, count int, wait time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	h, err := open(ctx, iface, "arp")
	if err != nil {
		return "", err
	}
	defer h.Close()

	var out strings.Builder
	for seen := 0; seen < count; {
		data, ci, ok, err := read(ctx, h)
		if err != nil {
			return out.String(), err
		}
		if !ok {
			fmt.Fprintf(&out, "%d of %d in %s", seen, count, wait)
			break
		}
		p := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)
		eth, _ := p.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
		a, _ := p.Layer(layers.LayerTypeARP).(*layers.ARP)
		if eth == nil || a == nil {
			continue
		}
		seen++
		fmt.Fprintf(&out, "%s %s > %s %s\n",
			ci.Timestamp.UTC().Format("15:04:05.000"),
			eth.SrcMAC, eth.DstMAC, describe(a))
	}
	return out.String(), nil
}

func describe(a *layers.ARP) string {
	sender, target := net.IP(a.SourceProtAddress), net.IP(a.DstProtAddress)
	switch a.Operation {
	case layers.ARPRequest:
		return fmt.Sprintf("who-has %s tell %s", target, sender)
	case layers.ARPReply:
		return fmt.Sprintf("%s is-at %s", sender, net.HardwareAddr(a.SourceHwAddress))
	}
	return fmt.Sprintf("op %d %s > %s", a.Operation, sender, target)
}

func arpPing(ctx context.Context, iface string, ip netip.Addr, count int) (string, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return "", err
	}
	c, err := arp.Dial(ifi)
	if err != nil {
		return "", fmt.Errorf("arp on %s: %w", iface, err)
	}
	defer c.Close()

	var out strings.Builder
	var answered int
	for i := range count {
		if ctx.Err() != nil {
			break
		}
		start := time.Now()
		if err := c.SetDeadline(start.Add(pingWait)); err != nil {
			return out.String(), err
		}
		mac, err := c.Resolve(ip)
		took := time.Since(start)
		if err != nil {
			fmt.Fprintf(&out, "%d no reply in %s\n", i, pingWait)
		} else {
			answered++
			fmt.Fprintf(&out, "%d %s is-at %s in %s\n", i, ip, mac, took.Round(time.Millisecond))
		}
		if rest := pingWait - took; rest > 0 && i < count-1 {
			select {
			case <-ctx.Done():
			case <-time.After(rest):
			}
		}
	}
	fmt.Fprintf(&out, "%d of %d answered", answered, count)
	return out.String(), nil
}

func neighbors(iface string) (string, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return "", err
	}
	conn, err := rtnetlink.Dial(nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	all, err := conn.Neigh.List()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, n := range all {
		if int(n.Index) != ifi.Index || n.Attributes == nil || n.Attributes.Address == nil {
			continue
		}
		fmt.Fprintf(
			&out,
			"%s\t%s\t%s",
			n.Attributes.Address,
			n.Attributes.LLAddress,
			state(n.State),
		)
		if ci := n.Attributes.CacheInfo; ci != nil {
			fmt.Fprintf(&out, "\tconfirmed %s used %s updated %s ago",
				ticks(ci.Confirmed), ticks(ci.Used), ticks(ci.Updated))
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}

// nda_cacheinfo ages are in USER_HZ ticks, 100 per second.
func ticks(t uint32) time.Duration {
	return (time.Duration(t) * time.Second / userHZ).Round(10 * time.Millisecond)
}

var states = []struct {
	bit  uint16
	name string
}{
	{unix.NUD_INCOMPLETE, "incomplete"},
	{unix.NUD_REACHABLE, "reachable"},
	{unix.NUD_STALE, "stale"},
	{unix.NUD_DELAY, "delay"},
	{unix.NUD_PROBE, "probe"},
	{unix.NUD_FAILED, "failed"},
	{unix.NUD_NOARP, "noarp"},
	{unix.NUD_PERMANENT, "permanent"},
}

func state(s uint16) string {
	var names []string
	for _, st := range states {
		if s&st.bit != 0 {
			names = append(names, st.name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}

func stats(iface string) (string, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return "", err
	}
	conn, err := rtnetlink.Dial(nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	link, err := conn.Link.Get(uint32(ifi.Index))
	if err != nil {
		return "", err
	}
	if link.Attributes == nil || link.Attributes.Stats64 == nil {
		return "", fmt.Errorf("%s reported no statistics", iface)
	}
	s := link.Attributes.Stats64
	return fmt.Sprintf(
		"rx packets %d bytes %d errors %d dropped %d multicast %d crc %d fifo %d missed %d\n"+
			"tx packets %d bytes %d errors %d dropped %d carrier %d fifo %d",
		s.RXPackets, s.RXBytes, s.RXErrors, s.RXDropped, s.Multicast,
		s.RXCRCErrors, s.RXFIFOErrors, s.RXMissedErrors,
		s.TXPackets, s.TXBytes, s.TXErrors, s.TXDropped, s.TXCarrierErrors, s.TXFIFOErrors,
	), nil
}

func radio(iface string) (string, error) {
	c, err := wifi.New()
	if err != nil {
		return "", err
	}
	defer c.Close()
	ifis, err := c.Interfaces()
	if err != nil {
		return "", err
	}
	var ifi *wifi.Interface
	for _, i := range ifis {
		if i.Name == iface {
			ifi = i
		}
	}
	if ifi == nil {
		return "", fmt.Errorf("%s is not a wifi interface nl80211 knows", iface)
	}

	var out strings.Builder
	if bss, err := c.BSS(ifi); err != nil {
		fmt.Fprintf(&out, "bss: %v\n", err)
	} else {
		fmt.Fprintf(&out, "bssid %s ssid %q frequency %d MHz beacon %s\n",
			bss.BSSID, bss.SSID, bss.Frequency, bss.BeaconInterval)
	}
	if sta, err := c.StationInfo(ifi); err != nil {
		fmt.Fprintf(&out, "station: %v\n", err)
	} else {
		for _, s := range sta {
			fmt.Fprintf(&out,
				"signal %d dBm (average %d) rx %d bit/s tx %d bit/s connected %s inactive %s "+
					"beacon loss %d tx retries %d tx failed %d\n",
				s.Signal, s.SignalAverage, s.ReceiveBitrate, s.TransmitBitrate,
				s.Connected, s.Inactive, s.BeaconLoss, s.TransmitRetries, s.TransmitFailed)
		}
	}
	if on, err := sharedwifi.PowerSave(iface); err != nil {
		fmt.Fprintf(&out, "power save: %v", err)
	} else {
		fmt.Fprintf(&out, "power save %s", map[bool]string{true: "on", false: "off"}[on])
	}
	return out.String(), nil
}
