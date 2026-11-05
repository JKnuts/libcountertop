package address

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestOnlyStableAddressesAreKept(t *testing.T) {
	for _, tc := range []struct {
		name   string
		family uint8
		flags  uint32
		want   bool
	}{
		{"permanent", unix.AF_INET6, unix.IFA_F_PERMANENT, true},
		{"slaac", unix.AF_INET6, unix.IFA_F_MANAGETEMPADDR, true},
		{"privacy", unix.AF_INET6, unix.IFA_F_TEMPORARY, false},
		{"expired privacy", unix.AF_INET6, unix.IFA_F_TEMPORARY | unix.IFA_F_DEPRECATED, false},
		{"deprecated", unix.AF_INET6, unix.IFA_F_DEPRECATED, false},
		{"tentative", unix.AF_INET6, unix.IFA_F_TENTATIVE, false},
		{"duplicate", unix.AF_INET6, unix.IFA_F_DADFAILED | unix.IFA_F_TENTATIVE, false},
		{"ipv4", unix.AF_INET, unix.IFA_F_PERMANENT, true},
		{"ipv4 secondary", unix.AF_INET, unix.IFA_F_SECONDARY | unix.IFA_F_PERMANENT, true},
		{"ipv4 deprecated", unix.AF_INET, unix.IFA_F_DEPRECATED, false},
	} {
		if got := stable(tc.family, tc.flags); got != tc.want {
			t.Errorf(
				"%s: stable(%d, %#x) = %v, want %v",
				tc.name,
				tc.family,
				tc.flags,
				got,
				tc.want,
			)
		}
	}
}
