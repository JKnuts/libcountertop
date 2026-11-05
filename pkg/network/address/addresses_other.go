//go:build !linux

package address

import "net"

func addresses() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	var ips []net.IP
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if ok && usable(ipnet.IP) {
			ips = append(ips, ipnet.IP)
		}
	}
	return ips
}
