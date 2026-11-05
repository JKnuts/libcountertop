package address

import (
	"net"
	"slices"
	"strings"
)

func Addresses() []net.IP {
	return addresses()
}

func AddressKey(ips []net.IP) string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}

func usable(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast()
}
