package address

import (
	"net"

	"github.com/jsimonetti/rtnetlink"
	"golang.org/x/sys/unix"
)

const unstable = unix.IFA_F_DEPRECATED | unix.IFA_F_TENTATIVE | unix.IFA_F_DADFAILED

func addresses() []net.IP {
	conn, err := rtnetlink.Dial(nil)
	if err != nil {
		return nil
	}
	defer conn.Close()

	msgs, err := conn.Address.List()
	if err != nil {
		return nil
	}

	var ips []net.IP
	for _, m := range msgs {
		if m.Attributes == nil || m.Attributes.Address == nil {
			continue
		}
		if !stable(m.Family, uint32(m.Flags)|m.Attributes.Flags) || !usable(m.Attributes.Address) {
			continue
		}
		ips = append(ips, m.Attributes.Address)
	}
	return ips
}

func stable(family uint8, flags uint32) bool {
	if family == unix.AF_INET6 && flags&unix.IFA_F_TEMPORARY != 0 {
		return false
	}
	return flags&unstable == 0
}
